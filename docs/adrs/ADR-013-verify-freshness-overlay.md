# ADR-013 — Verify Freshness Overlay

**Status**: Accepted (M15 Wave 3 design — Git-like freshness redesign; PRD: `docs/prds/PRD-verify-freshness.md`)
**Date**: 2026-04-27
**Deciders**: Core (M15 Wave 3 design — second revision after re-review)
**Supersedes**: ADR-012 (in full — every D1–D7 either replaced, retained, or dropped; see the supersession map below). The first-revision design (commit `8c3d72e`) extended `FeatureState` with a `tested` value; that approach is abandoned. The re-review of `8c3d72e` (findings F1, F2, F3, F4) is the trigger.
**Related**: ADR-010 (M12 resolver), ADR-011 (feature DAG), `docs/prds/PRD-verify-freshness.md` (successor PRD), `docs/prds/PRD-verify-and-tested-state.md` (superseded predecessor PRD), `docs/dependencies.md`, CHANGELOG v0.6.1

## Context

v0.6.1 closed M15 Wave 1 + Wave 2 with no new lifecycle states and no new read-only verbs. Wave 3 picks up two backlog items:

- `tpatch verify <slug>` — cheap, machine-checkable health command.
- A persistent answer to "verify last passed against the current world."

The first-revision design (approved on `8c3d72e`) modelled the second item as a new `tested` value on the `FeatureState` enum. An external re-review surfaced two structural problems:

- **F1**: V7/V8 shadow replay ignored the target's hard-parent closure, so verify was structurally meaningless for any non-leaf feature whose parent was locally `applied`.
- **F4**: The design conflated lifecycle (sticky, write-by-explicit-verb) with verification freshness (drift-sensitive). It routed a "parent-state hook" through `LoadFeatureStatus`, a read path, which would have meant a read command silently mutates `.tpatch/`.

The supervisor's binding adjudication: redesign with **Git-like semantics**. Lifecycle is the lifecycle (commits — sticky). Freshness is `git status` for the verify check (derived, read-time). This ADR locks the seven decisions of the rewrite. Operational details (exact JSON shapes, flag set, slice boundaries) live in the PRD; this ADR governs the load-bearing invariants.

ADR-011 (feature DAG) and ADR-010 (provider-assisted resolver) remain **binding context**: this ADR does not amend either, and it explicitly preserves ADR-011 D6 (`Reconcile.Outcome` source-truth guard) and ADR-010 D5 (artifact ownership split).

## Supersession map (relative to ADR-012)

This ADR supersedes ADR-012 in full. The mapping of each prior decision:

| Old decision | Disposition | New decision (this ADR) | Why |
|---|---|---|---|
| **D1** — `tested` as a `FeatureState` enum value (linear forward from `applied`) | **REPLACED** | D1 — verify produces a `Verify` sub-record on `FeatureStatus`; `FeatureState` enum is unchanged | Lifecycle and freshness are separate concerns. Encoding "verify last passed" as a sticky lifecycle state forced read paths to mutate state when parents drifted. The freshness overlay is derived at read time and only the explicit `verify` write verb persists anything. |
| **D2** — `tested` satisfies the hard-dep gate (equivalent to `applied`) | **DROPPED** (question is moot) | D2 — apply gate is pure-lifecycle; satisfaction set remains `{applied, upstream_merged}`; freshness is ignored by the gate | There is no `tested` lifecycle state to satisfy. Freshness is an operator/harness signal, not a gate input. Folding it into the gate would re-introduce the demote-on-read pathology from a different angle. |
| **D3** — producer set: only `verify` writes `tested`; `test` does not; `amend` does not | **REPLACED** (mostly retained) | D3 — `verify` writes the `Verify` sub-record; `amend` invalidates by clearing it; `test` does not write | Same producer-purity intent, but the artefact produced is a freshness record, not a lifecycle transition. `amend` must clear, not preserve, because amend touches recipe/patch hashes. |
| **D4** — `omitempty` round-trip for byte-identical v0.6.1 status.json | **RETAINED** | D4 — the new `Verify` sub-record carries `omitempty` on every nested field; v0.6.1 repos round-trip byte-identical until verify runs once | Backwards-compat invariant unchanged. |
| **D5** — forward/backward state-transition table (`applied → tested`, `tested → applied`, etc.) | **DROPPED** (no transitions to table) | D5 — derived label transitions only: `never-verified` / `verified-fresh` / `verified-stale` / `verify-failed`, recomputed at read time in `ComposeLabels` | No persisted transitions exist under the freshness model. Demotion is replaced by derived staleness; the derivation function is the new D5. |
| **D6** — source-truth alignment: `tested` lives in `status.json`, never inferred from `artifacts/reconcile-session.json` | **RETAINED** | D6 — the `Verify` sub-record lives in `status.json`; never inferred from artifacts | Reuses ADR-011 D6 verbatim. ADR-010 D5 source-truth guard binding. |
| **D7** — verify is read-only on the working tree; uses shadow workspace for apply-simulation | **RETAINED + EXTENDED** | D7 — verify is read-only on the working tree; shadow simulation now includes hard-parent topological closure replay before applying the target's recipe (F1 fix) | Without the closure replay, V7/V8 false-fail any non-leaf feature whose parent is locally `applied`. The closure is a verify-only construct; no other code path replays parent recipes into shadows. |

