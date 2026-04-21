# Current Handoff

## Active Task

- **Task ID**: Backlog triage + v0.4.4 scoping
- **Milestone**: Bridge between B1 (v0.4.3 "Stand-In Agent, Part 1") and B-next
- **Status**: v0.4.3 shipped; live stress-test in t3code surfaced 3 bugs; backlog triaged; v0.4.4 scope defined.

## Session Summary

### Live stress-test findings (t3code, ~20h)

The v0.4.3 release was exercised against a real fork (tesserabox/t3code, fork of pingdotgg/t3code). 9 features applied, 1 upstream sync of 14 commits. Three real bugs + three new feature candidates surfaced. All logged.

### Backlog triage

- Removed 1 duplicate: `feat-spec-drift-detection` merged into `feat-reconcile-metadata-refresh` (drift detection is the read-only subset of refresh; shared scope).
- Cross-linked 4 overlapping pairs via the "## Related" note convention so future passes do not re-cover the same ground:
  - `feat-record-scoped-files` ↔ `feat-noncontiguous-feature-commits` (same problem, two solutions).
  - `feat-feature-removal` ↔ `feat-richer-operation-types` (revert ≈ remove).
  - `feat-ci-cd-integration` ↔ `feat-dependabot-bot` (primitive vs wrapper).
  - `feat-record-autogen-recipe` ↔ `bug-recipe-stale-after-manual-flow` (autogen resolves the stale-recipe bug by construction).
  - `feat-recipe-migrate-to-templates` → `feat-recipe-template-ops` + `feat-record-autogen-recipe` (align schemas).

Backlog count: 46 pending, 52 done.

## v0.4.4 — Proposed Scope ("Honest Recipes")

Two HIGH-severity bugs from the case study are the only must-ship items. Small, tight release.

1. **`bug-skill-recipe-schema-mismatch` (HIGH)**
   The v0.4.3 skills document `op`/`contents`/`occurrences`/`delete-file`. CLI accepts `type`/`content`/no-occurrences/no-delete-file. Every Path B `implement` user hits the wall today.
   - Fix: correct all 6 skill formats + `docs/agent-as-provider.md` to match `internal/workflow/implement.go:27` exactly.
   - Document `append-file` (currently undocumented, supported).
   - Update the 6 v0.4.3 parity-guard anchors that reference the wrong field names.
   - Add a unit test that round-trips a skill's example recipe JSON through `json.Unmarshal` into `RecipeOperation` so docs and code cannot drift again.

2. **`bug-reconcile-reapplied-with-conflict-markers` (HIGH)**
   A4 (v0.4.2) added the 3WayConflicts verdict in an isolated worktree but the case study still got "reapplied" on a tree with 20 files of `<<<<<<<` markers.
   - Fix: add a final `git grep -E '^(<{7}|={7}|>{7}) '` scan of the working tree before any `ReconcileReapplied` return. Any hit promotes to `ReconcileBlocked` + names the offending files in the error.
   - Audit every code path in `internal/workflow/reconcile.go` that sets `ReconcileReapplied`; gate each on the marker scan.
   - Regression test: synthetic 3-file conflict fixture that must classify as `3WayConflicts` / `Blocked`.

### Not in v0.4.4
- `bug-record-roundtrip-false-positive-markdown` — cosmetic, needs more investigation. Next patch release.
- `feat-recipe-schema-expansion` — real feature work, target v0.5.0 alongside provider-conflict-resolver.
- Everything else in the backlog — untouched.

### Shipping checklist for v0.4.4
- [ ] Fix skill schema mismatch (6 skills + docs + parity anchors + test).
- [ ] Fix reconcile conflict-marker false positive (reconcile.go + fixture test).
- [ ] CHANGELOG v0.4.4 section.
- [ ] Bump `internal/cli/cobra.go` version to 0.4.4.
- [ ] `gofmt`, `go test ./...`, `go build ./cmd/tpatch`.
- [ ] Single commit with co-author trailer, tag, push.

## Files Changed (this session)

- Backlog-only (no code changes). SQL todos + `docs/handoff/CURRENT.md` only.

## Test Results

N/A (no code changes this session).

## Next Steps

1. Ship v0.4.4 per the scope above.
2. Decide v0.5.0 headline:
   - Option A: `feat-provider-conflict-resolver` (ADR-010, the real core value prop).
   - Option B: `feat-recipe-schema-expansion` + `feat-record-autogen-recipe` (the recipe-system modernisation).
   - Option C: `feat-feature-dependencies` DAG (unlocks several workflows).

## Blockers

None.

## Context for Next Agent

- Cross-link convention: add a `## Related` section at the bottom of a todo's description when another todo shares the same scenario or plumbing. Avoids silent duplicates.
- The `op`/`type` skill bug is my fault from the v0.4.3 rewrite. The fix must include a doctest-style guard so future skill edits cannot drift from the `RecipeOperation` struct again.
- `bug-reconcile-reapplied-with-conflict-markers` is the scariest bug in the tree right now — users can commit conflict markers on trust of a false "reapplied" verdict. Treat as P0.
