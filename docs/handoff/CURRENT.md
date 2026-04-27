# Current Handoff

## Active Task

- **Task ID**: M14.4 — Status DAG view + skills/docs rollout + v0.6.0 release cutover
- **Milestone**: M14 — Feature Dependencies / DAG (Tranche D, v0.6.0)
- **Status**: Awaiting user approval to dispatch (M14.3 ✅ + correctness pass ✅ + C5 fix-pass ✅ APPROVED 2026-04-26)
- **Estimated size**: ~300 LOC + version bump + tag

### Context

M14.1 ✅ data model + DAG primitives. M14.2 ✅ apply gate + `created_by` schema. M14.3 ✅ reconcile topological traversal + composable labels + compound verdict. **M14 correctness pass ✅** addressed three external-reviewer findings before M14.4:

- **F1** (HIGH, was cutover-blocking): `created_by` apply-time gate now wired in `recipe.go` via new `ErrPathCreatedByParent` sentinel. Hard/soft/missing-from-depends_on classification matches PRD §4.3 contract. Closed the M14.2 gap.
- **F2**: Stale-parent-applied label is cleared after a clean reconcile (shared `attemptedAt` threaded through `saveReconcileArtifacts` + `updateFeatureState`).
- **F3**: Children with intrinsic outcome `ReconcileUpstreamed` suppress all parent-derived labels per ADR-011.

All four sub-milestones still flag-protected. With `features_dependencies: false`, runtime behavior is byte-identical to v0.5.3.

**M14.4 is the user-facing cutover.** Flipping the flag default to `true`, shipping `tpatch status --dag`, rolling label/dep documentation across all 6 skill formats, writing `docs/dependencies.md`, tagging v0.6.0.

This is the first M14 sub-milestone where end users observe new behavior. Dispatch only after explicit user approval.

### Authoritative docs

1. `docs/adrs/ADR-011-feature-dependencies.md` — D1–D9 (locked)
2. `docs/prds/PRD-feature-dependencies.md` — §3.5 (label matrix), §4.5 (precedence), §5 (UX)
3. `docs/ROADMAP.md` — M14.4 line + Tranche D summary
4. M14.1, M14.2, M14.3, correctness-pass closeout entries in `docs/supervisor/LOG.md`

### Scope (8 chunks — full PRD scope per supervisor decision 2026-04-26)

> **Scope decision**: M14.4 ships the **full** PRD dependency feature in v0.6.0 (Option A). The dep-management CLI (`feature deps add/remove`, `amend --depends-on`, `remove --cascade`) is part of the cut, not deferred to v0.6.1. Without it the user-facing story is "we shipped a dependency feature, but to use it edit YAML by hand," which undercuts the rollout.

#### Chunk A — `tpatch status --dag` (~140 LOC)

- New `--dag` flag on `status` command in `internal/cli/cobra.go`.
- Renders the dependency DAG for all features, or a single feature's transitive parent + child set if a slug is given (PRD §10 — scoped DAG replaces v1's `graph` command).
- Output: ASCII tree (deterministic by slug) showing each feature with state + reconcile outcome + labels (using `EffectiveOutcome()`).
- Hard deps shown with `─►`, soft deps with `┄►`.
- `--json` flag for harness consumption (PRD §10 — reuses existing `--json` flag, NOT `--format json`).
- Tests: cycle handling (must never hang — protected by `DetectCycles`), empty DAG, scoped single-feature subset, label rendering, JSON shape stability.

#### Chunk B — Flag default flip (~5 LOC + many test fixtures)

- `internal/store/store.go`: change `features_dependencies` default from `false` to `true`.
- This is the moment the new behavior becomes observable. **Audit every test fixture that asserts byte-identity** — some may need updating to include `labels: []` or topo-ordered output.
- Run full suite. Fix every regression.

#### Chunk C — Dep-management CLI verbs (~250 LOC, **new in scope**)

Per PRD §3.7, §10. Without these, users edit YAML by hand.

- `tpatch feature deps <slug>` — read-only print of the dep block.
- `tpatch feature deps <slug> add <parent>[:hard|:soft]` — adds an edge. Validates cycles + parent existence + no self-ref + no kind conflict (existing M14.1 rules). Atomic write to `status.json`. Re-derives `dependents` across the store.
- `tpatch feature deps <slug> remove <parent>` — atomic edge removal. Re-derives `dependents`. Does NOT scan recipe bodies (per PRD §10 explanation — `created_by` cross-check catches stale references at next `implement`).
- `tpatch amend --depends-on <parent>[:hard|:soft]` and `tpatch amend --remove-depends-on <parent>` — same validation path as `feature deps add/remove`.
- `tpatch remove <slug> --cascade` — when dependents exist:
  - No `--cascade` → `ErrHasDependents` listing dependents (PRD §3.7).
  - `--cascade` + TTY → single confirm prompt listing full subtree; on Y, removes children in **reverse-topological order** (leaves first) then parent; per-feature summary.
  - `--cascade` non-TTY → `ErrInteractiveRequired` unless `--cascade --force`.
  - `--force` ALONE does NOT bypass dep check (force is for TTY confirm prompt, not graph integrity — PRD §3.7).
  - Soft and hard dependents treated identically (no "drop soft only" mode in v1).
