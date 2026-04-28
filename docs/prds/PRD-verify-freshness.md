# PRD — `tpatch verify` and verification freshness overlay

**Status**: Approved (M15 Wave 3 — APPROVED WITH NOTES at commit 3c122aa; Slice A in implementation. Supersedes `docs/prds/PRD-verify-and-tested-state.md`.)
**Date**: 2026-04-27
**ADR**: **ADR-013-verify-freshness-overlay.md — REQUIRED before Wave 3 implementation slices ship.** ADR-013 supersedes ADR-012 in full.
**Owner**: Core
**Milestone**: M15 → Wave 3 (lifecycle / reconcile semantics tranche)
**Related**: ADR-010 (M12 resolver), ADR-011 (feature DAG), ADR-013 (this PRD's binding ADR), `docs/prds/PRD-verify-and-tested-state.md` (superseded predecessor), `docs/dependencies.md`, `docs/prds/PRD-feature-dependencies.md`, CHANGELOG v0.6.1

> **This PRD supersedes [`docs/prds/PRD-verify-and-tested-state.md`](PRD-verify-and-tested-state.md) in full.**
> The predecessor encoded `tested` as a forward `FeatureState` lifecycle value.
> An external re-review of the approved design (commit `8c3d72e`) found two
> structural problems: F1 (verify shadows ignored the parent-replay closure)
> and F4 (the design conflated lifecycle with verification freshness, routing
> state mutation through a read path). The supervisor reopened Wave 3 with a
> binding redesign: a Git-like freshness overlay that keeps lifecycle
> untouched. ADR-013 holds the architecture decisions; this PRD captures the
> _what_ and the _why-now_. The reopening note is appended to
> `docs/supervisor/LOG.md` (2026-04-27 entry).

---

## 0. Meta

### 0.1 Why one PRD covers two backlog items

`feat-verify-command` and `feat-feature-tested-state` were originally scoped together because they share contract surface. The first revision approved on `8c3d72e` collapsed them into a single PRD. A re-review of that revision identified two structural problems (F1 — verify shadows ignored the parent-replay closure; F4 — the design conflated lifecycle with verification freshness and routed state mutation through a read path). The supervisor adjudicated both with a binding redesign:

- Verify produces a **freshness overlay** on `FeatureStatus`, not a new lifecycle state.
- The shadow workspace replays the **hard-parent topological closure** before applying the target's recipe.

This second revision pass rewrites the PRD around those two decisions. Slug-naming convention change: `feat-feature-tested-state` is renamed in spirit to `feat-feature-verify-freshness` (the backlog row keeps its slug; the contract behind it is the freshness overlay).

### 0.2 Architecture decisions to lock in ADR-013

This PRD records the _what_ and the _why-now_. Per repo rule (`AGENTS.md` → "Context Preservation Rules" §6) every architecture choice here is captured in **ADR-013-verify-freshness-overlay.md** before any Wave 3 implementer slice begins. The seven decisions, all rewritten for the freshness model:

1. **D1** — freshness overlay model: verify writes a `Verify` sub-record on `FeatureStatus`. The `FeatureState` enum is unchanged.
2. **D2** — apply gate stays pure-lifecycle. Satisfaction set remains `{applied, upstream_merged}`. Freshness does **not** alter the gate.
3. **D3** — producer set: `verify` writes the freshness record; `amend` invalidates it. `test` is not a producer.
4. **D4** — backwards-compatibility contract: `Verify` is `omitempty`-marshalled; v0.6.1 repos round-trip byte-identical until verify runs.
5. **D5** — derived label transitions: four read-time-derived labels (`never-verified`, `verified-fresh`, `verified-stale`, `verify-failed`). No persisted lifecycle transitions.
6. **D6** — source-truth alignment with ADR-011 D6 / ADR-010 D5: the freshness record lives in `status.json`; never inferred from any artifact.
7. **D7** — `verify` is read-only on the working tree. Apply-simulation runs in a `gitutil` shadow worktree that **first replays the target's hard-parent topological closure** before applying the target's recipe (F1 closure-replay spec).

Any deviation during implementation requires an ADR-013 amendment before the slice merges.

### 0.3 Out of scope (cross-linked)

- `feat-reconcile-code-presence-verdicts` — making reconcile assert that recipe ops are still represented in HEAD. Verify reuses the same shadow primitive but does not change reconcile's verdict set.
- `feat-reconcile-fresh-branch-mode` — running reconcile against a freshly-checked-out upstream branch. `tpatch verify --fresh-branch` is not in scope.
- **`delete-file` recipe op.** The recipe-op JSON schema is frozen (CHANGELOG v0.6.1 Notes). Verify's recipe-replay check tolerates deletions the same way recipe autogen does (skip with stderr note); a real `delete-file` op needs its own ADR.
- **Anything that reads `artifacts/reconcile-session.json`.** Verify reads `status.Reconcile.Outcome` only (ADR-010 D5 + ADR-011 D6).
- **Lifecycle state changes.** The `FeatureState` enum is not extended. Verify never mutates `state`.

---

## Summary

`tpatch verify <slug>` is a **read-only**, machine-checkable health command that runs every static and apply-simulation check we already know how to run for a single feature, prints a pass/fail report (human and `--json`), and writes a **freshness record** (`Verify` sub-record on `FeatureStatus`) capturing `verified_at`, `passed`, hashes of the recipe + patch as observed at verify time, and a snapshot of every hard parent's lifecycle state. The lifecycle state (`FeatureState`) is **never mutated** by verify.

> **Persistence note (LOG entry `3c122aa` Note 1).** The per-check
> `check_results` array is stdout-only on `--json`; it is **not**
> persisted to the `Verify` record. The persisted shape is the minimal
> set above — see `internal/store/types.go` `VerifyRecord` and §3.4.1.

A derived freshness label (`never-verified` / `verified-fresh` / `verified-stale` / `verify-failed`) is recomputed every time `ComposeLabels` (`internal/workflow/labels.go:89`) runs — purely at read time, from the freshness record + the current DAG snapshot. Drift in the recipe, the patch, or any hard parent's state flips the derived label to `verified-stale` without rewriting `status.json`.

The Git-like analogy: lifecycle states are commits (sticky, persisted, mutated only by explicit verbs); freshness is `git status` for the verify check (derived, read-time, no persistence beyond the last record).

Verify reuses the v0.6.1 primitives end-to-end:

- `store.ValidateDependencies` / `store.ValidateAllFeatures` (M14.1) for dependency hygiene.
- `store.satisfiedBySHA` regex + reachability check (v0.6.1 — `internal/store/validation.go:38–44, 101–108`) re-run for drift since edit time.
- `gitutil.IsAncestor` (`internal/gitutil/gitutil.go:680`) for parent-SHA reachability and `satisfied_by` revalidation.
- `gitutil.CapturePatchScoped` / `CaptureDiffStatScoped` (`internal/gitutil/gitutil.go:216`) for any drift-vs-recorded checks.
- `gitutil.CreateShadow` / `PruneShadow` (`internal/gitutil/shadow.go:56,122`) for apply-simulation in a throwaway worktree.
- `store.TopologicalOrder` (`internal/store/dag.go:107`) for the closure-replay ordering required by V7/V8.

The single schema change is the additive `Verify *VerifyRecord` field on `FeatureStatus`, with `omitempty`. No new file, no new artifact directory, no new `Reconcile.Outcome` value, no new `FeatureState` value.

---

## 1. Problem statement

### 1.1 What's missing today

Through v0.6.1 a feature reaches `applied` after `tpatch apply --mode execute` succeeds and stays there indefinitely. We have no way for an operator (or harness) to ask the cheap, structural question: "is this feature still healthy against the current tree?" The relevant signals are split across:

- **Static:** `spec.md` and `exploration.md` exist and reference real paths.
- **Recipe-shape:** `apply-recipe.json` (if present) parses; deps resolve; op targets resolve.
- **Apply-simulation:** the recipe still re-applies cleanly to a fresh shadow worktree **whose baseline already carries every hard parent's recipe replayed in topological order**.
- **Patch-replay:** `artifacts/post-apply.patch` still applies to that same closure-replayed baseline.
- **Dependency hygiene:** parent slugs exist, no cycles, `satisfied_by` SHAs still 40-hex AND reachable.
- **DAG context:** parent state is one of the apply-gate satisfying values.

Today these checks are scattered: dep validation runs at write time and during `status`; recipe parsing runs during `apply`; patch reverse-apply happens inside `reconcile`. There is no single command that runs them all without side effects, and there is no command that runs the apply-simulation against a baseline whose *parents* have first been replayed.

### 1.2 What the freshness overlay is for

The freshness overlay answers two questions:

1. **Did verify ever pass for this feature against the current world?** — operator/harness asks `verified-fresh ∈ labels`. This is needed because the harness must distinguish "the feature is `applied` and last verify said GREEN against unchanged inputs" from "the feature is `applied` and verify has never run."
2. **Has the world moved since the last verify?** — `verified-fresh` flips to `verified-stale` automatically when any of (a) the recipe hash, (b) the patch hash, (c) any hard parent's lifecycle state has drifted away from what was captured in the freshness record. No write to `status.json` is required to detect this; the derivation runs at read time.

The harness composition pattern is therefore: `tpatch verify parent && tpatch apply child`. The harness reads `verified-fresh` for the parent from `tpatch status --json` and decides whether to re-run verify on the parent before composing. The CLI itself does not enforce — gate-level enforcement is reserved for lifecycle states (see D2).

### 1.3 What `verify` is NOT

- `verify` is **not** `apply`. It does not write to the working tree. Apply-simulation runs in a `gitutil.CreateShadow` worktree that is pruned before exit.
- `verify` is **not** `reconcile`. It does not call the provider, does not run phase 3.5, does not produce a `reconcile-session.json`.
- `verify` is **not** `test`. It does not run `Config.TestCommand`. `tpatch test` remains the project-test runner.
- `verify` does **not** mutate `FeatureState`. Lifecycle is owned by `apply` / `amend` / `reconcile`. The freshness overlay is orthogonal.

---

## 2. Goals / non-goals

### Goals

- One read-only command, `tpatch verify <slug>`, that runs a fixed, ordered list of checks (§3.1) and produces both a human summary and a `--json` shape (§4.3).
- A new `Verify` sub-record on `FeatureStatus` (§3.4.1), produced by `verify`, invalidated by `amend`, never read by gates.
- Four derived freshness labels (§3.4.2), recomputed at read time by `ComposeLabels` (`internal/workflow/labels.go:89`).
- Hard-parent topological closure replay in V7/V8 (§3.4.3), so verify is structurally meaningful for non-leaf features.
- Byte-identical round-trip for v0.6.1 repos that never run `verify` (§3.4.4).
- Reuse of every existing primitive that overlaps a verify check (§3.1) — no new validation logic where the store already has it.

### Non-goals (explicit)

- **Provider calls.** Verify is offline by construction.
- **Verify mutates `FeatureState`.** It does not. There is no demote-on-fail, no promote-on-pass at the lifecycle level.
- **Apply gate consults freshness.** It does not. The gate stays pure-lifecycle (D2).
- **`tpatch test` integration in Wave 3.** `test` is not a producer of the freshness record; deferred to `feat-tested-state-test-producer`.
- **`amend --state tested`.** Manual flip is rejected; `tested` is not a state any more.
- **Code-presence reconcile verdicts.** Out of scope (§0.3).
- **Fresh-branch reconcile mode.** Out of scope (§0.3).
- **Schema additions to recipe-op JSON.** Frozen.
- **A new `verify-session.json` artifact.** Verify writes only `status.json`.

---

## 3. Specification

### 3.1 The check list — exact order, primitives, severity

`tpatch verify <slug>` runs the checks below. Severities:

- **`block`** — failure produces a non-zero exit and `passed=false` in the freshness record. Verify continues running so the operator gets the full picture.
- **`warn`** — failure is reported in the per-check entry but does not flip `passed` to `false`.

V0 aborts early — a feature whose `status.json` cannot be loaded cannot be verified meaningfully (`exit 2 — internal error`, no record written).

| #  | Check id                       | Trigger                            | Severity    | Pass criterion | Reuses |
|----|--------------------------------|------------------------------------|-------------|----------------|--------|
| V0 | `status_loaded`                | always                             | block-abort | `store.LoadFeatureStatus(slug)` returns OK | `internal/store/store.go:232` |
| V1 | `intent_files_present`         | always                             | block       | `spec.md` and `exploration.md` exist on disk under `.tpatch/features/<slug>/` and are non-empty | direct fs |
| V2 | `recipe_parses`                | recipe present                     | block       | `apply-recipe.json` parses through canonical unmarshal with `DisallowUnknownFields` | existing recipe loader |
| V3 | `recipe_op_targets_resolve`    | recipe present                     | block       | every op's `Path` exists OR carries a `created_by` whose parent is a declared **hard** dep currently in `applied`/`upstream_merged` | M14.2 `created_by` semantics, `internal/workflow/created_by_gate.go:57` |
| V4 | `dep_metadata_valid`           | always                             | block       | `store.ValidateDependencies(s, slug, status.DependsOn)` returns nil | `internal/store/validation.go:66` |
| V5 | `satisfied_by_reachable`       | every dep with `satisfied_by` set  | block       | `store.satisfiedBySHARe.MatchString` AND `gitutil.IsAncestor(repoRoot, dep.SatisfiedBy, "HEAD")` true | `internal/store/validation.go:101–108`, `internal/gitutil/gitutil.go:680` |
| V6 | `dependency_gate_satisfied`    | always (gated on `Config.DAGEnabled()`) | warn   | `workflow.CheckDependencyGate(s, slug)` returns nil | `internal/workflow/dependency_gate.go:42` |
| V7 | `recipe_replay_clean`          | recipe present                     | block       | After replaying the **hard-parent topological closure** of the target into a `gitutil.CreateShadow` worktree (§3.4.3), the target's recipe replays cleanly | `gitutil.CreateShadow`, `PruneShadow`, `store.TopologicalOrder`, recipe executor |
| V8 | `post_apply_patch_replay_clean`| `artifacts/post-apply.patch` present | block     | After the same closure replay used by V7, `git apply --check` of `post-apply.patch` succeeds against the closure-replayed shadow | shared shadow with V7; `git apply --check` |
| V9 | `reconcile_outcome_consistent` | `status.Reconcile.Outcome` set     | warn        | `Outcome ∈ {reapplied, upstreamed, still_needed}` | reads `status.Reconcile.Outcome` only — never any artifact (D6) |

#### 3.1.1 Ordering rationale

V0 → V6 are **static**: file checks, struct unmarshals, regex/git ancestor, in-process function calls. V7 and V8 are **dynamic** (shadow worktree + parent closure replay + recipe/patch apply). Static block runs first so a recipe-shape error doesn't waste a shadow allocation. V7 and V8 share a single shadow allocation: the closure is replayed once, the target recipe is applied, then `git apply --check` is run for the patch. V9 is last — informational read of `status.Reconcile.Outcome`.

#### 3.1.2 Remediation messages

Every fail surfaces a one-line remediation:

- V1 → `"file 'spec.md' missing or empty; re-run tpatch define <slug>"`
- V2 → `"apply-recipe.json failed to parse: <err>; fix the recipe or re-run tpatch implement <slug>"`
- V3 → `"recipe op #<n> path '<p>' missing and created_by empty; declare created_by=<parent> or apply <parent>"`
- V4 → wraps the validation sentinel verbatim.
- V5 → `"satisfied_by SHA <sha> for parent <slug> is no longer reachable from HEAD; re-run tpatch amend <slug> --remove-depends-on <parent> --depends-on <parent>"`
- V6 → `"hard parent <slug> in state=<state> (warn-only at verify time)"`
- V7 → `"recipe op #<n> failed in shadow replay: <err>; investigate or re-run tpatch implement <slug>"`
- V7 (parent-replay failure) → `"hard parent <slug> failed to replay in shadow: <err>; re-run tpatch verify <slug> on the parent first"` — see §3.4.3 for the exact JSON shape.
- V8 → `"post-apply.patch no longer applies to closure-replayed baseline; run tpatch reconcile <slug>"`
- V9 → `"reconcile outcome is <outcome>; verify cannot vouch for reconcile health (warn-only)"`

### 3.2 What `verify` writes

Verify is **read-only on the working tree** by ADR-013 D7. It writes exactly one thing, to the store:

1. The `Verify` sub-record on `FeatureStatus` (§3.4.1), including `verified_at`, `passed`, `recipe_hash_at_verify`, `patch_hash_at_verify`, and `parent_snapshot`. `LastCommand = "verify"` and `UpdatedAt` are bumped per existing `store.SaveFeatureStatus` semantics.

The per-check `check_results` array is **NOT persisted** — it is built in-memory and emitted on `--json` stdout only, per LOG entry `3c122aa` Note 1 (the authoritative disposition). The persisted record carries only the minimal field set needed by Slice B's read-time `ComposeLabels` derivation; the full diagnostic array is harness-consumable via `tpatch verify --json` stdout.

It writes **no** new file in `artifacts/`, no new file under `.tpatch/`, no new entry in `patches/`. The shadow worktree it spins up for V7/V8 is pruned before verify exits, regardless of pass/fail.

`FeatureState` is **not** mutated. There is no demote, no promote.

### 3.3 Dependency-gate severity is `warn`, not `block` — rationale

V6 (`dependency_gate_satisfied`) is warn because verify must be useful in two scenarios in which the hard-parent gate would fail despite the feature being structurally healthy:

1. **Pre-apply harness handoff.** A child whose hard parent is still `defined` is structurally fine. The harness wants to know "is this slug ready" before applying the parent.
2. **`upstream_merged` parent without `satisfied_by`.** Verify can detect structural drift via V5; the dep gate's behaviour for `upstream_merged` is to accept regardless. Warn lets V5 do the precise work and V6 echo context.

### 3.4 The verification freshness overlay

#### 3.4.1 The `Verify` sub-record on `FeatureStatus`

A new field is added to `FeatureStatus` (`internal/store/types.go:91`):

```go
type FeatureStatus struct {
    // … existing fields unchanged …
    Verify *VerifyRecord `json:"verify,omitempty"`
}

type VerifyRecord struct {
    VerifiedAt          time.Time           `json:"verified_at"`
    Passed              bool                `json:"passed"`
    RecipeHashAtVerify  string              `json:"recipe_hash_at_verify,omitempty"`
    PatchHashAtVerify   string              `json:"patch_hash_at_verify,omitempty"`
    ParentSnapshot      map[string]FeatureState `json:"parent_snapshot,omitempty"`
}

// VerifyCheckResult is built in-memory and emitted on --json stdout.
// It is NOT persisted to status.json (LOG entry 3c122aa Note 1).
type VerifyCheckResult struct {
    ID          string `json:"id"`
    Severity    string `json:"severity"`     // "block" | "block-abort" | "warn"
    Passed      bool   `json:"passed"`
    Remediation string `json:"remediation,omitempty"`
}
```

`Verify` is `omitempty`-marshalled: a `nil` pointer round-trips byte-identically with v0.6.1 status.json (D4). Hashes are SHA-256 of the canonical bytes of `apply-recipe.json` and `artifacts/post-apply.patch` respectively, computed at verify time. `ParentSnapshot` is keyed by parent slug; values are the parent's `FeatureState` as observed when verify ran.

The persisted record deliberately does **not** carry the per-check array. The full 10-row check results live on the in-memory `VerifyReport` and are emitted on `tpatch verify --json` stdout only (LOG entry `3c122aa` Note 1 — the authoritative disposition). Slice B's `ComposeLabels` derivation reads only the persisted minimal fields.

`Verify` is **not** a state, **not** a `Reconcile.Outcome`, and **not** an enum value on `FeatureState`. It is a freshness overlay.

#### 3.4.2 Derived freshness labels

The `ReconcileLabel` vocabulary (`internal/store/types.go:50–60`) is extended with four derived labels, recomputed every time `ComposeLabels` (`internal/workflow/labels.go:89`) runs:

| Label | Derivation |
|-------|-----------|
| `never-verified`   | `status.Verify == nil` |
| `verify-failed`    | `status.Verify != nil && status.Verify.Passed == false` |
| `verified-fresh`   | `status.Verify != nil && status.Verify.Passed == true` AND `recipe_hash_at_verify` matches `sha256(current apply-recipe.json)` (or both absent) AND `patch_hash_at_verify` matches `sha256(current post-apply.patch)` (or both absent) AND **for every `(parent_slug, parent_state)` in `parent_snapshot`**, the parent's current state satisfies `parent_state` (state-or-better; see below) |
| `verified-stale`   | `status.Verify != nil && status.Verify.Passed == true` AND any of the freshness conditions for `verified-fresh` is violated |

**State-or-better invariant for parent snapshots.** If the snapshot recorded a parent in `applied`, the parent currently being in `applied`, `upstream_merged` is acceptable (both satisfy the apply gate, so the structural guarantee the verify run leaned on is preserved). If the snapshot recorded a parent in `upstream_merged`, the parent currently being in `upstream_merged` is the only acceptable state (`upstream_merged` is terminal-by-design; transitioning out of it would be a manual-edit anomaly). For pre-apply states (`requested`/`analyzed`/`defined`/`implementing`), being in `applied` or `upstream_merged` is acceptable (the parent has only become more healthy). For `blocked` / `reconciling` / `reconciling-shadow`, any current state other than the recorded one invalidates freshness.

The four labels are **mutually exclusive**: exactly one applies to any given `FeatureStatus`. They live alongside the existing M14.3 labels (`waiting-on-parent`, `blocked-by-parent`, `stale-parent-applied`); compositions like `[verified-fresh, waiting-on-parent]` are valid and rendered by `tpatch status --dag`.

#### 3.4.3 V7/V8 hard-parent topological closure replay

This is the F1 closure-replay spec. Without it, V7/V8 are structurally useless for any non-leaf feature whose parent is locally `applied` (most of the DAG): the shadow worktree's baseline does not contain the parent's changes, so the target's recipe will fail to apply (op targets reference parent-created files; the patch references parent-modified hunks).

**Algorithm.**

1. Compute the hard-parent closure of the target slug: starting from `status.DependsOn`, follow only `DependencyKindHard` edges, transitively, until fixed point.
2. Order the closure deterministically using `store.TopologicalOrder` (`internal/store/dag.go:107`) over the hard-only sub-DAG. Parents appear before children.
3. Filter the ordered list:
   - Parents in `upstream_merged` are **skipped** — their changes are already on the shadow's baseline (the upstream tip), so replaying their recipe would be a no-op or cause double-apply errors.
   - Parents in `applied` are **replayed**: load `apply-recipe.json` for the parent, execute its ops in the shadow worktree.
   - Parents in any other state are a **fail-fast condition**: the closure cannot be reconstructed because the parent has no recorded recipe-replay-ready state. Verify aborts with `failed_at: "parent-replay"` and reports the offending parent and state.
4. After all replayable parents have replayed, apply the **target's** recipe in the same shadow. This is the V7 check.
5. After V7 succeeds, `git apply --check` the target's `post-apply.patch` against the same shadow tree. This is the V8 check.
6. Prune the shadow regardless of pass/fail.

**Fail-fast semantics.** The first parent that fails to replay causes verify to abort the V7/V8 phase (V0–V6 and V9 still run / have run). The freshness record is written with `passed=false` and the V7 entry's `remediation` carries the failing parent slug + wrapped error. The `--json` report includes a top-level `failed_at: "parent-replay"` and a `parent_slug` field; the human report includes the same.

**Example fail-fast JSON fragment.**

```json
{
  "verdict": "failed",
  "failed_at": "parent-replay",
  "parent_slug": "button-component",
  "checks": [
    { "id": "recipe_replay_clean", "severity": "block", "passed": false,
      "remediation": "hard parent button-component failed to replay in shadow: op #3 path 'src/button.tsx' already exists; re-run tpatch verify button-component first" }
  ]
}
```

**Why this is verify-only.** No other code path replays parent closures into shadows. `apply` works against the live tree (parent already applied locally). `reconcile` works against the upstream baseline + the target's own recipe (parents are out of band, by design — see ADR-010 D2). The closure-replay primitive is therefore not factored out into a shared helper; it lives in `internal/workflow/verify.go` only. If a future feature needs the same primitive, an ADR amendment factors it out.

**Cost.** O(closure size) shadow operations per verify. For a leaf with no hard parents, V7/V8 spin up the shadow once and apply only the target. For a 5-deep DAG, V7/V8 replay 5 recipes before the target. The cost is bounded by the DAG depth × per-recipe replay cost, comparable to a phase-2 reconcile op-replay pass per parent. Within the "cheap" budget verify is supposed to deliver for typical 1–3-deep DAGs; for pathologically deep DAGs the operator can verify parents first and rely on `verified-fresh` labels for the harness signal.

#### 3.4.4 Backwards-compatibility contract (D4)

A v0.6.1 repo that never runs `verify` must round-trip every `status.json` byte-identically through v0.6.2 read/write paths. Locked by:

- The `FeatureStatus` schema gains exactly one field: `Verify *VerifyRecord`, with `omitempty`. A `nil` pointer is omitted from the JSON output entirely. v0.6.1 fixtures (where the field never gets set) round-trip byte-identical.
- `FeatureState` enum is unchanged. No new value, no new state-write site.
- `ReconcileSummary` is unchanged. No new field.
- `Config` is unchanged. No new field, no new key in `.tpatch/config.yaml`.
- The `ReconcileLabel` vocabulary gains four values, but labels are **derived at read time** — they are never persisted to `status.json`. A v0.6.1 status.json round-trip never emits any of the new label strings.

Enforced by a regression fixture: `TestUpgradeFromV0_6_1_NoVerify_BehavesIdentically` — load v0.6.1 fixture, run every v0.6.1 command except `verify`, diff `.tpatch/` against v0.6.1 expected output, fail on any byte difference.

#### 3.4.5 Concurrency: verify during reconcile

Verify is read-only at the working-tree level, but V7/V8 spin up a shadow worktree. ADR-010 D2 reserves shadows for the M12 resolver, scoped per-slug:

- `tpatch verify <slug>` refuses (`exit 2 — feature is reconciling`) when the feature's lifecycle state is `reconciling` or `reconciling-shadow`.
- Verify creates its own shadow with the existing `<slug>-<timestamp>` naming convention; any prior verify shadow is reaped via the existing `gitutil.PruneAllShadows` semantics.
- The shadow is **always** pruned in a deferred call.

Verify on slug A does not block reconcile on slug B; the lock is per-slug.

#### 3.4.6 Apply gate: pure lifecycle, freshness ignored — D2

The apply gate (`workflow.CheckDependencyGate`, `internal/workflow/dependency_gate.go:79`) accepts hard parents in `{applied, upstream_merged}`. **This is unchanged in Wave 3.** The freshness overlay does not extend the satisfaction set.

The original first-revision PRD argued whether `tested` should join `{applied, upstream_merged}`. Under the freshness-overlay model that question is moot — there is no `tested` state to satisfy. The supervisor's binding adjudication on F4: lifecycle gates govern persistence, freshness governs harness composition. If the apply gate consulted freshness, it would re-create the demote-on-read problem from a different angle (a child applied at T1 against a `verified-fresh` parent could find its parent's freshness flipped to `verified-stale` at T2 with no operator action — a hidden retroactive change to gate satisfaction). Keeping the gate pure-lifecycle is the Git-like answer.

