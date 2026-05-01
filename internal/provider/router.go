package provider

// router.go — endpoint-aware provider selection for the copilot-api
// proxy. When the proxy's /v1/models response includes
// `supported_endpoints` per model (claude-*, gpt-5.x, etc.), pick the
// Provider implementation whose wire format matches the highest-
// capability endpoint the model advertises.
//
// Priority:
//
//	/v1/messages > /responses > /chat/completions
//
// Smart routing is scoped to the copilot-api proxy
// (IsCopilotProxyEndpoint). Outside that scope — real OpenAI, real
// Anthropic, Ollama, OpenRouter — we preserve the previous behaviour
// of NewFromConfig(cfg) so users don't see a transparent provider
// swap.

// PickProvider returns the right Provider implementation for cfg
// given the resolved *Health from a probe. Falls back to
// NewFromConfig(cfg) when:
//
//   - cfg does not target the copilot-api proxy, or
//   - health is nil / has no ModelInfo for cfg.Model, or
//   - the model only advertises endpoints we already match by default.
//
// When the model advertises /v1/messages we return AnthropicProvider
// (the proxy's /v1/messages route handles native passthrough,
// /responses, and /chat/completions translation internally — see
// `routes/messages/handler.ts` in the proxy). This sidesteps the
// missing branch in `routes/chat-completions/handler.ts` that aborts
// when a Claude model resolves to /v1/messages.
//
// /responses-only models (e.g. gpt-5.5) are NOT yet special-cased:
// the proxy currently aborts upstream regardless of which entry route
// we use, so client-side routing can't help. The error path in
// Generate surfaces a helpful hint instead. Once the
// ResponsesProvider scaffold (todo: responses-provider-scaffold) is
// flipped on, the second branch below should return that provider.
func PickProvider(cfg Config, health *Health) Provider {
	if !IsCopilotProxyEndpoint(cfg) || health == nil {
		return NewFromConfig(cfg)
	}

	supported := lookupSupportedEndpoints(health.ModelInfo, cfg.Model)
	if len(supported) == 0 {
		return NewFromConfig(cfg)
	}

	if hasEndpoint(supported, "/v1/messages") {
		return NewAnthropic()
	}
	if hasEndpoint(supported, "/responses") && responsesProviderEnabled() {
		// Off by default: the upstream Copilot /responses fetch is
		// currently aborted by the proxy regardless of route. Once
		// the upstream fix ships, flip
		// TPATCH_ENABLE_RESPONSES_PROVIDER=1 (or remove the gate).
		// See ADR-014 + responses.go for details.
		return NewResponses()
	}
	// /responses (gated off) and /chat/completions both flow through
	// the default OpenAICompatible provider today. The proxy's
	// chat-completions handler routes /responses internally;
	// OpenAICompatible just posts the OpenAI-format payload — when
	// the model is /responses-only the request will surface a
	// ProxyUpstreamAbortedError with a remediation hint.
	return NewFromConfig(cfg)
}

// lookupSupportedEndpoints finds the supported_endpoints list for
// modelID in info, or nil when not present.
func lookupSupportedEndpoints(info []ModelInfo, modelID string) []string {
	for _, m := range info {
		if m.ID == modelID {
			return m.SupportedEndpoints
		}
	}
	return nil
}

func hasEndpoint(list []string, want string) bool {
	for _, e := range list {
		if e == want {
			return true
		}
	}
	return false
}