- `tpatch feature deps --validate-all` — one-shot validation of the whole DAG (also runs as part of `init` sanity per PRD §6).

Tests:
- `TestFeatureDepsAdd_RejectsCycle` (full path in error)
- `TestFeatureDepsAdd_RejectsKindConflict`
- `TestFeatureDepsRemove_AtomicallyClearsAndRederivesDependents`
- `TestAmendDependsOn_ValidatedIdenticallyToFeatureDeps`
- `TestRemoveWithCascade_DeletesInReverseTopoOrder`
- `TestRemoveWithoutCascade_RefusesWhenDependentsExist`
- `TestRemoveForce_DoesNotBypassDepCheck`
- `TestRemoveCascadeNonTTY_RequiresForce`
- `TestFeatureDepsValidateAll_RunsOnInit`

#### Chunk D — Status-time DAG validation (~40 LOC)

Per PRD §6 + §10. `tpatch status` (with or without `--dag`) revalidates the DAG and surfaces any cycle/dangling/kind-conflict warnings inline. Currently validation only fires on dep writes.

Tests:
- `TestStatus_SurfacesDanglingDepWarning`
- `TestStatus_SurfacesCycleWarning`

#### Chunk E — 6-skill rollout (parity-guard coordinated, ~100 LOC of docs)

Update all 6 skill formats with:
- `dependencies` field documentation (analyze-phase bullet)
- Labels reference (`waiting-on-parent`, `blocked-by-parent`, `stale-parent-applied`)
- Compound verdict (`blocked-by-parent-and-needs-resolution`)
- `created_by` recipe field (now a real gate, not inert)
- `tpatch status --dag` mention
- **`tpatch feature deps add/remove` and `amend --depends-on`** usage examples
- **`tpatch remove --cascade`** + the `--force ≠ bypass` rule

Files (all 6 in lockstep):
- `assets/skills/claude/tessera-patch/SKILL.md`
- `assets/skills/copilot/tessera-patch.md`
- `assets/skills/copilot-prompt/tessera-patch.prompt.md`
- `assets/skills/cursor/tessera-patch.mdc`
- `assets/skills/windsurf/tessera-patch.md`
- `assets/skills/generic/tessera-patch.md`

`assets/assets_test.go` parity guard MUST pass after all 6 are updated.

Also: `docs/agent-as-provider.md` — if it covers reconcile-time agent behavior, add labels section.

#### Chunk F — `docs/dependencies.md` (~150 LOC)

User-facing reference doc:
- What dependencies are (hard vs soft)
- How to declare them (YAML examples + `tpatch feature deps add` examples)
- Validation rules (cycles, dangling, self-ref, kind conflict)
- Label semantics + matrix (lifted from PRD §3.5)
- Compound verdict explanation
- `created_by` apply-time gate behavior (with dry-run downgrade-to-W per PRD §4.3)
- `--cascade` and force semantics (PRD §3.7 — `--force` does NOT bypass dep integrity)
- `tpatch status --dag` examples (ASCII + `--json`)
- Migration note: existing v0.5.x projects keep working unchanged unless they add deps.

#### Chunk G — Release cutover

- Bump `version = "0.6.0"` in `internal/cli/cobra.go`.
- New `## 0.6.0 — 2026-MM-DD — Feature Dependencies (Tranche D)` section in `CHANGELOG.md` summarizing M14.1–M14.4 + correctness pass + C5 fix-pass.
- Update `docs/ROADMAP.md`: M14 ✅, Tranche D box closed.
- Tag `v0.6.0` AFTER push, AFTER full validation.

### Strict scope guards (DO NOT do)

- Do NOT skip the parity guard in Chunk C — all 6 skills move atomically.
- Do NOT add new external Go dependencies.
- Do NOT introduce `ReconcileWaitingOnParent` / `ReconcileBlockedByParent` enum values (still ADR-011 D3).
- Do NOT inject parent patches into the M12 resolver (ADR-011 D8 — deferred to v0.7).
- Do NOT add implement-phase heuristic inference of `created_by` (PRD §4.3.1 — separate backlog).
- Do NOT bypass DAG integrity with `--force` (ADR-011 D7 — explicit `--cascade` required).

### Validation gate

```
gofmt -l .
go build ./cmd/tpatch && rm -f tpatch
go test ./...
go test ./assets/...
go test ./internal/cli -run 'StatusDag' -count=1 -v
go test ./internal/workflow -run 'CreatedByGate|PlanReconcile|ComposeLabels|EffectiveOutcome|AcceptShadow|GoldenReconcile|Phase35|Labels' -count=1 -v
go test ./internal/store -run 'Label|Reconcile|DAG|Dependency|Roundtrip' -count=1 -v
```

