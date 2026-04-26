# Current Handoff

## Active Task

- **Task ID**: M14.3 — Reconcile topological traversal + composable labels + compound verdict
- **Milestone**: M14 — Feature Dependencies / DAG (Tranche D, v0.6.0)
- **Status**: Not Started (unblocked by M14.2 APPROVED, commits `24baf92`, `9a5f2f3`, `4dfe0f1`, `cdd5484`)
- **Assigned**: 2026-04-26
- **Estimated size**: ~500 LOC (largest M14 sub-milestone)

### Context

M14.1 ✅ data model + DAG primitives. M14.2 ✅ apply gate + `created_by` (inert). Now M14.3 introduces the first reconcile-time DAG behavior:

1. **Topological traversal** — when reconciling a set of features, run them in dependency order (parents first).
2. **Composable labels** — `waiting-on-parent`, `blocked-by-parent`, `stale-parent-applied` overlay onto the child's intrinsic verdict (per ADR-011 D6 + PRD §3.5).
3. **Compound verdict** — `blocked-by-parent-and-needs-resolution` skips phase 3.5 (resolver) when a hard parent isn't applied.

All gated behind `features_dependencies` (default false). Flag-off path is byte-identical to v0.5.3 reconcile.

### Authoritative docs (must read in order)

1. **`docs/adrs/ADR-011-feature-dependencies.md`** — locks 9 decisions. CRITICAL sections:
   - **D3** — Composable labels, NOT new states. Don't add `ReconcileWaitingOnParent` enum values.
   - **D6** — Read child's intrinsic verdict from `status.Reconcile.Outcome` FIRST, then overlay parent-derived labels. Compound verdict `blocked-by-parent-and-needs-resolution` skips phase 3.5.
   - **D7** — `--cascade` required for cross-feature operations; `--force` does NOT bypass DAG integrity.

2. **`docs/prds/PRD-feature-dependencies.md`**:
   - **§3.5** — composable labels matrix. Authoritative wording.
   - **§4.5** — precedence rules. AUTHORITATIVE when §3.4 contradicts.
   - **§3.4** — has residual terminology drift treating labels as enum verdicts. **DEFER to ADR-011 D6 + §4.5.** Do NOT introduce new `ReconcileOutcome` enum values from §3.4.
   - **§7** — acceptance criteria.

3. **`docs/adrs/ADR-010-provider-conflict-resolver.md` D5** — artifact ownership contract.

4. **`internal/workflow/reconcile.go`** — current reconcile state machine. Read end-to-end before touching it. Especially `RunReconcile`, `tryPhase35`, `saveReconcileArtifacts`.

5. **`internal/workflow/accept.go`** — `AcceptShadow` + `clearShadowPointerAndStamp`. M14.3 may need to extend the helper to compose labels at accept time.

6. **`internal/store/dag.go`** — M14.1 primitives (`TopologicalOrder`, `Children`).

7. M14.2 commits — gate semantics, especially how soft vs hard is interpreted.

### The external-reviewer guard (MANDATORY for M14.3)

> Any new dependency/DAG logic must read **`status.Reconcile.Outcome`** as the authoritative machine-readable reconcile result — NEVER `artifacts/reconcile-session.json`. The session artifact is an audit record of one `RunReconcile` invocation; `status.json` is the source of current truth post-accept (see ADR-010 D5).

This is **load-bearing** for M14.3. Label composition reads parent verdicts. Always go through `store.LoadFeatureStatus(parent).Reconcile.Outcome`, never any session artifact.

### M14.3 scope (~500 LOC across 3 chunks)

#### Chunk A — Topological reconcile traversal (~150 LOC)

Update `RunReconcile` (or wrap it) so when given multiple slugs, they execute in topological order (parents first). Currently the loop is sequential in input order.

- New: `workflow.PlanReconcile(s *Store, slugs []string) ([]string, error)` — builds the dep graph for the given set + their hard parents (transitive closure of hard deps), runs `TopologicalOrder`, returns the ordered slug list. Reject with cycle path on cycle (already supported by `dag.go`).
- Wire into `RunReconcile`'s entry point. Flag-gated:
  - `!cfg.DAGEnabled()`: process slugs in input order (current v0.5.3 behavior). Byte-identical exit, byte-identical `reconcile-session.json` per slug.
  - `cfg.DAGEnabled()`: call `PlanReconcile`, process in returned order.
