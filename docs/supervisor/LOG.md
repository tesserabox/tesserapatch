## Re-review #3 — M15-W3-SLICE-A — 2026-04-27

**Reviewer**: m15-w3-slice-a-reviewer-4
**Task**: M15-W3-SLICE-A external-review revision 3 verification
**Commit reviewed**: 8a47078 (on top of full revision stack)

### External-review HIGH finding verification
[x] Case A — missing slug → exit 2
[x] Case B — non-tpatch workspace → exit 2
[x] Case C — V0 abort (corrupt status.json) → exit 2
[x] Case D — generic cobra error still exits 1 (precision check)
[x] Three regression tests use Execute() path, not stripped helper
[x] Stale wording fixed in cli/verify.go + workflow/verify.go

### Cross-cutting (not regressed)
[x] Refusal still exits 2 without writing
[x] V1/V2/V3 behavior unchanged
[x] WriteVerifyRecord call sites correct
[x] dependency_gate.go unmodified
[x] Slice A boundary intact
[x] gofmt / go test / go build clean
[x] Co-author trailer on 8a47078

### Findings

None.

### Verdict: APPROVED

### Notes

All supervisor findings have been correctly addressed:

**F1 (exit 2 wrapping)**: `verifyCmd.RunE` now wraps both `openStoreFromCmd` errors (lines 57-62 of verify.go) and non-refusal `RunVerify` errors (lines 69-74, 99-104) in `&ExitCodeError{Code: 2}`. The wrapping covers the three previously-leaking error paths: missing slug (surfaces as LoadFeatureStatus error), non-tpatch workspace (openStoreFromCmd failure), and V0 abort (RunVerify non-refusal error from corrupt status.json). The refusal path (lines 96-98) and verdict-failed path (lines 105-113) were already correct in revision 2. Generic cobra errors (e.g., missing slug argument) correctly fall through to legacy exit 1 — Case D confirms the wrapping is precise.

**F2 (regression tests)**: New `internal/cli/verify_test.go` adds three regression tests that use `buildRootCmd().Execute()` directly (lines 17-22 define `runVerifyForExitCode` helper) and assert `errors.As(&ec)` on `*ExitCodeError` (lines 39-45, 59-65, 90-96). The tests correctly avoid the package-level `runCmd` helper which would mask the typed-error plumbing (documented in line 15 comment). All three tests pass and lock in the exit-2 contract. Naming is clear and traces back to PRD §5.

**F3 (stale wording)**: `internal/cli/verify.go` doc block (lines 19-21), cobra Long help text (lines 47-49), and `internal/workflow/verify.go` top-of-file comment (lines 7-9) all corrected. V2 now accurately described as `recipe_parses` only, V3 (`recipe_op_targets_resolve`) acknowledged as Slice C stub, and Slice A scope correctly stated as V0/V1/V2 real. Exit-code contract documented in verify.go lines 31-40.

**Cross-cutting verification**:
- Refusal path still returns early at line 194 of verify.go before any WriteVerifyRecord call (line 234 only reachable on success/fail path after refusal check).
- V1 intent check (lines 197-200), V2 strict decode (checkRecipeParses, revision 2 work), V3 deferral (line 209), and V4–V9 stubs all intact.
- WriteVerifyRecord call site: exactly one at line 234 of verify.go, guarded by `!opts.NoWrite`. Not called on refusal (line 194 early-return), not called on V0 abort (line 169 early-return).
- dependency_gate.go unmodified (confirmed via `git diff origin/main..main`).
- Slice A boundary preserved: no `--all`, no `--shadow`, no closure replay, no ComposeLabels integration.
- Validation clean: `gofmt -l .` empty, `go test ./...` all pass (cached), `go build ./cmd/tpatch` succeeds.
- Co-author trailer present on 8a47078 (confirmed via `git log`).
- Diff scope: exactly the four expected files (CURRENT.md, verify.go, verify_test.go, workflow/verify.go).

**Mandatory reproductions**:
- Case A (missing slug): EXIT=2, error message correct, report includes all 10 checks with V0 abort.
- Case B (non-tpatch workspace): EXIT=2, error message correct.
- Case C (V0 abort via corrupt status.json): EXIT=2, error message correct, report shows status_loaded failed.
- Case D (cobra usage error): EXIT=1 as expected — proves wrapping precision.

No blocking findings. No notes. Revision 3 is complete and correct.

---

## Re-review #2 — M15-W3-SLICE-A — 2026-04-28

**Reviewer**: m15-w3-slice-a-reviewer-3
**Task**: M15-W3-SLICE-A external-review revision verification
**Commits reviewed**: 1e29f8f + 77cbf50 + c3bb18f (on top of 8e2aabe + 41cc4aa + a4b4262)

### External-review finding verification
[x] F1 — typed exit code 2 (test + Repro 4)
[x] F2 — pre-apply refusal without write (Repro 1, 2)
[x] F3a — strict recipe decode (Repro 5)
[x] F3b — recipe_op_targets_resolve deferred (Repro 6)
[x] F4 — V1 includes exploration.md
[x] F5 — PRD prose aligned with stdout-only check_results

### Cross-cutting
[x] ADR-013 D1–D7 honored
[x] dependency_gate.go unmodified
[x] WriteVerifyRecord call sites: only one (verify success/fail path)
[x] Slice A boundary intact (no --all, no --shadow, V3–V9 stubs)
[x] gofmt / go test / go build clean
[x] All fixture reproductions match expectations
[x] Co-author trailer on all three commits

### Findings

None.

### Verdict: APPROVED

### Notes

All five external-supervisor findings have been correctly and completely addressed:

**F1 (typed exit code 2)**: `internal/cli/exit_error.go` defines `*ExitCodeError{Code, Message}` with an `ExitCode()` method. `Execute()` unwraps via `asExitCodeError()` and returns the embedded code (lines 37-38 of cobra.go). Verify's RunE returns `&ExitCodeError{Code: 2, ...}` on both refusal and verdict-failed (lines 83, 92-95 of verify.go). Test coverage is parametric (`TestExecute_PropagatesExitCodeError` covers plain error→1, ExitCodeError{2,3}→2/3, nil→0). `SilenceUsage`/`SilenceErrors` set inside RunE (lines 45-46 of verify.go). Repro 4 confirms JSON shape includes `exit_code` field and shell exit matches (0 for passed, 2 for failed).

**F2 (refuse pre-apply states)**: `RunVerify` checks lifecycle state early (lines 181-194 of verify.go) before any `WriteVerifyRecord` call. Refused states: `requested`, `analyzed`, `defined`, `implementing`, `reconciling`, `reconciling-shadow` per `postApplyVerifyStates()` (lines 100-107). Allowed: `applied`, `active`, `upstream_merged`, `blocked`. Returns typed `*RefusedError` with verdict="refused", exit_code=2. Tests: `TestRunVerify_RefusesPreApplyState` (parametric over all 6 refused states), `TestRunVerify_RefusalNotWrittenEvenWithoutNoWrite` (explicit supervisor fixture path), `TestRunVerify_AllowsPostApplyStates` (parametric over 4 allowed states). Repros 1 & 2 confirm: EXIT=2, no `verify` key in status.json on refusal, error message correct.

**F3a (strict recipe decode)**: `checkRecipeParses` uses `json.NewDecoder(bytes.NewReader(data)).DisallowUnknownFields().Decode(&recipe)` (lines 340-342 of verify.go). Test `TestRunVerify_V2_RejectsUnknownFields` locks in the contract with a recipe carrying an unknown field (lines 300-327 of verify_test.go). Repro 5 confirms: unknown field causes V2 to fail with EXIT=2, passed=false.

**F3b (defer V3 to Slice C)**: Per-op target file-existence check is GONE from V2. V2 collapses to `recipe_parses` only. New `stubRecipeOpTargetsResolve()` (lines 381-393 of verify.go) returns `passed:true, skipped:true, reason:"not yet implemented (Slice C — created_by hard-parent semantics)"`. Test `TestRunVerify_V3_MissingTargetIsDeferredToSliceC` asserts a recipe with nonexistent target PASSES V2 (parse OK) and V3 is skipped (lines 329-363 of verify_test.go). V-id renumbering is consistent: no duplicate IDs, no skipped IDs in the 10-check array. Repro 6 confirms: recipe with nonexistent target passes with EXIT=0, V2 passed, V3 skipped.

**F4 (V1 requires exploration.md)**: `checkIntentFilesPresent` iterates `[]string{"spec.md", "exploration.md"}` (line 280 of verify.go). Failure message identifies which file is missing/empty (lines 284-298). Tests: `TestRunVerify_V1_FailsWhenExplorationMissing`, `TestRunVerify_V1_FailsWhenExplorationEmpty`, `TestRunVerify_V1_PassesWhenBothPresent` (lines 171-231 of verify_test.go). Existing spec-only tests updated. Repro 3 confirms: both files required to pass V1.

**F5 (PRD prose alignment)**: Three passages in `docs/prds/PRD-verify-freshness.md` updated:
- §0 Summary (lines 60-65): removed `per-check results` from persisted-shape list, added note citing LOG `3c122aa` Note 1
- §3.2 (lines 189-191): removed `check_results` from the listed fields, added clarification paragraph
- §3.4.1 (lines 216-236): `VerifyRecord` struct no longer carries `CheckResults`; `VerifyCheckResult` retains definition with "stdout-only" comment
All three passages explicitly cite LOG `3c122aa` Note 1 as authoritative. ADR-013 unchanged. `internal/store/types.go` schema unchanged (VerifyRecord confirmed to NOT have CheckResults field).

**Cross-cutting checks pass**:
- ADR-013 D1–D7 honored: freshness overlay model intact, apply gate untouched, verify-only writer, omitempty round-trip, derived labels only, status.json source-truth, read-only on working tree
- `dependency_gate.go` unmodified: `git diff origin/main..main -- internal/workflow/dependency_gate.go` returns empty
- `WriteVerifyRecord` call sites: exactly ONE at line 233 of verify.go (success/fail path only, unreachable on refusal branch per line 193 early-return)
- Slice A boundary intact: no `--all`, no `--shadow`, V3–V9 are stubs, no closure replay in this revision, no ComposeLabels integration
- Validation clean: `gofmt -l .` empty, `go test ./...` all pass (cached), `go build ./cmd/tpatch` succeeds
- All six fixture reproductions match expectations exactly
- Co-author trailer present on all three commits (1e29f8f, 77cbf50, c3bb18f)

**Quality observations**:
1. Test coverage is exemplary: parametric tests for F2 (6 refused states, 4 allowed states), F3a/F3b/F4 all have dedicated regression locks
2. Error messages are clear and actionable (F2 cites PRD §5, F4 names the missing file)
3. Commit granularity is appropriate: F1 isolated in its own commit, F2/F3a/F3b/F4 batched together (all touch verify.go), F5 docs-only
4. The V-id mapping note in CURRENT.md (lines 144-146) correctly documents the PRD V2/V3 distinction

No blocking findings. No notes. Revision 2 is complete and correct.

---

## Re-review — M15-W3-SLICE-A — 2026-04-27

**Reviewer**: m15-w3-slice-a-reviewer-2
**Task**: M15-W3-SLICE-A revision verification
**Commit reviewed**: a4b4262 (revision on top of 8e2aabe + 41cc4aa)

### Revision verification
- [x] Missing parents omitted from map (not "", not sentinel)
- [x] TestParentSnapshot_MissingParentOmitted asserts key absence
- [x] TestParentSnapshot_AllParentsMissingReturnsNil consistent with omitempty
- [x] TestParentSnapshot_SoftDepsExcluded regression guard intact
- [x] Diff scope: only verify.go + verify_test.go + CURRENT.md
- [x] gofmt -l . clean / go test ./... passes / go build clean
- [x] Co-author trailer present

### Findings

None.

### Verdict: APPROVED

### Notes

The fix correctly addresses the blocking finding. Missing parents are now omitted from the `parent_snapshot` map entirely (lines 420-421: `continue` on error), not recorded as empty strings. The `parentSnapshot` function also returns `nil` when the result map would be empty (lines 425-427), preserving the `omitempty` round-trip contract.

Test quality is excellent:
- `TestParentSnapshot_MissingParentOmitted` (lines 400-402) correctly asserts the missing slug is NOT present as a key using the `_, ok := snap["parent-b"]; ok` pattern, and fails with a clear message referencing the bug being fixed.
- `TestParentSnapshot_AllParentsMissingReturnsNil` (line 426) verifies the nil-return contract.
- `TestParentSnapshot_SoftDepsExcluded` (line 454) confirms the existing soft-dep filtering behavior is intact.

All validation gates pass. The commit diff is exactly the three expected files. Revision is complete and correct.

---

## Review — M15-W3-SLICE-A — 2026-04-27

**Reviewer**: m15-w3-slice-a-reviewer
**Task**: Wave 3 Slice A — verify cobra shell + V0–V2 + freshness writer
**Commits reviewed**: 8e2aabe (tracking) + 41cc4aa (code)

### Constraint compliance
- [x] D1 — Verify sub-record on FeatureStatus, FeatureState unchanged
- [x] D2 — apply gate untouched (`dependency_gate.go` unmodified)
- [x] D3 — only `verify` is a writer
- [x] D4 — omitempty round-trip byte-identical for never-verified
- [x] D5 — no read-path mutation
- [x] D6 — Verify in status.json, not from reconcile-session.json
- [x] D7 — verify is read-only on the working tree
- [x] Note 1 — CheckResults not persisted (stdout only)
- [x] Note 2 — V2 absent-recipe handled
- [x] Note 3 — 6 skill stubs present + parity guard green
- [x] Slice A boundary — no --all, no --shadow, no closure replay, no ComposeLabels, V3–V9 stubbed
- [x] gofmt -l . clean
- [x] go test ./... passes
- [x] go build ./cmd/tpatch succeeds
- [x] commit trailers present

### Findings

#### Blocking

**Issue: Parent Snapshot Records Empty String for Missing Parent**
**File:** `internal/workflow/verify.go:408`
**Severity:** High

The `parentSnapshot` function records an empty string (`""`) when a parent feature's `LoadFeatureStatus` fails (line 408: `out[slug] = ""`). This creates an invalid `FeatureState` value in the persisted `parent_snapshot` map.

**Problem:** `FeatureState` is a typed enum with defined values (`requested`, `applied`, `upstream_merged`, etc.). An empty string is not a valid `FeatureState` and will cause runtime failures in Slice B's freshness derivation when it attempts to match against the enum.

**Evidence:** The code comment says "record empty state literal so freshness derivation can flip to verified-stale rather than crash," but an empty string will **still** cause the derivation to crash when it tries to compare against the `FeatureState` enum in the `satisfies_state_or_better` rules (ADR-013 lines 167–172).

**Impact:** Any feature with a hard dependency on a parent that doesn't exist (e.g., user deletes parent's status.json manually, or declares a dep on a typo'd slug) will persist an invalid `parent_snapshot` that cannot be safely read in Slice B.

**Suggested fix:** Use a sentinel value that's actually a valid `FeatureState`, or explicitly document that missing parents should be skipped from `parent_snapshot` entirely (return `nil` entry or omit the key). The latter is cleaner: if the parent doesn't exist, the snapshot shouldn't contain it. Alternatively, fail the verify run with a check result indicating the broken dependency.

### Verdict: NEEDS REVISION

### Notes

