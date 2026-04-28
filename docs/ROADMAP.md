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

## M12 — Provider-Assisted Conflict Resolver ✅ (Tranche B2, v0.5.0)

**Goal**: Phase 3.5 of reconcile — when 3-way merge fails, provider resolves each conflicted file in a shadow `git worktree`. Validation gates output; accept/reject is atomic.

**Scope doc**: `docs/milestones/M12-provider-conflict-resolver.md` (PRD: `docs/prds/PRD-provider-conflict-resolver.md`, ADR: `docs/adrs/ADR-010-provider-conflict-resolver.md`)

**Ships**: shadow worktree plumbing, per-file sequential resolver, Go-in-tree + configurable syntax validation, `reconciling-shadow` state, 6-skill update, ≥5 golden scenarios.

**Out of scope** (v0.5.x follow-ups): parallel calls, chunked context for huge files, opt-in heuristic fallback, auto spec-drift refresh.

## M13 — UX Polish & Quick Wins (Tranche C1, v0.5.1) ✅

## M13.5 — Correctness Fix Pass (Tranche C2, v0.5.2) ✅

Six confirmed findings from the v0.4.3..v0.5.1 delta review, shipped as a focused correctness-only release before starting M14.

- c2-resolve-apply-truthful — silent correctness bug on `reconcile --resolve --apply` (shadow → real tree copy was never happening for auto-apply). Fix: shared `workflow.AcceptShadow` helper used by both manual and auto paths.
- c2-refresh-index-clean — `DiffFromCommitForPaths` no longer leaks intent-to-add entries (uses `GIT_INDEX_FILE` temp index).
- c2-recipe-hash-provenance — stale guard now detects recipe content drift (sha256), not only HEAD drift. Legacy sidecars still accepted.
- c2-remove-piped-stdin — piped stdin auto-confirms remove, matching shipped v0.5.1 contract.
- c2-amend-append-flag — new `amend --append`; replace stays default; mutex with `--reset`.
- c2-max-conflicts-drift — 8 doc sites corrected to match runtime default of 10.

8 regression tests added. Code-review verdict: APPROVED. See `docs/supervisor/LOG.md`.

## M13.6 — Shadow Accept Accounting Fixes (Tranche C3, v0.5.3) ✅

Three external-reviewer findings on the v0.5.2 shadow-accept flow, shipped as a focused correctness release before M14.1.

- c3-separate-resolution-artifact — dual-writer schema collision on `reconcile-session.json` split into `resolution-session.json` (resolver-owned) + `reconcile-session.json` (reconcile summary). Manual `reconcile --accept` works again post-shadow-awaiting.
- c3-accept-stamps-reconcile-outcome — `AcceptShadow` now stamps `Reconcile.Outcome=reapplied`. Unblocks ADR-011 D6 label composition.
- c3-manual-accept-regression-test — `TestGoldenReconcile_ManualAcceptFlow` end-to-end guard.

Code-review verdict: APPROVED. See `docs/supervisor/LOG.md`.

**Goal**: Low-risk, high-daily-use-impact improvements. 8 items: apply default mode, stdin add, progress spinner, editor integration, feature amend/remove, recipe stale guard, record lenient mode.

**Scope**: Inline — no separate milestone file for polish tranches.

## M14 — Feature Dependencies / DAG (Tranche D, v0.6.0) ✅

**Goal**: Stacked / dependent features with hard vs soft semantics, topological reconcile, composable derived labels (`waiting-on-parent` + `blocked-by-parent`), cascade-remove, and amend-invalidation tracking.

**PRD**: `docs/prds/PRD-feature-dependencies.md` (commit `fa4bbb6`) — APPROVED WITH NOTES after 3 review cycles.

**ADR**: `docs/adrs/ADR-011-feature-dependencies.md` — **REQUIRED before M14.1 coding starts**. Locks 8 architectural decisions (storage in status.json, DFS cycle detection, Kahn traversal, labels-not-states, created_by hard-only, upstream_merged satisfies deps, cascade on remove, no parent-patch injection to M12 resolver in v0.6).

**Gated by**: `features.dependencies: true` config flag (default false) until M14.4 lands. Single atomic v0.6.0 flip — no half-shipped intermediate releases.