- Soft deps still contribute to ordering (per PRD §6 / M14.1 design). Hard vs soft only matters for label composition + apply gate, not topology.

Tests:
- `TestPlanReconcile_FlagOff_PreservesInputOrder`
- `TestPlanReconcile_FlagOn_TopologicallyOrders`
- `TestPlanReconcile_RejectsCycle`
- `TestPlanReconcile_TransitiveHardClosure` — given `[child]` only, closure includes hard parents

#### Chunk B — Composable labels (~250 LOC, the trickiest)

Per ADR-011 D3 + D6 + PRD §3.5, labels are computed AFTER the intrinsic reconcile verdict is determined. They overlay, not replace.

New types in `internal/store/types.go`:

```go
// ReconcileLabel is a derived overlay on top of Reconcile.Outcome that
// describes the DAG context. Labels are computed; they are NOT persisted
// as enum values on Reconcile.Outcome.
type ReconcileLabel string

const (
    LabelWaitingOnParent      ReconcileLabel = "waiting-on-parent"
    LabelBlockedByParent      ReconcileLabel = "blocked-by-parent"
    LabelStaleParentApplied   ReconcileLabel = "stale-parent-applied"
)
```

Add `Labels []ReconcileLabel \`json:"labels,omitempty"\`` to `FeatureStatus.Reconcile` (the existing struct that holds `Outcome`, `AttemptedAt`, etc.). `omitempty` is critical — empty list = field omitted = byte-identical to v0.5.3.

New file `internal/workflow/labels.go`:

```go
// ComposeLabels reads the current FeatureStatus + dependency declarations
// and computes the overlay labels. The intrinsic verdict (Reconcile.Outcome)
// is read FIRST and remains untouched; labels overlay on top.
//
// Authoritative reading rule (ADR-010 D5): for each parent, read
// store.LoadFeatureStatus(parent).Reconcile.Outcome — NEVER consult
// artifacts/reconcile-session.json. The session artifact may be stale
// or describe a pre-accept state.
//
// When Config.DAGEnabled() is false, returns empty slice (no labels).
func ComposeLabels(s *store.Store, slug string) ([]store.ReconcileLabel, error)
```

Behavior matrix per PRD §3.5 / ADR-011 D6:

| Parent state (hard dep) | Parent reconcile.Outcome | Label on child |
|---|---|---|
| `analyzed`/`defined`/`explored`/`implemented` (not yet applied) | n/a | `waiting-on-parent` |
| applied, but parent has `needs-human-resolution`/`blocked-*`/`shadow-awaiting` | (parent reconcile blocked) | `blocked-by-parent` |
| applied + parent recently changed (rebased/amended) and child hasn't been re-reconciled | parent newer than child's last reconcile | `stale-parent-applied` |

Soft deps NEVER produce labels (per ADR-011 D4 — soft is ordering-only).

Multiple labels can stack — e.g., one parent waiting + another stale gives the child `[waiting-on-parent, stale-parent-applied]`. Order labels deterministically (alphabetical by string).

Wire into the reconcile state machine in `RunReconcile`:
- Flag off: do not call `ComposeLabels`. Keep `Reconcile.Labels = nil`.
- Flag on: AFTER the intrinsic verdict is computed, call `ComposeLabels` and persist into `FeatureStatus.Reconcile.Labels`.

Tests in `internal/workflow/labels_test.go`:
- `TestComposeLabels_FlagOff_AlwaysEmpty`
- `TestComposeLabels_NoDeps_Empty`
- `TestComposeLabels_HardParentNotApplied_AddsWaitingOnParent`
- `TestComposeLabels_HardParentBlocked_AddsBlockedByParent`
- `TestComposeLabels_HardParentApplied_NoLabel`
- `TestComposeLabels_HardParentRecentlyChanged_AddsStaleParentApplied`
- `TestComposeLabels_SoftParentNeverProducesLabel`
- `TestComposeLabels_MultipleParentsStackLabels`
- `TestComposeLabels_DeterministicOrder` (run 50× on a fixture, assert equal each time)
- `TestComposeLabels_ReadsStatusJsonNotSessionArtifact` — adversarial: write a misleading `reconcile-session.json` for the parent and confirm the label uses `status.json` instead.

