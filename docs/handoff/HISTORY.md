# Handoff History

*Completed handoff entries are archived here in reverse chronological order.*

---

## 2026-04-17 — Distribution Setup (module rename + CI workflow) (v0.3.0)

**Task**: Enable 'go install' + add free CI workflow
**Agent**: Distribution agent
**Verdict**: APPROVED — committed as dc42718 + 305781d, tagged v0.3.0

## Session Summary

Two operational follow-ups:

1. **Module path fixed to match repo** — `go.mod` said `github.com/tesserabox/tpatch` while the GitHub repo is `tesserabox/tesserapatch`. That mismatch blocks `go install`. Renamed the module and all imports to `github.com/tesserabox/tesserapatch` (user-selected option). The binary is still called `tpatch` because Go names installed binaries after the final path segment (`cmd/tpatch`).
2. **CI workflow added** — `.github/workflows/ci.yml` runs on push and PR to `main`. It sets up Go via `go-version-file: go.mod` (so CI tracks local dev), checks formatting with `gofmt`, runs `go vet`, builds, tests, and runs an install smoke test. Matrix on `ubuntu-latest` + `macos-latest`. Concurrency group cancels superseded runs to save minutes. Free for public repos.
3. **README install block updated** — now points to the correct module path.

## Files Changed
- `go.mod` — `module github.com/tesserabox/tesserapatch`.
- All `.go` files under `cmd/`, `internal/`, `assets/` — import paths rewritten.
- `.github/workflows/ci.yml` — new CI workflow.
- `README.md` — install instructions updated.

## Test Results
- `gofmt -l .` — clean
- `go test ./... -count=1` — **ALL PASS** across 7 packages
- `go build -o tpatch ./cmd/tpatch` — OK
- `./tpatch --version` → `tpatch 0.3.0-dev`

## Post-Merge Checklist (for the repo owner)
1. Make the repo public (required for `go install` without auth and for free unlimited Actions minutes).
2. Push to `main`; CI should pass on both ubuntu + macOS.
3. Tag a release: `git tag v0.3.0 && git push origin v0.3.0`. `go install ...@latest` will then resolve to that tag.
4. Verify from a clean machine: `go install github.com/tesserabox/tesserapatch/cmd/tpatch@latest`.

## Provider Preset Clarification
`tpatch provider set --preset copilot` targets `http://localhost:4141` with `auth_env: GITHUB_TOKEN`. That is the **copilot-api proxy** endpoint, not the Copilot CLI auth itself. To use the same Copilot subscription as `copilot-cli`:

- Install and run `copilot-api` locally (it does the GitHub OAuth and exposes an OpenAI-compatible endpoint on 4141).
- Then `tpatch provider set --preset copilot` just works.

There is no direct-to-GitHub-Copilot path today because GitHub has not published a public OpenAI-compatible Copilot endpoint. If that changes, we add another preset.

## Blockers
None.

## Next Steps
1. Push + make repo public + tag v0.3.0.
2. Confirm CI green on first main push.
3. Optional: add a `release.yml` workflow with goreleaser for prebuilt binaries (not required for `go install`).


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
# Current Handoff

## Active Task
- **Task ID**: ADR-004 (M10 proxy UX) + ADR-005 (M11 native provider)
- **Milestone**: Planning locked-in for M10 and M11
- **Description**: User chose interactively through open questions; decisions captured as two ADRs. PRD updated to match the session-token-exchange direction (copilot-api/litellm pattern) instead of opencode's simpler path.
- **Status**: ADRs written, awaiting supervisor review
- **Assigned**: 2026-04-17

## Session Summary

1. **Committed Phase 2 work** as commit `dc42718` ("Phase 2 (v0.3.0): providers, validation, interactive/harness, distribution"). Includes all M7/M8/M9, refinement, and distribution changes.
2. **Released v0.3.0** — bumped version constant from `0.3.0-dev` to `0.3.0`, committed as `305781d`, tagged `v0.3.0` with a full release note. Tag is local; repo owner still needs to `git push origin main --tags`.
3. **Researched Copilot auth options**:
   - Pulled `tesserabox/copilot-api` README — explicitly "reverse-engineered proxy… not supported by GitHub… may trigger abuse-detection systems."
   - Pulled `github/copilot-cli` README and repo root listing — **not open source** (only README, install.sh, changelog, LICENSE published; the CLI is a closed-source binary on Homebrew/npm/WinGet). Official auth paths: `/login` OAuth or `GH_TOKEN`/`GITHUB_TOKEN` with "Copilot Requests" PAT permission.
   - Conclusion: **GitHub does not publish a public OpenAI-compatible Copilot endpoint.** Every third-party integration (copilot-api, Claude Code via proxy, tpatch) is on reverse-engineered surface.
4. **Wrote PRD** (`docs/prds/PRD-native-copilot-auth.md`) with 5 options evaluated and a two-phase recommendation: M10 managed-proxy UX (`copilot-start` / `copilot-stop` / `copilot-status`), then M11 opt-in native PAT provider calling `api.githubcopilot.com` directly. Shelling out to `copilot` CLI explicitly rejected (burns premium requests, re-runs its own agent loop).

