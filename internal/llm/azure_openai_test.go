package llm

import (
	"strings"
	"testing"
)

func TestResolvePromptsUsesDefaults(t *testing.T) {
	systemPrompt, userPrompt := (AzureOpenAI{}).resolvePrompts(`{"nodes":1}`)

	if systemPrompt != DefaultSystemPrompt {
		t.Fatalf("expected default system prompt, got %q", systemPrompt)
	}
	if strings.Contains(userPrompt, SnapshotPlaceholder) {
		t.Fatal("snapshot placeholder was not replaced")
	}
	if !strings.Contains(userPrompt, `{"nodes":1}`) {
		t.Fatal("snapshot was not included in default user prompt")
	}
}

func TestResolvePromptsUsesConfiguredPrompts(t *testing.T) {
	client := AzureOpenAI{
		SystemPrompt: "Custom system",
		UserPrompt:   "Before " + SnapshotPlaceholder + " after",
	}
	systemPrompt, userPrompt := client.resolvePrompts(`{"pods":2}`)

	if systemPrompt != "Custom system" {
		t.Fatalf("expected configured system prompt, got %q", systemPrompt)
	}
	if userPrompt != `Before {"pods":2} after` {
		t.Fatalf("expected placeholder replacement, got %q", userPrompt)
	}
}

func TestResolvePromptsAppendsSnapshotWithoutPlaceholder(t *testing.T) {
	_, userPrompt := (AzureOpenAI{UserPrompt: "Custom request"}).resolvePrompts(`{"services":3}`)

	if userPrompt != "Custom request\n\n"+`{"services":3}` {
		t.Fatalf("expected snapshot appended to user prompt, got %q", userPrompt)
	}
}