Round-trip:
- `TestStatusRoundtrip_FlagOff_LabelsOmitted` — flag off, save status, load, save again, byte-identical.
- `TestStatusRoundtrip_FlagOn_EmptyLabels_OmittedFromJSON` — `Labels: []` writes the same bytes as `Labels: nil`.

#### Chunk C — Compound verdict + phase 3.5 skip (~100 LOC)

Per ADR-011 D6: if a child has `LabelBlockedByParent` AND its intrinsic outcome would be `needs-human-resolution`, the COMPOSED outcome is the compound `blocked-by-parent-and-needs-resolution`. This compound verdict means: skip phase 3.5 (provider resolver) entirely — no point asking the LLM to resolve conflicts when a hard parent is itself broken.

This compound is NOT a new `ReconcileOutcome` enum value. It's a derived presentation. The persisted `Reconcile.Outcome` stays `needs-human-resolution` (intrinsic); the derived presentation is computed from `Outcome + Labels` at read time.

- Add a helper in `internal/store/types.go`:

```go
// EffectiveOutcome returns the compound presentation of (Outcome, Labels)
// per ADR-011 D6 + PRD §3.5. Labels overlay on top of Outcome:
//   - Outcome=needs-human-resolution + LabelBlockedByParent
//     → "blocked-by-parent-and-needs-resolution" (compound, M14.3)
//   - Otherwise: Outcome stringified.
//
// Callers like status display use this helper. Programmatic decisions
// MUST read Outcome + Labels separately, not the compound string.
func (r FeatureReconcile) EffectiveOutcome() string
```

- In `tryPhase35` (or wherever the resolver is invoked), before launching the resolver:
  - If `Config.DAGEnabled()` AND child has `LabelBlockedByParent`: short-circuit. Set `Outcome = ReconcileBlockedRequiresHuman` (existing enum, NOT a new one), set `Labels = [blocked-by-parent]`, persist, log a clear note pointing the user at the parent. Don't call the resolver.
  - The compound presentation is then computed by `EffectiveOutcome()` for display.

Tests:
- `TestReconcile_FlagOn_BlockedByParent_SkipsPhase35` — assert resolver was never called (use a scripted provider that fails the test if invoked).
- `TestEffectiveOutcome_CompoundComposition` — `(needs-human-resolution, [blocked-by-parent])` → `blocked-by-parent-and-needs-resolution`.
- `TestEffectiveOutcome_PassthroughWhenNoCompoundLabels` — other label combinations don't produce compounds.

#### Chunk D — Skill format updates (~minimal)

The 6 skill formats currently describe reconcile outcomes but not labels. **HOLD this for M14.4** — M14.3 keeps the labels invisible to humans (they live in `status.json` for tooling). The skill rollout for labels happens at M14.4 alongside `tpatch status --dag` and `docs/dependencies.md`.

**However**: if the parity guard (`assets/assets_test.go`) checks anything about the `status.json` schema (it might), confirm `Labels` field is documented OR confirm the parity guard does not require it. Run `go test ./assets/...` after every type change.

#### Chunk E — Interaction with `AcceptShadow` (~minimal but critical)

`AcceptShadow` is the shared accept helper from v0.5.2/v0.5.3. After it stamps `Reconcile.Outcome=ReconcileReapplied`:

