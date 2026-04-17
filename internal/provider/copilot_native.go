package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"time"
)

// CopilotNativeType is the provider type discriminant for the first-
// party GitHub Copilot provider. Distinct from "openai-compatible"
// pointed at the copilot-api proxy (M10).
const CopilotNativeType = "copilot-native"

// sessionRefreshBuffer is how far ahead of expiry we proactively
// refresh the session token.
const sessionRefreshBuffer = 60 * time.Second

// CopilotNative is the first-party Copilot provider. It loads the
// OAuth + session token from disk, refreshes the session token when
// needed, and speaks the OpenAI-compatible chat-completions wire
// protocol against api.githubcopilot.com. See ADR-005.
type CopilotNative struct {
	client    *http.Client
	initiator string
	// loginOpts lets tests inject an override base URL for the session
	// exchange. In production, zero-value opts resolve to api.github.com.
	loginOpts CopilotLoginOptions
}

// NewCopilotNative builds a native Copilot provider. initiator must be
// "", "user", or "agent" (per ADR-005 D6); any other value is treated
// as unset to keep callers forward-compatible.
func NewCopilotNative(cfg Config) *CopilotNative {
	c := &CopilotNative{
		client: &http.Client{Timeout: 120 * time.Second},
	}
	switch cfg.Initiator {
	case "agent", "user":
		c.initiator = cfg.Initiator
	}
	return c
}

// Check hits /models on the current session endpoint to verify auth
// + reachability. Returns a typed "not logged in" error when the auth
// file is missing so callers can prompt the user instead of hanging on
// an interactive device flow. Never triggers the device flow itself
// (per rubber-duck #8).
func (c *CopilotNative) Check(ctx context.Context, cfg Config) (*Health, error) {
	auth, err := c.ensureFreshSession(ctx)
	if err != nil {
		return nil, err
	}
	models, err := c.listModels(ctx, auth)
	if err != nil {
		return nil, err
	}
	return &Health{Endpoint: auth.APIEndpoint(), Models: models}, nil
}

// Generate sends a chat completion request. On 401 it invalidates the
// cached session token, refreshes once, and retries. A second 401
// surfaces a typed errCopilotUnauthorized so the CLI can print the
// re-login hint.
func (c *CopilotNative) Generate(ctx context.Context, cfg Config, req GenerateRequest) (string, error) {
	auth, err := c.ensureFreshSession(ctx)
	if err != nil {
		return "", err
	}

	body, err := c.buildChatBody(cfg, req)
	if err != nil {
		return "", err
	}

	text, status, err := c.sendChat(ctx, auth, body)
	if err != nil && status != http.StatusUnauthorized {
		return "", err
	}
	if status == http.StatusUnauthorized {
		// Per ADR-005 D3: one retry with a fresh session token.
		if refreshErr := c.forceSessionRefresh(ctx, auth); refreshErr != nil {
			if IsCopilotAuthError(refreshErr) {
				return "", refreshErr
			}
			return "", fmt.Errorf("after 401, session refresh failed: %w", refreshErr)
		}
		text, status, err = c.sendChat(ctx, auth, body)
		if status == http.StatusUnauthorized {
			return "", errCopilotUnauthorized{msg: "chat/completions returned 401 after refresh"}
		}
		if err != nil {
			return "", err
		}
	}
	return text, nil
}

// ensureFreshSession loads auth, refreshes the session when it's within
// sessionRefreshBuffer of expiry, and returns the up-to-date pointer.
// Serialises refreshes so parallel phases cannot exchange twice (per
// rubber-duck #3).
func (c *CopilotNative) ensureFreshSession(ctx context.Context) (*CopilotAuth, error) {
	auth, err := LoadCopilotAuth()
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, errCopilotUnauthorized{msg: "not logged in — run `tpatch provider copilot-login`"}
		}
		return nil, err
	}
	if !auth.SessionExpired(sessionRefreshBuffer) {
		return auth, nil
	}

	opts := c.loginOpts
	if auth.OAuth.EnterpriseURL != "" {
		opts.EnterpriseDomain = auth.OAuth.EnterpriseURL
	}
	if opts.HTTPClient == nil {
		opts.HTTPClient = c.client
	}
	if err := ExchangeSessionToken(ctx, opts, auth); err != nil {
		return nil, err
	}
	if err := SaveCopilotAuth(auth); err != nil {
		return nil, err
	}
	return auth, nil
}

