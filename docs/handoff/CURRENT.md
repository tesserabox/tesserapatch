# Current Handoff

## Active Task

- **Task ID**: M15-W3-SLICE-C
- **Milestone**: M15 Wave 3 — Verify freshness overlay
- **Description**: Slice C — V3–V9 real implementations including
  hard-parent topological closure replay (V7/V8). Replaces the V3–V9
  stubs shipped in Slice A.
- **Status**: Review — revision-2 complete, awaiting reviewer.
- **Assigned**: 2026-04-28

## Scope (per PRD-verify-freshness.md §9 Slice C)

Implement the seven verify checks currently stubbed in
`internal/workflow/verify.go`:

- **V3** `recipe_op_targets_resolve` — block. For each op in
  `apply-recipe.json`, the op's `Path` must exist OR carry a
  `created_by` whose parent is a declared **hard** dep currently in
  `applied`/`upstream_merged`. Reuse M14.2 `created_by` semantics
  (`internal/workflow/created_by_gate.go:57`).
- **V4** `dep_metadata_valid` — block. Call
  `store.ValidateDependencies(s, slug, status.DependsOn)`
  (`internal/store/validation.go:66`). Wraps the validation sentinel
  verbatim in the remediation field.
- **V5** `satisfied_by_reachable` — block. For every dep with
  `satisfied_by` set, validate the SHA matches
  `store.satisfiedBySHARe.MatchString` AND
  `gitutil.IsAncestor(repoRoot, dep.SatisfiedBy, "HEAD")` returns
  true (`internal/gitutil/gitutil.go:680`).
- **V6** `dependency_gate_satisfied` — **warn** (gated on
  `Config.DAGEnabled()`). Call
  `workflow.CheckDependencyGate(s, slug)`
  (`internal/workflow/dependency_gate.go:42`). Severity reduced from
  block to warn so V5 does the precise work and V6 echoes context;
  also accommodates the two scenarios documented in PRD §3.4.4.
- **V7** `recipe_replay_clean` — block. **Hard-parent topological
  closure replay** per PRD §3.4.3:
  1. Compute hard-parent closure via `store.DependsOn` walking only
     `DependencyKindHard` edges, transitively, until fixed point.
  2. Order the closure with `store.TopologicalOrder`
     (`internal/store/dag.go:107`) over the hard-only sub-DAG.
  3. For each parent in topo order, replay its
     `apply-recipe.json` into a `gitutil.CreateShadow` worktree.
     Skip parents in `upstream_merged` (their changes are already
     in the baseline). `applied` parents replay. Any other state is
     a fail-fast condition: abort with
     `failed_at: "parent-replay"` and `parent_slug: <slug>` in the
     report.
  4. After all replayable parents have replayed, apply the
     **target's** recipe in the same shadow.
- **V8** `post_apply_patch_replay_clean` — block. Reuses V7's shadow.
  After V7 succeeds, `git apply --check` of `post-apply.patch`
  against the closure-replayed shadow.
- **V9** `reconcile_outcome_consistent` — warn. Reads
  `status.Reconcile.Outcome` only. Adversarial test must pin that
  V9 never reads any artifact (D6 invariant).

V7+V8 share a **single shadow allocation** per verify run. The
closure replays once, target recipe applies once, target patch
`--check` once, then `PruneShadow` regardless of pass/fail.

## Constraints (binding from ADR-013 + PRD §3)

- **D6** — verify never reads `reconcile-session.json` or any
  `artifacts/` file beyond the recipe and the post-apply patch. V9
  reads `status.Reconcile.Outcome` from `status.json` only.
- **D7** — verify is read-only on the working tree. The shadow used
  by V7/V8 is pruned before verify exits.
- Static checks (V0–V6, V9) run before the dynamic V7/V8 phase, so a
  recipe-shape error doesn't waste a shadow allocation.
- The closure-replay primitive lives **only** in
  `internal/workflow/verify.go`. Don't factor it out — ADR-010 D2
  reserves shadows for the M12 resolver scope; if a future feature
  needs the same primitive, it gets an ADR amendment.
- V0–V2 contract is locked from Slice A — must not regress.
- Slice B freshness derivation reads from `Verify` only; Slice C
  changes what populates `Verify`, not how labels derive from it.

