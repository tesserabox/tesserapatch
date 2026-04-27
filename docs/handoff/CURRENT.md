# Current Handoff

## Active Task

- **Task ID**: (none — `v0.6.1` shipped)
- **Milestone**: M15 stabilization stream **CLOSED**. Next: Wave 3 design pass (PRD/ADR), implementer dispatch deferred until design lands.
- **Status**: Idle — awaiting user direction on Wave 3 scope.
- **Assigned**: 2026-04-27

## Session Summary

`v0.6.1` tagged and pushed. Release contents:

- **M15-W1** (3 items): satisfied_by reachability, skill frontmatter, `spec` alias.
- **M15-W2** (4 items): OS-aware shell selection, recipe autogen, recipe drift detection, `record --files` scoping.
- **Fix-pass** (4 medium findings against the merged surface): F1 satisfied_by 40-hex+reachability contract alignment, F2 scoped diffstat metadata, F3 propagated-pathspec errors, F4 Windows-aware shell quoting.

7 backlog items + 4 fix-pass findings closed since `v0.6.0`. 57 pending todos remain in SQL.

## Current State

- HEAD on `origin/main`: `v0.6.1` tag + release commit.
- Build clean, full test suite green, parity guard holds.
- Recipe-op JSON schema unchanged; `delete-file` op still gated on a future ADR.
- Source-truth guard (ADR-011 D6) preserved across the entire M15 stream.
- Hookable-var pattern (`var isAncestor`, `var userShellFor`) is now an established convention for unit-test isolation of external commands.
- Apply-gate ↔ validation contract is now aligned on `satisfied_by` (40-hex + reachability in validation; cheap 40-hex regex in apply-gate as defense-in-depth).

## Decision Points for User

### Wave 3 scope and ordering

Original 4-wave plan reserved Wave 3 for the **lifecycle / reconcile semantics tranche**. All four candidates touch ADR-011 territory and warrant a PRD/ADR pass before any implementer dispatch.

Recommended ordering (lowest blast radius first):

1. **`feat-verify-command`** — new `tpatch verify <slug>` re-applicability check. Mostly additive; minimal interaction with existing reconcile/state code. Good first PRD.
2. **`feat-feature-tested-state`** — new lifecycle state between `applied` and `active`. Clear ADR territory (state machine touches every transition). Should be PRD'd alongside `verify` since they likely share machinery.
3. **`feat-reconcile-code-presence-verdicts`** — evidence-based reconcile (kill false-positive `upstreamed`). Touches reconcile authority logic; needs an ADR amendment to ADR-010/ADR-011.
4. **`feat-reconcile-fresh-branch-mode`** — codify the fresh-branch recovery pattern. Largest scope; consider after the first three land.

Defer-for-later (still valid):
- `feat-feature-provider-overrides`, `feat-upstream-merged-override`, `feat-explore-prereq-enforcement`, `feat-agent-collision-detection`, plus the freshly-groomed `feat-reconcile-strategy-steering` and `feat-command-steering-hooks`.

## Files Changed

This release commit only (post-fix-pass):
- `internal/cli/cobra.go` (version bump `0.6.0` → `0.6.1`).
- `CHANGELOG.md` (new `v0.6.1` section above `v0.6.0`).
- `docs/handoff/HISTORY.md` (fix-pass archive prepended).
- `docs/handoff/CURRENT.md` (reset to post-release idle).

## Test Results

`v0.6.1` validation gate:
- `gofmt -l .` clean.
- `go build ./cmd/tpatch` clean (root binary removed).
- `go test ./...` clean across all 7 packages.

## Next Steps

**Idle.** When user gives direction, supervisor:

1. (If Wave 3 design first — recommended) write `docs/prds/PRD-verify-command.md` and an ADR amendment if needed; review; then dispatch implementer.
2. (If Wave 3 implementer-first for `verify` only) skip PRD; dispatch `m15-w3-verify-implementer` with a tight contract (additive command only, no state-machine touch).
3. (If pulling a different backlog item) pick from the deferred list and dispatch through the standard implementer→reviewer cycle.

## Blockers

None.

## Context for Next Agent

- **`tpatch` binary at the repo root is NOT gitignored.** Always `rm -f tpatch` after `go build ./cmd/tpatch` BEFORE staging.
- **Commit trailer mandatory**: `Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>`. Use `git -c commit.gpgsign=false`.
- **Source-truth guard (ADR-011 D6)**: any DAG/label/status code reads `status.Reconcile.Outcome` via `store.LoadFeatureStatus`, NEVER `artifacts/reconcile-session.json`.
- **Recipe vs patch authority**: `artifacts/post-apply.patch` is the reconcile source of truth. Recipes serve replay/inspection.
- **Skill parity guard** (`assets/assets_test.go`) enforces required CLI-command anchors and the recipe-op JSON schema. Treat as a real reviewer.
- **`git push` is slow** (60+ s typical).
- **Hookable-var pattern**: `var isAncestor = gitutil.IsAncestor` (Wave 1), `var userShellFor` (Wave 2). Use this for any new external-command call site so unit tests stay environment-free.
- **Recipe-op schema gap**: no `delete-file` op type. Wave 2's `RecipeFromPatch` skips deletes + warns. Adding the op type requires an ADR and parity-guard update.
- **`--regenerate-recipe` flag** on `recordCmd`: explicit operator opt-in to overwrite a stale recipe. Default behavior is non-destructive sidecar.
- **`satisfied_by` contract is now 40-hex AND reachable.** Validation refuses to persist anything else; apply-gate keeps the 40-hex regex as defense-in-depth. Existing v0.6.0 repos that declared a short SHA will fail validation on next status — fix by replacing with the full 40-hex commit SHA.
- **Self-reviews are status signals, not approval signals.** The M15-W2 sub-agent reviewer returned "zero findings" but an external re-review found 4 mediums. Always require an outside read for release-gating decisions.
