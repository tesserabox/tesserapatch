# Tessera Patch — Unified Implementation Roadmap

## Legend

| Symbol | Meaning |
|--------|---------|
| ⬜ | Not started |
| 🔨 | In progress |
| ✅ | Complete |
| 🚫 | Blocked |

---

## M0 — Bootstrap ✅

**Goal**: Go module, CLI skeleton, build pipeline.

See `docs/milestones/M0-bootstrap.md` for task list.

## M1 — Core Store & Init ✅

**Goal**: `.tpatch/` data model, `init`, `feature add`, `status`, `config`.

See `docs/milestones/M1-core-store.md` for task list.

## M2 — Provider & Analysis ✅

**Goal**: Provider abstraction, `provider check`, `analyze`, `define`, `explore` with heuristic fallback.

See `docs/milestones/M2-provider-analysis.md` for task list.

## M3 — Apply & Record ✅

**Goal**: Deterministic apply recipe, `implement`, `apply`, `record`, patch capture (tracked + untracked).

See `docs/milestones/M3-apply-record.md` for task list.

## M4 — Reconciliation ✅

**Goal**: 4-phase reconciliation (`reconcile`), `upstream.lock`, provider-assisted semantic detection.

See `docs/milestones/M4-reconciliation.md` for task list.

## M5 — Skill System ✅

**Goal**: 6 harness formats embedded, CLI-driven installation, parity guard test.

See `docs/milestones/M5-skill-system.md` for task list.

## M6 — Bug Bash Validation ✅

**Goal**: Pass the reconciliation bug bash end-to-end against tesseracode/copilot-api.

**Result**: Full pass. Feature A → upstream_merged (Phase 3), Feature B → reapplied (Phase 4 with 3-way merge). All 26 tests pass, typecheck clean. See `../tests/tpatch/BUG-BASH-REPORT.md`.

See `docs/milestones/M6-bug-bash.md` for task list.

---

## Future Milestones (Post-MVP)

## M7 — Provider Investigation & Integration ✅

**Goal**: Evaluate Ollama, OpenRouter, and Anthropic as provider options. Implement the best candidate.

**Result**: Anthropic Messages API adapter added alongside the existing OpenAI-compatible provider. Ollama and OpenRouter confirmed to work with the existing provider. Auto-detection extended. See `docs/adrs/ADR-002-provider-strategy.md`.

See `docs/milestones/M7-provider-investigation.md` for task list.

## M8 — LLM Output Validation & Retry ✅

**Goal**: Structured validation of LLM responses, retry with corrective feedback, quality metrics.

**Result**: `GenerateWithRetry` helper with per-phase validators (JSON for analyze/implement, non-empty for define/explore). Raw responses logged to `artifacts/raw-<phase>-response-N.txt`. Config key `max_retries` + `--no-retry` CLI flag.

See `docs/milestones/M8-llm-validation.md` for task list.

## M9 — Interactive Mode & Harness Integration ✅

**Goal**: `tpatch cycle --interactive` for human-driven flow + `tpatch next` protocol for harness-backed (Claude Code, Copilot CLI, OpenCode) integration.

**Result**: `cycle`, `test`, `next` commands shipped. `tpatch next --format harness-json` emits structured tasks (phase, instructions, context_files, on_complete). `tpatch test <slug>` runs the configured `test_command` and records validation status. All 6 skill formats updated, parity guard extended. Harness integration guides for codex (`docs/harnesses/codex.md`) and Copilot CLI (`docs/harnesses/copilot.md`) written.

See `docs/milestones/M9-interactive-harness.md` for task list.

## Refinement (2026-04-17) — SDK evaluation + presets + tracking cadence ✅

**Goal**: Evaluate OpenRouter/OpenAI/Anthropic Go SDKs and codex/copilot-cli harnesses; adopt the simplest integration with proven parity; strengthen tracking cadence.

**Result**: No third-party provider SDKs adopted (stdlib suffices for our narrow `Check`+`Generate` surface). Added `tpatch provider set --preset` for one-line vendor switching. Wrote harness integration guides for codex and Copilot CLI. Rewrote AGENTS.md context-preservation rules with a per-trigger cadence cheatsheet. See `docs/adrs/ADR-003-sdk-evaluation.md`.

## Distribution (2026-04-17) — `go install` + CI workflow + v0.3.0 release ✅

**Goal**: Make `go install github.com/tesseracode/tesserapatch/cmd/tpatch@latest` work and add a free CI workflow.

**Result**: Renamed the module path to match the repo (`github.com/tesseracode/tesserapatch`). Added `.github/workflows/ci.yml` (matrix ubuntu+macOS, `go-version-file: go.mod`, gofmt/vet/build/test/install smoke). Tagged `v0.3.0` locally; ready to push.

