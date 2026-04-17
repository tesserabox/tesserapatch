# Handoff History

*Completed handoff entries are archived here in reverse chronological order.*

---

## 2026-04-17 — Phase 2 Refinement: SDK Evaluation + Harness Guides + Tracking Cadence (v0.3.0-dev)

**Task**: Evaluate mainstream Go SDKs and agent CLIs; adopt simplest integration; tighten tracking cadence
**Agent**: Phase 2 refinement agent
**Verdict**: SUPERSEDED by 2026-04-17 distribution setup entry (see LOG.md)

## Session Summary

Iterated on the Phase 2 M7–M9 output after the user asked us to survey reference implementations and not waste resources on unneeded SDKs.

1. **SDK evaluation (ADR-003)** — Surveyed `OpenRouterTeam/go-sdk` (Speakeasy-generated, README marks non-production), `openai/openai-go`, `anthropics/anthropic-sdk-go`. Decided to keep stdlib providers because: (a) our surface is `Check` + `Generate` only, (b) OpenRouter is drop-in OpenAI-compatible, (c) SDKs would add ~20 transitive deps for zero new capability. Positioned `openai/codex` and `github/copilot-cli` as *harnesses* (callers of tpatch), not providers.
2. **Presets for API parity** — Added `tpatch provider set --preset copilot|openai|openrouter|anthropic|ollama` backed by a single `providerPresets` map. Refactored `autoDetectProvider` to reuse the same map so there is one source of truth. Preset composes with explicit flag overrides (e.g. `--preset anthropic --model claude-opus-4`). Invalid presets fail loudly.
3. **Harness integration guides** — Wrote `docs/harnesses/codex.md` and `docs/harnesses/copilot.md` explaining the `tpatch next --format harness-json` contract, example sessions, recommended allow-lists, and anti-patterns (do not let the harness re-implement workflow phases).
4. **Tracking cadence** — Rewrote "Context Preservation Rules" in `AGENTS.md` with an enforced cadence cheatsheet (trigger → update). Updated `CLAUDE.md` Working Rules to reference the cadence. Key directive: "A task is not complete until tracking reflects its state."

## Files Created
- `docs/adrs/ADR-003-sdk-evaluation.md` — SDK evaluation decision, matrix, rationale.
- `docs/harnesses/codex.md` — Codex CLI integration guide.
- `docs/harnesses/copilot.md` — GitHub Copilot CLI integration guide.

## Files Changed
- `internal/cli/cobra.go` — `providerPresets` map; `--preset` flag on `provider set`; auto-detect refactored to reuse presets.
- `internal/cli/phase2_test.go` — New `TestProviderSetPreset` covering openrouter/anthropic/unknown.
- `AGENTS.md` — Stronger "Context Preservation Rules" with cadence cheatsheet.
- `CLAUDE.md` — Working Rules point to cadence; explicit per-phase tracking requirement.

## Test Results
- `go test ./...` — **ALL PASS** (7 packages)
- `gofmt -l .` — **CLEAN**
- `go build -o tpatch ./cmd/tpatch` — **OK** (v0.3.0-dev)
- Manual verification:
  ```
  tpatch provider set --preset openrouter
  → type: openai-compatible, url: https://openrouter.ai/api, auth_env: OPENROUTER_API_KEY
  ```

## Key Decisions Locked In
- **No third-party provider SDKs.** Stdlib stays the provider layer.
- **`providerPresets` is the single source of truth.** Adding a new vendor = one map entry.
- **Harnesses (codex, copilot) call tpatch via CLI + JSON.** No SDK embed on either side.
- **Tracking updates are enforced per phase, not per session.**

## Blockers
None.

## Next Steps
1. Live smoke test with `codex exec` and `copilot` once an environment with both installed is available — confirm the handshake matches the guide.
2. Consider M10 (`tpatch mcp serve`) to expose the same state machine via MCP for Copilot CLI. Tracked as a follow-up only; not in the current ADR scope.
3. Supervisor review + roadmap update for this refinement pass.

## Context for Next Agent
- The preset map lives in `internal/cli/cobra.go` just below `providerSetCmd()`. Keep `--preset` and `autoDetectProvider` using the same map.
- Harness guides assume a repo-level `AGENTS.md` for codex and a `.github/copilot/cli/skills/tessera-patch/SKILL.md` for copilot-cli. Both are created by copying from the `.tpatch/steering/` outputs of `tpatch init`.
- ADR-003 explicitly lists the triggers that would cause us to reconsider adopting SDKs (streaming, non-standard schemas, official harness client libraries).
- Prior Phase 2 handoff (M7/M8/M9 initial) has been archived to `docs/handoff/HISTORY.md` under a 2026-04-17 entry.


