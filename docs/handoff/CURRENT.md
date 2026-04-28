# Current Handoff

## Active Task

- **Task ID**: M15-W3-SLICE-B
- **Milestone**: M15 Wave 3 — Verify freshness overlay
- **Description**: Implement Slice B — freshness derivation + label
  integration. Extend `ReconcileLabel` vocabulary, derive the four
  freshness labels in `composeLabelsFromStatus`, render them inline in
  `tpatch status` and `tpatch status --dag`, emit the `Verify`
  sub-record and `freshness_label` from `tpatch status --json`,
  invalidate `Verify.Passed` on recipe-touching `amend`, and reject
  `amend --state tested` with exit 2.
- **Status**: Not Started — staged for dispatch
- **Assigned**: 2026-04-27

## Slice A retrospective (just shipped)

External supervisor APPROVED WITH NOTES at commit `bce2252`. Note was
doc-only (stale V2/V3 wording in earlier CURRENT.md sections); resolved
by archiving to HISTORY.md. Slice A is push-ready.

The shipped contract is recorded in PRD-verify-freshness.md §9 Slice A
row; the implementation lives in `internal/cli/verify.go`,
`internal/workflow/verify.go`, the `Verify` sub-record on
`FeatureStatus`, and `Store.WriteVerifyRecord`. ADR-013 D1-D7 are all
honoured.

## Slice B scope (binding — from PRD-verify-freshness.md §9)

1. Extend `ReconcileLabel` enum with:
   - `LabelNeverVerified`
   - `LabelVerifiedFresh`
   - `LabelVerifiedStale`
   - `LabelVerifyFailed`
2. Extend `composeLabelsFromStatus` (`internal/workflow/labels.go:143`)
   to derive these four labels per the truth table in §3.4.2. Pure
   function; no writes; no read-path mutation (D5).
3. `tpatch status` and `tpatch status --dag` render the freshness label
   inline (e.g. `applied [verified-fresh]`).
4. `tpatch status --json` emits the `Verify` sub-record when present
   and the derived `freshness_label` in the labels array.
5. `tpatch amend` (recipe-touching change) invalidates `Verify.Passed`
   per ADR-013 D3. Recipe-untouching amend preserves freshness.
6. `tpatch amend --state tested` rejected with exit 2 (`no such state`).
7. Tests:
   - Derivation truth-table per §3.4.2 (matrix coverage of
     verified_at presence × hash match × parent_snapshot match × passed).
   - v0.6.1 round-trip byte-identity (no `Verify` set → no derivation
     → existing label set unchanged).
   - Apply-gate test pinning that freshness does NOT extend the
     satisfaction set (D2 invariant).

## Out of scope for Slice B

- V3-V9 real implementations (Slice C).
- Closure replay (Slice C).
- `tpatch verify --all` (Slice D).
- Skill bullets / parity-guard anchor regen (Slice D).
- Any new flag on `verify`.

## Reviewer-note carry-overs

None blocking. The three reviewer notes from the redesign approval at
`3c122aa` were closed in Slice A. Continue honouring D5 (no read-path
mutation): label derivation runs at read time but writes nothing.

## Files Changed

(none yet — staged)

## Test Results

Slice A stack last validation gate: gofmt clean, `go test ./...` all
pass, `go build ./cmd/tpatch` clean. To be re-run after Slice B lands.

## Next Steps

1. Push Slice A stack to `origin/main` (excluding untracked
   `docs/whitepapers/` and exploratory PRDs per supervisor scope note).
2. Dispatch Slice B implementer with the scope above.
3. Run sub-agent reviewer cycle until APPROVED.
4. External supervisor pass.
5. After Slice B approved + pushed, Slice C (V3-V9 + closure replay).

## Blockers

None.

## Context for Next Agent

- Slice B is a **pure-read derivation pass** — `composeLabelsFromStatus`
  is the central point. The function already composes lifecycle labels
  (`applied` / `upstream_merged` / `reconcile-needed`); the new
  freshness labels compose alongside, not in place.
- D2 binding: the existing apply-gate satisfaction set
  (lifecycle states `applied` and `upstream_merged`) does NOT change.
  If a Slice B test ever asserts that `verified-fresh` satisfies a
  hard dep, that test is wrong — flag it.
- D5 binding: `composeLabelsFromStatus` is a read path. It must NOT
  call any writer (no `WriteVerifyRecord`, no `SaveFeatureStatus`).
- D6 binding: freshness derivation reads only the persisted `Verify`
  sub-record on `FeatureStatus`. It must NOT read
  `artifacts/reconcile-session.json` or any patch / recipe content.
- The truth-table derivation is the most error-prone surface. Build
  it as a table-driven test with one row per cell of §3.4.2 — easier
  to extend in Slice C when V3-V9 results feed in.
- `tpatch amend` recipe-touching invalidation: confirm with the
  existing amend implementation whether the recipe-bytes change is
  detected by hash diff (compute new `recipe_hash_at_verify` and
  compare to persisted) or by an explicit code path on the amend
  verb. ADR-013 D3 picks the producer-set rule; do whatever D3 says.
- Co-author trailer required on every commit:
  `Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>`.
