# Current Handoff

## Active Task

- **Task ID**: M15-W3-SLICE-A
- **Milestone**: M15 → Wave 3 (lifecycle / reconcile semantics tranche) — **Slice A implementation**
- **Description**: Implement the Slice A surface of the approved freshness-overlay design: `tpatch verify <slug>` cobra shell with four flags, V0/V1/V2 real check implementations, V3–V9 stubs (the full 10-check array still appears in the report so the shape is reviewable now), `Verify *VerifyRecord` field on `FeatureStatus` with `omitempty`, and minimal EXPERIMENTAL skill stubs to keep the parity guard green.
- **Status**: Awaiting external review
- **Assigned**: 2026-04-27

## Binding context

- **Redesign approved**: `origin/main` at commit `3c122aa` — APPROVED WITH NOTES.
- **Design contract**: `docs/prds/PRD-verify-freshness.md` (Approved) + `docs/adrs/ADR-013-verify-freshness-overlay.md` (Accepted). Do **not** reopen the model. The freshness overlay is locked.
- **Reviewer notes (from `docs/supervisor/LOG.md` top entry, binding implementation guidance for this slice)**:
  - **Note 1 — persisted CheckResults bloat**. Implementer choice. Disposition: **drop** the per-check array from the persisted `VerifyRecord` and emit it only in the `--json` stdout report. Persisted record carries only `verified_at`, `passed`, `recipe_hash_at_verify`, `patch_hash_at_verify`, `parent_snapshot`.
  - **Note 2 — V2 absent recipe**. Disposition: V2 (`apply-recipe.json` parses + op targets resolve) treats an absent recipe as `passed: true, skipped: true, reason: "no apply-recipe.json (legacy / pre-autogen-era feature)"`. No false-fail; no crash.
  - **Note 3 — parity-guard handling**. Disposition: add minimal one-sentence EXPERIMENTAL `tpatch verify` stubs to all six skill surfaces. Full skill copy lands in Slice D; Slice A only has to keep `assets/assets_test.go` green.

## Hard rules in force for this slice

- Apply gate stays untouched (`internal/workflow/dependency_gate.go` not modified). ADR-013 D2.
- Writer lives only on the explicit `verify` verb. No mutation from `LoadFeatureStatus`, `ComposeLabels`, status rendering, or any other read path. ADR-013 D5.
- `Verify *VerifyRecord` carries `omitempty`; v0.6.1 fixtures that never run verify round-trip byte-identical. ADR-013 D4.
- Recipe-op JSON schema frozen.
- Reuse `safety.EnsureSafeRepoPath` for any file-path validation; reuse the existing slug-resolution / store-open helpers (`openStoreFromCmd`).
- Slice A explicitly **does not** ship: `--all`, `--shadow`, closure replay (V7/V8 stubbed), `ComposeLabels` freshness derivation, full-text skill copy. Slices B/C/D handle those.

## Session Summary

- Added the `Verify *VerifyRecord` field to `FeatureStatus` (`internal/store/types.go`) with `omitempty`. Persisted record carries only `verified_at`, `passed`, `recipe_hash_at_verify`, `patch_hash_at_verify`, `parent_snapshot` — Note 1 disposition: dropped per-check array from persistence (stdout-only).
- Added the dedicated explicit-write entry point `Store.WriteVerifyRecord` (`internal/store/store.go`). Read paths (`LoadFeatureStatus`, `MarkFeatureState`, etc.) are unchanged.
- New `internal/workflow/verify.go` with `RunVerify`, real V0/V1/V2 implementations, V3–V9 stubs (`passed:true, skipped:true, reason:"not yet implemented (Slice C)"`), and the in-memory 10-check report builder.
- New `internal/cli/verify.go` registering `tpatch verify <slug>` with `--json`, `--quiet`, `--no-write`. The `--path` persistent flag is inherited from root (matching `apply` / `record` / `status`).
- All six skill surfaces (claude, copilot, copilot-prompt, cursor, windsurf, generic) gain a single one-sentence EXPERIMENTAL bullet — Note 3 disposition. Full skill copy is deferred to Slice D per PRD §4.4.
- Tests: V0 abort, V1 pass + fail (missing + empty spec), V2 pass + fail (malformed JSON, missing op target) + absent-recipe Note 2 contract, `--no-write` non-persistence, `--json` shape with all 10 IDs in order, stub-reason naming a future slice. Plus two store-level round-trip tests guarding the `omitempty` byte-identity contract and the populated-record round-trip.
- Apply gate untouched. `composeLabelsFromStatus` untouched. No closure replay (Slice C). No `--all` (Slice D).

