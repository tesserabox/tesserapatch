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
