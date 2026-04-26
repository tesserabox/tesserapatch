# Current Handoff

## Active Task

- **Task ID**: M14.2 — Apply gate + `created_by` recipe op + 6-skill parity-guard rollout
- **Milestone**: M14 — Feature Dependencies / DAG (Tranche D, v0.6.0)
- **Status**: Not Started (unblocked by M14.1 APPROVED, commits `02f1ba9`, `d166281`, `7dd5941`)
- **Assigned**: 2026-04-26

### Context

M14.1 landed the data model: `Dependency` struct + `FeatureStatus.DependsOn` (omitempty) + DFS cycle detection + Kahn topo + 5 validation rules + sentinel errors + `features_dependencies` flag (default false). 30 new tests, byte-identity round-trip guard, no callers yet gate on the flag.

M14.2 adds the **first behavior change** — but still gated. With `features_dependencies=true`:
1. `tpatch apply` refuses to execute when any **hard** parent is not yet `applied`/`upstream_merged`.
2. The recipe gains a new optional op `created_by` so child features can declare which parent originated a file (used by M14.3 for label composition).

### Authoritative docs (must read before coding)

1. `docs/adrs/ADR-011-feature-dependencies.md` — locks 9 decisions. Especially **D4** (hard deps gate apply + `created_by`; soft gates neither) and **D5** (`upstream_merged` satisfies deps via `satisfied_by`).
2. `docs/prds/PRD-feature-dependencies.md` — §3.2 apply gate semantics, §3.3 validation, §3.5 labels (READ but DON'T IMPLEMENT — that's M14.3), §6 milestone sizing.
3. `docs/adrs/ADR-010-provider-conflict-resolver.md` D5 — artifact ownership contract. Note: M14.2 does NOT touch reconcile, so this is reference-only.
4. `assets/assets_test.go` — the parity guard. M14.2 mutates the recipe JSON contract — the parity guard MUST stay green after the rollout.

### M14.2 scope (~250 LOC + 6 skill format updates)

#### 1. Apply gate (~80 LOC)

- New: `workflow.CheckDependencyGate(s *Store, slug string) error` — looks up the feature's `DependsOn`, for each `Kind=hard` parent verifies `state ∈ {applied, upstream_merged}` (and if `upstream_merged`, that `SatisfiedBy` matches a parent commit reachable from current HEAD — minimal check, see PRD §3.2).
- Wire into `apply --mode execute` and `apply --mode auto` BEFORE the existing recipe execution begins. Soft deps are NOT gated — they're ordering hints only.
- **Gated by `features_dependencies` flag** — when false, `CheckDependencyGate` is a no-op. Same flag from M14.1.
- Error message must be actionable: list the blocking parent slug(s) and their current state. Suggest `tpatch apply <parent>` first.
- Sentinel: `ErrParentNotApplied` (wrappable via `errors.Is`).

Tests:
- gate-disabled-passes (flag off, hard parent in `analyzed` state — apply proceeds)
- gate-rejects-hard-unapplied (flag on, hard parent in `analyzed` — apply rejected)
- gate-allows-hard-applied (flag on, hard parent applied — apply proceeds)
- gate-allows-upstream-merged (flag on, hard parent in `upstream_merged` with valid `satisfied_by` — apply proceeds)
- gate-rejects-upstream-merged-bad-sha (flag on, `satisfied_by` not reachable from HEAD — apply rejected)
- gate-ignores-soft (flag on, only soft parents unapplied — apply proceeds)
- gate-mixed (flag on, one hard applied + one hard not + one soft not — apply rejected with only the unapplied hard listed)

#### 2. `created_by` recipe op (~120 LOC + 6-skill rollout)

PRD §3.4 (NOTE: this section has the residual ADR-011 D6 terminology drift — defer to ADR-011 D4 + §3.5 for any conflict). The recipe gains an optional field on each operation:

```json
{
  "op": "patch",
  "path": "src/auth.ts",
  "created_by": "feat-jwt-auth",   // optional; the parent slug that originated this file
  "content": "..."
}
```