### Revision (post-review, 2026-04-27)

- Reviewer issued **NEEDS REVISION** with one blocking finding: `parentSnapshot` recorded `""` for a missing hard parent, which is not a valid `FeatureState` enum and would defer a crash into Slice B's `satisfies_state_or_better` derivation.
- Chosen fix: **omit missing parents from the snapshot map entirely**, rather than encode a sentinel state. Detecting a structurally missing parent is a `tpatch status` / dependency-validation concern, not the freshness layer's job. Slice B can iterate present keys without enum-value gymnastics.
- Behavior on the all-missing edge: `parentSnapshot` returns `nil`, so the `omitempty`-tagged field stays absent from JSON, preserving byte-identical round-trip with the never-verified baseline (ADR-013 D4). Documented in the function godoc.
- Tests added in `internal/workflow/verify_test.go`:
  - `TestParentSnapshot_MissingParentOmitted` — one parent exists (`applied`), one is missing → exactly one key, missing slug not present.
  - `TestParentSnapshot_AllParentsMissingReturnsNil` — every hard parent missing → `nil`.
  - `TestParentSnapshot_SoftDepsExcluded` — preserves the existing soft-dep exclusion contract.
- Validation re-run: `gofmt -l .` clean, `go test ./...` green, `go build ./cmd/tpatch` succeeds.
- Status: **ready for re-review**.

## Current State

- Slice A surface complete and gated by full test suite.
- The four derived freshness labels (`never-verified` / `verified-fresh` / `verified-stale` / `verify-failed`) are NOT yet wired into `tpatch status` / `--dag` / `--json` — that is Slice B's scope.
- V7/V8 are stubs; closure-replay primitive lands in Slice C.
- The full skill copy from PRD §4.4 is not in the skill files yet — only the minimal one-liner that keeps the parity guard green.

## Files Changed

- `docs/handoff/CURRENT.md` (this file)
- `docs/handoff/HISTORY.md` (Phase-1 archive of M15-W3-REDESIGN)
- `docs/prds/PRD-verify-freshness.md` (Phase-1: status line)
- `internal/store/types.go` (new `Verify` field + `VerifyRecord`/`VerifyCheckResult` types)
- `internal/store/store.go` (new `WriteVerifyRecord` writer)
- `internal/store/roundtrip_test.go` (two new round-trip tests)
- `internal/cli/cobra.go` (registers `verifyCmd`)
- `internal/cli/verify.go` (new — cobra shell)
- `internal/workflow/verify.go` (new — `RunVerify` + checks + helpers)
- `internal/workflow/verify_test.go` (new — eleven test cases)
- `assets/skills/claude/tessera-patch/SKILL.md`
- `assets/skills/copilot/tessera-patch/SKILL.md`
- `assets/prompts/copilot/tessera-patch-apply.prompt.md`
- `assets/skills/cursor/tessera-patch.mdc`
- `assets/skills/windsurf/windsurfrules`
- `assets/workflows/tessera-patch-generic.md`

## Test Results

```
$ gofmt -l .
(empty)

$ go test ./...
ok  	github.com/tesseracode/tesserapatch/assets	1.688s
?   	github.com/tesseracode/tesserapatch/cmd/tpatch	[no test files]
ok  	github.com/tesseracode/tesserapatch/internal/cli	9.645s
ok  	github.com/tesseracode/tesserapatch/internal/gitutil	(cached)
ok  	github.com/tesseracode/tesserapatch/internal/provider	(cached)
ok  	github.com/tesseracode/tesserapatch/internal/safety	(cached)
ok  	github.com/tesseracode/tesserapatch/internal/store	2.354s
ok  	github.com/tesseracode/tesserapatch/internal/workflow	18.165s

$ go build ./cmd/tpatch
(success; binary removed with `rm -f tpatch` after build)
```

