package core

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func newTestJevClient(serverURL string, maxRetries int) *HTTPJevClient {
	return &HTTPJevClient{
		URL:        serverURL,
		APIKey:     "sk-super-secret-999",
		HTTP:       &http.Client{},
		MaxRetries: maxRetries,
		Sleep:      func(time.Duration) {},
	}
}

func TestHTTPJevClientDecide200(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		if r.Header.Get("Authorization") != "Bearer sk-super-secret-999" {
			t.Errorf("missing bearer auth header")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"classifier-v1","id":"resp-1","provider":"openrouter","answers":{"target_model":{"type":"choice","choice":"b"}},"usage":{"input_tokens":9,"output_tokens":2,"cost":0.001}}`))
	}))
	defer srv.Close()

	resp, jerr := newTestJevClient(srv.URL, 2).Decide(context.Background(), JevRequest{})
	if jerr != nil {
		t.Fatalf("unexpected error: %v", jerr)
	}
	if resp.ID != "resp-1" || resp.Provider != "openrouter" || resp.Answers["target_model"].Choice != "b" {
		t.Fatalf("resp %+v", resp)
	}
	if resp.Usage.Cost == nil || *resp.Usage.Cost != 0.001 {
		t.Fatalf("cost %v want 0.001", resp.Usage.Cost)
	}
}

func TestParseJevResponseProviderAsObject(t *testing.T) {
	resp, err := ParseJevResponse([]byte(`{"model":"m","provider":{"name":"openrouter"},"answers":{}}`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if resp.Provider != `{"name":"openrouter"}` {
		t.Fatalf("provider %q", resp.Provider)
	}
}

func TestHTTPJevClientDecide422NotRetried(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"detail":"bad input"}`))
	}))
	defer srv.Close()

	_, jerr := newTestJevClient(srv.URL, 2).Decide(context.Background(), JevRequest{})
	if jerr == nil {
		t.Fatal("expected error")
	}
	if atomic.LoadInt32(&hits) != 1 {
		t.Fatalf("hits %d want 1 (non-retryable)", atomic.LoadInt32(&hits))
	}
	if jerr.Retryable {
		t.Fatal("422 must not be retryable")
	}
	if jerr.Message != "bad input" {
		t.Fatalf("message %q want 'bad input'", jerr.Message)
	}
}

func TestHTTPJevClientDecide429Then200(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&hits, 1) == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"detail":"rate limited"}`))
			return
		}
		_, _ = w.Write([]byte(`{"model":"m","answers":{"target_model":{"type":"choice","choice":"a"}},"usage":{}}`))
	}))
	defer srv.Close()

	resp, jerr := newTestJevClient(srv.URL, 2).Decide(context.Background(), JevRequest{})
	if jerr != nil {
		t.Fatalf("unexpected error: %v", jerr)
	}
	if resp == nil {
		t.Fatal("expected response after retry")
	}
	if atomic.LoadInt32(&hits) != 2 {
		t.Fatalf("hits %d want 2", atomic.LoadInt32(&hits))
	}
}

func TestHTTPJevClientDecide529Exhausted(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(529)
		_, _ = w.Write([]byte(`{"detail":"overloaded"}`))
	}))
	defer srv.Close()

	_, jerr := newTestJevClient(srv.URL, 2).Decide(context.Background(), JevRequest{})
	if jerr == nil || !jerr.Retryable {
		t.Fatalf("err %+v want retryable", jerr)
	}
	want := int32(1 + 2) // 1 initial + MaxRetries retries
	if atomic.LoadInt32(&hits) != want {
		t.Fatalf("hits %d want %d", atomic.LoadInt32(&hits), want)
	}
}

func TestHTTPJevClientDecide401Message(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"bad key"}}`))
	}))
	defer srv.Close()

	_, jerr := newTestJevClient(srv.URL, 2).Decide(context.Background(), JevRequest{})
	if jerr == nil {
		t.Fatal("expected error")
	}
	if jerr.Message != "bad key" {
		t.Fatalf("message %q want 'bad key'", jerr.Message)
	}
	if jerr.Retryable {
		t.Fatal("401 must not be retryable")
	}
}

