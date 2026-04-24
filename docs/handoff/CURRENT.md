# Current Handoff

## Active Task

- **Task ID**: Tranche C3 / v0.5.3 — shadow accept accounting fixes (**IN FLIGHT**)
- **Status**: 🔨 Implementation sub-agent dispatched
- **Blocks**: M14.1 — M14.3 reads `status.Reconcile.Outcome` for ADR-011 D6 label composition; must be accurate before DAG work builds on top
- **Previous**: Tranche C2 / v0.5.2 shipped ✅ — archived in `HISTORY.md`

### C3 scope — external reviewer surfaced 3 follow-ups on v0.5.2 shadow flow

All verified by code inspection:

| ID | Severity | Finding |
|---|---|---|
| c3-separate-resolution-artifact | 🔴 Silent correctness (manual-accept regression) | Resolver writes `ResolveResult` (with `outcomes[]`) to `artifacts/reconcile-session.json`; reconcile.go:398 `saveReconcileArtifacts` overwrites with `ReconcileResult` (no outcomes); `loadResolvedFiles` reads outcomes → errors "no resolved files recorded". Fix: split into `resolution-session.json` (resolver) + `reconcile-session.json` (reconcile summary) |
| c3-manual-accept-regression-test | 🟡 Missing coverage | End-to-end shadow-awaiting → manual accept test. Counterpart to `TestGoldenReconcile_ResolveApplyTruthful` but for the manual path. Would have caught both other C3 findings in v0.5.2 |
| c3-accept-stamps-reconcile-outcome | 🟡 Internal consistency (M14.3 blocker) | `AcceptShadow` marks `State=applied` but leaves `Reconcile.Outcome=shadow-awaiting`. M14.3 label composition (ADR-011 D6) reads `Reconcile.Outcome` — stale outcome → wrong DAG labels |

### Why before M14.1

1. C3.1 is a regression we introduced in v0.5.2. `AcceptShadow` unified the file-copy path but broke the I/O contract. Manual recovery (`reconcile --accept <slug>` after shadow-awaiting) errors out today.
2. C3.3 directly blocks correct M14.3 label computation. ADR-011 D6: "Child's intrinsic reconcile verdict is always computed first; parent labels overlay on top." That computation reads `Reconcile.Outcome`. Building DAG on top of stale outcomes = wrong labels.
3. Same architectural zone as C2 (`accept.go` / `reconcile.go` / `resolver.go`). Cheap now; painful mid-M14.3.

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
