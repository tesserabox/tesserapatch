package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// CopilotClientID is the well-known VS Code Copilot Chat GitHub App
// client ID, shared across every field implementation that impersonates
// the editor extension (copilot-api, litellm, aider, Neovim Copilot).
// Not a secret; documented in every reference implementation.
const CopilotClientID = "Iv1.b507a08c87ecfe98"

// Device-flow endpoints. Enterprise deployments swap the host for the
// GHES domain; see copilotDeviceFlowHost.
const (
	githubDeviceCodeHost   = "github.com"
	githubAPIHost          = "api.github.com"
	defaultDeviceCodePath  = "/login/device/code"
	defaultAccessTokenPath = "/login/oauth/access_token"
	sessionTokenPath       = "/copilot_internal/v2/token"
)

// DeviceCodeResponse is the JSON reply from /login/device/code.
type DeviceCodeResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

// DeviceTokenResponse is the JSON reply from /login/oauth/access_token.
// Either AccessToken or Error+ErrorDescription is populated.
type DeviceTokenResponse struct {
	AccessToken      string `json:"access_token"`
	TokenType        string `json:"token_type"`
	Scope            string `json:"scope"`
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

// CopilotLoginOptions configures the device-flow handshake.
type CopilotLoginOptions struct {
	// EnterpriseDomain is the GHES host (e.g. "company.ghe.com") when
	// logging into GitHub Enterprise Cloud / Server. Empty = github.com.
	EnterpriseDomain string

	// HTTPClient is the transport used for all GitHub API calls. Tests
	// pass a fake; production uses http.DefaultClient with a timeout.
	HTTPClient *http.Client

	// Prompt writes the user_code + verification_uri to the UI. Tests
	// may pass an io.Discard or a capturing writer.
	Prompt io.Writer

	// OverrideBaseURL points device+token+session calls at a test
	// server. When set, the (enterprise-aware) URL builders are bypassed.
	OverrideBaseURL string
}

// httpClient returns the caller-provided client or a default with a
// sane timeout. GitHub's device endpoints typically respond in <1s.
func (o CopilotLoginOptions) httpClient() *http.Client {
	if o.HTTPClient != nil {
		return o.HTTPClient
	}
	return &http.Client{Timeout: 15 * time.Second}
}

// deviceCodeURL returns the full URL for the device-code request.
func (o CopilotLoginOptions) deviceCodeURL() string {
	if o.OverrideBaseURL != "" {
		return strings.TrimRight(o.OverrideBaseURL, "/") + defaultDeviceCodePath
	}
	host := githubDeviceCodeHost
	if o.EnterpriseDomain != "" {
		host = o.EnterpriseDomain
	}
	return "https://" + host + defaultDeviceCodePath
}

// accessTokenURL returns the full URL for token polling.
func (o CopilotLoginOptions) accessTokenURL() string {
	if o.OverrideBaseURL != "" {
		return strings.TrimRight(o.OverrideBaseURL, "/") + defaultAccessTokenPath
	}
	host := githubDeviceCodeHost
	if o.EnterpriseDomain != "" {
		host = o.EnterpriseDomain
	}
	return "https://" + host + defaultAccessTokenPath
}

// sessionTokenURL returns the Copilot session-token exchange endpoint.
// Note: this uses api.github.com (or api.<ghe-domain>) not the device-
// flow host.
func (o CopilotLoginOptions) sessionTokenURL() string {
	if o.OverrideBaseURL != "" {
		return strings.TrimRight(o.OverrideBaseURL, "/") + sessionTokenPath
	}
	host := githubAPIHost
	if o.EnterpriseDomain != "" {
		host = "api." + o.EnterpriseDomain
	}
	return "https://" + host + sessionTokenPath
}

// RequestDeviceCode kicks off the device-authorization flow and returns
// the user_code/verification_uri pair the user must visit.
//
// Per rubber-duck #1: GitHub answers with form-encoded bodies unless we
// explicitly ask for JSON; this function always sets Accept.
func RequestDeviceCode(ctx context.Context, opts CopilotLoginOptions) (*DeviceCodeResponse, error) {
	body := url.Values{}
	body.Set("client_id", CopilotClientID)
	body.Set("scope", "read:user")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, opts.deviceCodeURL(),
		strings.NewReader(body.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", copilotUserAgent)
	resp, err := opts.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("device-code request failed: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("device-code returned %d: %s", resp.StatusCode, string(raw))
	}
	var out DeviceCodeResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("device-code parse: %w (body=%s)", err, string(raw))
	}
	if out.DeviceCode == "" || out.UserCode == "" {
		return nil, fmt.Errorf("device-code response missing fields: %s", string(raw))
	}
	if out.Interval <= 0 {
		out.Interval = 5
	}
	if out.ExpiresIn <= 0 {
		out.ExpiresIn = 900
	}
	return &out, nil
}

// PollAccessToken blocks until the user completes the browser grant,
// the server reports a terminal error, the local expiry fires, or the
// context is cancelled.
//
// Adopts rubber-duck #1 — `slow_down` permanently increases the poll
// interval (GitHub's device-flow spec).
func PollAccessToken(ctx context.Context, opts CopilotLoginOptions, dc *DeviceCodeResponse) (string, error) {
	interval := time.Duration(dc.Interval) * time.Second
	if interval <= 0 {
		interval = 5 * time.Second
	}
	deadline := time.Now().Add(time.Duration(dc.ExpiresIn) * time.Second)

	for {
		if time.Now().After(deadline) {
			return "", fmt.Errorf("device code expired before user approved")
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(interval):
		}

		body := url.Values{}
		body.Set("client_id", CopilotClientID)
		body.Set("device_code", dc.DeviceCode)
		body.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, opts.accessTokenURL(),
			strings.NewReader(body.Encode()))
		if err != nil {
			return "", err
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("User-Agent", copilotUserAgent)
		resp, err := opts.httpClient().Do(req)
		if err != nil {
			return "", fmt.Errorf("poll failed: %w", err)
		}
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		var tok DeviceTokenResponse
		if err := json.Unmarshal(raw, &tok); err != nil {
			return "", fmt.Errorf("poll parse: %w (body=%s)", err, string(raw))
		}
		if tok.AccessToken != "" {
			return tok.AccessToken, nil
		}
		switch tok.Error {
		case "authorization_pending":
			// Wait out the interval and retry.
		case "slow_down":
			// Per RFC 8628 §3.5, permanently bump the interval.
			interval += 5 * time.Second
		case "expired_token":
			return "", fmt.Errorf("device code expired: %s", tok.ErrorDescription)
		case "access_denied":
			return "", fmt.Errorf("user denied authorization")
		case "":
			return "", fmt.Errorf("unexpected empty poll response: %s", string(raw))
		default:
			return "", fmt.Errorf("device-flow error %q: %s", tok.Error, tok.ErrorDescription)
		}
	}
}

// ExchangeSessionToken calls copilot_internal/v2/token with the long-
// lived OAuth token and returns a new ~25-min session token. Updates
// the provided auth in-place (session block).
//
// Per rubber-duck #2 + #5: uses endpoints["api"] verbatim from the
// response, does not reconstruct the host.
func ExchangeSessionToken(ctx context.Context, opts CopilotLoginOptions, auth *CopilotAuth) error {
	authStoreMu.Lock()
	defer authStoreMu.Unlock()
	return exchangeSessionTokenLocked(ctx, opts, auth)
}

// exchangeSessionTokenLocked is the body of ExchangeSessionToken that
// assumes the caller already holds authStoreMu. Used by the provider's
// retry-on-401 path which needs to hold the lock across invalidate+
// refresh.
func exchangeSessionTokenLocked(ctx context.Context, opts CopilotLoginOptions, auth *CopilotAuth) error {
	if auth == nil || auth.OAuth.AccessToken == "" {
		return errors.New("no OAuth token available; run `tpatch provider copilot-login`")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, opts.sessionTokenURL(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "token "+auth.OAuth.AccessToken)
	applyCopilotEditorHeaders(req.Header, "")
	req.Header.Set("x-request-id", newRequestID())

	resp, err := opts.httpClient().Do(req)
	if err != nil {
		return fmt.Errorf("session-token exchange failed: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusUnauthorized {
		return errCopilotUnauthorized{msg: string(raw)}
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("session-token exchange %d: %s", resp.StatusCode, string(raw))
	}

	// copilot_internal/v2/token returns a superset of fields; we only
	// care about token, expires_at, and endpoints. expires_at is an
	// epoch-seconds integer in this response, not RFC3339.
	var out struct {
		Token     string            `json:"token"`
		ExpiresAt int64             `json:"expires_at"`
		Endpoints map[string]string `json:"endpoints"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return fmt.Errorf("session-token parse: %w (body=%s)", err, string(raw))
	}
	if out.Token == "" || out.ExpiresAt == 0 {
		return fmt.Errorf("session-token response missing token/expires_at: %s", string(raw))
	}
	auth.Session.Token = out.Token
	auth.Session.ExpiresAt = time.Unix(out.ExpiresAt, 0).UTC().Format(time.RFC3339)
	auth.Session.Endpoints = out.Endpoints
	auth.Session.RefreshedAt = time.Now().UTC().Format(time.RFC3339)
	return nil
}

// errCopilotUnauthorized signals that the OAuth token was rejected.
// Callers unwrap it to decide whether to prompt the user to re-login.
type errCopilotUnauthorized struct{ msg string }

func (e errCopilotUnauthorized) Error() string {
	if e.msg == "" {
		return "copilot: OAuth token rejected (run `tpatch provider copilot-login` again)"
	}
	return "copilot: OAuth token rejected: " + e.msg
}

// IsCopilotAuthError reports whether err is a typed Copilot auth
// failure that should prompt the user to re-run copilot-login.
func IsCopilotAuthError(err error) bool {
	if err == nil {
		return false
	}
	var e errCopilotUnauthorized
	return errors.As(err, &e)
}

// PrintDevicePrompt writes the user_code + verification_uri to w in a
// format that's easy to spot in a terminal. Separated so login commands
// can customise framing.
func PrintDevicePrompt(w io.Writer, dc *DeviceCodeResponse) {
	if w == nil {
		return
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "To authorize tpatch with GitHub Copilot:")
	fmt.Fprintf(w, "  1. Open %s\n", dc.VerificationURI)
	fmt.Fprintf(w, "  2. Enter this code: %s\n", dc.UserCode)
	fmt.Fprintf(w, "  3. Wait — this window will update when GitHub approves.\n")
	fmt.Fprintln(w)
}
