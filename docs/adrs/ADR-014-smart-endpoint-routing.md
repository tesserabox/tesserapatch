# ADR-014 — Smart Endpoint Routing for the copilot-api Proxy

**Status**: Accepted
**Date**: 2026-05-01
**Deciders**: Core
**Related**: ADR-002 (provider strategy), ADR-004 (M10 copilot proxy UX),
ADR-005 (M11 native copilot provider), `docs/harnesses/copilot.md`

## Context

The local `copilot-api` proxy at `http://localhost:4141` serves three
upstream wire formats from a single port:

- `POST /v1/chat/completions` — OpenAI Chat Completions (legacy)
- `POST /responses` — OpenAI Responses API (GPT-5.x, `o1`)
- `POST /v1/messages` — Anthropic Messages API (Claude)

The proxy's `/v1/models` response includes a per-model
`supported_endpoints: ["..."]` array advertising which routes a model
accepts. As of the proxy's `endpoint-routing.ts` refactor, two
correctness gaps appear when tpatch sends *every* request to
`/v1/chat/completions` regardless of model:

1. **Claude on `/v1/chat/completions`** — the proxy resolves the
   correct endpoint (`/v1/messages`) but
   `routes/chat-completions/handler.ts` is missing the dispatch branch
   that actually forwards there. The request falls through to the
   OpenAI-format upstream call, which upstream Copilot aborts in
   ~180 ms with `"This operation was aborted"`.
2. **GPT-5.x on `/v1/chat/completions`** — the proxy *does* dispatch to
   `/responses`, but the upstream `fetch` aborts anyway in ~230 ms.
   No client-side workaround.

A user-visible workaround existed for case 1: `tpatch provider set
--type anthropic --base-url http://localhost:4141 --model claude-...`,
which forces the AnthropicProvider and sends the request to
`/v1/messages` directly. That workaround required users to know about
the proxy bug, type three extra flags, and remember to add `--type
anthropic` every time.

We needed to either (a) wait for the upstream proxy fix and document
the workaround, or (b) push routing intelligence into our client.

## Decision

**Adopt smart endpoint routing in `internal/provider`, scoped to the
copilot-api proxy** (`provider.IsCopilotProxyEndpoint`).

Three concrete changes:

1. **Capture `supported_endpoints` from `/v1/models`.** Extend
   `provider.Health` with `ModelInfo []ModelInfo` (each entry carries
   `ID` + `SupportedEndpoints []string`). Old callers that only need
   model IDs keep using `Health.Models`; the new field is additive and
   parsed when the upstream advertises it.

2. **Pick a Provider implementation based on the model's advertised
   endpoints.** A new `provider.PickProvider(cfg, *Health) Provider`
   chooses by priority `/v1/messages > /responses > /chat/completions`.
   `loadAndProbeProvider` calls it after the reachability probe so the
   selection is transparent at the workflow layer — no flags, no
   surprises.

3. **Drop `AuthEnv: "GITHUB_TOKEN"` from the `copilot` preset.** The
   proxy strips and replaces inbound auth headers (`copilotHeaders` in
   `lib/api-config.ts`), so requiring `GITHUB_TOKEN` was theatre. The
   `AnthropicProvider`'s "missing token" gate is bypassed when
   `IsCopilotProxyEndpoint(cfg)` is true.

Selection happens once per process per base URL; results are cached in
`probedEndpoints` so workflow phases don't re-probe. When the probe is
disabled (`TPATCH_NO_PROBE=1`), routing falls back to the configured
wire type — the user-set Type wins.

`/responses`-only models (GPT-5.x) currently fall through to
`OpenAICompatible` and surface a typed `ProxyUpstreamAbortedError` with
embedded remediation hints. A `responses-provider-scaffold` task
remains queued for when the upstream proxy fix lands; flipping it on
will be a one-line change in `PickProvider`.

## Alternatives considered

