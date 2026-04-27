# Current Handoff

## Active Task

- **Task ID**: M15-W3-REDESIGN
- **Milestone**: M15 → Wave 3 (lifecycle / reconcile semantics tranche) — **redesign in flight**
- **Description**: Re-review the freshness-overlay design package (PRD-verify-freshness.md + ADR-013) before any Slice A code dispatch. This is a design supersession of the v0.6.1-era tested-as-state model.
- **Status**: In Progress — design package landed, awaiting reviewer pass
- **Assigned**: 2026-04-27

## Why Wave 3 was reopened

An external re-review of the approved Wave 3 design (commit `8c3d72e`) identified two structural problems that survived the prior implementer/reviewer cycle:

- **F1**: V7/V8 shadow replay ignored the hard-parent topological closure, so verify would have been structurally meaningless for any non-leaf feature with a locally-`applied` parent.
- **F4**: The design conflated lifecycle (sticky, write-by-explicit-verb) with verification freshness (drift-sensitive), routing a "parent-state hook" through `LoadFeatureStatus`. That would have meant `tpatch status` silently mutates `.tpatch/`.

Plus two CURRENT.md drift findings (F2: invented `Tested *TestedRecord` field; F3: Slice A boundary misaligned).

The supervisor's binding adjudication: redesign with **Git-like semantics**. Lifecycle stays the lifecycle. Verification becomes a derived freshness overlay with a small persisted record. Read paths never mutate state.

## What landed in the redesign package

- **`docs/prds/PRD-verify-freshness.md`** (new, ~687 lines) — successor PRD. Freshness-overlay model, V7/V8 closure-replay spec, four derived labels, five JSON examples, four corrected slice boundaries.
- **`docs/adrs/ADR-013-verify-freshness-overlay.md`** (new, ~289 lines) — successor ADR. D1–D7 in the rewritten order. Includes a **supersession map** of every prior D1–D7 disposition: D1 REPLACED, D2 DROPPED, D3 REPLACED (mostly retained), D4 RETAINED, D5 DROPPED (no transitions), D6 RETAINED, D7 RETAINED + EXTENDED.
- **`docs/prds/PRD-verify-and-tested-state.md`** — predecessor PRD, SUPERSEDED banner added; preserved as historical record.
- **`docs/adrs/ADR-012-feature-tested-state.md`** — predecessor ADR, SUPERSEDED banner added; preserved as historical record.
- **`docs/handoff/HISTORY.md`** — top entry archives the prior idle CURRENT.md and the reopening rationale.
- **`docs/supervisor/LOG.md`** — top entry records the reopening + the binding non-negotiables for the redesign.

## Locked design contract (ADR-013, binding for all Wave 3 code)

- **D1** — `verify` writes a `Verify` sub-record on `FeatureStatus`. `FeatureState` enum unchanged. No new lifecycle state.
- **D2** — apply gate is pure-lifecycle. Satisfaction set remains `{applied, upstream_merged}`. Freshness is a harness signal, not a gate input.
- **D3** — `verify` writes the freshness record; `amend` invalidates by clearing it; `test` does not write.
- **D4** — `Verify` sub-record carries `omitempty` on every nested field; v0.6.1 repos round-trip byte-identical until verify runs once.
- **D5** — derived label transitions only: `never-verified` / `verified-fresh` / `verified-stale` / `verify-failed`, recomputed at read time in `ComposeLabels`. No persisted transitions.
- **D6** — `Verify` lives in `status.json`; never inferred from `artifacts/reconcile-session.json`. Reuses ADR-011 D6 source-truth guard.
- **D7** — `verify` is read-only on the working tree; shadow simulation includes hard-parent topological closure replay (the F1 fix).

## Pre-revision adjudications still binding (Q1–Q5)

- **Q1**: V9 severity = warn (default).
- **Q2**: `verify --all` skips pre-apply slugs with `"skipped: pre-apply state"` reason line.
- **Q3**: `passed: false` field name retained.
- **Q4**: SUPERSEDED by F4. The "does tested satisfy hard deps" question is moot because there is no `tested` lifecycle state.
- **Q5**: parent-state hook becomes pure read-time label recomputation in `ComposeLabels` (not a writer). Resolved by F4.

## Files Changed (this redesign pass)

- `docs/prds/PRD-verify-freshness.md` (created)
- `docs/adrs/ADR-013-verify-freshness-overlay.md` (created)
- `docs/prds/PRD-verify-and-tested-state.md` (SUPERSEDED banner added)
- `docs/adrs/ADR-012-feature-tested-state.md` (SUPERSEDED banner added)
- `docs/handoff/CURRENT.md` (this file — rewritten for the active redesign)
- `docs/handoff/HISTORY.md` (top-entry archive of the superseded design + idle CURRENT)
- `docs/supervisor/LOG.md` (top-entry reopening note)

## Test Results

N/A — design-only. The next code dispatch (Slice A, gated on this redesign's approval) will run the standard `go test ./... && go build ./cmd/tpatch && gofmt -l .` gate.

## Next Steps

1. **Reviewer dispatch** — `m15-w3-redesign-reviewer` (`code-review` agent, background). Focus areas:
   - Internal consistency of PRD ↔ ADR-013 (especially D1, D5, D7 + the closure-replay spec).
   - Adherence to the binding non-negotiables (lifecycle untouched, no read-path mutation, apply gate stays pure-lifecycle, freshness record minimal).
   - Supersession-map completeness: every old D1–D7 has a clear retained / replaced / dropped disposition with reasoning.
   - Slice boundaries: each of A/B/C/D is independently shippable.
   - Failure-mode coverage: closure-replay JSON shape, parent-snapshot derivation, amend-invalidation semantics.
2. **Hard gate** — do NOT auto-dispatch Slice A. The user gates on the reviewer verdict.
3. After approval: archive M15-W3-REDESIGN to HISTORY.md, dispatch Slice A implementer with a tight per-slice contract referencing ADR-013 + PRD-verify-freshness.md.

## Blockers

None — the package is review-ready.

## Context for Next Agent

- v0.6.1 is shipped (`origin/main` tag `v0.6.1`, commit `572a038`).
- Wave 3 is in **redesign**, not implementation. Slice A is **deliberately not dispatched.**
- Reading order for any new agent: ADR-013 first (architecture), PRD-verify-freshness.md second (operational detail), HISTORY.md 2026-04-27 entry third (why this shape was chosen).
- Hard rules still binding: ADR-010 D5 (source-truth guard), ADR-011 D6 (status-as-truth), recipe-op JSON schema frozen, `omitempty` round-trip, secret-by-reference, no nested map keys in YAML config.
- The `tpatch` root binary is not gitignored; `rm -f tpatch` after any local `go build`.
- Sub-agent self-reviews are status-only signals. Always run an external review before approving anything non-trivial. The Wave 3 reopening is a textbook example.
