# ADR-011 — Feature Dependency DAG

**Status**: Accepted (PRD approved — `docs/prds/PRD-feature-dependencies.md`)
**Date**: 2026-04-23
**Deciders**: Core (supervisor + PRD author sub-agent + rubber-duck reviewer, 3 revision cycles)
**Supersedes**: n/a
**Related**: `feat-feature-dependencies`, `docs/prds/PRD-feature-dependencies.md`, SPEC §Features, ADR-010 (M12 resolver)

## Context

Through v0.5.1 every feature in a tpatch repository is independent: its slug, patch, recipe and reconcile state have no relationship to any other feature. Users building non-trivial patch sets have two options, both unsatisfactory:

1. **Collapse everything into one feature** — loses the ability to land parts upstream piecemeal, produces giant recipes, and makes reconcile verdicts coarse.
2. **Keep features independent and rely on ordering discipline** — works until the user reconciles upstream; then parent-dependent children produce false `3WayConflicts` because they were built against a parent's intermediate tree state that no longer exists.

The PRD authoring cycle (v1 → v2 → v3, 3 rubber-duck passes) converged on a **stacked dependency DAG** where features declare `depends_on` relationships with hard (`requires`) or soft (`after`) semantics. Before landing ~1350 LOC of implementation across M14.1–M14.4 we lock the load-bearing architectural decisions here so sub-agent implementation runs don't re-litigate settled points.

The decisions below are the cross-cutting invariants. Operational details (exact JSON schemas, CLI flag surfaces, skill bullet wording) remain in the PRD — this ADR governs only choices that would be painful to reverse once code ships.

## Decision

### D1. Dependencies live in `status.json` only

No new `feature.yaml` schema field. No migration. `depends_on` is written into `.tpatch/features/<slug>/status.json` by the author's edit or by `tpatch define --depends-on <slug>[:hard|soft]`.

**Why**: Duplicating dependency declaration across yaml and json creates a dual-source-of-truth footgun — the two drift, and the store has to arbitrate. The PRD v1 proposed a yaml-primary model; the rubber-duck pass flagged it, v2 collapsed to status.json. Status-only is also consistent with existing state machine artifacts (apply_meta, reconcile_meta, labels) that all live in status.json.

**Cost**: Dependencies aren't visible in a plain `cat feature.yaml`. Mitigated by `tpatch status <slug>` always rendering parents + dependents.

### D2. DFS for cycle detection, Kahn's algorithm for operator traversal

The two problems use different algorithms deliberately:

- **Validation (write-time)**: `AddDependency` runs a depth-first search from the new edge's tail; if it can reach the head, reject with `ErrCyclicDependency` listing the full cycle. DFS gives us the path for free, which produces actionable error messages.
- **Planning (read-time)**: `tpatch reconcile` and `tpatch status --dag` use Kahn's algorithm to produce a deterministic topological order. Kahn's is chosen over DFS-postorder because the priority queue tiebreaker (lexicographic slug order) is trivial to express in Kahn's but awkward in DFS.

**Why not one algorithm for both**: DFS topo is fine if the graph is valid, but we need cycle detection *as a validation rule* with good error messages, and we need stable ordering for operator UX. One implementation can't serve both well.

### D3. `waiting-on-parent` and `blocked-by-parent` are composable derived labels, not states

A feature's `state` field stays in `{analyzed, defined, explored, implemented, applied, recorded, reconciled}`. Derived labels are computed from the DAG + parent states on every render and **may coexist**. Example: an `applied` feature whose parent is now `blocked-reconcile` gets labels `[blocked-by-parent]`; if the parent went `reconciled-needs-apply` it gets `[waiting-on-parent]`; if the parent is unknown (removed without cascade) it gets `[blocked-by-parent, stale-parent-gone]`.