---

## 2026-04-17 — M7 + M8 + M9 Phase 2 Implementation (v0.3.0-dev)

**Task**: Ship Phase 2 milestones (provider integration, LLM validation+retry, interactive/harness commands)
**Agent**: Phase 2 implementation agent
**Verdict**: APPROVED WITH NOTES (subsumed by 2026-04-17 refinement — see CURRENT.md)

## Session Summary

Implemented M7–M9 end-to-end:

1. **M7** — Added `AnthropicProvider` (`internal/provider/anthropic.go`) speaking the Messages API. Introduced `provider.NewFromConfig()` factory selecting by `cfg.Type`. Extended auto-detection to probe Ollama (localhost:11434), `ANTHROPIC_API_KEY`, and `OPENROUTER_API_KEY`. Added `provider set --type` flag and `provider.type` validation. Wrote `docs/adrs/ADR-002-provider-strategy.md` documenting the decision and live-probe evidence for copilot-api; Ollama/OpenRouter confirmed compatible via existing OpenAI-compat provider (no code changes required).
2. **M8** — Added `GenerateWithRetry` in `internal/workflow/retry.go` with pluggable validators. `JSONObjectValidator` strips fences and round-trips the payload; `NonEmptyValidator` guards define/explore. Each attempt logs to `artifacts/raw-<phase>-response-N.txt`. Retries reissue the prompt with a corrective suffix describing the validator error. `max_retries` added to `config.yaml` (default 2), `--no-retry` flag added to analyze/define/explore/implement, context-keyed via `workflow.WithDisableRetry` to avoid signature churn.
3. **M9** — Shipped three new commands: `cycle` (batch and `--interactive` with `--editor` and `--skip-execute` options), `test` (runs `config.test_command`, records outcome in `apply-session.json` + `artifacts/test-output.txt`), `next` (emits next action as plain text or `--format harness-json`). Registered in root, version bumped to `0.3.0-dev`. All 6 skill formats updated to include `cycle`/`test`/`next`. Parity guard extended.

## Files Created
- `internal/provider/anthropic.go` — Anthropic Messages provider + `NewFromConfig` factory
- `internal/provider/anthropic_test.go` — Anthropic + factory tests
- `internal/workflow/retry.go` — `GenerateWithRetry`, validators, context flag
- `internal/workflow/retry_test.go` — retry-path tests
- `internal/cli/phase2.go` — `cycle`, `test`, `next` commands
- `internal/cli/phase2_test.go` — integration tests for the new commands
- `docs/adrs/ADR-002-provider-strategy.md` — provider strategy decision

## Files Changed
- `internal/cli/cobra.go` — factory wiring, `--type` flag, `--no-retry` on 4 workflow commands, auto-detect extensions, config `max_retries`/`test_command` keys, version bump
- `internal/store/types.go` — `Config` gains `MaxRetries` and `TestCommand`
- `internal/store/store.go` — default config.yaml template + `SaveConfig` + `parseYAMLConfig` cover the new fields
- `internal/workflow/workflow.go` — analyze/define/explore call `GenerateWithRetry`
- `internal/workflow/implement.go` — implement calls `GenerateWithRetry`
- `assets/skills/*` + `assets/workflows/*` + `assets/prompts/*` — all 6 formats list the three new commands
- `assets/assets_test.go` — parity guard requires `cycle`, `test`, `next`
- `docs/ROADMAP.md` — M7/M8/M9 marked complete

## Test Results
- `go test ./...` — **ALL PASS** across 7 packages
- `gofmt -l .` — **CLEAN**
- `go build -o tpatch ./cmd/tpatch` — **OK** (v0.3.0-dev)
- Smoke test: `init` → `add` → `next --format harness-json` → `cycle --skip-execute` → `config set test_command echo hi` → `test` — all succeed end-to-end

## Noteworthy Details
- `Provider` interface unchanged (still `Check` + `Generate`). Adding providers is purely additive.
- Retry is disabled when no provider is configured (existing heuristic fallback untouched).
- `tpatch next` is state-aware: for `defined` features it further distinguishes "needs explore", "needs implement", or "needs apply" by probing the feature directory.
- `--no-retry` plumbing uses `context.WithValue` to avoid changing every workflow signature.
- Auto-detection order: copilot-api → Ollama → Anthropic (via env) → OpenAI (via env) → OpenRouter (via env).

## Blockers
None.

