package provider

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// ProbeTimeout is the default timeout for reachability probes. Kept short
// because a probe runs before every first workflow call; any value over
// ~2s starts to feel sluggish.
const ProbeTimeout = 2 * time.Second

// Reachable performs a fast best-effort health check against cfg's endpoint.
// It returns nil when the provider's /v1/models (or Anthropic equivalent)
// responds with a 200, or a descriptive error otherwise.
//
// Call-site pattern:
//
//	if err := provider.Reachable(ctx, cfg); err != nil {
//	    // warn-and-continue on init, or hard-fail on workflow commands
//	}
//
// Honors the caller-supplied deadline but enforces an upper bound of
// ProbeTimeout so a mistakenly-passed background context cannot hang.
//
// Use Probe instead when you need the resolved *Health back (e.g. to
// feed PickProvider).
func Reachable(ctx context.Context, cfg Config) error {
	_, err := Probe(ctx, cfg)
	return err
}

// Probe is Reachable + returns the resolved *Health so callers that
// need richer model metadata (notably supported_endpoints, used by
// PickProvider) can avoid re-issuing the /v1/models call.
func Probe(ctx context.Context, cfg Config) (*Health, error) {
	if !cfg.Configured() {
		return nil, fmt.Errorf("provider is not configured")
	}
	probeCtx, cancel := context.WithTimeout(ctx, ProbeTimeout)
	defer cancel()
	prov := NewFromConfig(cfg)
	return prov.Check(probeCtx, cfg)
}

// IsLocalEndpoint reports whether cfg points at a local address. Callers
// use this to decide whether to probe (local proxies benefit from a
// reachability check; remote endpoints we trust the user's network for).
func IsLocalEndpoint(cfg Config) bool {
	u := strings.ToLower(strings.TrimSpace(cfg.BaseURL))
	return strings.HasPrefix(u, "http://localhost:") ||
		strings.HasPrefix(u, "http://127.0.0.1:") ||
		strings.HasPrefix(u, "http://[::1]:") ||
		strings.HasPrefix(u, "https://localhost:") ||
		strings.HasPrefix(u, "https://127.0.0.1:")
}

// IsCopilotProxyEndpoint reports whether cfg looks like it points at a
// local copilot-api proxy (default port 4141). Used to scope Copilot-
// specific install hints, AUP warnings, smart routing in PickProvider,
// and the empty-token bypass in AnthropicProvider (the proxy strips
// inbound auth headers and forwards its own session token, so missing
// upstream auth isn't an error when targeting it).
//
// Both `openai-compatible` and `anthropic` Types are recognised: the
// proxy serves `/v1/chat/completions`, `/responses`, and `/v1/messages`
// from the same port, and users may legitimately configure either
// wire protocol against the same proxy URL.
func IsCopilotProxyEndpoint(cfg Config) bool {
	if testForceCopilotProxy {
		return true
	}
	switch cfg.Type {
	case "openai-compatible", "anthropic":
		// supported
	default:
		return false
	}
	u := strings.ToLower(strings.TrimSpace(cfg.BaseURL))
	return strings.Contains(u, ":4141")
}

// testForceCopilotProxy is flipped by tests via setForceCopilotProxy
// when they need IsCopilotProxyEndpoint to return true against a
// random-port httptest.Server (so the empty-token bypass and proxy-
// abort detection can be exercised without binding the privileged
// 4141 port). Production code never touches this flag.
var testForceCopilotProxy bool

// setForceCopilotProxy is a test hook. Returns a restore function so
// callers can defer restoration even if t.Cleanup isn't available.
func setForceCopilotProxy(v bool) func() {
	prev := testForceCopilotProxy
	testForceCopilotProxy = v
	return func() { testForceCopilotProxy = prev }
}
