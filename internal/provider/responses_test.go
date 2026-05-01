package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestResponsesProviderEnabledGate(t *testing.T) {
	t.Setenv("TPATCH_ENABLE_RESPONSES_PROVIDER", "")
	if responsesProviderEnabled() {
		t.Error("gate should be off when env var is empty")
	}
	t.Setenv("TPATCH_ENABLE_RESPONSES_PROVIDER", "1")
	if !responsesProviderEnabled() {
		t.Error("gate should be on for '1'")
	}
	t.Setenv("TPATCH_ENABLE_RESPONSES_PROVIDER", "true")
	if !responsesProviderEnabled() {
		t.Error("gate should be on for 'true' (case-insensitive)")
	}
	t.Setenv("TPATCH_ENABLE_RESPONSES_PROVIDER", "0")
	if responsesProviderEnabled() {
		t.Error("gate should be off for '0'")
	}
}

func TestPickProviderResponsesGated(t *testing.T) {
	cfg := Config{Type: "openai-compatible", BaseURL: "http://localhost:4141", Model: "gpt-5.5"}
	health := helperHealth(map[string][]string{
		"gpt-5.5": {"/responses", "ws:/responses"},
	})

	// Gate off — must NOT route to ResponsesProvider yet.
	t.Setenv("TPATCH_ENABLE_RESPONSES_PROVIDER", "")
	if got := PickProvider(cfg, health); fmtTypeName(got) == "*provider.ResponsesProvider" {
		t.Errorf("PickProvider with gate off = %T, want OpenAICompatible fallthrough", got)
	}

	// Gate on — must route to ResponsesProvider.
	t.Setenv("TPATCH_ENABLE_RESPONSES_PROVIDER", "1")
	if _, ok := PickProvider(cfg, health).(*ResponsesProvider); !ok {
		t.Errorf("PickProvider with gate on = unexpected type")
	}
}

// fmtTypeName is a small helper that doesn't depend on `reflect` for
// equality readability in failure messages.
func fmtTypeName(v any) string {
	switch v.(type) {
	case *ResponsesProvider:
		return "*provider.ResponsesProvider"
	case *AnthropicProvider:
		return "*provider.AnthropicProvider"
	case *OpenAICompatible:
		return "*provider.OpenAICompatible"
	default:
		return "unknown"
	}
}

func TestResponsesGenerateSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/responses" {
			http.NotFound(w, r)
			return
		}
		body, _ := io.ReadAll(r.Body)
		// System prompt should be sent as a "developer" role message
		// per the Responses API ResponsesInput type union.
		if !strings.Contains(string(body), `"role":"developer"`) {
			t.Errorf("expected developer role in body, got %s", string(body))
		}
		if !strings.Contains(string(body), `"max_output_tokens":4096`) {
			t.Errorf("expected max_output_tokens, got %s", string(body))
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"output": []map[string]any{
				{
					"type": "message",
					"role": "assistant",
					"content": []map[string]string{
						{"type": "output_text", "text": "hello from gpt-5"},
					},
				},
			},
		})
	}))
	defer srv.Close()

	cfg := Config{Type: "openai-compatible", BaseURL: srv.URL, Model: "gpt-5.5"}
	p := NewResponses()
	out, err := p.Generate(context.Background(), cfg, GenerateRequest{
		SystemPrompt: "be brief",
		UserPrompt:   "hi",
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if out != "hello from gpt-5" {
		t.Errorf("output = %q, want 'hello from gpt-5'", out)
	}
}

func TestResponsesGenerateProxyAbortDetected(t *testing.T) {
	defer setForceCopilotProxy(true)()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"This operation was aborted"}`))
	}))
	defer srv.Close()

	cfg := Config{Type: "openai-compatible", BaseURL: srv.URL, Model: "gpt-5.5"}
	p := NewResponses()
	_, err := p.Generate(context.Background(), cfg, GenerateRequest{UserPrompt: "hi"})
	if !IsProxyUpstreamAborted(err) {
		t.Fatalf("expected ProxyUpstreamAbortedError, got %v", err)
	}
}

func TestResponsesCheckUsesModelsEndpoint(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"gpt-5.5","supported_endpoints":["/responses"]}]}`))
	}))
	defer srv.Close()

	cfg := Config{Type: "openai-compatible", BaseURL: srv.URL, Model: "gpt-5.5"}
	p := NewResponses()
	h, err := p.Check(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(h.ModelInfo) != 1 || h.ModelInfo[0].ID != "gpt-5.5" {
		t.Errorf("ModelInfo = %v, want one entry for gpt-5.5", h.ModelInfo)
	}
	if len(h.ModelInfo[0].SupportedEndpoints) != 1 || h.ModelInfo[0].SupportedEndpoints[0] != "/responses" {
		t.Errorf("supported endpoints = %v, want [/responses]", h.ModelInfo[0].SupportedEndpoints)
	}
}