## Findings the redesign addresses

- **F1** (CRITICAL): V7/V8 shadow replay now includes the hard-parent topological closure. Spelled out in this ADR's D7 + the successor PRD §3.4.3.
- **F4** (CRITICAL): Lifecycle and freshness fully separated. The "parent-state hook" of the prior design is replaced by pure read-time label derivation in `ComposeLabels`. No read path mutates `.tpatch/`.
- **F2** (HIGH, CURRENT.md drift): the old idle CURRENT.md invented a `Tested *TestedRecord` field. Resolved: the new field is `Verify` (a freshness sub-record), explicitly locked in this ADR's D1.
- **F3** (MEDIUM, Slice A drift): the old idle CURRENT.md pulled `--all`, `--shadow`, and skill-anchor regen into Slice A. The successor PRD's slicing reaffirms Slice A as just the cobra shell + V0–V2 + freshness writer skeleton.

---

## Decision

### D1. Verify produces a freshness overlay; lifecycle stays untouched

`tpatch verify <slug>` writes a new sub-record on `FeatureStatus`:

```go
type FeatureStatus struct {
    // … existing fields unchanged …
    Verify *VerifyRecord `json:"verify,omitempty"`
}

type VerifyRecord struct {
    VerifiedAt          time.Time
    Passed              bool
    CheckResults        []VerifyCheckResult
    RecipeHashAtVerify  string
    PatchHashAtVerify   string
    ParentSnapshot      map[string]FeatureState
}
```

The `FeatureState` enum (`internal/store/types.go:9–19`) is **unchanged**. There is no `StateTested`, no new lifecycle value, no new state-write site outside the existing `apply` / `amend` / `reconcile` paths.

**Why A (chosen) — freshness overlay + lifecycle untouched.**

- Mirrors the Git mental model: lifecycle states are commits, freshness is `git status`. Operators already know this distinction.
- Reads stay reads. The label-recomputation loop that derives `verified-fresh` vs `verified-stale` runs in `composeLabelsFromStatus` (`internal/workflow/labels.go:143`) — a pure function over `(child, parents[])` — and writes nothing. F4 is structurally precluded.
- Backwards compat is automatic at the schema level: `Verify *VerifyRecord` with `omitempty` round-trips byte-identical for v0.6.1 fixtures (no `tested` field populated → field omitted).
- Drift detection is free: `recipe_hash_at_verify` + `patch_hash_at_verify` + `parent_snapshot` are exactly the inputs the verify run leaned on. If any drift, the next `ComposeLabels` call derives `verified-stale` without touching disk.

**Why B (rejected) — `StateTested` as a `FeatureState` enum value.**

- This was the first-revision design. Rejected on F4 grounds: conflates two orthogonal axes (lifecycle progress vs verification freshness). Forces every gate consumer to reason about a state whose semantics include "and the world hasn't moved since I last asserted this," which is not a property of any other lifecycle state.
- Demote-on-read problem: keeping a `tested` state correct under upstream drift requires either (a) writes from a read path (rejected by F4) or (b) every reader treating `tested` as "maybe-tested" (which makes the state useless).
- Apply gate becomes ambiguous: does `tested` satisfy hard deps? Either answer creates surprise. Direction A (yes) means a child applied at T1 against `tested` parent finds the parent demoted to `applied` at T2 with no operator action. Direction B (no) makes a strictly stronger state count as weaker, which is UX-hostile. Both directions were extensively argued in the first revision; F4 dissolves the question by removing the state.

**Why C (rejected) — `Verify` as a top-level file under `.tpatch/<slug>/verify/`.**