func TestHTTPJevClientKeyNeverInMessage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"detail":"something broke"}`))
	}))
	defer srv.Close()

	c := newTestJevClient(srv.URL, 2)
	_, jerr := c.Decide(context.Background(), JevRequest{})
	if jerr == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(jerr.Message, c.APIKey) {
		t.Fatalf("API key leaked into message %q", jerr.Message)
	}
}

func TestHTTPJevClientCtxTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()

	_, jerr := newTestJevClient(srv.URL, 2).Decide(ctx, JevRequest{})
	if jerr == nil {
		t.Fatal("expected error")
	}
	if !jerr.Retryable || jerr.Message != "timeout" {
		t.Fatalf("err %+v want retryable timeout", jerr)
	}
}

func TestNewJevClientMissingEnv(t *testing.T) {
	p := validJevPolicy()
	_, err := NewJevClient(p, "typesafe", "", func(string) string { return "" })
	if err == nil {
		t.Fatal("expected error for missing env var")
	}
	if !strings.Contains(err.Error(), "JEV_API_KEY") {
		t.Fatalf("error %q must name the env var", err.Error())
	}
	if strings.Contains(err.Error(), "secret") {
		t.Fatalf("error %q must never name a value", err.Error())
	}
}

func TestNewJevClientUnknownProvider(t *testing.T) {
	p := validJevPolicy()
	_, err := NewJevClient(p, "nope", "", func(string) string { return "x" })
	if err == nil {
		t.Fatal("expected error for unknown provider")
	}
}

func TestNewJevClientURLOverride(t *testing.T) {
	p := validJevPolicy()
	c, err := NewJevClient(p, "typesafe", "http://override.example", func(string) string { return "k" })
	if err != nil {
		t.Fatal(err)
	}
	if c.URL != "http://override.example" {
		t.Fatalf("url %q", c.URL)
	}
}

// TestHTTPJevClientDecideRequestShape asserts the serialized request carries the
// model and state the server can observe.
func TestHTTPJevClientDecideRequestShape(t *testing.T) {
	var got JevRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = w.Write([]byte(`{"model":"m","answers":{},"usage":{}}`))
	}))
	defer srv.Close()

	req := JevRequest{Model: "classifier-v1", State: map[string]string{"prompt": "hi"}}
	_, jerr := newTestJevClient(srv.URL, 0).Decide(context.Background(), req)
	if jerr != nil {
		t.Fatal(jerr)
	}
	if got.Model != "classifier-v1" || got.State["prompt"] != "hi" {
		t.Fatalf("request %+v", got)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type errReader struct{ err error }

func (e errReader) Read([]byte) (int, error) { return 0, e.err }
func (e errReader) Close() error             { return nil }

func TestHTTPJevClientMaxRetriesNegative(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		_, _ = w.Write([]byte(`{"model":"m","answers":{},"usage":{}}`))
	}))
	defer srv.Close()

	resp, jerr := newTestJevClient(srv.URL, -1).Decide(context.Background(), JevRequest{})
	if jerr != nil {
		t.Fatalf("unexpected error: %v", jerr)
	}
	if resp == nil {
		t.Fatal("expected response")
	}
	if atomic.LoadInt32(&hits) != 1 {
		t.Fatalf("hits %d want 1", atomic.LoadInt32(&hits))
	}
}

func TestHTTPJevClientNilHTTPUsesDefaultClient(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"model":"m","answers":{},"usage":{}}`))
	}))
	defer srv.Close()

	c := &HTTPJevClient{URL: srv.URL, APIKey: "k", MaxRetries: 0, Sleep: func(time.Duration) {}}
	_, jerr := c.Decide(context.Background(), JevRequest{})
	if jerr != nil {
		t.Fatalf("unexpected error: %v", jerr)
	}
}

func TestHTTPJevClientCanceledAtTopOfAttempt(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var slept bool
	c := newTestJevClient("http://example.invalid", 2)
	c.Sleep = func(time.Duration) { slept = true }

	_, jerr := c.Decide(ctx, JevRequest{})
	if jerr == nil || jerr.Message != "canceled" || jerr.Retryable {
		t.Fatalf("err %+v want canceled non-retryable", jerr)
	}
	if slept {
		t.Fatal("must not sleep on cancellation")
	}
}

func TestHTTPJevClientCanceledAfterFailedAttempt(t *testing.T) {
	var slept bool
	ctx, cancel := context.WithCancel(context.Background())
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cancel()
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := newTestJevClient(srv.URL, 2)
	c.Sleep = func(time.Duration) { slept = true }

	_, jerr := c.Decide(ctx, JevRequest{})
	if jerr == nil || jerr.Message != "canceled" || jerr.Retryable {
		t.Fatalf("err %+v want canceled non-retryable", jerr)
	}
	if slept {
		t.Fatal("must not sleep after cancellation")
	}
}

func TestHTTPJevClientBodyReadDeadline(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       errReader{err: context.DeadlineExceeded},
		}, nil
	})}
	c := &HTTPJevClient{URL: "http://example.invalid", APIKey: "k", HTTP: client, MaxRetries: 0, Sleep: func(time.Duration) {}}

	_, jerr := c.Decide(context.Background(), JevRequest{})
	if jerr == nil || jerr.Message != "timeout" || !jerr.Retryable {
		t.Fatalf("err %+v want retryable timeout", jerr)
	}
}

func TestHTTPJevClientNewRequestFailureNotRetried(t *testing.T) {
	var slept bool
	c := &HTTPJevClient{URL: "http://exa mple.com", APIKey: "k", MaxRetries: 3, Sleep: func(time.Duration) { slept = true }}

	_, jerr := c.Decide(context.Background(), JevRequest{})
	if jerr == nil {
		t.Fatal("expected error")
	}
	if jerr.Retryable {
		t.Fatal("new request failure must not be retryable")
	}
	if slept {
		t.Fatal("must not retry a request-construction failure")
	}
}

func TestHTTPJevClientRedactsKeyInErrorMessage(t *testing.T) {
	const key = "sk-echo-me-12345"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"detail":"error with sk-echo-me-12345 in body"}`))
	}))
	defer srv.Close()

	c := &HTTPJevClient{URL: srv.URL, APIKey: key, MaxRetries: 0, Sleep: func(time.Duration) {}}
	_, jerr := c.Decide(context.Background(), JevRequest{})
	if jerr == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(jerr.Message, key) {
		t.Fatalf("key leaked into message %q", jerr.Message)
	}
	if !strings.Contains(jerr.Message, "[redacted]") {
		t.Fatalf("message %q missing [redacted]", jerr.Message)
	}
}

func TestHTTPJevClientEmptyErrorBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	_, jerr := newTestJevClient(srv.URL, 0).Decide(context.Background(), JevRequest{})
	if jerr == nil {
		t.Fatal("expected error")
	}
	if jerr.Message != "HTTP 502" {
		t.Fatalf("message %q want 'HTTP 502'", jerr.Message)
	}
}
