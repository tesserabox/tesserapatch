# M7 — Provider Investigation & Integration

**Status**: ⬜ Not started  
**Depends on**: M6 (MVP complete)

## Overview

The current provider implementation supports only OpenAI-compatible APIs (copilot-api, OpenAI, etc.). This milestone investigates which provider to integrate next and implements it.

## Phase 1: Investigation (Research Spike)

Before writing code, evaluate three candidate paths:

### Candidate A: Anthropic Messages API (Direct)
- **Protocol**: Anthropic Messages API (`/v1/messages`), NOT OpenAI-compatible
- **Why**: Claude is the primary model used during development. Direct integration removes the copilot-api proxy dependency.
- **Complexity**: Medium — different request/response format (messages with content blocks, not chat completions)
- **Auth**: `ANTHROPIC_API_KEY` env var
- **Evaluate**: Can we share the `Provider` interface, or does it need restructuring?

### Candidate B: Ollama (Local)
- **Protocol**: OpenAI-compatible (`/v1/chat/completions`) since Ollama 0.1.24+
- **Why**: Fully local, free, no API keys. Great for development and testing.
- **Complexity**: Low — same OpenAI-compatible protocol, just different base URL
- **Auth**: None (local server)
- **Evaluate**: Does the existing `OpenAICompatible` provider already work with Ollama out of the box? (Likely yes — just `tpatch provider set --base-url http://localhost:11434`)

### Candidate C: OpenRouter / LiteLLM (Aggregator)
- **Protocol**: OpenAI-compatible
- **Why**: One integration, access to hundreds of models
- **Complexity**: Low — same protocol
- **Auth**: `OPENROUTER_API_KEY` or `LITELLM_API_KEY`
- **Evaluate**: Same as Ollama — probably works already

### Investigation Tasks

- [ ] M7.1 — Test `tpatch analyze` with Ollama locally (does the existing provider work as-is?)
- [ ] M7.2 — Test `tpatch analyze` with OpenRouter (does the existing provider work as-is?)
- [ ] M7.3 — Evaluate Anthropic Messages API: map request/response to our `GenerateRequest`/response
- [ ] M7.4 — Write investigation findings to `docs/adrs/ADR-002-provider-strategy.md`
- [ ] M7.5 — Decide: (a) Anthropic adapter, (b) confirm OpenAI-compat covers Ollama/OpenRouter, or (c) both

## Phase 2: Implementation

Based on investigation results:

- [ ] M7.6 — If Anthropic: implement `AnthropicProvider` alongside `OpenAICompatible`
- [ ] M7.7 — If Ollama/OpenRouter: document as "already supported" with config examples
- [ ] M7.8 — Add provider type detection to config: `type: anthropic | openai-compatible`
- [ ] M7.9 — Update auto-detection (GAP 6) to probe Ollama at localhost:11434
- [ ] M7.10 — Add `--provider-type` flag to `provider set` if multiple types exist
- [ ] M7.11 — Write integration tests against a mock for each provider type
- [ ] M7.12 — Update all 6 skill formats to document supported providers

## Acceptance Criteria

- Investigation ADR documents findings with test evidence
- At least one new provider path is validated end-to-end (analyze → define → explore)
- Existing copilot-api flow continues to work (no regressions)
- Provider auto-detection covers new endpoints
- Skill parity guard passes with updated provider documentation
