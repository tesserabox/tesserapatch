package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// ResponsesProvider speaks the OpenAI Responses API (`POST /responses`).
//
// The Responses API is what Copilot's GPT-5.x and o1 family expose;
// the local copilot-api proxy serves it from the same port as
// /v1/chat/completions and /v1/messages. Wire format reference:
// proxy/src/services/copilot/create-responses.ts (interface
// ResponsesPayload + ResponsesResponse).
//
// As of writing, the upstream Copilot /responses fetch aborts before
// sending a usable response (~230ms 500 with "This operation was
// aborted"), so this provider is feature-gated behind
// TPATCH_ENABLE_RESPONSES_PROVIDER=1. When the upstream proxy fix
// lands, flip the gate in PickProvider; the wire code below is
// already correct.
//
// Non-streaming only. The minimal Generate path used by tpatch's
// workflow phases doesn't need streaming today.
type ResponsesProvider struct {
	client *http.Client
}

// NewResponses creates a new OpenAI Responses API provider.
func NewResponses() *ResponsesProvider {
	return &ResponsesProvider{client: &http.Client{Timeout: 60 * time.Second}}
}

// responsesProviderEnabled reports whether the experimental
// /responses provider has been turned on. Off by default while the
// upstream proxy aborts upstream calls; on by setting
// TPATCH_ENABLE_RESPONSES_PROVIDER=1.
func responsesProviderEnabled() bool {
	v := strings.TrimSpace(os.Getenv("TPATCH_ENABLE_RESPONSES_PROVIDER"))
	return v == "1" || strings.EqualFold(v, "true")
}

// Check probes the proxy's /v1/models endpoint (the same one
// OpenAICompatible uses) so the configured model can be enumerated
// and `supported_endpoints` populated for downstream PickProvider
// calls. We don't issue a real /responses request because they're
// expensive and currently flaky upstream.
func (p *ResponsesProvider) Check(ctx context.Context, cfg Config) (*Health, error) {
	// Reuse OpenAICompatible.Check — the /v1/models response is
	// shared and parsing it here would duplicate the JSON shape.
	return New().Check(ctx, cfg)
}

// Generate sends a non-streaming /responses request and returns the
// concatenated assistant message text.
func (p *ResponsesProvider) Generate(ctx context.Context, cfg Config, req GenerateRequest) (string, error) {
	url := strings.TrimRight(cfg.BaseURL, "/") + "/responses"

	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = 4096
	}

	input := make([]map[string]any, 0, 2)
	if req.SystemPrompt != "" {
		// Responses API uses "developer" instead of "system" for the
		// directive role (see proxy ResponsesInput type union).
		input = append(input, map[string]any{
			"type":    "message",
			"role":    "developer",
			"content": req.SystemPrompt,
		})
	}
	input = append(input, map[string]any{
		"type":    "message",
		"role":    "user",
		"content": req.UserPrompt,
	})

	body := map[string]any{
		"model":             cfg.Model,
		"input":             input,
		"max_output_tokens": maxTokens,
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return "", err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if token := cfg.Token(); token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("generation request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		if pe := detectProxyAbort(cfg, "/responses", resp.StatusCode, string(respBody)); pe != nil {
			return "", pe
		}
		return "", fmt.Errorf("generation returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Output []struct {
			Type    string `json:"type"`
			Role    string `json:"role,omitempty"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content,omitempty"`
		} `json:"output"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("cannot parse responses response: %w", err)
	}

	var b strings.Builder
	for _, item := range result.Output {
		if item.Type != "message" {
			continue
		}
		for _, c := range item.Content {
			if c.Type == "output_text" {
				b.WriteString(c.Text)
			}
		}
	}
	out := strings.TrimSpace(b.String())
	if out == "" {
		return "", fmt.Errorf("no text content in responses response")
	}
	return out, nil
}