## Skill assets

Slice C is the V3–V9 implementation phase. Skill bullets and parity
guard updates are **Slice D scope**, not Slice C. Don't touch
`assets/` in this slice.

## Tests required (per PRD §9 Slice C)

- Per-check unit tests for V3, V4, V5, V6, V9 (V7/V8 covered by
  closure-replay fixtures).
- Closure-replay test fixtures:
  - 3-deep DAG happy path.
  - Parent-fail mid-closure → `failed_at: "parent-replay"`.
  - `upstream_merged` parent in middle of closure → skipped without
    error.
- Concurrency-with-reconcile refusal test (state-based refusal per
  PRD §3.4.6 / verify.go:101 + verify.go:182 — verify refuses when
  the target's lifecycle state is incompatible; no separate per-slug
  lock primitive exists or is required).
- Source-truth adversarial test pinning V9 reads no artifact files.

## Validation gate (must pass before review dispatch)

```bash
gofmt -l .                       # empty
go test ./...                    # all packages green
go build ./cmd/tpatch            # clean
```

## Slice C revision-2 (HIGH finding fix)

External supervisor reviewed Slice C revision-1 @ `5892ae0` and
returned NEEDS REVISION on a single HIGH finding: the `patchPresent`
probe in `verify.go:241-244` keyed off `fi.Size() > 0` in addition to
file presence. PRD-verify-freshness §3.1.2 V8 row defines the V8
precondition as **`artifacts/post-apply.patch` present** (file
exists), and §5 line 525's "post-apply.patch absent | V8 is skipped"
means the file is missing — not zero-byte.

Live repro on revision-1: `applied` feature, no `apply-recipe.json`,
zero-byte `artifacts/post-apply.patch` → `verdict=passed` with V8
skipped (`reason="no post-apply.patch (precondition not met)"`).
False pass on a malformed patch artifact.

### What changed

- `internal/workflow/verify.go` — **one logical line removed** from
  the patchPresent probe (`verify.go:242`). Probe now reads:
  ```go
  if fi, statErr := os.Stat(patchPath); statErr == nil && !fi.IsDir() {
  ```
  Zero-byte patches are now treated as present. Downstream
  `runClosureReplay` and `git apply --check` already handle the
  zero-byte case correctly: `git apply --check` exits 128 with
  `"No valid patches in input"`, and V8's existing error path
  emits the verbatim §3.1.2 remediation.
- `internal/workflow/verify_closure_replay_test.go` — added
  `TestRunVerify_PatchZeroByte_TreatedAsPresent_V8Fails`. Builds an
  `applied` feature with no recipe and a zero-byte
  `post-apply.patch`. Asserts V7 skipped (recipe absent), V8 fails
  with verbatim §3.1.2 remediation, verdict=failed, shadow pruned.

### Invariants preserved

- ADR-013 D7 — `defer PruneShadow` unchanged.
- Single `CreateShadow` per verify run unchanged.
- ADR-010 D2 — closure-replay primitive stays private.
- ADR-013 D6 — V9 source-truth unchanged.
- Slice B `RecipeHashAtVerify` semantics unchanged.
- V0–V6 + V9 logic unchanged.
- Static-before-dynamic ordering unchanged.
- V6 warn severity gated on `Config.DAGEnabled()` unchanged.
- All remediation strings verbatim.
- All revision-1 tests still pass unchanged.

### Live repro proof

BEFORE (rev1 `5892ae0`, zero-byte patch, no recipe):

```
VERDICT passed
V8 passed=True skipped=True reason='no post-apply.patch (precondition not met)'
```

AFTER (rev2, same repro):

```
VERDICT failed
V7 passed=True skipped=True reason='no apply-recipe.json (precondition not met)'
V8 passed=False skipped=False remediation='post-apply.patch no longer applies to closure-replayed baseline; run tpatch reconcile demo'
```

Shadow dir empty after run.

## Slice C revision-1 (HIGH finding fix)

External supervisor reviewed Slice C @ `32f50c8` and returned NEEDS
REVISION on a single HIGH finding: `runClosureReplay` short-circuited
**both** V7 and V8 when `apply-recipe.json` was absent, contradicting
PRD-verify-freshness §5 line 524:

> Recipe absent (`apply-recipe.json` missing) | V2/V3/V7 are skipped;
> V8 runs against the closure-replayed baseline if patch is present.
> V1/V4/V5/V6/V9 run.

The pre-fix behaviour silently passed verify when the post-apply
patch was unparseable, masking real drift. The supervisor's live
repro (no recipe + invalid patch text) showed verdict=`passed`,
V8={`passed:true, skipped:true, reason:"no apply-recipe.json …"`}.

### What changed

- `internal/workflow/verify.go`:
  - `RunVerify` now probes `post-apply.patch` (non-empty file probe,
    same shape as the existing V8 stat) and passes `patchPresent`
    into `runClosureReplay`.
  - `runClosureReplay` signature extended with `patchPresent bool`.
    Restructured into four matrix cells:
    * recipe absent + patch absent → both V7/V8 skipped; **no shadow
      allocated** (PRD §5 line 526).
    * recipe absent + patch present → shadow allocated, closure
      replayed; V7 = skip ("no apply-recipe.json (precondition not
      met)"), V8 runs `git apply --check` against the closure-
      replayed baseline.
    * recipe present + patch absent → V7 runs (existing); V8 skipped
      ("no post-apply.patch (precondition not met)").
    * recipe present + patch present → both run (existing).
  - Parent-replay fail-fast still fires regardless of `recipePresent`:
    V7 = parent-replay remediation (verbatim PRD §3.4.3 form); V8 =
    skip with reason `"skipped: parent-replay aborted before V8"`
    (PRD §4.3.5 example). Reason text updated from the previous
    less-specific "V7 (recipe_replay_clean) failed: parent-replay".
  - V8 remediation string remains verbatim PRD §3.1.2: `"post-apply
    .patch no longer applies to closure-replayed baseline; run
    tpatch reconcile <slug>"`.
- `internal/workflow/verify_closure_replay_test.go`: four new tests
  added at the bottom of the file:
  * `TestRunVerify_RecipeAbsent_PatchPresent_V8RunsAgainstClosureBaseline`
    — happy path: V7 skipped, V8 passes against valid new-file patch.
  * `TestRunVerify_RecipeAbsent_PatchPresent_V8FailsOnInvalidPatch`
    — **regression test for the supervisor's bug repro**. Asserts
    V8 fails with verbatim remediation and verdict=failed.
  * `TestRunVerify_RecipeAbsent_PatchAbsent_BothSkipped` — pins PRD
    §5 line 526; asserts no shadow lingers under `.tpatch/shadow/`.
  * `TestRunVerify_RecipeAbsent_PatchPresent_ParentReplayFailFast`
    — hard parent in `analyzed` state; asserts V7 fail with
    parent-replay remediation, V8 skipped with PRD §4.3.5 verbatim
    reason, `failed_at=parent-replay`, `parent_slug=stuck-parent`,
    verdict=failed.

### Invariants preserved

- ADR-013 D7 — `defer PruneShadow` covers every exit path.
- Single `CreateShadow` per verify run (gated on
  `recipePresent || patchPresent`).
- ADR-010 D2 — closure-replay primitive stays private to
  `verify.go`.
- ADR-013 D6 — V9 still reads `status.Reconcile.Outcome` only.
- Slice B `RecipeHashAtVerify` write semantics unchanged.
- V0–V2, V3–V6, V9 logic unchanged.
- Static-before-dynamic ordering preserved.
- All four existing `TestRunVerify_ClosureReplay_*` tests pass
  unchanged.

### Live repro proof