- Adds a new artifact lifecycle for no benefit. The freshness record is small (sub-1KB typical), tightly bound to `FeatureStatus` (read together by `LoadFeatureStatus`), and never the source of truth for a derived decision other than label composition. A separate file would mean two reads, two writes, two consistency invariants.
- ADR-010 D5 + ADR-011 D6 explicitly establish `status.json` as the canonical source for derived decisions. Adding a sibling artifact for verify-specific data fights that invariant.

**Decision: A (freshness overlay).**

### D2. Apply gate stays pure-lifecycle

`workflow.CheckDependencyGate` (`internal/workflow/dependency_gate.go:79`) accepts hard parents in `{applied, upstream_merged}`. **This is unchanged in Wave 3.** Freshness labels do not extend the satisfaction set.

**Why A (chosen) — gate ignores freshness.**

- Lifecycle gates govern persistence; freshness governs harness composition. This is the Git-like answer: `git checkout` doesn't ask `git status` whether the working tree is clean before allowing the checkout (with `--force`); the user composes the two checks at the harness level.
- Avoids the demote-on-read problem from a different angle: if the gate consulted freshness, a child applied at T1 against a `verified-fresh` parent could find its parent's freshness flipped to `verified-stale` at T2 with no operator action. The retroactive change to gate satisfaction would be invisible to `apply` callers — the same kind of hidden state the F4 redesign exists to preclude.
- Implementation is zero lines: the gate is unchanged, the original "extend `case StateApplied:` with `case StateTested:`" diff from the first revision is dropped entirely.
- The harness composition pattern `tpatch verify parent && tpatch apply child` keeps working at the harness level: the harness reads `verified-fresh` from `tpatch status --json` and decides whether to re-run verify before composing. The CLI does not enforce.

**Why B (rejected) — gate accepts `applied + verified-fresh` as a stronger form.**

- Re-creates the F4 problem. Freshness is read-time-derived; a parent's `verified-fresh` label can flip to `verified-stale` between T1 (child apply) and T2 (the operator re-checks), and neither side has any way to detect the transition without re-running both verify and apply.
- The "stronger" framing is misleading: `verified-fresh` is **dynamically** stronger than `applied` (more checks have passed) but **structurally** the same (same lifecycle position). Gate semantics should be structural, not dynamic.

**Decision: A (gate ignores freshness).**

The first-revision framing — "does `tested` satisfy hard deps?" — is now obsolete. There is no `tested` state to satisfy. The PRD §3.4.6 documents the collapse explicitly so future readers understand the shift.

### D3. `verify` is the unique writer of the freshness record; `amend` invalidates it

Producer set:

- `tpatch verify <slug>` writes the full `Verify` record (`VerifiedAt`, `Passed`, `CheckResults`, hashes, `ParentSnapshot`).
- `tpatch amend <slug>` (recipe-touching) **invalidates** the existing record by setting `Verify.Passed = false` and bumping no other field. Rationale: an amend that rewrites `apply-recipe.json` or `artifacts/post-apply.patch` causes `recipe_hash_at_verify` / `patch_hash_at_verify` to drift, so the next `ComposeLabels` would derive `verified-stale` regardless. We clear `Passed` proactively to make the invalidation visible at write time (operator inspecting `status.json` immediately after amend sees `passed: false`).
- `tpatch amend <slug>` (intent-only — `request.md` / `spec.md` only) leaves `Verify` untouched.
- `tpatch test`, `tpatch apply`, `tpatch reconcile` do **not** write or invalidate `Verify`. (Reconcile's recipe/patch rewrites cause hash drift naturally; the next `ComposeLabels` derives `verified-stale` without an explicit clear.)

**Why verify-only.**

- The freshness record is a machine-checkable claim. `tpatch test` runs a user-configurable `Config.TestCommand` whose contract on side effects is loose; conflating "user's tests passed" with "tpatch's invariants hold" produces false greens.
- A manual flip (`amend --notes "I promise this is verified"`) is rejected for the same reason. Harnesses cannot trust hand-written claims.
- Implicit `verify` after every `apply` inflates apply latency for a benefit the harness can opt into via `apply && verify`.

**Future-work expansion**: if a real harness need surfaces for `tpatch test` to also be a producer (e.g., "verify + tests-green = double-fresh"), revisit in `feat-tested-state-test-producer`. Out of Wave 3.

**Cost**: one new verb to learn, one extra hook on the `amend` write path.

### D4. Backwards-compatibility contract — byte-identical round-trip for v0.6.1 repos

A v0.6.1 repo that never runs `verify` must round-trip every `status.json` byte-identically through v0.6.2 read/write paths. Locked by:

- The `FeatureStatus` schema gains exactly one field: `Verify *VerifyRecord` with `json:"verify,omitempty"`. A `nil` pointer is omitted from JSON output entirely; the v0.6.1 byte-stream is unchanged.
- `FeatureState` enum is unchanged (this ADR explicitly does not extend it). `ReconcileSummary` and `Config` are unchanged.
- The `ReconcileLabel` vocabulary gains four values (`never-verified`, `verified-fresh`, `verified-stale`, `verify-failed`), but labels are **derived at read time** and never persisted to `status.json`. A v0.6.1 round-trip never emits any of the new label strings.

Enforced by a regression fixture: `TestUpgradeFromV0_6_1_NoVerify_BehavesIdentically` — load v0.6.1 fixture, run every v0.6.1 command except `verify`, diff `.tpatch/` against v0.6.1 expected output, fail on any byte difference.

**Cost**: any future change that touches `FeatureStatus` field set must include this regression fixture in its acceptance criteria.

### D5. Derived freshness labels — read-time computation, no persisted transitions

Four labels are added to `ReconcileLabel`. They are **derived** every time `ComposeLabels` (`internal/workflow/labels.go:89`) runs, from the freshness record + the current DAG snapshot. **No persisted state transitions.** The lifecycle never moves as a result of label derivation.

**Derivation function** (input: `FeatureStatus` for the child + `LoadFeatureStatus` for each hard parent; output: exactly one of the four labels):

```
if child.Verify == nil:
    return "never-verified"
if child.Verify.Passed == false:
    return "verify-failed"
// child.Verify.Passed == true; check freshness
if child.Verify.RecipeHashAtVerify != sha256(read(child/apply-recipe.json)):
    return "verified-stale"
if child.Verify.PatchHashAtVerify != sha256(read(child/artifacts/post-apply.patch)):
    return "verified-stale"
for (parent_slug, snapshot_state) in child.Verify.ParentSnapshot:
    parent_now := LoadFeatureStatus(parent_slug).State
    if not satisfies_state_or_better(parent_now, snapshot_state):
        return "verified-stale"
return "verified-fresh"
```

**`satisfies_state_or_better`** rules:

- `applied` snapshot is satisfied by current `applied` or `upstream_merged` (both satisfy the apply gate; the structural guarantee verify leaned on is preserved).
- `upstream_merged` snapshot is satisfied only by current `upstream_merged` (terminal-by-design; transition out is a manual-edit anomaly).
- Pre-apply snapshots (`requested` / `analyzed` / `defined` / `implementing`) are satisfied by current `applied` or `upstream_merged` (the parent has only become more healthy).
- `blocked` / `reconciling` / `reconciling-shadow` snapshots are satisfied only by themselves; any transition invalidates freshness.

**Transitions** (all derived, all read-time, none persisted):

| From label | To label | Trigger |
|------------|----------|---------|
| `never-verified` | `verified-fresh` | `tpatch verify` PASS |
| `never-verified` | `verify-failed` | `tpatch verify` FAIL |
| `verified-fresh` | `verified-stale` | recipe hash drift, patch hash drift, or parent state drift |
| `verified-fresh` | `verify-failed` | next `tpatch verify` FAIL |
| `verified-fresh` | `verified-fresh` | `tpatch verify` PASS again (record overwritten) |
| `verified-stale` | `verified-fresh` | `tpatch verify` PASS (with current world recorded) |
| `verified-stale` | `verify-failed` | `tpatch verify` FAIL |
| `verify-failed` | `verified-fresh` | `tpatch verify` PASS (with current world recorded) |
| `verify-failed` | `verify-failed` | `tpatch verify` FAIL again (record overwritten) |

Note that none of these "transitions" are observed at the `status.json` level except by the `tpatch verify` rewrites; the `verified-fresh ↔ verified-stale` flip in particular happens purely at read time.

The four labels compose orthogonally with M14.3 labels (`waiting-on-parent`, `blocked-by-parent`, `stale-parent-applied`). A child can carry any subset; rendering is `[m143-label, freshness-label]` in `tpatch status --dag`.

**Why this approach over a persisted state machine.**

- The first-revision state machine had to handle the "world moved" case via the parent-state hook, which (under any sound interpretation) required writes from a read path. The freshness overlay sidesteps this entirely.
- Read-time derivation is bounded by O(hard-parents) per `ComposeLabels` call — same cost as the existing M14.3 derivation. No new hot path.

### D6. Source-truth alignment with ADR-011 D6 / ADR-010 D5

The `Verify` sub-record lives in `status.json`; **never inferred from any other artifact**. Reusing ADR-011 D6 wording verbatim:

> Authoritative source for derived reconcile decisions: read `status.Reconcile.Outcome` via `store.LoadFeatureStatus` — never read `artifacts/reconcile-session.json` for DAG decisions. The session artifact is an audit record of one RunReconcile invocation; status.json is the source of current truth post-accept (see ADR-010 D5).

Applied to verify:

- `tpatch verify` reads `status.Reconcile.Outcome` (in V9 of the check list) — **never** `artifacts/reconcile-session.json` or `artifacts/resolution-session.json`.
- The freshness derivation reads `parent.State` via `store.LoadFeatureStatus` — never any reconcile artifact.
- The freshness record is written only to `status.json`. There is no `verify-session.json`, no new file in `artifacts/`, no new entry in `patches/`. Pass/fail per check is in the `--json` report on stdout; the persisted record is the `Verify` sub-record alone.

Enforced by an adversarial test: the verify implementation must not import or read `artifacts/reconcile-session.json` or `artifacts/resolution-session.json` at any code path; any reference in `internal/workflow/verify.go` is a test failure.

### D7. Verify is read-only on the working tree; closure-replay is the only side effect, in the shadow

Verify mutates **only** `status.json` (the `Verify` sub-record). Apply-simulation runs inside an existing `gitutil.CreateShadow` worktree (`internal/gitutil/shadow.go:56`) rooted at the upstream baseline; the shadow is pruned via `defer PruneShadow(...)` before verify exits, regardless of pass/fail.

**Closure-replay is the F1-mandated structural correction.** Before applying the target's recipe in the shadow, verify replays the target's hard-parent topological closure into the shadow, in order. Without this, V7/V8 are structurally meaningless for any non-leaf feature whose parents are locally `applied`: the shadow's baseline is the upstream tip, which does not contain the parent's changes; the target's recipe will fail to apply (op targets reference parent-created files; the patch references parent-modified hunks).

**Algorithm summary** (full spec in PRD §3.4.3):

1. Compute the hard-parent closure (only `DependencyKindHard` edges, transitively).
2. Order via `store.TopologicalOrder` (`internal/store/dag.go:107`) over the hard-only sub-DAG.
3. Skip parents in `upstream_merged` (their changes are already on the baseline).
4. Replay parents in `applied` — load each parent's `apply-recipe.json` and execute its ops in the shadow.
5. **Fail-fast** on first parent in any other state, or on first replay failure: verify aborts the V7/V8 phase, writes `Verify.Passed = false`, and emits `failed_at: "parent-replay"` with the failing parent slug in the JSON output.
6. Apply the target's recipe (V7) and `git apply --check` the target's `post-apply.patch` (V8) against the same shadow.
7. Prune.

**Per-slug shadow lock.** Verify and reconcile both write to `.tpatch/shadow/<slug>-<timestamp>/`. To prevent two concurrent writers, `verify` refuses when the lifecycle state is `reconciling` / `reconciling-shadow`. Per-slug only: verify on slug A while reconcile runs on slug B is allowed.

**Why closure replay is verify-only.** No other code path replays parent closures into shadows. `apply` works against the live tree (parents already applied locally). `reconcile` works against the upstream baseline + the target's own recipe (parents are out of band, by design — see ADR-010 D2). The closure-replay primitive lives in `internal/workflow/verify.go` only. If a future feature needs the same primitive, an ADR amendment factors it out.

**Cost.** O(closure size) shadow operations per verify. Bounded by DAG depth × per-recipe replay cost; comparable to a phase-2 reconcile op-replay pass per parent. Well within the cheap-budget for typical 1–3-deep DAGs.

## Consequences

**Positive**

- `FeatureState` enum stays single-axis-lifecycle. Every gate consumer reads `state` and gets a structurally meaningful answer.
- Read paths stay reads. F4-class problems are structurally precluded by routing freshness derivation through `composeLabelsFromStatus`.
- Closure replay makes V7/V8 structurally meaningful for non-leaf features. Verify is a useful tool across the entire DAG, not just leaves.
- Apply gate is zero-diff vs v0.6.1 — no behavioural change for existing users.
- D6 source-truth guard preserves ADR-011 D6 / ADR-010 D5 invariants.
- `omitempty` on the new `Verify` field gives byte-identical round-trip until first verify.

**Negative / accepted trade-offs**

- The `FeatureStatus` schema gains a field. Downstream JSON consumers that hard-code v0.6.1 schema-shape need to handle the omitempty case once first verify runs in any feature. CHANGELOG callout.
- Closure replay cost on deep DAGs scales with depth × per-recipe cost. Mitigated by harness pattern (verify parents first; rely on `verified-fresh` labels).
- Two simultaneous label systems (M14.3 + freshness) for `ComposeLabels` consumers to handle. Mitigated by orthogonal composition rules (PRD §3.5).
- Operator confusion: "I ran verify but the lifecycle state didn't change." Mitigated by skill bullet + CHANGELOG copy explaining the freshness-vs-lifecycle distinction.

**Neutral**

- `tpatch test` integration deferred (D3 future-work expansion).
- `tpatch verify --all` deferred to Slice D.
- `--fresh-branch` explicitly out of scope (PRD §0.3).
- Recipe-op JSON schema unchanged. Verify tolerates deletions in shadow replay the same way recipe autogen does.

## Alternatives considered

1. **`StateTested` as a `FeatureState` enum value** — rejected per D1. Conflates lifecycle with freshness; routes mutation through read paths to remain correct under drift.
2. **`Verify` as a top-level file under `.tpatch/<slug>/verify/`** — rejected per D1. Adds artifact lifecycle for no benefit; fights ADR-010 D5 / ADR-011 D6.
3. **Apply gate accepts `applied + verified-fresh` as a stronger form** — rejected per D2. Re-creates the demote-on-read problem at the gate level.
4. **Manual flip via `amend --state tested`** — rejected per D3. No `tested` state exists; the freshness record is machine-checkable only.
5. **Implicit `verify` after every `apply`** — rejected per D3. Inflates apply latency for a benefit the harness can opt into.
6. **`tpatch test` as a producer of the freshness record** — deferred to `feat-tested-state-test-producer`. Conflates user-test-suite-pass with tpatch-invariants-hold.
7. **A new `verify-session.json` artifact** — rejected per D6/D7. Verify writes only `status.json`.
8. **V9 (`reconcile_outcome_consistent`) as block severity** — left as PRD Q1; default warn.
9. **`--shadow` flag on verify** — rejected. V7/V8 already gate shadow allocation on recipe/patch presence.
10. **`--fresh-branch` flag on verify** — out of scope.
11. **Closure replay as a shared helper** — rejected. No other code path needs it; keeping it inside `internal/workflow/verify.go` avoids over-factoring.
12. **Skip-and-continue on parent-replay failure** — rejected per PRD §3.4.3. Skipping makes the V7 result meaningless (target's recipe applied against partial baseline).

## References

- `docs/prds/PRD-verify-freshness.md` — operational detail (second revision)
- `docs/adrs/ADR-011-feature-dependencies.md` — feature DAG contract; this ADR preserves D3 (composable labels), D6 (source-truth guard), D9 (config gate)
- `docs/adrs/ADR-010-provider-conflict-resolver.md` — shadow worktree contract (D2), artifact ownership split (D5)
- `docs/dependencies.md` — user-facing dep reference; will gain a one-paragraph cross-link to verify in Slice D
- CHANGELOG v0.6.1 → "Out of scope for v0.6.1" — names verify and tested as the two deferred items this PRD addresses
- `internal/store/types.go:91` — `FeatureStatus` struct (D1 field-add site)
- `internal/store/types.go:50–60` — `ReconcileLabel` vocabulary (D5 extension site)
- `internal/store/store.go:232` — `LoadFeatureStatus` (D6 source-truth read site; D5 derivation input)
- `internal/store/dag.go:107` — `TopologicalOrder` (D7 closure-replay ordering)
- `internal/store/validation.go:38–44, 101–108` — `satisfiedBySHA` regex + `gitutil.IsAncestor` reachability (V5 reuse)
- `internal/workflow/dependency_gate.go:79` — `CheckDependencyGate` (D2 anchor; explicitly NOT modified)
- `internal/workflow/labels.go:89` — `ComposeLabels` (D5 derivation site)
- `internal/workflow/labels.go:143` — `composeLabelsFromStatus` (D5 pure-function host; freshness derivation lives here)
- `internal/workflow/created_by_gate.go:57` — `checkCreatedByGate` (V3 reuse)
- `internal/gitutil/shadow.go:56` — `CreateShadow` (D7 reuse)
- `internal/gitutil/gitutil.go:680` — `IsAncestor` (V5 reuse)
