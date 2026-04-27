> **STATUS: SUPERSEDED — 2026-04-27.** This document captures the originally
> approved Wave 3 design (lifecycle-state model: `tested` as a forward state on
> the `FeatureState` enum). An external re-review (see `docs/supervisor/LOG.md`
> 2026-04-27 reopening entry) found that the design conflated lifecycle and
> verification freshness, and routed read-time mutation through `LoadFeatureStatus`.
> The supervisor reopened Wave 3 with a binding redesign: a Git-like freshness
> overlay model.
>
> - **Successor PRD**: [`docs/prds/PRD-verify-freshness.md`](../prds/PRD-verify-freshness.md)
> - **Successor ADR**: [`docs/adrs/ADR-013-verify-freshness-overlay.md`](ADR-013-verify-freshness-overlay.md)
>
> This document is preserved as historical record only. **Do not implement
> against it.**

---

# ADR-012 — `tested` Feature State + `tpatch verify` Command

**Status**: Accepted (PRD approved — `docs/prds/PRD-verify-and-tested-state.md`)
**Date**: 2026-04-27
**Deciders**: Core (M15 Wave 3 design dispatch)
**Supersedes**: n/a
**Related**: ADR-010 (M12 resolver), ADR-011 (feature DAG), `docs/prds/PRD-verify-and-tested-state.md`, `docs/prds/PRD-feature-dependencies.md`, `docs/dependencies.md`, CHANGELOG v0.6.1

## Context

v0.6.1 closed M15 Wave 1 + Wave 2 with no new lifecycle states and no new read-only verbs. Wave 3 picks up two backlog items the v0.6.1 closeout flagged as deferred (CHANGELOG v0.6.1 → "Out of scope for v0.6.1"):

- `tpatch verify <slug>` — cheap, machine-checkable health command.
- `tested` lifecycle state — persistent answer to "verify passed against the current world."

The reviewer go-to-tag note required clarifying how the two relate before scoping either; the resulting combined PRD (`PRD-verify-and-tested-state.md`) lays out the surface in detail. This ADR locks the architectural decisions that would be painful to reverse once code lands. Operational details (exact JSON shapes, flag set, slice boundaries) live in the PRD; this ADR governs only the load-bearing invariants.

ADR-011 (feature DAG) and ADR-010 (provider-assisted resolver) are both **binding context**: this ADR does not amend either, and it explicitly preserves ADR-011 D6 (`Reconcile.Outcome` source-truth guard) and ADR-010 D5 (artifact ownership split).

## Decision

### D1. `tested` is a sibling state of `applied` in `FeatureState`, linear

`tested` joins `FeatureState` between `applied` and `active`:

```
requested → analyzed → defined → implementing → applied → tested → active
                                                  ↑          ↓
                                                  └──────────┘  (verify FAIL / amend / re-apply / parent regression)
```

It is **not** a branching state and **not** a parallel-track state. `tested` is reached only from `applied` (forward) and decays only into `applied` (backward); `tested → active` is reached the same way `applied → active` is today (operator action, unchanged). `active`, `blocked`, `upstream_merged`, `reconciling`, `reconciling-shadow` are unrelated to `tested`.

**Why linear, not branching**: a branching design (`applied + tested-flag`) would force every consumer of `state` to read two fields. A separate `tested_at` timestamp would do the same. Linear keeps the state machine readable and keeps `state` a single string at every read site.

**Cost**: the v0.6.1 enum gains a value. Documented as an additive, omitempty-safe extension (D4). All existing v0.6.1 fixtures round-trip byte-identical because no v0.6.1 path writes `tested`.

**Why between `applied` and `active`**: `active` is operator-promoted ("I'm maintaining this feature"), `tested` is verify-promoted ("the feature's structural invariants hold"). Putting `tested` after `applied` matches the lifecycle reading order and keeps `active` as the terminal-stamp interpretation.

