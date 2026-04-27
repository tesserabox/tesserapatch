# Current Handoff

## Active Task

- **Task ID**: (none — M15-W2 closed)
- **Milestone**: M15 stream → **user-mandated review pause before Wave 3**
- **Status**: Idle — awaiting user direction on (a) v0.6.1 cut, (b) Wave 3 dispatch scope
- **Assigned**: 2026-04-26

## Session Summary

**M15-W1 + M15-W2 both shipped, both APPROVED.**

| Wave | Items | Commits | Verdict |
|---|---|---|---|
| W1 | satisfied_by reachability, skill frontmatter, spec alias | `aa0f93e`, `d5f934f`, `99ee60e`, `57bf1ab` | APPROVED WITH NOTES (note closed) |
| W2 | shell selection, recipe autogen, recipe drift detection, --files scoping | `e7f524d`, `dbd44c2`, `d402653` | APPROVED (zero findings) |

7 backlog items shipped end-to-end since `v0.6.0`. 57 pending todos remain in SQL.

## Current State

- HEAD is on `origin/main` post-Wave-2 closeout (this commit).
- Build clean, full test suite green, parity guard holds.
- Recipe-op JSON schema **unchanged** — Wave 2's recipe autogen does NOT silently extend it. `delete-file` op is a known schema gap that needs an ADR before adding (deferred).
- Source-truth guard (ADR-011 D6) preserved across all 7 commits.
- Hookable seam pattern (Wave 1 `isAncestor`, Wave 2 `userShellFor`) now established as a convention for unit-test isolation of external commands.

## Decision Points for User

### Decision 1 — `v0.6.1` cut?

7 user-visible improvements have shipped since `v0.6.0`. Strong v0.6.1 candidate:
- Validation hardening (satisfied_by reachability)
- Skill loader compatibility (frontmatter)
- CLI ergonomics (`spec` alias)
- Cross-platform fix (shell selection)
- Path B parity (recipe autogen, drift detection, scoped capture)

If yes → supervisor: bump version, write CHANGELOG section, tag, push tag.
If hold → continue accumulating Wave 3 items first.

### Decision 2 — Wave 3 scope

The original 4-wave plan called out Wave 3 as the **larger lifecycle/reconcile semantics tranche**:
- `feat-verify-command` — new `tpatch verify <slug>` re-applicability check
- `feat-feature-tested-state` — new lifecycle state between `applied` and `active` (likely needs ADR)
- `feat-reconcile-code-presence-verdicts` — evidence-based reconcile (kill false-positive `upstreamed`)
- `feat-reconcile-fresh-branch-mode` — codify the fresh-branch recovery pattern

These touch ADR-011 territory (state machine, reconcile authority) and warrant a PRD/ADR pass before dispatch. Recommend the user picks 1–2 to scope first rather than dispatching all 4 as one packet.

Defer-for-now items (still valid, lower priority):
- `feat-feature-provider-overrides` — per-feature provider/model
- `feat-upstream-merged-override` — auditable manual flip
- `feat-explore-prereq-enforcement` — guardrail before implement
- `feat-agent-collision-detection` — file-hash drift warnings

## Files Changed

This handoff transition only:
- `docs/handoff/CURRENT.md` (reset to idle)
- `docs/handoff/HISTORY.md` (M15-W2 archive prepended)

## Test Results

Last green validation gate: M15-W2 reviewer APPROVED on `go test ./...` clean across all 7 packages, `gofmt -l .` empty, `go build ./cmd/tpatch` clean, parity guard green.

## Next Steps

**Pause here.** User direction needed before either tagging `v0.6.1` or dispatching Wave 3 implementer.

When user gives direction, supervisor:
1. (If tag) bump version constants, write CHANGELOG entry, commit, tag, push.
2. (If Wave 3) write per-item dispatch contract for the chosen scope, mark todos `in_progress`, dispatch implementer.

## Blockers

None — explicit user-mandated review pause.

## Context for Next Agent

- **`tpatch` binary at the repo root is NOT gitignored.** Always `rm -f tpatch` after `go build ./cmd/tpatch` BEFORE staging.
- **Commit trailer mandatory**: `Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>`. Use `git -c commit.gpgsign=false`.
- **Source-truth guard (ADR-011 D6)**: any DAG/label/status code reads `status.Reconcile.Outcome` via `store.LoadFeatureStatus`, NEVER `artifacts/reconcile-session.json`.
- **Recipe vs patch authority**: `artifacts/post-apply.patch` is the reconcile source of truth. Recipes serve replay/inspection. Wave 2 preserved this invariant; Wave 3+ must too.
- **Skill parity guard** (`assets/assets_test.go`) enforces required CLI-command anchors and the recipe-op JSON schema. Treat as a real reviewer.
- **`git push` is slow** (60+ s typical).
- **Hookable-var pattern** (Wave 1 `var isAncestor = gitutil.IsAncestor`, Wave 2 `var userShellFor = ...`): use this for any new external-command call site so unit tests stay environment-free.
- **Recipe-op schema gap**: no `delete-file` op type. Wave 2's `RecipeFromPatch` skips deletes + warns. Adding the op type requires an ADR and parity-guard update — flagged for a future wave.
- **Wave 2 `--regenerate-recipe` flag** on `recordCmd`: explicit operator opt-in to overwrite a stale recipe. Default behavior is non-destructive sidecar.