The harness composition pattern `tpatch verify parent && tpatch apply child` keeps working — but the value is harness-level, not gate-level. The harness reads `verified-fresh` from `tpatch status --json` and decides whether to re-run verify on the parent before composing. The CLI itself does not enforce.

#### 3.4.7 Parent-state hook: read-only label recomputation — F4 collapse

In the first-revision design, a "parent-state hook" was inserted into the M14.3 label-recomputation loop and (under one interpretation) was poised to mutate state. The reviewer flagged this as routing a write through `LoadFeatureStatus`, a read path. Under the freshness-overlay model the hook collapses entirely:

- The hook's role is **only** to recompute `verified-fresh` vs `verified-stale` (and the existing `stale-parent-applied`-style M14.3 labels). All four computations are pure functions of `(child.FeatureStatus, parent.FeatureStatus[])`.
- The hook lives in `composeLabelsFromStatus` (`internal/workflow/labels.go:143`), the existing read-time label computer.
- The hook **never** writes `.tpatch/`. There is no demotion edge, no state mutation, nothing persisted. Drift in a parent's state simply causes the next `ComposeLabels` call to derive `verified-stale` instead of `verified-fresh`. The persisted `Verify` record is unchanged until the next `tpatch verify` runs.

ADR-013 D5 captures the derived-label transitions: `never-verified → verified-fresh` (verify PASS), `verified-fresh → verified-stale` (drift), `verified-fresh → verify-failed` (verify FAIL on next run), `verify-failed → verified-fresh` (verify PASS on next run after the underlying issue is fixed). All transitions are observed by the operator via `ComposeLabels`; none are persisted as transitions.

