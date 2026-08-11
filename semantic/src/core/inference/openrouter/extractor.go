package openrouter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/hsme/core/src/core/worker"
)

type Client struct {
	BaseURL    string
	APIKey     string
	HTTPClient *http.Client
}

func NewClient(baseURL, apiKey string) *Client {
	if baseURL == "" {
		baseURL = "https://openrouter.ai/api/v1"
	}
	baseURL = strings.TrimRight(baseURL, "/")
	return &Client{
		BaseURL: baseURL,
		APIKey:  apiKey,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Minute,
		},
	}
}

type Extractor struct {
	client     *Client
	model      string
	maxBackoff time.Duration
}

func NewExtractor(client *Client, model string) *Extractor {
	if model == "" {
		model = "nvidia/nemotron-nano-9b-v2:free"
	}
	return &Extractor{
		client:     client,
		model:      model,
		maxBackoff: 2 * time.Second,
	}
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type responseFormat struct {
	Type string `json:"type"`
}

type chatCompletionRequest struct {
	Model          string          `json:"model"`
	Messages       []chatMessage   `json:"messages"`
	Temperature    float64         `json:"temperature"`
	ResponseFormat *responseFormat `json:"response_format,omitempty"`
}

type chatCompletionChoice struct {
	Message chatMessage `json:"message"`
}

type chatCompletionResponse struct {
	Choices []chatCompletionChoice `json:"choices"`
}

const systemPrompt = `You are a technical graph extractor. Analyze the provided text (may be in Spanish or English) and extract technical entities and their relationships. 

EXAMPLES:
Text: "El servicio Redis depende de Docker para funcionar."
Output: {"nodes": [{"type": "TECH", "name": "Redis"}, {"type": "TECH", "name": "Docker"}], "edges": [{"source": "Redis", "target": "Docker", "relation": "DEPENDS_ON"}]}

Text: "Fix: Corregido el bug en auth.go que causaba error 500."
Output: {"nodes": [{"type": "FILE", "name": "auth.go"}, {"type": "ERROR", "name": "error 500"}], "edges": [{"source": "auth.go", "target": "error 500", "relation": "RESOLVES"}]}

RULES:
- Return ONLY valid JSON. No prose, no markdown code blocks.
- Preserve technical names exactly (e.g. "Entidad Alfa").
- If no entities are found, return {"nodes": [], "edges": []}.`

func (e *Extractor) ExtractEntities(ctx context.Context, text string) (worker.KnowledgeGraph, error) {
	reqBody := chatCompletionRequest{
		Model: e.model,
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: "Text to analyze:\n" + text},
		},
		Temperature:    0.0,
		ResponseFormat: &responseFormat{Type: "json_object"},
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return worker.KnowledgeGraph{}, fmt.Errorf("failed to marshal openrouter extract request: %w", err)
	}

	url := strings.TrimRight(e.client.BaseURL, "/") + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return worker.KnowledgeGraph{}, fmt.Errorf("failed to create openrouter extract request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if e.client.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+e.client.APIKey)
	}

	resp, err := e.client.HTTPClient.Do(req)
	if err != nil {
		return worker.KnowledgeGraph{}, fmt.Errorf("failed to execute openrouter extract request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		backoff := 1 * time.Second
		if retryAfter := resp.Header.Get("Retry-After"); retryAfter != "" {
			if sec, err := strconv.Atoi(retryAfter); err == nil && sec > 0 {
				backoff = time.Duration(sec) * time.Second
			}
		}
		if e.maxBackoff > 0 && backoff > e.maxBackoff {
			backoff = e.maxBackoff
		}

		select {
		case <-time.After(backoff):
		case <-ctx.Done():
			return worker.KnowledgeGraph{}, ctx.Err()
		}

		return worker.KnowledgeGraph{}, fmt.Errorf("openrouter rate limit exceeded (HTTP 429)")
	}

	if resp.StatusCode != http.StatusOK {
		return worker.KnowledgeGraph{}, fmt.Errorf("openrouter API returned status %d for extraction", resp.StatusCode)
	}

	var chatResp chatCompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return worker.KnowledgeGraph{}, fmt.Errorf("failed to decode openrouter extract response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return worker.KnowledgeGraph{}, fmt.Errorf("openrouter extract response contained no choices")
	}

	rawContent := chatResp.Choices[0].Message.Content
	kg, err := parseExtractedKG(rawContent)
	if err != nil {
		return worker.KnowledgeGraph{}, fmt.Errorf("failed to parse extracted graph: %w", err)
	}

	return kg, nil
}

func parseExtractedKG(raw string) (worker.KnowledgeGraph, error) {
	var kg worker.KnowledgeGraph

	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return worker.KnowledgeGraph{}, nil
	}

	if err := json.NewDecoder(strings.NewReader(trimmed)).Decode(&kg); err == nil {
		return kg, nil
	}

	if start := strings.Index(trimmed, "{"); start >= 0 {
		depth := 0
		inString := false
		escape := false
		for i := start; i < len(trimmed); i++ {
			c := trimmed[i]
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
					if err := json.Unmarshal([]byte(trimmed[start:i+1]), &kg); err == nil {
						return kg, nil
					}
				}
			}
		}
	}

	return worker.KnowledgeGraph{}, fmt.Errorf("unparseable graph content: %s", truncateString(trimmed, 100))
}

func truncateString(s string, maxLen int) string {
	if len(s) > maxLen {
		return s[:maxLen] + "..."
	}
	return s
}
