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

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"
)

type AzureOpenAI struct {
	Endpoint     string
	Deployment   string
	APIVersion   string
	APIKey       string
	HTTPClient   *http.Client
	SystemPrompt string
	UserPrompt   string
}

type chatRequest struct {
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
	MaxTokens   int           `json:"max_tokens"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

type responsesRequest struct {
	Model           string            `json:"model"`
	Instructions    string            `json:"instructions"`
	Input           string            `json:"input"`
	MaxOutputTokens int               `json:"max_output_tokens"`
	Reasoning       map[string]string `json:"reasoning,omitempty"`
	Store           bool              `json:"store"`
}

type responsesResponse struct {
	OutputText string `json:"output_text,omitempty"`
	Output     []struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text,omitempty"`
		} `json:"content,omitempty"`
	} `json:"output,omitempty"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
	IncompleteDetails *struct {
		Reason string `json:"reason"`
	} `json:"incomplete_details,omitempty"`
}

func (c AzureOpenAI) Analyze(ctx context.Context, snapshotJSON string) (string, error) {
	if c.Endpoint == "" || c.Deployment == "" || c.APIKey == "" {
		return "", fmt.Errorf("azure endpoint, deployment, and api key are required")
	}
	apiVersion := c.APIVersion
	if apiVersion == "" {
		apiVersion = "2024-10-21"
	}

	systemPrompt, userPrompt := c.resolvePrompts(snapshotJSON)

	result, err := c.analyzeWithChatCompletions(ctx, apiVersion, systemPrompt, userPrompt)
	if err == nil {
		return result, nil
	}
	if !isUnsupportedOperation(err) {
		return "", err
	}

	responsesResult, responsesErr := c.analyzeWithOpenAISDK(ctx, systemPrompt, userPrompt)
	if responsesErr != nil {
		restResult, restErr := c.analyzeWithResponses(ctx, systemPrompt, userPrompt)
		if restErr != nil {
			return "", fmt.Errorf("chat completions failed: %w; openai sdk responses failed: %w; rest responses failed: %w", err, responsesErr, restErr)
		}
		return restResult, nil
	}
	return responsesResult, nil
}

func (c AzureOpenAI) resolvePrompts(snapshotJSON string) (string, string) {
	return ResolvePrompts(c.SystemPrompt, c.UserPrompt, snapshotJSON)
}

func (c AzureOpenAI) analyzeWithChatCompletions(ctx context.Context, apiVersion, systemPrompt, userPrompt string) (string, error) {
	body := chatRequest{
		Temperature: 0.2,
		MaxTokens:   1800,
		Messages: []chatMessage{
			{
				Role:    "system",
				Content: systemPrompt,
			},
			{
				Role:    "user",
				Content: userPrompt,
			},
		},
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return "", err
	}

	reqURL, err := c.chatCompletionsURL(apiVersion)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(encoded))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("api-key", c.APIKey)

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var decoded chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if decoded.Error != nil {
			return "", fmt.Errorf("azure openai returned %s: %s", resp.Status, decoded.Error.Message)
		}
		return "", fmt.Errorf("azure openai returned %s", resp.Status)
	}
	if len(decoded.Choices) == 0 {
		return "", fmt.Errorf("azure openai returned no choices")
	}
	return strings.TrimSpace(decoded.Choices[0].Message.Content), nil
}

func (c AzureOpenAI) analyzeWithOpenAISDK(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	client := openai.NewClient(
		option.WithBaseURL(strings.TrimRight(c.Endpoint, "/")+"/openai/v1/"),
		option.WithAPIKey(c.APIKey),
		option.WithHTTPClient(c.httpClient()),
	)
	resp, err := client.Responses.New(ctx, responses.ResponseNewParams{
		Model: shared.ResponsesModel(c.Deployment),
		Input: responses.ResponseNewParamsInputUnion{
			OfString: openai.String(userPrompt),
		},
		Instructions:    openai.String(systemPrompt),
		MaxOutputTokens: openai.Int(4000),
		Reasoning: shared.ReasoningParam{
			Effort: shared.ReasoningEffortMedium,
		},
		Store: openai.Bool(false),
	})
	if err != nil {
		return "", err
	}
	text := strings.TrimSpace(resp.OutputText())
	if text == "" {
		return "", fmt.Errorf("openai sdk responses returned no text")
	}
	return text, nil
}

func (c AzureOpenAI) analyzeWithResponses(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	body := responsesRequest{
		Model:           c.Deployment,
		Instructions:    systemPrompt,
		Input:           userPrompt,
		MaxOutputTokens: 4000,
		Reasoning:       map[string]string{"effort": "medium"},
		Store:           false,
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return "", err
	}

	reqURL, err := c.responsesV1URL()
	if err != nil {
		return "", err
	}
	result, err := c.postResponses(ctx, reqURL, encoded)
	if err == nil {
		return result, nil
	}

	previewURL, previewURLErr := c.responsesPreviewURL()
	if previewURLErr != nil {
		return "", fmt.Errorf("responses v1 failed: %w; create preview url: %w", err, previewURLErr)
	}
	previewResult, previewErr := c.postResponses(ctx, previewURL, encoded)
	if previewErr != nil {
		return "", fmt.Errorf("responses v1 failed: %w; responses preview failed: %w", err, previewErr)
	}
	return previewResult, nil
}

func (c AzureOpenAI) postResponses(ctx context.Context, reqURL string, encoded []byte) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(encoded))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("api-key", c.APIKey)

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var raw map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return "", err
	}
	decodedBytes, err := json.Marshal(raw)
	if err != nil {
		return "", err
	}

	var decoded responsesResponse
	if err := json.Unmarshal(decodedBytes, &decoded); err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if decoded.Error != nil {
			return "", fmt.Errorf("azure openai responses returned %s: %s", resp.Status, decoded.Error.Message)
		}
		return "", fmt.Errorf("azure openai responses returned %s", resp.Status)
	}

	if text := strings.TrimSpace(decoded.OutputText); text != "" {
		return text, nil
	}
	for _, item := range decoded.Output {
		for _, content := range item.Content {
			if text := strings.TrimSpace(content.Text); text != "" {
				return text, nil
			}
		}
	}
	if text := firstTextValue(raw); text != "" {
		return text, nil
	}
	if decoded.IncompleteDetails != nil && decoded.IncompleteDetails.Reason != "" {
		return "", fmt.Errorf("azure openai responses incomplete: %s", decoded.IncompleteDetails.Reason)
	}
	return "", fmt.Errorf("azure openai responses returned no text")
}

func (c AzureOpenAI) chatCompletionsURL(apiVersion string) (string, error) {
	endpoint := strings.TrimRight(c.Endpoint, "/")
	escapedDeployment := url.PathEscape(c.Deployment)
	parsed, err := url.Parse(endpoint + "/openai/deployments/" + escapedDeployment + "/chat/completions")
	if err != nil {
		return "", err
	}
	params := parsed.Query()
	params.Set("api-version", apiVersion)
	parsed.RawQuery = params.Encode()
	return parsed.String(), nil
}

func (c AzureOpenAI) responsesV1URL() (string, error) {
	endpoint := strings.TrimRight(c.Endpoint, "/")
	return url.JoinPath(endpoint, "/openai/v1/responses")
}

func (c AzureOpenAI) responsesPreviewURL() (string, error) {
	endpoint := strings.TrimRight(c.Endpoint, "/")
	parsed, err := url.Parse(endpoint + "/openai/responses")
	if err != nil {
		return "", err
	}
	params := parsed.Query()
	params.Set("api-version", "2025-04-01-preview")
	parsed.RawQuery = params.Encode()
	return parsed.String(), nil
}

func (c AzureOpenAI) httpClient() *http.Client {
	if c.HTTPClient == nil || c.HTTPClient.Timeout == 0 {
		return &http.Client{Timeout: 120 * time.Second}
	}
	return c.HTTPClient
}

func isUnsupportedOperation(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "unsupported")
}

func firstTextValue(value any) string {
	switch typed := value.(type) {
	case map[string]any:
		for _, key := range []string{"output_text", "text"} {
			if value, ok := typed[key].(string); ok && strings.TrimSpace(value) != "" {
				return strings.TrimSpace(value)
			}
		}
		for _, value := range typed {
			if text := firstTextValue(value); text != "" {
				return text
			}
		}
	case []any:
		for _, value := range typed {
			if text := firstTextValue(value); text != "" {
				return text
			}
		}
	}
	return ""
}