### D2. `tested` satisfies hard dependencies — pragmatic equivalence to `applied`

The most consequential decision the PRD called out. Both directions argued in PRD §3.4.4; this ADR locks the answer.

**Decided: `tested` satisfies the hard-dep gate, equivalent to `applied`.**

Specifically, `workflow.CheckDependencyGate` (`internal/workflow/dependency_gate.go:79`) gains `case StateTested:` in the same arm as `case StateApplied:`. The satisfaction set becomes `{applied, tested, upstream_merged}`. No new logic; one-line change.

**Why direction A (yes) over direction B (no)**:

- `tested` is by construction a strict superset of `applied`'s structural guarantees. Every check that fires at apply time also fires at verify time (recipe parses, deps valid, satisfied_by reachable, recipe replay clean). A child that can apply on top of an `applied` parent can apply on top of a `tested` parent — the parent has only become more, not less, healthy.
- Mirrors operator intuition. The harness scenario "`verify parent && apply child`" should not require a redundant `apply parent` step.
- Symmetric with ADR-011 D5 (`upstream_merged` satisfies hard dependencies): if the parent's changes "live" anywhere reachable, the child can apply.

**Why not direction B (no)**:

- The argument against was the dynamic-vs-sticky asymmetry (`tested` can demote silently; `applied` is sticky). The mitigation is D5: `tested → applied` demotion does **not** cascade to children. A child applied while its parent was `tested` continues to satisfy its gate after the parent demotes to `applied` — because `applied` itself satisfies the gate. There is never a moment where a child sees an unsatisfied gate from a parent regression alone (the parent is at worst `applied`).

**Deliberately rejected alternative (Alt 3)**: a stricter form of dep satisfaction where children of a `tested` parent gain a `parent-tested` derived label. Rejected because it would extend the M14.3 label vocabulary (`waiting-on-parent`, `blocked-by-parent`, `stale-parent-applied`) for no operator-actionable benefit, and would re-open ADR-011 D3. If a real harness need surfaces, revisit in a future `feat-tested-cascade-labels` PRD.

**Cost**: the gate now treats two distinct states identically. The blocker-message text (`workflow/dependency_gate.go:111`) does not need to distinguish `applied` vs `tested` — the gate is satisfied either way; only the un-satisfied case produces the message.

### D3. `verify` is the unique producer of `tested`

Only `tpatch verify <slug>` writes `state = tested`. `tpatch test`, `tpatch amend`, `tpatch apply`, and `tpatch reconcile` never produce it.

**Why verify-only**:

- `tested` is a machine-checkable claim. `tpatch test` runs a user-configurable `Config.TestCommand` whose contract on side effects is loose; making it a producer of `tested` would conflate "the user's tests passed" with "tpatch's invariants hold." A harness can run both verbs and decide.
- A manual flip (`amend --state tested`) is rejected: `tested` is the kind of claim that should not be assertable by hand. ADR-012 explicitly does not add a flag for this.
- Implicit `verify` after every `apply` is rejected: it inflates the apply latency for a benefit the harness can opt into via `apply && verify`.

**Cost**: a v0.6.1 user who wants `tested` must learn one new verb. CHANGELOG and the skill bullet (PRD §4.4) cover the discoverability.

**Future-work expansion**: if a real harness need surfaces for `tpatch test` to also produce `tested` (e.g., "verify + tests-green = tested"), revisit in `feat-tested-state-test-producer`. Out of Wave 3.

### D4. Backwards-compatibility contract — byte-identical round-trip for v0.6.1 repos

A v0.6.1 repo that never runs `verify` must round-trip every `status.json` byte-identically through v0.6.2 read/write paths. Locked by:

- The `FeatureState` enum extension is additive. No field shape change on `FeatureStatus`, no new field, no new file.
- `StateTested` is written **only** by `verify`. Every existing write site (`apply`, `amend`, `reconcile`) is unchanged at the state-write level; the only edit is in `apply`/`amend`/`reconcile` paths that *demote* `tested → applied` — these are no-ops when the prior state is anything other than `tested`.
- `Reconcile.Labels` vocabulary is unchanged (no `tested-by-parent` label per D2 alt 3 rejection).
- No new field on `Config`. No new key in `.tpatch/config.yaml`.

Enforced by a regression fixture: `TestUpgradeFromV0_6_1_NoVerify_BehavesIdentically` — load v0.6.1 fixture, run every v0.6.1 command except `verify`, diff `.tpatch/` against the v0.6.1 expected output, fail on any byte difference.

**Cost**: the contract is strict — any future change that touches a state-write path must include this regression fixture in its acceptance criteria.

### D5. State-transition table — allowed and disallowed edges

Verify-driven edges (the only edges this ADR adds):

| Trigger | From | To |
|---------|------|-----|
| `verify` PASS | `applied` | `tested` |
| `verify` PASS | `tested` | `tested` (idempotent, bumps `UpdatedAt`) |
| `verify` PASS | `active` | `active` (unchanged — verify on `active` runs read-only and does **not** flip; D5 carve-out) |
| `verify` PASS | `blocked` / `upstream_merged` | unchanged (verify reports but does not flip) |
| `verify` FAIL (block-severity) | `tested` | `applied` |
| `verify` FAIL (block-severity) | `applied` / `active` | unchanged (warn-equivalent — block-severity fail produces non-zero exit but no state mutation outside the `tested → applied` demotion) |
| `verify` FAIL (warn-only) | any | unchanged |

Demotion edges driven by other verbs (cross-cutting; D5 makes them explicit):

| Trigger | From | To |
|---------|------|-----|
| `apply --mode execute` | `tested` | `applied` |
| `amend` (recipe-touching: `apply-recipe.json` or `artifacts/post-apply.patch` rewritten) | `tested` | `applied` |
| `amend` (intent-only: `request.md` / `spec.md` only) | `tested` | `tested` (preserved) |
| `reconcile` PASS (`reapplied` / `upstreamed`) | `tested` | `applied` (NOT `tested` — reconcile mutates the upstream baseline; prior verify is no longer guaranteed) |
| `reconcile` FAIL (`blocked-*`) | `tested` | `blocked` (per existing ADR-010 path) |
| Hard parent flips out of `{applied, tested, upstream_merged}` (parent-state hook) | `tested` (children) | `applied` (cascade depth = 1 only) |

Disallowed edges (rejected at runtime with `exit 2 — invalid state for verify` or equivalent):

- `verify` on `requested` / `analyzed` / `defined` / `implementing` / `reconciling` / `reconciling-shadow`. Refused with no checks run.
- `amend --state tested`. Refused; no flag is added to `amend` for this purpose (D3).
- `verify` while a reconcile is in flight on the same slug (state `reconciling` / `reconciling-shadow`). Refused per per-slug shadow lock (D7).

**Why this set**: every edge corresponds to either (a) a verify-driven state-write or (b) an event whose semantics invalidate the prior verify result. No edge produces `tested` other than verify itself (D3 invariant).

**Why one-step parent-state cascade**: a transitive cascade is unnecessary because the demotion target (`applied`) itself satisfies the dep gate (D2). Grandchildren see no change in their parent's gate-satisfaction status; their own `tested` claim is still backed by their direct parent's `applied` state.

### D6. Source-truth alignment with ADR-011 D6 / ADR-010 D5

`tested` is persisted in `status.json` and **never inferred from any other artifact**. Reusing ADR-011 D6 wording verbatim:

> Authoritative source for derived reconcile decisions: read `status.Reconcile.Outcome` via `store.LoadFeatureStatus` — never read `artifacts/reconcile-session.json` for DAG decisions. The session artifact is an audit record of one RunReconcile invocation; status.json is the source of current truth post-accept (see ADR-010 D5).

Applied to `tested`:

- `tpatch verify` reads `status.Reconcile.Outcome` (in V9 of the check list) — **never** `artifacts/reconcile-session.json` or `artifacts/resolution-session.json`.
- The parent-state hook (D5) reads `parent.State` via `store.LoadFeatureStatus` — never any reconcile artifact.
- The `tested` flip is written only to `status.json`. There is no `verify-session.json`, no new file in `artifacts/`, no new entry in `patches/`. (Pass/fail per check is in the `--json` report on stdout; the persisted state is the `tested` flip alone.)

Enforced by an adversarial test: the verify implementation must not import `artifacts/reconcile-session.json` or `artifacts/resolution-session.json` at any code path; any reference to those paths in `internal/workflow/verify.go` is a test failure.

**Why explicit**: ADR-011 D6 was specifically introduced to prevent a reviewer-flagged drift where DAG decisions might read the audit log instead of the canonical state. The same drift is possible for `verify`'s V9 check (reconcile-outcome consistency). Pinning the rule here forecloses it.

### D7. `verify` is read-only on the working tree; shadow workspace for apply-simulation

Verify mutates **only** `status.json` (the `tested` flip). Apply-simulation checks (V7 — `recipe_replay_clean`) operate inside an existing `gitutil.CreateShadow` worktree (`internal/gitutil/shadow.go:56`) rooted at the upstream baseline; the shadow is pruned via `defer PruneShadow(...)` before verify exits, regardless of pass/fail.

**Why shadow, not real tree**: verify must be safely runnable against a feature whose recipe might have side effects (file creation, content rewrites). Spinning up a shadow is the existing primitive for "let me apply something temporarily and discard the result" (ADR-010 D2). Reusing the existing plumbing means no new lifecycle to maintain.

**Per-slug lock**: verify and reconcile both write to `.tpatch/shadow/<slug>-<timestamp>/`. To prevent two concurrent writers, `verify` refuses when `state ∈ {reconciling, reconciling-shadow}`. The verify shadow is created with the same naming convention; any prior verify shadow is reaped via `gitutil.PruneAllShadows` (existing semantics) at verify start.

**Cost**: V7 requires a shadow allocation per verify. For features with no recipe, V7 is skipped (no allocation). For features with a recipe, the cost is on the order of a `git worktree add` + recipe replay; comparable to a phase-2 reconcile op-replay pass and well within the "cheap" budget the verify command is supposed to deliver.

**Why not a real-tree dry-run**: a recipe replay against the real tree would mutate working copy files (or produce false negatives if the working tree is dirty). Shadow is the only correct option here.

## Consequences

**Positive**

- Single-string `state` field continues to answer "where is this feature in its lifecycle." `tested` is just a new value; no consumer needs to read two fields.
- `verify` is a cheap, deterministic, offline command that closes the gap between `apply` and `reconcile` for harness handoff scenarios.
- D2 (pragmatic equivalence) means the dep gate has the same operator semantics as v0.6.1 plus one new accepted state — no surprises for existing dep-graph users.
- D6 source-truth guard preserves ADR-011 D6 / ADR-010 D5 invariants verbatim; no reviewer-flagged drift introduced.
- D7 reuses the existing shadow primitive; no new lifecycle to maintain.

**Negative / accepted trade-offs**

- The `FeatureState` enum gains a value; downstream JSON consumers that hard-code the v0.6.1 enum set need to update. Mitigated by the existing `tpatch status --json` enum-by-string contract (consumers that already handle unknowns gracefully are unaffected).
- The `tested → applied` demotion is silent in the sense that no provider call is made, but the operator may be surprised when a `tested` feature becomes `applied` after upstream change. Mitigated by stderr logging on every demotion and by the `stale-parent-applied` label (M14.3) which is the natural visual cue.
- Verify on `active` does not flip to `tested` (D5 carve-out). Operators wanting a `tested` claim on an `active` feature must temporarily demote (`amend` recipe-touching → `applied`) and re-verify, or accept that `active` is the stronger label. Documented as "active is operator-owned, tested is verify-owned" in PRD §3.4.2.
- Skill rollout (Slice D) requires updating 6 surfaces atomically; standard pattern, but adds coordination cost similar to M14.2's `created_by` rollout.