- Update `internal/workflow/recipe.go` (or wherever `RecipeOperation` is defined) to add `CreatedBy string \`json:"created_by,omitempty"\`` field.
- The field is **persisted but inert in M14.2** — no behavior depends on it. M14.3 reads it for label composition. Document this clearly in a doc comment.
- `omitempty` is critical — recipes generated for features with no DAG flag must round-trip byte-identical to v0.5.3.
- Add a positive recipe-parsing test that round-trips a recipe with `created_by` set; add a negative test confirming an unknown field still fails the parity guard's `DisallowUnknownFields` (the schema is closed except for known fields).

#### 3. 6-skill parity-guard rollout — COORDINATED ATOMIC CHANGE

The parity guard (`assets/assets_test.go`) enforces that the recipe schema documented in skill files matches the Go struct. Every skill format must be updated **in lockstep** with the Go struct change:

- `assets/skills/claude/tessera-patch/SKILL.md`
- `assets/skills/copilot/tessera-patch/SKILL.md`
- `assets/skills/cursor/tessera-patch.mdc`
- `assets/skills/windsurf/windsurfrules`
- `assets/workflows/tessera-patch-generic.md`
- `assets/prompts/copilot/tessera-patch-apply.prompt.md`

Plus `docs/agent-as-provider.md` (the canonical contract reference).

In each, document the `created_by` field as: optional, parent feature slug, ordering/label hint only, currently inert.

Run `go test ./assets/...` after each skill is updated to catch drift early.

#### 4. Strict scope guards

DO NOT in M14.2:
- Compose DAG labels or add the `blocked-by-parent-and-needs-resolution` compound verdict (M14.3)
- Touch reconcile topological traversal (M14.3)
- Add `tpatch status --dag` output (M14.4)
- Bump version, update CHANGELOG, or tag (M14.4 supervisor task at v0.6.0)
- Add new external Go dependencies

### External reviewer guard (still applies)

Any new logic must read `status.Reconcile.Outcome` for reconcile-result decisions, NEVER `artifacts/reconcile-session.json`. M14.2 doesn't touch reconcile, but `created_by` is read by M14.3's label composition — do NOT introduce any convenience that reads the session artifact in M14.2 prep.

### Validation gate

```
gofmt -l .
go build ./cmd/tpatch
go test ./...
go test ./assets/...   # parity guard
go test ./internal/workflow -run 'DependencyGate|CreatedBy|Recipe' -count=1 -v
```

### Workflow

1. Update CURRENT.md "Status: In Progress".
2. Read ADR-011 (D4, D5 especially), PRD §3.2, §3.4, parity guard test.
3. Add the recipe field + write the parity-guard-respecting tests FIRST. Run `go test ./assets/...`. (Get the parity guard green BEFORE adding the gate.)
4. Update the 6 skill formats in lockstep with the Go struct.
5. Implement `CheckDependencyGate` + tests. Wire into apply.
6. Run full validation gate.
7. 2-3 logical commits, all with the `Co-authored-by` trailer.
8. Push to `origin/main`.
9. Final CURRENT.md update flagging "ready for code-review sub-agent".

### Out-of-band reminder for the implementer

The repo's tpatch binary at root is NOT gitignored. After `go build ./cmd/tpatch`, delete the binary or build into `/bin/`. Don't commit it.

### Deferred behind M14.2

- M14.3 — Reconcile topo + composable labels + compound verdict (~500 LOC)
- M14.4 — `status --dag` + skills analyze-phase bullet + `docs/dependencies.md` + tag v0.6.0 (~300 LOC)

### Registered follow-ups (unchanged)

- `feat-ephemeral-mode` — depends on `feat-feature-import` + `feat-delivery-modes`
- `feat-feature-reorder` — depends on `feat-feature-dependencies` (i.e., M14)
- `feat-resolver-dag-context`, `feat-feature-autorebase`, `feat-amend-dependent-warning`
- `feat-skills-apply-auto-default`, `bug-record-roundtrip-false-positive-markdown`, `chore-gitignore-tpatch-binary`
