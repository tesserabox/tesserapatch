package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCheckSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" {
			json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]string{
					{"id": "gpt-4o"},
					{"id": "claude-opus-4.6"},
				},
			})
			return
		}
		w.WriteHeader(404)
	}))
	defer srv.Close()

	p := New()
	cfg := Config{BaseURL: srv.URL, Model: "gpt-4o"}
	health, err := p.Check(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if health.Endpoint != srv.URL {
		t.Errorf("endpoint = %q", health.Endpoint)
	}
	if len(health.Models) != 2 {
		t.Errorf("models count = %d, want 2", len(health.Models))
	}
}

// TestCheckParsesSupportedEndpoints locks in the wire-level contract
// that PickProvider relies on: when the upstream /v1/models response
// includes per-model `supported_endpoints`, OpenAICompatible.Check must
// surface them on Health.ModelInfo with matching IDs and order.
func TestCheckParsesSupportedEndpoints(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			w.WriteHeader(404)
			return
		}
		_, _ = w.Write([]byte(`{
			"data": [
				{"id": "claude-opus-4.6", "supported_endpoints": ["/v1/messages", "/chat/completions"]},
				{"id": "gpt-5.5", "supported_endpoints": ["/responses", "ws:/responses"]},
				{"id": "gpt-4o", "supported_endpoints": ["/chat/completions"]}
			]
		}`))
	}))
	defer srv.Close()

	p := New()
	cfg := Config{BaseURL: srv.URL, Model: "claude-opus-4.6"}
	health, err := p.Check(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(health.ModelInfo) != 3 {
		t.Fatalf("ModelInfo count = %d, want 3", len(health.ModelInfo))
	}
	want := map[string][]string{
		"claude-opus-4.6": {"/v1/messages", "/chat/completions"},
		"gpt-5.5":         {"/responses", "ws:/responses"},
		"gpt-4o":          {"/chat/completions"},
	}
	for _, mi := range health.ModelInfo {
		got := mi.SupportedEndpoints
		exp, ok := want[mi.ID]
		if !ok {
			t.Errorf("unexpected model %q in ModelInfo", mi.ID)
			continue
		}
		if len(got) != len(exp) {
			t.Errorf("%s: got %v, want %v", mi.ID, got, exp)
			continue
		}
		for i := range got {
			if got[i] != exp[i] {
				t.Errorf("%s[%d]: got %q, want %q", mi.ID, i, got[i], exp[i])
			}
		}
	}
}

// TestCheckMissingSupportedEndpoints exercises the back-compat path:
// upstreams that don't advertise the field (real OpenAI, OpenRouter,
// Ollama) must still produce a usable Health with Models populated.
func TestCheckMissingSupportedEndpoints(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			w.WriteHeader(404)
			return
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"gpt-4o"}]}`))
	}))
	defer srv.Close()

	p := New()
	cfg := Config{BaseURL: srv.URL, Model: "gpt-4o"}
	health, err := p.Check(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(health.Models) != 1 || health.Models[0] != "gpt-4o" {
		t.Errorf("Models = %v, want [gpt-4o]", health.Models)
	}
	if len(health.ModelInfo) != 1 || health.ModelInfo[0].ID != "gpt-4o" {
		t.Errorf("ModelInfo = %v, want one entry for gpt-4o", health.ModelInfo)
	}
	if len(health.ModelInfo[0].SupportedEndpoints) != 0 {
		t.Errorf("SupportedEndpoints = %v, want nil/empty", health.ModelInfo[0].SupportedEndpoints)
	}
}

func TestCheckFailure(t *testing.T) {
	p := New()
	cfg := Config{BaseURL: "http://localhost:1", Model: "test"}
	_, err := p.Check(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected error for unreachable server")
	}
}

func TestGenerateSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/chat/completions" {
			json.NewEncoder(w).Encode(map[string]any{
				"choices": []map[string]any{
					{"message": map[string]string{"content": "Test response"}},
				},
			})
			return
		}
		w.WriteHeader(404)
	}))
	defer srv.Close()

	p := New()
	cfg := Config{BaseURL: srv.URL, Model: "gpt-4o"}
	result, err := p.Generate(context.Background(), cfg, GenerateRequest{
		SystemPrompt: "You are a helpful assistant.",
		UserPrompt:   "Say hello",
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if !strings.Contains(result, "Test response") {
		t.Errorf("result = %q, want 'Test response'", result)
	}
}

func TestGenerateWithAuth(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"content": "ok"}},
			},
		})
	}))
	defer srv.Close()

	t.Setenv("TEST_TOKEN", "my-secret-token")
	p := New()
	cfg := Config{BaseURL: srv.URL, Model: "gpt-4o", AuthEnv: "TEST_TOKEN"}
	_, err := p.Generate(context.Background(), cfg, GenerateRequest{UserPrompt: "test"})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if gotAuth != "Bearer my-secret-token" {
		t.Errorf("auth = %q, want 'Bearer my-secret-token'", gotAuth)
	}
}