### 3.5 Interaction with M14 reconcile labels

The freshness labels compose orthogonally with the M14.3 derived labels. ADR-011 D3 invariant is preserved: labels are computed at READ time from parent state. The two systems do not interact.

| Lifecycle state | Freshness label | M14.3 label(s) | Renders as |
|-----------------|-----------------|-----------------|------------|
| `applied` | `verified-fresh` | none | `applied [verified-fresh]` |
| `applied` | `verified-stale` | none | `applied [verified-stale]` |
| `applied` | `verify-failed` | `stale-parent-applied` | `applied [verify-failed, stale-parent-applied]` |
| `applied` | `never-verified` | `waiting-on-parent` | `applied [never-verified, waiting-on-parent]` |
| `upstream_merged` | any | n/a (M14.3 does not apply) | `upstream_merged [<freshness>]` |

The compound `EffectiveOutcome()` rule (`internal/store/types.go:192`) is **not extended**. Freshness labels are derived overlays, not `Reconcile.Outcome` values. The compound presentation logic is unchanged.

`amend` invalidates the freshness record (D3): a recipe-touching amend rewrites the recipe bytes, so `recipe_hash_at_verify` no longer matches; the next `ComposeLabels` derives `verified-stale`. Optionally — and this is the implementation hook in Slice B — `amend` may proactively clear `Verify.Passed` to `false` (effectively a `verify-failed` derived label) so the harness sees the invalidation immediately rather than waiting for the next read. ADR-013 D3 records this as the producer-set rule.

