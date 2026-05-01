package provider

import (
	"errors"
	"strings"
	"testing"
)

func TestDetectProxyAbortMatchesProxy500(t *testing.T) {
	cfg := Config{Type: "openai-compatible", BaseURL: "http://localhost:4141", Model: "claude-opus-4.6"}
	body := `{"error":"This operation was aborted"}`
	err := detectProxyAbort(cfg, "/v1/chat/completions", 500, body)
	if err == nil {
		t.Fatal("detectProxyAbort returned nil for matching 500")
	}
	var pe *ProxyUpstreamAbortedError
	if !errors.As(err, &pe) {
		t.Fatalf("detectProxyAbort returned %T, want *ProxyUpstreamAbortedError", err)
	}
	if pe.Model != "claude-opus-4.6" {
		t.Errorf("Model = %q, want claude-opus-4.6", pe.Model)
	}
	if pe.Endpoint != "/v1/chat/completions" {
		t.Errorf("Endpoint = %q, want /v1/chat/completions", pe.Endpoint)
	}
}

func TestDetectProxyAbortNon500(t *testing.T) {
	cfg := Config{Type: "openai-compatible", BaseURL: "http://localhost:4141"}
	if err := detectProxyAbort(cfg, "/v1/chat/completions", 400, "This operation was aborted"); err != nil {
		t.Errorf("non-500 should not match: got %v", err)
	}
	if err := detectProxyAbort(cfg, "/v1/chat/completions", 200, "ok"); err != nil {
		t.Errorf("200 should not match: got %v", err)
	}
}

func TestDetectProxyAbortBodyMismatch(t *testing.T) {
	cfg := Config{Type: "openai-compatible", BaseURL: "http://localhost:4141"}
	if err := detectProxyAbort(cfg, "/v1/chat/completions", 500, "internal server error"); err != nil {
		t.Errorf("non-matching body should not produce typed error: got %v", err)
	}
}

func TestDetectProxyAbortNonProxyEndpoint(t *testing.T) {
	// Real OpenAI endpoint that happens to return a 500 with the same
	// phrase must not be misidentified as the copilot-api proxy abort.
	cfg := Config{Type: "openai-compatible", BaseURL: "https://api.openai.com", Model: "gpt-4o"}
	if err := detectProxyAbort(cfg, "/v1/chat/completions", 500, "This operation was aborted"); err != nil {
		t.Errorf("off-proxy 500 must not match: got %v", err)
	}
}

func TestDetectProxyAbortAnthropicTypeAccepted(t *testing.T) {
	// Users running --type anthropic against the proxy URL should also
	// get the typed error (the proxy serves /v1/messages from the same
	// port).
	cfg := Config{Type: "anthropic", BaseURL: "http://localhost:4141", Model: "claude-opus-4.6"}
	err := detectProxyAbort(cfg, "/v1/messages", 500, `{"error":"This operation was aborted"}`)
	if err == nil {
		t.Fatal("detectProxyAbort with anthropic type on proxy should match")
	}
	if !IsProxyUpstreamAborted(err) {
		t.Errorf("IsProxyUpstreamAborted = false, want true")
	}
}

func TestProxyUpstreamAbortedErrorMessage(t *testing.T) {
	pe := &ProxyUpstreamAbortedError{Model: "gpt-5.5", Endpoint: "/v1/chat/completions"}
	msg := pe.Error()
	// Sanity-check the embedded remediation hint so future refactors
	// don't accidentally swap it for a generic "internal error".
	for _, want := range []string{
		"gpt-5.5",
		"/v1/chat/completions",
		"copilot-api proxy",
		"upgrade",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("Error() missing %q\nfull message:\n%s", want, msg)
		}
	}
}

func TestIsProxyUpstreamAbortedWraps(t *testing.T) {
	wrapped := errors.Join(errors.New("outer"), &ProxyUpstreamAbortedError{Model: "x", Endpoint: "/y"})
	if !IsProxyUpstreamAborted(wrapped) {
		t.Error("IsProxyUpstreamAborted should detect wrapped errors")
	}
	if IsProxyUpstreamAborted(errors.New("plain")) {
		t.Error("IsProxyUpstreamAborted should not match unrelated errors")
	}
	if IsProxyUpstreamAborted(nil) {
		t.Error("IsProxyUpstreamAborted(nil) must be false")
	}
}
