// Package provider implements the LLM provider abstraction (OpenAI-compatible).
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

// Health is the result of a provider health check.
type Health struct {
	Endpoint string   `json:"endpoint"`
	Models   []string `json:"models"`

	// ModelInfo carries richer per-model metadata when the upstream
	// /v1/models response includes it (notably supported_endpoints,
	// used by PickProvider to route Claude/GPT-5.x to the right
	// endpoint on the copilot-api proxy). Optional — providers that
	// can't enumerate models (e.g. AnthropicProvider) leave it nil.
	ModelInfo []ModelInfo `json:"model_info,omitempty"`
}

// ModelInfo describes a single model exposed by a provider, including
// any endpoint metadata advertised on /v1/models. Currently consumed
// by the copilot-api smart-routing path; other call sites should
// continue to read Models for the legacy ID-only view.
type ModelInfo struct {
	ID                 string   `json:"id"`
	SupportedEndpoints []string `json:"supported_endpoints,omitempty"`
}

// GenerateRequest is a request to generate text.
type GenerateRequest struct {
	SystemPrompt string
	UserPrompt   string
	MaxTokens    int
	Temperature  float64
}

// Config holds provider connection settings.
type Config struct {
	Type    string `json:"type"`
	BaseURL string `json:"base_url"`
	Model   string `json:"model"`
	AuthEnv string `json:"auth_env"`

	// Initiator is the optional x-initiator header value for the native
	// Copilot provider ("", "user", or "agent"). Unused for other types.
	Initiator string `json:"initiator,omitempty"`
}

// Configured returns true if the provider has enough info to connect.
// For copilot-native the BaseURL check is relaxed — the real base URL
// comes from the auth file's session.endpoints map, not from config.
func (c Config) Configured() bool {
	if strings.EqualFold(strings.TrimSpace(c.Type), CopilotNativeType) {
		return c.Model != ""
	}
	return c.BaseURL != "" && c.Model != ""
}

// Token returns the auth token from the environment variable.
func (c Config) Token() string {
	if c.AuthEnv == "" {
		return ""
	}
	return os.Getenv(c.AuthEnv)
}

// Provider is the interface for LLM backends.
type Provider interface {
	Check(ctx context.Context, cfg Config) (*Health, error)
	Generate(ctx context.Context, cfg Config, req GenerateRequest) (string, error)
}

// OpenAICompatible is a provider that speaks the OpenAI chat completions API.
type OpenAICompatible struct {
	client *http.Client
}

// New creates a new OpenAI-compatible provider.
func New() *OpenAICompatible {
	return &OpenAICompatible{
		client: &http.Client{Timeout: 60 * time.Second},
	}
}

// Check probes the /v1/models endpoint.
func (p *OpenAICompatible) Check(ctx context.Context, cfg Config) (*Health, error) {
	url := strings.TrimRight(cfg.BaseURL, "/") + "/v1/models"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if token := cfg.Token(); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("provider unreachable at %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("provider returned %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Data []struct {
			ID                 string   `json:"id"`
			SupportedEndpoints []string `json:"supported_endpoints,omitempty"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("cannot parse models response: %w", err)
	}

	models := make([]string, len(result.Data))
	infos := make([]ModelInfo, len(result.Data))
	for i, m := range result.Data {
		models[i] = m.ID
		infos[i] = ModelInfo{ID: m.ID, SupportedEndpoints: m.SupportedEndpoints}
	}

	return &Health{Endpoint: cfg.BaseURL, Models: models, ModelInfo: infos}, nil
}

// Generate sends a chat completion request and returns the response text.
func (p *OpenAICompatible) Generate(ctx context.Context, cfg Config, req GenerateRequest) (string, error) {
	url := strings.TrimRight(cfg.BaseURL, "/") + "/v1/chat/completions"

	messages := []map[string]string{}
	if req.SystemPrompt != "" {
		messages = append(messages, map[string]string{"role": "system", "content": req.SystemPrompt})
	}
	messages = append(messages, map[string]string{"role": "user", "content": req.UserPrompt})

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
		"messages":    messages,
		"max_tokens":  maxTokens,
		"temperature": temp,
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
		if pe := detectProxyAbort(cfg, "/v1/chat/completions", resp.StatusCode, string(respBody)); pe != nil {
			return "", pe
		}
		return "", fmt.Errorf("generation returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("cannot parse completion response: %w", err)
	}

	if len(result.Choices) == 0 {
		return "", fmt.Errorf("no choices in completion response")
	}

	return strings.TrimSpace(result.Choices[0].Message.Content), nil
}
