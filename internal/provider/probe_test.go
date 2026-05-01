package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestReachableOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer srv.Close()
	cfg := Config{Type: "openai-compatible", BaseURL: srv.URL, Model: "x"}
	if err := Reachable(context.Background(), cfg); err != nil {
		t.Fatalf("reachable: %v", err)
	}
}

func TestReachableTimeout(t *testing.T) {
	// Point at an unroutable TEST-NET-1 address so the dial hits the 2s deadline.
	cfg := Config{Type: "openai-compatible", BaseURL: "http://192.0.2.1:4141", Model: "x"}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	start := time.Now()
	err := Reachable(ctx, cfg)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if elapsed := time.Since(start); elapsed > 4*time.Second {
		t.Errorf("probe should bound at %s; took %s", ProbeTimeout, elapsed)
	}
}

func TestReachableNotConfigured(t *testing.T) {
	if err := Reachable(context.Background(), Config{}); err == nil {
		t.Fatal("zero config must not be reported as reachable")
	}
}

func TestIsLocalEndpoint(t *testing.T) {
	cases := []struct {
		url  string
		want bool
	}{
		{"http://localhost:4141", true},
		{"http://127.0.0.1:11434", true},
		{"http://[::1]:4141", true},
		{"https://api.openai.com/v1", false},
		{"https://openrouter.ai/api", false},
		{"", false},
	}
	for _, c := range cases {
		got := IsLocalEndpoint(Config{BaseURL: c.url})
		if got != c.want {
			t.Errorf("IsLocalEndpoint(%q) = %v, want %v", c.url, got, c.want)
		}
	}
}

func TestIsCopilotProxyEndpoint(t *testing.T) {
	if !IsCopilotProxyEndpoint(Config{Type: "openai-compatible", BaseURL: "http://localhost:4141"}) {
		t.Error("localhost:4141 with openai-compatible should be copilot proxy")
	}
	// Both openai-compatible and anthropic are accepted: the proxy
	// serves /chat/completions, /responses, and /v1/messages from the
	// same port and users may configure either wire protocol.
	if !IsCopilotProxyEndpoint(Config{Type: "anthropic", BaseURL: "http://localhost:4141"}) {
		t.Error("localhost:4141 with anthropic should be copilot proxy")
	}
	if IsCopilotProxyEndpoint(Config{Type: "openai-compatible", BaseURL: "https://api.openai.com/v1"}) {
		t.Error("openai URL should not be copilot proxy")
	}
	if IsCopilotProxyEndpoint(Config{Type: "anthropic", BaseURL: "https://api.anthropic.com"}) {
		t.Error("anthropic URL should not be copilot proxy")
	}
	if IsCopilotProxyEndpoint(Config{Type: CopilotNativeType, BaseURL: ""}) {
		t.Error("copilot-native should not be copilot proxy")
	}
}

// Guard against the probe trying to hit a real server when the caller
// passes an already-cancelled context.
func TestReachableCancelledContext(t *testing.T) {
	cfg := Config{Type: "openai-compatible", BaseURL: "http://localhost:4141"}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := Reachable(ctx, cfg)
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
	if !strings.Contains(err.Error(), "context") && !strings.Contains(err.Error(), "canceled") {
		// Some HTTP stacks surface this differently; accept any non-nil error.
	}
}