**Why**: PRD v2 made them mutually-exclusive states. The rubber-duck pass showed the matrix blows up (8 parent states × 7 child states × 2 dep types) and produces unrepresentable combinations. Composable labels collapse the matrix to "compute each label independently, render all that apply."

**Rendering order** (locked): `[blocked-by-parent] [waiting-on-parent] [stale-parent-*]` — most-severe first, so operator scanning a status list sees the worst problem at the left margin.

### D4. Hard deps gate apply AND `created_by`; soft deps gate neither

| Action | Hard parent unmet | Soft parent unmet |
|---|---|---|
| `tpatch apply` (execute) | Refuse (`ErrBlockedByParent`) | Warn, proceed |
| Recipe op with `created_by: <parent-slug>` | Refuse (integrity violation) | N/A — soft deps can't carry `created_by` |
| `tpatch reconcile` | Planner schedules parent first; child waits | Planner schedules parent first; child proceeds if parent clean |

**Why**: `created_by` is a structural integrity field — it says "this file was introduced by `<parent>`'s patch; if the parent disappeared, the file shouldn't exist." That guarantee can't hold under soft semantics (soft parents can be absent by definition), so the field is only meaningful for hard parents. Apply is gated symmetrically.

### D5. `upstream_merged` satisfies hard dependencies

When a parent feature's state is `upstream_merged` (set by reconcile phase-3 obsolescence check), it counts as satisfied for all dependent features. The parent can be removed or archived without cascading delete of children.

**Why**: The whole point of soft-landing upstream is that your local feature stack becomes a smaller local feature stack. Children of an upstreamed parent lose their dependency link naturally — their `created_by` files are now part of upstream, not owed to a local parent.

**Cost**: Children referencing `<parent-slug>` by name will have a dangling reference after the parent is removed. Mitigation: `status.json.depends_on[].satisfied_by: "upstream_merged@<sha>"` is written at obsolescence time so the dependency edge carries its own provenance.

### D6. Reconcile verdict composition rules

Child's intrinsic reconcile verdict is **always computed first**. Parent-derived labels overlay on top. Specifically:

1. Run child through phases 1–4 independently (using its own patch baseline).
2. If child's intrinsic verdict is `Reapplied` or `NoOp`, apply parent labels as-is.
3. If child's intrinsic verdict is `3WayConflicts` AND parent is `blocked-*`, emit compound verdict `blocked-by-parent-and-needs-resolution` and **skip phase 3.5** (provider resolver). Running the resolver against a broken parent would poison the shadow worktree with parent drift.
4. If child's intrinsic verdict is `Obsolete` (upstream_merged), parent labels are **suppressed** — the child is going away, parent state is irrelevant.

**Why**: Parent state should never *mask* a child's own problem. An engineer seeing `waiting-on-parent` on a child with internal conflicts would incorrectly assume "fix the parent and this resolves itself." Intrinsic verdict first, parent labels second.

### D7. `remove --cascade` is the only way to delete a feature with dependents

```
$ tpatch remove parent-slug
Error: feature 'parent-slug' has 2 dependents: [child-a, child-b]
       Use --cascade to remove them in reverse-topological order, or
       detach dependents first with `tpatch amend <child> --remove-dep parent-slug`.

$ tpatch remove parent-slug --cascade
Removing in order: child-b → child-a → parent-slug ... done.
```

`--force` **does not** bypass dep integrity — it only skips the TTY confirmation prompt. Operators who explicitly want destructive removal must combine them: `--cascade --force` (CI use case).

**Why**: `--force` is currently understood as "I know what I'm doing, don't prompt me." Overloading it to mean "also ignore dep integrity" merges two distinct confirmations into one. Keeping them orthogonal makes both safer and explicit.

### D8. No parent-patch context to M12 resolver in v0.6

When `tpatch reconcile --resolve` runs on a child feature whose parent has `3WayConflicts`, the resolver is **not** given the parent's patch as additional context. The child is resolved in isolation against its own shadow worktree.