- If flag on: re-compute `Labels` for the accepted child (the parent state may have changed since reconcile started). Persist updated labels.
- If flag off: leave `Labels` nil (it was already nil if you didn't set it).

Tests:
- `TestAcceptShadow_FlagOn_RefreshesLabels` — set up child with stale label, run accept, assert labels recomputed.
- `TestAcceptShadow_FlagOff_LabelsRemainNil` — byte-identical `status.json` post-accept vs v0.5.3.

### Strict scope guards (DO NOT do these)

- DO NOT add `tpatch status --dag` output (M14.4)
- DO NOT update skill formats with labels documentation (M14.4)
- DO NOT bump version, update CHANGELOG, or tag (M14.4)
- DO NOT add `ReconcileWaitingOnParent` / `ReconcileBlockedByParent` enum values to `ReconcileOutcome` — labels are NOT new states (ADR-011 D3)
- DO NOT add new external Go dependencies
- DO NOT touch the apply gate from M14.2 (separate concern)
- DO NOT populate `created_by` from the implement phase yet — that's separate from M14.3 label work and can wait. Labels read parent state + dep declarations, not `created_by`.
- DO NOT inject parent patches into the M12 resolver context (ADR-011 D8)

### Validation gate

```
gofmt -l .
go build ./cmd/tpatch
go test ./...
go test ./assets/...                    # parity guard
go test ./internal/workflow -run 'PlanReconcile|ComposeLabels|EffectiveOutcome|AcceptShadow|GoldenReconcile' -count=1 -v
go test ./internal/store -run 'DAG|Dependency|Validate|Roundtrip|Reconcile' -count=1 -v
```

CRITICAL regression tests that must stay green:
- `TestGoldenReconcile_ResolveApplyTruthful`
- `TestGoldenReconcile_ManualAcceptFlow`
- All M14.1 dag/validation/roundtrip tests
- All M14.2 dependency-gate tests

### Workflow

1. Update CURRENT.md "Status: In Progress" with timestamp.
2. Read all required docs IN ORDER. ADR-011 D3 + D6 + PRD §3.5 + §4.5 are non-negotiable.
3. **Chunk A first** (planner) — pure logic on top of M14.1 `dag.go`. Easy regression target.
4. **Chunk B** (labels) — most code volume; do `ComposeLabels` + tests before wiring into reconcile.
5. **Chunk C** (compound verdict) — small but high-stakes. Skip-phase-3.5 test must use a tripwire provider (fails if invoked).
6. **Chunk E** (`AcceptShadow` integration) — small but easy to forget.
7. Run full validation gate. Iterate.
8. Update CURRENT.md with completion summary.
9. 3-5 logical commits, all with the Co-author trailer. Suggested:
   - `feat(workflow): add PlanReconcile topological planner (M14.3)`
   - `feat(store): add ReconcileLabel + Labels field (M14.3)`
   - `feat(workflow): add ComposeLabels + label-aware reconcile (M14.3)`
   - `feat(workflow): compound blocked-by-parent verdict + phase-3.5 skip (M14.3)`
   - `feat(workflow): AcceptShadow refreshes labels (M14.3)`
10. Push to `origin/main`. (`git push` takes 60+ seconds.)
11. Final CURRENT.md update flagging "Status: Review — ready for code-review sub-agent".

DO NOT bump version. DO NOT update CHANGELOG. DO NOT tag.

### Out-of-band reminders

- The `tpatch` binary at root is NOT gitignored — delete it after `go build`. NEVER commit it.
- Zero external Go deps.
- Update CURRENT.md at every phase transition (analyze → chunk-A → chunk-B → chunk-C → chunk-E → done).

### Deferred behind M14.3

- M14.4 — `tpatch status --dag` rendering, skills analyze-phase bullet for DAG, `docs/dependencies.md` user guide, flag default flip to true, CHANGELOG, tag v0.6.0 (~300 LOC). **THIS is the user-facing cutover.**

### Registered follow-ups (unchanged)

- `feat-ephemeral-mode` — depends on `feat-feature-import` + `feat-delivery-modes`
- `feat-feature-reorder` — depends on `feat-feature-dependencies` (i.e., M14)
- `feat-resolver-dag-context` — parent-patch injection to M12 resolver (DEFERRED — ADR-011 D8 explicitly excludes from v0.6)
- `feat-feature-autorebase`, `feat-amend-dependent-warning`
- `feat-skills-apply-auto-default`, `bug-record-roundtrip-false-positive-markdown`, `chore-gitignore-tpatch-binary`
- `feat-satisfied-by-reachability` — `git merge-base` reachability check for `satisfied_by`; M14.2 deferred this to keep gate logic pure.
