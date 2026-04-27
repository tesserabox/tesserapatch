# Current Handoff

## Active Task

- **Task ID**: (none — M15.1 closed)
- **Milestone**: v0.6.x stabilization (post-Tranche-D, post-M15.1)
- **Status**: Idle — awaiting next polish item or v0.6.1 cut decision
- **Assigned**: 2026-04-26

## Session Summary

**M15.1 shipped** — `created_by` auto-inference at implement time. Advisory only (recipe never mutated), hard-parents only, opt-out via `--no-created-by-infer`, flag-off byte-identity preserved. Reviewer APPROVED on commit `6407b6b` with no findings.

This closes the v0.6.0 user-experience loop: instead of a hard `ErrPathCreatedByParent` at apply time, users now get a stderr suggestion at implement time pointing at the likely parent.

**Not yet shipped as a release.** Version still `0.6.0`; no CHANGELOG entry yet. M15.1 is mid-cycle polish — supervisor decides whether to batch with the next stabilization item(s) into a `v0.6.1` cut, or tag now.

## Current State

- HEAD is `6407b6b` on `origin/main` (after handoff archive).
- Build clean, full test suite green, parity guard holds.
- ROADMAP M14 ✅ (Tranche D); no M15 box flipped (M15 is treated as a backlog stream, not a tranche).
- No active in-flight work.

## Files Changed

This handoff transition only:
- `docs/handoff/CURRENT.md` (this file — reset to idle)
- `docs/handoff/HISTORY.md` (M15.1 archive prepended)

## Test Results

Last green validation gate: M15.1 reviewer pass (see top entry of `docs/supervisor/LOG.md`).

## Next Steps — recommended sequencing

While v0.6.0 + M15.1 stabilize in the field, candidate next polish items (in priority order):

1. **`feat-satisfied-by-reachability`** — `git merge-base --is-ancestor` check on `satisfied_by` SHAs. Closes M14.1 deliberate limitation. ~50–80 LOC + tests. Low risk, additive validation.
2. **`chore-gitignore-tpatch-binary`** — one-liner. Kills the recurring `tpatch` root-binary slip that has bitten the entire M14 series. Trivial.
3. **Audit M14 follow-ups in SQL** — `feat-amend-dependent-warning`, `feat-feature-removal`, `m11-auth-store` may already be fully or partially shipped and just need SQL bookkeeping.
4. **`v0.6.1` cut decision** — once 1+ polish items land, decide whether to batch and tag. CHANGELOG section would summarize: M15.1 inference + whatever else lands.

Backlog stream view: 51 pending todos in SQL after M15.1 (`feat-created-by-inference` is now `done`). See `docs/supervisor/LOG.md` for review history.

## Blockers

None.

## Context for Next Agent

- **`tpatch` binary at the repo root is NOT gitignored.** Bare `tpatch` ignore would shadow `cmd/tpatch/`. Always `rm -f tpatch` after `go build ./cmd/tpatch` BEFORE staging.
- **Commit trailer mandatory**: `Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>`. Use `git -c commit.gpgsign=false`.
- **Source-truth guard (ADR-011 D6)**: any DAG/label/status code reads `status.Reconcile.Outcome` via `store.LoadFeatureStatus`, NEVER `artifacts/reconcile-session.json`.
- **`--force` is NOT a DAG-integrity bypass** (PRD §3.7, ADR-011 D7). Only `--cascade` opts into removing a feature with downstream dependents.
- **Skill parity guard** (`assets/assets_test.go`) enforces required CLI-command anchors and the recipe-op JSON schema. Treat as a real reviewer.
- **`git push` is slow** (60+ seconds typical).
- **M15.1 architectural choice**: inference errors degrade to warnings (don't block recipe persistence). Apply-time gate is the safety net. Mirror this pattern for future advisory features.
- **`internal/workflow/created_by_gate.go`** = apply-time gate (M14.2 + correctness pass + C5). **`internal/workflow/created_by_inference.go`** = implement-time advisor (M15.1). They are separate concerns; do not entangle.
