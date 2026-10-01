package core

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// JevClient is the minimal classifier transport contract: one request in, a
// response or a normalized error out. It never returns both non-nil.
type JevClient interface {
	Decide(ctx context.Context, req JevRequest) (*JevResponse, *JevError)
}

// HTTPJevClient is the HTTP implementation of JevClient with a bounded retry
// policy. Timeouts come from the caller's context, not a fixed deadline.
type HTTPJevClient struct {
	URL        string
	APIKey     string
	HTTP       *http.Client
	MaxRetries int
	Sleep      func(time.Duration)
}

// NewJevClient builds an HTTPJevClient from policy for the named provider. It
// resolves the API key from the environment variable named by the provider's
// api_key_env (never embedding the value) and honors an optional URL override.
func NewJevClient(p JevRouterPolicy, provider, urlOverride string, getenv func(string) string) (*HTTPJevClient, error) {
	cfg, ok := p.Providers[provider]
	if !ok {
		return nil, fmt.Errorf("unknown Jev provider %q", provider)
	}
	key := getenv(cfg.APIKeyEnv)
	if key == "" {
		return nil, fmt.Errorf("environment variable %s must be set", cfg.APIKeyEnv)
	}
	url := cfg.URL
	if urlOverride != "" {
		url = urlOverride
	}
	return &HTTPJevClient{
		URL:        url,
		APIKey:     key,
		HTTP:       &http.Client{},
		MaxRetries: 2,
		Sleep:      time.Sleep,
	}, nil
}

// Decide posts the request and returns the parsed response, retrying retryable
// failures (429/5xx and transport errors) up to MaxRetries with exponential
// backoff, honoring an integer Retry-After header when present and never
// sleeping past the context deadline.
func (c *HTTPJevClient) Decide(ctx context.Context, req JevRequest) (resp *JevResponse, jerr *JevError) {
	defer func() {
		if jerr != nil && c.APIKey != "" {
			jerr.Message = strings.ReplaceAll(jerr.Message, c.APIKey, "[redacted]")
		}
	}()

	body, err := json.Marshal(req)
	if err != nil {
		return nil, &JevError{Message: err.Error()}
	}

	maxRetries := c.MaxRetries
	if maxRetries < 0 {
		maxRetries = 0
	}

	var lastErr *JevError
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if cerr := ctxError(ctx); cerr != nil {
			return nil, cerr
		}
		gotResp, gotErr, retryAfter := c.doOnce(ctx, body)
		if gotErr == nil {
			return gotResp, nil
		}
		lastErr = gotErr
		if cerr := ctxError(ctx); cerr != nil {
			return nil, cerr
		}
		if !gotErr.Retryable {
			return nil, gotErr
		}
		if attempt == maxRetries {
			break
		}
		backoff := time.Duration(500*(1<<attempt)) * time.Millisecond
		if retryAfter > 0 {
			backoff = retryAfter
		}
		if !sleepWithinDeadline(ctx, backoff, c.Sleep) {
			break
		}
	}
	if lastErr == nil {
		lastErr = &JevError{Message: "no response"}
	}
	return nil, lastErr
}

// doOnce performs a single POST and returns the parsed response or a normalized
// error plus any Retry-After backoff the server requested.
func (c *HTTPJevClient) doOnce(ctx context.Context, body []byte) (*JevResponse, *JevError, time.Duration) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.URL, bytes.NewReader(body))
	if err != nil {
		return nil, &JevError{Message: err.Error(), Retryable: false}, 0
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")

	httpClient := c.HTTP
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	resp, err := httpClient.Do(httpReq)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, &JevError{Message: "timeout", Retryable: true}, 0
		}
		return nil, &JevError{Message: err.Error(), Retryable: true}, 0
	}
	defer resp.Body.Close()

	data, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		if errors.Is(readErr, context.DeadlineExceeded) {
			return nil, &JevError{Message: "timeout", Retryable: true}, 0
		}
		return nil, &JevError{Message: readErr.Error(), Retryable: true}, 0
	}

	retryAfter := retryAfterDuration(resp.Header.Get("Retry-After"))
	if resp.StatusCode == http.StatusOK {
		parsed, perr := ParseJevResponse(data)
		if perr != nil {
			return nil, &JevError{Status: resp.StatusCode, Message: perr.Error(), Retryable: false}, retryAfter
		}
		return parsed, nil, 0
	}

	msg := extractErrorMessage(data)
	if strings.TrimSpace(msg) == "" {
		msg = fmt.Sprintf("HTTP %d", resp.StatusCode)
	}
	return nil, &JevError{
		Status:    resp.StatusCode,
		Message:   msg,
		Retryable: isRetryableStatus(resp.StatusCode),
	}, retryAfter
}

// ctxError maps a canceled or expired context to a normalized JevError, or nil
// when the context is still live.
func ctxError(ctx context.Context) *JevError {
	switch {
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		return &JevError{Message: "timeout", Retryable: true}
	case errors.Is(ctx.Err(), context.Canceled):
		return &JevError{Message: "canceled", Retryable: false}
	default:
		return nil
	}
}

// sleepWithinDeadline sleeps d using the provided sleeper unless doing so would
// run past the context deadline, in which case it returns false without
// sleeping.
func sleepWithinDeadline(ctx context.Context, d time.Duration, sleep func(time.Duration)) bool {
	if d <= 0 {
		return true
	}
	if deadline, ok := ctx.Deadline(); ok {
		remaining := time.Until(deadline)
		if remaining <= 0 || d > remaining {
			return false
		}
	}
	if sleep == nil {
		sleep = time.Sleep
	}
	sleep(d)
	return true
}

// retryAfterDuration parses an integer-seconds Retry-After header; a missing or
// non-integer value yields zero, which falls back to the default backoff.
func retryAfterDuration(v string) time.Duration {
	if v == "" {
		return 0
	}
	secs, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil || secs < 0 {
		return 0
	}
	return time.Duration(secs) * time.Second
}

// isRetryableStatus reports whether an HTTP status is retryable under the
// classifier contract.
func isRetryableStatus(code int) bool {
	switch code {
	case 429, 500, 502, 503, 524, 529:
		return true
	default:
		return false
	}
}

// extractErrorMessage pulls a human-readable message from a non-200 body:
// {"error":{"message":...}}, else {"detail":...}, else the first 200 bytes of the
// raw body. It never echoes the request, so a secret can never leak here.
func extractErrorMessage(data []byte) string {
	var envelope struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
		Detail string `json:"detail"`
	}
	if err := json.Unmarshal(data, &envelope); err == nil {
		if envelope.Error.Message != "" {
			return envelope.Error.Message
		}
		if envelope.Detail != "" {
			return envelope.Detail
		}
	}
	msg := string(data)
	if len(msg) > 200 {
		msg = msg[:200]
	}
	return msg
}
