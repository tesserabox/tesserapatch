# Current Handoff

## Active Task

- **Task ID**: M12 / Tranche B2 / v0.5.0 — Provider-Assisted Conflict Resolver
- **Status**: In progress. Scoping landed; implementing `b2-shadow-worktree` first.
- **PRD**: `docs/prds/PRD-provider-conflict-resolver.md`
- **Milestone**: `docs/milestones/M12-provider-conflict-resolver.md`
- **ADR**: `docs/adrs/ADR-010-provider-conflict-resolver.md`
- **10 todos tracked** (`b2-*`) with dependencies. Next to work: `b2-shadow-worktree`, `b2-validation-gate` (parallel-safe).

### PRD-locked answers to ADR-010's 6 open questions

1. Sequential per-file calls (no parallelism in v0.5.0).
2. Single combined `patches/NNN-reconcile.patch`; per-file audit in `reconcile-session.json`.
3. `test_command` is the `--apply` gate; refused if unset.
4. One shadow per feature; reap prior on new resolve.
5. Full file with `max_file_bytes` cap (default 200 KB); oversize → `skipped-too-large`.
6. Golden scenarios harness ships with v0.5.0 (≥5 scenarios).

### Follow-ups registered

- `feat-resolver-heuristic-fallback` (opt-in `--heuristic`, post-B2 maturity).
- `feat-feature-standalonify` (blocks on `feat-feature-dependencies`).

## Session Summary

Two HIGH bugs from the t3code v0.4.3 live stress test fixed and shipped.

1. **Skill recipe schema mismatch** — v0.4.3 skills documented `op`/`contents`/`occurrences`/`delete-file`; CLI reads `type`/`content`/no-occurrences/no-delete-file. Corrected all 6 skills + `docs/agent-as-provider.md`. Added `TestSkillRecipeSchemaMatchesCLI` — extracts every ```json block, unmarshals into `workflow.RecipeOperation` with `DisallowUnknownFields`, and validates op types. Prevents future drift.

2. **Reconcile reapplied-with-conflict-markers** — the degraded `PreviewForwardApply` fallback used to return `3WayClean` when `git worktree add` failed, undoing v0.4.2 A4. Now returns `Blocked`. Added `ScanConflictMarkers` defensive pass on the live tree after every Reapplied verdict; markers promote to Blocked. New test `TestReconcilePromotesOnLiveMarkers`.

Both bugs were direct B2 prerequisites (agents need a correct recipe schema; B2's resolver hooks on `3WayConflicts` which phase 4 was silently skipping).

## Files Changed

- `assets/skills/claude/tessera-patch/SKILL.md` — recipe schema block rewritten (`type`/`content`, append-file documented, delete-file/occurrences disclaimer).
- `assets/skills/copilot/tessera-patch/SKILL.md`, `assets/prompts/copilot/tessera-patch-apply.prompt.md`, `assets/skills/cursor/tessera-patch.mdc`, `assets/skills/windsurf/windsurfrules`, `assets/workflows/tessera-patch-generic.md` — recipe JSON block + semantics rewritten to match CLI.
- `docs/agent-as-provider.md` — recipe schema rewritten.
- `assets/assets_test.go` — new `TestSkillRecipeSchemaMatchesCLI`.
- `internal/gitutil/gitutil.go` — `PreviewForwardApply` degraded path returns Blocked; `ScanConflictMarkers` exported.
- `internal/workflow/reconcile.go` — `promoteIfMarkers` defensive pass on Reapplied paths.
- `internal/workflow/reconcile_test.go` — `TestReconcilePromotesOnLiveMarkers` regression.
- `internal/cli/cobra.go` — version → 0.4.4.
- `CHANGELOG.md` — v0.4.4 section.

## Test Results

- `gofmt -l .` — clean.
- `go build ./...` — ok.
- `go test ./...` — all packages pass. Two new tests green (`TestSkillRecipeSchemaMatchesCLI`, `TestReconcilePromotesOnLiveMarkers`).

## Next Steps — pick Tranche B2 scope

1. **Option A — `feat-provider-conflict-resolver`** (ADR-010, v0.5.0 headline): phase 3.5 in reconcile, shadow worktree, per-file provider call. The core value prop. Now unblocked by v0.4.4.
2. **Option B — Recipe modernisation**: `feat-recipe-schema-expansion` (add `delete-file`, `rename-file`, op aliases) + `feat-record-autogen-recipe` (derive recipe from diff on record). Makes Path B fully self-contained.
3. **Option C — `feat-feature-dependencies` DAG**: first-class depends_on plumbing; unlocks stacked features and ordered reconcile.

## Blockers

None.

## Context for Next Agent

- The new `TestSkillRecipeSchemaMatchesCLI` is strict (`DisallowUnknownFields`). Any future skill edit that invents a field will fail the build at the assets test. If the CLI adds a field (e.g. `occurrences`), update both `workflow.RecipeOperation` and the skills in the same commit.
- `ScanConflictMarkers` is now public (`gitutil.ScanConflictMarkers`). Reuse it anywhere a "did this really succeed?" check is needed (e.g. after `apply --mode execute`).
- The degraded path in `PreviewForwardApply` now refuses to guess. If users start seeing "worktree preview unavailable — refusing to guess", they have a real environment issue (bare repo, disk full, permissions) that was previously being masked.
