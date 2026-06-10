//go:build sqlite_fts5 && sqlite_vec

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestFormatJSON(t *testing.T) {
	data := map[string]string{"foo": "bar"}
	got, err := FormatJSON(data)
	if err != nil {
		t.Fatalf("FormatJSON failed: %v", err)
	}

	expected := `{
  "foo": "bar"
}`
	if strings.TrimSpace(got) != expected {
		t.Errorf("got %q, want %q", got, expected)
	}
}

func TestWriteResult(t *testing.T) {
	tests := []struct {
		name     string
		v        interface{}
		format   string
		contains string
	}{
		{
			name:     "JSON format",
			v:        map[string]string{"foo": "bar"},
			format:   "json",
			contains: `"foo": "bar"`,
		},
		{
			name:     "Text format",
			v:        "hello world",
			format:   "text",
			contains: "hello world",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			err := WriteResult(&buf, tt.v, tt.format)
			if err != nil {
				t.Fatalf("WriteResult failed: %v", err)
			}
			if !strings.Contains(buf.String(), tt.contains) {
				t.Errorf("output %q does not contain %q", buf.String(), tt.contains)
			}
		})
	}
}

func TestWriteError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		code     int
		format   string
		contains string
	}{
		{
			name:     "JSON error",
			err:      errors.New("something went wrong"),
			code:     2,
			format:   "json",
			contains: `"error": "something went wrong"`,
		},
		{
			name:     "Text error",
			err:      errors.New("something went wrong"),
			code:     2,
			format:   "text",
			contains: "error: something went wrong",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			WriteError(&buf, tt.err, tt.code, tt.format)
			if !strings.Contains(buf.String(), tt.contains) {
				t.Errorf("output %q does not contain %q", buf.String(), tt.contains)
			}

			if tt.format == "json" {
				var res map[string]interface{}
				if err := json.Unmarshal(buf.Bytes(), &res); err != nil {
					t.Fatalf("failed to unmarshal JSON error: %v", err)
				}
				if res["code"].(float64) != float64(tt.code) {
					t.Errorf("got code %v, want %v", res["code"], tt.code)
				}
			}
		})
	}
}

func TestColorFunctions(t *testing.T) {
	// Mock ShouldColor to return false
	noColorFlag = true
	defer func() { noColorFlag = false }()

	if Green("test") != "test" {
		t.Errorf("Green() with no color want %q, got %q", "test", Green("test"))
	}

	// We can't easily test ShouldColor true without mocking IsTTY or Env vars,
	// but we can verify it respects noColorFlag.
}
