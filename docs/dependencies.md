# Feature Dependencies

> **Available:** v0.6.0+. Default: **on**. Toggle via
> `features_dependencies: true|false` in `.tpatch/config.yaml`.
> Authoritative spec: [`docs/prds/PRD-feature-dependencies.md`](./prds/PRD-feature-dependencies.md)
> · Locked decisions: [`docs/adrs/ADR-011-feature-dependencies.md`](./adrs/ADR-011-feature-dependencies.md)

Tessera Patch tracks a directed acyclic graph (DAG) of features so that

- features can declare which earlier features they build on,
- the apply / reconcile pipelines respect that order,
- and the operator can see at a glance which work is blocked by which
  upstream parent.

The graph is small, hand-curated, and derived from the same `status.json`
files that drive every other tpatch operation. Nothing is auto-inferred.

## What a dependency is

A dependency is an edge **child → parent**. It records "this feature
depends on the parent feature already being applied (or being merged
upstream)." Each child enumerates its parents in
`status.json -> depends_on`:

```jsonc
{
  "slug": "extra-button",
  "depends_on": [
    { "slug": "button-component", "kind": "hard" },
    { "slug": "polish-css",       "kind": "soft" }
  ]
}
```

There are two kinds, picked per edge:

| kind   | meaning                                                              | apply gate? | label/order? |
|--------|----------------------------------------------------------------------|-------------|--------------|
| `hard` | Child cannot meaningfully exist until the parent is applied.         | yes         | yes          |
| `soft` | Ordering hint only — useful but not load-bearing.                    | no          | yes          |

The third optional field, `satisfied_by`, only applies once the parent
reaches state `upstream_merged`; it stores the commit SHA that absorbed
the parent so the edge keeps its provenance even if the parent feature
is later removed.

## Declaring dependencies

