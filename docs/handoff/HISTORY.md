# 2026-04-28 — M15-W3-SLICE-B — APPROVED, shipped to origin/main

**Outcome**: Slice B (freshness derivation + label integration + amend invalidation) landed across two commits. The original (`a07acc7`) was approved by the sub-agent reviewer but flagged HIGH by the external supervisor: the recipe-touching amend invalidation contract (ADR-013 D3) was effectively dead at the CLI level because the pre/post bytes compare in `c1.go` could never trigger (no amend code path rewrites `apply-recipe.json`).

Revision-1 (`53a4d9a`) added an OR-condition to amend's invalidation logic: clear `Verify` if EITHER pre/post bytes differ within the amend invocation OR the on-disk recipe sha256 differs from the persisted `Verify.RecipeHashAtVerify` (`recipeDiffersFromVerify` helper at `c1.go:295`). Replaced the helper-only test with a real CLI-level regression that runs `amendCmd` via the cobra root (the supervisor's exact Case C reproduction). Live Case C now passes: BEFORE the fix the amend command exited 0 but Verify was preserved against a drifted recipe; AFTER the fix Verify is cleared as ADR-013 D3 mandates.

External supervisor's second pass on revision-1: APPROVED.

In parallel, an orthogonal record bug-fix stack landed at `9e96b38` + `9096d04`: lifted the artificial `--files` + `--from` rejection, added `--to <ref>` and `--commit-range <a>..<b>` flags, added `CapturePatchFromCommitsScoped` (with thin-wrapper byte-identity guarantee for `CapturePatchFromCommits`), and reordered help text to lead with the committed-range modes. External supervisor's pass: APPROVED.

Final push to origin/main on `1032cda`: `a07acc7` + `9e96b38` + `9096d04` + `53a4d9a` + a docs-only handoff alignment commit.

Slice B is now ✅ in `docs/ROADMAP.md`. Slice C (V3–V9 real implementations + closure replay) is the next active task.

---

# 2026-04-27 — M15-W3-REDESIGN — design package APPROVED WITH NOTES, archived for Slice A dispatch

**Outcome**: The freshness-overlay redesign package (PRD-verify-freshness.md + ADR-013) shipped at commit `37a483d` and was reviewed at commit `3c122aa`. Reviewer verdict: **APPROVED WITH NOTES**. Three advisory notes (CheckResults persistence bloat, Note 2 absent-recipe clarity, Note 3 parity-guard handling) are recorded in the top entry of `docs/supervisor/LOG.md` and now bind the Slice A implementer.

The handoff superseded by this archive is the active-redesign CURRENT.md that read "design package landed, awaiting reviewer pass." Wave 3 has now moved past the design phase. Slice A is the active task.

## Snapshot of the archived CURRENT.md (M15-W3-REDESIGN, redesign-active)

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
# 2026-04-27 — M15-W3-DESIGN — Wave 3 design REOPENED + SUPERSEDED

**Outcome**: The previously approved Wave 3 design (commits `fdc6e70` + `90375c9` + `e6473ea` + `8c3d72e`) is **SUPERSEDED**. An external re-review of `8c3d72e` identified two structural problems (F1: verify shadows ignored hard-parent closure replay; F4: lifecycle/freshness conflation routed read-path mutation through `LoadFeatureStatus`) plus two CURRENT.md drift findings (F2: invented `Tested *TestedRecord` field; F3: Slice A boundary misaligned). The supervisor reopened Wave 3 with a binding redesign: a Git-like freshness overlay model.

## Successor design (active as of this archive)

- **PRD**: `docs/prds/PRD-verify-freshness.md` (new file, supersedes `PRD-verify-and-tested-state.md`).
- **ADR**: `docs/adrs/ADR-013-verify-freshness-overlay.md` (new file, supersedes ADR-012 in full).
- **Predecessor docs preserved**: `PRD-verify-and-tested-state.md` and `ADR-012-feature-tested-state.md` carry SUPERSEDED banners pointing to the successors. They remain in the tree as historical record.

## Why supersession (not silent in-place revision)

The first-revision pass (`e6473ea`) corrected an internal contradiction inside an approved design. The second pass changes the load-bearing model itself: `tested` was a lifecycle state; under the redesign it does not exist as a state. Mutating ADR-012 into the opposite of what was approved would have erased the audit trail. New successor files preserve "this is what we approved at v0.6.1, this is what we adopted instead, this is why."

## Findings the redesign addresses

- **F1 (CRITICAL)** — V7/V8 shadow now replays the target's hard-parent topological closure (ordered by `store.TopologicalOrder` over the hard-only sub-DAG; `upstream_merged` parents skipped; fail-fast with `failed_at: "parent-replay"` on first non-replayable parent or replay failure) before applying the target's recipe.
- **F2 (HIGH)** — invented `Tested *TestedRecord` field replaced by `Verify` sub-record on `FeatureStatus`, locked in ADR-013 D1 with `omitempty`-marshalled fields.
- **F3 (MEDIUM)** — Slice A scope corrected: cobra command shell + V0–V2 + freshness writer skeleton. No `--all`, no `--shadow`, no skill anchor regen in Slice A.
- **F4 (CRITICAL)** — lifecycle and freshness fully separated. `FeatureState` enum unchanged. `verify` writes a freshness overlay; parent regressions produce derived stale labels at read time only; no read path mutates `.tpatch/`.

## Implementer / reviewer / revision timeline

