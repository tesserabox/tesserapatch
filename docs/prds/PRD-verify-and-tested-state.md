> **STATUS: SUPERSEDED — 2026-04-27.** This document captures the originally
> approved Wave 3 design (lifecycle-state model: `tested` as a forward state on
> the `FeatureState` enum). An external re-review (see `docs/supervisor/LOG.md`
> 2026-04-27 reopening entry) found that the design conflated lifecycle and
> verification freshness, and routed read-time mutation through `LoadFeatureStatus`.
> The supervisor reopened Wave 3 with a binding redesign: a Git-like freshness
> overlay model.
>
> - **Successor PRD**: [`docs/prds/PRD-verify-freshness.md`](PRD-verify-freshness.md)
> - **Successor ADR**: [`docs/adrs/ADR-013-verify-freshness-overlay.md`](../adrs/ADR-013-verify-freshness-overlay.md)
>
> This document is preserved as historical record only. **Do not implement
> against it.**

---

# PRD — `tpatch verify` + `tested` lifecycle state — `feat-verify-command` + `feat-feature-tested-state`

**Status**: Draft (M15 Wave 3 design dispatch)
**Date**: 2026-04-27
**ADR**: **ADR-012-feature-tested-state.md — REQUIRED before Wave 3 implementation slices ship.**
**Owner**: Core
**Milestone**: M15 → Wave 3 (lifecycle / reconcile semantics tranche)
**Related**: ADR-010 (M12 resolver), ADR-011 (feature DAG), `docs/dependencies.md`, `docs/prds/PRD-feature-dependencies.md`, CHANGELOG v0.6.1

---

## 0. Meta

### 0.1 Why one PRD covers two backlog items

The reviewer go-to-tag note for Wave 3 explicitly required clarifying how `feat-verify-command` and `feat-feature-tested-state` relate before scoping either. They share machinery (`verify` is the natural producer of `tested`) and they share contract surface (does `tested` satisfy hard dependencies? does `verify` transition state? does `verify` re-check the v0.6.1 `satisfied_by` reachability contract?). Splitting forces those decisions twice and risks them drifting. One PRD, two state-machine outcomes locked together; the implementation is sliced into four independently dispatchable waves (§9).

### 0.2 Architecture decisions to lock in ADR-012

This PRD records the _what_ and the _why-now_. Per repo rule (`AGENTS.md` → "Context Preservation Rules" §6) every architecture choice here is captured in **ADR-012-feature-tested-state.md** before any Wave 3 implementer slice begins. Specifically the ADR locks:

1. **D1** — placement of `tested` in `FeatureState` enum (linear sibling of `applied`, not a branch).
2. **D2** — **does `tested` satisfy hard dependencies the same way `applied` does?** This is the most consequential decision; argued both ways in §3.4.4 and locked in ADR-012.
3. **D3** — producer set for `tested` (`verify` only in Wave 3; `test` and `amend --state tested` deliberately deferred).
4. **D4** — backwards-compatibility contract: byte-identical round-trip for v0.6.1 repos that never run `verify`.
5. **D5** — forward / backward transition table for `tested`.
6. **D6** — source-truth alignment with ADR-011 D6 / ADR-010 D5: `tested` is persisted in `status.json`, never inferred from `artifacts/reconcile-session.json` or any other artifact.
7. **D7** — `verify` is read-only on the working tree; apply-simulation checks run inside an existing `gitutil` shadow worktree (`internal/gitutil/shadow.go`).

Any deviation during implementation requires an ADR-012 amendment before the slice merges.

### 0.3 Out of scope (cross-linked)

Explicitly **not** part of this PRD — each is a separate Wave 3 candidate or future PRD:

- `feat-reconcile-code-presence-verdicts` — making reconcile assert that recipe ops are still represented in HEAD. Verify reuses the same primitives (apply-simulation in a shadow) but does not change reconcile's verdict set.
- `feat-reconcile-fresh-branch-mode` — running reconcile against a freshly-checked-out upstream branch instead of `HEAD~`. `tpatch verify --fresh-branch` is **not** in scope for Slice A; see §9 future work.
- **`delete-file` recipe op.** The recipe-op JSON schema is frozen (CHANGELOG v0.6.1 Notes). Verify's recipe-replay check tolerates deletions the same way recipe autogen does (skip with stderr note); a real `delete-file` op needs its own ADR.
- **Anything that reads `artifacts/reconcile-session.json`.** Verify reads `status.Reconcile.Outcome` only (ADR-010 D5 + ADR-011 D6). This is not a punt — it is a hard constraint baked into every check in §3.

---

## Summary

`tpatch verify <slug>` is a **read-only**, machine-checkable health command that re-runs every static and apply-simulation check we already know how to run for a single feature, prints a pass/fail report (human and `--json`), and — if every check passes — promotes the feature into a new lifecycle state, **`tested`**, that sits between `applied` and `active`. `tested` records "the feature is structurally healthy and re-applies cleanly against the current upstream baseline." It does **not** mean the project's test suite has run; that remains `tpatch test`'s contract.

Verify reuses the v0.6.1 primitives end-to-end:

- `store.ValidateDependencies` / `store.ValidateAllFeatures` (M14.1) for dependency hygiene.
- `store.satisfiedBySHA` regex + reachability check (v0.6.1 — `internal/store/validation.go:38–44, 101–108`) re-run for drift since edit time.
- `gitutil.IsAncestor` (`internal/gitutil/gitutil.go:680`) for parent-SHA reachability and `satisfied_by` revalidation.
- `gitutil.CapturePatchScoped` / `CaptureDiffStatScoped` (`internal/gitutil/gitutil.go:216`) for any drift-vs-recorded checks.
- `gitutil.CreateShadow` / `PruneShadow` (`internal/gitutil/shadow.go:56,122`) for apply-simulation in a throwaway worktree.
- `workflow.UserShell` / `shellQuoteFor` (`internal/workflow/shell.go:13`) — only to invoke recipe-execution machinery; verify itself runs no `test_command`.

It does **not** introduce a new file, a new artifact directory, or any new persisted enum on `Reconcile.Outcome`. The single schema change is an additive, `omitempty`, byte-identity-preserving extension of `FeatureState` to include `tested`.

---

## 1. Problem statement

### 1.1 What's missing today

Through v0.6.1 a feature reaches `applied` after `tpatch apply --mode execute` succeeds and stays there indefinitely. We have no way for an operator (or harness) to ask the cheap, structural question: "is this feature still healthy against the current tree?" The state of the world is split across:

- **Static:** `spec.md` and `exploration.md` exist and reference real paths.
- **Recipe-shape:** `apply-recipe.json` (if present) parses, has dependencies that resolve, and op targets resolve.
- **Apply-simulation:** the recipe still re-applies cleanly to a fresh shadow worktree.
- **Patch-replay:** `artifacts/post-apply.patch` still applies to the upstream baseline.
- **Dependency hygiene:** parent slugs exist, no cycles, `satisfied_by` SHAs still 40-hex AND reachable.
- **DAG context:** parent state is one of the apply-gate satisfying values.

Today these checks are scattered: dep validation runs at write time and during `status`; recipe parsing runs during `apply`; patch reverse-apply happens inside `reconcile`. There is no single command that runs them all without side effects.

The pain shows up in three places:

1. **Pre-`reconcile` triage.** Operators run `reconcile` to find out if anything is structurally broken; reconcile is heavy (provider call, shadow worktree, optional resolver) and is the wrong tool for "does this feature still parse?"
2. **Post-`amend` confidence.** `tpatch amend` (M13) edits files and refreshes recipe metadata. There is no cheap follow-up that confirms the edit didn't break the recipe / patch invariants.
3. **Harness handoff.** The skill harness needs a cheap, machine-readable signal that a feature is "ready for the next phase" between implement and reconcile, without forcing a real test run. v0.6.1 left the harness with no such signal.

### 1.2 What `tested` is for

`tested` is the persistent answer to "verify passed for this feature against the current upstream baseline." It is **distinct from `applied`** because `applied` is a one-shot stamp set by `tpatch apply` and stays sticky; nothing demotes it short of a manual amend. `tested` is the dynamic answer — it can be lost when the world moves underneath the feature (parent re-applied, upstream changed, `amend` rewrote the recipe). The value proposition is: **a `tested` feature is one a harness can hand off downstream without further inspection**.

Why not just emit a transient verdict? Because the harness needs to know whether verify has _ever_ succeeded against the current world, not just whether it succeeded in this invocation. That is by definition state, not output. ADR-012 D1 records this.

### 1.3 What `verify` is NOT

To prevent scope creep, this PRD is explicit:

- `verify` is **not** `apply`. It does not write to the working tree. Apply-simulation runs in a `gitutil.CreateShadow` worktree and the shadow is pruned before exit.
- `verify` is **not** `reconcile`. It does not call the provider, does not run phase 3.5, does not produce a `reconcile-session.json`.
- `verify` is **not** `test`. It does not run `Config.TestCommand`. `tested` therefore does not imply "the test suite passed" — it implies "the feature's structural invariants hold." `tpatch test` remains the project-test runner.

---

## 2. Goals / non-goals

### Goals

- One read-only command, `tpatch verify <slug>`, that runs a fixed, ordered list of checks (§3.1) and produces both a human summary and a `--json` shape (§4.2).
- A new persisted lifecycle state, `tested` (§3.4), produced by `verify` on green and demoted automatically when verify-invalidating events happen (§3.4.5).
- An explicit, locked answer to "does `tested` satisfy hard dependencies?" (§3.4.4 + ADR-012 D2).
- Byte-identical round-trip for v0.6.1 repos that never run `verify` (§3.4.6).
- Reuse of every existing primitive that overlaps a verify check (§3.1) — no new validation logic where the store already has it.
- Composition with M14 reconcile labels (§3.5): `tested` overlays correctly with `waiting-on-parent`, `blocked-by-parent`, `stale-parent-applied`.

### Non-goals (explicit)

- **Provider calls.** Verify is offline by construction.
- **`tpatch test` integration in Wave 3.** §3.4.3 records why `test` is not a producer of `tested` in this PRD; deferred to `feat-tested-state-test-producer`.
- **`amend --state tested`.** Manual flip is rejected for Wave 3 (§3.4.3 D3 alternative). Operators who want to "promise" a feature is tested must run `verify`. Deferred indefinitely; no follow-up PRD.
- **Code-presence reconcile verdicts.** Out of scope (§0.3).
- **Fresh-branch reconcile mode.** Out of scope (§0.3).
- **Schema additions to recipe-op JSON.** Frozen.
- **A new `verify-session.json` artifact.** Verify writes only `status.json` (and stderr / stdout). The pass/fail per check is in the report; persisted state is the `tested` flip alone.

---

## 3. Specification

### 3.1 The check list — exact order, primitives, severity

`tpatch verify <slug>` runs the checks in the order below. Every check is one of two severities:

- **`block`** — failure prevents the `tested` flip and produces a non-zero exit. The check is reported and verify continues so the operator gets the full picture (no early abort, except where explicitly noted).
- **`warn`** — failure is reported but does not prevent the `tested` flip or the zero exit code.

Checks abort early **only** at `V0` (load) — a feature whose `status.json` cannot be loaded cannot be verified meaningfully, so verify exits with `exit 2 — internal error` and reports `status_load_failed`.

| # | Check id | Trigger | Severity | Pass criterion | Fail criterion | Reuses |
|---|----------|---------|----------|----------------|----------------|--------|
| V0 | `status_loaded` | always | block-abort | `store.LoadFeatureStatus(slug)` returns OK | any error from `LoadFeatureStatus` | `internal/store` |
| V1 | `intent_files_present` | always | block | `spec.md` and `exploration.md` exist on disk under `.tpatch/features/<slug>/` and are non-empty | missing or zero-byte | direct fs |
| V2 | `recipe_parses` | recipe present | block | `apply-recipe.json` parses through the canonical unmarshal path with `DisallowUnknownFields` | parse error / unknown field | existing recipe loader |
| V3 | `recipe_op_targets_resolve` | recipe present | block | every op's `Path` exists on disk **OR** the op carries a `created_by` whose parent is a declared **hard** dep currently in `applied`/`upstream_merged` (mirrors `created_by` apply-time gate, `docs/dependencies.md` §"created_by") | path missing AND `created_by` empty / soft / dangling | reuses `created_by` semantics from M14.2 — no new logic |
| V4 | `dep_metadata_valid` | always | block | `store.ValidateDependencies(s, slug, status.DependsOn)` returns nil (`internal/store/validation.go:66`) | any sentinel error from validation | `store.ValidateDependencies` |
| V5 | `satisfied_by_reachable` | every dep with `satisfied_by` set | block | `store.satisfiedBySHARe.MatchString(dep.SatisfiedBy)` AND `gitutil.IsAncestor(repoRoot, dep.SatisfiedBy, "HEAD")` returns true (mirrors v0.6.1 validation, `internal/store/validation.go:101–108`) | malformed SHA or not reachable | `gitutil.IsAncestor` (post-v0.6.1 contract — drift since edit time is exactly the case verify is meant to catch) |
| V6 | `dependency_gate_satisfied` | always (gated on `Config.DAGEnabled()`) | warn | `workflow.CheckDependencyGate(s, slug)` returns nil (`internal/workflow/dependency_gate.go:42`) | any wrapping of `ErrParentNotApplied` | `workflow.CheckDependencyGate` (warn-only because `tested` does not require apply-readiness in the present moment — this surfaces context, see §3.4.4) |
| V7 | `recipe_replay_clean` | recipe present | block | recipe replays cleanly into a `gitutil.CreateShadow` worktree rooted at upstream baseline; shadow pruned before exit | any recipe op fails to apply in the shadow; deletions tolerated per CHANGELOG v0.6.1 schema-frozen note | `gitutil.CreateShadow`, `PruneShadow`, existing recipe executor |
| V8 | `post_apply_patch_replay_clean` | `artifacts/post-apply.patch` present | block | `git apply --check` of `post-apply.patch` against the upstream baseline succeeds (read-only, no working-tree mutation) | non-zero exit from `git apply --check` | `gitutil.CapturePatchScoped` is the recorder; verify uses `git apply --check` directly via the same exec helper |
| V9 | `reconcile_outcome_consistent` | `status.Reconcile.Outcome` set | warn | `status.Reconcile.Outcome ∈ {reapplied, upstreamed, still_needed}` (a healthy terminal verdict) | `Outcome ∈ {blocked, blocked-too-many-conflicts, blocked-requires-human, shadow-awaiting}` | reads `status.Reconcile.Outcome` only — **never** `artifacts/reconcile-session.json` (ADR-010 D5, ADR-011 D6) |