Edit by hand if you like (it's just JSON), or use the CLI:

```bash
# show
tpatch feature deps extra-button

# add (kind defaults to hard)
tpatch feature deps extra-button add button-component
tpatch feature deps extra-button add polish-css:soft

# upgrade or downgrade kind
tpatch feature deps extra-button add polish-css:hard

# remove
tpatch feature deps extra-button remove polish-css

# in-batch with other amend edits
tpatch amend extra-button "Tighten copy" \
    --depends-on button-component \
    --depends-on polish-css:soft \
    --remove-depends-on legacy-shim

# global health check
tpatch feature deps --validate-all
```

Every write goes through the same validator that loads use. **Edits are
atomic** — a rejected change never leaves the store half-modified.

## Validation rules

The five rules from PRD §3.3:

1. **No self-dependency.** A feature cannot depend on itself.
2. **No dangling refs.** Every parent slug must exist in the store.
3. **No kind conflicts.** The same parent declared twice with different
   kinds is rejected.
4. **No cycles.** Considered against the *global* graph including the
   proposed change. The cycle path is included in the error.
5. **`satisfied_by` requires `upstream_merged`.** Setting `satisfied_by`
   on a parent in any other state is rejected; only an actually-merged
   parent should claim a commit SHA as provenance.

`tpatch status` (with or without `--dag`) re-runs the same validator at
read time and surfaces every violation inline. Run it routinely.

## Apply-time semantics

When `features_dependencies: true`:

- `tpatch apply <child> --mode execute` checks each **hard** parent.
  Parents must be in `applied`, `active`, or `upstream_merged`.
- Soft parents are never gates; they only contribute to ordering.
- The check fires before any file mutations. A blocked apply leaves the
  working tree untouched.

The error lists the unsatisfied parents and their states:

```
Error: feature "extra-button" has 1 unsatisfied hard dependency(ies):
  - button-component (state=defined)
Run `tpatch apply <parent>` (or merge upstream) for each blocking parent
before retrying.
```

### `created_by` — recipe-level provenance

Recipe operations may carry a `created_by: "<parent-slug>"` hint:

```jsonc
{
  "type": "write-file",
  "path": "src/extras/button.css",
  "content": "...",
  "created_by": "button-component"
}
```

From v0.6.0 this is **a live gate**, not a comment field. At
`apply --mode execute`:

- The op's `created_by` parent must appear in the recipe's `depends_on`.
  Missing → operation rejected (configuration error in the recipe).
- If the parent edge is **hard** and the parent is not in an applied
  state, the op is rejected (matches the apply-time hard-parent check).
- In `apply --mode execute` the rejection is fatal.
- In `apply --dry-run` a missing-hard-parent miss is **downgraded to a
  warning** so you can inspect the planned changes (PRD §4.3).
  Recipe-shape failures (parent absent from `depends_on`, unknown kind)
  remain hard errors in both modes.

## Reconcile-time semantics

Reconcile traverses the DAG in topological order so a child only
reconciles after every hard parent is resolved. The child's own
`Reconcile.Outcome` is computed exactly as before; what's new is a
**composable label overlay** read from parent state at display time.

Three labels can stack on a single child:

| label                  | when                                                                 |
|------------------------|----------------------------------------------------------------------|
| `waiting-on-parent`    | a hard parent is still pre-applied (requested/analyzed/.../shadow)   |
| `blocked-by-parent`    | a hard parent's outcome is `blocked-*` / `shadow-awaiting` / state=blocked |
| `stale-parent-applied` | an applied hard parent was updated after the child's last reconcile  |

Soft parents never produce labels. The child's own outcome is never
re-classified — labels are an overlay, not a new enum value.

### Compound presentation

When the child's intrinsic outcome is `blocked-requires-human` AND
`blocked-by-parent` is in its labels, `EffectiveOutcome()` reports the
compound string:

```
blocked-by-parent-and-needs-resolution
```

This signals to the operator that the child is broken on its own AND
gated on a broken parent — both must be fixed. The compound string is
**display-only**; programmatic decisions still read `Outcome` and
`Labels` separately.

### Authoritative source

Any code (including external harnesses) that reads reconcile state for
DAG-aware decisions **MUST** read `FeatureStatus.Reconcile.Outcome` via
`store.LoadFeatureStatus`. Never read
`artifacts/reconcile-session.json` — that is an audit log of one
RunReconcile invocation; it does not reflect post-accept truth (ADR-010
D5 + ADR-011 source-truth guard).

## Visualising the DAG

`tpatch status --dag` renders an ASCII tree. Roots come from features
with no in-scope parents; siblings sort alphabetically.

```text
$ tpatch status --dag
DAG (all features)
button-component [applied] reapplied
└─► extra-button [implementing]
└┄► polish-css [defined]
```

- `─►` = hard edge.
- `┄►` = soft edge.

Scope to one feature's transitive parents + children:

```text
$ tpatch status --dag extra-button
DAG (scope: extra-button)
button-component [applied] reapplied
└─► extra-button [implementing]
```

Add `--json` for harness consumption:

```bash
tpatch status --dag --json
```

Cycles are detected up front; on a corrupted graph the renderer emits a
flat list with a `⚠ cycle detected` warning rather than recursing.

## Removing features

Removing a feature with downstream dependents requires explicit consent:

| Command                                       | Behaviour                                               |
|-----------------------------------------------|---------------------------------------------------------|
| `tpatch remove <slug>`                        | Refuses if `<slug>` has any dependent.                  |
| `tpatch remove <slug> --force`                | Same — `--force` does **not** bypass dep integrity.     |
| `tpatch remove <slug> --cascade`              | Confirms at TTY, then removes leaves first.             |
| `tpatch remove <slug> --cascade --force`      | Skips the confirm prompt. Required for non-TTY runs.    |

`--cascade` deletes in **reverse-topological order** (leaves first), so
no point in time exists where a child remains while its parent is gone.

`--force` is a TTY-only override. PRD §3.7 + ADR-011 D7 explicitly
forbid bypassing DAG integrity with it.

## Migration from v0.5.x

Existing v0.5.x repos work unchanged. Until you add a `depends_on`
block to a feature:

- `status.json` round-trips byte-identical (`omitempty`).
- `apply` behaviour matches v0.5.3 exactly.
- `reconcile-session.json` is unchanged.
- No new label keys are written.

To opt out of the v0.6.0 default for one repo:

```yaml
# .tpatch/config.yaml
features_dependencies: false
```

That restores v0.5.3 byte-identity end-to-end. (We don't expect anyone
to need this in production — the gate is a strict no-op when no edges
exist — but it exists for projects that pin tpatch behaviour by SHA.)

## Limits and future work

These are **out of scope** for v0.6.0 (tracked separately):

- **Provider-assisted parent injection** — the M12 resolver does not
  yet feed parent-patch context into the conflict resolver
  (ADR-011 D8 — deferred to v0.7).
- **Auto-inference of `created_by`** — implement-phase heuristics that
  guess provenance from file paths are a separate backlog
  (PRD §4.3.1). Today, the field is operator-set or skill-suggested.
- **Soft-only cascade modes** — there is no "drop only soft dependents"
  mode in v1; cascade treats both kinds identically.

See ADR-011 D7–D9 and PRD-feature-dependencies §10 for the rationale.