---

## 4. CLI surface

### 4.1 `tpatch verify` flags

Slice A delivers a minimal flag set; richer flags arrive in Slice D.

| Flag | Default | Slice | Description |
|------|---------|-------|-------------|
| `<slug>` | required | A | The feature to verify. |
| `--json` | false | A | Emit the structured JSON report on stdout. Human report on stderr unless `--quiet`. |
| `--quiet` | false | A | Suppress human report. Combined with `--json`, only JSON is emitted. |
| `--no-write` | false | A | Run all checks but do NOT write the `Verify` record. Pure read-only mode. |
| `--path` | `.` | A | Standard tpatch flag — workspace path. |

**Out of Slice A:**

| Flag | Slice | Description |
|------|-------|-------------|
| `--all` | D | Verify every post-apply feature in topological order; aggregate report. |
| `--shadow` | (rejected) | Force shadow allocation even when no recipe is present. Verify already gates V7/V8 on recipe/patch presence; flag is unnecessary. |
| `--fresh-branch` | (out of scope) | Verify against a freshly-checked-out upstream branch. Belongs to `feat-reconcile-fresh-branch-mode`. |

`--no-promote` / `--no-demote` from the first-revision PRD are **dropped** — there is no lifecycle promotion or demotion to opt out of.

### 4.2 `tpatch amend` interaction

