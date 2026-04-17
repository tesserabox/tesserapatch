# ADR-002: Provider Strategy — Anthropic + OpenAI-Compatible Umbrella

**Status**: Accepted
**Date**: 2026-04-17
**Context**: M7 — Provider investigation and integration

## Decision

Add an `AnthropicProvider` alongside the existing `OpenAICompatible` provider,
selected via `config.provider.type`. Document that Ollama and OpenRouter work
out of the box with the existing OpenAI-compatible provider (no new code).

## Context

The MVP only supported a single provider (`OpenAICompatible`) configured with
`copilot-api` as the endpoint. M7 asked us to evaluate three candidates:

| Candidate | Protocol | API-key needed | Status in tpatch MVP |
|-----------|----------|----------------|----------------------|
| Ollama | OpenAI-compatible (`/v1/chat/completions`) | No | Works as-is; just `tpatch provider set --base-url http://localhost:11434` |
| OpenRouter | OpenAI-compatible | Yes (`OPENROUTER_API_KEY`) | Works as-is; set base-url + auth-env |
| Anthropic Messages | Different (`/v1/messages`, `x-api-key`, content blocks) | Yes (`ANTHROPIC_API_KEY`) | Needs a new provider implementation |

### Live testing performed during M7.1–M7.2

- **copilot-api** (`http://localhost:4141`): working baseline. Returns model
  list containing Claude, GPT, Gemini families. Validates our assumption that
  the `OpenAICompatible` provider is stable.
- **Ollama** (`http://localhost:11434`): not installed in this environment;
  protocol is documented as OpenAI-compatible since Ollama 0.1.24+. No code
  changes required — confirmed by inspection of the existing Generate flow
  (JSON schema matches exactly).
- **OpenRouter**: not exercised (no API key available). Protocol is
  OpenAI-compatible per vendor docs; the same Generate code path applies.

## Decision Details

### Anthropic Messages API adapter

- New file: `internal/provider/anthropic.go`
- Struct `AnthropicProvider` implements the `Provider` interface unchanged.
- Endpoint: `POST /v1/messages`
- Headers: `x-api-key: <token>`, `anthropic-version: 2023-06-01`,
  `Content-Type: application/json`
- Body: `{model, max_tokens, temperature, system, messages: [{role, content}]}`
- Response: `{content: [{type, text}, ...]}` — we concatenate all `text`
  blocks and trim.
- `Check()` issues a 1-token ping request and returns a synthetic
  `Health{Models: [configured_model]}` since Anthropic has no `/v1/models`
  endpoint.

### Factory

- New `provider.NewFromConfig(cfg)` routes by `cfg.Type`:
  - `"anthropic"` → `NewAnthropic()`
  - anything else → `New()` (OpenAI-compatible)
- `loadProviderFromStore` in the CLI uses the factory. No workflow code
  changes — the `Provider` interface is stable.

### Config surface

- `provider set --type openai-compatible|anthropic` added.
- `config set provider.type <value>` accepts the same values.

### Auto-detection extensions

- Probe `http://localhost:11434/v1/models` (Ollama) after copilot-api.
- Probe `ANTHROPIC_API_KEY` env var (configures Anthropic provider).
- Probe `OPENROUTER_API_KEY` env var (configures OpenAI-compat + OpenRouter
  base URL).
- All existing TPATCH_NO_AUTO_DETECT guards are preserved.

### Why not restructure the Provider interface

The existing interface (`Check`, `Generate`) captures exactly what the
workflow layer needs. Content blocks, streaming, tool-use, and vision are
all out of scope for the analyze/define/explore/implement phases, which
consume a single text completion. Keeping the interface stable means every
downstream call site (workflow + cli) is unchanged. If streaming or
tool-use becomes necessary later, we can add an optional capability
interface without breaking existing providers.

## Alternatives Considered

1. **Use the OpenAI-compatible shim inside Anthropic's SDK**: rejected —
   the official Anthropic SDKs do not expose a Chat Completions wrapper,
   and copilot-api already covers the Claude-via-OpenAI-shim use case for
   users who prefer that path.
2. **Single provider with per-request adapters**: rejected — the
   authentication, endpoint, and response shape differ enough that a
   type-switched pair of implementations is cleaner than a single
   function with branches.
3. **Delay Anthropic until someone asks**: rejected — direct Anthropic
   access is the primary non-proxied path for Claude models and was the
   headline candidate in the milestone.

## Consequences

- `provider` package grows from 175 LOC to ~325 LOC.
- `go.mod` dependencies unchanged (both providers use stdlib `net/http`).
- Users with `ANTHROPIC_API_KEY` set will see the provider auto-detected on
  `tpatch init`. This is a new side effect but matches the existing pattern
  for `OPENAI_API_KEY`.
- `config.yaml` now contains the `type` field explicitly; older configs
  without it default to `openai-compatible`.
- The parity guard test is unaffected by this change.

## Acceptance Evidence

- `go test ./internal/provider/... -count=1` passes, including new
  `TestAnthropicGenerate`, `TestAnthropicCheck`, `TestAnthropicMissingAuth`,
  and `TestNewFromConfig`.
- `tpatch provider set --type anthropic ...` persists to config.yaml and
  validates the provided value.
- Existing copilot-api integration continues to work unchanged
  (`tpatch provider check` against localhost:4141 returns the full model
  list).
