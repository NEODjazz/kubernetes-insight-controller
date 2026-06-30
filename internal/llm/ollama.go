package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Ollama struct {
	Endpoint     string
	Model        string
	APIKey       string
	HTTPClient   *http.Client
	SystemPrompt string
	UserPrompt   string
}

const ollamaRequestTimeout = 10 * time.Minute

type ollamaChatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
	Stream   bool          `json:"stream"`
	Options  struct {
		Temperature float64 `json:"temperature"`
		NumCtx      int     `json:"num_ctx"`
	} `json:"options"`
}

type ollamaChatResponse struct {
	Message chatMessage `json:"message"`
	Error   string      `json:"error,omitempty"`
}

func (c Ollama) Analyze(ctx context.Context, snapshotJSON string) (string, error) {
	if strings.TrimSpace(c.Endpoint) == "" || strings.TrimSpace(c.Model) == "" {
		return "", fmt.Errorf("ollama endpoint and model are required")
	}

	systemPrompt, userPrompt := ResolvePrompts(c.SystemPrompt, c.UserPrompt, snapshotJSON)
	body := ollamaChatRequest{
		Model:  c.Model,
		Stream: false,
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
	}
	body.Options.Temperature = 0.2
	body.Options.NumCtx = 16384
	encoded, err := json.Marshal(body)
	if err != nil {
		return "", err
	}

	reqURL, err := url.JoinPath(strings.TrimRight(c.Endpoint, "/"), "/api/chat")
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(encoded))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
	}

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var decoded ollamaChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if decoded.Error != "" {
			return "", fmt.Errorf("ollama returned %s: %s", resp.Status, decoded.Error)
		}
		return "", fmt.Errorf("ollama returned %s", resp.Status)
	}
	if decoded.Error != "" {
		return "", fmt.Errorf("ollama returned an error: %s", decoded.Error)
	}
	if result := strings.TrimSpace(decoded.Message.Content); result != "" {
		return result, nil
	}
	return "", fmt.Errorf("ollama returned no text")
}

func (c Ollama) httpClient() *http.Client {
	if c.HTTPClient == nil {
		return &http.Client{Timeout: ollamaRequestTimeout}
	}
	if c.HTTPClient.Timeout == 0 {
		client := *c.HTTPClient
		client.Timeout = ollamaRequestTimeout
		return &client
	}
	return c.HTTPClient
}
