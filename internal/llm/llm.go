package llm

import (
	"context"
	"strings"
)

const (
	ProviderAzureOpenAI = "azureOpenAI"
	ProviderOllama      = "ollama"

	DefaultSystemPrompt = "You are a senior Kubernetes SRE. Analyze cluster state and Prometheus metrics. Return concise, actionable recommendations in Russian. Prioritize reliability, cost, performance, security, and observability. Do not invent facts that are absent from the snapshot."
	DefaultUserPrompt   = "Analyze this Kubernetes cluster snapshot and produce prioritized improvements with evidence and suggested next actions:\n\n{{snapshot}}"
	SnapshotPlaceholder = "{{snapshot}}"
)

type Analyzer interface {
	Analyze(ctx context.Context, snapshotJSON string) (string, error)
}

func ResolvePrompts(systemPrompt, userPrompt, snapshotJSON string) (string, string) {
	systemPrompt = strings.TrimSpace(systemPrompt)
	if systemPrompt == "" {
		systemPrompt = DefaultSystemPrompt
	}

	userPrompt = strings.TrimSpace(userPrompt)
	if userPrompt == "" {
		userPrompt = DefaultUserPrompt
	}
	if strings.Contains(userPrompt, SnapshotPlaceholder) {
		userPrompt = strings.ReplaceAll(userPrompt, SnapshotPlaceholder, snapshotJSON)
	} else {
		userPrompt += "\n\n" + snapshotJSON
	}
	return systemPrompt, userPrompt
}