## Next Steps
1. Run live bug bash against copilot-api with retry enabled (ideally against a degraded-model path to exercise the corrective prompt).
2. Consider streaming/tool-use support as an optional capability interface when a future milestone needs it.
3. Consider harness integration guides (M9.10, M9.11) — deferred; the skill files and `tpatch next --format harness-json` already provide the contract.


---

## 2026-04-16 — M6 Live Provider Bug Bash (v0.2.0-dev, Session 4)

**Task**: Run bug bash with live copilot-api provider, add patch validation and merge strategy config  
**Agent**: Supervisor agent  
**Status**: Complete — Full pass with live LLM

**What was done**:
- Added `ValidatePatch()` to gitutil — automated patch validation on `record`
- Added `merge_strategy` config option (`3way` default, `rebase` alt) to types, store, and CLI
- Added `extractUpstreamContext()` to reconcile — reads affected files for Phase 3 prompt
- Ran complete bug bash with live copilot-api (claude-sonnet-4, 44 models)
- Live LLM analysis produced detailed, accurate results with correct file paths
- Feature A: `upstream_merged` via Phase 3 (LLM analyzed upstream model-mapping.ts)
- Feature B: `reapplied` via Phase 4 (LLM said still_needed, patch applied cleanly)

**Key finding**: Upstream context is critical for Phase 3. Without actual file contents, the LLM returns "unclear".

---

## 2026-04-16 — M6 Bug Bash + Bug Fixes (v0.2.0-dev)

**Task**: Run reconciliation bug bash, fix discovered bugs, re-test  
**Agent**: Supervisor agent (3 sessions)  
**Status**: Complete — Full pass

**What was done**:
- Session 2: Ran initial bug bash against `tesserabox/copilot-api` at commit `0ea08feb`
  - Feature A (model translation fix): Correctly detected as `upstream_merged` via Phase 3
  - Feature B (models CLI subcommand): Blocked — 3 bugs found in patch capture and CLI
  - Found BUG-1 (flag ordering), BUG-2 (corrupt patches), BUG-3 (stale recording)
- Session 3: Fixed all 3 bugs + bonus improvement
  - Migrated CLI from stdlib `flag` to `cobra` (fixes interspersed flags)
  - Rewrote `CapturePatch()` with `git add --intent-to-add` (fixes new file handling)
  - Added trailing newline to all patch output (fixes corrupt patch at EOF)
  - Added `--from` flag to `record` (captures committed diffs)
  - Added 3-way merge fallback to forward-apply (handles lockfile mismatches)
- Re-ran bug bash: Feature A → `upstream_merged`, Feature B → `reapplied`. Full pass.

**Key decisions**:
- Added cobra dependency (breaks zero-dep constraint, user-approved)
- Patches now always end with `\n`
- Forward-apply tries strict then 3-way merge fallback

---

## 2026-04-16 — M0–M5 Implementation (v0.1.0-dev)

**Task**: Build unified tpatch CLI from M0 through M5  
**Agent**: Supervisor agent (1 session)  
**Status**: Complete — All milestones approved

**What was done**:
- Built entire CLI in Go: 12 commands, ~2600 LOC source, ~850 LOC tests
- M0: Go module, CLI skeleton, Makefile
- M1: .tpatch/ data model, store layer, init/add/status/config, slug generation, path safety
- M2: OpenAI-compatible provider, analyze/define/explore with heuristic fallback
- M3: implement, apply (prepare/started/done), record, patch capture
- M4: 4-phase reconciliation engine with 4 test scenarios
- M5: 6 skill formats embedded via go:embed, parity guard test

---

## 2026-04-16 — Project Bootstrap (Governance)

**Task**: Bootstrap tpatch/ consolidation project with governance files  
**Agent**: Board review agent  
**Status**: Complete

**What was done**:
- Created SPEC.md consolidating technical decisions from all three teams
- Created CLAUDE.md for agent orientation with read-this-first table
- Created AGENTS.md defining the cyclic supervisor workflow (implementation → review → decision)
- Created ROADMAP.md with M0-M6 milestones + future M7-M11
- Created 7 milestone files with detailed task lists, acceptance criteria, and reference pointers
- Created handoff and supervisor log templates
- Created consolidation prompt for the supervisor agent

**Key decisions**:
- Go with zero dependencies (stdlib only)
- 4-phase reconciliation (reverse-apply → operation-level → provider-assisted → forward-apply)
- 6 skill formats (Claude, Copilot, Copilot Prompt, Cursor, Windsurf, Generic)
- Deterministic apply recipe with path traversal protection
- Secret-by-reference pattern for provider credentials