**Neutral**

- `tpatch test` integration deferred (D3 future-work expansion).
- `tpatch verify --all` deferred to Slice D (PRD §9).
- Fresh-branch verify mode (`--fresh-branch`) explicitly out of scope (PRD §0.3 + §9 future work). Belongs to a distinct `feat-reconcile-fresh-branch-mode` PRD.
- Recipe-op JSON schema unchanged. Verify tolerates deletions in shadow replay the same way recipe autogen does (skip with stderr note); a real `delete-file` op needs its own ADR (CHANGELOG v0.6.1 Notes).

## Alternatives considered

1. **Branching state machine** (`applied + tested-flag`) — rejected per D1. Forces every consumer of `state` to read two fields.
2. **`tested` does NOT satisfy the hard-dep gate** (D2 direction B-strict) — rejected. UX-hostile (operator sees "parent is tested, apply requires applied"); produces a confusing failure mode where a strictly stronger state is treated as weaker.
3. **`tested` produces a `parent-tested` derived label on children** (D2 alt 3) — rejected. Bloats the M14.3 label vocabulary for no operator-actionable benefit.
4. **Manual flip via `amend --state tested`** (D3 alt B) — rejected. `tested` should be a machine-checkable claim, not assertable by hand.
5. **Implicit `verify` after every `apply`** (D3 alt C) — rejected. Inflates apply latency for a benefit the harness can opt into via explicit chaining.
6. **`tpatch test` as a producer of `tested`** (D3 alt A) — deferred to `feat-tested-state-test-producer`. Conflates user-test-suite-pass with tpatch-invariants-hold.
7. **Transitive parent-state cascade** (D5 alt) — rejected. Demotion target (`applied`) itself satisfies the dep gate; grandchildren see no change in gate-satisfaction.
8. **A new `verify-session.json` artifact** — rejected. Verify writes only `status.json`; pass/fail per check is in the `--json` report, not persisted. Smaller surface, no new artifact lifecycle.
9. **V9 (`reconcile_outcome_consistent`) as block severity** — left as PRD open question Q1; default warn. If reviewer pushes for block, escalate to ADR amendment before Slice C.
10. **`--shadow` flag on verify** — rejected. V7 already gates shadow allocation on recipe presence; an explicit flag is unnecessary.
11. **`--fresh-branch` flag on verify** — out of scope (PRD §0.3). Belongs to a distinct PRD.

## References

- `docs/prds/PRD-verify-and-tested-state.md` — operational detail
- `docs/adrs/ADR-011-feature-dependencies.md` — feature DAG contract; this ADR preserves D3 (composable labels), D6 (source-truth guard), D9 (config gate)
- `docs/adrs/ADR-010-provider-conflict-resolver.md` — shadow worktree contract (D2), artifact ownership split (D5)
- `docs/dependencies.md` — user-facing dep reference; will gain a one-paragraph cross-link to verify in Slice D
- CHANGELOG v0.6.1 → "Out of scope for v0.6.1" — names verify and tested as the two deferred items this PRD addresses
- `internal/store/types.go` — `FeatureState` enum site
- `internal/store/validation.go:38–44, 101–108` — `satisfiedBySHA` regex + `gitutil.IsAncestor` reachability (v0.6.1 contract reused by V5)
- `internal/workflow/dependency_gate.go:79` — `CheckDependencyGate` switch site (D2 implementation point)
- `internal/gitutil/shadow.go:56` — `CreateShadow` (D7 reuse point)
- `internal/gitutil/gitutil.go:680` — `IsAncestor` (V5 reuse point)