Slice B: `tpatch amend <slug>` with a recipe-touching change invalidates the freshness record. Concretely the `Verify.Passed` bit is set to `false` (or `Verify` is cleared entirely; ADR-013 D3 picks one and locks it). `tpatch amend --state tested` is rejected (no such state exists); the existing v0.6.1 amend flag set is otherwise unchanged. No new flag is added.

### 4.3 `tpatch verify --json` output schema

Slice A. The schema is **frozen at the version field**: every consumer reads `schema_version` and refuses unknown majors.

#### 4.3.1 PASS — green verdict, freshness recorded

```json
{
  "schema_version": "1.0",
  "slug": "extra-button",
  "verified_at": "2026-04-27T18:30:11Z",
  "verdict": "passed",
  "exit_code": 0,
  "checks": [
    { "id": "status_loaded",          "severity": "block-abort", "passed": true, "remediation": "" },
    { "id": "intent_files_present",   "severity": "block",       "passed": true, "remediation": "" },
    { "id": "recipe_parses",          "severity": "block",       "passed": true, "remediation": "" },
    { "id": "recipe_op_targets_resolve","severity": "block",     "passed": true, "remediation": "" },
    { "id": "dep_metadata_valid",     "severity": "block",       "passed": true, "remediation": "" },
    { "id": "satisfied_by_reachable", "severity": "block",       "passed": true, "remediation": "" },
    { "id": "dependency_gate_satisfied","severity": "warn",      "passed": true, "remediation": "" },
    { "id": "recipe_replay_clean",    "severity": "block",       "passed": true, "remediation": "" },
    { "id": "post_apply_patch_replay_clean","severity": "block", "passed": true, "remediation": "" },
    { "id": "reconcile_outcome_consistent","severity": "warn",   "passed": true, "remediation": "" }
  ],
  "lifecycle_state": "applied",
  "freshness_label": "verified-fresh",
  "recipe_hash_at_verify": "sha256:7a1b…",
  "patch_hash_at_verify": "sha256:9f24…",
  "parent_snapshot": { "button-component": "applied" }
}
```