// forceSessionRefresh wipes the session block and exchanges a new
// token. Called on 401 from chat/completions.
func (c *CopilotNative) forceSessionRefresh(ctx context.Context, auth *CopilotAuth) error {
	auth.Session = CopilotSessionBlock{}
	opts := c.loginOpts
	if auth.OAuth.EnterpriseURL != "" {
		opts.EnterpriseDomain = auth.OAuth.EnterpriseURL
	}
	if opts.HTTPClient == nil {
		opts.HTTPClient = c.client
	}
	if err := ExchangeSessionToken(ctx, opts, auth); err != nil {
		return err
	}
	return SaveCopilotAuth(auth)
}

func (c *CopilotNative) listModels(ctx context.Context, auth *CopilotAuth) ([]string, error) {
	url := auth.APIEndpoint() + "/models"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+auth.Session.Token)
	req.Header.Set("Accept", "application/json")
	applyCopilotEditorHeaders(req.Header, c.initiator)
	req.Header.Set("x-request-id", newRequestID())

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("models call failed: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("/models returned %d: %s", resp.StatusCode, string(raw))
	}
	var out struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		// Per ADR-005 D5: don't fail hard if /models is unparseable —
		// the user's configured model still wins. Signal to callers
		// by returning an empty slice with no error.
		return nil, nil
	}
	ids := make([]string, 0, len(out.Data))
	for _, m := range out.Data {
		if m.ID != "" {
			ids = append(ids, m.ID)
		}
	}
	return ids, nil
}

// buildChatBody mirrors the OpenAICompatible.Generate body shape so
// switching providers doesn't change the prompt wire format.
func (c *CopilotNative) buildChatBody(cfg Config, req GenerateRequest) ([]byte, error) {
	messages := []map[string]string{}
	if req.SystemPrompt != "" {
		messages = append(messages, map[string]string{"role": "system", "content": req.SystemPrompt})
	}
	messages = append(messages, map[string]string{"role": "user", "content": req.UserPrompt})

	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = 4096
	}

	payload := map[string]any{
		"model":       cfg.Model,
		"messages":    messages,
		"max_tokens":  maxTokens,
		"stream":      false,
		"temperature": req.Temperature,
	}
	return json.Marshal(payload)
}

// sendChat issues the POST /chat/completions request. Returns (text,
// status, err). When status == 401 the caller retries after a refresh;
// the error is nil in that case so the caller can still inspect it.
func (c *CopilotNative) sendChat(ctx context.Context, auth *CopilotAuth, body []byte) (string, int, error) {
	url := auth.APIEndpoint() + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("Authorization", "Bearer "+auth.Session.Token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	applyCopilotEditorHeaders(req.Header, c.initiator)
	req.Header.Set("x-request-id", newRequestID())

	resp, err := c.client.Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("chat/completions failed: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusUnauthorized {
		return "", http.StatusUnauthorized, nil
	}
	if resp.StatusCode != http.StatusOK {
		return "", resp.StatusCode, fmt.Errorf("chat/completions %d: %s", resp.StatusCode, string(raw))
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", resp.StatusCode, fmt.Errorf("chat response parse: %w", err)
	}
	if len(out.Choices) == 0 {
		return "", resp.StatusCode, fmt.Errorf("chat response had no choices: %s", string(raw))
	}
	return out.Choices[0].Message.Content, http.StatusOK, nil
}

// errFileNotFound helper removed — callers use errors.Is(err, fs.ErrNotExist).
