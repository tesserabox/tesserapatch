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
5. `tpatch amend`: detects recipe-touching by EITHER (a) comparing
   `apply-recipe.json` bytes pre/post the amend invocation OR (b)
   comparing the on-disk recipe sha256 against the persisted
   `Verify.RecipeHashAtVerify` (catches external edits between
   `tpatch verify` and `tpatch amend`). When either trigger fires,
   `clearVerifyForAmend` sets `Verify = nil`. Producer-set rule per
   ADR-013 D3: amend asserts authorship; if the recipe drifted from
   what Verify recorded, Verify is no longer authoritative. The OR
   logic was added in revision-1 (`53a4d9a`) after the external
   supervisor reproduced live Case C against the original pre/post-only
   path. `--state` flag added, wired solely to reject any value
   (notably `tested`) with `*ExitCodeError{Code:2}` per PRD §9.

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
  - D3 (recipe-touching amend invalidates) — OR-condition in
    `c1.go:235`: pre/post recipe bytes differ within the amend
    invocation, OR the on-disk recipe sha256 differs from the
    persisted `Verify.RecipeHashAtVerify` (`recipeDiffersFromVerify`,
    `c1.go:295`). Either trigger calls `clearVerifyForAmend`.
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

---

## Bug-fix in flight: record --files compatibility

### Summary

Lifted the artificial `--files` + `--from` rejection in `record` and
extended the committed-range capture surface so the headline use case
(interleaved commits across multiple features on the same branch,
scoped to specific paths) works without manual `git diff` fallback.

### Changes

- `internal/gitutil/gitutil.go`: added
  `CapturePatchFromCommitsScoped(repoRoot, fromRef, toRef, pathspecs)`.
  The legacy `CapturePatchFromCommits` is now a thin wrapper that
  delegates with `nil` pathspecs (byte-for-byte identical output for
  existing callers — pinned by
  `TestCapturePatchFromCommits_DefaultMatchesScoped`). Excludes come
  before user pathspecs (mirrors `CapturePatchScoped`). Comment block
  documents why committed-range capture intentionally never consults
  `git ls-files --others`.
- `internal/cli/cobra.go`: removed the `--files` + `--from` rejection;
  added `--to <ref>` (defaults to `HEAD`, requires `--from`) and
  `--commit-range <a>..<b>` (mutually exclusive with `--from`/`--to`,
  parsed via `strings.SplitN(value, "..", 2)`). Help text restructured
  in `9096d04` to lead with the committed-range modes (`--from` and
  `--commit-range`) per the headline-first requirement; working-tree
  default falls below.
- Tests:
  - `internal/gitutil/capture_from_commits_scoped_test.go` (new):
    `_FilesScoping`, `_ToRefCaps`, `_ExcludesArtifacts`,
    `_DefaultMatchesScoped` (backwards-compat byte-for-byte pin).
  - `internal/cli/record_range_scoped_test.go` (new):
    `TestRecordCmd_FromAndFiles_Compatible`,
    `TestRecordCmd_CommitRangeAndFiles_Compatible`,
    `TestRecordCmd_ToRefCaps`,
    `TestRecordCmd_CommitRange_RejectsWithFrom`,
    `TestRecordCmd_CommitRange_RejectsWithTo`,
    `TestRecordCmd_To_RequiresFrom` (added in `9096d04` to cover the
    explicit "--to without --from" rejection),
    `TestRecordCmd_WorkingTreeFilesUnchanged`.
  - `internal/cli/cobra_test.go`: removed obsolete
    `TestRecordFilesIncompatibleWithFrom` (replaced with a comment
    pointing to the new compat test).

### External supervisor verdict

The orthogonal record bug-fix stack (`9e96b38` + `9096d04`) was
reviewed by the external supervisor as a separate pass and APPROVED.
Live CLI repro confirmed: `record --from <base> --files <path>`
produced a patch containing only the scoped path; `--to` without
`--from` rejected with the intended error. Stack is orthogonal to
verify/freshness/amend/status — no code-level interference. The only
finding was handoff drift, addressed in this commit.

### Validation

- `gofmt -l .` → empty
- `go build ./cmd/tpatch` → clean
- `go test ./...` → all packages pass

### Out of scope (untouched)

- Skill bullets and `SPEC.md` updates — tracked separately as
  `doc-skills-record-flags`.
- Slice B / verify / freshness / labels code — orthogonal.

### Non-obvious decisions

- Chose the additive API (`CapturePatchFromCommitsScoped` + thin
  wrapper) over a signature change to keep the byte-for-byte
  backwards-compat guarantee trivial to prove (single test:
  `legacy == scoped(nil)`).
- `--to` without `--from` is rejected with a clear error rather than
  silently defaulting `fromRef` — avoids ambiguity about whether
  "diff working tree against `<ref>`" was intended.
- `--commit-range` parsing rejects empty halves (e.g. `..HEAD` or
  `abc..`) for the same reason — better an early clear error than a
  surprising `git diff` failure later.
