# Current Handoff

## Active Task

- **Task ID**: M15-W3-SLICE-D
- **Milestone**: M15 Wave 3 — Verify freshness rollout (final slice)
- **Description**: Finalize the verify-freshness work by adding `tpatch verify --all` topo-ordered aggregate reporting, rolling the §4.4 freshness bullet across all 6 skill formats, extending the `assets/assets_test.go` parity guard with the new anchors, cross-linking `docs/dependencies.md` to verify, and shipping CHANGELOG v0.6.2.
- **Status**: Not Started — ready for implementer dispatch
- **Source PRD**: `docs/prds/PRD-verify-freshness.md` §9 (Slice D row), §4.4 (skill bullet contract)

## Predecessor — Slice C

✅ Approved by external supervisor on `23af23e`. Stack on `origin/main`:

- `32f50c8` — Slice C original (V3–V9 + closure-replay)
- `5892ae0` — revision-1 (V8 runs against closure-replayed baseline when recipe absent + patch present)
- `23af23e` — revision-2 (V8 precondition is file presence, not non-empty content)
- `08ed4e5` — tracking (external verdicts logged + ROADMAP flip)

Full retrospective archived in `docs/handoff/HISTORY.md` under `2026-04-29 — M15-W3-SLICE-C`.

## Scope (per PRD §9 Slice D)

1. **`tpatch verify --all`** — new aggregate runner.
   - Topologically order all features (Kahn, hard-deps first, same primitive used by `tpatch land --all` if applicable; otherwise vendored/inlined locally).
   - Skip pre-apply features (state ∈ {drafted, analyzed, defined, explored, implemented}) per PRD Q2 and emit a one-line `skipped: pre-apply` row for each. Only states with a recorded patch (applied / reconciled / verified) execute V0–V9.
   - Per-feature output: existing single-feature verdict line + checks block, prefixed with the slug.
   - Aggregate footer: counts per verdict, plus an exit code that is non-zero if ANY feature failed (verdict ∈ {failed} or any check.severity=error fails). `--json` emits a list of per-feature reports + an aggregate summary object.
   - No new state transitions; this is a read-only aggregate over existing single-feature verify.

2. **Skill bullet rollout (all 6 surfaces)** — add the §4.4 freshness bullet to:
   - `assets/skills/claude/SKILL.md`
   - `assets/skills/copilot/SKILL.md`
   - `assets/skills/copilot-prompt/tpatch.prompt.md`
   - `assets/skills/cursor/.cursor/rules/tpatch.mdc`
   - `assets/skills/windsurf/.windsurfrules`
   - `assets/skills/generic/AGENT.md`
   The bullet must verbatim match the PRD §4.4 wording (or as close as the surface allows; copy-paste anchor strings into all six files).

3. **Parity guard extension** — `assets/assets_test.go` adds anchor checks for the §4.4 bullet across all 6 surfaces. Pattern matches existing slice rollouts (one anchor substring per file).

4. **`docs/dependencies.md` cross-link** — add a short paragraph pointing readers from the dependency model to verify, since hard-dep semantics now drive V7/V8 closure replay.

5. **`CHANGELOG.md` v0.6.2 entry** — name the verb (`tpatch verify`), call out the freshness overlay (V0–V9 numbered checks), call out the explicit out-of-scope list (no provider calls, no state transitions, no `--all` interaction with shadow). Link to PRD §9 for the slice-by-slice landing.

## Constraints

- DO NOT touch `internal/workflow/verify.go` V3–V9 logic (Slice C closed). Slice D is additive surface only.
- DO NOT touch the closure-replay primitive or shadow lifecycle.
- `verify --all` must reuse the existing single-feature `RunVerify` entrypoint per feature; no separate code path.
- Pre-apply skip must be deterministic and ordered first in topo (i.e., even features with no recorded patch participate in topo order; their skip row appears at their topo position).
- Parity guard must keep the existing anchor checks intact and only ADD new ones.
- Out-of-scope file folders: `docs/whitepapers/`, exploratory PRDs (`PRD-feature-slices-and-nested-changes.md`, `PRD-intent-version-control-evaluation.md`, `PRD-record-auto-base.md`, `PRD-record-collision-detection.md`, `PRD-tpatch-git-primitive-mapping.md`, `PRD-tpatch-land.md`).

## Tests required

