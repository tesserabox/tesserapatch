package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// AnthropicProvider speaks the Anthropic Messages API (/v1/messages).
//
// Unlike OpenAI's Chat Completions, Anthropic:
//   - uses the `x-api-key` header (not `Authorization: Bearer`)
//   - requires an `anthropic-version` header
//   - takes `system` as a top-level field (not a message)
//   - returns `content` as a list of typed blocks (not `choices[].message.content`)
type AnthropicProvider struct {
	client *http.Client
}

// NewAnthropic creates a new Anthropic Messages API provider.
func NewAnthropic() *AnthropicProvider {
	return &AnthropicProvider{client: &http.Client{Timeout: 60 * time.Second}}
}

const anthropicVersion = "2023-06-01"

func anthropicBaseURL(cfg Config) string {
	base := strings.TrimRight(cfg.BaseURL, "/")
	if base == "" {
		base = "https://api.anthropic.com"
	}
	return base
}

// Check probes by issuing a minimal Messages request. Anthropic has no models
// listing endpoint, so we return a single-entry list containing the configured model.
func (p *AnthropicProvider) Check(ctx context.Context, cfg Config) (*Health, error) {
	token := cfg.Token()
	if token == "" && !IsCopilotProxyEndpoint(cfg) {
		return nil, fmt.Errorf("anthropic: missing auth token (set %s env var)", cfg.AuthEnv)
	}
	url := anthropicBaseURL(cfg) + "/v1/messages"
	body := map[string]any{
		"model":      cfg.Model,
		"max_tokens": 1,
		"messages":   []map[string]string{{"role": "user", "content": "ping"}},
	}
	payload, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("x-api-key", token)
	}
	req.Header.Set("anthropic-version", anthropicVersion)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("provider unreachable at %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("provider returned %d: %s", resp.StatusCode, string(b))
	}
	return &Health{Endpoint: anthropicBaseURL(cfg), Models: []string{cfg.Model}}, nil
}

// Generate sends a messages request and returns the concatenated text content.
func (p *AnthropicProvider) Generate(ctx context.Context, cfg Config, req GenerateRequest) (string, error) {
	token := cfg.Token()
	if token == "" && !IsCopilotProxyEndpoint(cfg) {
		return "", fmt.Errorf("anthropic: missing auth token (set %s env var)", cfg.AuthEnv)
	}
	url := anthropicBaseURL(cfg) + "/v1/messages"

	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = 4096
	}
	temp := req.Temperature
	if temp == 0 {
		temp = 0.1
	}

	body := map[string]any{
		"model":       cfg.Model,
		"max_tokens":  maxTokens,
		"temperature": temp,
		"messages":    []map[string]string{{"role": "user", "content": req.UserPrompt}},
	}
	if req.SystemPrompt != "" {
		body["system"] = req.SystemPrompt
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
	if token != "" {
		httpReq.Header.Set("x-api-key", token)
	}
	httpReq.Header.Set("anthropic-version", anthropicVersion)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("generation request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		if pe := detectProxyAbort(cfg, "/v1/messages", resp.StatusCode, string(respBody)); pe != nil {
			return "", pe
		}
		return "", fmt.Errorf("generation returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("cannot parse messages response: %w", err)
	}

	var b strings.Builder
	for _, block := range result.Content {
		if block.Type == "text" {
			b.WriteString(block.Text)
		}
	}
	out := strings.TrimSpace(b.String())
	if out == "" {
		return "", fmt.Errorf("no text content in messages response")
	}
	return out, nil
}

// NewFromConfig returns the appropriate Provider implementation for a Config.
// Defaults to OpenAICompatible for unknown or empty types.
func NewFromConfig(cfg Config) Provider {
	switch strings.ToLower(strings.TrimSpace(cfg.Type)) {
	case "anthropic":
		return NewAnthropic()
	case CopilotNativeType:
		return NewCopilotNative(cfg)
	default:
		return New()
	}
}
