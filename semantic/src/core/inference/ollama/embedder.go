package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type Embedder struct {
	client *Client
	model  string
	dim    int
}

func NewEmbedder(client *Client, model string, dim int) *Embedder {
	if model == "" {
		model = "nomic-embed-text"
	}
	if dim == 0 {
		dim = 768
	}
	return &Embedder{
		client: client,
		model:  model,
		dim:    dim,
	}
}

func (e *Embedder) GenerateVector(ctx context.Context, text string) ([]float32, error) {
	reqBody := map[string]interface{}{
		"model":  e.model,
		"prompt": text,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal embed request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", e.client.BaseURL+"/api/embeddings", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create embed request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.client.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute embed request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		if len(body) == 0 {
			return nil, fmt.Errorf("ollama API returned status %d for embeddings", resp.StatusCode)
		}
		return nil, fmt.Errorf("ollama API returned status %d for embeddings: %s", resp.StatusCode, string(body))
	}

	var resBody struct {
		Embedding []float64 `json:"embedding"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&resBody); err != nil {
		return nil, fmt.Errorf("failed to decode embed response: %w", err)
	}

	vector := make([]float32, len(resBody.Embedding))
	for i, v := range resBody.Embedding {
		vector[i] = float32(v)
	}

	return vector, nil
}

func (e *Embedder) Dimension() int {
	return e.dim
}

// ModelID retorna el identificador estable del modelo para el registro de
// configuración (§4.2). Se compara contra system_config.embedding_model al
// arrancar para detectar cambios incompatibles sin reindex.
func (e *Embedder) ModelID() string {
	return e.model
}