- `fdc6e70` — first design implementer: PRD + ADR-012 (lifecycle-state model).
- `90375c9` — first reviewer: NEEDS REVISION on D2 PRD/ADR contradiction.
- `e6473ea` — first revision: PRD §3.4.4 aligned with ADR-012 D2.
- `8c3d72e` — supervisor approval, archive, idle.
- External re-review (user-mediated, 2026-04-27): findings F1–F4.
- `m15-w3-design-revision-2` (background sub-agent): rewrote PRD + ADR-012 + CURRENT.md in place (commit `e8fde60`, locally only).
- Supervisor reorganization (this commit): commit `e8fde60` replayed into successor file structure (preserves audit trail per user's supersession brief). New PRD-verify-freshness.md + ADR-013, originals carry SUPERSEDED banners, prior idle CURRENT archived here.

## Process lessons reinforced

- **Implementer self-reviews remain status-only signals.** The first reviewer caught one finding; the external re-review caught four. Sub-agent verdicts are inputs to supervisor judgement, never approval signals.
- **Audit trail beats silent rewrites.** Replacing an approved ADR/PRD in place can read as "the design was always this" to a future agent. Successor files with explicit supersession banners + a supersession map preserve "this was the trade-off we examined and rejected."
- **Read paths must not mutate persisted state.** This is now an explicit binding constraint on every Wave 3 design choice (ADR-013 D5).

## Idle CURRENT.md state being archived

The CURRENT.md from `8c3d72e` (idle, claiming the design was approved) is preserved verbatim below for historical record. It contained two drift errors (F2 + F3) that contributed to the reopening.

```
# Current Handoff

## Active Task

- **Task ID**: _idle — M15 Wave 3 design APPROVED, awaiting Slice A code dispatch_
- **Milestone**: M15 → Wave 3 (lifecycle / reconcile semantics tranche)
- **Status**: Idle
- **Assigned**: 2026-04-27

## Session Summary

M15-W3-DESIGN approved after one revision cycle. PRD + ADR-012 locked; archived to `docs/handoff/HISTORY.md` (top entry, 2026-04-27).

The design covers `feat-verify-command` + `feat-feature-tested-state` in a single combined PRD because the two share contract surface — most notably D2 (does `tested` satisfy hard dependencies?), which is now locked: **yes, `tested` is a strict superset of `applied`**.

The PRD slices the work into four independently-dispatchable code waves (Slice A: verify command shell; Slice B: tested state plumbing; Slice C: verify produces tested; Slice D: --all / JSON / docs). Slice A is the next dispatch.

## Locked design contract (binding for all Wave 3 code dispatches)

- **D1**: `tested` is a linear forward state from `applied`. Single-direction extension to `FeatureState` enum.
- **D2**: `tested` satisfies the hard-dep gate. Implementation is one switch arm: extend `case StateApplied:` in `internal/workflow/dependency_gate.go:79–101` to also match `StateTested`.
- **D3**: `verify` is the sole producer of `tested` in v0.6.2. `test` is unchanged; `amend` does not produce `tested`.
- **D4**: New `Tested *TestedRecord` field on the feature status block carries `omitempty` so v0.6.1 repos round-trip byte-identical until verify is run.
- **D5**: Transitions: `applied + verify PASS → tested`; `tested + verify PASS → tested` (idempotent); `tested + verify FAIL (block-severity) → applied`; `tested + amend (recipe-touching) → applied`; `tested + amend (intent-only) → tested` (preserved). Demotion does NOT cascade to children.
- **D6**: `tested` lives in `status.json`. Never inferred from `artifacts/reconcile-session.json`. Reuses ADR-011 D6 source-truth guard verbatim.
- **D7**: `verify` is read-only on the working tree. Apply-simulation uses the existing shadow workspace plumbing.

## Reviewer adjudications (binding inputs to Slice A's contract)

- **Q1 (V9 severity)**: warn (default).
- **Q2 (`verify --all` skip)**: pre-apply slugs are skipped with a `"skipped: pre-apply state"` reason line in the JSON output, not a failure.
- **Q3 (`passed` field name)**: retained. `severity` carries gating; `passed` carries pass/fail intent.
- **Q4 (D2 wording)**: resolved by `e6473ea` revision pass.
- **Q5 (parent-state hook)**: inserted into the existing M14.3 label-recomputation loop. No new hot path.

## Files Changed

_No active task; nothing pending._

Last work: see `docs/handoff/HISTORY.md` 2026-04-27 entry for the full design dispatch + revision archive (commits `fdc6e70`, `90375c9`, `e6473ea`).

## Test Results

N/A — design-only phase. The next code dispatch (Slice A) will run the standard `go test ./... && go build ./cmd/tpatch && gofmt -l .` gate.

## Next Steps

1. **Refresh backlog mirror** to reflect Slice A as the next active code item:
   ```
   chmod 644 .tpatch-backlog/backlog.db
   sqlite3 $SESSION_DB ".backup '.tpatch-backlog/backlog.db'"
   chmod 444 .tpatch-backlog/backlog.db
   ```
2. **Dispatch `m15-w3-slice-a-implementer`** (general-purpose, background) with a tight per-slice contract:
   - **Scope**: verify command shell — register `tpatch verify <slug>` cobra command + `--json`, `--all`, `--shadow` flags + skeleton check runner that returns the new `VerifyReport` struct shape from PRD §4.2. Implement V0–V2 (cheap structural checks: spec.md present, exploration.md targets exist, recipe parses). Stub V3–V9 with TODO + clean-up sentinel.
   - **Out of scope for Slice A**: the actual `tested` state plumbing (Slice B), recipe re-apply against shadow (Slice C), `--all` orchestration (Slice D).
   - **Constraints**: PRD §4.2 JSON shape is binding; cobra wiring follows the existing `applyCmd` / `recordCmd` pattern; skill anchors must be regenerated to mention `verify` (parity guard will fail otherwise).
3. **Wait for completion**, dispatch `m15-w3-slice-a-reviewer` (`code-review` agent), then user gate before Slice B.

## Blockers

None.

## Context for Next Agent

- v0.6.1 is shipped on `origin/main` (tag `v0.6.1`, commit `572a038`). Wave 3 design commits (`fdc6e70`, `90375c9`, `e6473ea`) are committed locally and pushed. The current `main` HEAD is the supervisor approval of the revision pass.
- Authoritative design surface: `docs/prds/PRD-verify-and-tested-state.md` and `docs/adrs/ADR-012-feature-tested-state.md`. Read both before dispatching Slice A. Supplement with `docs/handoff/HISTORY.md` 2026-04-27 entry for the why-this-was-locked-this-way context and reviewer adjudications.
- Hard rules that still hold: ADR-010 D5 (source-truth guard), ADR-011 D6 (status-as-truth), recipe-op JSON schema frozen (no `delete-file` op), `omitempty` round-trip invariant, secret-by-reference, no nested map keys in YAML config (per stored memory).
- The `tpatch` root binary is not gitignored; `rm -f tpatch` after any local `go build`.
- Sub-agent self-reviews remain status-only signals. Always run an external review before approving anything non-trivial.

```


# 2026-04-27 — M15-W3-DESIGN — Wave 3 design (PRD + ADR-012) — APPROVED

**Outcome**: APPROVED after one revision cycle. Design is locked; ready for Slice A code dispatch.

## Deliverables

- `docs/prds/PRD-verify-and-tested-state.md` (~678 lines) — combined PRD covering `feat-verify-command` and `feat-feature-tested-state`. 10-check verify sequence (V0–V9), full state-transition truth table, JSON schema with 3 failure-case examples, 4 independently-dispatchable implementation slices, explicit out-of-scope cross-links to `feat-reconcile-code-presence-verdicts`, `feat-reconcile-fresh-branch-mode`, `delete-file` recipe op.
- `docs/adrs/ADR-012-feature-tested-state.md` (~201 lines) — locks D1–D7 with alternatives-considered. D2 (the consequential decision): `tested` satisfies the hard-dep gate, equivalent to `applied`. Cross-references ADR-010 D5 (source-truth guard) and ADR-011 (dep DAG); does not amend either.

## Key decisions locked

- **D1**: `tested` is a linear forward state from `applied`, not a parallel branch.
- **D2**: `tested` satisfies the hard-dep gate. The `case StateApplied:` arm in `internal/workflow/dependency_gate.go:79–101` extends to also match `StateTested`. `tested` is a strict superset of `applied`.
- **D3**: `verify` is the sole producer of `tested` in v0.6.2. `test` does not produce `tested` (separation of concerns).
- **D4**: v0.6.1 repos that never run verify keep status.json byte-identical via `omitempty` on the new field.
- **D5**: Forward/backward transitions table:
  - `applied + verify PASS → tested`
  - `tested + verify PASS → tested` (idempotent)
  - `tested + verify FAIL (block-severity) → applied`
  - `tested + amend (recipe-touching) → applied`
  - `tested + amend (intent-only) → tested` (preserved)
  - `tested → applied` demotion does NOT cascade to children
- **D6**: `tested` is persisted in `status.json`, never inferred from `artifacts/reconcile-session.json`. Reuses ADR-011 D6 wording verbatim.
- **D7**: `verify` is read-only on the working tree; uses shadow workspace for apply-simulation.

## Reviewer adjudications (binding inputs to Slice A)

- **Q1 (V9 severity)**: warn (default). Block would penalize features in `shadow-awaiting`, which is a pending human decision, not a structural integrity problem.
- **Q2 (`verify --all` skip)**: skip pre-apply slugs with a `"skipped: pre-apply state"` reason line in the JSON output.
- **Q3 (`passed: false` field name)**: keep `passed` (semantically accurate; `severity` carries gating).
- **Q4 (D2 wording)**: PRD §3.4.4 rewritten to align with ADR-012 D2 (Direction A, chosen; Direction B preserved as rejected alternative). Resolved by the revision pass.
- **Q5 (parent-state hook insertion)**: insert into the existing M14.3 label-recomputation loop. No new hot path.

## Process timeline

1. **`fdc6e70`** — implementer landed PRD + ADR-012. Implementer surfaced 5 open questions for the reviewer to adjudicate.
2. **`90375c9`** — `m15-w3-design-reviewer` (code-review sub-agent) verdict: **NEEDS REVISION**. One blocking finding: PRD §3.4.4 line 263 stated "Direction B (tested does NOT satisfy)" while ADR-012 D2 line 44 locked the opposite. PRD then walked back into "B-pragmatic" framing that implemented Direction A. Editorial misalignment, not a design flaw.
3. **`e6473ea`** — revision implementer rewrote PRD §3.4.4 only. Headline now plainly Direction A; Direction B preserved as labelled rejected alternative; ADR-012 D2 cited as locking record. 17 inserts / 18 deletes in PRD, plus reviewer-adjudication block in CURRENT.md.
4. Supervisor approved revision directly (no second sub-agent review): mechanical fix, single section, scope-bounded.

## Process lessons reinforced

- Single-finding sub-agent reviews remain a strong pattern: targeted, fast, auditable. The `code-review` agent identified a real PRD/ADR contradiction the implementer would not have surfaced solo.
- **Implementer self-reviews are status-only, never approval signals** (v0.6.1 fix-pass lesson holds): the original implementer call did not flag the D2 contradiction it had created.
- Combined PRD over two split PRDs paid off: the consequential D2 decision had to be answered exactly once, and the contradiction was localised to one section instead of needing cross-document reconciliation.

## Files changed (commits `fdc6e70`, `90375c9`, `e6473ea`)

- `docs/prds/PRD-verify-and-tested-state.md` — created (`fdc6e70`), revised §3.4.4 (`e6473ea`)
- `docs/adrs/ADR-012-feature-tested-state.md` — created (`fdc6e70`)
- `docs/handoff/CURRENT.md` — dispatch contract (`fdc6e70`), reviewer adjudications + revision note (`e6473ea`)
- `docs/supervisor/LOG.md` — reviewer verdict (`90375c9`)


## 2026-04-27 — M15-W2 fix-pass APPROVED, v0.6.1 release prep

**Trigger**: External re-review against the merged M15-W1+W2 surface (HEAD `ad040ac`) surfaced 4 medium findings. Supervisor closed all 4 in-tree before tagging rather than dispatching a separate implementer cycle (changes are tightly coupled and small).

**Fix-pass commit**: `eb92051`.

**Findings (re-reviewer) → fixes**:

1. **F1 — satisfied_by contract drift** (Wave 1 reachability vs Wave 2 gate): validation accepted any reachable ref including unique short SHAs; apply-time gate still rejected anything not 40-hex. Save-now/fail-later path. *Fix*: validation now requires 40-hex AND reachability; new sentinel `ErrSatisfiedByMalformed`; new test `TestValidateDependencies_SatisfiedByMalformed` (4 invalid forms); existing reachability/git-error tests rebased onto 40-hex literals.

2. **F2 — scoped record metadata leak** (Wave 2 `--files`): patch was scoped but `CaptureDiffStat` was unscoped, so `post-apply-diff.txt` and `record.md` still embedded full-tree diffstat. *Fix*: new `gitutil.CaptureDiffStatScoped(repoRoot, pathspecs)`; `CaptureDiffStat` delegates with `nil` (byte-identical default); `recordCmd` calls the scoped variant. Test: `TestCaptureDiffStatScoped_NarrowsToPathspec`.

3. **F3 — invalid pathspec swallowed** (Wave 2 `--files`): `CapturePatchScoped` collapsed any git-diff failure into an empty patch, then `recordCmd` reported "captured 0 bytes". Reviewer reproduced with `:(badmagic)foo`. *Fix*: when pathspecs is non-empty, propagate the wrapped git error; clean up intent-to-add markers on the failure path. Empty pathspecs preserves historical tolerant behavior. Test: `TestCapturePatchScoped_InvalidPathspecSurfacesError`.

4. **F4 — Windows syntax-check quoting** (Wave 2 shell selection): `UserShell` returns `cmd /C` on Windows but `shellQuote` always emitted POSIX single-quote form, leaking quote characters into argv. *Fix*: `shellQuote` → `shellQuoteFor(goos, p)`; Windows uses `"…"` with embedded `"` doubled, Unix retains `'…'` with `'''` escape. Tests: `TestShellQuoteFor` (6 cases) + `TestShellQuoteFor_PairsWithUserShell` invariant guard.

**Validation gate (eb92051)**:
- `gofmt -l .` clean.
- `go build ./cmd/tpatch` clean (root binary removed).
- `go test ./...` clean across all 7 packages.
- Focused `go test ./internal/store -run Validate` — 17/17 pass.

**Files changed (fix-pass)**:
- `internal/store/validation.go` (F1 — regex + sentinel + 40-hex+reachability ordering in both `ValidateDependencies` and `ValidateAllFeatures`)
- `internal/store/validation_test.go` (F1 — new malformed test; reachability tests rebased to 40-hex)
- `internal/gitutil/gitutil.go` (F2 — `CaptureDiffStatScoped` + delegating `CaptureDiffStat`; F3 — wrapped error on scoped diff failure with cleanup)
- `internal/gitutil/capture_scoped_test.go` (F2/F3 — diffstat-narrows test + invalid-pathspec test)
- `internal/cli/cobra.go` (F2 — `recordCmd` uses `CaptureDiffStatScoped`)
- `internal/workflow/validation.go` (F4 — OS-aware `shellQuoteFor`)
- `internal/workflow/shell_quote_test.go` (F4 — new file: matrix + pairing-invariant test)
- `docs/supervisor/LOG.md` (fix-pass entry prepended)
- `docs/handoff/CURRENT.md`

**Lessons / process notes**:

- **Self-review was overconfident.** The original M15-W2 reviewer (sub-agent code-review) returned "APPROVED, zero findings"; the external re-review found 4 medium issues. Treat sub-agent self-reviews as status signals, never as approval signals. Real approval requires an outside read.
- **Wave-1 / Wave-2 contract surfaces interact.** F1 only emerged because Wave 1 hardened validation while Wave 2 left the apply-gate's contract untouched. When two waves touch overlapping value spaces, an explicit contract-alignment audit between waves is cheap insurance.
- **Hookable seams paid off.** The Wave 1 `var isAncestor = gitutil.IsAncestor` pattern made F1's test rebase trivial; the Wave 2 `userShellFor` pattern made F4's pairing-invariant test possible without a Windows runner. Worth keeping as a convention.

**Decision taken**: cut `v0.6.1` immediately after this fix-pass. Wave 1 + Wave 2 + fix-pass form a coherent stabilization release. Wave 3 (verify, tested-state, code-presence verdicts, fresh-branch reconcile) is lifecycle/reconcile semantics and warrants a PRD/ADR pass before dispatch — explicitly NOT bundled into v0.6.1.

**Next**: tag `v0.6.1` (this commit + version bump + CHANGELOG), then queue a Wave 3 design pass starting with `feat-verify-command` (lowest blast radius of the four).

---

## 2026-04-26 — M15-W2 (Wave 2 Path B trio) APPROVED, archiving handoff

**Reviewer verdict**: APPROVED, zero findings (LOG entry `2fb11f5`).

**Final M15-W2 commits**: `e7f524d` (shell selection), `dbd44c2` (recipe autogen + drift detection), `d402653` (--files scoping), `2c5ae33` (impl handoff), `2fb11f5` (review LOG entry).

**5 design judgment calls verified**:
1. JC1 — deleted files skipped+warned; no silent schema extension
2. JC2 — stale-recipe sidecar non-destructive by default; `--regenerate-recipe` explicit
3. JC3 — drift detection file-set based only (documented floor)
4. JC4 — `--files` + `--from` mutual exclusion with explicit pre-side-effect error
5. JC5 — Unix shell behavior byte-identical to historical sh -c

**Critical invariants verified**: recipe-op JSON schema unchanged; ADR-011 D6 source-truth guard preserved; patch remains reconcile authority (no recipe inversion); pathspec injection prevented via `--` separator.

---

# Current Handoff

## Active Task

- **Task ID**: `M15-W2` (Wave 2 — Path B correctness and ergonomics)
- **Milestone**: M15 stream — v0.6.x stabilization
- **Status**: Implementation complete — review pending
- **Assigned**: 2026-04-26

## Session Summary

All four Wave 2 items shipped across three commits:

| SHA | Item |
|---|---|
| `e7f524d` | `bug-test-command-shell-selection` — OS-aware shell helper (`workflow.UserShell`) routes the three former `sh -c` call sites; Unix path byte-identical |
| `dbd44c2` | `feat-record-autogen-recipe` + `bug-recipe-stale-after-manual-flow` — patch-derived autogen of `apply-recipe.json` when missing; `recipe-stale.json` sidecar on drift; `--no-recipe-autogen` opt-out, `--regenerate-recipe` to overwrite |
| `d402653` | `feat-record-scoped-files` — `--files=<pathspec,...>` flag on `tpatch record` with `CapturePatchScoped` helper; default unset preserves full-tree capture byte-for-byte |

## Files Changed

**Item 1 — shell selection**

- `internal/workflow/shell.go` (new)
- `internal/workflow/shell_test.go` (new)
- `internal/workflow/validation.go` (two `sh -c` sites → `UserShell`)
- `internal/cli/phase2.go` (one `sh -c` site → `UserShell`)

**Items 2 + 3 — recipe autogen + stale detection**

- `internal/workflow/recipe_autogen.go` (new) — `RecipeFromPatch`, `AutogenRecipeForRecord`, `RecipeStaleness` sidecar type, file-set drift compare
- `internal/workflow/recipe_autogen_test.go` (new) — 9 tests (parse, generate, skip-if-off, noop, stale-marker, regenerate, clear-on-realign, schema allowlist)
- `internal/cli/cobra.go` (`recordCmd` wiring + new flags)

**Item 4 — scoped capture**

- `internal/gitutil/gitutil.go` (refactor `CapturePatch` → thin wrapper over new `CapturePatchScoped`)
- `internal/gitutil/capture_scoped_test.go` (new) — default parity, narrowing, multi-pathspec
- `internal/cli/cobra.go` (`--files` flag + `--from` clash guard)
- `internal/cli/cobra_test.go` (two integration tests)

## Test Results

```
ok  	github.com/tesseracode/tesserapatch/assets
ok  	github.com/tesseracode/tesserapatch/internal/cli         5.391s
ok  	github.com/tesseracode/tesserapatch/internal/gitutil     4.120s
ok  	github.com/tesseracode/tesserapatch/internal/provider
ok  	github.com/tesseracode/tesserapatch/internal/safety
ok  	github.com/tesseracode/tesserapatch/internal/store       0.534s
ok  	github.com/tesseracode/tesserapatch/internal/workflow    9.720s
```

`gofmt -l .` empty. `go build ./cmd/tpatch && rm -f tpatch` clean. Working
tree clean before push.

## Reviewer Attention Points

- **Recipe schema gap (deletions)**: `RecipeFromPatch` skips deleted files
  with a stderr warning because the recipe-op schema (parity guard)
  defines no `delete-file` op. This is a documented gap, NOT a silent
  schema extension. If reviewer wants delete coverage, that requires a
  schema-extension ADR + parity-guard update — flagged for Wave 3+.
- **Stale resolution (Item 3 design choice)**: when an existing recipe
  drifts from the captured patch, the default action is to write a
  `recipe-stale.json` sidecar and warn — the existing recipe is **not**
  overwritten, because a provider-generated recipe may carry richer
  `replace-in-file` context or `created_by` edges that a patch-derived
  recipe cannot reproduce. `--regenerate-recipe` is the explicit user
  action to overwrite; the sidecar self-clears once the recipe matches
  the captured patch again.
- **Drift detection scope**: file-set comparison only (path inclusion).
  Same files but altered content does not trigger drift. Sufficient for
  the manual-edit-after-implement scenario but a deliberate floor; a
  hash-based deep compare is a follow-up if needed.
- **`--files` + `--from` rejection**: explicit error rather than
  silently-ignored pathspecs. Prevents a misleading "captured nothing"
  outcome.
- **Source-of-truth invariant preserved**: `artifacts/post-apply.patch`
  remains the reconcile authority everywhere. Recipes serve replay /
  inspection only, even after autogen.

## Next Steps

Awaiting reviewer dispatch on M15-W2. Wave 3 holds for supervisor
review pause (verify command, tested state, reconcile semantics).

## Blockers

None.

## Constraints (still valid for next agent)

- Only edit files inside `tpatch/`.
- `tpatch` binary at repo root is **not** gitignored — always
  `rm -f tpatch` after `go build ./cmd/tpatch` BEFORE staging.
- Commit trailer mandatory: `Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>`.
- Source-truth guard (ADR-011 D6): label/status code reads
  `status.Reconcile.Outcome` via `store.LoadFeatureStatus`, never
  `artifacts/reconcile-session.json`.
- Recipe vs patch authority: `artifacts/post-apply.patch` is the
  reconcile source of truth.
- Skill parity guard (`assets/assets_test.go`) — recipe-op schema is
  enforced; the autogen path emits only `write-file` ops to stay
  inside the allowlist.
- `git push` is slow (60+s typical).

## Out of Scope for Wave 2 (still gated for Wave 3)

- `feat-verify-command`, `feat-feature-tested-state`,
  `feat-reconcile-code-presence-verdicts`, `feat-reconcile-fresh-branch-mode`.
- ROADMAP / CHANGELOG / version bumps.

## Context for Next Agent

- `RecipeStaleness` is held as a sidecar (`recipe-stale.json`), not a
  field on `ApplyRecipe`, so the parity guard's
  `DisallowUnknownFields` decoder against skill JSON examples stays
  green without touching the 6 skill assets.
- `userShellFor(goos)` is the testable seam for shell selection — same
  hookable-var pattern Wave 1 used for `gitutil.IsAncestor`.
- The autogen + stale path runs unconditionally after a successful
  patch capture in `recordCmd`, after `MarkFeatureState`. Failures
  inside the autogen helper are reported as warnings on stderr and do
  not fail the record itself.

---

## 2026-04-26 — M15-W1 (Wave 1 polish trio) APPROVED WITH NOTES, archiving handoff

# Current Handoff

## Active Task

- **Task ID**: `M15-W1` (Wave 1: `feat-satisfied-by-reachability`, `chore-skill-frontmatter`, `feat-define-spec-alias`)
- **Milestone**: M15 stream — v0.6.x stabilization and Path B follow-through
- **Status**: Implementation complete — review pending
- **Assigned**: 2026-04-26

## Session Summary

Three Wave 1 polish items landed as three focused commits, each with its own tests.

1. **`aa0f93e` — `feat(validation): verify satisfied_by SHA reachability via git merge-base`**
   Closes the deliberate M14.1 limitation where any well-formed hex string was accepted as `satisfied_by` provenance as long as the parent state was `upstream_merged`. Adds `gitutil.IsAncestor` (exit-code-aware wrapper around `git merge-base --is-ancestor`: exit 0 → reachable, exit 1 → unreachable, otherwise an error). Wires the check into both `ValidateDependencies` and `ValidateAllFeatures`, gated on the parent already being `upstream_merged` (no double-fail with the requires-upstream rule). Introduces a hookable `isAncestor` package var so unit tests stay git-free.

2. **`d5f934f` — `chore(skills): add YAML frontmatter to Copilot + Claude SKILL.md`**
   Prepends a `name: tessera-patch` / `description: …` YAML block to `assets/skills/copilot/tessera-patch/SKILL.md` and `assets/skills/claude/tessera-patch/SKILL.md` so the Copilot CLI / Claude Code skill loaders accept the file. Cursor `.mdc` (already has its own frontmatter format) and Windsurf rules (no frontmatter convention) audited and left as-is. Parity guard untouched — frontmatter doesn't remove any required CLI-command anchor.

3. **`99ee60e` — `feat(cli): add `spec` as an alias for `define`**
   `Aliases: []string{"spec"}` on `defineCmd()`. Alias only — same RunE, same flags, identical semantics. Two new tests (`TestSpecAliasResolvesToDefine`, `TestSpecAliasRunsDefine`). One small parenthetical doc note in `docs/feature-layout.md`. Skills + parity guard left untouched: `tpatch define` remains the canonical anchor.

## Current State

- HEAD `99ee60e`, two commits ahead of origin/main locally pre-push (push happens after this handoff is committed).
- Build clean, full test suite green, parity guard holds.
- ROADMAP unchanged (supervisor handles release/milestone box flips).
- Wave 2 (`bug-test-command-shell-selection`, `feat-record-autogen-recipe`, `bug-recipe-stale-after-manual-flow`, `feat-record-scoped-files`) intentionally NOT started — supervisor dispatches separately after Wave 1 is reviewed.

## Files Changed

- `internal/gitutil/gitutil.go` — new `IsAncestor` helper.
- `internal/gitutil/gitutil_test.go` — `TestIsAncestor` covering reachable / unreachable / bogus-ref.
- `internal/store/validation.go` — new `ErrSatisfiedBySHANotReachable`, `isAncestor` hook, reachability checks in both validators.
- `internal/store/validation_test.go` — `stubIsAncestor` helper, three new tests, existing `…OnUpstreamMerged` test updated to stub.
- `assets/skills/copilot/tessera-patch/SKILL.md` — YAML frontmatter prepended.
- `assets/skills/claude/tessera-patch/SKILL.md` — YAML frontmatter prepended.
- `internal/cli/cobra.go` — `Aliases: []string{"spec"}` on `defineCmd()`.
- `internal/cli/cobra_test.go` — `TestSpecAliasResolvesToDefine` + `TestSpecAliasRunsDefine`.
- `docs/feature-layout.md` — alias parenthetical on the `spec.md` row.

## Test Results

```
ok  github.com/tesseracode/tesserapatch/assets
?   github.com/tesseracode/tesserapatch/cmd/tpatch[no test files]
ok  github.com/tesseracode/tesserapatch/internal/cli
ok  github.com/tesseracode/tesserapatch/internal/gitutil
ok  github.com/tesseracode/tesserapatch/internal/provider
ok  github.com/tesseracode/tesserapatch/internal/safety
ok  github.com/tesseracode/tesserapatch/internal/store
ok  github.com/tesseracode/tesserapatch/internal/workflow
```

`gofmt -l .` clean. `go build ./cmd/tpatch` succeeded; root binary removed.

## Next Steps

Awaiting reviewer dispatch on M15-W1. Wave 2 holds until M15-W1 APPROVED.

## Blockers

None.

## Context for Next Agent

- **Reachability check is gated on `parent.State == StateUpstreamMerged`.** This is intentional: when the parent is in any other state, `ErrSatisfiedByRequiresUpstream` already fires and the reachability rule would just produce a noisier double-error. ADR-011 D5 still holds — `satisfied_by` is provenance metadata; runtime semantics are unchanged.
- **`isAncestor` is a package-level `var` hook in `internal/store`.** Tests stub it via `stubIsAncestor(t, ok, err)` which restores via `t.Cleanup`. If a future test creates a real git repo and wants the live behavior, just don't call the stub.
- **The `gitutil.IsAncestor` failure path returns `(false, err)` only for non-zero, non-1 exits** (e.g., bogus SHA, repo missing). Callers must NOT treat the error as "unreachable" — they may want to surface it as a configuration problem.
- **`spec` is alias-only.** Do not bulk-rewrite skills/docs to mention it — `tpatch define` remains the canonical CLI-command anchor enforced by the parity guard. The doc touch in `docs/feature-layout.md` is a single parenthetical and intentionally minimal.
- **Frontmatter prepend used only `name` + `description`.** No `globs`, no `alwaysApply` — Copilot/Claude loaders require frontmatter but don't consume those Cursor-specific keys, and adding them would be cargo-cult. Cursor's existing `.mdc` keeps its own keys.
- **`tpatch` binary at the repo root is NOT gitignored.** Bare `tpatch` ignore would shadow `cmd/tpatch/`. Always `rm -f tpatch` after `go build ./cmd/tpatch` BEFORE staging.
- **Commit trailer mandatory**: `Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>`. Use `git -c commit.gpgsign=false`.

---

**Reviewer follow-up (commit `57bf1ab`)**: Added `TestValidateDependencies_SatisfiedByGitError` and `TestValidateAllFeatures_SatisfiedByGitError` to pin the validation-layer contract that real git errors stay distinct from unreachable-SHA errors and surface dependent-slug + parent-slug + underlying error context. Closes the M15-W1 reviewer's medium finding.

**Final M15-W1 commits**: `aa0f93e` (reachability), `d5f934f` (skill frontmatter), `99ee60e` (spec alias), `192935b` (impl handoff), `76fcfef` (review LOG entry), `57bf1ab` (review-note follow-up tests).

## 2026-04-26 — M15.1 created_by auto-inference APPROVED, archiving handoff

# Current Handoff

## Active Task

- **Task ID**: M15.1 — `created_by` auto-inference at implement time (PRD §4.3.1)
- **Milestone**: M15 — v0.6.x stabilization & polish (post-Tranche-D)
- **Status**: Implementation complete — awaiting reviewer
- **Assigned**: 2026-04-26
- **Estimated size**: ~120–180 LOC + tests; one logical commit

## Why this is next

v0.6.0 just shipped `created_by` as a real apply-time gate (M14.2 schema + correctness pass + C5 fix-pass). First-time users will hit `ErrPathCreatedByParent` when their recipe omits the annotation. The PRD already specified an advisory inference heuristic at implement time (§4.3.1, line 381 of `docs/prds/PRD-feature-dependencies.md`); shipping it now closes the user-experience loop while users are field-testing v0.6.0.

This is **stabilization-tier polish** — small, additive, advisory-only. Not a milestone tranche.

## Files Changed

- `internal/workflow/created_by_inference.go` (new, ~210 LOC) — advisory matcher; `WithDisableCreatedByInference` ctx helper; `inferCreatedBy` scanner; `pristineHasSearch` working-tree probe.
- `internal/workflow/created_by_inference_test.go` (new, ~270 LOC) — all 8 tests from the dispatch contract.
- `internal/workflow/implement.go` — call `inferCreatedBy(ctx, s, slug, recipe)` between recipe parse and recipe write; failures degrade to a warning so persistence is never blocked.
- `internal/cli/cobra.go` — `--no-created-by-infer` flag on `implement` command, plumbed via `workflow.WithDisableCreatedByInference`.

The created-by **gate** (`internal/workflow/created_by_gate.go`) was NOT touched — apply-time concern, separate file, separate sentinel.

## Test Results

```
$ gofmt -l .
(no output)

$ go build ./cmd/tpatch && rm -f tpatch
BUILD OK

$ go test ./...
ok  	github.com/tesseracode/tesserapatch/assets	0.362s
?   	github.com/tesseracode/tesserapatch/cmd/tpatch	[no test files]
ok  	github.com/tesseracode/tesserapatch/internal/cli	4.007s
ok  	github.com/tesseracode/tesserapatch/internal/gitutil	(cached)
ok  	github.com/tesseracode/tesserapatch/internal/provider	(cached)
ok  	github.com/tesseracode/tesserapatch/internal/safety	(cached)
ok  	github.com/tesseracode/tesserapatch/internal/store	(cached)
ok  	github.com/tesseracode/tesserapatch/internal/workflow	8.567s

$ go test ./internal/workflow -run 'CreatedByInference|CreatedByGate' -count=1 -v
--- PASS: TestCreatedByGate_FlagOff_NoOp (0.01s)
--- PASS: TestCreatedByGate_HardParent_TargetMissing_ErrPathCreatedByParent (0.01s)
--- PASS: TestCreatedByGate_DryRun_HardParent_TargetMissing_DowngradesToWarning (0.01s)
--- PASS: TestCreatedByGate_Execute_HardParent_TargetMissing_ReturnsErr (0.01s)
--- PASS: TestCreatedByGate_HardParent_TargetExists_NoError (0.01s)
--- PASS: TestCreatedByGate_SoftParent_TargetMissing_FallsThroughWithWarning (0.01s)
--- PASS: TestCreatedByGate_ParentNotInDependsOn_RecipeRejected (0.01s)
--- PASS: TestCreatedByGate_ParentUpstreamMerged_TargetExists_NoError (0.01s)
--- PASS: TestCreatedByGate_AppliesToReplaceAndAppend (0.01s)
--- PASS: TestCreatedByInference_SuggestsHardParent (0.01s)
--- PASS: TestCreatedByInference_RespectsExistingAnnotation (0.01s)
--- PASS: TestCreatedByInference_AmbiguousMultipleParents (0.01s)
--- PASS: TestCreatedByInference_SkipsSoftParents (0.01s)
--- PASS: TestCreatedByInference_OptOut (0.01s)
--- PASS: TestCreatedByInference_FlagOff (0.01s)
--- PASS: TestCreatedByInference_PristineHasSearch_NoSuggestion (0.01s)
--- PASS: TestCreatedByInference_NoMatchSilent (0.01s)
PASS
ok  	github.com/tesseracode/tesserapatch/internal/workflow	0.535s

$ go test ./assets/... -count=1
ok  	github.com/tesseracode/tesserapatch/assets	0.334s
```

All 9 pre-existing CreatedByGate tests + all 8 new CreatedByInference tests green. Full suite green.

## Deviations from dispatch contract

None. The advisory output, opt-out flag, scope guards (replace-in-file only, hard parents only, non-transitive, no recipe mutation, advisory stderr only, flag-off byte identity), and 8-test layout all match the handoff verbatim.

One minor implementation note for the reviewer: the inference call inside `RunImplement` is wrapped in a `if ierr != nil { warn }` guard rather than a hard-return. Rationale: a transient error in the advisory layer must not block recipe persistence — the apply-time gate is the authoritative enforcement point and would catch any real downstream issue. The dispatch contract didn't pin this either way; this is the conservative choice.

## Next Steps

1. Reviewer dispatch.
2. On APPROVED: archive this handoff, decide on `v0.6.1` cut.

## Blockers

None.

## Context for Next Agent

- The inference scanner is intentionally cheap: it only loads the child status when at least one candidate op exists (fast-path skip), reads each parent's `post-apply.patch` once and caches the bytes for the whole walk, and short-circuits as soon as the pristine working tree contains the Search bytes.
- `ctxKeyDisableCreatedByInfer` is declared with explicit value `1` to sit alongside `ctxKeyDisableRetry = iota = 0` in `retry.go` — same `contextKey` private type. If we add a third workflow-context flag, switch them all to a `const ( ... iota )` block to avoid drift.
- `--no-created-by-infer` is `implement`-only by design (PRD §4.3.1 places inference there, the gate is a separate CLI surface on `apply`). Do not promote.
- Pending follow-ups (separate backlog, NOT in scope here):
  - `feat-satisfied-by-reachability` — git merge-base check on `satisfied_by` SHAs
  - v0.6.0 field-feedback issues if any surface
  - `--auto-apply-inferred` — if operators ask for it, the inference layer is now structured to support recipe mutation as a follow-up.

## 2026-04-26 — M14.4 v0.6.0 cutover APPROVED, archiving handoff

# Current Handoff

## Active Task
- **Task ID**: M14.4
- **Milestone**: M14 — Feature Dependencies / DAG (Tranche D, v0.6.0)
- **Description**: User-facing cutover for the feature-dependency DAG. Seven chunks (A–G): `tpatch status --dag`, default flip, dep-management verbs, status-time validation, 6-skill rollout, `docs/dependencies.md`, release commit.
- **Status**: **Implementation complete — awaiting reviewer**
- **Assigned**: closed 2026-04-26

## Session Summary

All 7 chunks landed on `main` (not yet pushed at write time of this file; push will be the next action). Six logical commits (A+D combined). v0.6.0 NOT tagged — that is supervisor's closeout.

| Chunk | Title | Commit | Headline |
|-------|-------|--------|----------|
| A + D | `tpatch status --dag` + status-time DAG validation | `d1aca5f` | ASCII (`─►` hard / `┄►` soft) + `--json`, scoped + full, cycle-safe; `ValidateAllFeatures` warnings inline on plain `tpatch status`. Reads `status.Reconcile.Outcome` only (ADR-010 D5). 9 new tests. |
| C     | Dep-management verbs                              | `ca23b35` | `tpatch feature deps [<slug> [add\|remove] <parent>[:hard\|:soft]] \| --validate-all`; `tpatch amend --depends-on / --remove-depends-on` (deps-only mode skips request.md rewrite); `tpatch remove --cascade` reverse-topo + `ErrInteractiveRequired` for non-TTY without `--force`; **`--force` does not bypass dep-integrity** (PRD §3.7 / ADR-011 D7). 9 tests + non-TTY pipe helper (because `/dev/null` is a char device on macOS). |
| B     | Flag default flip                                 | `5d5f594` | `parseYAMLConfig` defaults `features_dependencies` to `true` when absent; `Init()` template writes the explicit `true`. Two byte-identity tests (apply gate-off, accept-shadow labels-nil) updated to opt out explicitly. Roundtrip test inverted. |
| E     | 6-skill rollout                                   | `97a994f` | All six shipped skill formats updated atomically with the dependency surface; `created_by` description reframed from "inert" → live apply-time gate. Parity guard (`assets_test.go`) holds. |
| F     | `docs/dependencies.md`                            | `e0a7d47` | Single user-facing reference (~270 LOC): edge model, declaration, validation, apply gate, `created_by` op-level gate (with dry-run downgrade), reconcile labels, compound verdict, `status --dag` examples, `--cascade` contract, migration, out-of-scope list. |
| G     | Release cutover                                   | `f2d0d1b` | Version `0.5.3` → `0.6.0`; new `## v0.6.0 — 2026-04-26 — Feature Dependencies (Tranche D)` CHANGELOG section; M14 box flipped 🔨 → ✅ in ROADMAP; obsolete "Feature dependency DAG" bullet pruned from M15+ Future. **NO TAG.** |

## Current State

`main` carries `f2d0d1b`, six commits ahead of `origin/main`. Build is clean, full test suite is green, parity guard holds, scoped DAG/label/dependency tests are all green. Ready for reviewer.

## Files Changed

Created:
- `internal/cli/status_dag.go` — ASCII tree + JSON renderer for `tpatch status --dag`.
- `internal/cli/status_dag_test.go` — 9 tests (chunks A + D).
- `internal/cli/feature_deps.go` — `feature deps` command tree, `applyAmendDependsOn`, `runRemoveWithCascade`, `collectSubtree`, sentinel `ErrHasDependents` + `ErrInteractiveRequired`.
- `internal/cli/feature_deps_test.go` — 9 tests (chunk C).
- `internal/cli/test_helpers_test.go` — `openDevNull()` non-TTY pipe helper.
- `docs/dependencies.md` — user reference (chunk F).

Modified:
- `internal/cli/cobra.go` — `featureCmd()` registered on root; `--dag` flag wired onto status; status-time `ValidateAllFeatures` warnings; `version` bumped to `0.6.0`.
- `internal/cli/c1.go` — `amendCmd` gained `--depends-on` / `--remove-depends-on` (deps-only mode); `removeCmd` gained `--cascade` + integrity gate.
- `internal/cli/dependency_gate_apply_test.go` — `TestApplyExecute_FlagOff_*` opts out of the new default.
- `internal/store/store.go` — `parseYAMLConfig` defaults `features_dependencies: true`; `Init()` template writes the explicit `true`.
- `internal/store/types.go` — `Config.FeaturesDependencies` doc updated.
- `internal/store/roundtrip_test.go` — `TestConfig_FeaturesDependenciesRoundtrip` inverted (default-true + explicit-false opt-out).
- `internal/workflow/accept_labels_test.go` — `TestAcceptShadow_FlagOff_LabelsRemainNil` opts out explicitly.
- `assets/skills/claude/tessera-patch/SKILL.md` — `created_by` paragraph reframed; new "Feature dependencies (v0.6.0+)" section.
- `assets/skills/copilot/tessera-patch/SKILL.md` — same.
- `assets/skills/cursor/tessera-patch.mdc` — same.
- `assets/skills/windsurf/windsurfrules` — same.
- `assets/workflows/tessera-patch-generic.md` — same.
- `assets/prompts/copilot/tessera-patch-apply.prompt.md` — same.
- `CHANGELOG.md` — new v0.6.0 section.
- `docs/ROADMAP.md` — M14 box flipped to ✅; M14.4 line expanded with chunk-level breakdown and commit shas; obsolete M15+ "Feature dependency DAG" bullet removed.

## Test Results

```
$ gofmt -l .
(clean)

$ go build ./cmd/tpatch && rm -f tpatch
ok

$ go test ./...
ok  github.com/tesseracode/tesserapatch/assets        0.441s
?   github.com/tesseracode/tesserapatch/cmd/tpatch    [no test files]
ok  github.com/tesseracode/tesserapatch/internal/cli  4.968s
ok  github.com/tesseracode/tesserapatch/internal/gitutil   (cached)
ok  github.com/tesseracode/tesserapatch/internal/provider  (cached)
ok  github.com/tesseracode/tesserapatch/internal/safety    (cached)
ok  github.com/tesseracode/tesserapatch/internal/store     (cached)
ok  github.com/tesseracode/tesserapatch/internal/workflow  (cached)

$ go test ./assets/... -count=1
ok  github.com/tesseracode/tesserapatch/assets        0.371s
    (TestAllSkillFilesExist + TestSkillRecipeSchemaMatchesCLI both green
     across all 6 formats; TestSkillParityGuard implicit via build.)

$ go test ./internal/cli      -run 'StatusDag'                       -count=1   ok 1.073s
$ go test ./internal/workflow -run 'CreatedByGate|PlanReconcile|ComposeLabels|EffectiveOutcome|AcceptShadow|GoldenReconcile|Phase35|Labels' -count=1   ok 5.551s
$ go test ./internal/store    -run 'Label|Reconcile|DAG|Dependency|Roundtrip'   -count=1   ok 0.358s
```

## Next Steps

1. Reviewer runs the standard checklist (`AGENTS.md` review phase) against the six commits `d1aca5f..f2d0d1b`.
2. If APPROVED, supervisor:
   - Tags `v0.6.0` on `f2d0d1b`.
   - Archives this handoff to `docs/handoff/HISTORY.md`.
   - Picks the next milestone (M15+ from ROADMAP).
3. If NEEDS REVISION, the implementer reads the LOG.md verdict and iterates here.

## Blockers

None.

## Context for Next Agent

- **Tag is supervisor work, not implementer work.** The release commit deliberately omits a tag. Operator instruction was explicit on this point.
- **`tpatch` binary at the repo root is NOT gitignored.** Always `rm -f tpatch` after `go build ./cmd/tpatch` — this is a recurring slip that has bitten earlier sessions.
- **Source-truth guard (ADR-010 D5):** all DAG / label / status code reads `status.Reconcile.Outcome` via `store.LoadFeatureStatus` — never `artifacts/reconcile-session.json`. The M14.3 adversarial test pins this; do not regress.
- **`--force` is NOT a DAG-integrity bypass.** It only suppresses the TTY confirm prompt on `remove`. Only `--cascade` may opt into removing a feature with downstream dependents. PRD §3.7 / ADR-011 D7. The chunk-C tests pin this.
- **Default-flip compatibility:** v0.5.3-byte-identity behaviour is recoverable per-repo via `features_dependencies: false` in `.tpatch/config.yaml`. Two existing tests demonstrate the opt-out path (`TestApplyExecute_FlagOff_BypassesDependencyGate`, `TestAcceptShadow_FlagOff_LabelsRemainNil`).
- **Skill parity guard.** `assets/assets_test.go` enforces required CLI-command anchors and the recipe-op JSON schema. Adding new content to skills is safe; removing required anchors breaks the guard. The chunk-E rollout used the parity guard as the green-light signal.
- **`/dev/null` is a char device on macOS** — `canPromptForConfirmation` returns true for it. `internal/cli/test_helpers_test.go::openDevNull()` returns an `os.Pipe()` write-end-closed pipe to simulate non-TTY stdin. Reuse it.
- **Amend deps-only mode:** when `--depends-on` / `--remove-depends-on` is set with only the slug arg and no piped stdin, `amend` skips the request.md rewrite path. Don't accidentally re-couple them.
- **`store.Init()` refuses if `.tpatch/` already exists** — the validate-all-on-init style test in chunk C instead asserts that `feature deps --validate-all` runs cleanly post-init. Use the same shape for follow-up tests.

## 2026-04-26 — M14 correctness pass APPROVED, archiving handoff

# Current Handoff

## Active Task

- **Task ID**: M14 correctness pass (3 findings) — fix-pass before M14.4
- **Milestone**: M14 — Feature Dependencies / DAG (Tranche D, v0.6.0)
- **Status**: Complete — all 3 findings landed, ready for review (2026-04-26)
- **Estimated size**: ~190 LOC + 11 tests, no version bump (final: ~520 LOC including doc/comments + 11 tests)

### Three findings (all flag-gated, byte-identical when flag off)

1. **F1 (HIGH, cutover-blocking)**: Wire `created_by` apply-time gate. Today
   `RecipeOperation.CreatedBy` is parsed but inert. Per PRD §4.3 + ADR-011 D4
   it must gate `replace-in-file` / `append-file` against missing targets when
   the named parent is hard. Soft parents emit a warning. Validation error
   when `created_by` names a feature not in `depends_on`. New file
   `internal/workflow/created_by_gate.go` + sentinel `ErrPathCreatedByParent`
   + 7 regression tests.
2. **F2 (MEDIUM)**: `RunReconcile` persists `stale-parent-applied` labels
   computed against the OLD child `AttemptedAt`, then overwrites the
   timestamp with `time.Now()`. Result: child appears stale against itself.
   Fix: thread one shared `attemptedAt` through `saveReconcileArtifacts` →
   `updateFeatureState`, compose labels using it as the staleness baseline.
   2 regression tests.
3. **F3 (MEDIUM)**: `ComposeLabels` keeps emitting parent-derived labels
   for children whose own outcome is `ReconcileUpstreamed`. Per ADR-011 the
   child is being retired; surfacing `waiting-on-parent` is misleading. Fix:
   early return in `ComposeLabels` when `status.Reconcile.Outcome ==
   ReconcileUpstreamed`. 2 regression tests.

### Strict scope guards (DO NOT)

- DO NOT bump version, update CHANGELOG, or tag.
- DO NOT touch skill formats (M14.4 work).
- DO NOT add `tpatch status --dag` (M14.4 work).
- DO NOT add new `ReconcileOutcome` enum values (ADR-011 D3).
- DO NOT consult `artifacts/reconcile-session.json` from any new code path.

### Context

M14.1 ✅ data model + DAG primitives. M14.2 ✅ apply gate (inert until flag on). M14.3 ✅ reconcile topological traversal + composable labels + compound verdict (inert until flag on). All three landed flag-protected — runtime behavior with `features_dependencies: false` is **byte-identical to v0.5.3**.

**M14.4 is the user-facing cutover.** Flipping the flag default to `true`, shipping the `tpatch status --dag` view, rolling label/dep documentation across all 6 skill formats, writing `docs/dependencies.md`, and tagging v0.6.0.

This is the first M14 sub-milestone where end users observe new behavior. Dispatch only after explicit user approval.

### Authoritative docs

1. `docs/adrs/ADR-011-feature-dependencies.md` — D1–D9 (locked)
2. `docs/prds/PRD-feature-dependencies.md` — §3.5 (label matrix), §4.5 (precedence), §5 (UX)
3. `docs/ROADMAP.md` — M14.4 line + Tranche D summary
4. M14.1, M14.2, M14.3 closeout entries in `docs/supervisor/LOG.md`

### Scope (5 chunks)

#### Chunk A — `tpatch status --dag` (~120 LOC)

- New `--dag` flag on `status` command in `internal/cli/cobra.go`.
- Renders the dependency DAG for all features in the project, or a single feature's transitive parent + child set if a slug is given.
- Output: ASCII tree (deterministic by slug) showing each feature with state + reconcile outcome + labels (using `EffectiveOutcome()`).
- Hard deps shown with `─►`, soft deps with `┄►`.
- `--format json` for harness consumption (M9 contract).
- Tests: cycle handling (should never hang — already protected by `DetectCycles`), empty DAG, single-feature subset, label rendering.

#### Chunk B — Flag default flip (~5 LOC + many test fixtures)

- `internal/store/store.go`: change `features_dependencies` default from `false` to `true`.
- This is the moment the new behavior becomes observable. **Audit every test fixture that asserts byte-identity** — some may need updating to include `labels: []` or topo-ordered output.
- Run full suite. Fix every regression.

#### Chunk C — 6-skill rollout (parity-guard coordinated, ~80 LOC of docs)

Update all 6 skill formats with:
- `dependencies` field documentation (analyze-phase bullet)
- Labels reference (`waiting-on-parent`, `blocked-by-parent`, `stale-parent-applied`)
- Compound verdict (`blocked-by-parent-and-needs-resolution`)
- `tpatch status --dag` mention

Files (all 6 in lockstep):
- `assets/skills/claude/tessera-patch/SKILL.md`
- `assets/skills/copilot/tessera-patch.md`
- `assets/skills/copilot-prompt/tessera-patch.prompt.md`
- `assets/skills/cursor/tessera-patch.mdc`
- `assets/skills/windsurf/tessera-patch.md`
- `assets/skills/generic/tessera-patch.md`

`assets/assets_test.go` parity guard MUST pass after all 6 are updated.

Also: `docs/agent-as-provider.md` — if it covers reconcile-time agent behavior, add labels section.

#### Chunk D — `docs/dependencies.md` (~150 LOC)

User-facing reference doc:
- What dependencies are (hard vs soft)
- How to declare them (YAML examples)
- Validation rules (cycles, dangling, self-ref, etc.)
- Label semantics + matrix (lifted from PRD §3.5)
- Compound verdict explanation
- `--cascade` and force semantics (D7)
- `tpatch status --dag` examples
- Migration note: existing v0.5.x projects keep working unchanged unless they add deps.

#### Chunk E — Release cutover

- Bump `version = "0.6.0"` in `internal/cli/cobra.go`.
- New `## 0.6.0 — 2026-MM-DD — Feature Dependencies (Tranche D)` section in `CHANGELOG.md` summarizing M14.1–M14.4.
- Update `docs/ROADMAP.md`: M14 ✅, Tranche D box closed.
- Tag `v0.6.0` AFTER push, AFTER full validation.

### Strict scope guards (DO NOT do)

- Do NOT skip the parity guard in Chunk C — all 6 skills must move atomically.
- Do NOT add new external Go dependencies.
- Do NOT introduce `ReconcileWaitingOnParent` / `ReconcileBlockedByParent` enum values (still ADR-011 D3).
- Do NOT inject parent patches into the M12 resolver (ADR-011 D8 — deferred to v0.7).
- Do NOT populate `created_by` from the implement phase (separate backlog).
- Do NOT bypass DAG integrity with `--force` (ADR-011 D7 — explicit `--cascade` required).

### Validation gate

```
gofmt -l .
go build ./cmd/tpatch && rm -f tpatch
go test ./...
go test ./assets/...
go test ./internal/cli -run 'StatusDag' -count=1 -v
go test ./internal/workflow -run 'PlanReconcile|ComposeLabels|EffectiveOutcome|AcceptShadow|GoldenReconcile|Phase35|Labels' -count=1 -v
go test ./internal/store -run 'Label|Reconcile|DAG|Dependency|Roundtrip' -count=1 -v
```

All M14.1+M14.2+M14.3 tests stay green. Golden reconcile + manual accept regressions stay green.

### Workflow notes

- `tpatch` binary at root is NOT gitignored. After every `go build` run `rm -f tpatch` BEFORE staging. (Recurring slip — supervisor has tripped 3 times this session.)
- Use `git -c commit.gpgsign=false` for commits. Each carries the trailer.
- `git push` takes 60+ seconds on this machine.
- 5–6 logical commits expected (one per chunk + version bump + CHANGELOG).
- Do NOT tag during the implementer's run. Tagging is the supervisor's final closeout action after reviewer APPROVES.

## Session Summary

M14 correctness pass complete. Three findings landed in three logical
commits, all flag-protected:

  - F1 (cbe2873): `created_by` apply-time gate wired into recipe.go
    (`replace-in-file` / `append-file` only). New sentinel
    `ErrPathCreatedByParent`. Soft deps emit warning + fall through.
    7 regression tests.
  - F2 (071c5ed): one shared `attemptedAt` timestamp threaded through
    `saveReconcileArtifacts` → `updateFeatureState` so persisted
    `Labels` reflect the AttemptedAt about to be written. New
    `composeLabelsAt(s, slug, asOf)` helper; `ComposeLabels` refactored
    to delegate to `composeLabelsFromStatus(s, child)`. 2 regression
    tests.
  - F3 (cc95cbb): early return in `composeLabelsFromStatus` for
    children whose own outcome is in `childRetiredOutcomes`
    (currently only `ReconcileUpstreamed`). 2 regression tests.

Validation gate: `gofmt` clean, `go build ./cmd/tpatch` green,
`go test ./...` green, all targeted regression suites green
(workflow, store, cli, assets parity). M14.1 / M14.2 / M14.3
adversarial tripwires
(`TestComposeLabels_ReadsStatusJsonNotSessionArtifact`,
`TestReconcile_FlagOn_BlockedByParent_SkipsPhase35`) stay green.

## Files Changed (M14 fix-pass)

  - internal/workflow/created_by_gate.go          (new, F1)
  - internal/workflow/created_by_gate_test.go     (new, F1)
  - internal/workflow/recipe.go                   (F1: signatures + gate wiring)
  - internal/cli/cobra.go                         (F1: 2 call sites)
  - internal/cli/phase2.go                        (F1: 1 call site)
  - internal/workflow/reconcile.go                (F2: shared attemptedAt)
  - internal/workflow/labels.go                   (F2 helper extraction + F3 retired-outcomes)
  - internal/workflow/labels_freshness_test.go    (new, F2)
  - internal/workflow/labels_upstreamed_test.go   (new, F3)

## Test Results

  gofmt -l .                                                  → clean
  go build ./cmd/tpatch                                       → ok
  go test ./...                                               → all packages ok
  go test ./internal/workflow -run 'CreatedByGate|ComposeLabels|RunReconcile|GoldenReconcile|Phase35|Labels|AcceptShadow|PlanReconcile|Recipe' → ok
  go test ./internal/store -run 'Label|Reconcile|DAG|Dependency|Roundtrip' → ok
  go test ./internal/cli -run 'DependencyGate|Apply'                       → ok
  go test ./assets/...                                                     → ok

## Next Steps

1. Reviewer dispatched to verify the three commits against the
   PRD §4.3 contract and the regression test set.
2. On APPROVED → archive this handoff, then user may green-light
   M14.4 (status DAG view + skill rollout + v0.6.0 cutover).

## Blockers

None.

## Context for Next Agent

  - All three fixes are flag-gated: with `features_dependencies: false`
    (current default) behaviour is byte-identical to v0.5.3.
  - F1 changes the public signatures of `workflow.DryRunRecipe` and
    `workflow.ExecuteRecipe` from `(repoRoot, recipe)` to `(s, recipe)`.
    Three internal call sites updated; no external consumers.
  - F2 adds an unexported `attemptedAt` field to `ReconcileResult`.
    Unexported, so encoding/json ignores it — no schema impact.
  - F3 currently treats only `ReconcileUpstreamed` as "retired". If a
    future enum value (e.g. `ReconcileObsolete`) lands, add it to
    `childRetiredOutcomes`.
  - Implement-phase heuristic inference of `created_by` is still a
    separate backlog item per PRD §4.3.1 (NOT included here).

---

## 2026-04-26 — M14.3 APPROVED, archiving handoff

# Current Handoff

## Active Task

- **Task ID**: M14.3 — Reconcile topological traversal + composable labels + compound verdict
- **Milestone**: M14 — Feature Dependencies / DAG (Tranche D, v0.6.0)
- **Status**: Review — ready for code-review sub-agent (implementation complete 2026-04-26)
- **Assigned**: 2026-04-26
- **Estimated size**: ~500 LOC (largest M14 sub-milestone)

### Context

M14.1 ✅ data model + DAG primitives. M14.2 ✅ apply gate + `created_by` (inert). Now M14.3 introduces the first reconcile-time DAG behavior:

1. **Topological traversal** — when reconciling a set of features, run them in dependency order (parents first).
2. **Composable labels** — `waiting-on-parent`, `blocked-by-parent`, `stale-parent-applied` overlay onto the child's intrinsic verdict (per ADR-011 D6 + PRD §3.5).
3. **Compound verdict** — `blocked-by-parent-and-needs-resolution` skips phase 3.5 (resolver) when a hard parent isn't applied.

All gated behind `features_dependencies` (default false). Flag-off path is byte-identical to v0.5.3 reconcile.

### Authoritative docs (must read in order)

1. **`docs/adrs/ADR-011-feature-dependencies.md`** — locks 9 decisions. CRITICAL sections:
   - **D3** — Composable labels, NOT new states. Don't add `ReconcileWaitingOnParent` enum values.
   - **D6** — Read child's intrinsic verdict from `status.Reconcile.Outcome` FIRST, then overlay parent-derived labels. Compound verdict `blocked-by-parent-and-needs-resolution` skips phase 3.5.
   - **D7** — `--cascade` required for cross-feature operations; `--force` does NOT bypass DAG integrity.

2. **`docs/prds/PRD-feature-dependencies.md`**:
   - **§3.5** — composable labels matrix. Authoritative wording.
   - **§4.5** — precedence rules. AUTHORITATIVE when §3.4 contradicts.
   - **§3.4** — has residual terminology drift treating labels as enum verdicts. **DEFER to ADR-011 D6 + §4.5.** Do NOT introduce new `ReconcileOutcome` enum values from §3.4.
   - **§7** — acceptance criteria.

3. **`docs/adrs/ADR-010-provider-conflict-resolver.md` D5** — artifact ownership contract.

4. **`internal/workflow/reconcile.go`** — current reconcile state machine. Read end-to-end before touching it. Especially `RunReconcile`, `tryPhase35`, `saveReconcileArtifacts`.

5. **`internal/workflow/accept.go`** — `AcceptShadow` + `clearShadowPointerAndStamp`. M14.3 may need to extend the helper to compose labels at accept time.

6. **`internal/store/dag.go`** — M14.1 primitives (`TopologicalOrder`, `Children`).

7. M14.2 commits — gate semantics, especially how soft vs hard is interpreted.

### The external-reviewer guard (MANDATORY for M14.3)

> Any new dependency/DAG logic must read **`status.Reconcile.Outcome`** as the authoritative machine-readable reconcile result — NEVER `artifacts/reconcile-session.json`. The session artifact is an audit record of one `RunReconcile` invocation; `status.json` is the source of current truth post-accept (see ADR-010 D5).

This is **load-bearing** for M14.3. Label composition reads parent verdicts. Always go through `store.LoadFeatureStatus(parent).Reconcile.Outcome`, never any session artifact.

### M14.3 scope (~500 LOC across 3 chunks)

#### Chunk A — Topological reconcile traversal (~150 LOC)

Update `RunReconcile` (or wrap it) so when given multiple slugs, they execute in topological order (parents first). Currently the loop is sequential in input order.

- New: `workflow.PlanReconcile(s *Store, slugs []string) ([]string, error)` — builds the dep graph for the given set + their hard parents (transitive closure of hard deps), runs `TopologicalOrder`, returns the ordered slug list. Reject with cycle path on cycle (already supported by `dag.go`).
- Wire into `RunReconcile`'s entry point. Flag-gated:
  - `!cfg.DAGEnabled()`: process slugs in input order (current v0.5.3 behavior). Byte-identical exit, byte-identical `reconcile-session.json` per slug.
  - `cfg.DAGEnabled()`: call `PlanReconcile`, process in returned order.
- Soft deps still contribute to ordering (per PRD §6 / M14.1 design). Hard vs soft only matters for label composition + apply gate, not topology.

Tests:
- `TestPlanReconcile_FlagOff_PreservesInputOrder`
- `TestPlanReconcile_FlagOn_TopologicallyOrders`
- `TestPlanReconcile_RejectsCycle`
- `TestPlanReconcile_TransitiveHardClosure` — given `[child]` only, closure includes hard parents

#### Chunk B — Composable labels (~250 LOC, the trickiest)

Per ADR-011 D3 + D6 + PRD §3.5, labels are computed AFTER the intrinsic reconcile verdict is determined. They overlay, not replace.

New types in `internal/store/types.go`:

```go
// ReconcileLabel is a derived overlay on top of Reconcile.Outcome that
// describes the DAG context. Labels are computed; they are NOT persisted
// as enum values on Reconcile.Outcome.
type ReconcileLabel string

const (
    LabelWaitingOnParent      ReconcileLabel = "waiting-on-parent"
    LabelBlockedByParent      ReconcileLabel = "blocked-by-parent"
    LabelStaleParentApplied   ReconcileLabel = "stale-parent-applied"
)
```

Add `Labels []ReconcileLabel \`json:"labels,omitempty"\`` to `FeatureStatus.Reconcile` (the existing struct that holds `Outcome`, `AttemptedAt`, etc.). `omitempty` is critical — empty list = field omitted = byte-identical to v0.5.3.

New file `internal/workflow/labels.go`:

```go
// ComposeLabels reads the current FeatureStatus + dependency declarations
// and computes the overlay labels. The intrinsic verdict (Reconcile.Outcome)
// is read FIRST and remains untouched; labels overlay on top.
//
// Authoritative reading rule (ADR-010 D5): for each parent, read
// store.LoadFeatureStatus(parent).Reconcile.Outcome — NEVER consult
// artifacts/reconcile-session.json. The session artifact may be stale
// or describe a pre-accept state.
//
// When Config.DAGEnabled() is false, returns empty slice (no labels).
func ComposeLabels(s *store.Store, slug string) ([]store.ReconcileLabel, error)
```

Behavior matrix per PRD §3.5 / ADR-011 D6:

| Parent state (hard dep) | Parent reconcile.Outcome | Label on child |
|---|---|---|
| `analyzed`/`defined`/`explored`/`implemented` (not yet applied) | n/a | `waiting-on-parent` |
| applied, but parent has `needs-human-resolution`/`blocked-*`/`shadow-awaiting` | (parent reconcile blocked) | `blocked-by-parent` |
| applied + parent recently changed (rebased/amended) and child hasn't been re-reconciled | parent newer than child's last reconcile | `stale-parent-applied` |

Soft deps NEVER produce labels (per ADR-011 D4 — soft is ordering-only).

Multiple labels can stack — e.g., one parent waiting + another stale gives the child `[waiting-on-parent, stale-parent-applied]`. Order labels deterministically (alphabetical by string).

Wire into the reconcile state machine in `RunReconcile`:
- Flag off: do not call `ComposeLabels`. Keep `Reconcile.Labels = nil`.
- Flag on: AFTER the intrinsic verdict is computed, call `ComposeLabels` and persist into `FeatureStatus.Reconcile.Labels`.

Tests in `internal/workflow/labels_test.go`:
- `TestComposeLabels_FlagOff_AlwaysEmpty`
- `TestComposeLabels_NoDeps_Empty`
- `TestComposeLabels_HardParentNotApplied_AddsWaitingOnParent`
- `TestComposeLabels_HardParentBlocked_AddsBlockedByParent`
- `TestComposeLabels_HardParentApplied_NoLabel`
- `TestComposeLabels_HardParentRecentlyChanged_AddsStaleParentApplied`
- `TestComposeLabels_SoftParentNeverProducesLabel`
- `TestComposeLabels_MultipleParentsStackLabels`
- `TestComposeLabels_DeterministicOrder` (run 50× on a fixture, assert equal each time)
- `TestComposeLabels_ReadsStatusJsonNotSessionArtifact` — adversarial: write a misleading `reconcile-session.json` for the parent and confirm the label uses `status.json` instead.

Round-trip:
- `TestStatusRoundtrip_FlagOff_LabelsOmitted` — flag off, save status, load, save again, byte-identical.
- `TestStatusRoundtrip_FlagOn_EmptyLabels_OmittedFromJSON` — `Labels: []` writes the same bytes as `Labels: nil`.

#### Chunk C — Compound verdict + phase 3.5 skip (~100 LOC)

Per ADR-011 D6: if a child has `LabelBlockedByParent` AND its intrinsic outcome would be `needs-human-resolution`, the COMPOSED outcome is the compound `blocked-by-parent-and-needs-resolution`. This compound verdict means: skip phase 3.5 (provider resolver) entirely — no point asking the LLM to resolve conflicts when a hard parent is itself broken.

This compound is NOT a new `ReconcileOutcome` enum value. It's a derived presentation. The persisted `Reconcile.Outcome` stays `needs-human-resolution` (intrinsic); the derived presentation is computed from `Outcome + Labels` at read time.

- Add a helper in `internal/store/types.go`:

```go
// EffectiveOutcome returns the compound presentation of (Outcome, Labels)
// per ADR-011 D6 + PRD §3.5. Labels overlay on top of Outcome:
//   - Outcome=needs-human-resolution + LabelBlockedByParent
//     → "blocked-by-parent-and-needs-resolution" (compound, M14.3)
//   - Otherwise: Outcome stringified.
//
// Callers like status display use this helper. Programmatic decisions
// MUST read Outcome + Labels separately, not the compound string.
func (r FeatureReconcile) EffectiveOutcome() string
```

- In `tryPhase35` (or wherever the resolver is invoked), before launching the resolver:
  - If `Config.DAGEnabled()` AND child has `LabelBlockedByParent`: short-circuit. Set `Outcome = ReconcileBlockedRequiresHuman` (existing enum, NOT a new one), set `Labels = [blocked-by-parent]`, persist, log a clear note pointing the user at the parent. Don't call the resolver.
  - The compound presentation is then computed by `EffectiveOutcome()` for display.

Tests:
- `TestReconcile_FlagOn_BlockedByParent_SkipsPhase35` — assert resolver was never called (use a scripted provider that fails the test if invoked).
- `TestEffectiveOutcome_CompoundComposition` — `(needs-human-resolution, [blocked-by-parent])` → `blocked-by-parent-and-needs-resolution`.
- `TestEffectiveOutcome_PassthroughWhenNoCompoundLabels` — other label combinations don't produce compounds.

#### Chunk D — Skill format updates (~minimal)

The 6 skill formats currently describe reconcile outcomes but not labels. **HOLD this for M14.4** — M14.3 keeps the labels invisible to humans (they live in `status.json` for tooling). The skill rollout for labels happens at M14.4 alongside `tpatch status --dag` and `docs/dependencies.md`.

**However**: if the parity guard (`assets/assets_test.go`) checks anything about the `status.json` schema (it might), confirm `Labels` field is documented OR confirm the parity guard does not require it. Run `go test ./assets/...` after every type change.

#### Chunk E — Interaction with `AcceptShadow` (~minimal but critical)

`AcceptShadow` is the shared accept helper from v0.5.2/v0.5.3. After it stamps `Reconcile.Outcome=ReconcileReapplied`:

- If flag on: re-compute `Labels` for the accepted child (the parent state may have changed since reconcile started). Persist updated labels.
- If flag off: leave `Labels` nil (it was already nil if you didn't set it).

Tests:
- `TestAcceptShadow_FlagOn_RefreshesLabels` — set up child with stale label, run accept, assert labels recomputed.
- `TestAcceptShadow_FlagOff_LabelsRemainNil` — byte-identical `status.json` post-accept vs v0.5.3.

### Strict scope guards (DO NOT do these)

- DO NOT add `tpatch status --dag` output (M14.4)
- DO NOT update skill formats with labels documentation (M14.4)
- DO NOT bump version, update CHANGELOG, or tag (M14.4)
- DO NOT add `ReconcileWaitingOnParent` / `ReconcileBlockedByParent` enum values to `ReconcileOutcome` — labels are NOT new states (ADR-011 D3)
- DO NOT add new external Go dependencies
- DO NOT touch the apply gate from M14.2 (separate concern)
- DO NOT populate `created_by` from the implement phase yet — that's separate from M14.3 label work and can wait. Labels read parent state + dep declarations, not `created_by`.
- DO NOT inject parent patches into the M12 resolver context (ADR-011 D8)

### Validation gate

```
gofmt -l .
go build ./cmd/tpatch
go test ./...
go test ./assets/...                    # parity guard
go test ./internal/workflow -run 'PlanReconcile|ComposeLabels|EffectiveOutcome|AcceptShadow|GoldenReconcile' -count=1 -v
go test ./internal/store -run 'DAG|Dependency|Validate|Roundtrip|Reconcile' -count=1 -v
```

CRITICAL regression tests that must stay green:
- `TestGoldenReconcile_ResolveApplyTruthful`
- `TestGoldenReconcile_ManualAcceptFlow`
- All M14.1 dag/validation/roundtrip tests
- All M14.2 dependency-gate tests

### Workflow

1. Update CURRENT.md "Status: In Progress" with timestamp.
2. Read all required docs IN ORDER. ADR-011 D3 + D6 + PRD §3.5 + §4.5 are non-negotiable.
3. **Chunk A first** (planner) — pure logic on top of M14.1 `dag.go`. Easy regression target.
4. **Chunk B** (labels) — most code volume; do `ComposeLabels` + tests before wiring into reconcile.
5. **Chunk C** (compound verdict) — small but high-stakes. Skip-phase-3.5 test must use a tripwire provider (fails if invoked).
6. **Chunk E** (`AcceptShadow` integration) — small but easy to forget.
7. Run full validation gate. Iterate.
8. Update CURRENT.md with completion summary.
9. 3-5 logical commits, all with the Co-author trailer. Suggested:
   - `feat(workflow): add PlanReconcile topological planner (M14.3)`
   - `feat(store): add ReconcileLabel + Labels field (M14.3)`
   - `feat(workflow): add ComposeLabels + label-aware reconcile (M14.3)`
   - `feat(workflow): compound blocked-by-parent verdict + phase-3.5 skip (M14.3)`
   - `feat(workflow): AcceptShadow refreshes labels (M14.3)`
10. Push to `origin/main`. (`git push` takes 60+ seconds.)
11. Final CURRENT.md update flagging "Status: Review — ready for code-review sub-agent".

DO NOT bump version. DO NOT update CHANGELOG. DO NOT tag.

### Out-of-band reminders

- The `tpatch` binary at root is NOT gitignored — delete it after `go build`. NEVER commit it.
- Zero external Go deps.
- Update CURRENT.md at every phase transition (analyze → chunk-A → chunk-B → chunk-C → chunk-E → done).

### Deferred behind M14.3

- M14.4 — `tpatch status --dag` rendering, skills analyze-phase bullet for DAG, `docs/dependencies.md` user guide, flag default flip to true, CHANGELOG, tag v0.6.0 (~300 LOC). **THIS is the user-facing cutover.**

### Registered follow-ups (unchanged)

- `feat-ephemeral-mode` — depends on `feat-feature-import` + `feat-delivery-modes`
- `feat-feature-reorder` — depends on `feat-feature-dependencies` (i.e., M14)
- `feat-resolver-dag-context` — parent-patch injection to M12 resolver (DEFERRED — ADR-011 D8 explicitly excludes from v0.6)
- `feat-feature-autorebase`, `feat-amend-dependent-warning`
- `feat-skills-apply-auto-default`, `bug-record-roundtrip-false-positive-markdown`, `chore-gitignore-tpatch-binary`
- `feat-satisfied-by-reachability` — `git merge-base` reachability check for `satisfied_by`; M14.2 deferred this to keep gate logic pure.

---

## Implementation Summary (2026-04-26 — completed)

**Status**: All 5 chunks complete. Ready for code-review sub-agent.

### Chunks delivered

- **Chunk B-types** — `ReconcileLabel` newtype + 3 constants (`waiting-on-parent`, `blocked-by-parent`, `stale-parent-applied`), `ReconcileSummary.Labels []ReconcileLabel` (with `omitempty` for byte-identity round-trip), `EffectiveOutcome()` helper computing the compound `blocked-by-parent-and-needs-resolution` verdict at READ time (per ADR-011 D3).
- **Chunk A — PlanReconcile** — Hard-parent transitive closure + topological order. Wired into `RunReconcile` gated on `cfg.DAGEnabled()`. Wraps `store.ErrCycle` with cycle-path decoration.
- **Chunk B — ComposeLabels** — Reads parent verdicts via `store.LoadFeatureStatus(parent).Reconcile.Outcome` ONLY (per ADR-010 D5 / ADR-011 D6). Soft deps never produce labels (D4). Output sorted + deduped. Adversarial test `TestComposeLabels_ReadsStatusJsonNotSessionArtifact` enforces the artifact-ownership invariant.
- **Chunk C — Phase-3.5 short-circuit** — In `ForwardApply3WayConflicts` arm, `LabelBlockedByParent` short-circuits BEFORE `tryPhase35` runs. Phase string `phase-3.5-skipped-blocked-by-parent`. Tripwire test (`tripwireProvider`) confirms resolver is not invoked.
- **Chunk D — Skill HOLD** — No skill asset changes for M14.3 (deferred to M14.4 user-facing cutover). Parity guard `go test ./assets/...` green throughout.
- **Chunk E — AcceptShadow refresh** — When DAG flag on, recompute labels via `ComposeLabels` after `clearShadowPointerAndStamp` so children see refreshed labels next reconcile.

### Files

**New** (8): `internal/store/reconcile_label_test.go`, `internal/workflow/plan_reconcile.go`, `internal/workflow/plan_reconcile_test.go`, `internal/workflow/labels.go`, `internal/workflow/labels_test.go`, `internal/workflow/labels_phase35_test.go`, `internal/workflow/accept_labels_test.go`.

**Modified** (4): `internal/store/types.go`, `internal/workflow/reconcile.go`, `internal/workflow/accept.go`, `docs/handoff/CURRENT.md`.

### Tests added

- 4 ReconcileLabel/EffectiveOutcome/roundtrip tests (store)
- 4 PlanReconcile tests (closure, topo, cycle, soft-not-pulled-in)
- 11 ComposeLabels tests (matrix coverage + adversarial artifact-ownership)
- 3 phase-3.5 short-circuit tests (incl. tripwire)
- 2 AcceptShadow refresh tests

All passing. Full suite (`go test ./... -count=1`) green. `gofmt -l .` clean. Build clean.

### Validation gate (final)

```
gofmt -l .                                       → empty
go build ./cmd/tpatch                            → ok (binary removed)
go test ./... -count=1                           → all packages ok
go test ./assets/... -count=1                    → ok (parity guard green)
go test ./internal/workflow -run 'PlanReconcile|ComposeLabels|EffectiveOutcome|AcceptShadow|GoldenReconcile|Phase35|BlockedByParent' → ok
go test ./internal/store -run 'DAG|Dependency|Validate|Roundtrip|Reconcile' → ok
```

Critical regressions held: `TestGoldenReconcile_ResolveApplyTruthful`, `TestGoldenReconcile_ManualAcceptFlow`, all M14.1/M14.2 tests.

### Commits (4 + this docs commit)

1. `7c9aee4` feat(store): ReconcileLabel + Labels field + EffectiveOutcome
2. `bccf5e2` feat(workflow): PlanReconcile topological planner
3. `b9efd07` feat(workflow): ComposeLabels + label-aware reconcile + phase-3.5 skip
4. `a232a7b` feat(workflow): AcceptShadow refreshes labels

### Notes for reviewer

- ADR-011 D3 invariant: `Labels` is overlay; `Outcome` enum unchanged. Compound verdict computed at READ time only via `EffectiveOutcome()`.
- ADR-010 D5 invariant: every parent-verdict read goes through `store.LoadFeatureStatus(...).Reconcile.Outcome`. Adversarial test guards this.
- `omitempty` on `Labels` is load-bearing for pre-M14.3 fixture byte-identity (`TestRoundtrip_PreM14_3StatusByteIdentity`).
- Soft deps: explicitly exempt from labels (PRD §3.5 / ADR-011 D4). `TestComposeLabels_SoftDepNeverProducesLabels` enforces.
- `saveReconcileArtifacts` only invokes `ComposeLabels` when caller-set `result.Labels` is empty — preserves the phase-3.5 short-circuit's pre-set `[blocked-by-parent]`.
- No version bump, no CHANGELOG, no tag — deferred to M14.4.


---

## 2026-04-26 — M14.2 APPROVED, archiving handoff

# Current Handoff

## Active Task

- **Task ID**: M14.2 — Apply gate + `created_by` recipe op + 6-skill parity-guard rollout
- **Milestone**: M14 — Feature Dependencies / DAG (Tranche D, v0.6.0)
- **Status**: Review — ready for code-review sub-agent (implementation complete 2026-04-26)
- **Assigned**: 2026-04-26

## Session Summary

M14.2 implemented in three coordinated layers:

1. **Recipe schema** — added `CreatedBy string` (json:`created_by,omitempty`) to `workflow.RecipeOperation`. Field is persisted but inert; `omitempty` preserves byte-identity for v0.5.3 recipes.
2. **6-skill parity-guard rollout** — documented `created_by` in all 6 shipped skill formats + `docs/agent-as-provider.md`. Parity guard re-run after each file; stayed green throughout.
3. **Apply gate** — new `workflow.CheckDependencyGate(s, slug)` enforces ADR-011 D4. No-op when `Config.DAGEnabled()` is false; otherwise rejects hard parents not in `applied`/`upstream_merged` (with `satisfied_by` SHA-shape check, no reachability — documented limitation per ADR-011 D5). Wired at the top of `runApplyAuto` and inside `runApplyExecute` (defence-in-depth). Soft deps never block. Sentinel `ErrParentNotApplied`, wrappable via `errors.Is`.

## Files Changed

- `internal/workflow/implement.go` — added `CreatedBy` field on `RecipeOperation`
- `internal/workflow/dependency_gate.go` — new file, `CheckDependencyGate` + `ErrParentNotApplied`
- `internal/workflow/dependency_gate_test.go` — 9 unit tests (all 8 task-required scenarios + bad-SHA bonus)
- `internal/workflow/recipe_createdby_test.go` — 3 round-trip / schema-closure tests
- `internal/cli/cobra.go` — gate wired into `runApplyExecute` + `runApplyAuto`
- `internal/cli/dependency_gate_apply_test.go` — CLI integration tests (blocked + bypass-when-flag-off)
- `assets/skills/claude/tessera-patch/SKILL.md` — `created_by` documentation
- `assets/skills/copilot/tessera-patch/SKILL.md` — `created_by` documentation
- `assets/skills/cursor/tessera-patch.mdc` — `created_by` documentation
- `assets/skills/windsurf/windsurfrules` — `created_by` documentation
- `assets/workflows/tessera-patch-generic.md` — `created_by` documentation
- `assets/prompts/copilot/tessera-patch-apply.prompt.md` — `created_by` documentation
- `docs/agent-as-provider.md` — canonical `created_by` documentation
- `docs/handoff/CURRENT.md` — status updates (this file)

## Test Results

```
gofmt -l .                        # clean
go build ./cmd/tpatch             # ok
go test ./...                     # all green (assets, cli, gitutil, provider, safety, store, workflow)
go test ./internal/workflow -run 'DependencyGate|Recipe|CreatedBy' -count=1  # 12 PASS
go test ./internal/store    -run 'DAG|Dependency|Validate|Roundtrip' -count=1  # 17 PASS (M14.1 regression clean)
go test ./assets/...              # parity guard PASS
```

## Deferred / Documented Limitations

- `satisfied_by` reachability (`git merge-base`) is intentionally NOT checked in M14.2. The gate verifies only that the value is a 40-hex SHA; ADR-011 D5 treats `satisfied_by` as provenance, not a runtime guard. Logged here so M14.3+ can choose to add a reachability check if a real consumer materialises.
- `created_by` is not yet emitted by the implement phase — wiring deferred to M14.3 alongside the label-composition consumer.
- `--mode prepare` and `--mode started` are deliberately NOT gated. They write only `.tpatch/` artifacts and do not mutate the working tree; ADR-011 D4 scopes the gate to recipe execution.

## Context for Reviewer

- Reviewer guard remained dormant in M14.2 (no reconcile changes). Search `dependency_gate.go` for the `status.Reconcile.Outcome` rule comment — it's documented in the doc-comment so M14.3 inherits the constraint.
- Soft deps are not surfaced in the error message at all. M14.3 may want to surface soft-dep ordering hints separately; out of scope here.
- The CLI integration test seeds the recipe by hand under `.tpatch/features/<slug>/artifacts/` — same pattern as `TestApplyAutoMode`.


### Context

M14.1 landed the data model: `Dependency` struct + `FeatureStatus.DependsOn` (omitempty) + DFS cycle detection + Kahn topo + 5 validation rules + sentinel errors + `features_dependencies` flag (default false). 30 new tests, byte-identity round-trip guard, no callers yet gate on the flag.

M14.2 adds the **first behavior change** — but still gated. With `features_dependencies=true`:
1. `tpatch apply` refuses to execute when any **hard** parent is not yet `applied`/`upstream_merged`.
2. The recipe gains a new optional op `created_by` so child features can declare which parent originated a file (used by M14.3 for label composition).

### Authoritative docs (must read before coding)

1. `docs/adrs/ADR-011-feature-dependencies.md` — locks 9 decisions. Especially **D4** (hard deps gate apply + `created_by`; soft gates neither) and **D5** (`upstream_merged` satisfies deps via `satisfied_by`).
2. `docs/prds/PRD-feature-dependencies.md` — §3.2 apply gate semantics, §3.3 validation, §3.5 labels (READ but DON'T IMPLEMENT — that's M14.3), §6 milestone sizing.
3. `docs/adrs/ADR-010-provider-conflict-resolver.md` D5 — artifact ownership contract. Note: M14.2 does NOT touch reconcile, so this is reference-only.
4. `assets/assets_test.go` — the parity guard. M14.2 mutates the recipe JSON contract — the parity guard MUST stay green after the rollout.

### M14.2 scope (~250 LOC + 6 skill format updates)

#### 1. Apply gate (~80 LOC)

- New: `workflow.CheckDependencyGate(s *Store, slug string) error` — looks up the feature's `DependsOn`, for each `Kind=hard` parent verifies `state ∈ {applied, upstream_merged}` (and if `upstream_merged`, that `SatisfiedBy` matches a parent commit reachable from current HEAD — minimal check, see PRD §3.2).
- Wire into `apply --mode execute` and `apply --mode auto` BEFORE the existing recipe execution begins. Soft deps are NOT gated — they're ordering hints only.
- **Gated by `features_dependencies` flag** — when false, `CheckDependencyGate` is a no-op. Same flag from M14.1.
- Error message must be actionable: list the blocking parent slug(s) and their current state. Suggest `tpatch apply <parent>` first.
- Sentinel: `ErrParentNotApplied` (wrappable via `errors.Is`).

Tests:
- gate-disabled-passes (flag off, hard parent in `analyzed` state — apply proceeds)
- gate-rejects-hard-unapplied (flag on, hard parent in `analyzed` — apply rejected)
- gate-allows-hard-applied (flag on, hard parent applied — apply proceeds)
- gate-allows-upstream-merged (flag on, hard parent in `upstream_merged` with valid `satisfied_by` — apply proceeds)
- gate-rejects-upstream-merged-bad-sha (flag on, `satisfied_by` not reachable from HEAD — apply rejected)
- gate-ignores-soft (flag on, only soft parents unapplied — apply proceeds)
- gate-mixed (flag on, one hard applied + one hard not + one soft not — apply rejected with only the unapplied hard listed)

#### 2. `created_by` recipe op (~120 LOC + 6-skill rollout)

PRD §3.4 (NOTE: this section has the residual ADR-011 D6 terminology drift — defer to ADR-011 D4 + §3.5 for any conflict). The recipe gains an optional field on each operation:

```json
{
  "op": "patch",
  "path": "src/auth.ts",
  "created_by": "feat-jwt-auth",   // optional; the parent slug that originated this file
  "content": "..."
}
```

- Update `internal/workflow/recipe.go` (or wherever `RecipeOperation` is defined) to add `CreatedBy string \`json:"created_by,omitempty"\`` field.
- The field is **persisted but inert in M14.2** — no behavior depends on it. M14.3 reads it for label composition. Document this clearly in a doc comment.
- `omitempty` is critical — recipes generated for features with no DAG flag must round-trip byte-identical to v0.5.3.
- Add a positive recipe-parsing test that round-trips a recipe with `created_by` set; add a negative test confirming an unknown field still fails the parity guard's `DisallowUnknownFields` (the schema is closed except for known fields).

#### 3. 6-skill parity-guard rollout — COORDINATED ATOMIC CHANGE

The parity guard (`assets/assets_test.go`) enforces that the recipe schema documented in skill files matches the Go struct. Every skill format must be updated **in lockstep** with the Go struct change:

- `assets/skills/claude/tessera-patch/SKILL.md`
- `assets/skills/copilot/tessera-patch/SKILL.md`
- `assets/skills/cursor/tessera-patch.mdc`
- `assets/skills/windsurf/windsurfrules`
- `assets/workflows/tessera-patch-generic.md`
- `assets/prompts/copilot/tessera-patch-apply.prompt.md`

Plus `docs/agent-as-provider.md` (the canonical contract reference).

In each, document the `created_by` field as: optional, parent feature slug, ordering/label hint only, currently inert.

Run `go test ./assets/...` after each skill is updated to catch drift early.

#### 4. Strict scope guards

DO NOT in M14.2:
- Compose DAG labels or add the `blocked-by-parent-and-needs-resolution` compound verdict (M14.3)
- Touch reconcile topological traversal (M14.3)
- Add `tpatch status --dag` output (M14.4)
- Bump version, update CHANGELOG, or tag (M14.4 supervisor task at v0.6.0)
- Add new external Go dependencies

### External reviewer guard (still applies)

Any new logic must read `status.Reconcile.Outcome` for reconcile-result decisions, NEVER `artifacts/reconcile-session.json`. M14.2 doesn't touch reconcile, but `created_by` is read by M14.3's label composition — do NOT introduce any convenience that reads the session artifact in M14.2 prep.

### Validation gate

```
gofmt -l .
go build ./cmd/tpatch
go test ./...
go test ./assets/...   # parity guard
go test ./internal/workflow -run 'DependencyGate|CreatedBy|Recipe' -count=1 -v
```

### Workflow

1. Update CURRENT.md "Status: In Progress".
2. Read ADR-011 (D4, D5 especially), PRD §3.2, §3.4, parity guard test.
3. Add the recipe field + write the parity-guard-respecting tests FIRST. Run `go test ./assets/...`. (Get the parity guard green BEFORE adding the gate.)
4. Update the 6 skill formats in lockstep with the Go struct.
5. Implement `CheckDependencyGate` + tests. Wire into apply.
6. Run full validation gate.
7. 2-3 logical commits, all with the `Co-authored-by` trailer.
8. Push to `origin/main`.
9. Final CURRENT.md update flagging "ready for code-review sub-agent".

### Out-of-band reminder for the implementer

The repo's tpatch binary at root is NOT gitignored. After `go build ./cmd/tpatch`, delete the binary or build into `/bin/`. Don't commit it.

### Deferred behind M14.2

- M14.3 — Reconcile topo + composable labels + compound verdict (~500 LOC)
- M14.4 — `status --dag` + skills analyze-phase bullet + `docs/dependencies.md` + tag v0.6.0 (~300 LOC)

### Registered follow-ups (unchanged)

- `feat-ephemeral-mode` — depends on `feat-feature-import` + `feat-delivery-modes`
- `feat-feature-reorder` — depends on `feat-feature-dependencies` (i.e., M14)
- `feat-resolver-dag-context`, `feat-feature-autorebase`, `feat-amend-dependent-warning`
- `feat-skills-apply-auto-default`, `bug-record-roundtrip-false-positive-markdown`, `chore-gitignore-tpatch-binary`

---

## 2026-04-26 — M14.1 APPROVED, archiving handoff

# Current Handoff

## Active Task

- **Task ID**: M14.1 — Feature Dependencies data model + validation
- **Milestone**: M14 — Feature Dependencies / DAG (Tranche D, v0.6.0)
- **Status**: Review (ready for code-review sub-agent, completed 2026-04-24)
- **Assigned**: 2026-04-24

### Session Summary (2026-04-24)

Implemented the M14.1 data-model + validation slice, fully gated behind `features_dependencies` (default false). No user-visible behaviour change. All 5 PRD §3.3 validation rules covered with sentinel errors + tests; DFS cycle detection and Kahn topological order pure functions in `internal/store/dag.go`; round-trip byte-identity verified against a pre-M14 `status.json` fixture.

### Files Changed

- `internal/store/types.go` — added `Dependency` struct, kind constants, `DependsOn []Dependency` (omitempty) on `FeatureStatus`, `FeaturesDependencies bool` config field, `Config.DAGEnabled()` helper.
- `internal/store/dag.go` (new) — `DetectCycles`, `TopologicalOrder` (Kahn, deterministic), `Children`, `ErrCycle` sentinel. Pure, no IO. Doc comments enforce the ADR-010 D5 reminder for downstream readers.
- `internal/store/validation.go` (new) — `ValidateDependencies` + `ValidateAllFeatures`; sentinels `ErrSelfDependency`, `ErrDanglingDependency`, `ErrKindConflict`, `ErrSatisfiedByRequiresUpstream`, `ErrInvalidDependencyKind`.
- `internal/store/store.go` — repo `SaveConfig`/`parseYAMLConfig` now round-trip the flat `features_dependencies:` key.
- `internal/store/global.go` — global `renderGlobalYAML` and `mergeConfig` carry the same key (repo-true OR'd into global).
- `internal/store/dag_test.go` (new) — empty graph, isolated node, self-edge, 2-/3-node cycles, linear acyclic, diamond, deterministic topo (50 iters), Kahn cycle error path, `Children` ordering.
- `internal/store/validation_test.go` (new) — positive + negative cases for all 5 rules, plus `ValidateAllFeatures` surfacing all sentinels at once.
- `internal/store/roundtrip_test.go` (new) — pre-M14 fixture byte-identity, empty `depends_on` omit guard, populated `depends_on` round-trip, `Config.FeaturesDependencies` round-trip.
- `docs/handoff/CURRENT.md` — this update.

### Test Results

- `gofmt -l .` → clean
- `go build ./cmd/tpatch` → ok
- `go test ./...` → all packages pass (store 1.6s, cli 5.1s, workflow 12.2s).
- Targeted: `go test ./internal/store -run 'DAG|Cycle|Topo|Children|Validate|Roundtrip|Config_Features' -count=1 -v` → 30 cases, all PASS.

### Implementation choices (M14.1)

- **Config flag shape**: Option A (flat top-level key `features_dependencies: true|false`). Lower risk; works with existing flat YAML parser (`internal/store/store.go:497`). Nested `features:` block deferred — would force a parser rewrite for no semantic gain.
- **Flag wiring scope**: Flag parses + round-trips. No callers gate on it in M14.1 — apply/reconcile wiring lives in M14.2/M14.3.
- **Doc-comment guard**: `Dependency` and DAG types carry an explicit comment that `status.Reconcile.Outcome` is the authoritative reconcile result; `reconcile-session.json` is audit-only (per ADR-010 D5).

### Context

v0.5.3 shipped (`4636878`, `3ac7465`, `8a4af4b`, `6024942`, tag `v0.5.3`). All correctness baselines needed for M14 now in place:

- `workflow.AcceptShadow` is the single accept helper for shadow → real (v0.5.2) and stamps `Reconcile.Outcome=reapplied` (v0.5.3) — M14.3 label composition will read it.
- Resolver and reconcile have clean artifact ownership: `resolution-session.json` (per-file outcomes) vs `reconcile-session.json` (high-level summary).
- Recipe stale guard catches both HEAD and content drift.
- Index-dirty bug on refresh fixed.

No shipped feature currently exposes `depends_on` — M14.1 adds the data model behind `features.dependencies: true` config flag (default false).

### Authoritative docs (read before coding)

1. `docs/adrs/ADR-011-feature-dependencies.md` — **MUST READ**. Locks 9 decisions.
2. `docs/prds/PRD-feature-dependencies.md` — 736-line PRD (APPROVED WITH NOTES). §3.1 data model, §3.5 composable labels, §4.5 precedence, §6 milestone sizing, §7 acceptance criteria. Note §3.4 residual terminology drift — **always defer to ADR-011 + §4.5** when the two conflict.
3. `docs/ROADMAP.md` M14 section — sub-milestone boundaries.

### M14.1 scope (~300 LOC)

**Code additions**:
- `internal/store/types.go`: `Dependency` struct (`slug`, `kind` = `hard|soft`, optional `satisfied_by` for `upstream_merged`) added to `FeatureStatus` as `depends_on []Dependency`.
- `internal/store/dag.go` (new): DFS cycle detection + Kahn topological traversal over the feature set. Pure functions; no IO.
- `internal/store/validation.go` (new): 5 validation rules per PRD §3.3:
  1. No self-dependency.
  2. No cycles.
  3. No dangling refs (every `slug` must exist in the store).
  4. No kind conflict (same parent declared both hard and soft is rejected).
  5. `satisfied_by` only valid when parent state is `upstream_merged`.
- `internal/store/config.go` (or wherever config lives): `features.dependencies` bool flag, default false. All DAG code paths must no-op when flag is off.
- CLI plumbing: no user-visible commands in M14.1. Just make `add`/`status` round-trip the new field when the flag is on.

**Tests**:
- `dag_test.go`: cycle detection (direct self, 2-node, 3-node), topo order determinism (ties broken by slug), empty graph, single node.
- `validation_test.go`: each of 5 rules with positive and negative cases.
- Round-trip: add a feature with `depends_on`, reload from disk, verify equality.
- Feature-flag off: all new code paths bypassed; `status.json` schema unchanged byte-for-byte for pre-M14.1 fixtures.

**Not in M14.1** (belongs to M14.2+):
- Apply gate enforcement.
- `created_by` recipe op.
- Reconcile topological traversal.
- Composable DAG labels.
- `status --dag` output.
- Any of the 6 skill-format updates.

### Suggested approach

1. Read ADR-011 end to end, then PRD §3 and §4.5.
2. Sketch the `Dependency` struct + `FeatureStatus` additions.
3. Write `dag.go` + tests first (pure, fast iteration).
4. Write `validation.go` + tests.
5. Wire the config flag; ensure zero behavior change when flag is off.
6. Round-trip test from existing `status.json` fixtures to prove backward compat.

### Validation required

- `gofmt -l .` clean
- `go build ./cmd/tpatch`
- `go test ./...`

### Guardrails

- No scope creep into M14.2/.3/.4.
- No changes to the recipe JSON schema (that's M14.2 — gated by the parity guard).
- No new external Go dependencies.
- All commits must carry the `Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>` trailer.

### Deferred behind M14.1

- M14.2 — Apply gate + `created_by` recipe op + 6-skill parity-guard rollout (~250 LOC)
- M14.3 — Reconcile topological traversal + composable labels + compound verdict (~500 LOC)
- M14.4 — `status --dag`, skills analyze-phase bullet, `docs/dependencies.md`, tag v0.6.0 (~300 LOC)

### Registered follow-ups (unchanged from C3)

- `feat-ephemeral-mode` — depends on `feat-feature-import` + `feat-delivery-modes`
- `feat-feature-reorder` — depends on `feat-feature-dependencies` (i.e., M14)
- `feat-resolver-dag-context`, `feat-feature-autorebase`, `feat-amend-dependent-warning`
- `feat-skills-apply-auto-default`, `bug-record-roundtrip-false-positive-markdown`, `chore-gitignore-tpatch-binary`

---

## 2026-04-24 — Tranche C3 / v0.5.3 shipped

# Current Handoff

## Active Task

- **Task ID**: Tranche C3 / v0.5.3 — shadow accept accounting fixes (✅ **3/3 items landed on main; release task is supervisor's**)
- **Status**: ✅ Implementation + regression test landed on `origin/main`. Tag + CHANGELOG + version bump deferred to supervisor (per agent guardrails).
- **Blocks**: M14.1 — M14.3 reads `status.Reconcile.Outcome` for ADR-011 D6 label composition. C3 clears the baseline.
- **Previous**: Tranche C2 / v0.5.2 shipped ✅ — archived in `HISTORY.md`

### C3 scope — external reviewer surfaced 3 follow-ups on v0.5.2 shadow flow

All verified by code inspection:

| ID | Severity | Finding | Status |
|---|---|---|---|
| c3-separate-resolution-artifact | 🔴 Silent correctness (manual-accept regression) | Resolver writes `ResolveResult` (with `outcomes[]`) to `artifacts/reconcile-session.json`; reconcile.go:398 `saveReconcileArtifacts` overwrites with `ReconcileResult` (no outcomes); `loadResolvedFiles` reads outcomes → errors "no resolved files recorded". Fix: split into `resolution-session.json` (resolver) + `reconcile-session.json` (reconcile summary) | ✅ `4636878` |
| c3-manual-accept-regression-test | 🟡 Missing coverage | End-to-end shadow-awaiting → manual accept test. Counterpart to `TestGoldenReconcile_ResolveApplyTruthful` but for the manual path. Would have caught both other C3 findings in v0.5.2 | ✅ `8a4af4b` |
| c3-accept-stamps-reconcile-outcome | 🟡 Internal consistency (M14.3 blocker) | `AcceptShadow` marks `State=applied` but leaves `Reconcile.Outcome=shadow-awaiting`. M14.3 label composition (ADR-011 D6) reads `Reconcile.Outcome` — stale outcome → wrong DAG labels | ✅ `3ac7465` |

### Session Summary — 2026-04-24 — C3 fix pass complete

Resumed the partial C3 run (resolver-session split + CLI reader already staged)
and completed the three outstanding deliverables:

- **Split artifact fully landed** (`4636878`): `internal/workflow/resolver.go`
  (`persistSession`), `internal/cli/cobra.go` (`loadResolvedFiles` +
  `shadow-diff`), `resolver_test.go`, and the Notes string in
  `reconcile.go:tryPhase35` all point at `resolution-session.json`. Drift
  audit updated the matching copy in 5 skill/prompt/workflow assets plus
  `docs/agent-as-provider.md` and `docs/prds/PRD-provider-conflict-resolver.md`.
  CHANGELOG, HISTORY, ADR-010, and M12 milestone are left historical.
- **AcceptShadow now stamps Outcome** (`3ac7465`):
  `clearShadowPointerAndStamp` signature extended to `(s, slug, sessionID, phase)`;
  sets `Reconcile.Outcome = ReconcileReapplied` and `Reconcile.AttemptedAt`.
  Auto-apply path unchanged externally (outer `updateFeatureState` still writes
  the same value on top); manual `reconcile --accept` now leaves a truthful
  `Outcome=reapplied` in status.json.
- **Regression test** (`8a4af4b`): `TestGoldenReconcile_ManualAcceptFlow`
  in `internal/workflow/golden_reconcile_test.go` drives
  `RunReconcile(Resolve:true)` → parses `resolution-session.json` inline
  (mirroring `loadResolvedFiles`) → calls `workflow.AcceptShadow` → asserts
  merged content on disk, `State=applied`, `Reconcile.Outcome=reapplied`,
  `ShadowPath` cleared, shadow directory pruned. Guards all three C3 fixes
  together.

### Commits (pushed to `origin/main`)

- `4636878` — fix(workflow): split resolver artifact into resolution-session.json
- `3ac7465` — fix(workflow): AcceptShadow stamps Reconcile.Outcome=reapplied
- `8a4af4b` — test(reconcile): end-to-end shadow-awaiting → manual accept regression

### Test results

```
ok  	github.com/tesseracode/tesserapatch/assets
?   	github.com/tesseracode/tesserapatch/cmd/tpatch		[no test files]
ok  	github.com/tesseracode/tesserapatch/internal/cli
ok  	github.com/tesseracode/tesserapatch/internal/gitutil
ok  	github.com/tesseracode/tesserapatch/internal/provider
ok  	github.com/tesseracode/tesserapatch/internal/safety
ok  	github.com/tesseracode/tesserapatch/internal/store
ok  	github.com/tesseracode/tesserapatch/internal/workflow
```

`gofmt -l .` clean; `go build ./cmd/tpatch` succeeds.

### Files changed (drift audit — resolver context only)

Assets: `assets/skills/copilot/tessera-patch/SKILL.md`,
`assets/skills/cursor/tessera-patch.mdc`,
`assets/skills/windsurf/windsurfrules`,
`assets/workflows/tessera-patch-generic.md`,
`assets/prompts/copilot/tessera-patch-apply.prompt.md`
(Claude SKILL.md was already updated by the prior sub-agent).

Docs: `docs/agent-as-provider.md`,
`docs/prds/PRD-provider-conflict-resolver.md`.

Intentionally left historical: `CHANGELOG.md`, `docs/handoff/HISTORY.md`,
`docs/supervisor/LOG.md`, `docs/adrs/ADR-010-*.md`,
`docs/milestones/M12-*.md`, `docs/milestones/M4-reconciliation.md`
(the latter refers to the classical phase-4 reconcile summary, which
legitimately still writes to `reconcile-session.json`).

### Next Steps

1. **Supervisor**: run the code-review sub-agent on the three C3 commits.
2. **Supervisor**: tag `v0.5.3`, bump version string, and add the
   v0.5.3 heading to `CHANGELOG.md` (implementation agent was explicitly
   instructed not to do any of these three).
3. **Supervisor**: unblock M14.1 once the review verdict lands.

### Artifact naming (locked: Option A)

- `artifacts/resolution-session.json` — resolver-owned, per-file `Outcomes[]`
- `artifacts/reconcile-session.json` — reconcile-owned, high-level `ReconcileResult` (unchanged external contract)

### Deferred behind v0.5.3

- M14.1 Data model + validation (~300 LOC)
- M14.2 Apply gate + `created_by` + 6-skill rollout (~250 LOC)
- M14.3 Reconcile topo + composable labels + compound verdict (~500 LOC)
- M14.4 `status --dag` + skills + release v0.6.0 (~300 LOC)

M14.3 will extend `workflow.AcceptShadow` (with the C3-stamped outcome) for the `blocked-by-parent-and-needs-resolution` compound verdict. C2+C3 correctness baselines are prerequisites.

### Registered follow-ups (not in any tranche yet)

- `feat-ephemeral-mode` — one-shot add-feature with no tracking artifacts; depends on `feat-feature-import` + `feat-delivery-modes`
- `feat-feature-reorder` — flip parent-child in DAG; depends on `feat-feature-dependencies`
- `feat-resolver-dag-context` — parent-patch to M12 resolver
- `feat-feature-autorebase` — auto-rebase child on parent drift
- `feat-amend-dependent-warning` — stale-parent-* labels
- `feat-skills-apply-auto-default` — 6 skills still reference `--mode prepare/execute/done`
- `bug-record-roundtrip-false-positive-markdown` — `--lenient` fallback shipped; live repro pending
- `chore-gitignore-tpatch-binary` — trivial; bundle into next release

---

## 2026-04-23 — Tranche C2 / v0.5.2 shipped

# Current Handoff

## Active Task

- **Task ID**: Tranche C2 / v0.5.2 — correctness fix pass (**IMPLEMENTATION COMPLETE — awaiting supervisor review + release**)
- **Status**: ✅ 6/6 code+doc items landed on `main`; release task is supervisor's (see Next Steps)
- **Blocks**: M14.1 — cannot start data model work until reconcile `--resolve --apply` is truthful and the `refresh.go` path no longer dirties the user's index (M14.3 extends both)
- **Next on deck after C2**: ADR-011 ✅ done → M14.1 data model + validation

### C2 fix scope (7 items, verified real)

| ID | Severity | Finding |
|---|---|---|
| c2-resolve-apply-truthful | 🔴 Silent correctness bug | `--resolve --apply` sets `ReconcileReapplied` without copying shadow → real tree |
| c2-refresh-index-clean | 🟡 UX bug | `DiffFromCommitForPaths` leaves `git add -N` intent-to-add entries in user's index |
| c2-recipe-hash-provenance | 🟡 Guard incomplete | Stale guard catches HEAD drift but not recipe content drift |
| c2-remove-piped-stdin | 🟡 Contract drift | `printf y\| tpatch remove` refuses despite shipped contract saying piped stdin skips confirm |
| c2-amend-append-flag | 🟢 Feature add | Lock replace-default, add explicit `--append`, mutex with `--reset` |
| c2-max-conflicts-drift | 🟢 Doc drift | 6 sites claim default 3; code is 10 (CHANGELOG, agent-as-provider, 4 shipped skill/prompt files) |
| c2-release-v0.5.2 | supervisor | Tag after code-review sub-agent approves |

### Why before M14.1

1. Finding #1 is silent correctness on the v0.5.0 headline feature (`--resolve --apply`). Building DAG on top compounds the bug × N features in M14.3's Kahn traversal.
2. M14.3 extends `refresh.go` (finding #2's code) — fix the temp-index leak once, inherit clean plumbing.
3. The **shared accept-helper** extraction (finding #1's preferred fix) is the exact primitive M14.3's `blocked-by-parent-and-needs-resolution` compound verdict will need.
4. Skills max-conflicts drift will be re-touched by M14.2/M14.4 parity-guard rollouts anyway — cleaner to fix drift before the DAG adds 3 new label strings to the same skill files.

### Deferred decisions locked in PRD (for M14 reference)

1. `depends_on` in `status.json` only (no new `feature.yaml`, no migration)
2. DFS for cycle detection, Kahn's algorithm for operator traversal
3. `waiting-on-parent` + `blocked-by-parent` are **composable derived labels** (not states)
4. `created_by` recipe op gated by **hard deps only**
5. `upstream_merged` satisfies hard deps
6. Child's own reconcile verdict **always computed first**; parent labels overlay clean verdicts
7. `remove --cascade` required to delete parents with dependents — `--force` alone does NOT bypass
8. Parent-patch context **NOT** passed to M12 resolver in v0.6 (deferred to `feat-resolver-dag-context`)
9. All gated by `features.dependencies` config flag until v0.6.0 atomic flip

See `docs/adrs/ADR-011-feature-dependencies.md` for full rationale.

### Tranche D scope (v0.6.0, after C2)

| Milestone | Scope | Est. LOC |
|---|---|---|
| M14.1 | Data model + validation (Dependency struct, cycle DFS, 5 rules) | ~300 |
| M14.2 | Apply gate + `created_by` recipe op + 6-skill parity-guard rollout | ~250 |
| M14.3 | Reconcile topological traversal + composable labels + compound verdict | ~500 |
| M14.4 | `status --dag` + skills + release v0.6.0 | ~300 |

### Registered follow-ups (not in any tranche yet)

- `feat-ephemeral-mode` — one-shot add-feature mode with no tracking artifacts; depends on `feat-feature-import` + `feat-delivery-modes`
- `feat-feature-reorder` — flip parent-child in DAG; depends on `feat-feature-dependencies`
- `feat-resolver-dag-context` — parent-patch to M12 resolver
- `feat-feature-autorebase` — auto-rebase child on parent drift
- `feat-amend-dependent-warning` — stale-parent-* labels
- `feat-skills-apply-auto-default` — 6 skills still reference `--mode prepare/execute/done`; v0.5.1 flip not documented
- `bug-record-roundtrip-false-positive-markdown` — shipped `--lenient` fallback only; needs live repro for root-cause fix
- `chore-gitignore-tpatch-binary` — trivial one-liner; bundle into next release

## Session Summary — 2026-04-24 — C2 fix pass complete (6/6 code+doc items landed)

All 6 code/doc findings from the C2 correctness pass have landed on `main`. Remaining todo is the supervisor's release task (tag v0.5.2 + CHANGELOG heading) — implementation work is done.

### Commits (on `main`, after `f5e6d9e`)

| # | Finding | Commit |
|---|---|---|
| 1 | c2-max-conflicts-drift (docs: default 3 → 10 across 8 sites) | `36e058d` |
| 2 | c2-remove-piped-stdin (`printf y\|tpatch remove` now auto-yes on pipe) | `dbf7a31` |
| 3 | c2-amend-append-flag (add `--append`, mutex with `--reset`) | `1c6697e` |
| 4 | c2-refresh-index-clean (`DiffFromCommitForPaths` uses throwaway `GIT_INDEX_FILE`) | `bc938ec` |
| 5 | c2-recipe-hash-provenance (stale guard detects content drift via sha256) | `b5e1f88` |
| 6 | c2-resolve-apply-truthful (`--resolve --apply` actually copies shadow → real) | `73cd648` |

### Key design choices

- **Shared `workflow.AcceptShadow` helper** (new file `internal/workflow/accept.go`) now owns the accept sequence. Both `runReconcileAccept` (manual `--accept`) and the auto-apply branch in `reconcile.go`'s `tryPhase35` route through it — they cannot drift again. On mid-flight failure the shadow is preserved and the outcome flips to `ReconcileBlockedRequiresHuman` so the human can investigate.
- **`RecipeProvenance.RecipeSHA256` is a `*string` pointer** so legacy sidecars (no field) decode as `nil` and `warnRecipeStale` emits a one-line "predates recipe-hash guard" note instead of a false-positive stale warning. Forward-compatible.
- **`GIT_INDEX_FILE` approach for `DiffFromCommitForPaths`**: seed a `os.CreateTemp("", "tpatch-idx-*")` file from `.git/index` bytes, run both `git add -N` and `git diff` with `GIT_INDEX_FILE=<temp>`, delete on return. Zero leakage to the user's real index.
- **`canPromptForConfirmation` + `os.Pipe` in tests**: pipes report `false` (not a TTY), matching real `printf y|tpatch remove`. The existing `SetIn(strings.NewReader(...))` path still reports `true` via the `*os.File` type-check fallback, preserving existing test behavior.

### Fixed drift sites (8, not 6 — also found cursor + windsurf drifts)

`CHANGELOG.md`, `docs/agent-as-provider.md`, `assets/workflows/tessera-patch-generic.md`, `assets/prompts/copilot/tessera-patch-apply.prompt.md`, `assets/skills/copilot/tessera-patch-apply.md`, `assets/skills/cursor/tessera-patch.mdc`, `assets/skills/claude/tessera-patch.md`, `assets/skills/windsurf/.windsurfrules`.

## Files Changed (tranche C2 aggregate)

- `internal/workflow/accept.go` — **NEW** — shared `AcceptShadow` + `AcceptOptions` / `AcceptResult`.
- `internal/workflow/accept_test.go` — **NEW** — direct coverage + failure-path test.
- `internal/workflow/reconcile.go` — `ResolveVerdictAutoAccepted` branch rewired through `AcceptShadow`; failure → `BlockedRequiresHuman` + shadow preserved.
- `internal/workflow/implement.go` — `RecipeProvenance.RecipeSHA256 *string`; provenance now re-reads recipe and hashes it.
- `internal/workflow/refresh_test.go` — `TestRefreshAfterAcceptLeavesIndexClean` regression guard.
- `internal/workflow/golden_reconcile_test.go` — `TestGoldenReconcile_ResolveApplyTruthful` end-to-end guard.
- `internal/gitutil/gitutil.go` — `DiffFromCommitForPaths` uses `GIT_INDEX_FILE` throwaway.
- `internal/cli/cobra.go` — extended `warnRecipeStale` for HEAD + hash + legacy; `runReconcileAccept` rewritten as thin wrapper over `workflow.AcceptShadow`.
- `internal/cli/c1.go` — `amendCmd` gained `--append` + mutex with `--reset`; `removeCmd` skips prompt on piped stdin.
- `internal/cli/cobra_test.go` — stale-guard content-drift + legacy subtests, `TestRemovePipedStdinSkipsConfirmation`, `TestAmendAppendConcatenates`, `TestAmendAppendAndResetRejected`.
- 8 drift-fix sites (see list above).

## Test Results

- `gofmt -l .` — clean.
- `go build ./...` — ok.
- `go test ./...` — all packages green (assets, cli, gitutil, provider, safety, store, workflow).
- No new Go deps.

## Next Steps

1. **Supervisor**: dispatch code-review sub-agent for the 6 C2 commits (`36e058d..73cd648`).
2. **Supervisor** (on APPROVED): write `v0.5.2` heading in `CHANGELOG.md`, bump internal version string if present, commit as `release(v0.5.2)`, tag `v0.5.2`, push `main` + tag.
3. After v0.5.2 tag: archive this CURRENT entry to HISTORY and open the M14.1 data-model handoff.

## Blockers

None. C2 implementation is complete.

## Context for Next Agent

- **Do NOT run `go build ./cmd/tpatch` at repo root** — writes a bare `tpatch` binary not covered by `.gitignore` (registered follow-up `chore-gitignore-tpatch-binary`). Use `go vet + go test`.
- **`AcceptShadow` is the new single entry point** for anything that wants to promote a shadow into the real tree. Do not open-code the sequence in callers — use the helper.
- **`RecipeProvenance.RecipeSHA256` being a pointer is load-bearing**: if a future refactor flips it to a value type, legacy sidecars will appear stale and emit spurious warnings. Change only with a migration.
- **Auto-apply failure mode is `ReconcileBlockedRequiresHuman` with shadow preserved** (ADR-010 §D4). Tests `TestGoldenReconcile_ResolveApplyTruthful` and `TestAcceptShadowErrorsWithoutShadow` lock this in.

---

## Session Summary — 2026-04-23 — PRD approved, C2 fix pass opened

Supervisor-driven: after ADR-011 shipped, reviewer session surfaced 4 confirmed bugs + 2 doc drifts. Verified findings #1, #2, #6 via direct code inspection (resolver.go:218-222 comment is explicit; gitutil.go:689-697 leaks intent-to-add; 6 skill/doc sites claim max-conflicts default 3 against code's 10). Registered 7 C2 todos with dependencies; M14.1 blocked behind v0.5.2 release.

## Files Changed

- `docs/prds/PRD-feature-dependencies.md` — NEW — 736 lines
- `docs/ROADMAP.md` — M14 section populated
- `docs/supervisor/LOG.md` — PRD review cycle entry
- `docs/handoff/CURRENT.md` — this file, flipped to M14 scoping state

## Test Results

N/A — docs-only session.

## Next Steps

1. Draft ADR-011 (can be done as a sub-agent task or directly by supervisor — small scope).
2. Create `docs/milestones/M14-feature-dependencies.md` with the 4-sub-milestone contract.
3. Launch M14.1 implementation sub-agent once ADR-011 is in place.

## Blockers

None. ADR-011 is the only gating artifact before M14.1 coding starts.

## Context for Next Agent

- PRD review had **3 passes** and every pass improved the artifact materially — this is the pattern for non-trivial features. Budget review cycles, don't treat first-pass approval as the norm.
- Rubber-duck agent is highly effective at catching self-introduced contradictions in revisions. Always re-review after revisions.
- `m14.1-data-model` must not start until ADR-011 is committed — it's a repo rule per AGENTS.md.
- PRD has ONE non-blocking cleanup note: §3.4 still uses enum-style `ReconcileWaitingOnParent` / `ReconcileBlockedByParent` verdicts while §4.5 locks label semantics. ADR-011 should normalize (labels win).

### Post-release user testing

User did manual testing after release — no bugs reported. Removed the stray `tpatch` build artifact from repo root manually.

### Registered follow-ups (not in any tranche yet)

- **Skill-asset refresh for apply default flip** — all 6 skill formats + `docs/agent-as-provider.md` still reference `apply --mode prepare/execute/done` explicitly. New `--mode auto` default is not documented there. Low-priority polish; cluster with next skill touch.
- **`bug-record-roundtrip-false-positive-markdown`** — shipped `--lenient` fallback only. Real repro needed to root-cause. Re-open if a user reports live.
- **`.gitignore /tpatch`** — bare binary at repo root from `go build ./cmd/tpatch` is not gitignored. Trivial one-line fix bundled into next tranche.

## Session Summary — 2026-04-22 — Tranche C1 / v0.5.1 shipped

10 commits on `main`, pushed to `origin`. Tag `v0.5.1` pushed. All tests green. No new Go deps.

| # | Item | Commit |
|---|---|---|
| 1 | c1-recipe-stale-guard | `4f49c76` |
| 2 | c1-apply-default-execute | `3a12b2e` |
| 3 | c1-add-stdin | `d727ea2` |
| 4 | c1-progress-indicator | `5dba3b4` |
| 5 | c1-edit-flag | `1dbc812` |
| 6 | c1-feature-amend | `36587c9` |
| 7 | c1-feature-removal | `958e6d0` |
| 8 | c1-record-lenient | `5dae00b` |
| 9 | release(v0.5.1) | `e069cd8` + tag `v0.5.1` |
| 10 | supervisor log: C1 review — APPROVED | `c4cccb3` |

### Breaking UX

- `tpatch apply` default mode flipped from `prepare` to `auto`. Users relying on the previous behavior must pass `--mode prepare` explicitly.

### Notes for next agent

- **Item 8 shipped as fallback, not root-cause fix.** Three synthetic repros of `bug-record-roundtrip-false-positive-markdown` (trailing whitespace, new untracked markdown with `--intent-to-add`, modified tracked markdown) all passed reverse-apply cleanly. Without a live fixture, I shipped the documented `--lenient` escape hatch instead of a speculative `--ignore-whitespace` fix. If the bug resurfaces with a real repro, revisit.
- **Recipe provenance is a sidecar** (`artifacts/recipe-provenance.json`), not a field on `apply-recipe.json` — avoids changing all 6 skill formats + failing the strict `DisallowUnknownFields` parity guard.
- **Spinner lives at the single `GenerateWithRetry` choke point.** Any new LLM-calling code path gets the spinner for free if it goes through that function.
- **`.gitignore` does NOT ignore a bare `tpatch` binary at repo root.** Don't `go build ./cmd/tpatch` from the root — it writes a binary that gets picked up by `git add -A`. Use `go vet + go test` only.
- **Stdin detection pattern**: `stdinIsPiped` (permissive — true for tests that use `cmd.SetIn(strings.NewReader(...))`) for input; `canPromptForConfirmation` (inverse, requires real TTY) for destructive ops.

## Files Changed (tranche C1 aggregate)

- `internal/cli/cobra.go` — version bump, apply default mode flip, addCmd stdin, stale-guard, record --lenient, c1 subcommand registrations.
- `internal/cli/c1.go` — NEW — edit/amend/remove commands.
- `internal/cli/cobra_test.go` — tests for all C1 items + shared helpers.
- `internal/workflow/implement.go` — `RecipeProvenance` sidecar.
- `internal/workflow/spinner.go` (NEW) + `spinner_test.go` (NEW).
- `internal/workflow/retry.go` — spinner wired in `GenerateWithRetry`.
- `internal/store/store.go` — `RemoveFeature`.
- `CHANGELOG.md` — v0.5.1 section.
- `docs/ROADMAP.md` — M13 status flipped to ✅.
- `docs/handoff/CURRENT.md` + `docs/handoff/HISTORY.md` — archived.

## Test Results

- `gofmt -l .` — clean.
- `go vet ./...` — clean.
- `go test ./...` — all packages green.

## Next Steps

1. ✅ Supervisor review of C1 commits — APPROVED (see `docs/supervisor/LOG.md`).
2. ✅ Pushed `main` + tag `v0.5.1` to `origin`.
3. ⏭️ Pick next tranche from ROADMAP M14+ backlog (see supervisor proposal in latest chat turn).

## Blockers

None.

## Context for Next Agent

- All C1 commits are single-purpose and can be reverted individually if any one item is rejected in review.
- `--mode prepare` → `--mode auto` default flip is the only user-visible regression risk. Skill assets were NOT updated in this tranche (still say "apply --mode prepare/started/done") — worth a follow-up touch if the new default sticks.

---

## 2026-04-22 — Tranche C1 / v0.5.1 shipped

# Current Handoff

## Active Task

- **Task ID**: M13 / Tranche C1 / v0.5.1 — UX Polish & Quick Wins
- **Status**: 🔨 **In Progress — scoped, implementation prompt ready**
- **Milestone**: (inline — no separate milestone file for polish tranches)
- **Previous**: M12 / B2 / v0.5.0 shipped ✅ — archived below

### C1 scope (8 items, all low-risk)

| Todo ID | Type | Description |
|---------|------|-------------|
| `c1-apply-default-execute` | feat | `tpatch apply <slug>` without `--mode` runs prepare→execute→done in one shot; keep `--mode` for granular control |
| `c1-add-stdin` | feat | `tpatch add -` or pipe detection reads feature description from stdin |
| `c1-progress-indicator` | feat | Lightweight stderr spinner during LLM calls (zero-dep, stdlib only) |
| `c1-edit-flag` | feat | `tpatch edit <slug> [artifact]` opens feature artifacts in `$EDITOR` |
| `c1-feature-amend` | feat | `tpatch amend <slug> <new-description>` updates request.md, optionally resets state |
| `c1-feature-removal` | feat | `tpatch remove <slug> [--force]` deletes feature directory with confirmation |
| `c1-recipe-stale-guard` | bug | Warn when `apply-recipe.json` base commit doesn't match current HEAD |
| `c1-record-lenient` | bug | `tpatch record --lenient` skips reverse-apply check for whitespace-sensitive files |

### B2 progress

| Todo | Status | Commit | File(s) |
|---|---|---|---|
| `b2-shadow-worktree` | ✅ done | `8bd8eb6` | `internal/gitutil/shadow.go` + test |
| `b2-validation-gate` | ✅ done | `bf28b58` | `internal/workflow/validation.go` + test; `gitutil.HasConflictMarkers` exported |
| `b2-resolver-core` | ✅ done | `25b7774` | `internal/workflow/resolver.go` + test |
| `b2-reconcile-wiring` | ✅ done | `53b38ee` | `internal/workflow/reconcile.go` + `gitutil.FileAtCommit`/`MergeBase` + test |
| `b2-state-machine` | ✅ done | (this commit) | `StateReconcilingShadow` + `ReconcileSummary` shadow fields + `status` command surfaces shadow pointer + test |
| `b2-cli-flags` | ✅ done | `c022b19` | `reconcileCmd` + 7 flags + accept/reject/shadow-diff handlers + `validateReconcileFlags` + 2 tests |
| `b2-derived-refresh` | ✅ done | `1507b7a` | `FilesInPatch`/`ForwardApplyExcluding`/`DiffFromCommitForPaths` + `RefreshAfterAccept` + accept flow rewired + 4 tests |
| `b2-golden-tests` | ✅ done | (this commit) | `golden_reconcile_test.go` — 5 ADR-010 acceptance scenarios (clean-reapply / shadow-awaiting / validation-failed / too-many-conflicts / no-provider) |
| `b2-skills-update` | ✅ done | (this commit) | 6 skills + `docs/agent-as-provider.md` — Phase 3.5 section, `--resolve/--apply/--accept/--reject/--shadow-diff/--max-conflicts/--model` flags, `reconciling-shadow` state, `reconcile-session.json` schema, shadow worktree concept; parity guard green |
| `b2-release` | ✅ done | (this commit) | v0.5.0: version bump in `cobra.go`, CHANGELOG entry, git tag pushed |

SQL: `SELECT id, status FROM todos WHERE id LIKE 'b2-%' ORDER BY id;`

### What `b2-cli-flags` needs to do (NEXT)

Add flags to `reconcileCmd` in `internal/cli/cobra.go`:

- `--resolve` bool → `ReconcileOptions.Resolve`
- `--apply` bool → `ReconcileOptions.Apply` (requires `--resolve`)
- `--max-conflicts N` int → `ReconcileOptions.MaxConflicts`
- `--model NAME` string → `ReconcileOptions.Model`
- `--accept <slug>`, `--reject <slug>`, `--shadow-diff <slug>` — terminal operations; read `status.Reconcile.ShadowPath` (already populated by b2-state-machine). Mutually exclusive with `--resolve`.

Handler sketch:

- `--accept`: refuse if state != `reconciling-shadow`. Look up resolved_files from `reconcile-session.json`. Call `gitutil.CopyShadowToReal(shadow, root, files)`. Transition state to `applied` via `s.MarkFeatureState`. Add TODO note: "derived artifacts not yet refreshed — run `tpatch record` until b2-derived-refresh lands."
- `--reject`: `gitutil.PruneShadow(shadow)`. Roll state back to `applied`. Clear `status.Reconcile.ShadowPath`.
- `--shadow-diff`: walk resolved_files, shell out to `diff -u` per pair, stream to stdout.

Also: truthful validation errors for nonsensical combos (e.g. `--accept` + `--resolve`).

### What was in the old wiring guidance (preserved below for reference — all implemented)

1. **Trigger condition**: only when `PreviewForwardApply` returns `ForwardApply3WayConflicts` AND the caller set `ReconcileOpts.Resolve = true` (new field — add to the opts struct).
2. **Git plumbing** (new, needs a helper in gitutil or inline): for each conflicted file from the preview, fetch three versions:
   - `base` = file at the feature's base upstream commit (from `upstream.lock` or the patch's base)
   - `ours` = file after feature's patch is applied on `base` (either read from real working tree if currently on base+patch, OR synthesize: `git show <base>:<path>` + apply feature's post-apply.patch selectively).
   - `theirs` = `git show <upstreamCommit>:<path>`
   - Simplest v0.5.0 approach: use `git show <ref>:<path>` via `runGit` for base and theirs; for ours, read the file from the real working tree (reconcile runs after `tpatch apply` has put the feature on disk). Document the assumption.
3. **Call `RunConflictResolve`** with the gathered `ConflictInput`s and `upstreamCommit`. Pass through `ResolveOptions{AutoApply: opts.Apply, ModelOverride: opts.Model, MaxConflicts: opts.MaxConflicts, Validation: ValidationConfig{TestCommand: cfg.TestCommand, IdentifierCheck: true}}`.
4. **Map `ResolveResult` → `ReconcileResult`**: new `ReconcileOutcome` values mirror the resolver verdicts. Add `ShadowPath`, `ResolvedFiles`, `FailedFiles`, `SkippedFiles` to `ReconcileResult`.
5. **Preserve v0.4.4 `promoteIfMarkers`** on every Reapplied path that bypasses phase 3.5 (when `--resolve` is off). Already present; just make sure new branching doesn't orphan it.
6. **Skip phase 3.5 entirely** when forward-apply preview verdict is anything other than `3WayConflicts` — the resolver only exists to turn that verdict into something actionable.

### Key technical facts (for a fresh agent)

- **Module path**: `github.com/tesseracode/tesserapatch` (renamed from `tesserabox` on 2026-04-21).
- **Provider interface**: `provider.Provider{ Check, Generate }`. Resolver uses `Generate` only. `cfg.Configured()` is the "usable?" check.
- **Store API**: `s.ReadFeatureFile(slug, name)`, `s.WriteArtifact(slug, name, content)`, `s.LoadConfig()`, `s.Root` (repo root). Flat YAML config.
- **Shadow path**: `.tpatch/shadow/<slug>-<ts>/` where ts is `2006-01-02T15-04-05.000000Z`. Microsecond precision — required to avoid collisions on rapid recreate.
- **No heuristic fallback** (ADR-010 D9): when provider not configured, resolver returns `BlockedRequiresHuman` with per-file `provider-error` status. Never degrade silently.
- **Fence stripping**: use `stripResolverFences` (conservative whole-response regex), NOT `stripCodeFences` (JSON-lenient). Documented in resolver.go.
- **Validation**: `ValidateResolvedFile` runs markers + native-parse + identifier-preservation (opt-in). `RunTestCommandInShadow` is a SEPARATE call, run after all files resolve.
- **Session JSON**: written on EVERY path, including short-circuit verdicts (too-many-conflicts, no-provider). Auditability > optimization.
- **Parity guard**: `assets/assets_test.go` has `TestSkillRecipeSchemaMatchesCLI` with `DisallowUnknownFields`. Any skill edit that invents a field fails build. B2 skill update must extend the anchors + recipe schema carefully.

### Follow-ups registered (post-B2, later tranches)

- `feat-resolver-heuristic-fallback` — opt-in `--heuristic` for provider-unavailable cases. Depends on `b2-release`.
- `feat-feature-standalonify` — rebase a dependent feature into standalone. Depends on `feat-feature-dependencies`.
- `feat-parallel-feature-workflows` — `tpatch workon --parallel` fans out features into per-feature worktrees. Depends on `feat-feature-dependencies`.

### Bugs fixed in v0.5.0 alongside B2

- `bug-features-md-stale-state` — `FEATURES.md` not regenerated on state transitions from `apply --mode done` / `record` / etc. Fix: `SaveFeatureStatus` now calls `RefreshFeaturesIndex` unconditionally. Regression test: `TestSaveFeatureStatusRefreshesIndex`.

## Session Summary (2026-04-22 session — B2 derived-refresh + golden-tests)

**Commits this session** (continuing):
- `c022b19` — b2-cli-flags (prior)
- `3aab0c4` — docs checkpoint (prior)
- `1507b7a` — **b2-derived-refresh**: accept-flow correctness fix + atomic post-apply.patch regen + numbered reconcile patch + 4 tests
- (this commit) — **b2-golden-tests**: 5 ADR-010 PRD#6 acceptance scenarios

All pushed. `gofmt`, `go vet`, `go test ./...` clean.

### `b2-derived-refresh` fixed a real bug

The prior `--accept` only copied resolved (conflicted) files from the shadow.
Non-conflicted hunks from `post-apply.patch` were **never applied** to the real
tree, leaving the feature half-reconciled. New accept flow:

1. `ForwardApplyExcluding(patch, resolvedFiles)` — non-conflicted hunks land via 3-way
2. `CopyShadowToReal(resolvedFiles)` — resolver output overlays those paths
3. `RefreshAfterAccept` — regenerates post-apply.patch restricted to originally-touched files (via `git diff <upstreamCommit> -- <paths>` with `git add -N` first so new files appear); snapshots new patch as `patches/NNN-reconcile.patch`
4. `MarkFeatureState → applied`; prune shadow; clear status pointer

Explicitly deferred: `apply-recipe.json` regen (lossy from a raw diff);
documented in `refresh.go`. `tpatch record` remains the fallback.

### `b2-golden-tests` — 5 scenarios via `RunReconcile`

File: `internal/workflow/golden_reconcile_test.go`

| Scenario | Fixture | Expected outcome |
|---|---|---|
| clean-reapply | Non-conflicting feature vs unchanged upstream | `reapplied` / `upstreamed`, no shadow |
| shadow-awaiting | Conflict + provider returns clean merge | `shadow-awaiting`, 1 resolved, shadow populated |
| validation-failed | Conflict + provider returns content with `<<<<<<<` markers | `blocked-requires-human`, 1 failed |
| too-many-conflicts | 2 conflicted files, MaxConflicts=1 | `blocked-too-many-conflicts`, provider.calls==0 |
| no-provider | Conflict + nil provider + `--resolve` | `blocked-requires-human`, no shadow |

Pattern reuses `scriptedProvider` with `keyed` map for resolver calls + positional response for phase-3 semantic probe. Fixtures capture real `git diff --cached HEAD` output so `--3way` can locate the base blob.

## Session Summary (2026-04-22 session — B2 cli-flags)

**Commits this session** (continuing from b2-state-machine):
- `53b38ee` — `b2-reconcile-wiring` (prior)
- `1767c1d` — `b2-state-machine` (prior)
- `6229203` — docs checkpoint (prior)
- (this commit) — `b2-cli-flags`: 7 new `tpatch reconcile` flags + 3 terminal handlers (accept/reject/shadow-diff) + mutex validation + 2 tests

All pushed. All tests green. `gofmt`, `go vet` clean.

### What `b2-cli-flags` shipped

- `--resolve`, `--apply`, `--max-conflicts`, `--model` → wired into `ReconcileOptions` struct
- `--accept <slug>`: reads `reconcile-session.json`, copies resolved files via `gitutil.CopyShadowToReal`, transitions state to `applied`, prunes shadow, clears status pointer. TODO emitted pointing to `tpatch record` (derived-refresh deferred)
- `--reject <slug>`: prunes shadow, rolls state back to `applied` if parked in `reconciling-shadow`
- `--shadow-diff <slug>`: non-destructive; streams `gitutil.ShadowDiff` to stdout
- `validateReconcileFlags`: rejects terminal-op combos + `--apply` without `--resolve`
- Safety: terminal ops never call `openStoreFromCmd` before flag validation

## Session Summary (2026-04-22 session — B2 middle)

**Commits this session** (continuing from B2 kickoff):
- `ed8457b` — docs: checkpoint B2 progress in CURRENT.md
- `53b38ee` — `b2-reconcile-wiring` (reconcile.go + gitutil.FileAtCommit/MergeBase + 1 test)
- `1767c1d` — `b2-state-machine` (StateReconcilingShadow + ReconcileSummary fields + status surface + 1 test)

All pushed to origin/main. All tests green.

## Session Summary (2026-04-21 evening session — B2 kickoff)

**Commits this session** (post-v0.4.4):
- `a6bd734` — docs: scope M12 / Tranche B2 (PRD + milestone + ROADMAP + CURRENT)
- `8bd8eb6` — `b2-shadow-worktree` (gitutil/shadow.go + 7 tests)
- `bf28b58` — `b2-validation-gate` (workflow/validation.go + 10 tests; gitutil.HasConflictMarkers exported)
- `25b7774` — `b2-resolver-core` (workflow/resolver.go + 6 tests)

All green: `gofmt -l .` clean, `go vet ./...` clean, `go test ./...` pass.

---

## Prior session summary (v0.4.4 + org rename)

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

## Archived 2026-04-20 — v0.4.2 Tranche A handoff (superseded by B1 --manual flag landing)

# Current Handoff

## Active Task

- **Task ID**: v0.4.2 released — Tranche A "Truthful Errors" complete
- **Milestone**: All 10 Tranche A items (A1–A10) landed + `docs/{record,feature-layout,reconcile}.md` shipped.
- **Status**: Ready to tag `v0.4.2`. No open Tranche A work.
- **Next**: Tranche B kickoff — headline is `feat-provider-conflict-resolver`. Full backlog in session SQL `todos` table (32 pending feature/improvement todos).

## Session Summary

One full v0.4.2 release cycle landed in this session:

- **A1 bug-implement-silent-fallback** — `Config.MaxTokensImplement` knob (default 16384, was hard-coded 8192). New `WarnWriter io.Writer = os.Stderr` in `internal/workflow/implement.go`; fallback emits a stderr warning naming retry count, error, raw-response path, and the config knob.
- **A2 bug-cycle-state-mismatch** — `RunImplement` writes `StateImplementing`. `assertCycleState` + `featureStateRank` check every phase transition in `internal/cli/phase2.go`.
- **A3 bug-record-validation-false-positive** — new `gitutil.ValidatePatchReverse`. Record now validates round-trip against the tree it captured from; forward validation stays for reconcile.
- **A4 bug-reconcile-phase4-false-positive** — new `gitutil.PreviewForwardApply` runs `--3way` in an isolated `git worktree` and classifies `Strict | 3WayClean | 3WayConflicts | Blocked`. Conflicts promote to `ReconcileBlocked`.
- **A5 bug-skill-invocation-clarity** — three canonical top-of-file blocks (Invocation / Phase Ordering / Preflight) in all 6 skill formats. Parity guard (`assets/assets_test.go`) enforces anchor phrases — wording can't drift.
- **A6 bug-provider-set-global** — `tpatch provider set` defaults to the global config; `--repo` for per-repo override. New `TestMain` in `internal/cli/phase2_test.go` redirects `XDG_CONFIG_HOME` so tests cannot clobber the developer's machine config.
- **A7 bug-extract-json-robustness** — one `ExtractJSONObject` helper replaces four extractors. Brace-balanced, string-aware, handles trailing prose / nested / arrays / escaped quotes / fences. 11-case table test.
- **A8 doc-record-timing** — `tpatch record` refuses clean-tree-no-`--from` with a "captured 0 bytes" diagnostic + up to 10 `git log` candidates. New helpers: `gitutil.RecentCommits`, `gitutil.IsWorkingTreeDirty`. Plus `docs/record.md` + skill one-liner.
- **A9 doc-patches-vs-artifacts** — `docs/feature-layout.md` with the "canonical vs audit trail" callout. `tpatch record` prints a cleanup hint past 6 patches. CLI subcommand (`tpatch patches`) + dedup deferred to v0.5.x (`feat-patches-subcommand`, `feat-record-dedup-patches`).
- **A10 doc-reconcile-workflow** — new `gitutil.PreflightReconcile` + `ReconcilePreflight` struct. `tpatch reconcile` refuses dirty trees / conflict markers / `*.orig|*.rej`. New flags: `--preflight`, `--allow-dirty`. Untracked-`.tpatch/` tip. `docs/reconcile.md` + skill one-liner.

### Version / release

- `internal/cli/cobra.go`: `const version = "0.4.2"`.
- `CHANGELOG.md`: new file, v0.4.2 section written.
- Commit + tag `v0.4.2` pending at time of handoff write.

## Files Changed (net vs v0.4.1)

New files:
- `CHANGELOG.md`
- `docs/record.md`
- `docs/feature-layout.md`
- `docs/reconcile.md`
- `internal/workflow/jsonextract.go` + `jsonextract_test.go`
- `internal/workflow/implement_test.go` (A1/A2)
- `internal/gitutil/gitutil_test.go` (A3/A4/A10)

Substantial edits:
- `internal/cli/cobra.go` — record empty-capture refusal, reconcile preflight + flags, `providerSetCmd` global default, version bump.
- `internal/cli/phase2.go` — `assertCycleState`, `featureStateRank`.
- `internal/cli/phase2_test.go` — `TestMain` XDG isolation, 3 new regression tests.
- `internal/gitutil/gitutil.go` — `ValidatePatchReverse`, `PreviewForwardApply`, `RecentCommits`, `IsWorkingTreeDirty`, `IsPathTracked`, `PreflightReconcile`.
- `internal/workflow/implement.go` — `WarnWriter`, state transition fix, MaxTokens knob, `ExtractJSONObject` migration.
- `internal/workflow/workflow.go`, `retry.go`, `reconcile.go` — migrated to `ExtractJSONObject`.
- `internal/store/{types,store,global}.go` — `MaxTokensImplement` knob.
- All 6 skill files (Claude / Copilot / Cursor / Windsurf / Generic / prompt) — 3 canonical blocks + 2 one-liners (record timing, reconcile clean tree).
- `assets/assets_test.go` — `requiredAnchors` list (10 anchors total).

## Test Results

```
$ gofmt -l .
(clean)

$ go build ./cmd/tpatch
(clean)

$ go test ./...
ok  	.../assets              0.469s
ok  	.../internal/cli        0.945s
ok  	.../internal/gitutil    1.486s
ok  	.../internal/provider   (cached)
ok  	.../internal/safety     (cached)
ok  	.../internal/store      (cached)
ok  	.../internal/workflow   2.124s
```

## Next Steps

1. Single commit with all v0.4.2 changes + co-author trailer; tag `v0.4.2`; push.
2. Begin Tranche B. Top of the backlog: **`feat-provider-conflict-resolver`** — a dedicated LLM-assisted resolver that can process phase-4 3-way conflicts instead of bubbling them up as `blocked`. Natural fit with `feat-soft-recipe-mode` (guidance recipes reconcile more easily).
3. Secondary Tranche B candidates (from session SQL):
   - `feat-feature-amend` — amend an already-recorded feature from an in-tree edit.
   - `feat-noncontiguous-feature-commits` — per-feature commit ledger for features that span discontiguous commits.
   - `feat-init-skill-drift` — apt/dpkg-style skill reconciliation on re-init.
   - `feat-max-tokens-uncapped` — research OpenRouter / LiteLLM / OpenCode conventions before deciding.
4. Stretch (v0.6.0): `feat-ci-cd-integration`, `feat-autoresearch-iterate-until-green`, `feat-delivery-modes`.

## Blockers

None.

## Context for Next Agent

- Session SQL is the authoritative task tracker. 29 pending todos, 49 done at this point.
- All three new docs in `docs/` (`record.md`, `feature-layout.md`, `reconcile.md`) cross-link to each other and `SPEC.md`. When adding another lifecycle doc, follow the same Related section pattern.
- The parity guard (`assets/assets_test.go` `requiredAnchors`) is now the enforcement surface for "what must all skill files say verbatim". When adding a skill block, add an anchor here or it will silently drift.
- `TestMain` in `internal/cli/phase2_test.go` redirects `XDG_CONFIG_HOME`. Any new CLI test that writes provider / global config MUST run in the `internal/cli` package (not elsewhere) to inherit that isolation.
- Reconcile preflight is now a hard gate. When writing tests that exercise reconcile phases, stage a fully clean tree first OR pass `--allow-dirty`.
- The `WarnWriter` pattern (see implement.go) is the convention for non-fatal workflow warnings. Swappable in tests via `prev := WarnWriter; WarnWriter = &buf; defer func() { WarnWriter = prev }()`.

## Archived 2026-04-18 — M11 handoff (superseded by v0.4.2 Tranche A)

# Current Handoff

## Active Task
- **Task ID**: M11 — Native Copilot provider (ADR-005)
- **Milestone**: M11 delivered
- **Description**: First-party Go provider speaking directly to `api.githubcopilot.com`. Mirrors the copilot-api/litellm pattern: device-code OAuth → session-token exchange → editor headers.
- **Status**: Implemented; awaiting supervisor review.
- **Assigned**: 2026-04-18

## Session Summary

1. **Auth store** (`internal/provider/copilot_auth.go`) — schema
   `{version, oauth, session}`, atomic write at `$XDG_DATA_HOME/tpatch/copilot-auth.json`
   with 0600 perms, rejects symlinks + world/group-writable parent dirs, tightens
   file perms on load, `TPATCH_COPILOT_AUTH_FILE` env override for tests,
   `authStoreMu` serialises writes + refreshes.
2. **Device-code flow** (`internal/provider/copilot_login.go`) — `RequestDeviceCode`,
   `PollAccessToken` (honours `authorization_pending`, permanent `slow_down` bump,
   `expired_token`, `access_denied`, local deadline + ctx cancel, always sends
   `Accept: application/json`), `ExchangeSessionToken` (+ `…Locked` variant used
   by the provider's retry-on-401 path). Client ID `Iv1.b507a08c87ecfe98`
   matches copilot-api.
3. **Editor headers** (`internal/provider/copilot_headers.go`) — version
   constants tracking copilot-api 0.26.7, `x-request-id` uuid, `TODO(adr-005)`
   to refresh when upstream bumps.
4. **Provider impl** (`internal/provider/copilot_native.go`) — `CopilotNative`
   satisfies `Provider`. `Check` never initiates device flow (returns
   `errCopilotUnauthorized` if no auth file). `Generate` proactively refreshes
   the session 60s before expiry, retries once on 401 with a forced refresh,
   then fails. Routes via `auth.Session.Endpoints["api"]` verbatim (D5).
5. **Registry** — `provider.NewFromConfig` dispatches
   `CopilotNativeType = "copilot-native"`. `Config.Configured()` relaxed for
   copilot-native so `Model` alone is enough (`BaseURL` comes from the auth
   file). New `Config.Initiator` field plumbed through `store.ProviderConfig`,
   the YAML parser, `SaveConfig`, and `renderGlobalYAML`.
6. **Opt-in gate** — `store.AcknowledgeCopilotNativeOptIn`,
   `store.CopilotNativeOptedIn`, plus `CopilotNativeOptIn` + `…At` fields
   written to **global config only** (same class as `CopilotAUPAckAt`) so they
   don't leak via repo clones. Enforced in `providerSetCmd`, `config set`
   (`provider.type=copilot-native`), and implicitly in auto-detect (which never
   lists copilot-native as a candidate).
7. **CLI** (`internal/cli/copilot_native.go`) — `provider copilot-login`
   (enterprise prompt, device flow, AUP notice), `provider copilot-logout`
   (deletes auth file). Re-uses AUP language from M10.
8. **Config set** — `config set provider.copilot_native_optin true` routes
   to `SaveGlobalConfig` (rubber-duck #3); `config set provider.initiator`
   validates `""|user|agent`.
9. **Preset** — `--preset copilot-native` in `providerPresets` (empty
   BaseURL, default model `claude-sonnet-4`, empty AuthEnv).
10. **Version bump** — `0.4.0-dev`.
11. **Docs** — new `docs/faq.md` (macOS `~/Library/Application Support`
    caveat + `XDG_CONFIG_HOME` override + auth-file locations); harness
    doc `docs/harnesses/copilot.md` gains "Native path (experimental,
    opt-in)" section; ROADMAP M11 marked ✅.

## Files Created
- `internal/provider/copilot_auth.go`
- `internal/provider/copilot_login.go`
- `internal/provider/copilot_headers.go`
- `internal/provider/copilot_native.go`
- `internal/cli/copilot_native.go`
- `docs/faq.md`

## Files Modified
- `internal/provider/provider.go` — `Config.Initiator`, relaxed `Configured()`
- `internal/provider/anthropic.go` — `NewFromConfig` dispatches copilot-native
- `internal/store/types.go` — `CopilotNativeOptIn` + `…At`, `ProviderConfig.Initiator`, relaxed `ProviderConfig.Configured()`
- `internal/store/store.go` — YAML parse/emit for new fields
- `internal/store/global.go` — global opt-in render + merge + helpers
- `internal/cli/cobra.go` — preset, type flag, opt-in gate, config-set routing, version bump
- `internal/cli/copilot.go` — pipes `Initiator` into `provider.Config`
- `docs/harnesses/copilot.md` — native path section
- `docs/ROADMAP.md` — M11 marked ✅

## Test Results

```
$ go test ./... -count=1
ok  github.com/tesseracode/tesserapatch/assets
ok  github.com/tesseracode/tesserapatch/internal/cli
ok  github.com/tesseracode/tesserapatch/internal/provider
ok  github.com/tesseracode/tesserapatch/internal/safety
ok  github.com/tesseracode/tesserapatch/internal/store
ok  github.com/tesseracode/tesserapatch/internal/workflow
$ go build ./cmd/tpatch
# binary reports 0.4.0-dev
```

## Next Steps
1. Supervisor review per `AGENTS.md` cadence → approve → tag `v0.4.0`
   so the CI release job publishes notes.
2. Live smoke test against a real GitHub account with Copilot entitlement:
   - `tpatch config set provider.copilot_native_optin true`
   - `tpatch provider copilot-login`
   - `tpatch provider set --preset copilot-native`
   - `tpatch provider check`
   - full `tpatch cycle` of a toy feature.
3. Follow-up: add provider-level unit tests with an httptest fake for
   the device flow + session exchange + 401 retry (scaffolded but not
   included in this cut to keep the diff surgical).

## Blockers
None. Editor-header policy is a known unknown per ADR-005 OQ1; we ship
with editor headers until GitHub publishes an official compatibility
endpoint.

## Context for Next Agent
- `CopilotAuthFilePath()` returns `(string, error)` — don't call it as a
  single-value expression.
- `ExchangeSessionToken(ctx, opts, auth)` **mutates `auth` in place** and
  returns only `error`. That's intentional: the provider's retry-on-401
  path needs to refresh the in-memory struct without re-reading the file
  before writing.
- `CopilotSessionBlock.Endpoints["api"]` is the routing root. Treat it as
  opaque — don't parse or reconstruct it.
- `authStoreMu` guards **both** the file and `exchangeSessionTokenLocked`;
  always call `ExchangeSessionToken` (the public wrapper) unless you
  already hold the mutex.
- macOS + `os.UserConfigDir()` resolves to `~/Library/Application Support/tpatch/`.
  Documented in `docs/faq.md`; users who want XDG layout set
  `XDG_CONFIG_HOME`.

---

# Handoff History

*Completed handoff entries are archived here in reverse chronological order.*

---

## 2026-04-17 — Distribution Setup (module rename + CI workflow) (v0.3.0)

**Task**: Enable 'go install' + add free CI workflow
**Agent**: Distribution agent
**Verdict**: APPROVED — committed as dc42718 + 305781d, tagged v0.3.0

## Session Summary

Two operational follow-ups:

1. **Module path fixed to match repo** — `go.mod` said `github.com/tesseracode/tpatch` while the GitHub repo is `tesseracode/tesserapatch`. That mismatch blocks `go install`. Renamed the module and all imports to `github.com/tesseracode/tesserapatch` (user-selected option). The binary is still called `tpatch` because Go names installed binaries after the final path segment (`cmd/tpatch`).
2. **CI workflow added** — `.github/workflows/ci.yml` runs on push and PR to `main`. It sets up Go via `go-version-file: go.mod` (so CI tracks local dev), checks formatting with `gofmt`, runs `go vet`, builds, tests, and runs an install smoke test. Matrix on `ubuntu-latest` + `macos-latest`. Concurrency group cancels superseded runs to save minutes. Free for public repos.
3. **README install block updated** — now points to the correct module path.

## Files Changed
- `go.mod` — `module github.com/tesseracode/tesserapatch`.
- All `.go` files under `cmd/`, `internal/`, `assets/` — import paths rewritten.
- `.github/workflows/ci.yml` — new CI workflow.
- `README.md` — install instructions updated.

## Test Results
- `gofmt -l .` — clean
- `go test ./... -count=1` — **ALL PASS** across 7 packages
- `go build -o tpatch ./cmd/tpatch` — OK
- `./tpatch --version` → `tpatch 0.3.0-dev`

## Post-Merge Checklist (for the repo owner)
1. Make the repo public (required for `go install` without auth and for free unlimited Actions minutes).
2. Push to `main`; CI should pass on both ubuntu + macOS.
3. Tag a release: `git tag v0.3.0 && git push origin v0.3.0`. `go install ...@latest` will then resolve to that tag.
4. Verify from a clean machine: `go install github.com/tesseracode/tesserapatch/cmd/tpatch@latest`.

## Provider Preset Clarification
`tpatch provider set --preset copilot` targets `http://localhost:4141` with `auth_env: GITHUB_TOKEN`. That is the **copilot-api proxy** endpoint, not the Copilot CLI auth itself. To use the same Copilot subscription as `copilot-cli`:

- Install and run `copilot-api` locally (it does the GitHub OAuth and exposes an OpenAI-compatible endpoint on 4141).
- Then `tpatch provider set --preset copilot` just works.

There is no direct-to-GitHub-Copilot path today because GitHub has not published a public OpenAI-compatible Copilot endpoint. If that changes, we add another preset.

## Blockers
None.

## Next Steps
1. Push + make repo public + tag v0.3.0.
2. Confirm CI green on first main push.
3. Optional: add a `release.yml` workflow with goreleaser for prebuilt binaries (not required for `go install`).


---

## 2026-04-17 — Phase 2 Refinement: SDK Evaluation + Harness Guides + Tracking Cadence (v0.3.0-dev)

**Task**: Evaluate mainstream Go SDKs and agent CLIs; adopt simplest integration; tighten tracking cadence
**Agent**: Phase 2 refinement agent
**Verdict**: SUPERSEDED by 2026-04-17 distribution setup entry (see LOG.md)

## Session Summary

Iterated on the Phase 2 M7–M9 output after the user asked us to survey reference implementations and not waste resources on unneeded SDKs.

1. **SDK evaluation (ADR-003)** — Surveyed `OpenRouterTeam/go-sdk` (Speakeasy-generated, README marks non-production), `openai/openai-go`, `anthropics/anthropic-sdk-go`. Decided to keep stdlib providers because: (a) our surface is `Check` + `Generate` only, (b) OpenRouter is drop-in OpenAI-compatible, (c) SDKs would add ~20 transitive deps for zero new capability. Positioned `openai/codex` and `github/copilot-cli` as *harnesses* (callers of tpatch), not providers.
2. **Presets for API parity** — Added `tpatch provider set --preset copilot|openai|openrouter|anthropic|ollama` backed by a single `providerPresets` map. Refactored `autoDetectProvider` to reuse the same map so there is one source of truth. Preset composes with explicit flag overrides (e.g. `--preset anthropic --model claude-opus-4`). Invalid presets fail loudly.
3. **Harness integration guides** — Wrote `docs/harnesses/codex.md` and `docs/harnesses/copilot.md` explaining the `tpatch next --format harness-json` contract, example sessions, recommended allow-lists, and anti-patterns (do not let the harness re-implement workflow phases).
4. **Tracking cadence** — Rewrote "Context Preservation Rules" in `AGENTS.md` with an enforced cadence cheatsheet (trigger → update). Updated `CLAUDE.md` Working Rules to reference the cadence. Key directive: "A task is not complete until tracking reflects its state."

## Files Created
- `docs/adrs/ADR-003-sdk-evaluation.md` — SDK evaluation decision, matrix, rationale.
- `docs/harnesses/codex.md` — Codex CLI integration guide.
- `docs/harnesses/copilot.md` — GitHub Copilot CLI integration guide.

## Files Changed
- `internal/cli/cobra.go` — `providerPresets` map; `--preset` flag on `provider set`; auto-detect refactored to reuse presets.
- `internal/cli/phase2_test.go` — New `TestProviderSetPreset` covering openrouter/anthropic/unknown.
- `AGENTS.md` — Stronger "Context Preservation Rules" with cadence cheatsheet.
- `CLAUDE.md` — Working Rules point to cadence; explicit per-phase tracking requirement.

## Test Results
- `go test ./...` — **ALL PASS** (7 packages)
- `gofmt -l .` — **CLEAN**
- `go build -o tpatch ./cmd/tpatch` — **OK** (v0.3.0-dev)
- Manual verification:
  ```
  tpatch provider set --preset openrouter
  → type: openai-compatible, url: https://openrouter.ai/api, auth_env: OPENROUTER_API_KEY
  ```

## Key Decisions Locked In
- **No third-party provider SDKs.** Stdlib stays the provider layer.
- **`providerPresets` is the single source of truth.** Adding a new vendor = one map entry.
- **Harnesses (codex, copilot) call tpatch via CLI + JSON.** No SDK embed on either side.
- **Tracking updates are enforced per phase, not per session.**

## Blockers
None.

## Next Steps
1. Live smoke test with `codex exec` and `copilot` once an environment with both installed is available — confirm the handshake matches the guide.
2. Consider M10 (`tpatch mcp serve`) to expose the same state machine via MCP for Copilot CLI. Tracked as a follow-up only; not in the current ADR scope.
3. Supervisor review + roadmap update for this refinement pass.

## Context for Next Agent
- The preset map lives in `internal/cli/cobra.go` just below `providerSetCmd()`. Keep `--preset` and `autoDetectProvider` using the same map.
- Harness guides assume a repo-level `AGENTS.md` for codex and a `.github/copilot/cli/skills/tessera-patch/SKILL.md` for copilot-cli. Both are created by copying from the `.tpatch/steering/` outputs of `tpatch init`.
- ADR-003 explicitly lists the triggers that would cause us to reconsider adopting SDKs (streaming, non-standard schemas, official harness client libraries).
- Prior Phase 2 handoff (M7/M8/M9 initial) has been archived to `docs/handoff/HISTORY.md` under a 2026-04-17 entry.


---

## 2026-04-17 — M7 + M8 + M9 Phase 2 Implementation (v0.3.0-dev)

**Task**: Ship Phase 2 milestones (provider integration, LLM validation+retry, interactive/harness commands)
**Agent**: Phase 2 implementation agent
**Verdict**: APPROVED WITH NOTES (subsumed by 2026-04-17 refinement — see CURRENT.md)

## Session Summary

Implemented M7–M9 end-to-end:

1. **M7** — Added `AnthropicProvider` (`internal/provider/anthropic.go`) speaking the Messages API. Introduced `provider.NewFromConfig()` factory selecting by `cfg.Type`. Extended auto-detection to probe Ollama (localhost:11434), `ANTHROPIC_API_KEY`, and `OPENROUTER_API_KEY`. Added `provider set --type` flag and `provider.type` validation. Wrote `docs/adrs/ADR-002-provider-strategy.md` documenting the decision and live-probe evidence for copilot-api; Ollama/OpenRouter confirmed compatible via existing OpenAI-compat provider (no code changes required).
2. **M8** — Added `GenerateWithRetry` in `internal/workflow/retry.go` with pluggable validators. `JSONObjectValidator` strips fences and round-trips the payload; `NonEmptyValidator` guards define/explore. Each attempt logs to `artifacts/raw-<phase>-response-N.txt`. Retries reissue the prompt with a corrective suffix describing the validator error. `max_retries` added to `config.yaml` (default 2), `--no-retry` flag added to analyze/define/explore/implement, context-keyed via `workflow.WithDisableRetry` to avoid signature churn.
3. **M9** — Shipped three new commands: `cycle` (batch and `--interactive` with `--editor` and `--skip-execute` options), `test` (runs `config.test_command`, records outcome in `apply-session.json` + `artifacts/test-output.txt`), `next` (emits next action as plain text or `--format harness-json`). Registered in root, version bumped to `0.3.0-dev`. All 6 skill formats updated to include `cycle`/`test`/`next`. Parity guard extended.

## Files Created
- `internal/provider/anthropic.go` — Anthropic Messages provider + `NewFromConfig` factory
- `internal/provider/anthropic_test.go` — Anthropic + factory tests
- `internal/workflow/retry.go` — `GenerateWithRetry`, validators, context flag
- `internal/workflow/retry_test.go` — retry-path tests
- `internal/cli/phase2.go` — `cycle`, `test`, `next` commands
- `internal/cli/phase2_test.go` — integration tests for the new commands
- `docs/adrs/ADR-002-provider-strategy.md` — provider strategy decision

## Files Changed
- `internal/cli/cobra.go` — factory wiring, `--type` flag, `--no-retry` on 4 workflow commands, auto-detect extensions, config `max_retries`/`test_command` keys, version bump
- `internal/store/types.go` — `Config` gains `MaxRetries` and `TestCommand`
- `internal/store/store.go` — default config.yaml template + `SaveConfig` + `parseYAMLConfig` cover the new fields
- `internal/workflow/workflow.go` — analyze/define/explore call `GenerateWithRetry`
- `internal/workflow/implement.go` — implement calls `GenerateWithRetry`
- `assets/skills/*` + `assets/workflows/*` + `assets/prompts/*` — all 6 formats list the three new commands
- `assets/assets_test.go` — parity guard requires `cycle`, `test`, `next`
- `docs/ROADMAP.md` — M7/M8/M9 marked complete

## Test Results
- `go test ./...` — **ALL PASS** across 7 packages
- `gofmt -l .` — **CLEAN**
- `go build -o tpatch ./cmd/tpatch` — **OK** (v0.3.0-dev)
- Smoke test: `init` → `add` → `next --format harness-json` → `cycle --skip-execute` → `config set test_command echo hi` → `test` — all succeed end-to-end

## Noteworthy Details
- `Provider` interface unchanged (still `Check` + `Generate`). Adding providers is purely additive.
- Retry is disabled when no provider is configured (existing heuristic fallback untouched).
- `tpatch next` is state-aware: for `defined` features it further distinguishes "needs explore", "needs implement", or "needs apply" by probing the feature directory.
- `--no-retry` plumbing uses `context.WithValue` to avoid changing every workflow signature.
- Auto-detection order: copilot-api → Ollama → Anthropic (via env) → OpenAI (via env) → OpenRouter (via env).

## Blockers
None.

## Next Steps
1. Run live bug bash against copilot-api with retry enabled (ideally against a degraded-model path to exercise the corrective prompt).
2. Consider streaming/tool-use support as an optional capability interface when a future milestone needs it.
3. Consider harness integration guides (M9.10, M9.11) — deferred; the skill files and `tpatch next --format harness-json` already provide the contract.


---

## 2026-04-16 — M6 Live Provider Bug Bash (v0.2.0-dev, Session 4)

**Task**: Run bug bash with live copilot-api provider, add patch validation and merge strategy config  
**Agent**: Supervisor agent  
**Status**: Complete — Full pass with live LLM

**What was done**:
- Added `ValidatePatch()` to gitutil — automated patch validation on `record`
- Added `merge_strategy` config option (`3way` default, `rebase` alt) to types, store, and CLI
- Added `extractUpstreamContext()` to reconcile — reads affected files for Phase 3 prompt
- Ran complete bug bash with live copilot-api (claude-sonnet-4, 44 models)
- Live LLM analysis produced detailed, accurate results with correct file paths
- Feature A: `upstream_merged` via Phase 3 (LLM analyzed upstream model-mapping.ts)
- Feature B: `reapplied` via Phase 4 (LLM said still_needed, patch applied cleanly)

**Key finding**: Upstream context is critical for Phase 3. Without actual file contents, the LLM returns "unclear".

---

## 2026-04-16 — M6 Bug Bash + Bug Fixes (v0.2.0-dev)

**Task**: Run reconciliation bug bash, fix discovered bugs, re-test  
**Agent**: Supervisor agent (3 sessions)  
**Status**: Complete — Full pass

**What was done**:
- Session 2: Ran initial bug bash against `tesseracode/copilot-api` at commit `0ea08feb`
  - Feature A (model translation fix): Correctly detected as `upstream_merged` via Phase 3
  - Feature B (models CLI subcommand): Blocked — 3 bugs found in patch capture and CLI
  - Found BUG-1 (flag ordering), BUG-2 (corrupt patches), BUG-3 (stale recording)
- Session 3: Fixed all 3 bugs + bonus improvement
  - Migrated CLI from stdlib `flag` to `cobra` (fixes interspersed flags)
  - Rewrote `CapturePatch()` with `git add --intent-to-add` (fixes new file handling)
  - Added trailing newline to all patch output (fixes corrupt patch at EOF)
  - Added `--from` flag to `record` (captures committed diffs)
  - Added 3-way merge fallback to forward-apply (handles lockfile mismatches)
- Re-ran bug bash: Feature A → `upstream_merged`, Feature B → `reapplied`. Full pass.

**Key decisions**:
- Added cobra dependency (breaks zero-dep constraint, user-approved)
- Patches now always end with `\n`
- Forward-apply tries strict then 3-way merge fallback

---

## 2026-04-16 — M0–M5 Implementation (v0.1.0-dev)

**Task**: Build unified tpatch CLI from M0 through M5  
**Agent**: Supervisor agent (1 session)  
**Status**: Complete — All milestones approved

**What was done**:
- Built entire CLI in Go: 12 commands, ~2600 LOC source, ~850 LOC tests
- M0: Go module, CLI skeleton, Makefile
- M1: .tpatch/ data model, store layer, init/add/status/config, slug generation, path safety
- M2: OpenAI-compatible provider, analyze/define/explore with heuristic fallback
- M3: implement, apply (prepare/started/done), record, patch capture
- M4: 4-phase reconciliation engine with 4 test scenarios
- M5: 6 skill formats embedded via go:embed, parity guard test

---

## 2026-04-16 — Project Bootstrap (Governance)

**Task**: Bootstrap tpatch/ consolidation project with governance files  
**Agent**: Board review agent  
**Status**: Complete

**What was done**:
- Created SPEC.md consolidating technical decisions from all three teams
- Created CLAUDE.md for agent orientation with read-this-first table
- Created AGENTS.md defining the cyclic supervisor workflow (implementation → review → decision)
- Created ROADMAP.md with M0-M6 milestones + future M7-M11
- Created 7 milestone files with detailed task lists, acceptance criteria, and reference pointers
- Created handoff and supervisor log templates
- Created consolidation prompt for the supervisor agent

**Key decisions**:
- Go with zero dependencies (stdlib only)
- 4-phase reconciliation (reverse-apply → operation-level → provider-assisted → forward-apply)
- 6 skill formats (Claude, Copilot, Copilot Prompt, Cursor, Windsurf, Generic)
- Deterministic apply recipe with path traversal protection
- Secret-by-reference pattern for provider credentials
# Current Handoff

## Active Task
- **Task ID**: ADR-004 (M10 proxy UX) + ADR-005 (M11 native provider)
- **Milestone**: Planning locked-in for M10 and M11
- **Description**: User chose interactively through open questions; decisions captured as two ADRs. PRD updated to match the session-token-exchange direction (copilot-api/litellm pattern) instead of opencode's simpler path.
- **Status**: ADRs written, awaiting supervisor review
- **Assigned**: 2026-04-17

## Session Summary

1. **Committed Phase 2 work** as commit `dc42718` ("Phase 2 (v0.3.0): providers, validation, interactive/harness, distribution"). Includes all M7/M8/M9, refinement, and distribution changes.
2. **Released v0.3.0** — bumped version constant from `0.3.0-dev` to `0.3.0`, committed as `305781d`, tagged `v0.3.0` with a full release note. Tag is local; repo owner still needs to `git push origin main --tags`.
3. **Researched Copilot auth options**:
   - Pulled `tesseracode/copilot-api` README — explicitly "reverse-engineered proxy… not supported by GitHub… may trigger abuse-detection systems."
   - Pulled `github/copilot-cli` README and repo root listing — **not open source** (only README, install.sh, changelog, LICENSE published; the CLI is a closed-source binary on Homebrew/npm/WinGet). Official auth paths: `/login` OAuth or `GH_TOKEN`/`GITHUB_TOKEN` with "Copilot Requests" PAT permission.
   - Conclusion: **GitHub does not publish a public OpenAI-compatible Copilot endpoint.** Every third-party integration (copilot-api, Claude Code via proxy, tpatch) is on reverse-engineered surface.
4. **Wrote PRD** (`docs/prds/PRD-native-copilot-auth.md`) with 5 options evaluated and a two-phase recommendation: M10 managed-proxy UX (`copilot-start` / `copilot-stop` / `copilot-status`), then M11 opt-in native PAT provider calling `api.githubcopilot.com` directly. Shelling out to `copilot` CLI explicitly rejected (burns premium requests, re-runs its own agent loop).

## Files Created
- `docs/prds/PRD-native-copilot-auth.md`

## Files Changed
- `internal/cli/cobra.go` — version `0.3.0-dev` → `0.3.0` (committed)

## Git State
- `dc42718` — Phase 2 feature commit
- `305781d` — "Release v0.3.0" (version bump)
- `v0.3.0` — tag on 305781d
- **Not yet pushed.** Repo owner needs `git push origin main && git push origin v0.3.0`.

## Test Results
- `gofmt -l .` clean
- `go test ./...` — all 7 packages pass
- `tpatch --version` → `tpatch 0.3.0`

## Key Decisions (captured in ADR-004 and ADR-005)

**M10 — copilot-api UX (ADR-004)**
- No process supervision; we warn when unreachable, point at install instructions.
- Upstream `ericc-ch/copilot-api` is the recommended proxy; internal TODO to revisit the tesseracode fork if its fixes become blocking.
- New global config at `~/.config/tpatch/config.yaml`; per-repo `.tpatch/config.yaml` overrides.
- Reachability probe on first call (`GET /v1/models`, 2s timeout); warn-but-continue on `init`, hard-fail on workflow commands.
- First-run AUP warning stored in global config; no log piping; Windows deferred.

**M11 — native Copilot provider (ADR-005)**
- **Changed direction**: port ericc-ch/copilot-api's internal flow (session-token exchange via `copilot_internal/v2/token` + VS Code Copilot Chat client ID `Iv1.b507a08c87ecfe98`) rather than opencode's simpler Bearer-the-OAuth-token path. copilot-api and litellm both use this flow → proven, field-exposed surface that matches what Copilot's own editor plugins do.
- Token storage: `$XDG_DATA_HOME/tpatch/copilot-auth.json`, chmod 0600. OS keychain deferred.
- OAuth token treated as long-lived; 401 triggers one retry then "run copilot-login again".
- Device-flow prompts for GitHub.com vs Enterprise; Enterprise domain captured at login.
- `GET /models` every session, no persistent cache.
- Editor headers overridable via `provider.headers_override`; `x-initiator` opt-in, unset by default.
- `type: copilot-native` distinct from `type: openai-compatible` + copilot proxy.
- Opt-in gate with AUP acknowledgement in global config.

## Blockers
- None for the PRD itself.
- M11 (native provider) is soft-blocked on the "can we ship the editor header set?" legal question noted in the PRD.

## Next Steps
1. **Repo owner**: decide whether to create a GitHub Release for v0.3.0 (or add `softprops/action-gh-release@v2` to CI for automation on future tags).
2. **Before M11 implementation begins**: answer the two open questions in the PRD and ADR-005 (legal/ToS on editor headers; GitHub roadmap for an official endpoint).
3. **Next agent session — M10 implementation** per ADR-004: add global-config loader, reachability probe in provider-set/init flow, first-run AUP warning helper.
4. **After M10 lands — M11 implementation** per ADR-005, gated on the open questions.

## Context for Next Agent
- PRD lives at `docs/prds/PRD-native-copilot-auth.md`. It includes the full options matrix and the rejection rationale for each alternative.
- The `Provider` interface is stable and Phase 1 does not need to touch it at all — the managed proxy still routes through the existing `OpenAICompatible` code path. Phase 2 adds a sibling struct.
- `docs/harnesses/copilot.md` already documents the current manual setup; update it when M10 lands.
- GitHub has explicitly warned users in copilot-api's README about abuse-detection. Our UX for M10/M11 must surface that warning prominently.



---


---

# Archived — 2026-04-17T08:26:19Z

# Current Handoff

## Active Task
- **Task ID**: M10 — Managed Copilot proxy UX (ADR-004)
- **Milestone**: M10 delivered
- **Description**: Honest UX for the reverse-engineered `copilot-api` proxy — global config, reachability probe, first-run AUP warning, install pointers, CI release automation.
- **Status**: Implemented; awaiting supervisor review.
- **Assigned**: 2026-04-17

## Session Summary

1. **CI release automation** — added a `release` job to `.github/workflows/ci.yml` that triggers on `v*` tag pushes, creates a GitHub Release via `softprops/action-gh-release@v2`, auto-generates release notes, and marks tags containing `-` as prereleases. Uses the default `GITHUB_TOKEN` with `contents: write`. Cost: free.
2. **Global config** — new `internal/store/global.go` adds `GlobalConfigPath()`, `LoadGlobalConfig`, `SaveGlobalConfig`, `(s *Store).LoadMergedConfig`, `AcknowledgeCopilotAUP`, `CopilotAUPAcknowledged`, `mergeConfig`, `renderGlobalYAML`. Honors `XDG_CONFIG_HOME`, falls back to `os.UserConfigDir()` (macOS caveat documented in the harness doc). Chmod 0600 on write.
3. **Config precedence** — repo `.tpatch/config.yaml` overrides the global config field-by-field; zero values do **not** clear globals (must set the field explicitly). AUP ack is global-only.
4. **Types** — `Config.CopilotAUPAckAt string` added to `internal/store/types.go`.
5. **Reachability probe** — new `internal/provider/probe.go` with `Reachable(ctx, cfg)` (2s timeout), `IsLocalEndpoint(cfg)`, `IsCopilotProxyEndpoint(cfg)` helpers. Probes via existing `Check()`.
6. **CLI wiring** — new `internal/cli/copilot.go` with `copilotInstallHint`, `copilotAUPWarning`, `maybeShowAUPWarning`, `ensureProviderReachable`, `warnIfUnreachable`, `providerConfigFromStore`. Wired into `init` (warn-continue + AUP) and `providerSetCmd` + `autoDetectProvider` (AUP on first Copilot selection).
7. **Workflow hard-fail** — `loadAndProbeProvider(ctx, s)` replaces `loadProviderFromStore` in analyze/define/explore/implement/cycle. Probes once per process (cached per base URL). Local-endpoint-only; opt-out via `TPATCH_NO_PROBE=1`. Non-local endpoints skip the probe to avoid penalising custom remote configs.
8. **Execute now surfaces errors** — `Execute()` prints `error: %v` to stderr before returning exit code 1 so probe failures are visible. Preserves existing `SilenceErrors: true` cobra behaviour for graceful formatting.
9. **Harness doc refresh** — `docs/harnesses/copilot.md` now documents the install path, OS-dependent global config path (macOS caveat), warn-vs-fail behaviour, and links to ADR-004/005.
10. **Tests** — 6 new tests in `internal/store/global_test.go` (roundtrip, missing file, ack idempotency, precedence, merge-no-clear, save creates dir) and 5 in `internal/provider/probe_test.go` (httptest OK, TEST-NET-1 timeout, not-configured, URL matcher, cancelled ctx). All 7 packages pass.

## Files Created
- `.github/workflows/ci.yml` — amended (release job)
- `internal/store/global.go`
- `internal/store/global_test.go`
- `internal/provider/probe.go`
- `internal/provider/probe_test.go`
- `internal/cli/copilot.go`

## Files Changed
- `internal/cli/cobra.go` — `loadAndProbeProvider`, `Execute` prints errors, AUP wiring in `init` / `providerSetCmd` / `autoDetectProvider`, `sync` import.
- `internal/store/types.go` — `CopilotAUPAckAt` field.
- `docs/harnesses/copilot.md` — M10 section.

## Test Results
- `gofmt -w .` clean
- `go vet ./...` clean
- `go test ./... -count=1` — 7/7 packages pass
- `go build ./cmd/tpatch` OK
- Smoke: `init` + `provider set --preset copilot` prints AUP warning exactly once; second run is quiet; `analyze` against a dead localhost port hard-fails with an install hint; against a live copilot-api proxy falls through to the workflow.

## Key Behaviours

- **Warn vs fail**: `init` and `provider set` are warn-continue (a user may be bootstrapping before starting the proxy). Workflow commands that actually call the LLM (`analyze|define|explore|implement|cycle`) hard-fail when the local endpoint is unreachable.
- **Probe scope**: only runs for local endpoints (`localhost`, `127.0.0.1`, `[::1]`). Remote endpoints are trusted.
- **AUP once**: the AUP warning fires only when the new config actually points at the copilot-api proxy (`openai-compatible` + port 4141) and the user has not acknowledged before.
- **TODO**: `copilotInstallHint` carries an inline `TODO(adr-004)` comment to revisit the tesseracode fork recommendation if its divergent fixes become blocking.

## Blockers
- None for M10.
- M11 still soft-blocked on the two open questions in ADR-005 (editor-headers legal/ToS, official endpoint roadmap). User direction: proceed with editor headers, monitor; so these are effectively closed as "accept risk".

## Next Steps
1. Supervisor review of M10 implementation.
2. Commit as `feat(m10): managed copilot-api proxy UX (ADR-004)` and push.
3. Consider tagging `v0.3.1` once review lands — CI will produce the GitHub Release automatically.
4. Start M11 implementation per ADR-005 (native Copilot provider with session-token exchange) once M10 is merged.

## Context for Next Agent
- Global config on macOS defaults to `~/Library/Application Support/tpatch/config.yaml` unless `XDG_CONFIG_HOME` is set. Every test that touches global state sets `XDG_CONFIG_HOME` to a tempdir; follow this pattern.
- `TPATCH_NO_PROBE=1` disables the workflow hard-fail probe (useful for offline demos or CI steps that only read store state). Add it to future tests that should not hit the network.
- The probe cache is a process-level `map[string]error` guarded by a mutex — fine for the CLI's one-shot lifecycle but intentionally not time-bound, so long-running processes would need to invalidate it. Not a concern today.
- `Execute()` now prints errors. Tests that exercise `rootCmd.Execute()` directly still use the cobra `SetErr` buffer; only the top-level wrapper prints to stderr.
- The AUP warning text lives in `internal/cli/copilot.go::copilotAUPWarning`. Tweak there, not in harness docs.
# Current Handoff

## Active Task
- **Task ID**: v0.4.2 / A1 — `bug-implement-silent-fallback`
- **Milestone**: Tranche A "Truthful Errors" (post-stress-test, plan.md)
- **Description**: Surface the implement-phase fallback to the user, raise
  the LLM token budget so legitimate recipes are not truncated, and let
  the user override the budget via config.
- **Status**: A1 complete; A2 (`bug-cycle-state-mismatch`) is now active.
- **Assigned**: 2026-04-18

## Session Summary

A1 landed in this session:

1. **Config knob** — `Config.MaxTokensImplement` (`internal/store/types.go`),
   default `DefaultMaxTokensImplement = 16384`. Repo override via
   `max_tokens_implement:` in `.tpatch/config.yaml`; global override via
   the same key in `~/.config/tpatch/config.yaml`. `parseYAMLConfig` reads
   it; `SaveConfig` and `renderGlobalYAML` emit it; `mergeConfigs` lets
   the repo value win when set.
2. **Implement fallback no longer silent** — `internal/workflow/implement.go`
   gained a package-level `WarnWriter io.Writer = os.Stderr`. When
   `GenerateWithRetry` exhausts its retry budget the fallback writes a
   warning to `WarnWriter` naming the retry count, the underlying error,
   the path to `raw-implement-response-*.txt`, and the config knob to
   bump on retry.
3. **MaxTokens bump** — implement phase now requests
   `cfg.MaxTokensImplement` (defaulting to 16384) instead of the
   hard-coded 8192. Other phases unchanged for now (analyze/define/explore
   stay at 4096; revisit if real failures surface).
4. **Tests** — `internal/workflow/implement_test.go`:
   - `TestRunImplement_FallbackEmitsWarning` drives `RunImplement` with
     a fake provider that returns un-parseable JSON, captures
     `WarnWriter`, asserts the warning text, and confirms the heuristic
     recipe is the one written to disk.
   - `TestConfig_DefaultMaxTokensImplement` confirms a freshly-`Init`-ed
     repo loads the 16384 default.

## Current State

- Repo at clean working tree on top of v0.4.1 (no commits yet for v0.4.2;
  Tranche A will be tagged together once A1–A10 land).
- `gofmt -l .` clean, `go build ./cmd/tpatch` ok, `go test ./...` green.
- Plan lives at
  `~/.copilot/session-state/f2c5d9eb-cef9-41dc-aab7-ad825ffca018/plan.md`.

## Files Changed (A1)

- `internal/store/types.go` — added `MaxTokensImplement` field +
  `DefaultMaxTokensImplement` const.
- `internal/store/store.go` — parser entry, repo template, `SaveConfig`
  renderer.
- `internal/store/global.go` — merge precedence + `renderGlobalYAML`.
- `internal/workflow/implement.go` — `WarnWriter`, dynamic `MaxTokens`,
  surfaced fallback warning.
- `internal/workflow/implement_test.go` — new test file.

## Test Results

```
ok  github.com/tesseracode/tesserapatch/assets
ok  github.com/tesseracode/tesserapatch/internal/cli
ok  github.com/tesseracode/tesserapatch/internal/provider
ok  github.com/tesseracode/tesserapatch/internal/safety
ok  github.com/tesseracode/tesserapatch/internal/store
ok  github.com/tesseracode/tesserapatch/internal/workflow
```

## Next Steps

Continue Tranche A in order. The full ordered list is in plan.md; the
next 4 tasks are:

1. **A2 `bug-cycle-state-mismatch`** — audit `cycle` state transitions,
   ensure `state` advances even on heuristic fallback, add per-phase
   post-condition assertions, add a `cycle --skip-execute` test that
   reaches `implemented`. Currently `in_progress` in SQL.
2. **A3 `bug-record-validation-false-positive`** — switch record-time
   validation to `git apply --reverse --check` (add
   `gitutil.ValidatePatchReverse`).
3. **A4 `bug-reconcile-phase4-false-positive`** — three-state verdict
   (`reapplied-strict` / `reapplied-with-3way` / `blocked`); detect
   conflict markers via temp worktree apply.
4. **A5 `bug-skill-invocation-clarity`** — Invocation + Phase-ordering +
   Preflight blocks across all 6 skill formats; parity guard updated.

Then A6–A10, version bump to 0.4.2, CHANGELOG, tag.

## Blockers

None.

## Context for Next Agent

- Use `WarnWriter` (not `fmt.Fprintln(os.Stderr, ...)` directly) for any
  new non-fatal phase warnings; tests rely on being able to swap it.
- The implement phase is the only phase that needs the larger token
  budget right now. If you change another phase's budget, mirror the
  pattern (config knob + `Default*` const + global+repo merge).
- The Tranche-A version bump happens **once** at the end of A10. Do NOT
  bump `cobra.go:version` or write a CHANGELOG entry as you go — group
  them in a single v0.4.2 commit.
- The session SQL is the source of truth for task progress
  (`SELECT id, status FROM todos WHERE status='pending' ORDER BY id`).
- Co-author trailer required on every commit:
  `Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>`.

## 2026-04-27 — M15 Wave 3 Slice A — APPROVED WITH NOTES

External supervisor approved the Slice A stack with a single low-severity
doc finding: earlier sections of CURRENT.md described V2 as `recipe_parses
+ recipe_op_targets_resolve` (the pre-revision contract) while later
Revision 2 / Revision 3 sections correctly recorded the shipped boundary
(V2 = `recipe_parses` only; V3 = stub deferred to Slice C). The audit trail
is preserved here as-is — the evolving understanding is the history.

Final shipped contract (post-Revision 3):
- V0 (`status_loadable`), V1 (`intent_files_present` for spec.md AND
  exploration.md), V2 (`recipe_parses` with `DisallowUnknownFields`).
- V3 (`recipe_op_targets_resolve`) and V4–V9 are stubs returning
  `passed:true, skipped:true, reason:"not yet implemented (Slice <X>)"`.
- Pre-apply lifecycle states refused with exit 2; no persistence.
- V0 abort / missing slug / non-tpatch workspace all exit 2 (typed
  `ExitCodeError` plumb in `internal/cli/exit_error.go`).
- Generic cobra usage errors still exit 1.
- Persisted record carries minimal fields only; full 10-check array on
  `--json` stdout per LOG `3c122aa` Note 1.
- Skill stubs: minimal one-line EXPERIMENTAL anchors in 6 surfaces;
  parity guard green.
- ADR-013 D1–D7 honoured; apply gate untouched; no read-path mutation.

Commit stack on `main` ahead of `origin/main`:
- `8e2aabe` tracking normalization
- `41cc4aa` initial Slice A (verify shell + V0–V2 + freshness writer)
- `7b29dcf` reviewer-1 NEEDS REVISION
- `a4b4262` revision-1 (parent_snapshot omits missing parents)
- `dbede9b` reviewer-2 APPROVED
- `1e29f8f` revision-2: typed `ExitCodeError`
- `77cbf50` revision-2: refuse pre-apply + V1 exploration.md + strict
  decode + V3 deferral
- `c3bb18f` revision-2: PRD prose alignment with stdout-only `check_results`
- `d738b47` reviewer-3 APPROVED
- `8a47078` revision-3: exit 2 for V0 abort / missing slug / non-init
  workspace + stale-wording cleanup
- `bce2252` reviewer-4 APPROVED
- (this commit) tracking transition into Slice B

External-supervisor reproductions confirmed: nonexistent feature exits 2,
non-init workspace exits 2, corrupt status.json (V0 abort) exits 2,
generic cobra usage error exits 1.

---

## Archived CURRENT.md (Slice A active-task entry as it stood at approval)

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

---

## Revision 2 (post-external-review, 2026-04-28)

External supervisor returned 4 binding findings + 1 PRD/schema reconciliation. All five addressed surgically.

### Disposition per finding

- **F1 (typed exit code 2)** — Added `*ExitCodeError{Code, Message}` in new `internal/cli/exit_error.go`. `cli.Execute()` now unwraps `*ExitCodeError` via `asExitCodeError()` and returns its `ExitCode()`; legacy errors still collapse to 1. `verifyCmd.RunE` returns `&ExitCodeError{Code: 2, ...}` on verdict-fail and on refusal. `cmd.SilenceUsage`/`SilenceErrors` set inside RunE. New tests in `internal/cli/exit_error_test.go` lock in the plumb (`TestExecute_PropagatesExitCodeError` parametric over plain-error / ExitCodeError{2,3} / nil).
- **F2 (refuse pre-apply states, no persist)** — `RunVerify` returns a typed `*RefusedError{Slug,State,Reason}` and a `Verdict: "refused", ExitCode: 2, Reason: "..."` report when the lifecycle state is one of `requested / analyzed / defined / implementing / reconciling / reconciling-shadow`. Allowed: `applied / active / upstream_merged / blocked` (per PRD §5). The refusal early-returns before any `WriteVerifyRecord` call, so status.json stays untouched even with `--no-write` unset. `IsRefused(err)` exported; CLI maps to `ExitCodeError{2}`. New tests: `TestRunVerify_RefusesPreApplyState` (parametric over all six refused states), `TestRunVerify_RefusalNotWrittenEvenWithoutNoWrite` (the supervisor's exact fixture path), `TestRunVerify_AllowsPostApplyStates` (parametric over the four allowed states).
- **F3a (strict recipe decode)** — `checkRecipeParses` (renamed from `checkRecipe`) now uses `json.NewDecoder(bytes.NewReader(data)).DisallowUnknownFields().Decode(&recipe)`, matching the canonical pattern in `recipe_createdby_test.go`. `LoadRecipe` in `internal/workflow/recipe.go` left untouched (apply-path behaviour preserved per scope constraint). New test `TestRunVerify_V2_RejectsUnknownFields` locks in the strict-decode contract for verify.
- **F3b (defer V3 to Slice C)** — `recipe_op_targets_resolve` is now a Slice C stub returning `passed:true, skipped:true, reason:"not yet implemented (Slice C — created_by hard-parent semantics)"`. Slice A's V2 collapses to a single real check (`recipe_parses`); the file-existence check that used to live in V2 is gone, and V3 takes the existing position in the 10-check array (shape preserved). Old test `TestRunVerify_V2_OpTargetMissingFails` replaced by `TestRunVerify_V3_MissingTargetIsDeferredToSliceC` which asserts the same recipe now PASSES Slice A verify (V2 parse OK, V3 stub passed+skipped).
- **F4 (V1 also requires exploration.md)** — `checkIntentFilesPresent` now iterates `[]string{"spec.md", "exploration.md"}` and fails with file-named remediation on missing/empty for either. Three new tests: `TestRunVerify_V1_FailsWhenExplorationMissing`, `TestRunVerify_V1_FailsWhenExplorationEmpty`, `TestRunVerify_V1_PassesWhenBothPresent`. Existing spec.md tests preserved (and `TestRunVerify_V1_FailsWhenSpecEmpty` updated to write exploration so the failure narrows to spec). Helper `writeExploration` + `writeIntentFiles` introduced.
- **F5 (PRD prose alignment)** — `docs/prds/PRD-verify-freshness.md` updated in three places (Summary §0, §3.2 list, §3.4.1 Go struct example) to remove `check_results` from the persisted shape and add a one-sentence pointer to LOG entry `3c122aa` Note 1 as the authoritative disposition. ADR-013, store types, and `WriteVerifyRecord` all left untouched.

### V-id mapping note

The supervisor flagged "if the recipe-target check is V2 itself rather than a separate V-id, then V2 collapses". After re-reading PRD §3.1: V2 is `recipe_parses` (a separate row), V3 is `recipe_op_targets_resolve` (a separate row). The codebase's `CheckRecipeOpTargetsResolve` constant maps to PRD V3. So Slice A keeps **V0/V1/V2 real** and **V3–V9 stubbed** — boundary unchanged from the PRD §9 Slice A row. Documented in `verify.go` doc comment and the V3 stub function `stubRecipeOpTargetsResolve`.

### Reproduction transcripts

**Refused path (the supervisor's fixture, post-fix):**

```
$ ./tpatch_bin init "$tmp"
$ ./tpatch_bin --path "$tmp" add "Fresh requested verify reproduction"
  Created feature: fresh-requested-verify-reproduction (state: requested)
$ ./tpatch_bin --path "$tmp" verify fresh-requested-verify-reproduction
  verify fresh-requested-verify-reproduction — refused
  error: feature fresh-requested-verify-reproduction is in lifecycle state "requested";
         verify refuses pre-apply / mid-flight states (PRD §5)
EXIT=2

status.json (no `verify` key):
{
  "id": "fresh-requested-verify-reproduction",
  "slug": "fresh-requested-verify-reproduction",
  "state": "requested",
  ...
  "apply": {},
  "reconcile": {}
}
```

**Applied path (manually flipped to `state: applied`):**

```
=== Test 1: applied + missing intent files (should fail with EXIT=2) ===
verify demo-applied — failed
  ✓ [block-abort] status_loaded
  ✗ [block] intent_files_present — spec.md missing for demo-applied — re-run …
  ⊘ [block] recipe_parses — no apply-recipe.json (legacy / pre-autogen-era feature)
  ⊘ [block] recipe_op_targets_resolve — not yet implemented (Slice C — created_by hard-parent semantics)
  ⊘ [block] dep_metadata_valid — not yet implemented (Slice C)
  …
EXIT=2

=== Test 2: applied + spec.md + exploration.md present (should pass with EXIT=0) ===
verify demo-applied — passed
  ✓ [block-abort] status_loaded
  ✓ [block] intent_files_present
  ⊘ [block] recipe_parses — no apply-recipe.json (legacy / pre-autogen-era feature)
  ⊘ … (V3–V9 stubs)
EXIT=0

status.json after passing verify:
"verify": {
  "verified_at": "2026-04-28T01:42:25Z",
  "passed": true
}
```

Both reproductions confirm: F1 typed exit code is plumbed end-to-end, F2 refusal does not persist, F4 exploration.md is required for V1 to pass.

### Files Changed (Revision 2)

- `internal/cli/exit_error.go` (new)
- `internal/cli/exit_error_test.go` (new)
- `internal/cli/cobra.go` (Execute() unwraps ExitCodeError)
- `internal/cli/verify.go` (RunE returns ExitCodeError on fail / refusal; SilenceUsage/Errors)
- `internal/workflow/verify.go` (RefusedError type, postApplyVerifyStates set, V1 dual-file check, V2 strict decode + DisallowUnknownFields, V3 stub `stubRecipeOpTargetsResolve`, refusal early-return, Reason field on report)
- `internal/workflow/verify_test.go` (existing tests updated for new V1 contract; new tests for F2 refusal, F3a strict decode, F3b V3 deferral, F4 exploration.md)
- `docs/prds/PRD-verify-freshness.md` (F5 — three prose passages aligned with stdout-only check_results)
- `docs/handoff/CURRENT.md` (this Revision 2 section)

### Validation

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
$ go build ./cmd/tpatch && rm -f tpatch
(success)
```

ADR-013 untouched. Store types untouched. Apply gate untouched. Skill stubs untouched. Slice A boundary preserved (no `--all`, no `--shadow`, no closure replay, no `ComposeLabels` integration).

**Status: ready for re-review.**

---

## Revision 3 (post-external-review #2, 2026-04-29)

External supervisor's second pass kept Revision 2's fixes (refusal, V1
exploration.md, strict V2 decode, V3 deferral, PRD prose) and returned
**one HIGH finding plus a comment-drift cleanup**. Both closed.

### Disposition per finding

- **F1 (HIGH — wrap remaining error paths in ExitCodeError(2))** — `verifyCmd.RunE` had two surviving plain-error returns:
  1. `openStoreFromCmd` failure (covers both *non-tpatch workspace* and *missing-slug-as-store-error*).
  2. `RunVerify` returning a non-refusal error (covers *V0 abort* — `status.json` unreadable for the requested slug).
  Both now return `&ExitCodeError{Code: 2, Message: ...}` so `cli.Execute()` propagates exit 2. The refusal path (`*RefusedError`) and the verdict-failed path are unchanged. Generic cobra errors (usage parse, unknown flag) still fall through to legacy exit 1.

  **Design choice on shape**: PRD §5 lists the *exit-code contract* for non-tpatch workspace, missing slug, and V0 abort but does NOT require a structured `--json` payload for these abort surfaces. Picked the simpler stderr-text form (`"verify aborted: <reason>"`) since `--json` mid-flight aborts have no schema in PRD §4.3. Documented in the comment block on `verifyCmd`.

- **F2 (regression tests)** — Added `internal/cli/verify_test.go` with three test cases that drive `buildRootCmd().Execute()` directly and unwrap the returned error via `errors.As(&ec)` so the typed exit code is asserted (the package's existing `runCmd` helper collapses every error to 1, which would mask exactly the plumbing under test):
  - `TestVerify_MissingSlug_ExitsTwo` — `init` then `verify nope` → `*ExitCodeError{Code: 2}`.
  - `TestVerify_NonTpatchWorkspace_ExitsTwo` — `--path` to bare temp dir → `*ExitCodeError{Code: 2}`.
  - `TestVerify_V0AbortFromRunVerify_ExitsTwo` — feature added, `status.json` overwritten with `{not valid json` → `*ExitCodeError{Code: 2}`.

- **F3 (stale wording)** — `internal/cli/verify.go` doc block, the `verifyCmd.Long` help text, and `internal/workflow/verify.go` top-of-file scope comment all still claimed V2 was "recipe parses + op targets resolve" and that Slice A "ships V0/V1/V2 (… op targets resolve)". Rewrote to:
  - V2 = `recipe_parses` only.
  - V3 (`recipe_op_targets_resolve`) is a Slice C stub.
  - Slice A ships V0/V1/V2 as real, V3–V9 as stubs.
  Added an explicit "Exit code contract" block in `verify.go` referencing PRD §6 Q7 + §5 so future readers see the typed-exit invariant alongside the help text.

### Reproduction transcripts (supervisor's three cases, post-fix)

```
$ go build -o ./bin/tpatch-rev3 ./cmd/tpatch && BIN=$(pwd)/bin/tpatch-rev3

# Case A: missing slug, initialized workspace
$ tmp=$(mktemp -d) && (cd "$tmp" && git init -q && git config user.email t@t && git config user.name t)
$ $BIN init "$tmp" >/dev/null
$ $BIN --path "$tmp" verify nope; echo "EXIT=$?"
verify nope — failed
  ✗ [block-abort] status_loaded — could not load status.json: …features/nope/status.json: no such file or directory
  ⊘ [block] intent_files_present — skipped: V0 (status_loaded) aborted the run
  …
error: verify aborted: open …/features/nope/status.json: no such file or directory
EXIT=2

# Case B: non-tpatch workspace
$ empty=$(mktemp -d)
$ $BIN --path "$empty" verify nope; echo "EXIT=$?"
error: verify aborted: could not find .tpatch in this directory or any parent
EXIT=2

# Case C (bonus): V0 abort via corrupt status.json
$ $BIN --path "$tmp" add --slug demo demo >/dev/null
$ echo "{not valid json" > "$tmp/.tpatch/features/demo/status.json"
$ $BIN --path "$tmp" verify demo; echo "EXIT=$?"
verify demo — failed
  ✗ [block-abort] status_loaded — could not load status.json: invalid character 'n' looking for beginning of object key string
  …
error: verify aborted: invalid character 'n' looking for beginning of object key string
EXIT=2
```

All three previously leaked exit 1; all three now exit 2.

### Files Changed (Revision 3)

- `internal/cli/verify.go` — F1 wraps store-open and RunVerify-non-refusal errors in `ExitCodeError{2}`; F3 rewrites doc block + `Long` help to acknowledge V3 deferral and document the exit-code contract.
- `internal/workflow/verify.go` — F3 only: top-of-file scope comment updated (V2 = `recipe_parses`; V3 stubbed as Slice C). No behavioural change.
- `internal/cli/verify_test.go` — new file with the three F2 regression tests.
- `docs/handoff/CURRENT.md` — this Revision 3 section.

### Validation

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
$ go build ./cmd/tpatch && rm -f tpatch
(success)
```

ADR-013, PRD-verify-freshness.md, store types, store.go, the apply gate, the refusal path, V1's intent check, V2's strict decode, V3's deferral, and skill stubs are all untouched. Slice A boundary preserved.

**Status: ready for re-review.**

## 2026-04-27 — M15 Wave 3 Slice A — APPROVED WITH NOTES

External supervisor approved the Slice A stack with a single low-severity
doc finding: earlier sections of CURRENT.md described V2 as `recipe_parses
+ recipe_op_targets_resolve` (the pre-revision contract) while later
Revision 2 / Revision 3 sections correctly recorded the shipped boundary
(V2 = `recipe_parses` only; V3 = stub deferred to Slice C). The audit trail
is preserved here as-is — the evolving understanding is the history.

Final shipped contract (post-Revision 3):
- V0 (`status_loadable`), V1 (`intent_files_present` for spec.md AND
  exploration.md), V2 (`recipe_parses` with `DisallowUnknownFields`).
- V3 (`recipe_op_targets_resolve`) and V4-V9 are stubs returning
  `passed:true, skipped:true, reason:"not yet implemented (Slice <X>)"`.
- Pre-apply lifecycle states refused with exit 2; no persistence.
- V0 abort / missing slug / non-tpatch workspace all exit 2 (typed
  `ExitCodeError` plumb in `internal/cli/exit_error.go`).
- Generic cobra usage errors still exit 1.
- Persisted record carries minimal fields only; full 10-check array on
  `--json` stdout per LOG `3c122aa` Note 1.
- Skill stubs: minimal one-line EXPERIMENTAL anchors in 6 surfaces;
  parity guard green.
- ADR-013 D1-D7 honoured; apply gate untouched; no read-path mutation.

Commit stack on `main` ahead of `origin/main`:
- `8e2aabe` tracking normalization
- `41cc4aa` initial Slice A
- `7b29dcf` reviewer-1 NEEDS REVISION
- `a4b4262` revision-1 (parent_snapshot omits missing parents)
- `dbede9b` reviewer-2 APPROVED
- `1e29f8f` revision-2: typed `ExitCodeError`
- `77cbf50` revision-2: refuse pre-apply + V1 exploration.md + strict decode + V3 deferral
- `c3bb18f` revision-2: PRD prose alignment
- `d738b47` reviewer-3 APPROVED
- `8a47078` revision-3: exit 2 for V0 abort / missing slug / non-init workspace
- `bce2252` reviewer-4 APPROVED
- (this commit) tracking transition into Slice B

External-supervisor reproductions confirmed: nonexistent feature exits 2,
non-init workspace exits 2, corrupt status.json (V0 abort) exits 2,
generic cobra usage error exits 1.

---

## Archived CURRENT.md (Slice A active-task entry as it stood at approval)

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

---

## Revision 2 (post-external-review, 2026-04-28)

External supervisor returned 4 binding findings + 1 PRD/schema reconciliation. All five addressed surgically.

### Disposition per finding

- **F1 (typed exit code 2)** — Added `*ExitCodeError{Code, Message}` in new `internal/cli/exit_error.go`. `cli.Execute()` now unwraps `*ExitCodeError` via `asExitCodeError()` and returns its `ExitCode()`; legacy errors still collapse to 1. `verifyCmd.RunE` returns `&ExitCodeError{Code: 2, ...}` on verdict-fail and on refusal. `cmd.SilenceUsage`/`SilenceErrors` set inside RunE. New tests in `internal/cli/exit_error_test.go` lock in the plumb (`TestExecute_PropagatesExitCodeError` parametric over plain-error / ExitCodeError{2,3} / nil).
- **F2 (refuse pre-apply states, no persist)** — `RunVerify` returns a typed `*RefusedError{Slug,State,Reason}` and a `Verdict: "refused", ExitCode: 2, Reason: "..."` report when the lifecycle state is one of `requested / analyzed / defined / implementing / reconciling / reconciling-shadow`. Allowed: `applied / active / upstream_merged / blocked` (per PRD §5). The refusal early-returns before any `WriteVerifyRecord` call, so status.json stays untouched even with `--no-write` unset. `IsRefused(err)` exported; CLI maps to `ExitCodeError{2}`. New tests: `TestRunVerify_RefusesPreApplyState` (parametric over all six refused states), `TestRunVerify_RefusalNotWrittenEvenWithoutNoWrite` (the supervisor's exact fixture path), `TestRunVerify_AllowsPostApplyStates` (parametric over the four allowed states).
- **F3a (strict recipe decode)** — `checkRecipeParses` (renamed from `checkRecipe`) now uses `json.NewDecoder(bytes.NewReader(data)).DisallowUnknownFields().Decode(&recipe)`, matching the canonical pattern in `recipe_createdby_test.go`. `LoadRecipe` in `internal/workflow/recipe.go` left untouched (apply-path behaviour preserved per scope constraint). New test `TestRunVerify_V2_RejectsUnknownFields` locks in the strict-decode contract for verify.
- **F3b (defer V3 to Slice C)** — `recipe_op_targets_resolve` is now a Slice C stub returning `passed:true, skipped:true, reason:"not yet implemented (Slice C — created_by hard-parent semantics)"`. Slice A's V2 collapses to a single real check (`recipe_parses`); the file-existence check that used to live in V2 is gone, and V3 takes the existing position in the 10-check array (shape preserved). Old test `TestRunVerify_V2_OpTargetMissingFails` replaced by `TestRunVerify_V3_MissingTargetIsDeferredToSliceC` which asserts the same recipe now PASSES Slice A verify (V2 parse OK, V3 stub passed+skipped).
- **F4 (V1 also requires exploration.md)** — `checkIntentFilesPresent` now iterates `[]string{"spec.md", "exploration.md"}` and fails with file-named remediation on missing/empty for either. Three new tests: `TestRunVerify_V1_FailsWhenExplorationMissing`, `TestRunVerify_V1_FailsWhenExplorationEmpty`, `TestRunVerify_V1_PassesWhenBothPresent`. Existing spec.md tests preserved (and `TestRunVerify_V1_FailsWhenSpecEmpty` updated to write exploration so the failure narrows to spec). Helper `writeExploration` + `writeIntentFiles` introduced.
- **F5 (PRD prose alignment)** — `docs/prds/PRD-verify-freshness.md` updated in three places (Summary §0, §3.2 list, §3.4.1 Go struct example) to remove `check_results` from the persisted shape and add a one-sentence pointer to LOG entry `3c122aa` Note 1 as the authoritative disposition. ADR-013, store types, and `WriteVerifyRecord` all left untouched.

### V-id mapping note

The supervisor flagged "if the recipe-target check is V2 itself rather than a separate V-id, then V2 collapses". After re-reading PRD §3.1: V2 is `recipe_parses` (a separate row), V3 is `recipe_op_targets_resolve` (a separate row). The codebase's `CheckRecipeOpTargetsResolve` constant maps to PRD V3. So Slice A keeps **V0/V1/V2 real** and **V3–V9 stubbed** — boundary unchanged from the PRD §9 Slice A row. Documented in `verify.go` doc comment and the V3 stub function `stubRecipeOpTargetsResolve`.

### Reproduction transcripts

**Refused path (the supervisor's fixture, post-fix):**

```
$ ./tpatch_bin init "$tmp"
$ ./tpatch_bin --path "$tmp" add "Fresh requested verify reproduction"
  Created feature: fresh-requested-verify-reproduction (state: requested)
$ ./tpatch_bin --path "$tmp" verify fresh-requested-verify-reproduction
  verify fresh-requested-verify-reproduction — refused
  error: feature fresh-requested-verify-reproduction is in lifecycle state "requested";
         verify refuses pre-apply / mid-flight states (PRD §5)
EXIT=2

status.json (no `verify` key):
{
  "id": "fresh-requested-verify-reproduction",
  "slug": "fresh-requested-verify-reproduction",
  "state": "requested",
  ...
  "apply": {},
  "reconcile": {}
}
```

**Applied path (manually flipped to `state: applied`):**

```
=== Test 1: applied + missing intent files (should fail with EXIT=2) ===
verify demo-applied — failed
  ✓ [block-abort] status_loaded
  ✗ [block] intent_files_present — spec.md missing for demo-applied — re-run …
  ⊘ [block] recipe_parses — no apply-recipe.json (legacy / pre-autogen-era feature)
  ⊘ [block] recipe_op_targets_resolve — not yet implemented (Slice C — created_by hard-parent semantics)
  ⊘ [block] dep_metadata_valid — not yet implemented (Slice C)
  …
EXIT=2

=== Test 2: applied + spec.md + exploration.md present (should pass with EXIT=0) ===
verify demo-applied — passed
  ✓ [block-abort] status_loaded
  ✓ [block] intent_files_present
  ⊘ [block] recipe_parses — no apply-recipe.json (legacy / pre-autogen-era feature)
  ⊘ … (V3–V9 stubs)
EXIT=0

status.json after passing verify:
"verify": {
  "verified_at": "2026-04-28T01:42:25Z",
  "passed": true
}
```

Both reproductions confirm: F1 typed exit code is plumbed end-to-end, F2 refusal does not persist, F4 exploration.md is required for V1 to pass.

### Files Changed (Revision 2)

- `internal/cli/exit_error.go` (new)
- `internal/cli/exit_error_test.go` (new)
- `internal/cli/cobra.go` (Execute() unwraps ExitCodeError)
- `internal/cli/verify.go` (RunE returns ExitCodeError on fail / refusal; SilenceUsage/Errors)
- `internal/workflow/verify.go` (RefusedError type, postApplyVerifyStates set, V1 dual-file check, V2 strict decode + DisallowUnknownFields, V3 stub `stubRecipeOpTargetsResolve`, refusal early-return, Reason field on report)
- `internal/workflow/verify_test.go` (existing tests updated for new V1 contract; new tests for F2 refusal, F3a strict decode, F3b V3 deferral, F4 exploration.md)
- `docs/prds/PRD-verify-freshness.md` (F5 — three prose passages aligned with stdout-only check_results)
- `docs/handoff/CURRENT.md` (this Revision 2 section)

### Validation

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
$ go build ./cmd/tpatch && rm -f tpatch
(success)
```

ADR-013 untouched. Store types untouched. Apply gate untouched. Skill stubs untouched. Slice A boundary preserved (no `--all`, no `--shadow`, no closure replay, no `ComposeLabels` integration).

**Status: ready for re-review.**

---

## Revision 3 (post-external-review #2, 2026-04-29)

External supervisor's second pass kept Revision 2's fixes (refusal, V1
exploration.md, strict V2 decode, V3 deferral, PRD prose) and returned
**one HIGH finding plus a comment-drift cleanup**. Both closed.

### Disposition per finding

- **F1 (HIGH — wrap remaining error paths in ExitCodeError(2))** — `verifyCmd.RunE` had two surviving plain-error returns:
  1. `openStoreFromCmd` failure (covers both *non-tpatch workspace* and *missing-slug-as-store-error*).
  2. `RunVerify` returning a non-refusal error (covers *V0 abort* — `status.json` unreadable for the requested slug).
  Both now return `&ExitCodeError{Code: 2, Message: ...}` so `cli.Execute()` propagates exit 2. The refusal path (`*RefusedError`) and the verdict-failed path are unchanged. Generic cobra errors (usage parse, unknown flag) still fall through to legacy exit 1.

  **Design choice on shape**: PRD §5 lists the *exit-code contract* for non-tpatch workspace, missing slug, and V0 abort but does NOT require a structured `--json` payload for these abort surfaces. Picked the simpler stderr-text form (`"verify aborted: <reason>"`) since `--json` mid-flight aborts have no schema in PRD §4.3. Documented in the comment block on `verifyCmd`.

- **F2 (regression tests)** — Added `internal/cli/verify_test.go` with three test cases that drive `buildRootCmd().Execute()` directly and unwrap the returned error via `errors.As(&ec)` so the typed exit code is asserted (the package's existing `runCmd` helper collapses every error to 1, which would mask exactly the plumbing under test):
  - `TestVerify_MissingSlug_ExitsTwo` — `init` then `verify nope` → `*ExitCodeError{Code: 2}`.
  - `TestVerify_NonTpatchWorkspace_ExitsTwo` — `--path` to bare temp dir → `*ExitCodeError{Code: 2}`.
  - `TestVerify_V0AbortFromRunVerify_ExitsTwo` — feature added, `status.json` overwritten with `{not valid json` → `*ExitCodeError{Code: 2}`.

- **F3 (stale wording)** — `internal/cli/verify.go` doc block, the `verifyCmd.Long` help text, and `internal/workflow/verify.go` top-of-file scope comment all still claimed V2 was "recipe parses + op targets resolve" and that Slice A "ships V0/V1/V2 (… op targets resolve)". Rewrote to:
  - V2 = `recipe_parses` only.
  - V3 (`recipe_op_targets_resolve`) is a Slice C stub.
  - Slice A ships V0/V1/V2 as real, V3–V9 as stubs.
  Added an explicit "Exit code contract" block in `verify.go` referencing PRD §6 Q7 + §5 so future readers see the typed-exit invariant alongside the help text.

### Reproduction transcripts (supervisor's three cases, post-fix)

```
$ go build -o ./bin/tpatch-rev3 ./cmd/tpatch && BIN=$(pwd)/bin/tpatch-rev3

# Case A: missing slug, initialized workspace
$ tmp=$(mktemp -d) && (cd "$tmp" && git init -q && git config user.email t@t && git config user.name t)
$ $BIN init "$tmp" >/dev/null
$ $BIN --path "$tmp" verify nope; echo "EXIT=$?"
verify nope — failed
  ✗ [block-abort] status_loaded — could not load status.json: …features/nope/status.json: no such file or directory
  ⊘ [block] intent_files_present — skipped: V0 (status_loaded) aborted the run
  …
error: verify aborted: open …/features/nope/status.json: no such file or directory
EXIT=2

# Case B: non-tpatch workspace
$ empty=$(mktemp -d)
$ $BIN --path "$empty" verify nope; echo "EXIT=$?"
error: verify aborted: could not find .tpatch in this directory or any parent
EXIT=2

# Case C (bonus): V0 abort via corrupt status.json
$ $BIN --path "$tmp" add --slug demo demo >/dev/null
$ echo "{not valid json" > "$tmp/.tpatch/features/demo/status.json"
$ $BIN --path "$tmp" verify demo; echo "EXIT=$?"
verify demo — failed
  ✗ [block-abort] status_loaded — could not load status.json: invalid character 'n' looking for beginning of object key string
  …
error: verify aborted: invalid character 'n' looking for beginning of object key string
EXIT=2
```

All three previously leaked exit 1; all three now exit 2.

### Files Changed (Revision 3)

- `internal/cli/verify.go` — F1 wraps store-open and RunVerify-non-refusal errors in `ExitCodeError{2}`; F3 rewrites doc block + `Long` help to acknowledge V3 deferral and document the exit-code contract.
- `internal/workflow/verify.go` — F3 only: top-of-file scope comment updated (V2 = `recipe_parses`; V3 stubbed as Slice C). No behavioural change.
- `internal/cli/verify_test.go` — new file with the three F2 regression tests.
- `docs/handoff/CURRENT.md` — this Revision 3 section.

### Validation

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
$ go build ./cmd/tpatch && rm -f tpatch
(success)
```

ADR-013, PRD-verify-freshness.md, store types, store.go, the apply gate, the refusal path, V1's intent check, V2's strict decode, V3's deferral, and skill stubs are all untouched. Slice A boundary preserved.

**Status: ready for re-review.**
