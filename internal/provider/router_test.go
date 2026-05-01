package provider

import (
	"testing"
)

// helperHealth builds a Health snapshot like the one OpenAICompatible.Check
// produces from a copilot-api /v1/models response.
func helperHealth(modelToEndpoints map[string][]string) *Health {
	infos := make([]ModelInfo, 0, len(modelToEndpoints))
	models := make([]string, 0, len(modelToEndpoints))
	for id, eps := range modelToEndpoints {
		infos = append(infos, ModelInfo{ID: id, SupportedEndpoints: eps})
		models = append(models, id)
	}
	return &Health{Endpoint: "http://localhost:4141", Models: models, ModelInfo: infos}
}

func TestPickProviderClaudeOnCopilotProxy(t *testing.T) {
	cfg := Config{Type: "openai-compatible", BaseURL: "http://localhost:4141", Model: "claude-opus-4.6"}
	health := helperHealth(map[string][]string{
		"claude-opus-4.6": {"/v1/messages", "/chat/completions"},
		"gpt-4o":          {"/chat/completions"},
	})
	got := PickProvider(cfg, health)
	if _, ok := got.(*AnthropicProvider); !ok {
		t.Fatalf("PickProvider for Claude on copilot proxy = %T, want *AnthropicProvider", got)
	}
}

func TestPickProviderClaudeSonnetOnCopilotProxy(t *testing.T) {
	cfg := Config{Type: "openai-compatible", BaseURL: "http://localhost:4141", Model: "claude-sonnet-4"}
	health := helperHealth(map[string][]string{
		"claude-sonnet-4": {"/v1/messages", "/chat/completions"},
	})
	got := PickProvider(cfg, health)
	if _, ok := got.(*AnthropicProvider); !ok {
		t.Fatalf("PickProvider for claude-sonnet-4 on copilot proxy = %T, want *AnthropicProvider", got)
	}
}

func TestPickProviderResponsesOnlyFallsThrough(t *testing.T) {
	// /responses-only models stay on OpenAICompatible until a real
	// ResponsesProvider scaffold is wired in. The proxy aborts upstream
	// either way today; the typed error in Generate gives the hint.
	cfg := Config{Type: "openai-compatible", BaseURL: "http://localhost:4141", Model: "gpt-5.5"}
	health := helperHealth(map[string][]string{
		"gpt-5.5": {"/responses", "ws:/responses"},
	})
	got := PickProvider(cfg, health)
	if _, ok := got.(*OpenAICompatible); !ok {
		t.Fatalf("PickProvider for /responses-only model = %T, want *OpenAICompatible", got)
	}
}

func TestPickProviderChatCompletionsOnly(t *testing.T) {
	cfg := Config{Type: "openai-compatible", BaseURL: "http://localhost:4141", Model: "gpt-4o"}
	health := helperHealth(map[string][]string{
		"gpt-4o": {"/chat/completions"},
	})
	got := PickProvider(cfg, health)
	if _, ok := got.(*OpenAICompatible); !ok {
		t.Fatalf("PickProvider for /chat/completions-only model = %T, want *OpenAICompatible", got)
	}
}

func TestPickProviderClaudeOffCopilotProxy(t *testing.T) {
	// Real OpenAI URL — even if Health says /v1/messages is supported
	// (it doesn't, but for safety), smart routing must NOT fire
	// outside the copilot-api proxy scope.
	cfg := Config{Type: "openai-compatible", BaseURL: "https://api.openai.com", Model: "claude-opus-4.6"}
	health := helperHealth(map[string][]string{
		"claude-opus-4.6": {"/v1/messages"},
	})
	got := PickProvider(cfg, health)
	if _, ok := got.(*OpenAICompatible); !ok {
		t.Fatalf("PickProvider off-proxy = %T, want *OpenAICompatible (NewFromConfig)", got)
	}
}

func TestPickProviderNilHealth(t *testing.T) {
	cfg := Config{Type: "openai-compatible", BaseURL: "http://localhost:4141", Model: "claude-opus-4.6"}
	got := PickProvider(cfg, nil)
	// With no Health metadata we have no endpoint info, so fall back to
	// the wire type the user picked. This preserves behaviour when the
	// probe was skipped.
	if _, ok := got.(*OpenAICompatible); !ok {
		t.Fatalf("PickProvider with nil health = %T, want *OpenAICompatible (NewFromConfig)", got)
	}
}

func TestPickProviderModelNotInHealth(t *testing.T) {
	// Probe succeeded but the user's model wasn't enumerated (rare but
	// possible if the proxy paginates or hides certain models). Fall
	// back to the default wire type rather than guessing.
	cfg := Config{Type: "openai-compatible", BaseURL: "http://localhost:4141", Model: "claude-opus-4.6"}
	health := helperHealth(map[string][]string{
		"gpt-4o": {"/chat/completions"},
	})
	got := PickProvider(cfg, health)
	if _, ok := got.(*OpenAICompatible); !ok {
		t.Fatalf("PickProvider with unknown model = %T, want *OpenAICompatible", got)
	}
}

func TestPickProviderRespectsAnthropicTypeOnProxy(t *testing.T) {
	// User explicitly picked --type anthropic against the proxy. We
	// should still return AnthropicProvider — the routing decision and
	// the user's wire-type preference agree, no surprise.
	cfg := Config{Type: "anthropic", BaseURL: "http://localhost:4141", Model: "claude-opus-4.6"}
	health := helperHealth(map[string][]string{
		"claude-opus-4.6": {"/v1/messages", "/chat/completions"},
	})
	got := PickProvider(cfg, health)
	if _, ok := got.(*AnthropicProvider); !ok {
		t.Fatalf("PickProvider with --type anthropic on proxy = %T, want *AnthropicProvider", got)
	}
}

func TestHasEndpoint(t *testing.T) {
	if !hasEndpoint([]string{"/v1/messages", "/chat/completions"}, "/v1/messages") {
		t.Error("hasEndpoint should find /v1/messages")
	}
	if hasEndpoint([]string{"/chat/completions"}, "/v1/messages") {
		t.Error("hasEndpoint should not find /v1/messages in chat-only list")
	}
	if hasEndpoint(nil, "/v1/messages") {
		t.Error("hasEndpoint on nil should be false")
	}
}

func TestLookupSupportedEndpoints(t *testing.T) {
	infos := []ModelInfo{
		{ID: "a", SupportedEndpoints: []string{"/x"}},
		{ID: "b", SupportedEndpoints: []string{"/y", "/z"}},
	}
	if got := lookupSupportedEndpoints(infos, "b"); len(got) != 2 || got[0] != "/y" {
		t.Errorf("lookup b = %v, want [/y /z]", got)
	}
	if got := lookupSupportedEndpoints(infos, "missing"); got != nil {
		t.Errorf("lookup missing = %v, want nil", got)
	}
}