#### 4.3.2 NEVER-VERIFIED rendering — when `tpatch status --json` (not verify) reports a feature with no record

Verify itself does not emit a `never-verified` report — every verify run produces a record. This shape is what `tpatch status --json` emits for a feature whose `Verify` is `nil`:

```json
{
  "slug": "extra-button",
  "state": "applied",
  "labels": ["never-verified"]
}
```

#### 4.3.3 STALE — verify last passed but a parent has drifted

Same: not emitted by `verify` (which writes a fresh record). This is `tpatch status --json` after a parent was amended:

```json
{
  "slug": "extra-button",
  "state": "applied",
  "labels": ["verified-stale"],
  "verify": {
    "verified_at": "2026-04-26T10:00:00Z",
    "passed": true,
    "parent_snapshot": { "button-component": "applied" }
  }
}
```

#### 4.3.4 FAIL — block-severity check, freshness records the failure

```json
{
  "schema_version": "1.0",
  "slug": "extra-button",
  "verified_at": "2026-04-27T18:31:02Z",
  "verdict": "failed",
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
  "lifecycle_state": "applied",
  "freshness_label": "verify-failed"
}
```

#### 4.3.5 PARENT-REPLAY FAIL — closure-replay aborted at a hard parent

```json
{
  "schema_version": "1.0",
  "slug": "extra-button",
  "verified_at": "2026-04-27T18:32:14Z",
  "verdict": "failed",
  "exit_code": 2,
  "failed_at": "parent-replay",
  "parent_slug": "button-component",
  "checks": [
    { "id": "status_loaded",          "severity": "block-abort", "passed": true,  "remediation": "" },
    { "id": "intent_files_present",   "severity": "block",       "passed": true,  "remediation": "" },
    { "id": "recipe_parses",          "severity": "block",       "passed": true,  "remediation": "" },
    { "id": "recipe_op_targets_resolve","severity": "block",     "passed": true,  "remediation": "" },
    { "id": "dep_metadata_valid",     "severity": "block",       "passed": true,  "remediation": "" },
    { "id": "satisfied_by_reachable", "severity": "block",       "passed": true,  "remediation": "" },
    { "id": "dependency_gate_satisfied","severity": "warn",      "passed": true,  "remediation": "" },
    { "id": "recipe_replay_clean",    "severity": "block",       "passed": false,
      "remediation": "hard parent button-component failed to replay in shadow: op #3 path 'src/button.tsx' already exists; re-run tpatch verify button-component first" },
    { "id": "post_apply_patch_replay_clean","severity": "block", "passed": false,
      "remediation": "skipped: parent-replay aborted before V8" },
    { "id": "reconcile_outcome_consistent","severity": "warn",   "passed": true,  "remediation": "" }
  ],
  "lifecycle_state": "applied",
  "freshness_label": "verify-failed"
}
```

### 4.4 Skill / harness updates

Slice D updates all 6 skill formats with a one-paragraph addition under the "Lifecycle" section:

> **Verify before composing.** When you finish `tpatch apply` and want a cheap, machine-checkable signal that the feature is structurally healthy, run `tpatch verify <slug>`. Verify writes a freshness record on the feature; downstream readers see a `verified-fresh` label until the recipe, patch, or any hard parent's state drifts, at which point the label flips to `verified-stale`. The lifecycle state is never changed by verify — `applied` stays `applied`. Verify is read-only on the working tree. It does **not** run the project's test suite; for that, use `tpatch test`.

Anchor list (parity-guard `assets/assets_test.go` extension):

- `assets/skills/claude/tessera-patch/SKILL.md`
- `assets/skills/copilot/tessera-patch/SKILL.md`
- `assets/skills/copilot-prompt/...`
- `assets/skills/cursor/...`
- `assets/skills/windsurf/...`
- `assets/skills/generic/...`

### 4.5 Status rendering

Slice B: `tpatch status` gains the freshness label inline (`applied [verified-fresh]`). `tpatch status --dag` renders the same. `tpatch status --json` emits the `Verify` sub-record when present and the derived `freshness_label` in the labels array.

---

## 5. Edge cases / failure modes

| Case | Handling |
|------|----------|
| `verify <slug>` on a feature that does not exist | `exit 2 — feature not found`. No record write. |
| `verify <slug>` on a feature whose `status.json` is malformed | V0 fails (block-abort); `exit 2 — internal error`. No record write. |
| Recipe absent (`apply-recipe.json` missing) | V2/V3/V7 are skipped; V8 runs against the closure-replayed baseline if patch is present. V1/V4/V5/V6/V9 run. |
| `post-apply.patch` absent | V8 is skipped. |
| Both recipe and patch absent | Verify still runs static checks (V1/V4/V5/V6/V9). Reasonable for `applied`-from-pre-autogen-era features. |
| Hard parent in `defined` (not replayable) when V7 needs to replay | V7 fails with `failed_at: "parent-replay"`; freshness record `passed=false`. |
| Hard parent in `upstream_merged` | Skipped during closure replay (its changes are on the baseline). |
| `verify` during a concurrent `reconcile` on the same slug | Refused per §3.4.5. |
| Verify inside a non-tpatch-init repo | `exit 2 — not a tpatch workspace`. |
| Verify on a child whose parent has cycle drift | V4 fails (block); freshness record `passed=false`. |
| `--no-write` honoured with all checks run | Verify runs read-only on `.tpatch/`; freshness record is not updated. |
| Repo with `Config.FeaturesDependencies = false` | V4 still runs. V5 is a no-op. V6 is a no-op. V7 closure replay still runs (DAG flag does not gate hard-dep traversal). |
| Verify on a feature in `requested`/`analyzed`/`defined`/`implementing` | Refused with `exit 2 — feature is pre-apply, nothing to verify`. No record write. |
| Verify on `blocked` / `upstream_merged` | Allowed; runs all applicable checks; writes the freshness record. The harness can interpret `verified-fresh` on `upstream_merged` as "the feature is retired and the artifacts are still healthy." |

---

## 6. Open questions / decisions

The reviewer-adjudicated questions from the first revision (Q1–Q5) are listed for traceability; their resolutions still hold under the new model except where superseded.