#### 3.1.1 Ordering rationale

V0 → V6 are **static**: file checks, struct unmarshals, regex/git ancestor, in-process function calls. V7 and V8 are **dynamic** (shadow worktree spin-up + `git apply --check`). The static block runs first so a recipe-shape error doesn't waste a shadow allocation. V9 is last because it is purely an informational read of `status.Reconcile.Outcome`.

#### 3.1.2 Remediation messages

Every fail surfaces a one-line remediation, mirroring the style already established by `CheckDependencyGate`. Examples (exhaustive list in §4.3 JSON schema):

- V1 → `"file 'spec.md' missing or empty; re-run tpatch define <slug>"`
- V2 → `"apply-recipe.json failed to parse: <err>; fix the recipe or re-run tpatch implement <slug>"`
- V3 → `"recipe op #<n> path '<p>' missing and created_by empty; declare created_by=<parent> or apply <parent>"`
- V4 → wraps the validation sentinel verbatim plus `"; re-run tpatch feature deps --validate-all to surface every violation"`
- V5 → `"satisfied_by SHA <sha> for parent <slug> is no longer reachable from HEAD (drift since edit); re-run tpatch amend <slug> --remove-depends-on <parent> --depends-on <parent>"`
- V6 → `"hard parent <slug> in state=<state> (warn-only at verify time)"`
- V7 → `"recipe op #<n> failed in shadow replay: <err>; investigate or re-run tpatch implement <slug>"`
- V8 → `"post-apply.patch no longer applies to upstream baseline; run tpatch reconcile <slug>"`
- V9 → `"reconcile outcome is <outcome>; tested cannot be promoted while reconcile is in a blocked state (warn-only)"` — **but see §3.4.4 — V9 demotes if currently `tested`.**

### 3.2 What `verify` writes

Verify is **read-only on the working tree** by ADR-012 D7. It writes exactly two things, both to the store:

1. `status.json` flip from `applied` (or `tested` already) → `tested` on green; or from `tested` → `applied` on any V1/V2/V3/V4/V5/V7/V8 fail (the demotion path; see §3.4.5).
2. `status.json` `LastCommand = "verify"` and bumped `UpdatedAt`.

It writes **no** new file in `artifacts/`, no new file under `.tpatch/`, no new entry in `patches/`. The shadow worktree it spins up for V7 is pruned before verify exits, regardless of pass/fail.

### 3.3 Dependency-gate severity is `warn`, not `block` — rationale

V6 (`dependency_gate_satisfied`) is the one check the reviewer will challenge: why isn't an unsatisfied hard parent a blocker for `tested`?

Because verify must be useful in two scenarios in which the hard-parent gate would fail despite the feature being structurally healthy:

1. **Pre-apply harness handoff.** A child whose hard parent is still `defined` is structurally fine — its recipe parses, its `satisfied_by` is reachable. The harness wants to know "is this slug ready" before it tries to apply the parent. Blocking `tested` on parent-state penalises the cheaper question.
2. **`upstream_merged` parent without `satisfied_by`.** Verify can detect the structural drift via V5; the dep gate's behaviour for `upstream_merged` is to accept regardless. A warn here lets V5 do the precise work and V6 echo context.

The reviewer-relevant counter-argument is recorded as a deliberately-rejected alternative in §6 (D2 alt 3).

### 3.4 The `tested` lifecycle state

#### 3.4.1 Placement in the enum

`tested` is a new value of `FeatureState`, sibling of `applied` (ADR-012 D1). The full enum becomes:

```go
StateRequested         FeatureState = "requested"
StateAnalyzed          FeatureState = "analyzed"
StateDefined           FeatureState = "defined"
StateImplementing      FeatureState = "implementing"
StateApplied           FeatureState = "applied"
StateTested            FeatureState = "tested"          // NEW
StateActive            FeatureState = "active"
StateReconciling       FeatureState = "reconciling"
StateReconcilingShadow FeatureState = "reconciling-shadow"
StateBlocked           FeatureState = "blocked"
StateUpstreamMerged    FeatureState = "upstream_merged"
```

It sits **between `applied` and `active`** semantically: `active` is operator-promoted "I'm maintaining this feature in the fork" (an explicit human signal). `tested` is verify-promoted "the feature is structurally healthy as of the last check." A feature can be `tested` without ever being `active`, and can be `active` without ever being `tested` (the latter is the v0.6.1 default — every existing repo is `applied` → `active` with no detour).

#### 3.4.2 State-transition truth table

Rows = current state. Columns = trigger. Cells = next state. Empty cell = trigger is rejected (`exit 2 — invalid state transition`).

| current state \ trigger | `apply --mode execute` | `verify` PASS | `verify` FAIL | `amend` (recipe-touching) | `amend` (intent-only, request.md/spec.md) | `reconcile` PASS | `reconcile` FAIL (blocked-*) | parent state-flip event |
|-----|-----|-----|-----|-----|-----|-----|-----|-----|
| `requested`        | ❌ | ❌ | ❌ | ❌ | `requested` | ❌ | ❌ | — |
| `analyzed`         | ❌ | ❌ | ❌ | ❌ | `analyzed`  | ❌ | ❌ | — |
| `defined`          | ❌ | ❌ | ❌ | ❌ | `defined`   | ❌ | ❌ | — |
| `implementing`     | `applied` | ❌ | ❌ | `implementing` | `implementing` | ❌ | ❌ | — |
| **`applied`**      | `applied` (idempotent) | **`tested`** | `applied` | `applied` | `applied` | `applied` | `blocked` (per ADR-010) | label overlay only — state unchanged (§3.5) |
| **`tested`**       | `applied` (re-apply demotes; §3.4.5) | `tested` | **`applied`** (V1–V8 demotion; see below) | **`applied`** (any recipe-touching amend) | `tested` (intent-only does not invalidate) | `tested` | `blocked` | label overlay; if `stale-parent-applied` triggered, **demote to `applied`** (§3.4.5) |
| `active`           | `applied` (operator pulled out of "active" by re-applying) | `active` (ADR-012 D5: verify on `active` runs the checks but does **not** flip to `tested`; `active` is operator-owned) | `active` (warn-only; no demotion of `active`) | `active` | `active` | `active` | `blocked` | label overlay |
| `reconciling`      | ❌ | ❌ — verify refused while a reconcile is mid-flight (`exit 2`); see §3.4.7 | n/a | ❌ | ❌ | `applied` (per ADR-010) | `blocked` | — |
| `reconciling-shadow` | ❌ | ❌ — same as `reconciling` | n/a | ❌ | ❌ | depends on accept/reject | `blocked` | — |
| `blocked`          | ❌ (must amend / reconcile first) | ❌ — verify on `blocked` runs and reports but **does not** flip to `tested`; the operator needs to clear the block first | n/a | `applied` (per existing M13) | `blocked` | `applied` | `blocked` | — |
| `upstream_merged`  | ❌ | `upstream_merged` (verify runs read-only; `tested` does not apply to a retired feature) | `upstream_merged` | ❌ | `upstream_merged` | `upstream_merged` | `upstream_merged` | — |

**Locked invariants** (cross-referenced into ADR-012 D5):

