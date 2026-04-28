# Current Handoff

## Active Task

- **Task ID**: M15-W3-SLICE-C
- **Milestone**: M15 Wave 3 — Verify freshness overlay
- **Description**: Slice C — V3–V9 real implementations including
  hard-parent topological closure replay (V7/V8). Replaces the V3–V9
  stubs shipped in Slice A.
- **Status**: Not Started — staged, awaiting implementer dispatch.
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
- Concurrency-with-reconcile refusal test (lock contention path).
- Source-truth adversarial test pinning V9 reads no artifact files.

## Validation gate (must pass before review dispatch)

```bash
gofmt -l .                       # empty
go test ./...                    # all packages green
go build ./cmd/tpatch            # clean
```

## Files Changed

(none yet — task not started)

## Test Results

(none yet)

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
- `gitutil.CreateShadow` allocates a worktree under `.tpatch/shadows/`
  scoped per-slug. `PruneShadow` cleans up. Both already exist from
  M12.
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
  Pre-V0 aborts (e.g. lock contention) do NOT.

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