**Why**: Cross-feature context injection into the resolver is a distinct feature (`feat-resolver-dag-context`) with its own PRD-worthy surface — prompt size impact, token budget, response parsing, attribution of conflicts to parent vs child. v0.6 ships the DAG itself; resolver integration is a v0.6.x follow-up. This keeps M14 scope inside the ~1350 LOC envelope.

**Cost**: Users with conflicts that span a parent-child boundary will get two rounds of resolution (parent, then child) instead of one holistic pass. Acceptable for v0.6; tracked as `feat-resolver-dag-context`.

### D9. Guarded by `features.dependencies: false` until v0.6.0 flip

The entire DAG code path — status.json new fields, CLI flags, reconcile planner changes — is gated by a config flag that defaults false until M14.4 lands. Any `depends_on` read on a repo with the flag off is an error; writes are refused. This lets M14.1 and M14.2 ship in trunk without exposing half-implemented behavior.

**Why**: Single atomic v0.6.0 flip avoids "it half works on v0.5.2" support questions. Users who want to preview can opt in by flipping the flag; the test suite does so for the DAG tests.

**Cost**: The flag check is a ~20-line cross-cutting concern. Mitigated by a single `features.Dependencies(cfg)` helper that all callers use — no ad-hoc config access.

## Consequences

**Positive**

- Dependency graph has one storage site, one cycle detection algorithm, one traversal algorithm — no architectural splits to debug later.
- Composable labels scale: future parent-derived signals (e.g. `stale-parent-amended-since-apply`) slot in without schema changes.
- `created_by` + hard-only gating makes recipe integrity mechanically enforceable rather than convention.
- Flag-gated rollout lets the 4 sub-milestones ship independently to trunk without a long-lived feature branch.

**Negative**

- Dependencies invisible in `feature.yaml` alone — tooling must read status.json.
- Operator learning curve: hard vs soft, labels vs states, `--cascade` vs `--force`. Mitigated by `docs/dependencies.md` (M14.4) and skill bullet in analyze phase.
- 6-skill parity-guard rollout in M14.2 for the `created_by` field requires coordinating 7 edits in one commit (6 skills + `docs/agent-as-provider.md` + parity test update). Standard pattern, but bigger than usual.

**Neutral**

- Parent-patch context for resolver (D8) deferred — explicit scope decision documented as `feat-resolver-dag-context`.
- Auto-rebase-on-parent-drift deferred — `feat-feature-autorebase`.
- Per-dependency version ranges (`depends_on: slug@>=v2`) deferred indefinitely; v0.6 has only bare slug references.

## Terminology normalization (addresses PRD §3.4 drift)

The PRD §3.4 lists `ReconcileWaitingOnParent` and `ReconcileBlockedByParent` as reconcile verdicts. §4.5 of the same PRD treats them as composable labels. **This ADR locks the label interpretation** (D3): they are NOT verdicts, they are labels that overlay verdicts. M14.3 implementers should read the PRD §4.5 semantics, not §3.4 enum form. The reviewer flagged this as non-blocking; it is normalized here.

## Alternatives considered

1. **Dual yaml + json storage** (PRD v1) — rejected per D1.
2. **Mutually-exclusive states** (PRD v2) — rejected per D3.
3. **`--orphan-soft` auto-downgrade** (PRD v2) — deferred to `feat-orphan-soft-downgrade` in v9 future work. Too much policy surface for v0.6.
4. **Parent-patch to resolver** — deferred to `feat-resolver-dag-context` per D8.
5. **Graph in a separate `depends.json` file** — rejected; one more file to keep in sync with status.json for no storage win.

## References

- `docs/prds/PRD-feature-dependencies.md` — operational detail
- PRD review trajectory in `docs/supervisor/LOG.md` (3 cycles, APPROVED WITH NOTES)
- ADR-010 — M12 resolver architecture (parent-patch context deferred from this ADR interacts with ADR-010)