- **Aggregate ordering**: 3-feature DAG with one hard-dep chain (A → B → C) and one independent feature D — assert `verify --all` runs A, B, C, D in topo order; insertion order in `.tpatch/features/` must NOT determine output order.
- **Pre-apply skip**: feature in state `defined` shows up in topo position with `skipped: pre-apply` row; does NOT execute V0; does NOT cause a non-zero exit on its own.
- **Aggregate exit code**: at least one failed feature → non-zero exit; all passing → zero exit.
- **Aggregate JSON shape**: `--json` emits `{ features: [...], summary: {passed, failed, skipped, error} }` (or equivalent already established for single-feature; extend rather than break).
- **Malformed-but-present artifact case** (carryover lesson from Slice C external review): include at least one `verify --all` test where one feature has a malformed-but-present artifact (e.g., zero-byte `post-apply.patch` or invalid `apply-recipe.json`) and assert the aggregate correctly reports that feature as failed without poisoning other features in the run.
- **Parity guard**: `go test ./assets/...` green with new anchors; intentionally remove a bullet locally to confirm the guard fails.

## Validation gate (must pass before review dispatch)

1. `gofmt -l .` — empty.
2. `go build ./cmd/tpatch` — success.
3. `go vet ./...` — clean.
4. `go test ./...` — all pass; new tests counted.
5. `tpatch verify --all` smoke run on a fixture repo with at least 3 features.
6. Skill files visually inspected — §4.4 bullet present in all 6.

## Reviewer prompt notes (for Slice D reviewer dispatch)

- **Carry forward the artifact-presence gate lesson from Slice C**: any new precondition probe added in Slice D (e.g., `verify --all` skipping pre-apply, or aggregate JSON shape gating) must be exercised with a malformed-but-present artifact case. Reviewer must explicitly run a 2-cell matrix (well-formed vs malformed-but-present) on every new gate.
- Reviewer should diff the 6 skill files against each other to confirm the bullet is consistent (not just present).
- Reviewer should `gofmt -l .` and confirm no unintended file format changes.

## Out of scope (DO NOT touch)

- V3–V9 logic in `internal/workflow/verify.go` (Slice C is closed; only `verify --all` orchestration above this layer).
- Shadow lifecycle.
- Closure-replay primitive.
- Provider integration (verify is local-only per PRD §3).
- `docs/whitepapers/` and the exploratory PRDs listed in Constraints.
- `tpatch` binary at repo root (untracked artifact).

## Files Changed

(None yet — implementer dispatch pending.)

## Test Results

(None yet.)

## Session Summary

Slice C external supervisor verdict came in APPROVED on `23af23e`. Tracking commit `08ed4e5` logged all three external verdicts (Slice C original NEEDS REVISION, rev-1 NEEDS REVISION, rev-2 APPROVED) into `docs/supervisor/LOG.md` and flipped Slice C from ⬜ to ✅ in `docs/ROADMAP.md`. Pushed the full 4-commit Slice C stack + tracking commit to `origin/main`. Archived Slice C to `docs/handoff/HISTORY.md`. This handoff stages Slice D for the next implementer dispatch.

## Next Steps

1. Hold for user direction before dispatching the Slice D implementer (per the established cadence).
2. On dispatch: implementer reads this handoff + PRD §9 Slice D + §4.4 + the Slice C archive in HISTORY.md.
3. After implementer ships, dispatch the sub-agent reviewer; on APPROVED, hand off to user for external supervisor pass.
4. After Slice D APPROVED externally, tag and ship `v0.6.2` (final M15 release).

## Blockers

None.

## Context for Next Agent

- Sub-agent reviewer prior-misses pattern is now 4 cycles deep: Slice A reviewer-1, Slice B reviewer-1, bug-fix reviewer (APPROVED WITH NOTES), Slice C reviewer-1. Slice C rev-1 + rev-2 reviewers broke the streak by following strict matrix instructions. Slice D reviewer prompts should keep the matrix-coverage discipline.
- The `tpatch` binary at repo root is now `.gitignore`d (added 2026-05-01 alongside the smart-routing work) — do NOT add it.
- `4945093` was already pushed before Slice C work; the Slice C stack itself is `32f50c8` → `5892ae0` → `23af23e` → `08ed4e5`.

## Side Work — 2026-05-01 — Smart Endpoint Routing for the copilot-api Proxy

Out-of-band fix dispatched in the same session as the Slice D queue. The
user reported `"This operation was aborted"` 500s when running
`tpatch provider set --preset copilot --model claude-opus-4.6` (and
gpt-5.5). Root cause split: the proxy's
`routes/chat-completions/handler.ts` is missing the `/v1/messages`
dispatch branch (proxy-side bug, owners notified separately) AND
tpatch was hitting `/v1/chat/completions` for Claude regardless of
what the proxy advertised on `/v1/models`.

