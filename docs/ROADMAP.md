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

**Goal**: Pass the reconciliation bug bash end-to-end against tesserabox/copilot-api.

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

**Goal**: Make `go install github.com/tesserabox/tesserapatch/cmd/tpatch@latest` work and add a free CI workflow.

**Result**: Renamed the module path to match the repo (`github.com/tesserabox/tesserapatch`). Added `.github/workflows/ci.yml` (matrix ubuntu+macOS, `go-version-file: go.mod`, gofmt/vet/build/test/install smoke). Tagged `v0.3.0` locally; ready to push.

## Planning (2026-04-17) — Native Copilot auth research + PRD ✅

**Goal**: Plan a "native" Copilot auth provider. Confirm whether the existing `copilot-api` proxy is officially supported and whether `github/copilot-cli` is open source.

**Result**: Confirmed `copilot-api` is reverse-engineered and explicitly unsupported by GitHub; confirmed `github/copilot-cli` is closed-source (only README/install/changelog/LICENSE published). Wrote `docs/prds/PRD-native-copilot-auth.md` with a two-phase recommendation (M10 managed proxy, M11 opt-in native PAT provider). Shelling out to the `copilot` CLI is explicitly rejected — it burns premium requests and re-runs its own agent loop. M11 is soft-blocked on a ToS question (can tpatch send editor headers against `api.githubcopilot.com`?).

## M10 — Managed Copilot Proxy (planned, not started)

**Goal**: Wrap `copilot-api` lifecycle so a user can `tpatch provider copilot-start/stop/status` without juggling a separate terminal.

**Scope**: Install-check, background `copilot-api start` supervision, PID/port tracked in `.tpatch/provider-runtime.json` (gitignored), auto-start hook on `tpatch analyze` when preset is `copilot` and proxy is down, prominent abuse-detection warning on first run. No change to the `Provider` interface.

## M11 — Native Copilot Provider (opt-in, soft-blocked)

**Goal**: First-party Go provider speaking directly to `api.githubcopilot.com` after an OAuth device flow — port of anomalyco/opencode's proven pattern. Removes the Node/Bun dependency.

**Blueprint**: opencode's `packages/opencode/src/plugin/github-copilot/copilot.ts` — OAuth device flow with editor client ID `Ov23li8tweQw6odWQebz`, Bearer token used directly against `api.githubcopilot.com` (no session-token exchange), `x-initiator: agent` header. ~200 LOC of Go.

**Gate**: Requires `provider.copilot_native_optin: true` in config + acceptance of an abuse-detection warning. Blocked pending a legal review of editor-header usage (see PRD open question 1).

## M12+ — Future

- Cost tracking and token budgeting
- Multi-repo orchestration
- Web dashboard