## Reviewer-note dispositions (recorded for the external review)

- **Note 1 (CheckResults persistence)**: **Resolved by dropping** the per-check array from the persisted record. The full 10-check array is built in-memory by `RunVerify` and emitted on `--json` stdout only. Persisted record matches the brief's minimal field set (`verified_at`, `passed`, `recipe_hash_at_verify`, `patch_hash_at_verify`, `parent_snapshot`).
- **Note 2 (V2 absent recipe)**: An absent `apply-recipe.json` makes both V2 sub-checks (`recipe_parses` and `recipe_op_targets_resolve`) emit `passed: true, skipped: true, reason: "no apply-recipe.json (legacy / pre-autogen-era feature)"`. Verdict stays `passed`; no false-fail; covered by `TestRunVerify_V2_AbsentRecipe_SkippedNotFailed`.
- **Note 3 (parity guard)**: Minimal one-sentence EXPERIMENTAL stubs added to all six skill surfaces. The parity guard's `requiredCommands` array was NOT extended to add `tpatch verify`; the guard's existing list still passes byte-for-byte. Full §4.4 skill copy remains Slice D's deliverable.

## What remains for Slices B / C / D

- **Slice B**: extend `ReconcileLabel` vocabulary with `LabelNeverVerified` / `LabelVerifiedFresh` / `LabelVerifiedStale` / `LabelVerifyFailed`; wire freshness derivation into `composeLabelsFromStatus` per PRD §3.4.2 (pure function, no writes); `tpatch status` and `--json` rendering; `tpatch amend (recipe-touching)` invalidates `Verify.Passed`; reject `tpatch amend --state tested`.
- **Slice C**: V3 (created_by semantics), V4 (`store.ValidateDependencies`), V5 (`gitutil.IsAncestor`), V6 (warn), V7+V8 hard-parent topological closure replay + target recipe replay + patch `--check`, V9 (`status.Reconcile.Outcome` consistency). Replace stub records with real check results in `RunVerify`.
- **Slice D**: `tpatch verify --all` (topo-ordered aggregate, pre-apply skips per Q2), full skill paragraph from PRD §4.4 across all six surfaces, parity-guard anchor extension (if needed for the new copy), `docs/dependencies.md` cross-link, CHANGELOG v0.6.2 entry.

## Open questions for the reviewer

None — Slice A scope was precise. Two minor implementation choices flagged for the reviewer's eye:

1. **Skill stub form**: I added one bullet under each skill's command-list section rather than a dedicated paragraph. Slice D's full §4.4 paragraph will replace these stubs cleanly. Reviewer may prefer a different anchor; trivial to relocate.
2. **`computeVerdict` semantics in Slice A**: warn-severity failures do not flip the verdict. The PRD §6 / Q1 records this as the binding rule for V9; Slice A's only warn-severity stubs are V6 and V9 stubs (both currently `passed: true`), so the rule is not exercised yet but already coded.

## Blockers

None. Awaiting external review.

## Context for Next Agent

- Read order: PRD-verify-freshness.md §3.4 + §4 + §9 (Slice A row), ADR-013 D1/D4/D5/D7, then `docs/supervisor/LOG.md` top entry.
- The persisted record's minimal field set is locked. Slice B's `composeLabelsFromStatus` extension reads `Verify.RecipeHashAtVerify`, `Verify.PatchHashAtVerify`, `Verify.ParentSnapshot`, `Verify.Passed` — all present.
- The full 10-check report shape is exercised by `TestRunVerify_JSONShape`. Slice C must keep the order + IDs stable when filling in real implementations for V3–V9.
- `tpatch verify` lives on the explicit-write side. Do NOT add the field to a read path. ADR-013 D5 + Reviewer Note 1.
- The `tpatch` root binary is not gitignored. `rm -f tpatch` after `go build`.
- Every commit must carry the `Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>` trailer.