- `verify` only ever transitions between `applied` ↔ `tested`. No other state in / out.
- `verify` on a feature in any state other than `{applied, tested, active, blocked, upstream_merged}` returns `exit 2 — invalid state for verify` and runs no checks.
- `verify` on `active`, `blocked`, `upstream_merged` runs all checks (so the harness gets a report) but **does not flip state**. Rationale: those states are operator/system-owned terminal stamps; `tested` is for the in-flight applied-feature segment of the lifecycle.
- `tested` → `applied` happens automatically on **any** of: a verify FAIL on a block-severity check; any recipe-touching `amend`; a re-apply (`apply --mode execute`); a reconcile that emits a `blocked-*` outcome; the v0.6.1 `stale-parent-applied` label being computed on a hard parent (§3.5). Demotion is **silent** in the sense that no provider call is made, but it is **logged** to stderr (`"feature %s demoted: tested → applied (reason: %s)"`).

#### 3.4.3 Producers of `tested` (D3)

**Producer set in Wave 3: `verify` only.**

Argued alternatives (rejected, recorded in ADR-012 D3 "alternatives considered"):

- **Alt A: `verify` + `tpatch test`.** Tempting because `test` is the obvious final phase of the lifecycle. Rejected because `test` runs the project's `test_command`, which is configurable per-repo, has no contract on side effects, and may produce false greens (a stale test cache passes). `tested` should mean "tpatch's invariants hold," not "the user's test suite happened to pass." Adding `test` as a producer conflates two questions; the harness can run both and decide.
- **Alt B: `verify` + `amend --state tested` (manual flip).** Rejected because `tested` is supposed to be a machine-checkable claim. A manual flip is just `amend --notes "I promise this is tested"` with extra steps — and harnesses cannot trust it. ADR-012 D3 records the rejection; no flag is added to `amend` for this purpose.
- **Alt C: implicit `verify` after every `apply`.** Rejected because `apply` already touches the working tree; chaining a shadow allocation onto the apply path inflates apply latency for a benefit (`tested` immediately after `apply`) that the harness can opt into by running `apply && verify`. Keeps the two phases separate and observable.

Producers therefore: **`tpatch verify <slug>` is the unique writer of `tested`.** The reverse demoter (writes `applied` over `tested`) is multi-source per the §3.4.2 table — the demotion is a label-style overlay computed by the existing `apply` / `amend` / `reconcile` paths plus the parent-state event hook (§3.4.5).

#### 3.4.4 Does `tested` satisfy hard dependencies? — D2

This is the consequential decision the dispatch flagged. Both directions are argued in full so the reviewer / supervisor can adjudicate.

**Direction A — yes, `tested` satisfies hard dependencies (added to the satisfaction set alongside `applied`, `upstream_merged`).**

The dependency gate (`workflow.CheckDependencyGate`, `internal/workflow/dependency_gate.go:79–101`) accepts `StateApplied` and `StateUpstreamMerged`. Adding `StateTested` is a one-line change. Pros:

- Mirrors operator intuition: "I verified the parent; surely the child can apply on top of it." `tested` is strictly stronger than `applied` (every check that fires on apply also fires on verify, plus more), so anything `applied` satisfies, `tested` satisfies a fortiori.
- Lets a harness chain `verify parent && apply child` without an `apply parent` step — useful for the harness handoff scenario (§1.1).
- Symmetric with `upstream_merged`: if the parent's changes "live" anywhere reachable (upstream or local-and-verified), the child can apply.

**Direction B — no, `tested` does NOT satisfy hard dependencies. Only `applied` and `upstream_merged` satisfy.**

