package provider

import (
	"crypto/rand"
	"fmt"
	"net/http"
)

// Header defaults matching ericc-ch/copilot-api 0.26.7's api-config.ts.
// Keep these in lock-step with upstream when we observe a version bump.
//
// TODO(adr-005): track copilot-api's editor-version/editor-plugin-version
// pins and bump here when they bump there. The copilot-chat version
// appears in https://github.com/ericc-ch/copilot-api/blob/main/src/lib/api-config.ts.
const (
	copilotEditorVersion       = "vscode/1.95.0"
	copilotEditorPluginVersion = "copilot-chat/0.26.7"
	copilotUserAgent           = "GitHubCopilotChat/0.26.7"
	copilotIntegrationID       = "vscode-chat"
	copilotOpenAIIntent        = "conversation-panel"
	copilotAPIVersion          = "2025-04-01"
	copilotVSCodeUALibVersion  = "electron-fetch"
)

// applyCopilotEditorHeaders stamps the VS Code Copilot Chat headers on
// h. The caller is responsible for setting Authorization and any
// per-request identifiers (x-request-id).
//
// initiator is the value for the optional x-initiator header:
//   - "" (default) → header omitted, matching copilot-api/litellm
//   - "agent"      → x-initiator: agent
//   - "user"       → x-initiator: user
func applyCopilotEditorHeaders(h http.Header, initiator string) {
	h.Set("editor-version", copilotEditorVersion)
	h.Set("editor-plugin-version", copilotEditorPluginVersion)
	h.Set("user-agent", copilotUserAgent)
	h.Set("copilot-integration-id", copilotIntegrationID)
	h.Set("openai-intent", copilotOpenAIIntent)
	h.Set("x-github-api-version", copilotAPIVersion)
	h.Set("x-vscode-user-agent-library-version", copilotVSCodeUALibVersion)
	if initiator == "agent" || initiator == "user" {
		h.Set("x-initiator", initiator)
	}
}

// newRequestID returns a random, unpadded hex identifier suitable for
// the x-request-id header. Uses crypto/rand for uniqueness; does not
// follow the UUID v4 format but copilot-api accepts any unique string.
func newRequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// Extremely unlikely; fallback is fine for a request correlator.
		return "tpatch-reqid-fallback"
	}
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