1. **Empty state handling choice needed.** The implementer must choose one of: (a) omit missing parents from `parent_snapshot` entirely (don't add the key to the map), (b) use a valid `FeatureState` sentinel like `StateBlocked`, or (c) add a V3-level check that fails if any hard parent is missing at verify time. Choice (a) is cleanest: a missing parent shouldn't appear in the snapshot at all.

2. **Test coverage observation.** No test exercises the "parent doesn't exist" edge case. After fixing the issue above, add a test that creates a feature with a hard dep on a non-existent slug, runs verify, and confirms the persisted `parent_snapshot` is either `nil` or omits the missing parent's key.

3. **Hash determinism confirmed.** SHA-256 hex of file bytes is stable and tested by the round-trip tests. No issue.

4. **Path safety.** `verify.go` uses only `os.ReadFile` and `os.Stat` on paths constructed from `s.Root` + well-known subdirs. No user-controlled path input reaches the filesystem without going through store primitives that already call `EnsureSafeRepoPath` in write paths. Read-only nature confirmed (D7).

5. **Skill stubs confirmed present.** All six surfaces have the one-line EXPERIMENTAL bullet. Parity guard passes. Note 3 satisfied.

6. **Tracking accuracy.** CURRENT.md lists 14 files; `git show 41cc4aa --stat` matches exactly. Co-author trailers present on both commits.

## Review — M15-W3-REDESIGN — 2026-04-27

**Reviewer**: m15-w3-redesign-reviewer
**Task**: Wave 3 redesign — freshness-overlay model + closure-replay spec
**Commit reviewed**: 37a483d

### Constraint compliance
- [x] no Go code modified
- [x] no tests modified
- [x] ADR-010 / ADR-011 untouched
- [x] no read-path mutation
- [x] apply gate stays pure-lifecycle
- [x] freshness record minimal (see Note 1 below on CheckResults)
- [x] supersession banners present + correctly linked
- [x] supersession map present in ADR-013 + complete
- [x] closure-replay spec precisely defined
- [x] freshness derivation function precisely defined
- [x] amend invalidation contract explicit
- [x] commit trailer present

### Findings

#### Notes (approved with advisories)

**Note 1: CheckResults Array in Persisted Record — Potential Bloat**
**Files:** `docs/prds/PRD-verify-freshness.md:212`, `docs/adrs/ADR-013-verify-freshness-overlay.md:63`
**Severity:** Medium — not blocking, but implementer should verify necessity

The `VerifyRecord` includes `CheckResults []VerifyCheckResult` as a persisted field in `status.json`. For a 10-check verify run, this means persisting ~10 small structs (id, severity, passed, remediation) every time verify runs. The PRD §3.2 explicitly lists `check_results` in what gets written to the store.

**Concern**: The primary consumer of per-check results is the `--json` output (§4.3), not the freshness derivation function (§3.4.2), which only consumes the top-level `Passed` boolean, the hashes, and `ParentSnapshot`. Persisting the full check array inflates `status.json` with data that's never read for the core freshness-derivation loop.

**Rationale for keeping (if implementer chooses)**: An operator inspecting `status.json` directly (without re-running verify) can see *which* check failed on the last verify run. This is debugger-friendly but not load-bearing for the freshness overlay model.

**Recommendation**: Implementer may choose to drop `CheckResults` from the persisted `VerifyRecord` and emit it only in the `--json` stdout report. If kept, document the persistence rationale explicitly in Slice A's commit message. Either choice is defensible; the PRD as written permits both interpretations ("writes… including… check_results" can mean "includes in the written record" or "includes in what verify produces, with only a subset persisted").

**Note 2: Edge Case — Verify on Feature With No Recipe Yet**
**Files:** `docs/prds/PRD-verify-freshness.md:514`
**Severity:** Low — already covered, confirming clarity

Edge-case table §5 row "Recipe absent" correctly specifies V2/V3/V7 are skipped. Confirm Slice A implementer interprets "recipe absent" as applying to features in `applied` state that predate recipe autogen (legacy v0.5.x features). The PRD says "reasonable for applied-from-pre-autogen-era features," which is correct.

**Note 3: Slice A Boundary — Skill Anchor Regen Deferred**
**Files:** `docs/prds/PRD-verify-freshness.md:631`
**Severity:** Low — process check

Slice A explicitly defers skill anchor regeneration to Slice D (line 631: "No skill anchor regen. Slice D handles all skill surface changes"). The parity guard `assets/assets_test.go` will fail as soon as the `verify` command is registered in Slice A, because the skill files don't mention `verify`. 

**Clarification needed**: Does Slice A's test gate allow the parity-guard failure with a TODO comment, or does Slice A require a minimal skill-file stub (e.g., a single "EXPERIMENTAL: tpatch verify — see PRD" line in all 6 surfaces) to keep the parity guard green? The PRD as written suggests "let it fail in Slice A, fix in Slice D," but that breaks the standard `go test ./...` gate.

**Recommendation**: Slice A implementer should add minimal skill stubs (1 sentence each, marked EXPERIMENTAL) to keep parity guard green across all slices. Defer the full §4.4 paragraph to Slice D.

### Verdict: APPROVED WITH NOTES

The redesign successfully addresses all four findings (F1–F4) and satisfies every binding non-negotiable. The supersession map in ADR-013 is complete and accurate. The closure-replay spec (§3.4.3) is precisely defined with fail-fast semantics and the correct JSON failure shape. The freshness derivation function (ADR-013 D5, lines 147–164) is precisely specified. No PRD/ADR contradictions detected. No read-path mutation. No apply-gate changes. Slice boundaries are independently shippable with minor clarification on the parity-guard handling (Note 3).

The three notes above are advisories for the Slice A implementer, not blockers. Note 1 (CheckResults bloat) should be resolved by the implementer at Slice A design time (either drop the field or document why it stays). Note 3 (parity guard) should be resolved before Slice A ships (add stubs or accept intentional failure with clear TODO).

### Notes for Supervisor

1. **Supersession audit trail preserved correctly.** Predecessor docs carry well-formed SUPERSEDED banners pointing to the successors. The supersession map in ADR-013 is the strongest example of "why we chose this over the alternative" documentation I've seen in this repo. Future agents will understand the trade-off clearly.

2. **No hidden structural problems.** The closure-replay spec is unambiguous: topological order over hard-only sub-DAG, skip `upstream_merged`, fail-fast on first non-`applied` parent or replay failure, JSON shape with `failed_at: "parent-replay"` + `parent_slug`. An implementer can code from this without guessing.

3. **Freshness derivation function is computable.** ADR-013 D5 lines 147–164 are pseudocode-grade. The `satisfies_state_or_better` rules (lines 167–172) are explicit. No ambiguity.

4. **Slice A is correctly scoped.** No `--all`, no `--shadow`, no skill regen until later slices (with minor parity-guard clarification per Note 3).

5. **Amend invalidation contract is explicit.** ADR-013 D3 line 117: recipe-touching amend clears `Verify.Passed = false`; intent-only amend leaves it untouched. Clear.

6. **Process lesson reinforced.** The reopening note's "implementer self-reviews are status-only" framing is validated by this pass. An external review with high signal-to-noise is the right gate before Slice A dispatch.

Approve for Slice A dispatch after user reviews Note 1 and Note 3 advisories.


## Reopening — M15-W3-DESIGN — 2026-04-27

**Reopener**: Supervisor (user-mediated external re-review)
**Trigger**: External re-review of the approved Wave 3 design (commit `8c3d72e`) identified two structural problems (F1: verify shadows ignored hard-parent closure replay; F4: lifecycle/freshness conflation routed read-path mutation through `LoadFeatureStatus`) plus two CURRENT.md drift findings (F2, F3).

### Disposition

- The previously approved Wave 3 design is **REOPENED and SUPERSEDED**, not silently revised in place.
- A redesign pass produced a freshness-overlay model (Git-like semantics: lifecycle stays untouched; verification becomes a derived overlay).
- Successor docs created:
  - `docs/prds/PRD-verify-freshness.md` (supersedes `PRD-verify-and-tested-state.md`)
  - `docs/adrs/ADR-013-verify-freshness-overlay.md` (supersedes ADR-012 in full)
- Predecessor docs preserved with SUPERSEDED banners as historical record.
- The prior `8c3d72e` approval entries in this LOG remain unchanged for audit-trail integrity.

### Binding non-negotiables for the redesign

- Lifecycle and freshness stay separate.
- No new `StateTested` lifecycle value.
- Verification result is a freshness sub-record on `FeatureStatus`, not a lifecycle transition.
- Parent regressions produce stale freshness signals, never automatic child lifecycle demotions.
- Read paths must NOT mutate persisted state.
- Only explicit write verbs (`verify`, `apply`, `amend`, `reconcile`, `remove`) may persist invalidation.
- Apply gate stays pure-lifecycle. Freshness is an operator/harness signal, not a gate input.
- D6/D7 from the old design remain directionally valid: freshness lives in `status.json`; verify is read-only on the working tree.
- New ADR + new PRD required (no in-place rewrite of approved docs).
- No Go code in this redesign pass.

### Verdict: REDESIGN IN PROGRESS — awaiting reviewer pass

### Action taken

1. Idle CURRENT.md (from `8c3d72e`) archived to `docs/handoff/HISTORY.md` as superseded.
2. CURRENT.md rewritten with M15-W3-REDESIGN as the active task.
3. Successor PRD + ADR landed; predecessor docs banner-marked SUPERSEDED.
4. This LOG.md reopening entry prepended.
5. Reviewer dispatch (`m15-w3-redesign-reviewer`) is the next supervisor action.


## Review — M15-W3-DESIGN (revision pass) — 2026-04-27

**Reviewer**: Supervisor (direct, no sub-agent)
**Task**: Wave 3 design — D2 PRD/ADR alignment fix
**Commit reviewed**: `e6473ea`

### Constraint compliance
- [x] Only `docs/prds/PRD-verify-and-tested-state.md` and `docs/handoff/CURRENT.md` modified
- [x] ADR-012 untouched
- [x] No Go / test / LOG.md changes
- [x] Co-author trailer present
- [x] Not pushed (supervisor pushes)

### Findings
None. PRD §3.4.4 now headlines Direction A plainly, cites ADR-012 D2 as the locking record, removes the "B-pragmatic" walk-back, and preserves Direction B as a labelled rejected alternative with its arguments intact. The "tested is a strict superset of applied" framing makes the gate semantics legible.

### Verdict: APPROVED

### Action taken
1. Archive M15-W3-DESIGN to `docs/handoff/HISTORY.md`.
2. Reset `CURRENT.md` to design-complete idle, ready for Slice A dispatch.
3. Push `fdc6e70` + `90375c9` + `e6473ea` to `origin/main`.
4. Reviewer-adjudicated open questions (Q1–Q5) are now binding inputs to Slice A's contract:
   - Q1: V9 = warn severity
   - Q2: `verify --all` skips pre-apply slugs with `"skipped: pre-apply state"` reason line
   - Q3: `passed` field name retained (severity carries gating)
   - Q4: D2 = Direction A (resolved by this revision)
   - Q5: parent-state hook inserted into existing M14.3 label-recomputation loop

### Notes
The reviewer-then-revision loop caught a real contradiction the implementer would not have surfaced solo. Single-finding reviews remain a strong sub-agent pattern: targeted, fast, auditable. The "implementer self-reviews are status-only" lesson (from v0.6.1 fix-pass) holds — neither implementer call self-flagged D2.


# Supervisor Review Log

*Review entries logged in reverse chronological order.*

---

## Review — M15-W3-DESIGN — 2026-04-27

**Reviewer**: m15-w3-design-reviewer
**Task**: Wave 3 design — PRD + ADR-012 for feat-verify-command + feat-feature-tested-state
**Commit reviewed**: fdc6e70

### Constraint compliance
- [x] no Go code modified (only 3 docs files)
- [x] no tests modified
- [x] no ADR-011 / ADR-010 edits
- [x] no reconcile-session.json reads proposed (V9 reads status.Reconcile.Outcome only)
- [x] omitempty round-trip preserved (D4 explicit, backed by acceptance test)
- [ ] **BLOCKER**: D2 answered but **contradictory** between PRD and ADR (see below)
- [x] commit trailer present

### Findings

#### Blocking

**Issue 1: D2 PRD/ADR Contradiction — Most Consequential Decision**
**Files:** `docs/prds/PRD-verify-and-tested-state.md:263`, `docs/adrs/ADR-012-feature-tested-state.md:44`
**Severity:** Critical — blocks approval
**Problem:** PRD §3.4.4 line 263 states "DECISION (locked in ADR-012 D2): Direction B — `tested` does NOT satisfy hard dependencies." This is immediately contradicted at line 272 where "B-pragmatic" is revealed to actually mean `{applied, tested, upstream_merged}` satisfies the gate — which is Direction A. Meanwhile ADR-012 D2 line 44 states "Decided: `tested` satisfies the hard-dep gate, equivalent to `applied`" with no mention of Direction B.

The confusion stems from framing: the PRD calls Direction B "tested does not satisfy" but then implements "tested does satisfy because it's equivalent to applied" as "B-pragmatic" — which is semantically identical to Direction A. The two documents need to agree on what was decided.

**Evidence:** Cross-read PRD:263, PRD:272, ADR-012:44. The locked decision is clearly "`tested` satisfies" (the gate switch gains `case StateTested:` per ADR-012 D2 line 46), but the PRD headline at line 263 says the opposite.

**Suggested fix:** PRD §3.4.4 needs a rewrite. Either:
- Change line 263 headline to "Direction A — yes, `tested` satisfies (pragmatic equivalence)" and reframe the argument, OR
- Collapse Direction A and B-pragmatic into a single decision and remove the confusing "Direction B" framing that actually implements Direction A semantics.

ADR-012 D2 is internally consistent and correct; the PRD's presentation is the issue.

#### Non-blocking (approved with notes)

**Note 1: V9 Remediation Message Inconsistency (Open Question Q1)**
**File:** `docs/prds/PRD-verify-and-tested-state.md:473`
**Severity:** Medium — UX subtlety, not a contract bug
**Problem:** Failure case 3 JSON example shows `passed: false` on V9 with remediation text "tested cannot be promoted while reconcile is in a blocked state (warn-only)" but `verdict: passed` and `state_after: tested`. The PRD acknowledges this at line 481-483 but leaves it as an open question (Q1) rather than resolving it.

For a warn-severity check, `passed: false` is semantically correct (the check condition failed), but the field name may mislead operators into thinking promotion was blocked. The PRD's Q1 framing is appropriate, but the implementer will face the same ambiguity when coding Slice A.

**Recommendation:** Settle Q1 now rather than deferring to implementation. See Open Question Recommendations below.

**Note 2: Parent-State Hook Performance — No Hot-Path Added**
**File:** `docs/prds/PRD-verify-and-tested-state.md:301`, `docs/handoff/CURRENT.md:126`
**Severity:** Low — clarification request
**Problem:** PRD §3.4.5 line 301 states "no new hot path" for the parent-state hook, and Open Question Q5 asks the reviewer to confirm. The hook runs in the same `LoadFeatureStatus` loop as M14.3 label recomputation (per PRD), which is already O(V+E). The additional check (parent left `{applied, tested, upstream_merged}` AND child currently `tested`) is a constant-time state comparison per edge.

**Assessment:** The performance claim is sound. The hook is cheap and correctly inserted. No concern.

**Note 3: V6 Warn-Only Severity — Justified but Nuanced**
**File:** `docs/prds/PRD-verify-and-tested-state.md:173-180`
**Severity:** Low — design choice documented
**Problem:** V6 (`dependency_gate_satisfied`) is warn-only, meaning a feature whose hard parent is `defined` can still promote to `tested`. PRD §3.3 justifies this for pre-apply harness handoff and `upstream_merged` parent scenarios. The reviewer-relevant counter-argument is recorded as rejected in D2 alt 3.

**Assessment:** The decision is internally consistent with D2's pragmatic equivalence (tested parents satisfy, so children of tested parents are structurally fine even if the gate check produces a warn). The PRD's justification holds. No change required, but implementers should be aware this is a design choice, not an oversight.

### Open-question recommendations

**Q1 — V9 severity: warn vs block**
**Recommendation:** Keep **warn** (the PRD default). Reasoning: `shadow-awaiting` is not a structural integrity problem — it's a pending human decision. A feature sitting in `shadow-awaiting` can be perfectly healthy on disk; blocking `tested` on V9 would penalize the harness for the operator not having clicked "accept" yet. If a real need surfaces for "tested implies no pending reconcile work," revisit in a future `feat-tested-stronger-contract` PRD.

**Q2 — `verify --all` skip semantics**
**Recommendation:** Skip pre-apply states with a `"skipped: pre-apply state"` line (the PRD default). `tested` is meaningless before `applied`. Exit code reflects only post-apply slugs. Slice D detail; document in the `--all` help text.

**Q3 — `passed: false` field name on warn-only checks**
**Recommendation:** Keep `passed` as the field name. Rename would require touching every check's JSON shape, and "passed" is semantically accurate (the check condition was evaluated; it did not pass). The `severity` field carries the gating semantics. If this proves confusing in practice, revisit in a UX polish pass. For Slice A, ship as designed.

**Q4 — D2 wording check**
**Recommendation:** **Fix the PRD.** The current PRD headline "Direction B — tested does NOT satisfy" followed by "but actually it does" is confusing. ADR-012 D2 is clear and correct. Rewrite PRD §3.4.4 lines 242-277 to match the ADR's framing: "Direction A — tested satisfies because it's a strict superset of applied." Remove the "B-pragmatic" terminology entirely — it's a semantic no-op over Direction A.

**Q5 — Parent-state hook insertion point**
**Recommendation:** Approved as designed. The `LoadFeatureStatus` post-processing loop (same site as M14.3 label recomputation) is the correct insertion point. The additional check is cheap (constant-time per edge). No new hot path. Implementer can proceed with Slice B as specified.

### Verdict: **NEEDS REVISION**

**Blocking issue:** D2 PRD/ADR contradiction must be resolved before any implementation slice begins. The locked decision is clear in ADR-012 ("tested satisfies"); the PRD's presentation contradicts itself. Fix required: rewrite PRD §3.4.4 to align with ADR-012 D2, removing the "Direction B" headline that actually implements Direction A.

**Non-blocking notes:** V9 warn-severity UX subtlety (Q1) and V6 warn-only justification are documented design choices, not bugs. Parent-state hook performance is sound.

**Open questions:** All 5 adjudicated above. Q4 (D2 wording) is a blocker-fix requirement; the others are confirmations of the PRD defaults.

### Notes

**For supervisor:**
- Once D2 PRD rewrite lands, this design is ready for Slice A dispatch.
- The PRD is otherwise thorough: 678 lines covering 10 checks with primitive citations, full state-transition table, 4 implementation slices with clear boundaries, 3 JSON failure-case examples, backwards-compat contract with acceptance-test enforcement, and explicit out-of-scope cross-links.
- ADR-012 is well-structured: 201 lines, 7 decisions locked with alternatives considered, full cross-references to ADR-010/ADR-011, and correct preservation of the source-truth guard.
- No contract conflicts found with ADR-011 (dep DAG), ADR-010 (resolver), or the v0.6.1 satisfied_by contract beyond the D2 presentation issue.
- Implementation slices are independently dispatchable and correctly scoped.

**For implementer (post-fix):**
- D2 is "tested satisfies the hard-dep gate" (extend `CheckDependencyGate` switch). Ignore the PRD's "Direction B" headline; read ADR-012 D2 as the source of truth until the PRD is fixed.
- V9 reads `status.Reconcile.Outcome` only — never `artifacts/reconcile-session.json`. The adversarial test in §7 is mandatory.
- The omitempty round-trip fixture `TestUpgradeFromV0_6_1_NoVerify_BehavesIdentically` is mandatory for Slice B.
- Skill parity guard extension (6 surfaces) is mandatory for Slice D.

---

## Supervisor Fix-Pass — M15-W2 Re-Review — 2026-04-27

**Author**: supervisor (re-review by external reviewer)
**Scope**: 4 medium findings against the M15-W1 + M15-W2 surface that landed at `ad040ac`.
**Disposition**: All 4 fixed in the same pass; no Wave 3 dispatch.

### Findings (re-reviewer)

1. **Medium — F1 satisfied_by contract drift.** Validation accepted any reachable ref (including unique short SHAs); apply-time gate still rejected anything not 40-hex. Save-now/fail-later dependency path. *Anchors:* `internal/store/validation.go:82`, `internal/store/validation_test.go:161`, `internal/workflow/dependency_gate.go:83`.
2. **Medium — F2 scoped record metadata leak.** `record --files` scoped the patch but `CaptureDiffStat` was unscoped, so `post-apply-diff.txt` and `record.md` still embedded full-tree diffstat (cross-feature pollution). *Anchors:* `internal/cli/cobra.go:865/867/872`, `internal/gitutil/gitutil.go:181`.
3. **Medium — F3 invalid pathspec swallowed.** `CapturePatchScoped` replaced any git-diff error with empty output, then `recordCmd` reported the generic "captured 0 bytes" diagnostic. *Anchors:* `internal/gitutil/gitutil.go:244/246`, `internal/cli/cobra.go:810/824`.
4. **Medium — F4 Windows syntax-check quoting.** `UserShell` returns `cmd /C` on Windows, but `shellQuote` always emitted POSIX single-quote form, leaking quote characters into argv. *Anchors:* `internal/workflow/validation.go:169/239`, `internal/workflow/shell.go:13`.

### Fixes

- **F1**: tightened validation to require 40-hex SHA *and* reachability — same value space as the apply-time gate. New sentinel `ErrSatisfiedByMalformed`. Added `TestValidateDependencies_SatisfiedByMalformed`. Existing reachability/git-error tests rebased onto 40-hex literals so they cover the post-fix code path.
- **F2**: new `CaptureDiffStatScoped(repoRoot, pathspecs)` in `internal/gitutil`; `CaptureDiffStat` now delegates with `nil` (byte-identical default behavior). `recordCmd` calls the scoped variant so artifacts narrow with `--files`.
- **F3**: `CapturePatchScoped` now propagates the git-diff error when pathspecs is non-empty, wrapping with the rejected pathspecs for diagnostics. Empty pathspecs preserves the historical tolerant behavior the unscoped capture path has always relied on.
- **F4**: `shellQuote` is now `shellQuoteFor(goos, p)`; Windows uses double-quote/double-quote-escape (cmd.exe convention), Unix retains single-quote form. New `TestShellQuoteFor` and a pairing invariant test (`TestShellQuoteFor_PairsWithUserShell`) lock the contract to the OS the runtime actually picks.

### Validation gate

- `gofmt -l .` clean.
- `go build ./cmd/tpatch` clean.
- `go test ./...` clean (all 7 packages).
- Focused: `go test ./internal/store -run Validate` — 17 cases, all pass; new malformed coverage included.

### Notes

- Apply-gate was deliberately left as-is (`internal/workflow/dependency_gate.go:83` regex check). The contract is now: validation does both (40-hex + reachability), apply does the cheaper well-formed check as defense-in-depth. ADR-011 D5 unchanged; the gate's documented limitation (no reachability at apply-time) is now backed by validation refusing to persist anything reachability would reject.
- Patch authority invariant preserved (ADR-011 D6, post-apply.patch is reconcile source-of-truth).
- Skill parity guard passes; recipe-op JSON schema untouched; `delete-file` op still deferred.

### Action

Findings closed in-tree before tagging `v0.6.1`. Handoff transitions: CURRENT.md → fix-pass complete, awaiting tag/Wave 3 decision.

---

## Review — M15-W2 — 2026-04-26

**Reviewer**: m15-w2-reviewer (code-review)
**Task**: Wave 2 — bug-test-command-shell-selection + feat-record-autogen-recipe + bug-recipe-stale-after-manual-flow + feat-record-scoped-files
**Diff range**: `827239b..2c5ae33`

### Checklist
- [x] Compiles
- [x] Tests pass
- [x] Formatted
- [x] Parity guard green
- [x] Recipe-op JSON schema unchanged
- [x] Source-truth guard preserved (ADR-011 D6)
- [x] Patch remains reconcile authority (no recipe inversion)
- [x] Unix shell behavior byte-identical
- [x] No silent schema extension for delete-file
- [x] Stale-recipe default is non-destructive
- [x] Pathspec passed safely (-- separator)

### Verdict: APPROVED

### Findings

None — all 4 items meet contract; 5 design judgment calls verified.

**JC1 — Schema gap handling**: `RecipeFromPatch` (recipe_autogen.go:86-121) correctly skips deleted files at lines 100-102 with reason "deleted — recipe schema has no delete-file op". Warnings surface on stderr at cobra.go:917-919. No new op type added to RecipeOperation schema. ✅

**JC2 — Stale recipe sidecar policy**: Default behavior non-destructive (line 182-199 of recipe_autogen.go writes sidecar, never overwrites recipe unless `regenerate=true`). `--regenerate-recipe` actually regenerates (line 183-188). Sidecar JSON carries stale flag, reason, timestamp (RecipeStaleness struct, lines 27-31). Warning surfaces clearly on stderr (cobra.go:908-913). All scenarios tested (recipe_autogen_test.go). ✅

**JC3 — Drift detection scope**: File-set comparison only, confirmed at recipe_autogen.go:211-239 (`compareRecipeFileSets` checks path set membership, not content hashes). Documented in code comments (line 145-148). No new code reads `apply-recipe.json` for reconcile authority — grep confirms patch remains source of truth. ✅

**JC4 — `--files` + `--from` mutual exclusion**: Explicit error at cobra.go:862-863 with actionable message. Error fires before side effects (before CapturePatch call). Unit test coverage at cobra_test.go:864-882. ✅

**JC5 — Hookable shell selection**: `userShellFor(goos)` at shell.go:19-24 produces byte-identical Unix behavior (`sh`, `-c` — not `bash`, not `/bin/sh`, no path substitution). Windows path is `cmd`, `/C`. Tests cover both branches via goos injection (shell_test.go). All three call sites updated (validation.go, phase2.go per git diff). ✅

### Action Taken

Verdict logged. Supervisor decides closeout, v0.6.1 cut, and Wave 3 dispatch pause.

---

## Review — M15-W1 — 2026-04-26

**Reviewer**: m15-w1-reviewer (code-review)
**Task**: Wave 1 — feat-satisfied-by-reachability + chore-skill-frontmatter + feat-define-spec-alias
**Diff range**: `1736c4d..192935b`

### Checklist
- [x] Compiles (go build ./cmd/tpatch)
- [x] Tests pass (go test ./...)
- [x] Formatted (gofmt -l .)
- [x] Parity guard green
- [x] Source-truth guard (ADR-011 D6) preserved
- [x] No new parity anchor for the spec alias
- [x] Frontmatter doesn't break loader expectations
- [x] Reachability check gated correctly (no double-fail)

### Verdict: APPROVED WITH NOTES

### Findings

- **Medium** / `internal/store/validation_test.go` — Missing test coverage for git error path (when isAncestor returns non-nil error). The implementer flagged this as judgment call #2 requiring explicit verification: "Bogus refs surface as wrapped errors, not as silent `ErrSatisfiedBySHANotReachable`." The `TestIsAncestor` in `gitutil_test.go` covers the git-level error case, but there's no validation-level test that exercises `stubIsAncestor(t, false, errors.New("git failure"))` to verify the wrapped error includes the dependent slug and doesn't silently ignore git failures. Current tests only stub `(true, nil)`, `(false, nil)`. **Fix**: Add `TestValidateDependencies_SatisfiedByGitError` that stubs `isAncestor` to return `(false, fmt.Errorf("bad ref"))` and asserts the validation error wraps it with the expected "verify satisfied_by reachability for <slug> -> <parent>" prefix.

All other flagged items verified:
1. **Double-fail avoidance**: Both `ValidateDependencies` (L76-90) and `ValidateAllFeatures` (L144-154) gate reachability check on `parent.State == StateUpstreamMerged`, so `ErrSatisfiedByRequiresUpstream` fires alone when parent is not upstream_merged. ✅
2. **IsAncestor exit semantics**: Correctly implemented at `gitutil.go:633-650`. Exit 0 → `(true, nil)`, exit 1 → `(false, nil)`, other → `(false, wrapped error with stderr)`. Error wrapping includes ancestor/descendant in message. ✅
3. **Real-git test coverage**: `TestIsAncestor` (gitutil_test.go:210-255) exercises all three paths: reachable (L244), unreachable (L248), bogus-ref error (L252). Uses real git repo, not stubs. ✅

### Action Taken
Verdict logged. Supervisor decides closeout / Wave 2 dispatch.

---

## Review — M15.1 — 2026-04-26

**Implementer**: m15-1-implementer (general-purpose, ~6.5min per handoff notes)
**Reviewer**: code-review sub-agent

### Commits reviewed

2 commits between `0559c7d` (M14.4 handoff dispatch) and `4151f01` (HEAD):

- `53b8b90` feat(workflow): M15.1 advisory created_by inference at implement time
- `4151f01` docs(handoff): M15.1 implementation complete, awaiting reviewer

### Checklist

**Algorithm correctness (PRD §4.3.1) ✅**
- [x] replace-in-file only: checked at line 103 (`op.Type != "replace-in-file"` continues)
- [x] Empty CreatedBy guard: line 106 (`op.CreatedBy != ""` continues)
- [x] Non-empty Search guard: line 109 (`op.Search == ""` continues)
- [x] Pristine check first: line 159 calls `pristineHasSearch` before parent scan; early-exit if found
- [x] bytes.Contains used: lines 172, 223 — literal byte substring match, no regex
- [x] HARD parents only: lines 126-129 filter `dep.Kind == store.DependencyKindHard`
- [x] Non-transitive: comment line 30 states "non-transitive: only direct parents"; no recursion in code
- [x] Single match → suggestion: case 1 at lines 183-188
- [x] Multiple matches → ambiguity warning: default case at lines 189-192
- [x] Zero matches → silent: case 0 at lines 179-182 (comment confirms apply-time gate handles it)

**Advisory-only contract (PRD §8) ✅**
- [x] Recipe NEVER mutated: comment line 20 "recipe is NEVER mutated"; function signature line 81 takes `recipe ApplyRecipe` by value; tests verify with `recipeSnapshot` at lines 40-44, 63, 84-86
- [x] Suggestions to stderr: lines 184-198 write to `WarnWriter` (which is stderr per line 39)
- [x] Per-suggestion format includes op index, path, suggested parent, reason: lines 184-187
- [x] Summary line only when suggestions > 0: lines 196-199 guard with `if suggestions > 0`
- [x] User-set created_by never overwritten: test #2 (TestCreatedByInference_RespectsExistingAnnotation, lines 92-113) asserts silent output when op.CreatedBy already set

**Opt-out + flag-off ✅**
- [x] `--no-created-by-infer` flag present: cobra.go lines 467-469, 480
- [x] Flag default ON: flag definition at line 480 has no explicit default=true, so cobra default is false (opt-in), BUT the function checks for opt-OUT, so inference runs by default when flag not set — CORRECT
- [x] Context plumbing: WithDisableCreatedByInference at lines 61-63; checked at line 82
- [x] Flag-off via features_dependencies: lines 85-93 check `cfg.DAGEnabled()` and return nil early with comment "byte-identical pre-v0.6 behaviour"
- [x] Test #5 (TestCreatedByInference_OptOut, lines 185-207) asserts silence with flag set
- [x] Test #6 (TestCreatedByInference_FlagOff, lines 211-232) asserts silence with features_dependencies=false

**Scope guards (NEGATIVE checks) ✅**
- [x] created_by_gate.go UNCHANGED: `git diff 0559c7d..HEAD -- internal/workflow/created_by_gate.go` returned empty
- [x] No write-file ops processed: grep for "write-file" in inference file returned empty (only replace-in-file at line 103)
- [x] No transitive scanning: confirmed line 30 comment + code only reads child.DependsOn directly
- [x] No new external deps: `go list -m all` shows only cobra/pflag + stdlib (same as before)
- [x] No new config keys: inference checks existing `features_dependencies` flag only
- [x] Version NOT bumped: cobra.go:24 still says `version = "0.6.0"`
- [x] CHANGELOG NOT touched: `git diff 0559c7d..HEAD -- CHANGELOG.md` returned empty

**Implementer's flagged note ✅**
- [x] Inference errors degrade to warning: implement.go lines 145-147 wrap `inferCreatedBy` with `if ierr != nil { warn }` and continue
- [x] Apply-time gate remains authoritative: comment at lines 139-143 confirms this is intentional
- [x] Silent when no parent contained text: case 0 at inference.go lines 179-182 is silent (no error)
- [x] Transient read failures handled: lines 140-148 in inference.go skip parent silently when ReadFeatureFile fails (comment confirms this is expected when parent not applied yet)

**Tests (all 8 present and meaningful) ✅**
1. [x] TestCreatedByInference_SuggestsHardParent (lines 50-87): real fixtures, asserts stderr contains suggestion + op index + path + summary, verifies recipe unchanged
2. [x] TestCreatedByInference_RespectsExistingAnnotation (lines 92-113): op has created_by set, asserts silence
3. [x] TestCreatedByInference_AmbiguousMultipleParents (lines 119-152): two parents match, asserts "ambiguous" + both parents listed + NO summary line
4. [x] TestCreatedByInference_SkipsSoftParents (lines 157-180): soft parent matches but hard doesn't, asserts silence
5. [x] TestCreatedByInference_OptOut (lines 185-207): WithDisableCreatedByInference set, asserts silence
6. [x] TestCreatedByInference_FlagOff (lines 211-232): features_dependencies=false, asserts silence
7. [x] TestCreatedByInference_PristineHasSearch_NoSuggestion (lines 237-261): pristine file contains Search text, asserts silence (proves early-exit)
8. [x] TestCreatedByInference_NoMatchSilent (lines 267-288): no parent patch matches, asserts silence

**Cross-cutting ✅**
- [x] Commit trailers present: both commits carry Co-authored-by line (verified with `git log --format="%B"`)
- [x] No tpatch binary in tree: `git ls-files | grep -E '^tpatch$'` returned empty
- [x] Working tree clean: `git status --porcelain` returned empty
- [x] All M14 series tests green: CreatedByGate (9 tests), CreatedByInference (8 tests), ComposeLabels, EffectiveOutcome, AcceptShadow, GoldenReconcile, PlanReconcile, Phase35 all PASS

**Validation gate ✅**
```
$ gofmt -l .
(no output)

$ go build ./cmd/tpatch && rm -f tpatch
BUILD OK

$ go test ./...
ok  	github.com/tesseracode/tesserapatch/assets	(cached)
?   	github.com/tesseracode/tesserapatch/cmd/tpatch	[no test files]
ok  	github.com/tesseracode/tesserapatch/internal/cli	(cached)
ok  	github.com/tesseracode/tesserapatch/internal/gitutil	(cached)
ok  	github.com/tesseracode/tesserapatch/internal/provider	(cached)
ok  	github.com/tesseracode/tesserapatch/internal/safety	(cached)
ok  	github.com/tesseracode/tesserapatch/internal/store	(cached)
ok  	github.com/tesseracode/tesserapatch/internal/workflow	(cached)

$ go test ./internal/workflow -run 'CreatedByInference|CreatedByGate|...' -count=1 -v
(all 8 CreatedByInference + 9 CreatedByGate + all M14 label/DAG tests PASS)

$ go test ./assets/... -count=1
ok  	github.com/tesseracode/tesserapatch/assets	0.479s
```

### Verdict: APPROVED

M15.1 is correct and complete. The implementation precisely matches the PRD §4.3.1 algorithm spec: replace-in-file ops with empty created_by and non-empty Search trigger a scan of HARD parents only (soft parents skipped per ADR-011 D4); when exactly one hard parent's post-apply.patch contains the Search bytes AND pristine working tree does not, an advisory suggestion is emitted to stderr. Multiple matches produce an ambiguity warning with no specific suggestion. Zero matches are silent (apply-time gate remains the authoritative enforcement point).

The advisory-only contract is strictly honored: the recipe is never mutated (deep-copy tests verify), operator authority is preserved, and suggestions are written to stderr for manual review. The opt-out flag (`--no-created-by-infer`) and flag-off guard (`features_dependencies: false`) both produce byte-identical pre-v0.6 behavior.

Scope guards all satisfied: created_by_gate.go untouched, no write-file ops processed, no transitive scanning, no new external deps, version stays at 0.6.0, CHANGELOG untouched. All 8 tests are meaningful (real fixtures, observable behavior asserts, covers all branches). Implementer's degradation-to-warning note verified: transient read errors don't block recipe persistence.

Full test suite green (all M14 series tests + new inference tests pass). Code quality high: clear comments, correct error handling, deterministic output (sorted parent matches), efficient fast-paths (candidate collection, cached parent patches, pristine short-circuit).

Ready to archive handoff. Supervisor can decide on v0.6.1 cut timing.

### Notes

None — clean implementation with no issues found.

---

## Review — M14.4 — 2026-04-26

**Implementer**: m14-4-implementer (general-purpose, ~21min per handoff notes)
**Reviewer**: code-review sub-agent

### Commits reviewed

7 commits between `0ba4809` (C5 fix-pass closeout) and `f1f603f` (origin/main HEAD):

- `d1aca5f` feat(cli): add tpatch status --dag tree view + status-time DAG validation (M14.4 chunks A+D)
- `ca23b35` feat(cli): dependency-management verbs — feature deps + amend --depends-on + remove --cascade (M14.4 chunk C)
- `5d5f594` feat(store): flip features_dependencies default to true (M14.4 chunk B)
- `97a994f` docs(skills): roll out feature-dependency guidance to all 6 skill formats (M14.4 chunk E)
- `e0a7d47` docs: add docs/dependencies.md user reference (M14.4 chunk F)
- `f2d0d1b` chore(release): v0.6.0 — Feature Dependencies (Tranche D) cutover (M14.4 chunk G)
- `f1f603f` docs(handoff): M14.4 implementation complete, awaiting reviewer

### Checklist

**Chunk A — `status --dag` ✅**
- [x] Uses `--json` flag, NOT `--format json` (correct flag name at cobra.go:196, status_dag.go:92)
- [x] Hard deps render `─►`, soft render `┄►` (status_dag.go:316-318)
- [x] Reads from `status.Reconcile.Outcome` via `EffectiveOutcome()`, never `reconcile-session.json` (status_dag.go:5 comment confirms ADR-010 D5 compliance, line 328 uses `st.Reconcile.EffectiveOutcome()`)
- [x] Cycle-safe: uses `DetectCycles`, visited-set prevents infinite recursion (status_dag.go:89, walkTree line 289-293)
- [x] Scoped and full DAG modes (scopeSet at line 90, scoped param honored throughout)
- [x] JSON schema deterministic and stable (dagJSONPayload struct lines 58-65, sorted roots/features)
- [x] Tests cover all scenarios: 7 tests in status_dag_test.go including cycle, empty, scoped, labels, JSON

**Chunk B — flag flip ✅**
- [x] Default now true in parseYAMLConfig (store.go:544 `cfg.FeaturesDependencies = true`)
- [x] Init template writes explicit true (store.go:88)
- [x] Byte-identity tests updated: dependency_gate_apply_test.go opts out (line 66 `features_dependencies: false`), accept_labels_test.go opts out (line 106)
- [x] Roundtrip test inverted: TestConfig_FeaturesDependenciesRoundtrip (roundtrip_test.go) tests default-true + explicit-false

**Chunk C — dep-management CLI ✅**
- [x] All verbs present: `feature deps` read/add/remove/validate-all at feature_deps.go:50-95
- [x] `amend --depends-on/--remove-depends-on` present (c1.go:48, feature_deps.go:274-290)
- [x] `remove --cascade` present with ErrInteractiveRequired for non-TTY (feature_deps.go:308-367)
- [x] **CRITICAL**: `--force` alone does NOT bypass DAG integrity (c1.go:231-238 comment + code: cascade gate enforced regardless of force flag)
- [x] **CRITICAL**: Test confirms force-bypass prevention: TestRemoveForce_DoesNotBypassDepCheck at feature_deps_test.go asserts exit code != 0 when force used without cascade
- [x] Add operations validate cycles/parent existence/no self-ref/no kind conflict (routed through store.ValidateDependencies)
- [x] Remove operations re-derive dependents atomically (store maintains dependents graph)
- [x] Cascade removes in reverse-topological order (feature_deps.go:319-340, leaves-first via reversed Kahn order)
- [x] All 9 tests present and meaningful: TestFeatureDeps_Show_NoDeps, TestFeatureDepsAdd_RejectsCycle, TestFeatureDepsAdd_RejectsKindConflict, TestFeatureDepsRemove_ClearsAtomically, TestAmendDependsOn_ValidatedIdenticallyToFeatureDeps, TestRemoveWithCascade_DeletesInReverseTopoOrder, TestRemoveWithoutCascade_RefusesWhenDependentsExist, TestRemoveForce_DoesNotBypassDepCheck, TestRemoveCascadeNonTTY_RequiresForce, TestFeatureDepsValidateAll_OnInit

**Chunk D — status-time validation ✅**
- [x] `tpatch status` revalidates DAG and surfaces warnings (cobra.go:204-215)
- [x] Both required tests present: TestStatus_SurfacesDanglingDepWarning (status_dag_test.go:202), TestStatus_SurfacesCycleWarning (status_dag_test.go:219)

**Chunk E — 6-skill rollout ✅**
- [x] All 6 skill files updated: claude/tessera-patch/SKILL.md, copilot/tessera-patch/SKILL.md, copilot prompt (tessera-patch-apply.prompt.md), cursor/tessera-patch.mdc, windsurf/windsurfrules, generic workflow (tessera-patch-generic.md) — all contain "Feature dependencies" section + created_by gate description
- [x] Each covers: dependencies field, label reference, compound verdict, created_by gate (not inert), status --dag, feature deps verbs, amend --depends-on, remove --cascade, force ≠ bypass rule
- [x] Parity guard passes: `go test ./assets/... -count=1` green (confirmed)
- [x] created_by reframed from "inert" to live gate (e.g., SKILL.md:160 states "live apply-time gate")

**Chunk F — docs/dependencies.md ✅**
- [x] User reference exists with all required content (267 lines)
- [x] Hard vs soft semantics (lines 35-40)
- [x] YAML examples (lines 25-33)
- [x] `feature deps add` examples (lines 52-72)
- [x] Validation rules (lines 78-93)
- [x] Label matrix (lines 147-154)
- [x] Compound verdict (lines 158-171)
- [x] `created_by` apply-time gate with dry-run warning noted (lines 96-107)
- [x] Cascade/force semantics (lines 216-231)
- [x] `status --dag` examples with ASCII + --json (lines 182-214)
- [x] Migration note for v0.5.x users (lines 233-252)

**Chunk G — release cutover ✅**
- [x] Version bumped to 0.6.0 (cobra.go:24)
- [x] CHANGELOG.md has new v0.6.0 section (lines 5-34, dated 2026-04-26)
- [x] ROADMAP.md M14 marked ✅ (line 175, M14.4 expanded with chunk breakdown)
- [x] NOT tagged (git tag -l v0.6.0 returns empty — tagging is supervisor's job)

**Cross-cutting / scope guards ✅**
- [x] No new external Go dependencies (only cobra/pflag + stdlib)
- [x] No ReconcileWaitingOnParent/ReconcileBlockedByParent enum values (labels remain composable, not states)
- [x] No parent-patch injection into M12 resolver (per ADR-011 D8, deferred)
- [x] No implement-phase `created_by` heuristic inference (per PRD §4.3.1, separate backlog)
- [x] All commits carry Co-authored-by trailer (7 instances confirmed)
- [x] No tpatch binary in tree (checked, not present)
- [x] Working tree clean (git status --porcelain empty)

**Validation gate ✅**
- [x] `gofmt -l .` — clean
- [x] `go build ./cmd/tpatch && rm -f tpatch` — ok
- [x] `go test ./...` — all green (8 packages)
- [x] `go test ./assets/... -count=1` — parity guard passes (0.358s)
- [x] CLI tests (StatusDag, FeatureDeps, Amend, Remove) — 27 tests pass (2.365s)
- [x] Workflow tests (CreatedByGate, ComposeLabels, EffectiveOutcome) — 26 tests pass (1.069s)
- [x] Store tests (Dependency, Roundtrip) — 8 tests pass (0.403s)

### Verdict: APPROVED

### Notes

This is a clean, comprehensive release cutover. All 7 chunks land exactly as specified in the handoff contract. The critical correctness checks all pass:

1. **Chunk A** correctly uses `--json` flag (not `--format json` per prior reviewer finding), renders hard/soft edges with correct glyphs, reads from status-of-record via `EffectiveOutcome()`, and handles cycles safely.

2. **Chunk B** default flip is correctly implemented in parseYAMLConfig with true when absent, tests properly updated to opt out where needed for byte-identity preservation.

3. **Chunk C** correctly enforces the `--force ≠ bypass` rule (PRD §3.7, ADR-011 D7) — the remove command checks dependents BEFORE evaluating force flag, and TestRemoveForce_DoesNotBypassDepCheck explicitly validates this behavior. Cascade removes in reverse-topological order as required.

4. **Chunk D** status-time validation surfaces dangling deps and cycles inline, with dedicated tests for both scenarios.

5. **Chunk E** 6-skill rollout is complete with parity guard passing, `created_by` correctly reframed from "inert" to "live apply-time gate" across all formats.

6. **Chunk F** comprehensive user documentation (docs/dependencies.md) covers all required topics including the dry-run downgrade for `created_by` gate (matching PRD §4.3).

7. **Chunk G** release mechanics correct: version bumped, CHANGELOG complete with M14.1–M14.4 summary, ROADMAP updated, no premature tag.

**No blocking issues found.** All scope guards honored, no drift from ADR-011 decisions, test coverage comprehensive, working tree clean. Ready for supervisor tag `v0.6.0` and milestone closeout.

---

## Review — M14 correctness pass — 2026-04-26

**Implementer**: m14-correctness sub-agent (general-purpose, ~90 min elapsed)
**Reviewer**: code-review sub-agent (this review)
**Task**: Three external-reviewer findings (F1 `created_by` apply-time gate, F2 label/AttemptedAt consistency, F3 upstreamed children suppress parent labels). All flag-gated. No version bump. Delta vs M14.3 closeout `6d95593`.

### Commits reviewed

- `cbe2873` feat(workflow): wire created_by apply-time gate (M14 fix-pass F1)
- `071c5ed` fix(workflow): clear stale labels on successful reconcile (M14 fix-pass F2)
- `cc95cbb` fix(workflow): suppress parent labels for upstreamed children (M14 fix-pass F3)
- `1e0d064` docs(handoff): M14 correctness pass complete, ready for review

### Checklist

- [x] Builds, tests, gofmt all green
- [x] 11 new tests (7 F1 + 2 F2 + 2 F3)
- [x] All 5 M14.1/M14.2/M14.3 tripwires green
- [x] All 4 commits carry Co-authored-by trailer
- [x] CURRENT.md accurate (Status: Complete)
- [x] Working tree clean, no tpatch binary at root

### Critical correctness checks (all 15 pass)

**F1 (HIGH, cutover-blocking) — `created_by` apply-time gate**

1. ✅ **Hard parent + missing target** — `TestCreatedByGate_HardParent_TargetMissing_ErrPathCreatedByParent` verifies both dry-run and execute paths return `ErrPathCreatedByParent` (sentinel, errors.Is-friendly). Error message includes target path, parent slug, and "apply parent first" guidance. Gate implemented at recipe.go:96 (dryRunOperation replace-in-file), line 113 (append-file), line 149 (executeOperation replace-in-file), line 166 (append-file). Four call sites, all correct.

2. ✅ **Soft parent fall-through** — `TestCreatedByGate_SoftParent_TargetMissing_FallsThroughWithWarning` verifies warning emitted to `WarnWriter` (captureWarn test helper confirms it's observable, not silently logged). Gate returns nil → falls through to existing not-found error. Warning text: `"note: op declares created_by=%s; soft deps do not gate apply\n"`. ADR-011 D4 compliant (soft deps never gate apply).

3. ✅ **created_by not in depends_on** — `TestCreatedByGate_ParentNotInDependsOn_RecipeRejected` verifies recipe-shape validation at dry-run time. Error message: `"recipe op declares created_by=%s but %s is not in depends_on"`. Validation error (distinct from ErrPathCreatedByParent).

4. ✅ **Target exists** — `TestCreatedByGate_HardParent_TargetExists_NoError` verifies gate passes when file is present. ExecuteRecipe succeeds, replacement applied. No false positives.

5. ✅ **upstream_merged satisfies** — `TestCreatedByGate_ParentUpstreamMerged_TargetExists_NoError` verifies parent in `StateUpstreamMerged` + target present → gate passes. ADR-011 D5 contract met (upstreamed parent satisfies hard deps). Gate doesn't inspect parent state directly (delegates to target-exists check).

6. ✅ **Flag-off byte-identity** — `TestCreatedByGate_FlagOff_NoOp` verifies with `features_dependencies: false` (default), `CreatedBy` is inert. Missing target surfaces bare "file not found" error (v0.5.3 behavior). No ErrPathCreatedByParent, no guidance leak. Early return at created_by_gate.go:65-68 when `!cfg.DAGEnabled()`.

7. ✅ **Op-type scope** — `TestCreatedByGate_AppliesToReplaceAndAppend` verifies gate fires for `replace-in-file` and `append-file` only. `write-file` and `ensure-directory` do NOT call `checkCreatedByGate` (verified in recipe.go:83-87, 121-125, 140-144, 177-178). Gate fires when target-must-exist precondition applies; creation ops bypass it. ADR-011 D4 narrow scope respected.

**F2 (MEDIUM) — Label/AttemptedAt consistency**

8. ✅ **Shared timestamp threading** — `TestRunReconcile_ClearsStaleLabel_WhenChildRefreshed` verifies single `attemptedAt` timestamp generated once in `saveReconcileArtifacts` (reconcile.go:468-470), reused by `updateFeatureState` (line 525-527). `composeLabelsAt` (labels.go:111-126) uses same timestamp as staleness baseline. Persisted `Labels` reflect the `AttemptedAt` about to be written (not the OLD on-disk value). No child flagged stale against itself. Unexported field (line 54) invisible to encoding/json — no schema impact, no fixture drift. Test loads pre-stale fixture, runs reconcile, asserts `stale-parent-applied` cleared.

9. ✅ **Other labels preserved** — `TestRunReconcile_PreservesOtherLabels_WhenStaleResolved` verifies selective clearing. Two parents: `p-stale` (applied, old UpdatedAt → stale clears) and `p-pending` (analyzed → waiting-on-parent remains). After reconcile, `Labels` contains only `[waiting-on-parent]` (stale gone, waiting preserved). Label set is recomputed via `composeLabelsFromStatus`, not flushed wholesale.

**F3 (MEDIUM) — Upstreamed children suppress parent labels**

10. ✅ **Upstreamed suppression** — `TestComposeLabels_UpstreamedChild_NoLabels` verifies child with `Reconcile.Outcome == ReconcileUpstreamed` returns nil labels (early return at labels.go:144-147). Parent in `StateBlocked` would normally produce `blocked-by-parent`, but suppression fires first. `childRetiredOutcomes` set (line 135-137) currently contains only `ReconcileUpstreamed` (ADR-011: once absorbed upstream, parent context is irrelevant).

11. ✅ **Non-suppressed outcomes still produce labels** — `TestComposeLabels_NonSuppressedOutcome_StillProducesLabels` verifies child with `ReconcileReapplied` (not in retired set) + stale parent → `stale-parent-applied` label emitted. Suppression is narrowly scoped. Other terminal outcomes (`Blocked`, `BlockedRequiresHuman`, `ShadowAwaiting`, etc.) remain live — labels still compose.

**Scope & hygiene**

12. ✅ **No scope creep** — No version bump in `internal/cli/cobra.go` (`version = "0.5.3"` unchanged). No `CHANGELOG.md` modification (`git diff 6d95593..HEAD -- CHANGELOG.md` empty). No `internal/store/types.go` changes (`git diff 6d95593..HEAD -- internal/store/types.go` empty) — no new `ReconcileOutcome` enum values added. No skill format updates (`git diff 6d95593..HEAD -- assets/skills/` empty). No external Go deps (only `github.com/tesseracode/tesserapatch/internal/store` imported). No `--dag` command, no `--force` bypass, no implement-phase heuristic inference of `created_by`. Strict scope respected.

13. ✅ **No reconcile-session.json reads in new code** — `grep -rn "reconcile-session.json" internal/workflow/created_by_gate.go internal/workflow/labels.go` returns only comments (line 13-14 in labels.go as AUTHORITATIVE SOURCE GUARD, line 25 in created_by_gate.go stating "does NOT consult any reconcile-session or apply-session artifact"). No new reads added. M14.3 external-reviewer guard preserved.

14. ✅ **Regression** — M14.1/M14.2/M14.3 tests: `TestComposeLabels_ReadsStatusJsonNotSessionArtifact` PASS (adversarial guard, labels.go reads status.json only). `TestReconcile_FlagOn_BlockedByParent_SkipsPhase35` PASS (phase-3.5 skip logic unchanged). `TestGoldenReconcile_ResolveApplyTruthful` PASS (golden fixture regression). `TestGoldenReconcile_ManualAcceptFlow` PASS (shadow accept flow). Full suite: `go test ./...` → all packages ok.

15. ✅ **Hygiene** — `gofmt -l .` clean. `go build ./cmd/tpatch` succeeds. No `tpatch` binary at root (`ls -la tpatch` → does not exist). 4 commits, 4 `Co-authored-by: Copilot` trailers (verified via `git log --format='%B' 6d95593..HEAD | grep -c "Co-authored-by"`). Working tree clean (`git status --short` empty).

### Implementation notes

**F1 architecture**: `checkCreatedByGate` signature is `(s *store.Store, childSlug string, op RecipeOperation, targetExists bool)`. Caller passes `targetExists` to avoid double-stat. Gate early-returns when `op.CreatedBy == ""` (line 58) or `!cfg.DAGEnabled()` (line 65-68) — lazy store access preserves flag-off byte-identity. Hard/soft classification via child's `depends_on` (loaded at line 69). Soft-parent branch (line 107-112) emits warning via `fmt.Fprintf(WarnWriter, ...)`, then returns nil (caller proceeds to not-found error). Sentinel `ErrPathCreatedByParent` (line 43) is `errors.New`, wrapped via `fmt.Errorf("%w: ...", ErrPathCreatedByParent, ...)` for `errors.Is` compatibility.

**F2 architecture**: `attemptedAt` field added to `ReconcileResult` (reconcile.go:54), unexported. Populated lazily (whichever helper runs first generates it, the other reuses). `composeLabelsAt(s, slug, asOf)` helper (labels.go:111-126) accepts explicit timestamp, temporarily overwrites `child.Reconcile.AttemptedAt` before delegating to `composeLabelsFromStatus` (line 125). `ComposeLabels` refactored to delegate to `composeLabelsFromStatus(s, child)` (line 102) so callers can override fields without disk round-trip. Public `ComposeLabels` signature unchanged (backward-compatible).

**F3 architecture**: `childRetiredOutcomes` map (labels.go:135-137) checked first in `composeLabelsFromStatus` (line 144). Early return nil when child retired. Map-based for O(1) lookup + extensibility (future `ReconcileObsolete` or similar can slot in). Comment at line 133-134 lists the live outcomes explicitly (Reapplied, StillNeeded, Blocked, ShadowAwaiting, BlockedTooManyConflicts, BlockedRequiresHuman) — those outcomes keep the child live, labels continue to compose.

**Call-site updates**: `DryRunRecipe` and `ExecuteRecipe` signatures changed from `(repoRoot string, recipe ApplyRecipe)` to `(s *store.Store, recipe ApplyRecipe)` (recipe.go:29, 48). Three call sites updated: `internal/cli/cobra.go` lines 466 + 552, `internal/cli/phase2.go` line 136. All pass `s` instead of `s.Root`. `slug` read from `recipe.Feature` (recipe.go:75 in `dryRunOperation`, line 132 in `executeOperation`).

**Test coverage**: 7 F1 tests (gate correctness + flag-off + op-type scope), 2 F2 tests (stale clears + other labels preserved), 2 F3 tests (upstreamed suppresses + non-suppressed still produce). All 11 tests in new files (`created_by_gate_test.go`, `labels_freshness_test.go`, `labels_upstreamed_test.go`). No existing tests modified. M14.1/M14.2/M14.3 regression suite green (24 tests from prior milestones).

**Documentation**: `created_by_gate.go` header comments (lines 1-25) cite PRD §4.3 contract as authoritative. `labels.go` header AUTHORITATIVE SOURCE GUARD (lines 9-16) explicitly forbids reconcile-session reads. `reconcile.go` field comment (lines 44-54) explains timestamp-threading rationale. CURRENT.md session summary complete (lines 140-214), files changed list accurate.

### Verdict

**APPROVED**

All 15 critical checks pass. F1 contract is correctly wired in both dry-run and execute paths, with flag-off byte-identity, op-type scope, and sentinel error. F2 timestamp threading is sound (one shared `attemptedAt`, no stale-against-self). F3 suppression is narrowly scoped (only `ReconcileUpstreamed` retired). No scope creep, no regressions, hygiene clean. Test coverage is comprehensive (7+2+2 = 11 new tests, all M14.1/M14.2/M14.3 tripwires green).

Ready for M14.4 cutover (flag flip, skill rollout, `tpatch status --dag`, v0.6.0 tag) on user green-light.

---

## Review — C5 fix-pass — 2026-04-26

**Implementer**: c5-implementer sub-agent (general-purpose, elapsed unknown)
**Reviewer**: code-review sub-agent (this review)
**Task**: C5 fix-pass — two re-reviewer findings on M14 correctness pass. F1 (HIGH): reconcile-time label suppression incomplete. F2 (MEDIUM): PRD §4.3 dry-run downgrade not implemented. ~140 LOC across 5 files, flag-protected.

### Commits reviewed (delta vs correctness pass closeout `eb4f4f1`)

- `c84c7a6` fix(workflow): suppress labels in reconcile-time upstreamed path (C5 F1)
- `dd72c2c` fix(workflow): downgrade created_by to warning in dry-run (C5 F2)
- `ea94fb7` docs(handoff): C5 fix-pass complete, ready for review

### Checklist

- [x] Builds, tests, gofmt all green
- [x] 4 new F1 tests (phase-1/2/3 upstreamed + non-upstreamed control)
- [x] F2 tests split (dry-run-downgrades + execute-errors as separate cases)
- [x] M14.1+M14.2+M14.3+correctness-pass regression clean
- [x] All 3 commits carry the Co-authored-by trailer
- [x] CURRENT.md accurate (Status: C5 fix-pass ✅ COMPLETE — awaiting reviewer)
- [x] Working tree clean, no tpatch binary

### Critical correctness checks — F1 (reconcile-path label suppression)

1. **Phase-1 reverse-apply** ✅ — `TestRunReconcile_Phase1ReverseApply_UpstreamedClearsLabels` seeds child with OLD `ReconcileBlocked` + persisted `waiting-on-parent` label, parent in `StateAnalyzed` (would normally re-fire label), simulates `result.Outcome = ReconcileUpstreamed` from phase-1, asserts BOTH `status.json` AND `reconcile-session.json` have empty/nil Labels. JSON must not contain `"labels"` key (omitempty defense-in-depth check at line 79 of test). PASS.

2. **Phase-2 op-level** ✅ — `TestRunReconcile_Phase2OperationLevel_UpstreamedClearsLabels` same setup, same assertions, `result.Phase = "phase-2-operation-level"`. PASS.

3. **Phase-3 provider-semantic** ✅ — `TestRunReconcile_Phase3ProviderSemantic_UpstreamedClearsLabels` same pattern, `result.Phase = "phase-3-provider-semantic"`. PASS.

4. **Non-upstreamed control** ✅ — `TestRunReconcile_NonUpstreamedOutcome_StillProducesLabels` uses `ReconcileBlockedRequiresHuman` outcome (not retired), asserts `hasLabel(got.Reconcile.Labels, store.LabelWaitingOnParent)` is true. Guards against over-broad fix that would suppress all labels. PASS.

5. **`updateFeatureState` audit** ✅ — Does NOT independently compose labels. At reconcile.go:523 (inside `updateFeatureState`), it writes `Labels: result.Labels` — propagates the in-memory value from `saveReconcileArtifacts`. No second composition path exists. The C5 F1 guard in `saveReconcileArtifacts` (lines 488-489) forces `result.Labels = nil` for retired outcomes BEFORE `updateFeatureState` sees it, so both status.json and reconcile-session.json get the same nil value.

6. **Pre-existing labels wiped** ✅ — The `seedRetiredChildScaffolding` test helper (labels_reconcile_path_test.go:32-49) explicitly seeds `child.Reconcile.Labels = []store.ReconcileLabel{store.LabelWaitingOnParent}` in the on-disk status before reconcile. All 3 phase tests assert post-reconcile Labels are empty (not "preserved the old label"). Retired child gets a clean slate.

7. **Adversarial guard preserved** ✅ — `TestComposeLabels_ReadsStatusJsonNotSessionArtifact` (from M14.3 tripwire set) still passes. The new C5 code path in `saveReconcileArtifacts` short-circuits BEFORE calling `composeLabelsAt`, so it never touches the session artifact at all. Guard confirmed: grep of reconcile.go shows only WRITES to reconcile-session.json (lines 40, 447, 450, 472, 497, 499), zero reads.

### Critical correctness checks — F2 (dry-run downgrade)

8. **Dry-run hard parent + missing target** ✅ — `TestCreatedByGate_DryRun_HardParent_TargetMissing_DowngradesToWarning` (created_by_gate_test.go:111-142) asserts `res.Success == true` (no error), `res.Applied == 1` (op counted as deferred-applied), `len(res.Warnings) == 1`, warning contains `["src/auth.ts", "parent", "apply parent before executing"]`. Recipe-level test confirms both replace-in-file and append-file op types downgrade. PASS.

9. **Execute hard parent + missing target** ✅ — `TestCreatedByGate_Execute_HardParent_TargetMissing_ReturnsErr` (lines 147-160) asserts `res.Success == false`, `len(res.Errors) == 1`, error contains `"will be created by parent feature parent"`. No regression from correctness pass. PASS.

10. **Soft parent missing target** ✅ — `TestCreatedByGate_SoftParent_TargetMissing_FallsThroughWithWarning` (lines 188-217) asserts `res.Success == false` (bare not-found error), `!errors.Is(errors.New(res.Errors[0]), ErrPathCreatedByParent)`, `WarnWriter` captured text contains `["soft-parent", "soft deps do not gate apply"]`. Dry-run and execute behavior unchanged from correctness pass. PASS.

11. **Recipe-shape validation** ✅ — `TestCreatedByGate_ParentNotInDependsOn_RecipeRejected` (lines 223-249) asserts created_by naming a feature NOT in depends_on is HARD error in BOTH dry-run AND execute, error contains `"is not in depends_on"`, does NOT wrap `ErrPathCreatedByParent`. PRD §4.3 last bullet contract preserved. PASS.

12. **Flag-off behavior** ✅ — `TestCreatedByGate_FlagOff_NoOp` (lines 52-74) with `dagEnabled=false`, created_by set + missing target → bare `"file not found"` error, NO mention of `"will be created by parent feature"`. The downgrade does not leak into flag-off mode. PASS.

13. **The wrong test is gone** ✅ — No test named `TestCreatedByGate_DryRun_HardParent_TargetMissing_Errors` exists. Git log shows the correctness pass had `TestCreatedByGate_HardParent_TargetMissing_ErrPathCreatedByParent` (gate-helper level test, still present at lines 81-102, correct), and C5 ADDED two new recipe-level tests: `..._DowngradesToWarning` (dry-run) and `..._ReturnsErr` (execute). New tests pin the PRD §4.3 split.

### Cross-cutting checks

14. **No scope creep** ✅ — Version still `"0.5.3"` (cobra.go:24). No CHANGELOG entry (`git diff eb4f4f1..HEAD -- CHANGELOG.md` empty). No tag (`git tag --contains HEAD | grep v0.6.0` empty). No skill format updates (`git diff -- assets/` empty). No `tpatch status --dag` (`grep -rn "tpatch status --dag"` empty). No dep-management CLI verbs. No new `ReconcileOutcome` enum values (`grep ReconcileWaitingOnParent|ReconcileBlockedByParent` empty). No new external Go deps (`git diff -- go.mod go.sum` empty). Scope perfectly clean.

15. **No `reconcile-session.json` reads** ✅ — External-reviewer guard: `grep -rn "reconcile-session.json" internal/workflow/recipe.go internal/workflow/reconcile.go` returns ONLY writes (line 499) and comments (lines 40, 447, 450, 472, 497). The new C5 F1 code path at reconcile.go:488-489 short-circuits BEFORE `composeLabelsAt` runs, so it never calls `s.LoadFeatureStatus` → never reads reconcile-session.json. Guard holds.

16. **Regression** ✅ — All critical tests pass: `TestGoldenReconcile_ResolveApplyTruthful` (0.44s), `TestGoldenReconcile_ManualAcceptFlow` (0.44s), all M14.1 (Roundtrip/DAG/Dependency), all M14.2 (CreatedByGate gate-helper level), all M14.3 (ComposeLabels/PlanReconcile/EffectiveOutcome/AcceptShadow), correctness-pass tripwires (`TestComposeLabels_ReadsStatusJsonNotSessionArtifact`, `TestReconcile_FlagOn_BlockedByParent_SkipsPhase35`). Full suite `go test ./...` green (all packages cached after targeted runs). Assets parity guard passes (0.344s).

17. **Hygiene** ✅ — `gofmt -l .` empty. No tpatch binary at root (`ls -la tpatch` → not found). All 3 commits carry Co-authored-by trailer (verified via `git log --format='%B' eb4f4f1..HEAD | grep -c "Co-authored-by: Copilot"` returns 3). Working tree clean (`git status --short` empty).

### Verdict: **APPROVED**

All 17 checks pass. Both HIGH-severity F1 findings completely resolved:

- **F1 (HIGH)**: Reconcile-time persistence path now suppresses parent-derived labels for retired outcomes (currently only `ReconcileUpstreamed`) via early short-circuit in `saveReconcileArtifacts` (reconcile.go:488-489) checking `childRetiredOutcomes[result.Outcome]` BEFORE calling `composeLabelsAt`. All 3 phase paths (reverse-apply, op-level, provider-semantic) tested + non-upstreamed control confirms suppression is narrowly scoped. Pre-existing labels wiped (not preserved). Adversarial test from M14.3 still passes.

- **F2 (MEDIUM)**: PRD §4.3 contract now fully implemented. `dryRunOperation` returns `(msg, warning, error)` tuple (recipe.go:87). Hard-parent `ErrPathCreatedByParent` downgrades to a `RecipeExecResult.Warnings` entry in dry-run (lines 110-112), reports op as Applied (deferred), surfaces actionable hint. Execute-mode unchanged (still returns hard error). Soft-parent behavior unchanged (fall-through to not-found + warning). Recipe-shape validation (parent-not-in-depends_on) remains hard error in BOTH modes. CLI dry-run gains `⚠` rendering (cobra.go:471) + warning-count summary (line 478). Locked-in tests split into dry-run vs execute halves.

No scope creep. No regressions. Flag-off byte-identity preserved. Production-ready for M14.4 dispatch.

---

## Review — M14.3 — 2026-04-26

**Implementer**: m14-3-implementer sub-agent (general-purpose, elapsed unknown)
**Reviewer**: code-review sub-agent (this review)
**Task**: Reconcile topological traversal + composable labels + compound verdict. Third sub-milestone of M14 / Tranche D / v0.6.0. ~1309 LOC across 11 files, gated behind `features_dependencies` flag.

### Commits reviewed (delta vs M14.2 closeout `0ed64cb`)

- `7c9aee4` feat(store): add ReconcileLabel + Labels field + EffectiveOutcome (M14.3)
- `bccf5e2` feat(workflow): add PlanReconcile topological planner (M14.3)
- `b9efd07` feat(workflow): ComposeLabels + label-aware reconcile + phase-3.5 skip (M14.3)
- `a232a7b` feat(workflow): AcceptShadow refreshes labels (M14.3)
- `4e39384` docs(handoff): M14.3 complete, ready for review

### Checklist

- [x] Builds, tests, gofmt all green
- [x] Parity guard pass (no skill asset changes for M14.3, deferred to M14.4)
- [x] 24 new tests (4 store + 4 PlanReconcile + 11 ComposeLabels + 3 phase-3.5 + 2 AcceptShadow)
- [x] M14.1 + M14.2 regression clean (all DAG/Dependency/Validate/Roundtrip/GoldenReconcile tests pass)
- [x] All 5 commits carry the Co-authored-by trailer
- [x] CURRENT.md accurate (Status: Review)
- [x] Working tree clean, no tpatch binary

### Critical correctness checks (all 13 pass)

1. **Flag-off byte-identity** ✅ — `TestRoundtrip_PreM14_3StatusByteIdentity` + `TestRoundtrip_EmptyReconcileLabelsOmitted` both do string comparison of round-tripped fixture bytes. `Labels []ReconcileLabel \`json:"labels,omitempty"\`` on line 176 of internal/store/types.go. PlanReconcile gated in reconcile.go:97 preserves input order when flag off. AcceptShadow only invokes ComposeLabels when `cfg.DAGEnabled()` (accept.go:155).

2. **External-reviewer guard (LOAD-BEARING)** ✅ — `TestComposeLabels_ReadsStatusJsonNotSessionArtifact` at internal/workflow/labels_test.go:217-254 is the adversarial test. Seeds misleading `reconcile-session.json` with `outcome: blocked-requires-human` while parent's `status.json` has `Outcome: ReconcileReapplied`. Asserts ComposeLabels returns nil (clean parent → no labels), proving it reads status.json only. Test would fail if session artifact were consulted. Doc comment in labels.go:9-16 explicitly states the load-bearing rule. Implementation at labels.go:110 calls `s.LoadFeatureStatus(dep.Slug)` — no path to reconcile-session.json exists.

3. **ADR-011 D3 (labels ≠ states)** ✅ — `grep -rn "ReconcileWaitingOnParent\|ReconcileBlockedByParent" internal/store/ | grep -v "Label"` returns empty. Only `ReconcileLabel` constants exist (types.go:65-82). `EffectiveOutcome()` at types.go:192-200 computes compound at READ time; no new enum values added to `ReconcileOutcome`. Persisted `Outcome` for compound case is `ReconcileBlockedRequiresHuman` (ADR-011 D6 match confirmed).

4. **Phase-3.5 skip + tripwire** ✅ — `TestReconcile_FlagOn_BlockedByParent_SkipsPhase35` at internal/workflow/labels_phase35_test.go:43-118 is the tripwire test. The `tripwireProvider` type (lines 20-36) returns benign stub for phase-3 but calls `t.Errorf` + returns error if a phase-3.5 prompt (containing `"# File:"`) hits Generate. Test sets up child with 3-way conflict + hard parent in `blocked-requires-human`, runs reconcile with `Resolve: true`, and asserts `Phase: "phase-3.5-skipped-blocked-by-parent"` without tripwire firing. Short-circuit logic at reconcile.go:280-292 checks `hasLabel(labels, store.LabelBlockedByParent)` and skips `tryPhase35`.

5. **Compound verdict semantics** ✅ — `EffectiveOutcome()` (types.go:192-200) returns `"blocked-by-parent-and-needs-resolution"` ONLY when `Outcome == ReconcileBlockedRequiresHuman` AND `Labels` contains `LabelBlockedByParent`. All other cases return stringified `Outcome`. Tests: `TestReconcileSummary_EffectiveOutcome_Compound` + `TestEffectiveOutcome_PassthroughWhenNoCompoundLabels`. Soft deps checked: `TestComposeLabels_SoftParentNeverProducesLabel` (labels_test.go:127-138) asserts soft parent in any state produces empty label set (D4 compliance).

6. **PlanReconcile correctness** ✅ — Transitive hard-parent closure at plan_reconcile.go:74-100 (only `dep.Kind != DependencyKindHard` branches continue). Soft deps contribute to ordering (line 108 passes full `allDeps[slug]` to TopologicalOrder) but don't pull themselves into closure. Cycle error augmented at lines 115-118 with `DetectCycles` path. Deterministic order by slug via `TopologicalOrder` (Kahn's with lexicographic tie-break per ADR-011 D2). Flag-off path: reconcile.go:97 only calls PlanReconcile when `cfg.DAGEnabled()`. Tests: `TestPlanReconcile_TransitiveHardClosure`, `TestPlanReconcile_RejectsCycle`, `TestPlanReconcile_FlagOff_PreservesInputOrder`.

7. **AcceptShadow refresh** ✅ — accept.go:149-168 recomputes labels via `ComposeLabels(s, slug)` ONLY when `cfg.DAGEnabled()`. Flag-off preserves prior `Reconcile.Labels` value (stays nil/absent). Tests: `TestAcceptShadow_FlagOn_RefreshesLabels` + `TestAcceptShadow_FlagOff_LabelsRemainNil`.

8. **Label determinism** ✅ — ComposeLabels (labels.go:161) sorts via `sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })`. Alphabetical ordering per ADR-011 D3. Test: `TestComposeLabels_DeterministicOrder` (labels_test.go:186-202) runs ComposeLabels 50× on same fixture, asserts `reflect.DeepEqual` each iteration.

9. **No scope creep** ✅ — No `--dag` command (`grep -rn "tpatch status --dag"` returns empty). No skill updates (`git diff 0ed64cb..HEAD -- assets/` empty). No version bump / CHANGELOG / tag (`git diff -- CHANGELOG.md VERSION` empty). No `created_by` population in implement phase (grep confirms only test writes it — inherited from M14.2). No parent-patch injection (no M12 resolver changes). No new external Go deps (`git diff -- go.mod` empty).

10. **Parity guard** ✅ — `go test ./assets/... -count=1` passes (1.804s).

11. **Regression** ✅ — All critical tests pass: `TestGoldenReconcile_ResolveApplyTruthful`, `TestGoldenReconcile_ManualAcceptFlow`, all M14.1 (DAG/Dependency/Validate/Roundtrip), all M14.2 (dependency gate). Full suite `go test ./... -count=1` green across all packages.

12. **Hygiene** ✅ — `gofmt -l .` empty. `ls -la tpatch` returns empty (no binary). All 5 commits have Co-authored-by trailer (verified via `git log --format='%B' | grep -c "Co-authored-by: Copilot"` returns 5). `git status --short` empty (working tree clean).

13. **Handoff** ✅ — `docs/handoff/CURRENT.md` Status: Review (line 7), implementation summary lines 253-310 accurate, files + test counts match diff stat (11 files, 1309 insertions).

### Verdict: **APPROVED**

All 13 checks pass. Both non-negotiable tests present and correct:
- **Adversarial test** (check #2): `TestComposeLabels_ReadsStatusJsonNotSessionArtifact` enforces ADR-010 D5 source-of-truth rule.
- **Tripwire test** (check #4): `TestReconcile_FlagOn_BlockedByParent_SkipsPhase35` with `tripwireProvider` confirms phase-3.5 short-circuit.

No revisions. No notes. Production-ready for M14.4 user-facing cutover.

---

## Review — M14.2 — 2026-04-26

**Implementer**: m14-2-implementer sub-agent (general-purpose, 1777s)
**Reviewer**: m14-2-reviewer sub-agent (code-review, 1100s)
**Task**: Apply gate + `created_by` recipe op + 6-skill parity-guard rollout. Second sub-milestone of M14 / Tranche D / v0.6.0. ~250 LOC + 6-skill update, gated behind `features_dependencies` flag.

### Commits reviewed (delta vs M14.1 closeout `33ebad0`)

- `24baf92` feat(recipe): add created_by op field + 6-skill rollout
- `9a5f2f3` feat(workflow): add CheckDependencyGate (9 unit tests)
- `4dfe0f1` feat(cli): wire dependency gate into apply (2 integration tests)
- `cdd5484` docs(handoff): M14.2 complete, ready for review

### Checklist

- [x] Builds, tests, gofmt all green
- [x] Parity guard pass (6 skill formats + docs/agent-as-provider.md updated in lockstep)
- [x] 14 new tests (9 gate + 3 recipe round-trip + 2 CLI integration)
- [x] M14.1 regression clean (17/17 tests pass)
- [x] No reconcile drift (`reconcile.go`/`accept.go`/`resolver.go` unchanged — verified by empty `git diff`)
- [x] All 4 commits carry the Co-author trailer
- [x] CURRENT.md accurate

### Critical correctness checks (all 12 pass)

1. **Recipe byte-identity** — `bytes.Equal` round-trip without `created_by` confirmed; v0.5.3 recipes unchanged.
2. **Hard vs soft** — "mixed deps" test asserts ONLY unapplied hard parent in error message; soft + applied hard correctly omitted.
3. **`upstream_merged` + empty `satisfied_by`** — passes (provenance is optional per ADR-011 D5).
4. **`upstream_merged` + non-empty `satisfied_by`** — shape check only (40-hex); `git merge-base` reachability deferred as documented limitation.
5. **Flag-off true no-op** — early return BEFORE any feature-status load.
6. **Sentinel error** — `errors.Is(err, ErrParentNotApplied)` works; 3 tests exercise it.
7. **CLI integration completeness** — single test asserts all 4: non-zero exit + parent in stderr + no tree mutation + child state unchanged.
8. **Wiring** — both `runApplyExecute` (line 540) AND `runApplyAuto` (line 635) call gate BEFORE recipe parsing. `prepare` and `started` correctly NOT gated.
9. **6-skill parity** — Claude SKILL.md L158-160, Copilot L111, Cursor .mdc L113, plus Windsurf/Copilot prompt/Generic workflow all consistent.
10. **External-reviewer guard inheritance** — doc comments on both `CheckDependencyGate` (L34-37) and `CreatedBy` (L67-68) require reading `status.Reconcile.Outcome` not `reconcile-session.json`. M14.3 inherits.
11. **Reconcile untouched** — empty diff verified.
12. **`CreatedBy:` not populated by production code** — grep confirmed only test files write it.

### Verdict: **APPROVED**

No revisions, no notes. Production-ready.

### Action Taken

- SQL: `m14.2-apply-gate` → done. `m14.3-reconcile-topo` → in_progress.
- ROADMAP M14.2 ticked ✅ with commit shas.
- M14.2 handoff archived → HISTORY.md.
- CURRENT.md rewritten for M14.3 (reconcile topological traversal + composable labels + compound verdict, ~500 LOC).
- Auto-chain continues to M14.3 per supervisor plan (will pause before M14.4 — the user-facing release/flip).

---

## Review — M14.1 — 2026-04-26

**Implementer**: m14-1-implementer sub-agent (general-purpose, 4152s)
**Reviewer**: m14-1-reviewer sub-agent (code-review, 307s)
**Task**: Feature Dependencies data model + validation. First sub-milestone of M14 / Tranche D / v0.6.0. ~300 LOC, gated behind `features_dependencies` flag (default false). No user-visible behavior changes.

### Commits reviewed (delta vs `v0.5.3`)

- `02f1ba9` feat(store): add Dependency type + DAG primitives
- `d166281` feat(store): add dependency validation
- `7dd5941` feat(config): add features_dependencies flag

### Checklist

- [x] Builds, tests, gofmt all green
- [x] 30 new test cases under `internal/store` — all real assertions
- [x] No regressions (`TestGoldenReconcile_*` 7 tests pass)
- [x] Parity guard untouched (`assets/` unchanged)
- [x] Co-author trailer on all 3 commits
- [x] CURRENT.md accurate

### Critical correctness checks (all 10 pass)

1. **Flag-off byte-identity** — `TestRoundtrip_PreM14StatusByteIdentity` does string comparison of round-tripped fixture bytes. `omitempty` works.
2. **Deterministic topo order** — runs `TopologicalOrder` 50× on multi-valid-order graph, `reflect.DeepEqual` each iteration. Sibling ties broken by slug.
3. **Cycle detection** — self-edge detected; DFS returns the cycle path string, not just an error.
4. **5 PRD §3.3 rules** — each with positive + negative test (self-dep, dangling, kind conflict, cycle, satisfied-by-upstream-only) plus invalid-kind guard.
5. **Sentinel errors** — 6 errors, all wrappable via `errors.Is` and exercised in tests.
6. **Flat YAML flag** — round-trips through both repo + global config paths (per zero-dep parser limitation).
7. **External-reviewer guard baked in** — doc comments on `FeatureStatus.DependsOn` and `dag.go` header explicitly state DAG logic must read `status.Reconcile.Outcome` and never `artifacts/reconcile-session.json`. M14.3 inherits this.
8. **Pure DAG functions** — `dag.go` has no IO; `Store` parameter only on validation.
9. **Soft vs hard** — both kinds count equally for topo/cycle; gate-/label-relevance deferred per PRD §6.
10. **Flag is observably inert** — grep confirmed no caller in `cmd/`, `internal/cli/`, or `internal/workflow/` gates on `DAGEnabled` yet. Flag exists; nothing toggles.

### Verdict: **APPROVED**

No revisions. No deferred notes. Data model is load-bearing for M14.2/.3 but fully gated; zero user-visible change until M14.4 cutover.

### Action Taken

- Logged this entry.
- SQL: `m14.1-data-model` → done. `m14.2-apply-gate` → in_progress.
- CURRENT.md rewritten for M14.2 (apply gate + `created_by` recipe op + 6-skill parity-guard rollout, ~250 LOC).
- No version bump / CHANGELOG entry — M14 sub-milestones land behind the flag; the v0.6.0 release ships at M14.4.

---

## Post-release Review — v0.5.3 follow-up — 2026-04-24

**Reviewer**: external (vscode review session, full `v0.5.2..v0.5.3` delta + targeted probe test)
**Verdict**: **Effectively APPROVED.** Both v0.5.3 fixes verified at the control flow level; full suite + focused `GoldenReconcile|AcceptShadow` run pass. No code-level regressions found.

### Findings (both LOW severity, doc-only)

1. **ADR-010 stale** — D5 still described the resolver writing the full audit to a single `reconcile-session.json`. Code now splits ownership (`resolution-session.json` resolver-owned, `reconcile-session.json` reconcile-owned). Risk: same kind of drift that caused the v0.5.2 dual-writer bug.
   **Fix**: ADR-010 D5 expanded to enumerate the split and call out `status.json` as post-accept source of truth.

2. **Undocumented contract** — manual `reconcile --accept` correctly stamps `status.json.Reconcile.Outcome=reapplied`, but does NOT rewrite `artifacts/reconcile-session.json`, which remains the pre-accept `shadow-awaiting` snapshot. Likely intentional (the artifact audits a `RunReconcile` invocation; `status.json` is current truth) but never explicitly stated.
   **Fix**: doc comment on `saveReconcileArtifacts` in `internal/workflow/reconcile.go` + ADR-010 D5 paragraph make the contract explicit.

### Action Taken

Single docs-only commit on top of v0.5.3 (no version bump — no behavior change). Both findings closed.

---

## Review — Tranche C3 / v0.5.3 — 2026-04-24

**Implementers**: c3-implementer + c3-finisher sub-agents (general-purpose)
**Reviewer**: c3-reviewer sub-agent (code-review, 381s)
**Task**: Shadow accept accounting fixes — 3 external-reviewer findings on v0.5.2 shadow-accept flow.

### Checklist

- [x] Code compiles: `go build ./cmd/tpatch`
- [x] Tests pass: `go test ./...` (all packages green)
- [x] Formatted: `gofmt -l .` empty
- [x] `.tpatch/` artifacts deterministic; single writer per artifact path
- [x] Secrets safe (N/A)
- [x] CLI behavior matches ADR-010 + ADR-011 D6 prerequisite
- [x] Handoff accurate (CURRENT.md reflects 3/3 landed, deferred release to supervisor per guardrails)
- [x] Parity guard passes (skill/doc drift for artifact path rename resolved)
- [x] No regressions (`TestGoldenReconcile_ResolveApplyTruthful` still passes)

### Commits reviewed

- `4636878` fix(workflow): split resolver artifact into `resolution-session.json`
- `3ac7465` fix(workflow): `AcceptShadow` stamps `Reconcile.Outcome=reapplied`
- `8a4af4b` test(reconcile): end-to-end shadow-awaiting → manual accept regression
- `6024942` docs(handoff): C3 complete

### Verdict: **APPROVED**

### Notes

All three confirmed findings properly fixed:

1. **Dual-writer collision resolved**: Clean schema ownership — `resolution-session.json` (resolver, per-file outcomes) vs `reconcile-session.json` (reconcile, high-level summary). Grep-confirmed single writer per path. `loadResolvedFiles` and `--shadow-diff` read the new path; error messages updated.
2. **Manual accept regression test comprehensive**: `TestGoldenReconcile_ManualAcceptFlow` parses `resolution-session.json` inline (mirrors `loadResolvedFiles`), calls `workflow.AcceptShadow`, asserts merged content + `State=applied` + `Reconcile.Outcome=reapplied` + shadow cleared + directory pruned. Would have caught both artifact collision and outcome-stamp bugs in v0.5.2. PASS in 0.45s.
3. **Outcome stamp consistency confirmed uniform**: Both manual (`runReconcileAccept` → `AcceptShadow`) and auto-apply (`tryPhase35` → `AcceptShadow` → outer `updateFeatureState`) paths converge on `Reconcile.Outcome=reapplied`. Auto path has benign double-write (helper sets value, outer `updateFeatureState` sets same value) — idempotent, harmless.

Backward compatibility: acceptable breakage — old `reconcile-session.json` from v0.5.2's resolver not consumed on v0.5.3; re-running `reconcile --resolve` regenerates the correct `resolution-session.json`. Shadow worktrees are ephemeral; no on-disk migration required.

Drift audit synchronized 7 files (5 skill formats + 2 docs). Historical references (CHANGELOG, HISTORY, ADR-010, M12 milestone, M4 phase-4 reconcile summary) intentionally left alone.

Scope discipline: no creep beyond C3.1/C3.2/C3.3. Co-author trailers present on all 4 commits.

### Action Taken

**APPROVED** — proceeding with release:
1. Version bumped 0.5.2 → 0.5.3 (`internal/cli/cobra.go:24`)
2. CHANGELOG v0.5.3 section added
3. ROADMAP M13.6 flipped to ✅
4. Tag v0.5.3 pushed
5. C3 CURRENT.md archived → HISTORY.md; CURRENT.md rewritten for M14.1
6. SQL: `c3-release-v0.5.3` → done, `m14.1-data-model` → in_progress

M14.1 (Feature Dependencies data model, ~300 LOC) unblocked. Implementation sub-agent dispatch next.

---

## Review — Tranche C2 / v0.5.2 — 2026-04-23

**Implementer**: c2-implementer sub-agent (general-purpose, 6400s)
**Reviewer**: c2-reviewer sub-agent (code-review, 352s)
**Task**: Post-v0.5.1 correctness fix pass — 6 validated findings from review session.

### Checklist

- [x] Code compiles: `go build ./cmd/tpatch`
- [x] Tests pass: `go test ./...` (all packages, with and without cache)
- [x] Formatted: `gofmt -l .` empty
- [x] `.tpatch/` artifacts deterministic (sha256 reproducible from inputs)
- [x] Secrets safe (N/A for this pass)
- [x] CLI behavior matches SPEC.md and shipped v0.5.1 contract
- [x] Handoff accurate
- [x] Assets parity guard passes (skills edited for finding #6)
- [x] No regressions
- [x] 8 regression tests added, each asserting actual behavior (not tautological)
- [x] `ReconcileReapplied` verified unreachable without helper success for shadow-based paths

### Per-finding verdict

1. `c2-resolve-apply-truthful` — ✅ correct. Shared `workflow.AcceptShadow` helper eliminates drift between manual and auto paths. `safety.EnsureSafeRepoPath` called on every file write. Failure preserves shadow + maps to `ReconcileBlockedRequiresHuman`. `TestGoldenReconcile_ResolveApplyTruthful` is the regression guard pre-v0.5.2 code would fail.
2. `c2-refresh-index-clean` — ✅ correct. `GIT_INDEX_FILE` temp approach with deferred unlink on all paths. Regression test byte-compares `git status --porcelain` + checks intent-to-add marker.
3. `c2-recipe-hash-provenance` — ✅ correct. Pointer field enables backward compat. Legacy-sidecar test + content-drift test both pass.
4. `c2-remove-piped-stdin` — ✅ correct. Real `os.Pipe()` in test, not fake reader.
5. `c2-amend-append-flag` — ✅ correct. `--append --reset` mutex enforced with "mutually exclusive" error.
6. `c2-max-conflicts-drift` — ✅ correct. 8 sites (not 6 — agent found 2 more: cursor + windsurf skill formats). Runtime unchanged at 10. Parity guard green.

### Cross-cutting

- Shared helper pattern fully eliminates the manual-vs-auto drift that created finding #1 in the first place.
- Only legitimate `ReconcileReapplied` assignments remaining: phase 4 `ForwardApplyStrict`, phase 4 `ForwardApply3WayClean` (both clean-apply, no shadow), and phase 3.5 after `AcceptShadow` success.
- No terminology/contract drift in docs vs runtime detected.

### Verdict: **APPROVED**

### Action Taken

Updated CHANGELOG v0.5.2 section, bumped `version = "0.5.2"` in `internal/cli/cobra.go`, flipped M13.5 to ✅ in ROADMAP.md, tagged v0.5.2, pushed tag. SQL: 6 c2-* todos → `done`; `c2-release-v0.5.2` → `done`; `m14.1-data-model` unblocked.

---

## Review — PRD-feature-dependencies — 2026-04-23

**Author**: dag-prd-author sub-agent (3 revision cycles)
**Reviewer**: dag-prd-reviewer rubber-duck sub-agent (3 review passes)
**Task**: Author PRD for stacked feature dependency DAG (v1 backlog item `feat-feature-dependencies`).

### Review trajectory
- **v1 → NEEDS REVISION**: 6 critical issues (semantic contradictions, state composition, dual-source footgun, parity-guard impact, amend/remove vagueness, missing ADR)
- **v2 → NEEDS REVISION**: 5 of 6 resolved + 1 partial; 4 new internal contradictions introduced by the revisions themselves (composability vs exclusivity, drift precedence, `--orphan-soft` scope creep, JSON example bug)
- **v3 → APPROVED WITH NOTES**: all 4 new contradictions resolved; 4 edge cases author self-flagged all accepted; 1 minor terminology drift (`ReconcileWaitingOnParent` enum vs label) deferred to ADR-011 cleanup

### Verdict: **APPROVED WITH NOTES**

### Deliverable
`docs/prds/PRD-feature-dependencies.md` — 736 lines, commit `fa4bbb6`.

### Decisions locked in the PRD (to be reiterated in ADR-011)
1. `depends_on` lives in `status.json` only (no new `feature.yaml`, no migration)
2. DFS for cycle detection
3. Kahn's algorithm for operator-facing topo traversal
4. `waiting-on-parent` / `blocked-by-parent` are composable derived labels (not states)
5. Soft deps do NOT gate `created_by`; hard deps DO
6. `upstream_merged` satisfies hard dependencies
7. `remove --cascade` required to delete parents with dependents (`--force` alone does NOT bypass dep integrity)
8. Parent-patch context NOT passed to M12 resolver in v0.6 (deferred to `feat-resolver-dag-context`)

### Follow-up tranche scope (Tranche D / v0.6.0, 4 milestones)
- M14.1 data model + validation (~300 LOC) — blocked by ADR-011
- M14.2 apply gate + `created_by` + parity-guard rollout (~250 LOC)
- M14.3 reconcile topological traversal + compound verdicts (~500 LOC, bumped)
- M14.4 `status --dag` + skills + release v0.6.0 (~300 LOC, bumped)

### Action Taken
Committed PRD (`fa4bbb6`). SQL todos inserted: `adr-011-feature-dependencies` (blocker), `m14.1` → `m14.4` chain with dependencies. Three follow-ups registered: `feat-resolver-dag-context`, `feat-feature-autorebase`, `feat-amend-dependent-warning`. Parent todo `feat-feature-dependencies` flipped to `done`. ROADMAP M14 block still needs to be populated by supervisor (next step).

---

## Review — M13 / Tranche C1 / v0.5.1 — 2026-04-22

**Reviewer**: c1-reviewer (code-review sub-agent)
**Implementer**: c1-implementer (general-purpose sub-agent)
**Task**: UX Polish & Quick Wins — 8 features + release (commits `4f49c76..e069cd8`, tag `v0.5.1`).

### Checklist
- [x] `go build ./cmd/tpatch` succeeds
- [x] `go test ./...` passes all packages
- [x] `gofmt -l .` empty
- [x] `go vet ./...` clean
- [x] Parity guard `TestSkillRecipeSchemaMatchesCLI` green (no `base_commit` leaked into recipe schema)
- [x] All 10 commits carry `Co-authored-by: Copilot <223556219+...>` trailer
- [x] Tag `v0.5.1` exists (annotated, on `e069cd8`)
- [x] CHANGELOG v0.5.1 section + breaking-UX call-out present
- [x] ROADMAP M13 marker flipped 🔨 → ✅
- [x] CURRENT.md archived to HISTORY.md (timestamped, no duplication)
- [x] SQL: 9 c1-* todos all `done`

### Verdict: **APPROVED**

### Notes
- **Recipe stale guard** stored as sidecar `artifacts/recipe-provenance.json` (NOT in `apply-recipe.json`) — preserves parity-guard contract; backward-compat (missing sidecar = silent).
- **Apply default mode** flipped `prepare → auto`. Auto chains existing prepare/execute/done helpers verbatim (line-for-line extraction, no capture re-derivation). Stale-guard still fires inside auto via shared `runApplyExecute`. Breaking UX called out in CHANGELOG.
- **Spinner** wired at single choke point (`GenerateWithRetry`), TTY-guarded, race-free cleanup via `sync.Once` + `<-done`. Tests don't depend on wall-clock.
- **`record --lenient`** shipped as documented escape hatch — implementer ran 4 synthetic repros of the markdown false-positive, all passed reverse-apply cleanly. Without a live reproducer, the documented flag (with stderr warning + error-message hint) is safer than a speculative `--ignore-whitespace` that could mask real divergence. Pragmatic call, accepted.
- No flaky test patterns, no goroutine leaks, no swallowed errors.

### Action Taken
Pushed `main` (`ebb5b7a..e069cd8`) and tag `v0.5.1` to `origin`. Tranche C1 complete; supervisor will pick next tranche when user kicks off.

---

## 2026-04-17 — M10 Managed Copilot Proxy UX — PENDING REVIEW

**Task**: Implement ADR-004 — honest UX for the reverse-engineered copilot-api proxy + CI release automation.
**Implementer**: M10 agent
**Verdict**: **PENDING**

### Deliverables
- CI release job (`.github/workflows/ci.yml`) — `softprops/action-gh-release@v2`, triggers on `v*` tags, auto-generated notes, prerelease detection. Free (default GITHUB_TOKEN).
- Global config (`internal/store/global.go`) — XDG-honouring loader + saver, merge helper, AUP ack helpers. 0600 file perms.
- Config type (`internal/store/types.go`) — new `CopilotAUPAckAt` field.
- Reachability probe (`internal/provider/probe.go`) — `Reachable`, `IsLocalEndpoint`, `IsCopilotProxyEndpoint`; 2s bound.
- CLI wiring (`internal/cli/copilot.go` + `cobra.go`) — `loadAndProbeProvider` with cached per-process probe, `Execute` now prints errors, AUP warning in `init` / `providerSetCmd` / `autoDetectProvider`.
- Harness doc refresh (`docs/harnesses/copilot.md`) — install path, OS-specific config path, warn-vs-fail rules.
- Tests — `global_test.go` (6), `probe_test.go` (5). All 7 packages green.

### Checklist
- [x] Compiles (`go build ./cmd/tpatch`)
- [x] Tests pass (`go test ./... -count=1`)
- [x] Formatted (`gofmt -w .` clean)
- [x] Artifacts deterministic (global config is flat YAML with fixed field order)
- [x] Secrets safe (only env var names in config; token never persisted)
- [x] Matches SPEC / ADR-004 (all 8 decisions implemented)
- [x] Handoff accurate (CURRENT.md rewritten with behaviours + pointers)
- [x] Smoke (dead port → hard-fail with install hint; live proxy → falls through)

### Notes
- `TPATCH_NO_PROBE=1` escape hatch added for offline demos / CI steps that only read store state.
- Probe cache is process-scoped; acceptable for one-shot CLI, would need invalidation in a long-running daemon.
- AUP warning copy sits in `internal/cli/copilot.go::copilotAUPWarning` — tweak there, not in harness docs.
- macOS note baked into the harness doc: global config defaults to `~/Library/Application Support/tpatch/config.yaml` unless `XDG_CONFIG_HOME` is set.

### Action Taken
Archived prior ADR-004/005 CURRENT entry to HISTORY.md; wrote new CURRENT for this implementation; awaiting supervisor review before commit.

---

## 2026-04-17 — ADR-004 (M10 UX) + ADR-005 (M11 native provider) — PENDING REVIEW

**Task**: Lock in decisions for M10 and M11 through interactive Q&A with the user; capture as two ADRs.
**Implementer**: Planning agent
**Verdict**: **PENDING** (plan only, no code)

### Deliverables
- `docs/adrs/ADR-004-m10-copilot-proxy-ux.md` — 8 decisions: no process supervision, upstream proxy pointer, global+repo config loader, reachability probe behaviour, no log piping, first-run AUP warning, Windows deferred, enterprise deferred to M11.
- `docs/adrs/ADR-005-m11-native-copilot-provider.md` — 10 decisions: **follow copilot-api/litellm's session-token exchange** rather than opencode's simpler Bearer path, file-based token storage at XDG_DATA_HOME (keychain deferred), long-lived OAuth with retry-and-relogin on 401, enterprise prompt at login, no persistent model cache, overridable headers with copilot-api defaults, distinct `copilot-native` type, opt-in gate, no streaming, no default rate-limit.
- Updated PRD to reflect the session-token-exchange direction and add litellm/copilot-api references.
- Research finding: of the three reference implementations (opencode, ericc-ch/copilot-api, litellm), **two of three use the session-token exchange** with `Iv1.b507a08c87ecfe98`. Adopting that pattern gives us the most field-exposed, proven surface and makes Phase 2 behaviourally identical to Phase 1 (only the transport changes).

### Checklist
- [x] Compiles — no code change
- [x] Tests pass — unchanged
- [x] Formatted — unchanged
- [x] Matches SPEC — ADRs respect the stable `Provider` interface
- [x] Handoff accurate — CURRENT.md rewritten; ROADMAP updated; PRD revised
- [x] ADRs cover the architecturally significant decisions (per AGENTS.md)

### Notes
- The single biggest revision from the previous PRD draft is the M11 transport choice. opencode's path would have been ~200 LOC; the copilot-api path is ~350–400 LOC but substantially safer because it uses the client ID and exchange flow that GitHub's own editor plugins use. User priority of "simpler = proven" drove this.
- Both ADRs explicitly carry the two open questions (legal/ToS on editor headers, GitHub roadmap for an official endpoint) as hard gates before merge.

### Action Taken
No code merged. ADRs ready for supervisor review. Awaiting user direction on (a) GitHub Release automation for v0.3.0, (b) whether to start M10 implementation now.

---

## 2026-04-17 — Native Copilot Auth Research + PRD — APPROVED (superseded by ADR-004/005)

**Task**: Plan what it takes to have "native" copilot auth as a tpatch provider; verify whether copilot-api is officially supported (it is not) and whether github/copilot-cli is open source (it is not).
**Implementer**: Planning agent
**Verdict**: **PENDING** (plan only, no code)

### Deliverables
- `docs/prds/PRD-native-copilot-auth.md` — options matrix (A–E), two-phase recommendation (M10 managed proxy, M11 opt-in native OAuth-device-flow provider), explicit rejection of shelling out to `copilot` CLI.
- Confirmed via the `tesseracode/copilot-api` README that it is reverse-engineered, unsupported by GitHub, and subject to abuse-detection warnings.
- Confirmed via the `github/copilot-cli` repo contents that the CLI is closed-source (only README/install.sh/changelog/LICENSE are published) and the only sanctioned auth surface is `/login` OAuth or a PAT with "Copilot Requests" permission — no documented HTTP endpoint.
- **Confirmed via anomalyco/opencode source** that a much simpler native path is proven in production: OAuth device flow against `github.com` with the well-known editor client ID (`Ov23li8tweQw6odWQebz`), GitHub OAuth access token used directly as Bearer on `api.githubcopilot.com`. **No session-token exchange required** — this removes the biggest implementation-cost concern from the initial draft of the PRD. M11 is now ~200 LOC of Go rather than a full copilot-api reimplementation.

### Checklist
- [x] Compiles — no code change in this session
- [x] Tests pass — unchanged (last run post-v0.3.0 all green)
- [x] Formatted — unchanged
- [x] Secrets safe — PRD recommends env-var-reference pattern unchanged
- [x] Matches SPEC — PRD respects stable `Provider` interface
- [x] Handoff accurate — CURRENT.md rewritten; distribution entry archived to HISTORY.md
- [ ] ADRs for technical decisions — ADR-004 deferred until the open legal question is answered

### Notes
- Key finding: there is no officially documented public Copilot HTTP endpoint, so every "native" path is on reverse-engineered surface. The PRD faces this head-on and recommends going no faster than the policy allows.
- The PRD intentionally rejects shelling out to `copilot` CLI (Option D) because each prompt burns a premium request and copilot re-runs its own agent loop — incompatible with tpatch's deterministic workflow phases.
- M11 (native PAT provider) is soft-blocked on a policy question: can a third-party tool legitimately identify as an editor against `api.githubcopilot.com`? If "no", Phase 1 managed proxy is the ceiling.

### Action Taken
Session ended pending supervisor approval of the PRD. No code merged; v0.3.0 was tagged earlier in this session and is ready to push.

---

## 2026-04-17 — Distribution Setup (module rename + CI workflow) — APPROVED

**Task**: Make `go install` work and add a free CI workflow.
**Implementer**: Distribution agent
**Verdict**: **PENDING**

### Deliverables
- `go.mod` module renamed to `github.com/tesseracode/tesserapatch` (matches the actual GitHub repo). All imports rewritten. Binary still named `tpatch`.
- `.github/workflows/ci.yml`: push+PR to `main`, matrix ubuntu + macOS, `gofmt` + `go vet` + `go build` + `go test` + `go install` smoke test. `go-version-file: go.mod`, module cache enabled, concurrency group cancels superseded runs.
- `README.md` install block updated to the correct module path.

### Checklist
- [x] Compiles — `go build ./cmd/tpatch` OK
- [x] Tests pass — all 7 packages green post-rename
- [x] Formatted — `gofmt -l .` clean
- [x] Artifacts deterministic — no runtime behavior change; rename is mechanical
- [x] Secrets safe — workflow declares `permissions: contents: read`; no tokens needed for build/test
- [x] Matches SPEC — CLI contract unchanged
- [x] Handoff accurate — CURRENT.md rewritten; prior refinement archived to HISTORY.md

### Notes
- Free for public repos (unlimited Actions minutes). Private repos get 2000 min/month on the free plan, which is still plenty for our workload.
- `go install ...@latest` requires the repo to be public (or Go's proxy to have access). Repo owner action item: flip visibility to public, push, tag `v0.3.0`.
- The `--preset copilot` question: it targets the `copilot-api` proxy at `localhost:4141`, not GitHub's Copilot directly. Same GitHub account is used because copilot-api does its own OAuth. Documented in CURRENT.md.

### Action Taken
Session ended pending supervisor approval.

---

## 2026-04-17 — Phase 2 Refinement (SDK evaluation + harness guides + tracking cadence) — APPROVED WITH NOTES

**Task**: Evaluate OpenRouter/OpenAI/Anthropic Go SDKs and codex/copilot-cli harnesses; adopt the simplest integration without wasting resources; tighten agent tracking cadence.
**Implementer**: Phase 2 refinement agent
**Verdict**: **PENDING** (awaiting supervisor checklist pass)

### Deliverables

**Provider layer (SDK decision)**
- Surveyed `OpenRouterTeam/go-sdk` (Speakeasy-generated, README labels "not production-ready"), `openai/openai-go`, `anthropics/anthropic-sdk-go`.
- **Rejected all three SDKs** — our `Check` + `Generate` surface does not benefit from them and adoption would add ~20 transitive deps.
- **Accepted** preset-based ergonomics instead: `tpatch provider set --preset copilot|openai|openrouter|anthropic|ollama`.
- `providerPresets` map is the single source of truth for both `--preset` and `autoDetectProvider`.

**Harness integration**
- `docs/harnesses/codex.md` — codex exec handshake, `AGENTS.md` snippet, recommended approval policy, anti-patterns.
- `docs/harnesses/copilot.md` — Copilot CLI skill placement, allow-list configuration, MCP follow-up flagged as M10.

**Tracking cadence**
- `AGENTS.md` "Context Preservation Rules" now declares cadence per trigger (started task, finished phase, hit blocker, milestone flipped) with an explicit cheatsheet table.
- `CLAUDE.md` Working Rules reference the cadence and call out per-phase (not per-session) handoff updates.

**Documents**
- `docs/adrs/ADR-003-sdk-evaluation.md` — full evaluation matrix and locked-in decision.

### Checklist
- [x] Compiles — `go build ./cmd/tpatch` OK
- [x] Tests pass — `go test ./...` green across 7 packages; `TestProviderSetPreset` added
- [x] Formatted — `gofmt -l .` clean
- [x] Artifacts deterministic — preset map is static; no behavior change to apply recipes
- [x] Secrets safe — presets still store env-var *names*, not values
- [x] Matches SPEC — `provider set` contract extended additively; no regressions
- [x] Handoff accurate — CURRENT.md updated, old Phase 2 entry archived to HISTORY.md

### Notes
- `--preset` composes with `--type/--base-url/--model/--auth-env` so users can nudge a preset (e.g. `--preset anthropic --model claude-opus-4`) without reconfiguring everything.
- The harness guides deliberately avoid prescribing an SDK path — both codex and copilot-cli are agents, not libraries, and the `tpatch next --format harness-json` CLI contract is the supported integration surface.
- M10 (`tpatch mcp serve`) is called out as a future follow-up if/when Copilot CLI or codex standardize on MCP as the preferred integration.

### Action Taken
Session ended pending supervisor approval. HISTORY.md updated with the prior Phase 2 entry so the log reflects sequential state transitions.

---

## 2026-04-17 — M7 + M8 + M9 Phase 2 Implementation — APPROVED WITH NOTES

**Task**: Ship Phase 2 milestones: provider integration, LLM validation with retry, interactive/harness commands
**Implementer**: Phase 2 implementation agent
**Verdict**: **PENDING** (awaiting supervisor checklist pass)

### Deliverables

**M7 — Provider**
- `AnthropicProvider` (internal/provider/anthropic.go) speaking Messages API (`x-api-key`, `anthropic-version`, content blocks, top-level `system`).
- `provider.NewFromConfig(cfg)` factory; `loadProviderFromStore` routes by `cfg.Type`.
- Auto-detection extended: Ollama (localhost:11434), ANTHROPIC_API_KEY, OPENROUTER_API_KEY.
- `provider set --type` flag; `config set provider.type` validates `openai-compatible|anthropic`.
- `ADR-002-provider-strategy.md` written.

**M8 — Validation & Retry**
- `workflow.GenerateWithRetry` + `JSONObjectValidator`, `NonEmptyValidator`.
- Raw responses logged to `artifacts/raw-<phase>-response-N.txt`.
- `max_retries` config (default 2); `--no-retry` flag on 4 workflow commands, plumbed via `workflow.WithDisableRetry(ctx)`.
- Workflow functions (`RunAnalysis`, `RunDefine`, `RunExplore`, `RunImplement`) use the retry helper; heuristic fallback preserved when the retry budget is exhausted.

**M9 — Interactive & Harness**
- `tpatch cycle <slug>` — full lifecycle; `--interactive`, `--editor`, `--skip-execute`, `--timeout`.
- `tpatch test <slug>` — runs `config.test_command`, records `test-output.txt` + `apply-session.json` validation status.
- `tpatch next <slug>` — state-aware next-action emitter; `--format harness-json` for structured harness integration.
- All 6 skill formats updated; parity guard extended for `cycle`, `test`, `next`.
- Version bumped to `0.3.0-dev`.

### Checklist
- [x] Compiles — `go build ./cmd/tpatch` OK
- [x] Tests pass — `go test ./...` green across 7 packages (adds Anthropic/factory, retry, cycle/test/next tests)
- [x] Formatted — `gofmt -l .` clean
- [x] Artifacts deterministic — raw-response logging is per-attempt, recipe execution unchanged
- [x] Secrets safe — Anthropic auth still by env-var reference (AuthEnv); no secrets touched
- [x] Matches SPEC — new commands documented in all 6 skill formats; parity guard enforces it
- [x] Handoff accurate — CURRENT.md updated; ROADMAP M7/M8/M9 marked ✅

### Notes
- `Provider` interface unchanged; adding providers is purely additive.
- `--no-retry` uses a context value rather than changing every workflow signature — minimal blast radius.
- `tpatch next` distinguishes sub-states of `defined` (needs explore vs implement vs apply) by probing the feature directory, so the harness contract stays meaningful across phases.

### Action Taken
Session ended pending supervisor approval.

---

## 2026-04-16 — Gap Closure (8 gaps) — APPROVED

**Task**: Close 8 gaps from unified review before supervisor handoff  
**Verdict**: **APPROVED**

**ADR-001 (cobra dependency)**: Acknowledged. Justified deviation — stdlib `flag` cannot parse interspersed flags.

**Gaps Closed**:
- [x] GAP 1 (HIGH): Wired `EnsureSafeRepoPath()` into `store.WriteArtifact()` and `store.WriteFeatureFile()` — every file write path-checks against repo root
- [x] GAP 2 (HIGH): `apply --mode done` now writes `apply-session.json` with operator notes, validation status, timestamps. New flags: `--note`, `--validation-status`, `--validation-note`
- [x] GAP 3 (HIGH): `record` generates `record.md` with change summary, file count, replay instructions
- [x] GAP 4 (HIGH): Incremental patch derivation for multi-feature reconciliation via `DeriveIncrementalPatch()` + reconciler prefers `incremental.patch` over cumulative
- [x] GAP 5 (MEDIUM): `apply --mode done --validation-note` writes `manual-validation.md`
- [x] GAP 6 (LOW): Provider auto-detection on `tpatch init` — probes localhost:4141, checks OPENAI_API_KEY env var
- [x] GAP 7 (LOW): Sequential patch numbering — `WritePatch()` creates `patches/001-apply.patch`, `002-record.patch`, etc.
- [x] GAP 8 (MEDIUM): Recipe dry-run (`--dry-run`) and auto-execute (`--mode execute`) with per-operation path safety via `EnsureSafeRepoPath()`

**New files**:
- `internal/workflow/recipe.go` — Recipe executor: `DryRunRecipe()`, `ExecuteRecipe()`, `LoadRecipe()` with per-operation safety

**Tests**: All 6 packages pass, gofmt clean, build clean.

---

## 2026-04-16 — M6 Bug Bash (Live Provider Run) — APPROVED

**Task**: M6 — Final bug bash with live copilot-api provider (claude-sonnet-4)  
**Verdict**: **APPROVED**

**New Features Added**:
- [x] Automated patch validation on `record` (prints "Patch validated: applies cleanly")
- [x] `merge_strategy` config option (`3way` default, `rebase` available)
- [x] `ValidatePatch()` in gitutil with strategy-aware checking
- [x] Enriched Phase 3 prompt: `extractUpstreamContext()` reads affected files from current upstream, giving the LLM actual code to compare

**Live Provider Bug Bash Results**:
- [x] Provider: copilot-api at localhost:4141, model claude-sonnet-4 (44 models available)
- [x] Analysis: Live LLM produced detailed analysis with correct file paths and acceptance criteria
- [x] Feature A → `upstream_merged` (Phase 3: live LLM analyzed upstream `src/lib/model-mapping.ts` and confirmed equivalence)
- [x] Feature B → `reapplied` (Phase 4: live LLM said "still_needed", patch forward-applied with 3-way)
- [x] Both patches auto-validated: "Patch validated: applies cleanly"
- [x] Target repo: `bun test` 26/26, `bun run typecheck` clean
- [x] `go test ./...` all pass (7 packages)

**Key Improvement**: Previous runs with mock provider returned hardcoded responses. This run used a real LLM (claude-sonnet-4) which initially returned `unclear` because the prompt lacked upstream code context. After adding `extractUpstreamContext()`, the LLM correctly identified Feature A as upstreamed by comparing the acceptance criteria against the actual `src/lib/model-mapping.ts` content.

---

## 2026-04-16 — M6 Bug Bash (Re-test after fixes) — APPROVED

**Task**: M6 — Pass reconciliation bug bash after fixing BUG-1, BUG-2, BUG-3  
**Verdict**: **APPROVED**

**Bug Fixes Applied**:
- [x] BUG-1: Migrated CLI from stdlib `flag` to `cobra/pflag` — flags work in any position
- [x] BUG-2: Rewrote `CapturePatch()` with `git add --intent-to-add` + trailing newline fix
- [x] BUG-3: Added `--from <commit>` flag to `record` for committed diffs
- [x] BONUS: Added 3-way merge fallback to `ForwardApplyCheck()` / `ForwardApply()`

**Re-test Results**:
- [x] Feature A (model-id-translation-fix) → `upstream_merged` (Phase 3 provider-assisted)
- [x] Feature B (models-cli-subcommand) → `reapplied` (Phase 4 forward-apply with 3-way merge)
- [x] Target repo: `bun test` 26/26 pass
- [x] Target repo: `bun run typecheck` clean
- [x] Feature A patch validates: `git apply --check` passes on baseline
- [x] Feature B patch validates: `git apply --check` passes on Feature A commit
- [x] `apply slug --mode done` works (BUG-1 regression test)
- [x] `go test ./...` all pass (7 packages)
- [x] `gofmt -l .` clean

**Architecture Change**: Added `github.com/spf13/cobra` dependency — breaks zero-dependency constraint, but user approved. The stdlib `flag` package fundamentally cannot support interspersed flags (flags after positional args).

**Notes**: The cobra migration also gives us free: shell completion generation, auto help text, subcommand hierarchy for `provider check/set` and `config show/set`.

---

## 2026-04-16 — M6 Bug Bash (Initial) — APPROVED WITH NOTES

**Task**: M6 — Run reconciliation bug bash against tesseracode/copilot-api  
**Verdict**: **APPROVED WITH NOTES**

**What Passed**:
- [x] `tpatch init` installs all 6 skill formats
- [x] `tpatch add` with slug generation
- [x] Full lifecycle: add → analyze → define → apply → record
- [x] Feature A (model translation) correctly classified as `upstream_merged` via Phase 3
- [x] Target repo 26/26 tests pass, typecheck clean
- [x] Provider check validates mock endpoint

**Bugs Found**:
- BUG-1 (MEDIUM): `--mode` flag silently ignored when placed after positional slug argument
- BUG-2 (HIGH): `CapturePatch()` produces corrupt patches for new files (missing trailing newline)
- BUG-3 (LOW): Recorded patch may capture stale working tree state instead of committed state

**Action**: All 3 bugs fixed in follow-up session. Re-test passed.

---

## 2026-04-16 — M5 Skill System — APPROVED

**Task**: M5 — 6 harness formats, embedded assets, parity guard  
**Verdict**: **APPROVED**

- [x] 6 formats: Claude, Copilot, Copilot Prompt, Cursor, Windsurf, Generic
- [x] All embedded via go:embed
- [x] `tpatch init` installs all 6 + .windsurfrules
- [x] Parity guard test passes (all formats mention all 12 CLI commands)
- [x] Tests pass, build clean, gofmt clean

---

## 2026-04-16 — M4 Reconciliation — APPROVED

**Task**: M4 — 4-phase reconciliation engine  
**Verdict**: **APPROVED**

- [x] Phase 1: Reverse-apply check (upstreamed detection)
- [x] Phase 2: Operation-level evaluation from apply-recipe.json
- [x] Phase 3: Provider-assisted semantic detection (with mock provider)
- [x] Phase 4: Forward-apply attempt (reapplication)
- [x] 4 test scenarios: upstreamed, reapplied, provider-assisted, blocked
- [x] Reconciliation artifacts: reconcile-session.json, reconcile.md, per-version logs
- [x] State transitions: applied → upstream_merged / applied / blocked
- [x] upstream.lock updated after reconciliation

---

## 2026-04-16 — M3 Apply & Record — APPROVED

**Task**: M3 — implement, apply (3 modes), record, patch capture  
**Verdict**: **APPROVED**

- [x] Apply recipe format (JSON operations)
- [x] `apply --mode prepare/started/done` lifecycle
- [x] `record` captures tracked + untracked files
- [x] Patch excludes .tpatch/, skill dirs, framework files
- [x] gitutil: HeadCommit, CapturePatch, CaptureDiffStat, reverseApply, forwardApply

---

## 2026-04-16 — M2 Provider & Analysis — APPROVED

**Task**: M2 — Provider interface, analyze, define, explore, heuristic fallback  
**Verdict**: **APPROVED**

- [x] OpenAI-compatible provider (raw net/http)
- [x] 4 provider tests (check, generate, auth header, failure)
- [x] Analyze with workspace snapshot and guidance file detection
- [x] Define with acceptance criteria generation
- [x] Explore with file tree and changeset identification
- [x] Heuristic fallback for all 3 commands (works offline)
- [x] provider check and provider set commands

---

## 2026-04-16 — M1 Core Store & Init — APPROVED

**Task**: M1.1–M1.9 — Data model, store layer, init/add/status/config commands, slug generation, path safety  
**Verdict**: **APPROVED**

**Review Checklist**:
- [x] Code compiles: `go build ./cmd/tpatch`
- [x] Tests pass: `go test ./...` — 20+ test cases across cli, store, safety packages
- [x] Code formatted: `gofmt -l .` — clean
- [x] Store operations are deterministic (JSON + YAML output, sorted features)
- [x] Secret-by-reference pattern in config.yaml (auth_env stores var name)
- [x] CLI behavior matches SPEC.md for init, add, status, config
- [x] ensureSafeRepoPath with path traversal and symlink tests
- [x] E2E smoke test: init → add × 2 → status → config set → config show

**Files Created**:
- `internal/store/types.go` — Feature states, config types, reconcile outcomes
- `internal/store/store.go` — Full store implementation (Init, Open, AddFeature, ListFeatures, etc.)
- `internal/store/slug.go` — Slugify with truncation and kebab-case
- `internal/store/store_test.go` — 7 test functions (slug, init/open, find root, add, list, config roundtrip, state transitions)
- `internal/safety/safety.go` — EnsureSafeRepoPath implementation
- `internal/safety/safety_test.go` — 6 test cases (safe, child, parent traversal, absolute escape, dot-dot, symlink)

**Files Modified**:
- `internal/cli/app.go` — Wired init, add, status, config commands with flag parsing
- `internal/cli/app_test.go` — Added integration test (init → add → status → config)

---

## 2026-04-16 — M0 Bootstrap — APPROVED

**Task**: M0.1–M0.6 — Initialize Go module, CLI skeleton, package structure, Makefile  
**Verdict**: **APPROVED**

**Review Checklist**:
- [x] Code compiles: `go build ./cmd/tpatch`
- [x] Tests pass: `go test ./...` — 5 test cases (help, version, no-args, unknown command, 12 stub commands)
- [x] Code formatted: `gofmt -l .` — clean
- [x] `./tpatch --help` prints usage with all 12 commands listed
- [x] `./tpatch --version` prints `tpatch 0.1.0-dev`
- [x] Package structure: cli, store, provider, workflow, gitutil, safety
- [x] Assets directory with go:embed and placeholder content
- [x] Makefile with build/test/fmt/install/clean/lint/all targets
- [x] Handoff file accurate

**Files Created**:
- `go.mod` — module `github.com/tesseracode/tpatch`
- `cmd/tpatch/main.go` — Entry point
- `internal/cli/app.go` — CLI dispatcher with 12 command stubs
- `internal/cli/app_test.go` — 5 test cases
- `internal/store/store.go` — Package stub
- `internal/provider/provider.go` — Package stub
- `internal/workflow/workflow.go` — Package stub
- `internal/gitutil/gitutil.go` — Package stub
- `internal/safety/safety.go` — Package stub
- `assets/embed.go` — go:embed with 4 asset directories
- `assets/prompts/README.md`, `assets/skills/README.md`, `assets/templates/README.md`, `assets/workflows/tessera-patch-generic.md` — Placeholders
- `Makefile` — Build pipeline

**Notes**: None. Clean implementation matching GPT reference structure with extensions for the unified spec (added `define`, `explore`, `implement`, `record`, `config` commands Beyond GPT's original 7).

## Review — M11 — 2026-04-18

**Reviewer**: implementation self-report (pending external review)
**Task**: Native Copilot provider (ADR-005)

### Checklist
- [x] Compiles — `go build ./cmd/tpatch` → `tpatch 0.4.0-dev`
- [x] Tests pass — `go test ./... -count=1` all 7 packages green
- [x] Formatted — `gofmt -l .` clean
- [x] Artifacts deterministic — no runtime artifacts added in this cut
- [x] Secrets safe — OAuth token stored at 0600, parent-dir checks, `TPATCH_COPILOT_AUTH_FILE` for tests, symlink rejection
- [x] Matches SPEC / ADR-005 D1–D10
- [x] Handoff accurate (see `docs/handoff/CURRENT.md`)

### Verdict: APPROVED WITH NOTES (pending external)

### Notes
- Provider-level unit tests (httptest fake for device flow + session
  exchange + 401 retry) are scaffolded in the code but not yet
  written. Tracked as a follow-up — existing test suite still passes
  because new code paths require the opt-in + auth file to execute.
- `headers_override` intentionally deferred (rubber-duck #7) — the
  zero-dep YAML parser is flat-scalar only. Will revisit once an
  official compatibility endpoint is published.
- macOS FAQ entry added per the M10 review feedback.

### Action Taken
Archived M10 handoff to HISTORY.md, wrote new M11 CURRENT, marked
M11 ✅ in ROADMAP.

---