All M14.1+M14.2+M14.3+correctness-pass tests stay green. Golden reconcile + manual accept regressions stay green.

### Workflow notes

- `tpatch` binary at root is NOT gitignored. After every `go build` run `rm -f tpatch` BEFORE staging. (Recurring slip.)
- Use `git -c commit.gpgsign=false` for commits. Each carries the trailer.
- `git push` takes 60+ seconds on this machine.
- 5–6 logical commits expected (one per chunk + version bump + CHANGELOG).
- Do NOT tag during the implementer's run. Tagging is the supervisor's final closeout action after reviewer APPROVES.

## Session Summary

M14.3 closed out. M14 correctness pass closed out (F1/F2/F3 from external reviewer). **C5 fix-pass complete** (re-review of correctness pass surfaced two real gaps): reconcile-time label suppression on retired outcomes and PRD-aligned dry-run downgrade for hard-parent created_by misses. Ready to dispatch M14.4 once C5 reviewer signs off.

### C5 fix-pass details

- **F1** (HIGH, was M14.4-blocking): `saveReconcileArtifacts` previously called `composeLabelsAt` which re-loaded the child status from disk. When a reconcile produced `ReconcileUpstreamed` via phase-1/2/3, the OLD on-disk outcome was used to compose labels — parent state would re-fire `waiting-on-parent`/`blocked-by-parent` and persist alongside the freshly-upstreamed verdict. Fix: gate label composition on `result.Outcome` (the in-memory truth) — retired outcomes force `Labels = nil` before any disk read. `updateFeatureState` propagates the same value. 4 new tests in `labels_reconcile_path_test.go` (one per phase + a non-upstreamed control).
- **F2** (MEDIUM, PRD alignment): hard-parent `created_by` + missing target now downgrades to a warning in dry-run (per PRD §4.3) while keeping the hard error in execute mode. `dryRunOperation` returns `(msg, warning, error)`; `RecipeExecResult` gains a `Warnings []string` slice; CLI dry-run gains `⚠` lines and a warning-count summary. Locked-in tests in `created_by_gate_test.go` updated to pin the new dry-run-vs-execute split. Recipe-shape validation errors (parent-not-in-depends_on, unknown kind) remain hard errors in both modes.

## Files Changed

C5 fix-pass:

- `internal/workflow/reconcile.go` — `saveReconcileArtifacts` short-circuits label composition for retired outcomes.
- `internal/workflow/recipe.go` — `dryRunOperation` returns `(msg, warning, err)`; `RecipeExecResult.Warnings` added; hard-parent created_by misses downgrade to W in dry-run.
- `internal/cli/cobra.go` — apply --dry-run renders warnings + summary line.
- `internal/workflow/labels_reconcile_path_test.go` — new (4 tests).
- `internal/workflow/created_by_gate_test.go` — split locked-in test, added dry-run-vs-execute parity assertion.

See `docs/handoff/HISTORY.md` for the M14.3 + correctness-pass entries.

## Test Results

C5 fix-pass: gofmt clean, `go test ./...` green, full validation gate green (workflow/store/cli targeted runs all pass, all M14.1/M14.2/M14.3/correctness-pass tests green including the adversarial source-truth and phase-3.5 skip tripwires).

### Status

- M14.1 ✅, M14.2 ✅, M14.3 ✅, correctness pass ✅, C5 fix-pass ✅ — all APPROVED, all on `origin/main`.
- M14.4 is the user-facing cutover (flag flip + dep-CLI + skill rollout + v0.6.0 tag) — paused awaiting user green-light.

## Next Steps

1. **Wait for user approval** to dispatch M14.4.
2. On green-light: dispatch `m14-4-implementer` against this expanded 8-chunk handoff (A status --dag, B flag flip, C dep-management CLI, D status-time validation, E 6-skill rollout, F docs/dependencies.md, G release cutover).
3. After M14.4 implementer: `m14-4-reviewer` (expect a beefy review — dep-CLI surface alone is ~250 LOC + 9 tests).
4. On APPROVED: supervisor bumps version, updates CHANGELOG, ROADMAP, archives this handoff, tags `v0.6.0`, pushes.

## Blockers

None. M14 is ready for the user-facing cutover as soon as user authorizes.

## Context for Next Agent

- M14.1+M14.2+M14.3+correctness-pass are all flag-protected. Flipping the flag default in Chunk B is the load-bearing change.
- The PRD §3.4 has residual terminology drift treating labels as enum verdicts. Defer to ADR-011 D6 + PRD §4.5.
- External-reviewer guard: any DAG/label code reads `status.Reconcile.Outcome`, NEVER `artifacts/reconcile-session.json`.
- `created_by` is now a live gate (not inert). The implement-phase auto-inference heuristic from PRD §4.3.1 is separate backlog — authors set the field manually or via skill examples.