- **Wait for the upstream proxy fix.** Rejected. Two failure modes hit
  users today; only one (Claude routing) is fixable upstream. The
  other (GPT-5.x upstream abort) needs a clearer client-side error
  regardless.
- **Always use `AnthropicProvider` against `localhost:4141`.** Rejected.
  GPT-4o, GPT-4.1, and other non-Claude `/chat/completions`-native
  models would break. The proxy's `/v1/messages` route does not handle
  every model.
- **Extend the `/v1/messages`-aware logic into the
  `OpenAICompatible` provider** (translate request shape internally).
  Rejected. Forces one provider type to know two wire formats; the
  separation of concerns in `internal/provider` is a strength —
  swapping the impl is cheaper than a polymorphic body.
- **Make `--type anthropic` the default for the `copilot` preset.**
  Rejected. The default model for that preset (`claude-sonnet-4`) does
  advertise `/v1/messages`, but users frequently override `--model
  gpt-4o`, which would then break in the opposite direction. Routing
  must be data-driven, not preset-driven.

## Consequences

### Positive

- `tpatch provider set --preset copilot --model claude-opus-4.6` Just
  Works without `--type anthropic`. The previous workaround is
  documented but no longer required.
- Clean error path: `ProxyUpstreamAbortedError` carries model + endpoint
  in its message and is detectable via `IsProxyUpstreamAborted` for
  workflow layers that may want to short-circuit retries.
- Future-proof: when the upstream proxy fix for `/responses` ships, we
  flip a single branch in `PickProvider` to add the
  `ResponsesProvider`. The probe path and caching are reusable.

### Negative / costs

- One more code path to maintain (`router.go` plus its tests). Mitigated
  by keeping it pure (no I/O — operates on already-probed `Health`).
- `IsCopilotProxyEndpoint` now accepts both `openai-compatible` AND
  `anthropic` Types when the URL contains `:4141`. This is a semantic
  broadening: any caller that uses it as "is the user picking the
  copilot preset?" must re-read the docstring. Two existing call sites
  (`copilot.go`'s install hint + AUP warning) still behave correctly
  under the broader definition.
- Smart routing is silent. The `tpatch config show` output still shows
  the user's stored Type (`openai-compatible`); the actual wire format
  may differ. We trade observability for ergonomics here. If users
  complain, `tpatch provider check -v` could surface "selected
  provider: AnthropicProvider via /v1/messages" — left as future work.

### Out of scope (deferred)

- Building a real `ResponsesProvider`. The proxy's `/responses` upstream
  is broken regardless of which client route we use; client work first
  is wasted.
- Filing an upstream PR for the missing
  `chat-completions/handler.ts` branch. The proxy team is already
  working on a fix; we coordinate via the
  `responses-provider-scaffold` todo so we're ready when it lands.
- Cross-host smart routing (e.g., a remote copilot-api deployment).
  `IsCopilotProxyEndpoint` keys on `:4141`; remote deployments would
  need an explicit opt-in flag. Not blocking any user today.

## Validation

- `internal/provider/router_test.go` — `PickProvider` matrix:
  Claude/GPT-5.x/GPT-4o on copilot proxy and off-proxy, nil health,
  unknown model, `--type anthropic` on proxy.
- `internal/provider/errors_test.go` — `detectProxyAbort` matrix:
  status codes, body match, off-proxy endpoint, anthropic type accepted.
- `internal/provider/anthropic_test.go` — `TestAnthropicProxyEmptyTokenIntegration`
  proves the no-token bypass works end-to-end against a fixed-port
  listener; `TestAnthropicProxyAbortDetected` proves the typed error
  surfaces from `Generate`.
- `internal/provider/provider_test.go` — `TestCheckParsesSupportedEndpoints`
  + `TestCheckMissingSupportedEndpoints` lock in the wire-level parsing
  contract.
- `internal/cli/phase2_test.go` — `TestCopilotPresetNoAuthEnv` pins
  the dropped-`GITHUB_TOKEN` decision.

All tests pass: `go test ./...` clean.