## Files Created
- `docs/prds/PRD-native-copilot-auth.md`

## Files Changed
- `internal/cli/cobra.go` — version `0.3.0-dev` → `0.3.0` (committed)

## Git State
- `dc42718` — Phase 2 feature commit
- `305781d` — "Release v0.3.0" (version bump)
- `v0.3.0` — tag on 305781d
- **Not yet pushed.** Repo owner needs `git push origin main && git push origin v0.3.0`.

## Test Results
- `gofmt -l .` clean
- `go test ./...` — all 7 packages pass
- `tpatch --version` → `tpatch 0.3.0`

## Key Decisions (captured in ADR-004 and ADR-005)

**M10 — copilot-api UX (ADR-004)**
- No process supervision; we warn when unreachable, point at install instructions.
- Upstream `ericc-ch/copilot-api` is the recommended proxy; internal TODO to revisit the tesserabox fork if its fixes become blocking.
- New global config at `~/.config/tpatch/config.yaml`; per-repo `.tpatch/config.yaml` overrides.
- Reachability probe on first call (`GET /v1/models`, 2s timeout); warn-but-continue on `init`, hard-fail on workflow commands.
- First-run AUP warning stored in global config; no log piping; Windows deferred.

**M11 — native Copilot provider (ADR-005)**
- **Changed direction**: port ericc-ch/copilot-api's internal flow (session-token exchange via `copilot_internal/v2/token` + VS Code Copilot Chat client ID `Iv1.b507a08c87ecfe98`) rather than opencode's simpler Bearer-the-OAuth-token path. copilot-api and litellm both use this flow → proven, field-exposed surface that matches what Copilot's own editor plugins do.
- Token storage: `$XDG_DATA_HOME/tpatch/copilot-auth.json`, chmod 0600. OS keychain deferred.
- OAuth token treated as long-lived; 401 triggers one retry then "run copilot-login again".
- Device-flow prompts for GitHub.com vs Enterprise; Enterprise domain captured at login.
- `GET /models` every session, no persistent cache.
- Editor headers overridable via `provider.headers_override`; `x-initiator` opt-in, unset by default.
- `type: copilot-native` distinct from `type: openai-compatible` + copilot proxy.
- Opt-in gate with AUP acknowledgement in global config.

## Blockers
- None for the PRD itself.
- M11 (native provider) is soft-blocked on the "can we ship the editor header set?" legal question noted in the PRD.

## Next Steps
1. **Repo owner**: decide whether to create a GitHub Release for v0.3.0 (or add `softprops/action-gh-release@v2` to CI for automation on future tags).
2. **Before M11 implementation begins**: answer the two open questions in the PRD and ADR-005 (legal/ToS on editor headers; GitHub roadmap for an official endpoint).
3. **Next agent session — M10 implementation** per ADR-004: add global-config loader, reachability probe in provider-set/init flow, first-run AUP warning helper.
4. **After M10 lands — M11 implementation** per ADR-005, gated on the open questions.

## Context for Next Agent
- PRD lives at `docs/prds/PRD-native-copilot-auth.md`. It includes the full options matrix and the rejection rationale for each alternative.
- The `Provider` interface is stable and Phase 1 does not need to touch it at all — the managed proxy still routes through the existing `OpenAICompatible` code path. Phase 2 adds a sibling struct.
- `docs/harnesses/copilot.md` already documents the current manual setup; update it when M10 lands.
- GitHub has explicitly warned users in copilot-api's README about abuse-detection. Our UX for M10/M11 must surface that warning prominently.



---


---

# Archived — 2026-04-17T08:26:19Z

# Current Handoff

## Active Task
- **Task ID**: M10 — Managed Copilot proxy UX (ADR-004)
- **Milestone**: M10 delivered
- **Description**: Honest UX for the reverse-engineered `copilot-api` proxy — global config, reachability probe, first-run AUP warning, install pointers, CI release automation.
- **Status**: Implemented; awaiting supervisor review.
- **Assigned**: 2026-04-17

## Session Summary

