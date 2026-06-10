package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/hsme/core/src/core/worker"
)

// parseExtractedKG tolera dos vicios típicos de modelos pequeños como phi3.5:
//   1) Emiten prosa después del JSON ("Given the provided text...").
//   2) Se cuelgan a mitad del JSON y emiten texto suelto.
// Estrategia: intentar decode directo (ignora basura al final) y, si falla,
// buscar el PRIMER objeto { ... } balanceado en el texto y reintentar.
// Si nada parsea, devolver KG vacío — la memoria sigue indexada por FTS/vector,
// y NO queremos quemar 5 intentos por un output que nunca va a mejorar.
func parseExtractedKG(raw string) worker.KnowledgeGraph {
	var kg worker.KnowledgeGraph

	if err := json.NewDecoder(strings.NewReader(raw)).Decode(&kg); err == nil {
		return kg
	}

	if start := strings.Index(raw, "{"); start >= 0 {
		depth := 0
		inString := false
		escape := false
		for i := start; i < len(raw); i++ {
			c := raw[i]
			if escape {
				escape = false
				continue
			}
			if c == '\\' {
				escape = true
				continue
			}
			if c == '"' {
				inString = !inString
				continue
			}
			if inString {
				continue
			}
			switch c {
			case '{':
				depth++
			case '}':
				depth--
				if depth == 0 {
					if err := json.Unmarshal([]byte(raw[start:i+1]), &kg); err == nil {
						return kg
					}
					return worker.KnowledgeGraph{}
				}
			}
		}
	}
	return worker.KnowledgeGraph{}
}

type Extractor struct {
	client *Client
	model  string
}

func NewExtractor(client *Client, model string) *Extractor {
	if model == "" {
		model = "phi3.5"
	}
	return &Extractor{
		client: client,
		model:  model,
	}
}

func (e *Extractor) ExtractEntities(ctx context.Context, text string) (worker.KnowledgeGraph, error) {
        systemPrompt := `You are a technical graph extractor. Analyze the provided text (may be in Spanish or English) and extract technical entities and their relationships. 

EXAMPLES:
Text: "El servicio Redis depende de Docker para funcionar."
Output: {"nodes": [{"type": "TECH", "name": "Redis"}, {"type": "TECH", "name": "Docker"}], "edges": [{"source": "Redis", "target": "Docker", "relation": "DEPENDS_ON"}]}

Text: "Fix: Corregido el bug en auth.go que causaba error 500."
Output: {"nodes": [{"type": "FILE", "name": "auth.go"}, {"type": "ERROR", "name": "error 500"}], "edges": [{"source": "auth.go", "target": "error 500", "relation": "RESOLVES"}]}

RULES:
- Return ONLY valid JSON. No prose, no markdown code blocks.
- Preserve technical names exactly (e.g. "Entidad Alfa").
- If no entities are found, return {"nodes": [], "edges": []}.`

        reqBody := map[string]interface{}{		"model":  e.model,
		"prompt": systemPrompt + "\n\nText to analyze:\n" + text,
		"format": "json",
		"stream": false,
		"options": map[string]interface{}{
			"temperature": 0.0,
		},
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return worker.KnowledgeGraph{}, fmt.Errorf("failed to marshal extract request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", e.client.BaseURL+"/api/generate", bytes.NewBuffer(jsonData))
	if err != nil {
		return worker.KnowledgeGraph{}, fmt.Errorf("failed to create extract request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.client.HTTPClient.Do(req)
	if err != nil {
		return worker.KnowledgeGraph{}, fmt.Errorf("failed to execute extract request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return worker.KnowledgeGraph{}, fmt.Errorf("ollama API returned status %d for extraction", resp.StatusCode)
	}

	var resBody struct {
		Response string `json:"response"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&resBody); err != nil {
		return worker.KnowledgeGraph{}, fmt.Errorf("failed to decode extract response: %w", err)
	}

	kg := parseExtractedKG(resBody.Response)
	if len(kg.Nodes) == 0 && len(kg.Edges) == 0 && strings.TrimSpace(resBody.Response) != "" {
		// El modelo respondió pero no pudimos extraer nada útil. Log para diagnóstico,
		// pero no fallamos: la memoria queda indexada por FTS/vector igual.
		preview := resBody.Response
		if len(preview) > 200 {
			preview = preview[:200] + "…"
		}
		fmt.Fprintf(os.Stderr, "[extractor] parseo fallido o vacío, continúo sin KG (preview: %s)\n", preview)
	}
	return kg, nil
}
