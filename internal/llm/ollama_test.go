package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestOllamaAnalyze(t *testing.T) {
	var received ollamaChatRequest
	httpClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/api/chat" {
			t.Fatalf("expected /api/chat, got %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer secret" {
			t.Fatalf("expected bearer authorization, got %q", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"message":{"role":"assistant","content":"  recommendation  "}}`)),
			Request:    r,
		}, nil
	})}

	result, err := (Ollama{
		Endpoint:   "http://ollama.example",
		Model:      "qwen3:8b",
		APIKey:     "secret",
		HTTPClient: httpClient,
	}).Analyze(context.Background(), `{"nodes":1}`)
	if err != nil {
		t.Fatalf("analyze: %v", err)
	}
	if result != "recommendation" {
		t.Fatalf("expected trimmed recommendation, got %q", result)
	}
	if received.Model != "qwen3:8b" || received.Stream {
		t.Fatalf("unexpected request settings: %+v", received)
	}
	if received.Options.NumCtx != 16384 {
		t.Fatalf("expected 16384 context window, got %d", received.Options.NumCtx)
	}
	if len(received.Messages) != 2 || received.Messages[0].Role != "system" || received.Messages[1].Role != "user" {
		t.Fatalf("unexpected messages: %+v", received.Messages)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestOllamaRequiresEndpointAndModel(t *testing.T) {
	if _, err := (Ollama{}).Analyze(context.Background(), "{}"); err == nil {
		t.Fatal("expected configuration error")
	}
}