### Q1 — V9 (`reconcile_outcome_consistent`) severity: warn vs block?

**Adjudicated, still binding: warn.** A feature can be structurally healthy while sitting in `Reconcile.Outcome = shadow-awaiting`. Demoting verify on V9 would make `verified-fresh` un-reachable for any feature with a pending reconcile.

### Q2 — `verify --all` on pre-apply slugs

**Adjudicated, still binding: skipped with a per-slug `"skipped: pre-apply state"` reason line in the JSON output, exit 0 if all post-apply slugs pass.** Slice D detail.

### Q3 — `passed` field name

**Adjudicated, still binding: retained.** `severity` carries gating; `passed` carries pass/fail intent.

### Q4 — Apply-gate D2 wording

**Superseded by F4.** The first-revision question "does `tested` satisfy hard deps?" is moot under the freshness-overlay model. D2 is now: apply gate is pure-lifecycle, freshness is ignored. See §3.4.6.

### Q5 — Parent-state hook placement

**Adjudicated, still binding under the new model: lives in `composeLabelsFromStatus` (`internal/workflow/labels.go:143`), the existing read-time label computer. No new hot path. Crucially: read-only — never writes `.tpatch/`.** See §3.4.7 for why this is even more restrictive under the freshness model.

### Q6 — Should `verify` clear `Verify.Passed = false` on amend, or just leave the freshness record stale?

**Decision: clear on amend.** The `Verify` record carries an embedded `recipe_hash_at_verify` and `patch_hash_at_verify`; an amend that rewrites the recipe causes the next `ComposeLabels` to derive `verified-stale` from hash drift alone, even if `Verify.Passed` is left at `true`. So strictly speaking the explicit clear is redundant. We clear anyway to make the invalidation visible at write time (operator inspecting `status.json` immediately after amend sees `passed: false`); without the clear, the record's `passed: true` is technically a live but stale claim. This is documented in ADR-013 D3.

### Q7 — `tpatch verify` exit codes

- `0` — verdict passed; freshness recorded.
- `2` — verdict failed (any block-severity check); also covers V0 abort, refused-state, non-existent slug.
- `1` — reserved for "verify aborted by signal / context cancellation"; no record write.

Stable across slices; documented in `--help`.

### Q8 — Open: does the `tpatch verify` recipe-replay tolerate parent-failure mid-closure by skipping the failed parent and trying the rest?