## Planning (2026-04-17) — Native Copilot auth research + PRD ✅

**Goal**: Plan a "native" Copilot auth provider. Confirm whether the existing `copilot-api` proxy is officially supported and whether `github/copilot-cli` is open source.

**Result**: Confirmed `copilot-api` is reverse-engineered and explicitly unsupported by GitHub; confirmed `github/copilot-cli` is closed-source (only README/install/changelog/LICENSE published). Wrote `docs/prds/PRD-native-copilot-auth.md` with a two-phase recommendation (M10 managed proxy, M11 opt-in native PAT provider). Shelling out to the `copilot` CLI is explicitly rejected — it burns premium requests and re-runs its own agent loop. M11 is soft-blocked on a ToS question (can tpatch send editor headers against `api.githubcopilot.com`?).

## M10 — Managed Copilot Proxy UX ✅ (delivered 2026-04-17, pending review)

**Goal**: One-command access to GitHub Copilot via the `ericc-ch/copilot-api` proxy, without tpatch taking on process-supervision responsibilities. **See ADR-004 for the locked-in decisions.**

**Scope**: Global config file at `~/.config/tpatch/config.yaml` (XDG-honouring; macOS defaults to `~/Library/Application Support/tpatch/config.yaml`), reachability probe (`GET /v1/models`, 2s timeout), warn-but-continue on `init`/`provider set`, hard-fail on workflow commands (`analyze|define|explore|implement|cycle`), first-run AUP warning persisted once per user, no log piping, Windows deferred.

**Delivered**:
- `internal/store/global.go` + `types.go::CopilotAUPAckAt`
- `internal/provider/probe.go` (`Reachable`, `IsLocalEndpoint`, `IsCopilotProxyEndpoint`)
- `internal/cli/copilot.go` + `cobra.go::loadAndProbeProvider`
- CI release automation in `.github/workflows/ci.yml` (tag-triggered GitHub Release via `softprops/action-gh-release@v2`, free)
- Tests in `internal/store/global_test.go` (6) and `internal/provider/probe_test.go` (5)
- `docs/harnesses/copilot.md` refresh

**Opt-out**: `TPATCH_NO_PROBE=1` for offline/CI steps.

## M11 — Native Copilot Provider (opt-in) ✅ (delivered, pending review)

**Goal**: First-party Go provider speaking directly to `api.githubcopilot.com` — port of the copilot-api/litellm pattern (session-token exchange via `copilot_internal/v2/token`). Removes the Node/Bun dependency. **See ADR-005 for the locked-in decisions.**

**Blueprint**: ericc-ch/copilot-api's `src/lib/api-config.ts` + `src/services/github/` — client ID `Iv1.b507a08c87ecfe98`, VS Code Copilot Chat editor headers, session-token refresh on ~25-min cadence. ~350–400 LOC of Go.

**Gate**: Requires `provider.copilot_native_optin: true` in global config + acceptance of AUP warning. Editor-header policy ships as-is; will switch to an official compatibility endpoint once GitHub documents one.

**Delivered**:
- `internal/provider/copilot_{auth,login,headers,native}.go` — device-code flow, session-token exchange, on-disk auth store (0600, atomic write, symlink-reject), editor headers matching copilot-api 0.26.7, 401-retry-once semantics.
- CLI: `tpatch provider copilot-login`, `copilot-logout`, `--preset copilot-native`, opt-in gate enforced in `provider set` + `config set`.
- `docs/faq.md` (macOS config-path note, auth-file locations).
- Harness doc updated with "Native path (experimental)" section.

**Opt-in**: `tpatch config set provider.copilot_native_optin true` → `tpatch provider copilot-login` → `tpatch provider set --preset copilot-native`.

## M12 — Provider-Assisted Conflict Resolver 🔨 (Tranche B2, v0.5.0)

**Goal**: Phase 3.5 of reconcile — when 3-way merge fails, provider resolves each conflicted file in a shadow `git worktree`. Validation gates output; accept/reject is atomic.

**Scope doc**: `docs/milestones/M12-provider-conflict-resolver.md` (PRD: `docs/prds/PRD-provider-conflict-resolver.md`, ADR: `docs/adrs/ADR-010-provider-conflict-resolver.md`)

**Ships**: shadow worktree plumbing, per-file sequential resolver, Go-in-tree + configurable syntax validation, `reconciling-shadow` state, 6-skill update, ≥5 golden scenarios.

**Out of scope** (v0.5.x follow-ups): parallel calls, chunked context for huge files, opt-in heuristic fallback, auto spec-drift refresh.

## M13+ — Future

- Cost tracking and token budgeting
- Multi-repo orchestration
- Web dashboard