`tested` is dynamic — it can demote silently when a parent state-flip event fires (§3.4.5). If `tested` satisfied the gate, a child that successfully applied at time T1 (when parent was `tested`) could later find its parent demoted to `applied` (because the parent's verify failed at T2), and the dep graph would be retroactively "fine" but the parent's structural invariants would not hold. The asymmetry — child applied because parent was `tested`, parent now `applied` (which still satisfies the gate) — is correct but disorienting.

Pros of B:

- Keeps the gate's truth set monotone: the satisfaction set is `{applied, upstream_merged}`, both of which are sticky-by-design (they only change on explicit operator action). `tested` is intentionally not sticky.
- Means the dep gate has the same behaviour in v0.6.1 and v0.6.2-with-tested. Backwards compat is automatic at the gate level: existing `CheckDependencyGate` logic is untouched.
- Avoids a confusing failure mode where the child apply succeeds, the parent's verify later fails (demoting parent to `applied` — still satisfying the gate, so the child stays applied), and the operator believes "verify on the parent is implied." The `tested → applied` demotion does not propagate to children, by design (§3.4.5), so a `tested`-satisfies-gate world produces a transient inconsistency that the operator must reason about.

**DECISION (locked in ADR-012 D2): Direction A — `tested` satisfies the hard-dep gate, equivalent to `applied`.**

The dep gate's satisfaction set extends from `{applied, upstream_merged}` to `{applied, tested, upstream_merged}`. Implementation: the `case StateApplied:` arm in `CheckDependencyGate` (`internal/workflow/dependency_gate.go:79–101`) is extended to also match `StateTested`. The change is one switch arm and has no other behavioural effect; `tested` gains no extra power vs. `applied` for gating purposes — it satisfies because it implies `applied`-level structural guarantees, not because the gate learns a new concept.

The framing is therefore: `tested` is a strict superset of `applied`. Every check that fires at apply time also fires at verify time (recipe parses, deps valid, satisfied_by reachable, recipe replay clean). A child that can apply on top of an `applied` parent can apply on top of a `tested` parent — the parent has only become more, not less, healthy. The harness scenario "`verify parent && apply child`" works without a redundant `apply parent` step. ADR-012 D2 is the locking record.

The dynamic-vs-sticky asymmetry called out under "Pros of B" above (parent demotes silently from `tested` → `applied` mid-flight) is mitigated by the §3.4.5 parent-state hook: the `tested → applied` demotion does **not** cascade to children. A child that applied while its parent was `tested` continues to satisfy its gate after the parent demotes to `applied` — because `applied` itself satisfies the gate. There is never a moment where a child sees an unsatisfied gate from a parent regression alone (the parent is at worst `applied`). `tpatch status --dag` surfaces a `stale-parent-applied` label per existing v0.6.1 logic when this happens (no new label needed — `LabelStaleParentApplied` is exactly this signal).

To be clear, ADR-012 D2 locks: "`tested` satisfies the hard-dep gate; does not change soft-dep semantics; does not change `created_by` semantics; demotion does not cascade to children."

**Rejected alternative — Direction B (`tested` does NOT satisfy).** The arguments above ("Pros of B") were considered and rejected. The strict variant (`CheckDependencyGate` gains an explicit `case StateTested:` that rejects with `parent foo is tested but apply gate requires applied/upstream_merged; run tpatch apply <foo>`) is UX-hostile and forces operators to re-apply parents whose structural invariants are already known to hold. The argument from monotone gate semantics is real but does not outweigh the operator-intuition and harness-handoff costs; the cascade-free demotion design (§3.4.5) removes the inconsistency that would otherwise have justified Direction B.

**Rejected alternative — derived-label variant (ADR-012 D2 alt 3).** "`tested` produces a stricter form of dep satisfaction — children of a `tested` parent inherit a `parent-tested` label." Rejected because it bloats the M14.3 label set (`waiting-on-parent`, `blocked-by-parent`, `stale-parent-applied`) for no operator-actionable benefit, and re-opens ADR-011 D3 (composable derived labels) to extend the label vocabulary for `tested`. Out of scope; revisit in a hypothetical `feat-tested-cascade-labels` if a real harness need surfaces.

#### 3.4.5 Backward / forward transitions and the parent-state hook

`tested` is forward from `applied` and backward from a number of triggers, summarised below. The complete table is §3.4.2; this subsection walks the load-bearing edges.

**Forward edges (into `tested`):**

- `applied + verify PASS → tested`. Single edge that produces `tested`.
- `tested + verify PASS → tested` (idempotent re-verification, bumps `UpdatedAt`).

**Backward edges (out of `tested`, all into `applied`):**

- `tested + verify FAIL (V1/V2/V3/V4/V5/V7/V8) → applied`. The block-severity demotion path. V6 and V9 are warn-only; they do **not** demote.
- `tested + amend (recipe-touching) → applied`. The amend path that rewrites `apply-recipe.json` or `artifacts/post-apply.patch` invalidates the prior verify result by definition.
- `tested + amend (intent-only) → tested` (preserved). `request.md`-only or `spec.md`-only edits do not invalidate verify; the recipe / patch / dep graph are untouched.
- `tested + apply --mode execute → applied`. A re-apply restamps the apply-time invariant; `tested` is reset until the next verify.
- `tested + reconcile (any blocked-*) → blocked` (per existing ADR-010 path; verify-state is replaced by reconcile-state).
- `tested + reconcile (reapplied / upstreamed) → applied` (NOT `tested`). Rationale: reconcile changes the upstream baseline and may rewrite `artifacts/post-apply.patch` and `apply-recipe.json`; the prior verify is no longer guaranteed to hold. Operator re-runs `verify` if they want `tested` back. ADR-012 D5 records this.
- **`tested + parent state-flip event → applied`** (the parent-state hook). When any hard parent transitions out of an apply-gate-satisfying state — specifically `applied/tested/upstream_merged` → anything else — every child currently in `tested` is demoted to `applied`. This is the propagation rule that mirrors the `LabelStaleParentApplied` overlay (M14.3); the difference is that the label is informational while the demotion is structural.

The parent-state hook is the only **automatic cross-feature** state mutation in this PRD, and it is by design narrow: only `tested → applied` propagates, and it propagates only one step (children of the demoted parent — not grandchildren, because the children are flipping to `applied`, which is itself a satisfying state). Cycles are impossible because the DAG is acyclic by ADR-011 invariant.

**Implementation note:** the parent-state hook is invoked from the same call sites that today invoke `Reconcile.Labels` recomputation (the M14.3 plumbing) — i.e., on every `LoadFeatureStatus` for any feature with non-empty `DependsOn`. The cost is the same iteration; only the action on a stale-parent detection differs (label overlay → demotion). No new hot path.

#### 3.4.6 Backwards-compatibility contract (D4)

A v0.6.1 repo that never runs `verify` must round-trip every `status.json` byte-identically through v0.6.2 read/write paths. This is enforced by:

- The `FeatureState` enum gaining `StateTested` is an additive change. Any feature whose `state` is one of the v0.6.1 values continues to round-trip — there is no field-shape change.
- `tested` is **only** written by `verify`. Any code path that writes `state` today (apply, amend, reconcile) is unchanged. A v0.6.1 repo upgraded to v0.6.2 without running `verify` therefore has zero `state="tested"` features and is byte-identical at rest.
- The `Reconcile.Labels` overlay set is not extended (§3.4.4 decision). The label vocabulary stays `{waiting-on-parent, blocked-by-parent, stale-parent-applied}`.
- No new field is added to `FeatureStatus`, `ReconcileSummary`, or `Config`.

The acceptance test (§7) enforces this with a fixture: `TestUpgradeFromV0_6_1_NoVerify_BehavesIdentically` — a v0.6.1 fixture is loaded, every command except `verify` is run, the resulting `.tpatch/` is diffed against v0.6.1 expected output, and any byte-level difference is a test failure.

#### 3.4.7 Concurrency: verify during reconcile

Verify is read-only, but V7 spins up a shadow worktree. ADR-010 D2 reserves shadows for the M12 resolver, scoped per-slug. To avoid two writers contending for the shadow path:

- `tpatch verify <slug>` refuses (`exit 2 — feature is reconciling`) when the feature's state is `reconciling` or `reconciling-shadow`. Listed in the §3.4.2 truth table.
- Verify creates its own shadow with the same `<slug>-<timestamp>` naming convention so any prior verify shadow is reaped per the existing `gitutil.PruneAllShadows` semantics.
- The shadow is **always** pruned in a deferred call; no shadow survives a verify exit.

Verify does NOT block reconcile or vice versa across distinct slugs — the lock is per-slug, mirroring the existing shadow contract.

### 3.5 Interaction with M14 reconcile labels

`tested` composes orthogonally with the M14.3 derived labels. ADR-012 D6 cross-references ADR-011 D3 verbatim: labels are computed at READ time from parent state; `state` is persisted from explicit transitions. The two systems do not interact except via the parent-state hook (§3.4.5).

Specifically:

| Child state | Hard-parent state | Child labels | Child renders as |
|-------------|-------------------|--------------|------------------|
| `tested` | all `applied`/`tested`/`upstream_merged` | (none) | `[tested]` |
| `tested` | one `requested`/`analyzed`/`defined`/`implementing` | `waiting-on-parent` | `[tested] [waiting-on-parent]` |
| `tested` | one `blocked` / `reconciling-shadow` | `blocked-by-parent` | `[tested] [blocked-by-parent]` |
| `tested` (transient before §3.4.5 hook fires) | one `applied` that was just amended | `stale-parent-applied` | `[tested] [stale-parent-applied]` (then hook demotes to `[applied] [stale-parent-applied]`) |

The compound `EffectiveOutcome()` rule (`internal/store/types.go:192`) is **not extended**. `tested` is not a `Reconcile.Outcome`; it is a `FeatureState`. The compound presentation logic is unchanged.

---

## 4. CLI surface

### 4.1 `tpatch verify` flags

Slice A:

| Flag | Default | Description |
|------|---------|-------------|
| `<slug>` | required | The feature to verify. Verify-all (`--all`) is Slice D. |
| `--json` | false | Emit the structured JSON report on stdout. Human report on stderr unless `--quiet`. |
| `--quiet` | false | Suppress human report. Combined with `--json`, only JSON is emitted. |
| `--no-promote` | false | Run all checks but do NOT flip state to `tested` even on green. Read-only mode for harness pre-flight. |
| `--no-demote` | false | Run all checks; on FAIL of a block-severity check, report but do NOT demote `tested → applied`. Read-only mode for harness debugging. |
| `--path` | `.` | Standard tpatch flag — workspace path. |

`--no-promote` and `--no-demote` are independent flags; the harness can opt into either.

**Out of Slice A:**

| Flag | Slice | Description |
|------|-------|-------------|
| `--all` | D | Verify every feature in topological order; aggregate report. |
| `--shadow` | (rejected for v0.6.2) | Force shadow allocation even when no recipe is present. Verify already gates V7 on recipe presence; flag is unnecessary. |
| `--fresh-branch` | (out of scope per §0.3) | Verify against a freshly-checked-out upstream branch. Belongs to `feat-reconcile-fresh-branch-mode`. |

### 4.2 `tpatch amend` interaction

Slice C: `tpatch amend <slug>` with a recipe-touching change demotes `tested → applied` per §3.4.5. **No new amend flag.** Specifically:

- `tpatch amend <slug> --state tested` is rejected (D3). Returns `exit 2 — manual --state tested is not supported; run tpatch verify <slug>`.
- All existing amend flags retain their behaviour. Only the post-amend state-write logic changes: when the prior state was `tested`, the new state is `applied`; when the prior state was anything else, behaviour is unchanged.

### 4.3 `tpatch verify --json` output schema

Slice A. The schema is **frozen at the version field**: every consumer reads `schema_version` and refuses unknown majors.

```json
{
  "schema_version": "1.0",
  "slug": "extra-button",
  "verified_at": "2026-04-27T18:30:11Z",
  "verdict": "passed",
  "promoted": true,
  "demoted": false,
  "exit_code": 0,
  "checks": [
    { "id": "status_loaded",          "severity": "block-abort", "passed": true,  "remediation": "" },
    { "id": "intent_files_present",   "severity": "block",       "passed": true,  "remediation": "" },
    { "id": "recipe_parses",          "severity": "block",       "passed": true,  "remediation": "" },
    { "id": "recipe_op_targets_resolve","severity": "block",     "passed": true,  "remediation": "" },
    { "id": "dep_metadata_valid",     "severity": "block",       "passed": true,  "remediation": "" },
    { "id": "satisfied_by_reachable", "severity": "block",       "passed": true,  "remediation": "" },
    { "id": "dependency_gate_satisfied","severity": "warn",      "passed": true,  "remediation": "" },
    { "id": "recipe_replay_clean",    "severity": "block",       "passed": true,  "remediation": "" },
    { "id": "post_apply_patch_replay_clean","severity": "block", "passed": true,  "remediation": "" },
    { "id": "reconcile_outcome_consistent","severity": "warn",   "passed": true,  "remediation": "" }
  ],
  "state_before": "applied",
  "state_after": "tested",
  "labels": []
}
```

**Failure case 1 — block-severity fail, demotion path:**

```json
{
  "schema_version": "1.0",
  "slug": "extra-button",
  "verified_at": "2026-04-27T18:31:02Z",
  "verdict": "failed",
  "promoted": false,
  "demoted": true,
  "exit_code": 2,
  "checks": [
    { "id": "status_loaded",          "severity": "block-abort", "passed": true,  "remediation": "" },
    { "id": "intent_files_present",   "severity": "block",       "passed": true,  "remediation": "" },
    { "id": "recipe_parses",          "severity": "block",       "passed": true,  "remediation": "" },
    { "id": "recipe_op_targets_resolve","severity": "block",     "passed": false,
      "remediation": "recipe op #2 path 'src/extras/button.css' missing and created_by empty; declare created_by=button-component or apply button-component" },
    { "id": "dep_metadata_valid",     "severity": "block",       "passed": true,  "remediation": "" },
    { "id": "satisfied_by_reachable", "severity": "block",       "passed": true,  "remediation": "" },
    { "id": "dependency_gate_satisfied","severity": "warn",      "passed": true,  "remediation": "" },
    { "id": "recipe_replay_clean",    "severity": "block",       "passed": false,
      "remediation": "recipe op #2 failed in shadow replay: open src/extras/button.css: no such file or directory" },
    { "id": "post_apply_patch_replay_clean","severity": "block", "passed": true,  "remediation": "" },
    { "id": "reconcile_outcome_consistent","severity": "warn",   "passed": true,  "remediation": "" }
  ],
  "state_before": "tested",
  "state_after": "applied",
  "labels": []
}
```

**Failure case 2 — `satisfied_by` drift since edit time (V5):**

```json
{
  "schema_version": "1.0",
  "slug": "extra-button",
  "verified_at": "2026-04-27T18:32:14Z",
  "verdict": "failed",
  "promoted": false,
  "demoted": false,
  "exit_code": 2,
  "checks": [
    { "id": "satisfied_by_reachable", "severity": "block", "passed": false,
      "remediation": "satisfied_by SHA abc123…0def for parent button-component is no longer reachable from HEAD (drift since edit); re-run tpatch amend extra-button --remove-depends-on button-component --depends-on button-component" }
  ],
  "state_before": "applied",
  "state_after": "applied",
  "labels": []
}
```

(Truncated `checks[]` for brevity in the example; full implementation always emits all 10.)

**Failure case 3 — warn-only fails, green verdict, promotion still happens:**

```json
{
  "schema_version": "1.0",
  "slug": "extra-button",
  "verified_at": "2026-04-27T18:33:55Z",
  "verdict": "passed",
  "promoted": true,
  "demoted": false,
  "exit_code": 0,
  "checks": [
    { "id": "dependency_gate_satisfied","severity": "warn", "passed": false,
      "remediation": "hard parent button-component in state=defined (warn-only at verify time)" },
    { "id": "reconcile_outcome_consistent","severity": "warn", "passed": false,
      "remediation": "reconcile outcome is shadow-awaiting; tested cannot be promoted while reconcile is in a blocked state (warn-only)" }
  ],
  "state_before": "applied",
  "state_after": "tested",
  "labels": ["waiting-on-parent"]
}
```

Wait — the third case contradicts itself: V9 says "tested cannot be promoted while reconcile is in a blocked state" but `state_after = tested` and `verdict = passed`. **Resolved**: V9 is `warn`, so it does not gate promotion. The remediation message text is informational; the field name `passed=false` reflects "the reconcile outcome is not in the healthy set," not "this gates promotion." This subtle contract is documented in §3.1.2 V9 — the operator-facing message intentionally describes the situation, not a hypothetical block.

If this UX is judged confusing in review, the alternative is to make V9 a `block`, demoting to applied if the reconcile outcome is `blocked-*` / `shadow-awaiting`. **Open question Q1 (§10).**

### 4.4 Skill / harness updates

Slice D updates all 6 skill formats with a one-paragraph addition under the "Lifecycle" or equivalent section:

> **Verify before reconcile.** When you finish `tpatch apply` and want a cheap, machine-checkable signal that the feature is structurally healthy, run `tpatch verify <slug>`. On green this promotes the feature to state `tested`; on a block-severity failure the feature is demoted from `tested` back to `applied` and the failing check is reported. Verify is read-only — it never touches the working tree. It does **not** run the project's test suite; for that, use `tpatch test`. Verify is the natural pre-handoff phase between implement and reconcile in agentic harnesses.

Anchor list (parity-guard `assets/assets_test.go` extension):

- `assets/skills/claude/tessera-patch/SKILL.md` — Lifecycle section
- `assets/skills/copilot/tessera-patch/SKILL.md` — Lifecycle section
- `assets/skills/copilot-prompt/...` — Companion prompt
- `assets/skills/cursor/...` — Cursor rules
- `assets/skills/windsurf/...` — Windsurf rules
- `assets/skills/generic/...` — Generic workflow markdown

`tpatch test` is mentioned in the same paragraph to forestall the conflation question.

### 4.5 Status rendering

Slice C: `tpatch status` prints `tested` like any other state. `tpatch status --dag` renders `[tested]` in the state badge slot. No new label is introduced.

---

## 5. Edge cases / failure modes

| Case | Handling |
|------|----------|
| `verify <slug>` on a feature that does not exist | `exit 2 — feature not found`. No state mutation. Same shape as today's `apply <unknown-slug>`. |
| `verify <slug>` on a feature whose `status.json` is malformed | V0 fails (block-abort); `exit 2 — internal error`. No state mutation. Operator must hand-edit. |
| Recipe absent (`apply-recipe.json` missing) | V2/V3/V7 are skipped. V1/V4/V5/V6/V8/V9 still run. Verdict reports "skipped" in those checks. `tested` flip is allowed if no block-severity check failed. |
| `post-apply.patch` absent | V8 is skipped (same model as V2/V3/V7 absent). |
| Both recipe and patch absent | Verify still runs static checks (V1/V4/V5/V6/V9). Reasonable if the feature is in `applied` from before the autogen-recipe era; the operator gets a green verify, and `tested` flips. |
| `verify` during a concurrent `reconcile` on the same slug | refused per §3.4.7. |
| `verify` while the shadow path is occupied by an old reconcile shadow | verify reaps via `gitutil.PruneAllShadows` (existing semantics). Logged to stderr. |
| Verify inside a non-tpatch-init repo | `exit 2 — not a tpatch workspace`, same as every other tpatch verb. |
| Verify on a child whose parent has cycle drift (manual `status.json` edit introduced cycle) | V4 fails (block); demotion + non-zero exit. The cycle is also surfaced by `tpatch status` per M14.4 chunk D. |
| `--no-promote --no-demote` combined | Both honoured; verify runs read-only on state. Useful for CI gating. |
| Repo with `Config.FeaturesDependencies = false` | V4 still runs (`ValidateDependencies` is sound for empty dep lists). V5 is a no-op (no `satisfied_by` to check). V6 is a no-op (per `CheckDependencyGate` early return). The DAG-disabled path is byte-identical to v0.6.1. |
| Verify during an in-flight reconcile **on a different slug** | Allowed; per-slug shadow lock. |

---

## 6. Open questions / decisions

### Q1 — V9 (`reconcile_outcome_consistent`) severity: warn vs block?

**Default in this PRD: warn.** Rationale: a feature can be perfectly structurally healthy while sitting in `Reconcile.Outcome = shadow-awaiting`; the operator hasn't accepted/rejected the shadow yet, but the feature on disk is intact. Demoting on V9 would make `tested` un-reachable for any feature with a pending reconcile, even though that pending reconcile says nothing about the feature's structural integrity.

**Alternative (revisit if reviewer pushes back):** make V9 a block. Pros: stronger contract — `tested` implies "no pending reconcile work." Cons: the harness loses the ability to verify a feature that has a `shadow-awaiting` reconcile (a perfectly normal state). **Surfaced for reviewer adjudication; default kept warn.**

### Q2 — Parent-state demotion cascade depth

**Locked in §3.4.5 / ADR-012 D5: cascade is one step (parents → direct children only).** Grandchildren are not demoted because their parents flip from `tested` to `applied`, which is itself a satisfying state.

Argued but rejected: a transitive cascade. Rejected because the intended semantics of the demotion is "this child's parent stopped being `tested`, so the child's verify result is no longer guaranteed." Grandchild's verify is guaranteed against the child's `tested` state — and the child is still `applied`, which is the dep-gate equivalent of `tested` (per D2). No transitive invalidation needed.

### Q3 — Should `verify --all` include features in `requested`/`analyzed`/`defined`/`implementing`?

**Default: skip them with a per-slug "skipped: pre-apply state" line in the report, exit 0 if all post-apply slugs pass.** Rationale: `tested` is meaningless before `applied`. Slice D detail; not blocking Slice A.

### Q4 — What happens to `tested` when a feature is `tpatch remove`d?

**Untouched: `remove` deletes the feature directory entirely; there is no state to preserve.** No new behaviour.

### Q5 — Does the `tested` state appear in `--json` `status` output today?

Yes — `tpatch status --json` will emit `state: "tested"` automatically once the enum gains the value. The schema is enum-by-string; adding a new value does not break readers that handle unknowns gracefully (per existing `tpatch status --json` consumer contract). Slice C adds an acceptance test.

### Q6 — `tpatch verify` exit codes

- `0` — verdict=passed, promoted (or already `tested`), all warn-only fails surfaced.
- `2` — verdict=failed (any block-severity check); also covers V0 abort, invalid state for verify, non-existent slug.
- `1` — reserved for "verify aborted by signal / context cancellation"; no state mutation.

Stable across slices; documented in `--help`.

### Q7 — Can `verify` be run as part of `tpatch cycle <slug>` (the Phase-2 lifecycle command)?

Out of Slice A; revisit when `cycle` is updated to handle the `tested` state. Safe default: `cycle` does not run `verify`; operator runs `verify` separately if they want the `tested` flip.

---

## 7. Acceptance criteria (combined verify + tested ships when…)

- [ ] **ADR-012 merged** before any Wave 3 implementation slice lands.
- [ ] `go build ./...`, `go test ./...`, `gofmt -l .` all clean.
- [ ] `FeatureState` enum includes `StateTested`. v0.6.1 fixtures round-trip byte-identical.
- [ ] `tpatch verify <slug>` runs the 10-check sequence in order, with the severities documented in §3.1.
- [ ] On green, state transitions `applied → tested`; idempotent on `tested → tested`.
- [ ] On block-severity fail, state transitions `tested → applied` with stderr demotion line; warn-only fail does NOT demote.
- [ ] `--no-promote` runs the checks but does not flip on green; `--no-demote` runs the checks but does not flip on fail; both flags are honoured independently.
- [ ] `--json` emits the schema in §4.3 with exact field names; `schema_version: "1.0"` is present.
- [ ] V0 abort produces `exit 2 — internal error` and writes nothing to `status.json`.
- [ ] V4 reuses `store.ValidateDependencies` (no parallel validator).
- [ ] V5 reuses the v0.6.1 `satisfied_by` 40-hex + `gitutil.IsAncestor` reachability contract; drift since edit time is detected.
- [ ] V6 reuses `workflow.CheckDependencyGate`; soft parents are silent; hard parents emit warn.
- [ ] V7 spins up a `gitutil.CreateShadow` worktree, runs the recipe in shadow only, prunes the shadow before verify exits regardless of outcome.
- [ ] V8 uses `git apply --check` (no working-tree mutation) against the upstream baseline.
- [ ] V9 reads `status.Reconcile.Outcome` only — never `artifacts/reconcile-session.json`. Adversarial test pins this.
- [ ] `verify` during in-flight reconcile (state ∈ `{reconciling, reconciling-shadow}`) refuses with `exit 2`.
- [ ] Parent-state hook: when a hard parent flips out of `{applied, tested, upstream_merged}`, every direct child currently `tested` demotes to `applied`. Cascade depth = 1; verified by a 3-tier DAG test (root, mid, leaf — only mid demotes when root regresses).
- [ ] `amend (recipe-touching) on tested` demotes to `applied`; `amend (intent-only)` preserves `tested`.
- [ ] `apply --mode execute on tested` demotes to `applied`.
- [ ] `reconcile (reapplied/upstreamed) on tested` produces `applied`, NOT `tested`.
- [ ] `tested` parent satisfies the hard-dep gate (`CheckDependencyGate` extension; D2-pragmatic).
- [ ] Skill bullet present in all 6 surfaces; parity guard (`assets/assets_test.go`) green.
- [ ] **Backwards compat:** `TestUpgradeFromV0_6_1_NoVerify_BehavesIdentically` — v0.6.1 fixture, all v0.6.1 commands run except `verify`, resulting `.tpatch/` is byte-identical to v0.6.1 expected.
- [ ] **Source-truth guard:** adversarial test asserts the verify implementation does NOT import or read `artifacts/reconcile-session.json` or `artifacts/resolution-session.json` at any code path.
- [ ] CHANGELOG v0.6.2 callout names `verify` and `tested` with exact contract surface.

---

## 8. Risks and mitigations

| Risk | Mitigation |
|------|------------|
| Operator confusion: "I ran `verify` but my tests didn't run." | §1.3 wording in skill bullet calls this out. CHANGELOG explicit. `tpatch test` and `tpatch verify` are distinct verbs. |
| Demotion surprise: a `tested` feature flips back to `applied` after upstream change. | Logged to stderr with a reason string. Status `--dag` `stale-parent-applied` label is the visual cue. Documented in §3.4.5. |
| V7 shadow contention with reconcile on the same slug. | §3.4.7: verify refused while reconcile is in flight. Per-slug lock. |
| V7 shadow leak on crash. | `defer PruneShadow(...)` in the verify entry point; `PruneAllShadows` called at start to reap stale shadows from prior crashed runs. |
| `tested` enum value breaks downstream JSON consumers that hard-code the v0.6.1 enum set. | Existing `--json` output for `tpatch status` already documents the enum-by-string contract. Adding a value is an additive change; consumers that did not handle unknowns are already broken. CHANGELOG callout. |
| Parent-state hook cost on large DAGs. | Hook runs in the same loop as the M14.3 label recomputation — same O(V+E) walk. No new hot path. |
| D2-pragmatic gate change misread as "`tested` is required for apply gate." | The PRD §3.4.4 + ADR-012 D2 explicit text covers this; `applied` parents continue to satisfy without change. |
| Verify run as part of CI before reconcile passes — false confidence. | V9 surfaces reconcile state; warn-only by default but visible. Q1 left open for reviewer. |

---

## 9. Implementation slices (downstream Wave 3 dispatches)

The dispatch contract names four slices (A: command shell, B: state plumbing, C: wiring, D: polish). This PRD adopts those boundaries with explicit scope per slice. Each slice is small enough to dispatch as a single implementer task with its own mini-handoff.

### Slice A — Verify command shell + checks V0-V9 (no state transitions)

- New file `internal/cli/verify.go` (registered under `cmd/tpatch/main.go` via the existing cobra root).
- New file `internal/workflow/verify.go` carrying `RunVerify(s *store.Store, slug string, opts VerifyOptions) (*VerifyReport, error)` and the 10 individual check functions (one per V*).
- All checks reuse existing primitives — no new validators.
- Shadow worktree spin-up uses existing `gitutil.CreateShadow` + `defer PruneShadow`.
- `--json` schema per §4.3.
- **No state mutation.** Slice A delivers `--no-promote --no-demote` behaviour as the *only* behaviour; `tested` does not exist yet at the enum level.
- Tests: per-check pass/fail unit tests using table-driven fixtures; `--json` golden output; concurrent-with-reconcile refusal.
- Acceptance: PRD §7 entries for V0–V9 plus `--json` schema.

### Slice B — `tested` enum + state-transition plumbing (no verify wiring)

- `FeatureState` enum gains `StateTested` in `internal/store/types.go`.
- `CheckDependencyGate` extended: `case StateApplied, StateTested:` arm (D2-pragmatic).
- `apply --mode execute on tested → applied` demotion in the apply path.
- `amend (recipe-touching) on tested → applied` demotion in the amend path; `amend (intent-only)` preserves `tested` (existing path is touched only with a guard).
- `reconcile on tested` produces `applied` (not `tested`) on green.
- Parent-state hook: in the `LoadFeatureStatus` post-processing that already recomputes `Reconcile.Labels`, when a hard parent leaves the `{applied, tested, upstream_merged}` set and the child is currently `tested`, demote.
- Tests: state-machine truth-table per §3.4.2 (every cell asserted); v0.6.1 round-trip byte-identity; `tested` parent satisfies dep gate.
- Acceptance: PRD §7 enum, transitions, gate-satisfies, round-trip entries.

### Slice C — Wire verify into state transitions + `amend` interaction

- Verify command (Slice A) gains the `--no-promote` / `--no-demote` flag wiring; default behaviour now flips state per §3.4.2.
- `tpatch amend --state tested` rejected with the §4.2 error.
- `tpatch status` and `tpatch status --dag` display `tested` correctly (no new label, just the existing state badge).
- `tpatch status --json` includes `state: "tested"` for the relevant features.
- Tests: PRD §7 promotion / demotion / idempotency entries; `amend --state tested` rejection; `status --json` shape.

### Slice D — Skill bullets, harness anchors, parity guard, CHANGELOG, polish

- All 6 skill surfaces gain the §4.4 bullet.
- `assets/assets_test.go` parity guard extended with the new anchors.
- `tpatch verify --all` (Slice D add — was deferred from Slice A).
- `docs/dependencies.md` cross-link to verify (one-paragraph aside near the apply-time gate section).
- `CHANGELOG.md` v0.6.2 entry naming the verb, the new state, and the explicit out-of-scope list.
- Tests: parity-guard green; `verify --all` topo-order test.

Each slice is independently dispatchable; each has its own handoff contract section reusing the §7 acceptance entries scoped to its slice.

---

## 10. Cross-cutting impact matrix

| Other feature / surface | Relationship | Notes |
|-------------------------|--------------|-------|
| `feat-feature-dependencies` (M14, shipped) | **extends** | `CheckDependencyGate` extended for D2-pragmatic. M14.3 labels untouched. |
| `feat-provider-conflict-resolver` (M12, shipped) | **independent** | Verify never calls the resolver. Shadow worktrees are per-slug; verify and reconcile cannot collide on the same slug (§3.4.7). |
| `tpatch amend` (M13, shipped) | **extends** | Recipe-touching amend demotes `tested → applied`. `--state tested` rejected. No new flag. |
| `tpatch test` (existing command) | **independent** | Distinct verb; not a producer of `tested` (D3). |
| `tpatch reconcile` | **extends** | Green reconcile from `tested` produces `applied` (not `tested`). |
| `tpatch status` / `--dag` / `--json` | **extends** | Renders `tested` as a state badge. No new label. |
| `assets/assets_test.go` | **extends** | New skill anchor for the verify/tested bullet. |
| `docs/dependencies.md` | **extends** (one paragraph) | Cross-links verify in the apply-time-gate section. |
| `feat-reconcile-code-presence-verdicts` | **out of scope** | Distinct PRD; reuses some primitives but does not depend on verify. |
| `feat-reconcile-fresh-branch-mode` | **out of scope** | Distinct PRD; verify deliberately does NOT add `--fresh-branch`. |
| `delete-file` recipe op | **out of scope** | Recipe-op JSON schema frozen (CHANGELOG v0.6.1). Verify tolerates deletions in shadow replay the same way recipe autogen does. |
| `feat-tested-state-test-producer` (future) | **enables** | Once `tpatch test` integration is scoped, `test` may join `verify` in the producer set. ADR-012 D3 lists this as the future-work expansion. |

---

**End of PRD.** Implementation handoff for Slice A will live in `docs/handoff/CURRENT.md` once this PRD + ADR-012 are reviewed and approved.
