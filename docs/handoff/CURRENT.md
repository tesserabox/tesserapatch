# Current Handoff

## Active Task

- **Task ID**: M15-W3-SLICE-A
- **Milestone**: M15 → Wave 3 (lifecycle / reconcile semantics tranche) — **Slice A implementation**
- **Description**: Implement the Slice A surface of the approved freshness-overlay design: `tpatch verify <slug>` cobra shell with four flags, V0/V1/V2 real check implementations, V3–V9 stubs (the full 10-check array still appears in the report so the shape is reviewable now), `Verify *VerifyRecord` field on `FeatureStatus` with `omitempty`, and minimal EXPERIMENTAL skill stubs to keep the parity guard green.
- **Status**: In Progress
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

(populated as work proceeds)

## Current State

(populated as work proceeds)

## Files Changed

(populated as work proceeds)

## Test Results

(populated as work proceeds)

## Next Steps

1. Add `VerifyRecord` + `VerifyCheckResult` to `internal/store/types.go`. Persisted record carries the minimal field set per Note 1.
2. Add `Verify *VerifyRecord` field to `FeatureStatus` with `omitempty`.
3. Add `internal/workflow/verify.go` with `RunVerify(...) (*VerifyReport, error)`, V0/V1/V2 real implementations, V3–V9 stubs, full 10-check JSON report builder, parent-snapshot derivation.
4. Add `verifyCmd` to `internal/cli/cobra.go` (style: `applyCmd` / `recordCmd`).
5. Add minimal EXPERIMENTAL `tpatch verify` line to all six skill surfaces.
6. Add tests: V0/V1/V2 pass+fail, V2 absent recipe, `--no-write` honoured, `--json` shape, omitempty round-trip.
7. Run `gofmt -l . && go test ./... && go build ./cmd/tpatch && rm -f tpatch`.
8. Update CURRENT.md with Slice B/C/D remaining-work bullets and `Status: Awaiting external review`.

## Blockers

None — Slice A scope is precisely defined by PRD §9 + reviewer notes.

## Context for Next Agent

- Read order if you arrive cold: PRD-verify-freshness.md §3.4 + §4 + §9 (Slice A row), ADR-013 D1/D4/D5/D7, then `docs/supervisor/LOG.md` top entry for the three notes.
- The stub semantics for V3–V9 are: `passed: true, skipped: true, reason: "not yet implemented (Slice <X>)"`. The full 10-check array still emits in `--json`; only the persisted record is trimmed.
- Persisted record minimal field set is the disposition for Note 1. The full check array round-trips via stdout, never through `status.json`.
- Closure-replay primitive (V7/V8) is Slice C; Slice A's V7/V8 are stubs.
- `composeLabelsFromStatus` is **not** extended in this slice (Slice B). The four derived freshness labels (`never-verified` / `verified-fresh` / `verified-stale` / `verify-failed`) are not wired into `tpatch status` yet.
- The `tpatch` root binary is not gitignored. `rm -f tpatch` after `go build`.
- Every commit must carry the `Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>` trailer.
- This task ends with `Status: Awaiting external review` — do **not** self-approve. Supervisor + user gate before push.