**No, fail-fast on first parent failure.** Spelled out in §3.4.3. Skipping a failed parent would make the V7 result meaningless (the target's recipe applied against a partial baseline). Re-confirmed at design time; no Slice B reconsideration.

---

## 7. Acceptance criteria (combined verify + freshness ships when…)

- [ ] **ADR-013 merged** before any Wave 3 implementation slice lands.
- [ ] `go build ./...`, `go test ./...`, `gofmt -l .` all clean.
- [ ] `FeatureStatus.Verify *VerifyRecord` field present, `omitempty`-marshalled. v0.6.1 fixtures round-trip byte-identical.
- [ ] `FeatureState` enum is unchanged (no `StateTested`).
- [ ] `tpatch verify <slug>` runs the 10-check sequence in order, with the severities documented in §3.1.
- [ ] V7/V8 replay the **hard-parent topological closure** before applying the target's recipe (§3.4.3). Order is `store.TopologicalOrder` over the hard-only sub-DAG. `upstream_merged` parents are skipped.
- [ ] V7/V8 fail-fast on first parent-replay failure with `failed_at: "parent-replay"` and the failing parent slug in the JSON output.
- [ ] On green, `Verify` record is written with `passed=true`; no lifecycle mutation.
- [ ] On block-severity fail, `Verify` record is written with `passed=false`; no lifecycle mutation.
- [ ] `--no-write` runs all checks but does not write `Verify`.
- [ ] `--json` emits the schema in §4.3 with exact field names; `schema_version: "1.0"` is present.
- [ ] V0 abort produces `exit 2 — internal error` and writes nothing to `status.json`.
- [ ] V4 reuses `store.ValidateDependencies` (no parallel validator).
- [ ] V5 reuses the v0.6.1 `satisfied_by` 40-hex + `gitutil.IsAncestor` reachability contract.
- [ ] V6 reuses `workflow.CheckDependencyGate`; soft parents are silent.
- [ ] V7/V8 spin up a single `gitutil.CreateShadow` worktree, replay the closure, run target recipe + patch check, prune before exit.
- [ ] V8 uses `git apply --check` against the closure-replayed shadow tree.
- [ ] V9 reads `status.Reconcile.Outcome` only — never `artifacts/reconcile-session.json`. Adversarial test pins this.
- [ ] `verify` during in-flight reconcile on the same slug refuses with `exit 2`.
- [ ] `ComposeLabels` derives `never-verified` / `verified-fresh` / `verified-stale` / `verify-failed` per the §3.4.2 table. Composes orthogonally with M14.3 labels.
- [ ] `amend (recipe-touching)` invalidates the freshness record (clears `Verify.Passed`); `amend (intent-only)` does not.
- [ ] `tpatch amend --state tested` is rejected with a "no such state" error.
- [ ] **Apply gate is unchanged.** `CheckDependencyGate` satisfaction set remains `{applied, upstream_merged}`. Test `TestApplyGate_FreshnessIsIgnored` pins this.
- [ ] Skill bullet present in all 6 surfaces; parity guard (`assets/assets_test.go`) green.
- [ ] **Backwards compat:** `TestUpgradeFromV0_6_1_NoVerify_BehavesIdentically` — v0.6.1 fixture, all v0.6.1 commands run except `verify`, resulting `.tpatch/` is byte-identical to v0.6.1 expected.
- [ ] **Source-truth guard:** adversarial test asserts the verify implementation does NOT import or read `artifacts/reconcile-session.json` or `artifacts/resolution-session.json` at any code path.
- [ ] CHANGELOG v0.6.2 callout names `verify` and the freshness overlay with exact contract surface.

---

## 8. Risks and mitigations

| Risk | Mitigation |
|------|------------|
| Operator confusion: "I ran verify but the lifecycle state didn't change." | §1.3 + skill bullet explain that verify writes a freshness record, not a lifecycle transition. CHANGELOG explicit. |
| Closure-replay cost on deep DAGs. | Bounded by DAG depth × per-recipe replay cost. For typical 1–3-deep DAGs the cost is well within the cheap-budget. Operators with deeper DAGs verify parents first; harness reads `verified-fresh` and skips redundant reverify. |
| Closure-replay drift: a parent's `apply-recipe.json` has changed since the parent was last applied locally. | Verify replays the parent's *current* `apply-recipe.json` regardless. If the parent itself is `verified-stale` due to recipe drift, the operator should `tpatch verify <parent>` first; the failing-parent message names the parent explicitly. |
| Freshness record getting out of date silently. | The four-label derivation is read-time; staleness is visible in `tpatch status` immediately. No silent rot. |
| Closure-replay shadow contention with reconcile. | §3.4.5: per-slug lock; verify refused while reconcile in flight on the same slug. |
| Shadow leak on crash. | `defer PruneShadow(...)` at verify entry; `PruneAllShadows` reaps stale shadows from prior crashed runs. |
| `Verify` field break on downstream JSON consumers that hard-code v0.6.1 schema. | `omitempty` means v0.6.1 round-trip is byte-identical until first verify. After first verify, the schema gains one new top-level field; downstream consumers need to handle the omitempty case. CHANGELOG callout. |
| Apply gate ignoring freshness misread as "verify is pointless." | The PRD §1.2 + skill bullet emphasise that freshness is a harness signal, not a gate. The harness pattern (`verify parent && apply child`) keeps working. |

---

## 9. Implementation slices (downstream Wave 3 dispatches)

The dispatch contract names four slices. Boundaries below reflect F3's correction of Slice A scope.

### Slice A — Verify command shell + V0–V2 cheap structural checks + freshness writer skeleton

- New file `internal/cli/verify.go` (registered under `cmd/tpatch/main.go`).
- New file `internal/workflow/verify.go` with `RunVerify(s *store.Store, slug string, opts VerifyOptions) (*VerifyReport, error)` and the V0–V2 check functions.
- Cobra wiring: `<slug>` arg, `--json`, `--quiet`, `--no-write`, `--path`. **No `--all`, no `--shadow`.**
- New `VerifyRecord` and `VerifyCheckResult` structs in `internal/store/types.go` with `omitempty` JSON tags.
- New `WriteVerifyRecord` (or equivalent) helper in `internal/store/store.go` that updates `FeatureStatus.Verify` and persists.
- V3–V9 stubbed with `TODO` and a sentinel result that marks them `passed=true, severity=warn` so the report still emits a 10-entry array.
- **No closure replay.** V7/V8 stubs return immediately.
- **No skill anchor regen.** Slice D handles all skill surface changes.
- **No `ComposeLabels` extension.** Slice B integrates the freshness derivation.
- Tests: V0–V2 unit tests; `--json` shape golden test for the green/fail rows V0–V2 produce; `--no-write` honoured.

### Slice B — Freshness derivation + label integration

- `ReconcileLabel` vocabulary gains `LabelNeverVerified`, `LabelVerifiedFresh`, `LabelVerifiedStale`, `LabelVerifyFailed`.
- `composeLabelsFromStatus` (`internal/workflow/labels.go:143`) extended to derive the four labels per §3.4.2. Pure function; no writes.
- `tpatch status` and `tpatch status --dag` render the freshness label inline.
- `tpatch status --json` emits the `Verify` sub-record + the derived label.
- `amend (recipe-touching)` invalidates `Verify.Passed`; `amend (intent-only)` preserves.
- `amend --state tested` rejected with `exit 2 — no such state`.
- Tests: derivation truth-table per §3.4.2; v0.6.1 round-trip byte-identity (no `Verify` set); apply-gate test pinning that freshness does NOT extend the satisfaction set.

### Slice C — V3–V9 implementation incl. closure replay

- V3 implementation reusing M14.2 `created_by` semantics.
- V4 calling `store.ValidateDependencies`.
- V5 calling `gitutil.IsAncestor` per the v0.6.1 satisfied_by contract.
- V6 calling `workflow.CheckDependencyGate` with severity reduced to warn.
- V7 + V8: hard-parent topological closure replay per §3.4.3, target recipe apply, target patch `--check`. Single shadow allocation. `failed_at: "parent-replay"` JSON field on first parent failure.
- V9 reading `status.Reconcile.Outcome` (adversarial test pins no artifact reads).
- Tests: per-check unit tests; closure replay test fixtures (3-deep DAG; parent-fail mid-closure; `upstream_merged` parent skipped); concurrency-with-reconcile refusal; source-truth adversarial test.

### Slice D — `--all`, skill bullets, harness anchors, parity guard, CHANGELOG

- `tpatch verify --all` topo-ordered aggregate report; pre-apply slugs skipped per Q2.
- All 6 skill surfaces gain the §4.4 bullet.
- `assets/assets_test.go` parity guard extended with the new anchors.
- `docs/dependencies.md` cross-link to verify (one-paragraph aside near the apply-time gate section).
- `CHANGELOG.md` v0.6.2 entry naming the verb, the freshness overlay, and the explicit out-of-scope list.
- Tests: parity-guard green; `verify --all` topo-order test; pre-apply skip test.

Each slice is independently dispatchable and shippable.

---

## 10. Cross-cutting impact matrix

| Other feature / surface | Relationship | Notes |
|-------------------------|--------------|-------|
| `feat-feature-dependencies` (M14, shipped) | **independent** | `CheckDependencyGate` unchanged. M14.3 labels unchanged. Freshness labels compose orthogonally via the existing `ComposeLabels` plumbing. |
| `feat-provider-conflict-resolver` (M12, shipped) | **independent** | Verify never calls the resolver. Per-slug shadow lock prevents collision. |
| `tpatch amend` (M13, shipped) | **extends** | Recipe-touching amend invalidates the freshness record. No new flag. |
| `tpatch test` (existing command) | **independent** | Distinct verb; not a producer of the freshness record. |
| `tpatch reconcile` | **independent at the lifecycle level**; **invalidates freshness** | Reconcile rewriting `apply-recipe.json` or `post-apply.patch` causes `recipe_hash_at_verify` / `patch_hash_at_verify` drift; next `ComposeLabels` derives `verified-stale`. |
| `tpatch status` / `--dag` / `--json` | **extends** | Renders freshness label and emits `Verify` sub-record. |
| `assets/assets_test.go` | **extends** | New skill anchor for the verify/freshness bullet. |
| `docs/dependencies.md` | **extends** (one paragraph) | Cross-links verify in the apply-time-gate section. |
| `feat-reconcile-code-presence-verdicts` | **out of scope** | Distinct PRD; reuses some primitives. |
| `feat-reconcile-fresh-branch-mode` | **out of scope** | Distinct PRD. |
| `delete-file` recipe op | **out of scope** | Recipe-op JSON schema frozen. Verify tolerates deletions in shadow replay. |
| `feat-tested-state-test-producer` (future) | **enables** | If `tpatch test` ever joins as a producer of the freshness record, ADR-013 D3 names it as the future-work expansion. |

---

**End of PRD.** Implementation handoff for Slice A will live in `docs/handoff/CURRENT.md` once this PRD + ADR-013 are reviewed and approved.