**Scope (4 sub-milestones, ~1350 LOC total)**:
- **M14.1** — data model + validation (~300 LOC). Dependency struct, cycle DFS, 5 validation rules. ✅ APPROVED (commits `02f1ba9`, `d166281`, `7dd5941`).
- **M14.2** — apply gate + `created_by` recipe op + 6-skill parity-guard rollout (~250 LOC). Coordinated atomic change to recipe schema + all 6 skill formats + `docs/agent-as-provider.md`. ✅ APPROVED (commits `24baf92`, `9a5f2f3`, `4dfe0f1`, `cdd5484`). **Note**: the apply-time `created_by` integrity gate was not wired in M14.2 — closed in the M14 correctness pass below.
- **M14.3** — reconcile topological traversal + composable labels + compound verdict (~500 LOC). Kahn planner, label composition matrix, `blocked-by-parent-and-needs-resolution` compound verdict, M12 interaction. ✅ APPROVED (commits `7c9aee4`, `bccf5e2`, `b9efd07`, `a232a7b`, `4e39384`).
- **M14 correctness pass** — three external-reviewer findings before the M14.4 cutover. F1 (HIGH, was cutover-blocking): wire `created_by` apply-time gate via new `ErrPathCreatedByParent` sentinel, closing the M14.2 gap. F2: clear stale-parent-applied label after a clean reconcile (label/AttemptedAt consistency). F3: suppress parent-derived labels when child outcome is `ReconcileUpstreamed`. ✅ APPROVED (commits `cbe2873`, `071c5ed`, `cc95cbb`, `1e0d064`).
- **C5 fix-pass** — re-review caught two real gaps in the correctness pass. F1 (HIGH): F3 only fired on already-persisted upstreamed status, not the in-flight reconcile-time persistence path; fixed by short-circuiting label composition in `saveReconcileArtifacts` on `result.Outcome ∈ childRetiredOutcomes`. F2 (MEDIUM): PRD §4.3 contract drift — dry-run was erroring on hard-parent `created_by` misses; now downgrades to a warning while apply still errors. ✅ APPROVED (commits `c84c7a6`, `dd72c2c`, `ea94fb7`).
- **M14.4** — user-facing cutover (~1100 LOC across 7 chunks A–G). Chunk A: `tpatch status --dag` (ASCII + `--json`, scoped + full, cycle-safe). Chunk B: `features_dependencies` default flipped to true. Chunk C: `tpatch feature deps` verb tree + `amend --depends-on/--remove-depends-on` + `remove --cascade` (with the `--force ≠ bypass` rule per PRD §3.7 / ADR-011 D7). Chunk D: status-time `ValidateAllFeatures` warnings inline. Chunk E: 6-skill rollout with `created_by` reframed as live apply-time gate. Chunk F: `docs/dependencies.md` user reference. Chunk G: v0.6.0 release (version bump, CHANGELOG, ROADMAP). Tag is supervisor's closeout action. ✅ implementation complete (commits `d1aca5f`, `ca23b35`, `5d5f594`, `97a994f`, `e0a7d47`, plus this release commit).

**Out of scope** (v0.6.x follow-ups): auto-rebase on parent drift (`feat-feature-autorebase`), parent-patch context for resolver (`feat-resolver-dag-context`), per-dep version ranges (`feat-patch-compatibility`), stacked-PR delivery (`feat-delivery-modes`).

## M15 Wave 3 — Verify freshness overlay (v0.6.2 prep)

Successor to the superseded `feat-feature-tested-state` design (ADR-012).
Replaced the tested-as-state model with a Git-like freshness overlay per
ADR-013 / PRD-verify-freshness.md after external review surfaced
lifecycle/freshness conflation. Lifecycle and verification stay separate.

- **Slice A** — `tpatch verify <slug>` cobra shell + V0-V2 real + V3-V9
  stubs + minimal `Verify` sub-record + minimal EXPERIMENTAL skill
  stubs. ✅ APPROVED WITH NOTES (commits `8e2aabe`-`bce2252`,
  ~2026-04-27).
- **Slice B** — `ReconcileLabel` extension + `composeLabelsFromStatus`
  freshness derivation + `tpatch status` / `--dag` / `--json`
  rendering + `tpatch amend` invalidation + truth-table tests.
  ✅ APPROVED (commits `a07acc7` original + `53a4d9a` revision-1
  fixing amend invalidation dead-branch reproduced live by external
  supervisor, ~2026-04-28).
- **Slice C** — V3-V9 real implementations including hard-parent
  topological closure replay (V7/V8). ⬜
- **Slice D** — `tpatch verify --all` + 6-skill rollout + parity-guard
  anchors + `docs/dependencies.md` cross-link + CHANGELOG v0.6.2.
  ⬜

## M15+ — Future

- Cost tracking and token budgeting
- Multi-repo orchestration
- Web dashboard
- Recipe modernization (`feat-recipe-schema-expansion`, `feat-record-autogen-recipe`)
- Parallel feature workflows

