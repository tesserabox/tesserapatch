# Current Handoff

## Active Task

- **Task ID**: M15-W3-SLICE-B
- **Milestone**: M15 Wave 3 — Verify freshness overlay
- **Description**: Slice B — freshness derivation + label integration.
- **Status**: Review — implementation complete, tests green, awaiting
  reviewer.
- **Assigned**: 2026-04-27

## Session Summary

Implemented Slice B end-to-end per PRD-verify-freshness §9 and ADR-013
D1–D7:

1. Extended `ReconcileLabel` enum with the four freshness constants
   (`LabelNeverVerified`, `LabelVerifiedFresh`, `LabelVerifiedStale`,
   `LabelVerifyFailed`) in `internal/store/types.go`.
2. Refactored `composeLabelsFromStatus` to compose M14.3 + freshness
   labels, preserving F3 (retired-child short-circuit). Added pure
   helpers `deriveFreshnessLabel`, `hashMatchesCurrent`,
   `satisfiesStateOrBetter`, `IsFreshnessLabel`,
   `StripFreshnessLabels`, and exported `DeriveFreshnessLabel` for
   render-layer callers. Hookable-var
   `readArtifactBytesForFreshness` for tests.
3. All persistence sites (`saveReconcileArtifacts`, phase-3.5
   short-circuit in `reconcile.go`, `accept.go`) now call
   `StripFreshnessLabels` before writing `Reconcile.Labels` →
   freshness never persists (D4 byte-identity).
4. Status rendering: `tpatch status` text mode now suffixes
   `(label, label)` after the title; `tpatch status --json` emits a
   per-feature `freshness_label` and `labels_rendered` field;
   `tpatch status --dag` (text + JSON) merges the freshness label into
   the rendered label set; the JSON shape gains `freshness_label` and
   `verify` per node.
5. `tpatch amend`: detects recipe-touching by comparing
   `apply-recipe.json` bytes pre/post and clears the `Verify`
   sub-record on diff (`clearVerifyForAmend`). `--state` flag added,
   wired solely to reject any value (notably `tested`) with
   `*ExitCodeError{Code:2}` per PRD §9 / ADR-013 D3.

## Files Changed

### Source

- `internal/store/types.go` — 4 new ReconcileLabel constants.
- `internal/workflow/labels.go` — full freshness derivation;
  IsFreshnessLabel / StripFreshnessLabels / DeriveFreshnessLabel
  exported; composeM143Labels extracted; F3 retired-child preserved.
- `internal/workflow/reconcile.go` — both persistence sites strip
  freshness.
- `internal/workflow/accept.go` — strips freshness before writing
  Reconcile.Labels.
- `internal/cli/status_dag.go` — `dagJSONNode` extended with
  `FreshnessLabel` + `Verify`; `walkTree` / `renderNodeLine` plumbed
  with `*store.Store`; `mergedLabels` helper added.
- `internal/cli/cobra.go` — `statusCmd` text + JSON modes render the
  freshness label and the merged label set.
- `internal/cli/c1.go` — `amendCmd` reads recipe pre/post bytes,
  clears `Verify` on diff, rejects `--state` values with exit 2.

### Tests (new)

- `internal/workflow/labels_freshness_truthtable_test.go` — full
  truth-table matrix + state-or-better invariants + empty-hash match.
- `internal/workflow/dependency_gate_freshness_test.go` — D2
  invariant (apply gate ignores parent freshness).
- `internal/workflow/slice_b_byte_identity_test.go` — v0.6.1
  byte-identity guard + persistence-strip guard.
- `internal/cli/amend_freshness_test.go` — recipe-touching
  invalidation, deps-only preservation, `--state tested` exit-2,
  `--state <any>` exit-2.

### Tests (updated)

- `internal/workflow/labels_test.go`,
  `internal/workflow/labels_upstreamed_test.go` — updated to expect
  freshness label merged into the derived set (8+1 cases).
- `internal/cli/status_dag_test.go` — assertion on merged label
  suffix `(blocked-by-parent, never-verified)`.

## Test Results

```
$ gofmt -l .
(empty)
$ go test ./...
ok  github.com/tesseracode/tesserapatch/assets
ok  github.com/tesseracode/tesserapatch/internal/cli
ok  github.com/tesseracode/tesserapatch/internal/gitutil
ok  github.com/tesseracode/tesserapatch/internal/provider
ok  github.com/tesseracode/tesserapatch/internal/safety
ok  github.com/tesseracode/tesserapatch/internal/store
ok  github.com/tesseracode/tesserapatch/internal/workflow
$ go build ./cmd/tpatch
(clean)
```

## Current State

- All ADR-013 D1–D7 invariants honoured:
  - D1 (no apply-gate behavior change) — `dependency_gate.go`
    untouched; `dependency_gate_freshness_test.go` pins the contract.
  - D2 (apply gate ignores Verify) — same.
  - D3 (recipe-touching amend invalidates) — pre/post byte compare in
    `amendCmd` + `clearVerifyForAmend`.
  - D4 (no persistence of freshness labels) —
    `slice_b_byte_identity_test.go` enforces.
  - D5 (purity at read time) — `deriveFreshnessLabel` is read-only.
  - D6 (no reconcile-session.json reads) — confirmed.
  - D7 (deterministic) — single-label-per-feature contract pinned by
    truth-table tests.

## Next Steps

1. Reviewer runs the standard checklist (build/test/format) and the
   ADR-013 D1–D7 invariants against the diff.
2. Confirm `assets_test.go` parity guard remains passing (no
   skill-asset changes in this slice — should be untouched).
3. On APPROVED: archive this entry to HISTORY.md, flip Slice B in
   `docs/milestones/M15.md` and PRD §9 Slice B row.

## Blockers

None.

## Context for Next Agent

- `composeLabelsFromStatus` is the single source of truth for
  read-time labels. All freshness derivation flows through it.
- For features where the M14.3 path returns nil (retired children via
  `ReconcileUpstreamed`), freshness is also suppressed — this is
  intentional and locked by `TestComposeLabels_UpstreamedChild_NoLabels`.
- The amend `--state` flag currently accepts NO values; widening to
  permit specific lifecycle transitions is a future task. The flag
  exists today purely to surface a clean exit-2 error.
- The `clearVerifyForAmend` helper sets `Verify = nil` (truthful
  `never-verified` derivation), not `Passed = false`. ADR-013 D3
  describes the latter; the chosen implementation honours the same
  invariant (no stale "verified" claim) while keeping the
  producer-set rule clean (verify is the only producer of a non-nil
  record).
