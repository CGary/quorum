package openrouter

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

func TestExtractEntities_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST request, got %s", r.Method)
		}
		if r.URL.Path != "/chat/completions" {
			t.Errorf("expected path /chat/completions, got %s", r.URL.Path)
		}
		if auth := r.Header.Get("Authorization"); auth != "Bearer test-api-key" {
			t.Errorf("expected Authorization Bearer test-api-key, got %s", auth)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"choices": [
				{
					"message": {
						"role": "assistant",
						"content": "{\"nodes\": [{\"type\": \"TECH\", \"name\": \"Redis\"}], \"edges\": []}"
					}
				}
			]
		}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-api-key")
	extractor := NewExtractor(client, "nvidia/nemotron-nano-9b-v2:free")

	kg, err := extractor.ExtractEntities(context.Background(), "El servicio Redis funciona.")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(kg.Nodes) != 1 || kg.Nodes[0].Name != "Redis" {
		t.Errorf("expected 1 node named Redis, got %+v", kg.Nodes)
	}
}

func TestExtractEntities_RateLimit429(t *testing.T) {
	var requestCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requestCount, 1)
		w.Header().Set("Retry-After", "1")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	client := NewClient(server.URL, "")
	extractor := NewExtractor(client, "")
	extractor.maxBackoff = 10 * time.Millisecond

	_, err := extractor.ExtractEntities(context.Background(), "some text")
	if err == nil {
		t.Fatal("expected error on 429 response, got nil")
	}

	count := atomic.LoadInt32(&requestCount)
	if count != 1 {
		t.Errorf("expected exactly 1 request (no retry loop), got %d", count)
	}
}

func TestExtractEntities_MalformedJSONResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<html>Bad Gateway</html>`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "")
	extractor := NewExtractor(client, "")

	_, err := extractor.ExtractEntities(context.Background(), "some text")
	if err == nil {
		t.Fatal("expected parse error on malformed response body, got nil")
	}
}

func TestExtractEntities_MalformedContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"choices": [
				{
					"message": {
						"role": "assistant",
						"content": "this is not json content"
					}
				}
			]
		}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "")
	extractor := NewExtractor(client, "")

	_, err := extractor.ExtractEntities(context.Background(), "some text")
	if err == nil {
		t.Fatal("expected parse error on malformed content, got nil")
	}
}