BEFORE (pre-fix `32f50c8`, supervisor's exact bug repro):

```
VERDICT passed
V8 passed=True skipped=True reason=no apply-recipe.json (precondition not met)
```

AFTER (rev1, same repro):

```
VERDICT failed
V7 passed=True skipped=True reason=no apply-recipe.json (precondition not met)
V8 passed=False skipped=False remediation=post-apply.patch no longer applies to closure-replayed baseline; run tpatch reconcile demo
```

## Files Changed

**Revision-2 delta** (on top of revision-1):

- `internal/workflow/verify.go` — one logical line removed from
  the `patchPresent` probe (the `&& fi.Size() > 0` clause). No
  other production change.
- `internal/workflow/verify_closure_replay_test.go` — added
  `TestRunVerify_PatchZeroByte_TreatedAsPresent_V8Fails`
  regression test.

**Revision-1 + original Slice C files** (unchanged in revision-2):

- `internal/workflow/verify.go` — V3–V9 stubs replaced with real
  implementations + hard-parent closure-replay primitive
  (`runClosureReplay`, `replayRecipeOpsInShadow`,
  `replayOpInShadow`, `loadParentRecipe`, `filterHardDeps`,
  `depSlugsHard`, `parentReplayFail`, `skipV8Because`,
  `anyBlockFailed`). Added `FailedAt`/`ParentSlug` fields to
  `VerifyReport` (omitempty). New imports: `os/exec`, `gitutil`,
  `safety`. `stubChecksAfterAbort` rewritten to emit the correct
  9-check shape with severities (V6/V9 warn, others block).
  ~+330 lines.
- `internal/workflow/verify_test.go` — Added `gitInitVerifyTest`
  helper; `setupVerifyFeature` now gitInits the repo before
  `store.Init` so `gitutil.CreateShadow` can create a worktree.
  Replaced `TestRunVerify_V3_MissingTargetIsDeferredToSliceC` with
  `TestRunVerify_V3_MissingReplaceTarget_FailsBlock`. Removed
  `TestRunVerify_StubsCarrySliceReason`. Updated
  `TestRunVerify_V0V1V2_AllPass` so V3 expects real-pass
  non-skipped.
- `internal/workflow/verify_slice_c_test.go` — NEW. 12 unit tests
  covering V3 (3), V4 (2), V5 (3), V6 (2), V9 (4 incl. source-
  truth adversarial via poisoned `reconcile-session.json` +
  `post-reconcile.json`) plus a Slice A V0/V1/V2 regression guard.
  Includes the `commitInRepo` helper used by closure-replay tests.
- `internal/workflow/verify_closure_replay_test.go` — NEW. Four
  closure-replay fixtures: `_3DeepDAG_Happy`,
  `_ParentFailMidClosure_FailFast` (asserts `failed_at`,
  `parent_slug`, verbatim PRD §3.4.3 remediation),
  `_UpstreamMergedParentSkipped`, and `_PrunesShadowOnExit`
  (pins ADR-013 D7 — shadow always pruned).

## Test Results

- `gofmt -l .` → clean.
- `go vet ./...` → clean.
- `go build ./cmd/tpatch` → clean.
- `go test ./... -count=1` → all packages green. Total `=== RUN`
  count: **453** (was 452 at revision-1 land — +1 for the new
  zero-byte regression test).
- Live supervisor repro (recipe absent + zero-byte patch) now
  reports verdict=failed, V8 fail with verbatim PRD §3.1.2
  remediation. Shadow dir empty after run.

## Test Results (revision-1, kept for context)

- `gofmt -l .` → clean.
- `go vet ./...` → clean.
- `go build ./cmd/tpatch` → clean.
- `go test ./...` → all packages green; `internal/workflow` runs
  in ~15s. `TestRunVerify_*` count post-revision-1: 49 (was 45 at
  Slice C land — +4 for revision-1 matrix tests).
- Live supervisor repro (recipe absent + invalid patch) now reports
  verdict=failed, V8 fail with verbatim PRD §3.1.2 remediation.

## Test Results (pre-revision-1, kept for context)

- `gofmt -l .` → clean.
- `go build ./cmd/tpatch` → clean.
- `go test ./...` → all packages green; `internal/workflow` runs
  in ~20s. `TestRunVerify_*` count: 45 (was ~28 pre-Slice-C).

## Session Summary

Slice C is implemented end-to-end against the single-file
constraint. Key decisions:

- **V3 op-type scope**: only `replace-in-file` and `append-file`
  trigger the existence check (mirroring `created_by_gate.go`'s
  invocation surface). `write-file`/`ensure-directory` create
  their target so existence is not a precondition.
- **V9 source-truth adversarial test**: technique (b) — poisoned
  `reconcile-session.json` + `post-reconcile.json`. If V9 ever
  parses either file the test would fail; passing proves V9 reads
  only `status.Reconcile.Outcome`.
- **Closure replay primitive scope**: kept private to
  `verify.go`. ADR-010 D2 + ADR-013 §3.4.3 reserve a shared
  shadow primitive for the M12 resolver — premature extraction
  rejected.
- **Parent recipe replay does NOT go through `ExecuteRecipe`**:
  the shadow has no `.tpatch/`, so the `created_by` apply-time
  gate cannot run. Implemented `replayOpInShadow` mirroring the
  4 op types directly. The gate is an apply-time concern, not a
  replay concern.
- **Test fixture gitInit**: pre-Slice-C `setupVerifyFeature` did
  not init a git repo, so V7 would have failed at
  `gitutil.CreateShadow` for every test. Added
  `gitInitVerifyTest` (package-local to avoid import cycle with
  the gitutil test package).

## Next Steps

1. Dispatch the Slice C implementer sub-agent (general-purpose,
   background) with this handoff as its scope.
2. Await internal sub-agent reviewer cycle (mandatory live closure
   replay reproduction — sub-agent reviewers have missed live
   reproductions on three previous Slice cycles, so the reviewer
   prompt must require the 3-deep DAG fixture run).
3. On internal APPROVED, send external supervisor prompt with the
   full Slice C diff plus the live shadow-replay reproduction.
4. On external APPROVED, archive Slice C to HISTORY.md and stage
   Slice D (`--all`, skill bullets, parity guard, CHANGELOG v0.6.2).

## Blockers

None. Slice B is shipped to origin/main on `1032cda`. ROADMAP shows
Slice B ✅. Working tree clean except untracked exploratory PRDs
(`docs/whitepapers/`, `docs/prds/PRD-feature-slices-and-nested-changes.md`,
`docs/prds/PRD-intent-version-control-evaluation.md`,
`docs/prds/PRD-tpatch-git-primitive-mapping.md`) which other agents
are working on — keep these untracked, do NOT include in Slice C
commits.

## Context for Next Agent

- The V0–V2 logic in `internal/workflow/verify.go` is the structural
  template. Each V3+ check follows the same pattern: probe
  pre-condition, run the actual check, populate the `CheckResult`
  with `Status` (`pass`/`fail`/`skip`), `Severity` (`block`/`warn`),
  and a remediation string per PRD §3.4.5.
- Remediation strings are spec'd in PRD §3.4.5 — copy verbatim, do
  not paraphrase. Harnesses scrape these.
- The closure-replay primitive is the architectural core of Slice C.
  Get it right once in `verify.go`; resist any urge to share it with
  the resolver / reconcile paths (ADR-010 D2 boundary).
- `gitutil.CreateShadow` allocates a worktree under `.tpatch/shadow`
  (singular, per `internal/gitutil/shadow.go:35`) scoped per-slug.
  `PruneShadow` cleans up. Both already exist from M12.
- `store.TopologicalOrder` exists from M14 — pass it the hard-only
  sub-DAG (filter `DependencyKindHard` edges).
- Slice B's `RecipeHashAtVerify` field on the `Verify` sub-record is
  populated by Slice C's verify writer. Don't change the
  `Verify.RecipeHashAtVerify` write semantics — Slice B's amend
  invalidation depends on its byte-identity guarantee.
- The V0–V2 freshness contract from Slice A says `Verify` is only
  populated when verify completes (pass or fail), not on
  parent-replay abort. PRD §3.4.3 is explicit:
  > The freshness record is written with `passed=false` and the V7
  > entry's `remediation` carries the failing parent slug + wrapped
  > error.
  So parent-replay failures DO write `Verify` with passed=false.
  State-based refusals (incompatible lifecycle state per
  verify.go:101 + verify.go:182) abort BEFORE V0 and do NOT write
  `Verify`.

## Out of scope (DO NOT touch)

- Skill assets (`assets/`) — Slice D.
- `tpatch verify --all` aggregate runner — Slice D.
- `assets_test.go` parity-guard anchor extension — Slice D.
- `CHANGELOG.md` v0.6.2 entry — Slice D.
- `docs/whitepapers/` and untracked exploratory PRDs.
- Any code path outside `internal/workflow/verify.go` (V3–V9 are
  pure additions to the existing verify.go).
- Existing apply-gate semantics — `dependency_gate.go` stays
  block-severity for the live apply path; V6 only reduces severity
  for the verify read path.