1. **CI release automation** — added a `release` job to `.github/workflows/ci.yml` that triggers on `v*` tag pushes, creates a GitHub Release via `softprops/action-gh-release@v2`, auto-generates release notes, and marks tags containing `-` as prereleases. Uses the default `GITHUB_TOKEN` with `contents: write`. Cost: free.
2. **Global config** — new `internal/store/global.go` adds `GlobalConfigPath()`, `LoadGlobalConfig`, `SaveGlobalConfig`, `(s *Store).LoadMergedConfig`, `AcknowledgeCopilotAUP`, `CopilotAUPAcknowledged`, `mergeConfig`, `renderGlobalYAML`. Honors `XDG_CONFIG_HOME`, falls back to `os.UserConfigDir()` (macOS caveat documented in the harness doc). Chmod 0600 on write.
3. **Config precedence** — repo `.tpatch/config.yaml` overrides the global config field-by-field; zero values do **not** clear globals (must set the field explicitly). AUP ack is global-only.
4. **Types** — `Config.CopilotAUPAckAt string` added to `internal/store/types.go`.
5. **Reachability probe** — new `internal/provider/probe.go` with `Reachable(ctx, cfg)` (2s timeout), `IsLocalEndpoint(cfg)`, `IsCopilotProxyEndpoint(cfg)` helpers. Probes via existing `Check()`.
6. **CLI wiring** — new `internal/cli/copilot.go` with `copilotInstallHint`, `copilotAUPWarning`, `maybeShowAUPWarning`, `ensureProviderReachable`, `warnIfUnreachable`, `providerConfigFromStore`. Wired into `init` (warn-continue + AUP) and `providerSetCmd` + `autoDetectProvider` (AUP on first Copilot selection).
7. **Workflow hard-fail** — `loadAndProbeProvider(ctx, s)` replaces `loadProviderFromStore` in analyze/define/explore/implement/cycle. Probes once per process (cached per base URL). Local-endpoint-only; opt-out via `TPATCH_NO_PROBE=1`. Non-local endpoints skip the probe to avoid penalising custom remote configs.
8. **Execute now surfaces errors** — `Execute()` prints `error: %v` to stderr before returning exit code 1 so probe failures are visible. Preserves existing `SilenceErrors: true` cobra behaviour for graceful formatting.
9. **Harness doc refresh** — `docs/harnesses/copilot.md` now documents the install path, OS-dependent global config path (macOS caveat), warn-vs-fail behaviour, and links to ADR-004/005.
10. **Tests** — 6 new tests in `internal/store/global_test.go` (roundtrip, missing file, ack idempotency, precedence, merge-no-clear, save creates dir) and 5 in `internal/provider/probe_test.go` (httptest OK, TEST-NET-1 timeout, not-configured, URL matcher, cancelled ctx). All 7 packages pass.

## Files Created
- `.github/workflows/ci.yml` — amended (release job)
- `internal/store/global.go`
- `internal/store/global_test.go`
- `internal/provider/probe.go`
- `internal/provider/probe_test.go`
- `internal/cli/copilot.go`

## Files Changed
- `internal/cli/cobra.go` — `loadAndProbeProvider`, `Execute` prints errors, AUP wiring in `init` / `providerSetCmd` / `autoDetectProvider`, `sync` import.
- `internal/store/types.go` — `CopilotAUPAckAt` field.
- `docs/harnesses/copilot.md` — M10 section.

## Test Results
- `gofmt -w .` clean
- `go vet ./...` clean
- `go test ./... -count=1` — 7/7 packages pass
- `go build ./cmd/tpatch` OK
- Smoke: `init` + `provider set --preset copilot` prints AUP warning exactly once; second run is quiet; `analyze` against a dead localhost port hard-fails with an install hint; against a live copilot-api proxy falls through to the workflow.

## Key Behaviours

- **Warn vs fail**: `init` and `provider set` are warn-continue (a user may be bootstrapping before starting the proxy). Workflow commands that actually call the LLM (`analyze|define|explore|implement|cycle`) hard-fail when the local endpoint is unreachable.
- **Probe scope**: only runs for local endpoints (`localhost`, `127.0.0.1`, `[::1]`). Remote endpoints are trusted.
- **AUP once**: the AUP warning fires only when the new config actually points at the copilot-api proxy (`openai-compatible` + port 4141) and the user has not acknowledged before.
- **TODO**: `copilotInstallHint` carries an inline `TODO(adr-004)` comment to revisit the tesserabox fork recommendation if its divergent fixes become blocking.

## Blockers
- None for M10.
- M11 still soft-blocked on the two open questions in ADR-005 (editor-headers legal/ToS, official endpoint roadmap). User direction: proceed with editor headers, monitor; so these are effectively closed as "accept risk".

## Next Steps
1. Supervisor review of M10 implementation.
2. Commit as `feat(m10): managed copilot-api proxy UX (ADR-004)` and push.
3. Consider tagging `v0.3.1` once review lands — CI will produce the GitHub Release automatically.
4. Start M11 implementation per ADR-005 (native Copilot provider with session-token exchange) once M10 is merged.

## Context for Next Agent
- Global config on macOS defaults to `~/Library/Application Support/tpatch/config.yaml` unless `XDG_CONFIG_HOME` is set. Every test that touches global state sets `XDG_CONFIG_HOME` to a tempdir; follow this pattern.
- `TPATCH_NO_PROBE=1` disables the workflow hard-fail probe (useful for offline demos or CI steps that only read store state). Add it to future tests that should not hit the network.
- The probe cache is a process-level `map[string]error` guarded by a mutex — fine for the CLI's one-shot lifecycle but intentionally not time-bound, so long-running processes would need to invalidate it. Not a concern today.
- `Execute()` now prints errors. Tests that exercise `rootCmd.Execute()` directly still use the cobra `SetErr` buffer; only the top-level wrapper prints to stderr.
- The AUP warning text lives in `internal/cli/copilot.go::copilotAUPWarning`. Tweak there, not in harness docs.
