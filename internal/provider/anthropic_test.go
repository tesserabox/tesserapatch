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

func TestAnthropicGenerate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			http.NotFound(w, r)
			return
		}
		if got := r.Header.Get("x-api-key"); got != "sk-ant-test" {
			t.Errorf("expected x-api-key sk-ant-test, got %q", got)
		}
		if got := r.Header.Get("anthropic-version"); got == "" {
			t.Errorf("missing anthropic-version header")
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `"system"`) {
			t.Errorf("expected system field in body, got %s", string(body))
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"content": []map[string]string{
				{"type": "text", "text": "claude says hi"},
			},
		})
	}))
	defer srv.Close()

	t.Setenv("TEST_ANTH_KEY", "sk-ant-test")
	p := NewAnthropic()
	cfg := Config{Type: "anthropic", BaseURL: srv.URL, Model: "claude-sonnet-4", AuthEnv: "TEST_ANTH_KEY"}
	out, err := p.Generate(context.Background(), cfg, GenerateRequest{
		SystemPrompt: "you are a tester",
		UserPrompt:   "hi",
	})
	if err != nil {
		t.Fatalf("generate failed: %v", err)
	}
	if out != "claude says hi" {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestAnthropicCheck(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"content": []map[string]string{{"type": "text", "text": "pong"}},
		})
	}))
	defer srv.Close()

	t.Setenv("TEST_ANTH_KEY", "sk-ant-test")
	p := NewAnthropic()
	cfg := Config{Type: "anthropic", BaseURL: srv.URL, Model: "claude-sonnet-4", AuthEnv: "TEST_ANTH_KEY"}
	h, err := p.Check(context.Background(), cfg)
	if err != nil {
		t.Fatalf("check failed: %v", err)
	}
	if len(h.Models) != 1 || h.Models[0] != "claude-sonnet-4" {
		t.Errorf("expected configured model returned, got %v", h.Models)
	}
}

func TestAnthropicMissingAuth(t *testing.T) {
	p := NewAnthropic()
	cfg := Config{Type: "anthropic", BaseURL: "http://example.invalid", Model: "m", AuthEnv: "UNSET_VAR_TPATCH_XYZ"}
	_, err := p.Generate(context.Background(), cfg, GenerateRequest{UserPrompt: "hi"})
	if err == nil || !strings.Contains(err.Error(), "missing auth token") {
		t.Fatalf("expected missing auth error, got %v", err)
	}
}

// TestAnthropicProxyEmptyTokenIntegration uses httptest with the
// testForceCopilotProxy hook so the no-token bypass exercises end-to-
// end without binding the privileged :4141 port (which the user's
// real proxy may already own).
func TestAnthropicProxyEmptyTokenIntegration(t *testing.T) {
	defer setForceCopilotProxy(true)()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "" {
			t.Errorf("x-api-key should not be set when token is empty, got %q", r.Header.Get("x-api-key"))
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"content": []map[string]string{{"type": "text", "text": "ok"}},
		})
	}))
	defer srv.Close()

	cfg := Config{Type: "openai-compatible", BaseURL: srv.URL, Model: "claude-opus-4.6"}
	if !IsCopilotProxyEndpoint(cfg) {
		t.Fatal("test setup: cfg should be detected as copilot proxy")
	}
	p := NewAnthropic()
	out, err := p.Generate(context.Background(), cfg, GenerateRequest{UserPrompt: "hi"})
	if err != nil {
		t.Fatalf("Generate on proxy without token: %v", err)
	}
	if out != "ok" {
		t.Fatalf("output = %q, want ok", out)
	}
}

// TestAnthropicProxyAbortDetected exercises the typed-error path: when
// the proxy returns its 500 + "This operation was aborted" body,
// Generate must surface a *ProxyUpstreamAbortedError, not a generic
// "generation returned 500" string.
func TestAnthropicProxyAbortDetected(t *testing.T) {
	defer setForceCopilotProxy(true)()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"This operation was aborted"}`))
	}))
	defer srv.Close()

	cfg := Config{Type: "openai-compatible", BaseURL: srv.URL, Model: "claude-opus-4.6"}
	p := NewAnthropic()
	_, err := p.Generate(context.Background(), cfg, GenerateRequest{UserPrompt: "hi"})
	if !IsProxyUpstreamAborted(err) {
		t.Fatalf("expected ProxyUpstreamAbortedError, got %v", err)
	}
}

func TestNewFromConfig(t *testing.T) {
	cases := []struct {
		typ     string
		wantAnt bool
	}{
		{"anthropic", true},
		{"Anthropic", true},
		{"openai-compatible", false},
		{"", false},
		{"unknown", false},
	}
	for _, c := range cases {
		p := NewFromConfig(Config{Type: c.typ})
		_, isAnt := p.(*AnthropicProvider)
		if isAnt != c.wantAnt {
			t.Errorf("type=%q: got Anthropic=%v, want %v", c.typ, isAnt, c.wantAnt)
		}
	}
}
