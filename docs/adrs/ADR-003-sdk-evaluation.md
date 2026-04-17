# ADR-003: SDK Evaluation — Keep Stdlib Providers, Use Presets for API Parity

**Status**: Accepted
**Date**: 2026-04-17
**Context**: Follow-up to ADR-002. The user asked us to evaluate mainstream Go SDKs and agent CLIs so the tpatch provider layer stays simple, well-tested, and doesn't waste resources on wrappers we don't need.

## Decision

1. **Do not adopt third-party provider SDKs.** Keep the existing stdlib-only `OpenAICompatible` and `AnthropicProvider` implementations.
2. **Do not adopt the `OpenRouterTeam/go-sdk`.** Ship a `copilot | openai | openrouter | anthropic | ollama` preset switch in `tpatch provider set --preset <name>` so users get the same ergonomics without the dependency weight.
3. **Position `codex` and `copilot` CLIs as *harnesses* that invoke `tpatch`, not as providers.** Document the `tpatch next --format harness-json` contract in per-harness integration guides.

## Evaluation Matrix

| Candidate | Type | Protocol | Maturity (2026-04) | Fit for tpatch |
|-----------|------|----------|--------------------|----------------|
| [`OpenRouterTeam/go-sdk`](https://github.com/OpenRouterTeam/go-sdk) | Provider SDK (Speakeasy-generated) | OpenAI-compatible (Bearer + `OPENROUTER_API_KEY`) | README explicitly says "not yet ready for production use" | **Reject** — OpenRouter is already a drop-in OpenAI-compatible endpoint. Our existing provider speaks it today. Adopting the SDK would add ~20 transitive deps for zero new capability. |
| [`openai/openai-go`](https://github.com/openai/openai-go) | Official OpenAI SDK | Chat completions + Responses | GA | **Reject for now** — brings in full Responses API, streaming, tool calls that we do not use. A `--preset openai` covers the 99% case; if we ever need Responses API features, revisit. |
| [`anthropics/anthropic-sdk-go`](https://github.com/anthropics/anthropic-sdk-go) | Official Anthropic SDK | Messages API | GA | **Reject** — our stdlib `AnthropicProvider` is 145 LOC, tested, and exercises exactly the one endpoint we need (`POST /v1/messages`). An SDK would force us to deal with streaming/tool-use surface we do not exercise. |
| [`openai/codex`](https://github.com/openai/codex) | Agent harness CLI | `codex exec <prompt>` spawns an agent that runs shell tools | GA | **Accept as harness** — codex is not a provider. It is a sibling agent that calls `tpatch` commands. Document the handshake. |
| [`github/copilot-cli`](https://github.com/github/copilot-cli) | Agent harness CLI | `copilot` runs an agentic loop with MCP + shell | Early but public | **Accept as harness** — same role as codex. Can invoke `tpatch next --format harness-json`. |

## Rationale

### Provider layer
- The tpatch Provider interface has exactly two methods: `Check(ctx, cfg)` and `Generate(ctx, cfg, req)`. We do not use streaming, tool use, embeddings, function calling, responses API, vision, or file uploads. These are the features that justify an SDK. Without them, an SDK is overhead.
- Stdlib `net/http` plus `encoding/json` is ~170 LOC per provider, boots in one package, has no version pinning risk, and is easy to mock in tests with `httptest`.
- API parity for OpenAI-compatible endpoints (OpenAI, OpenRouter, Ollama, Groq, Together, copilot-api) is a protocol property, not an SDK property. By speaking the protocol directly we automatically support new vendors the moment they implement it.
- For the Anthropic Messages API, the protocol is stable (`anthropic-version: 2023-06-01`) and our direct implementation covers it. If Anthropic deprecates v1, we update one file.

### Presets
- "Simple integration with proven implementations" is solved by *configuration ergonomics*, not by SDKs. `tpatch provider set --preset openrouter` is one command; discovering the right base URL is zero.
- The `providerPresets` map in `internal/cli/cobra.go` is the single source of truth for both `provider set --preset` and `autoDetectProvider`, so adding a new provider ecosystem is a three-line change.

### Harnesses
- `codex exec` and `copilot` are *callers* of `tpatch`, not callees. Their agents receive `AGENTS.md` / skill files that instruct them to run `tpatch next`, parse the JSON, and execute the suggested command.
- This inverts the dependency: instead of tpatch embedding harness SDKs (which don't exist in a library form anyway), harnesses read our CLI contract. That keeps tpatch a single static binary.
- The `tpatch next --format harness-json` payload is the protocol. We commit to keeping it stable and versioned.

## Consequences

### Added
- `tpatch provider set --preset <copilot|openai|openrouter|anthropic|ollama>` — one-command configuration, regression-tested.
- Single `providerPresets` map powering both `--preset` and auto-detection.
- `docs/harnesses/codex.md` — integration guide for OpenAI Codex CLI.
- `docs/harnesses/copilot.md` — integration guide for GitHub Copilot CLI.

### Not added
- No new Go dependencies. `go.mod` stays on `cobra/pflag` + stdlib.
- No streaming, tool-use, or Responses API surface in the provider layer.

### Follow-up triggers (revisit this ADR if)
- We need streaming for a new workflow phase (then evaluate `openai-go` + `anthropic-sdk-go` together).
- A provider ships features behind a non-standard schema we can't mirror in <50 LOC.
- A harness publishes an official Go client that makes direct invocation (MCP, RPC) materially easier than the CLI contract.

## Acceptance Evidence

- `TestProviderSetPreset` validates `--preset openrouter|anthropic`, including composition with explicit flags and rejection of unknown presets.
- Existing `TestAnthropicGenerate`, `TestAnthropicCheck`, `TestNewFromConfig`, `TestCycleBatchHeuristic`, and the parity guard continue to pass.
- `go.mod` direct dependencies unchanged: `github.com/spf13/cobra` only.