### What landed

- `internal/provider/router.go` (NEW) — `PickProvider(cfg, *Health)`
  selects Anthropic/Responses/OpenAICompatible based on the model's
  `supported_endpoints`. Scoped to the copilot-api proxy via
  `IsCopilotProxyEndpoint`.
- `internal/provider/responses.go` (NEW) — `ResponsesProvider` for the
  OpenAI Responses API, gated behind
  `TPATCH_ENABLE_RESPONSES_PROVIDER=1` (off by default; flip when the
  upstream proxy fix lands).
- `internal/provider/errors.go` (NEW) — `ProxyUpstreamAbortedError`
  typed error + `IsProxyUpstreamAborted` + `detectProxyAbort` helper.
  Replaces the cryptic "generation returned 500" with a multi-line
  remediation message.
- `internal/provider/provider.go` — `Health` extended with
  `ModelInfo []ModelInfo`; `OpenAICompatible.Check` parses
  `supported_endpoints`. `Generate` calls `detectProxyAbort` on 500.
- `internal/provider/anthropic.go` — empty-token check relaxed to
  `token == "" && !IsCopilotProxyEndpoint(cfg)` in both `Check` and
  `Generate`; the proxy strips inbound `x-api-key`.
- `internal/provider/probe.go` — `Probe()` returns `(*Health, error)`;
  `Reachable` kept as thin wrapper. `IsCopilotProxyEndpoint` broadened
  to accept both `openai-compatible` and `anthropic` Types when URL
  contains `:4141`. Added `setForceCopilotProxy` test hook.
- `internal/cli/cobra.go` — `probedEndpoints` cache changed to
  `map[string]probedResult{health, err}`; `loadAndProbeProvider`
  routes through `PickProvider`. Dropped `AuthEnv: "GITHUB_TOKEN"`
  from the `copilot` preset (proxy strips/replaces auth headers).
- `internal/cli/copilot.go` — `ensureProviderReachable` returns
  `(*Health, error)` so the cache can hold the parsed metadata.
- `.gitignore` — added `/tpatch` rule (anchored to repo root so it
  doesn't shadow `cmd/tpatch/`).
- `docs/adrs/ADR-014-smart-endpoint-routing.md` (NEW) + index entry.
- `docs/harnesses/copilot.md` — replaced the `--auth-env GITHUB_TOKEN`
  example, documented smart routing + the `/responses`-only limitation
  and the `TPATCH_ENABLE_RESPONSES_PROVIDER` opt-in.

### Tests added

- `router_test.go` — 9-case `PickProvider` matrix (Claude/GPT-5.x/GPT-4o,
  on-proxy/off-proxy, nil health, missing model, anthropic-type on
  proxy, responses gate on/off).
- `errors_test.go` — `detectProxyAbort` matrix + `IsProxyUpstreamAborted`
  wrapping tests.
- `responses_test.go` — gate, success path, abort detection, Check.
- `anthropic_test.go` — added `TestAnthropicProxyEmptyTokenIntegration`
  + `TestAnthropicProxyAbortDetected` (use `setForceCopilotProxy`
  hook so they don't bind the privileged :4141 port).
- `provider_test.go` — `TestCheckParsesSupportedEndpoints` +
  `TestCheckMissingSupportedEndpoints`.
- `phase2_test.go` — `TestCopilotPresetNoAuthEnv` pins the dropped
  `GITHUB_TOKEN`.
- `probe_test.go` — extended `TestIsCopilotProxyEndpoint` for the
  broadened type predicate.

### Validation

- `go build ./...` clean.
- `gofmt -l .` clean.
- `go vet ./...` clean.
- `go test ./...` all packages pass (cli 23s, provider 13s, workflow 43s).

### Status of the gpt-5.5 case

`/responses`-only models still surface
`ProxyUpstreamAbortedError` until the upstream proxy team's fix lands.
`ResponsesProvider` is wired but gated; flipping the env var without
the upstream fix will not help. This is documented in
`docs/harnesses/copilot.md` and ADR-014 §"Out of scope".

### Out-of-band — does NOT block Slice D

Slice D queued work above is unaffected. The smart-routing changes
touch `internal/provider/`, `internal/cli/cobra.go` /
`internal/cli/copilot.go`, `.gitignore`, and docs only. Slice D's
edit set is `internal/workflow/verify.go` (additive),
`internal/cli/verify.go` (additive), `assets/skills/*`,
`assets/assets_test.go`, `docs/dependencies.md`, `CHANGELOG.md` — no
overlap.
