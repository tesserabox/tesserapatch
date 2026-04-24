# Current Handoff

## Active Task

- **Task ID**: M14.1 — Feature Dependency DAG: data model + validation
- **Milestone**: M14 / Tranche D / v0.6.0
- **Status**: 🔨 Ready to start — C2 correctness baseline shipped as v0.5.2 ✅
- **PRD**: `docs/prds/PRD-feature-dependencies.md`
- **ADR**: `docs/adrs/ADR-011-feature-dependencies.md` (9 decisions locked)
- **Previous**: Tranche C2 / v0.5.2 — archived in `HISTORY.md`

## M14.1 Scope (~300 LOC)

Land the foundation for the stacked-feature DAG without any user-visible behavior change:

1. **Data model** (`internal/store/types.go`):
   - `Dependency` struct with `Slug string`, `Kind string` (hard|soft), `SatisfiedBy string` (optional — e.g. `"upstream_merged@<sha>"`)
   - `FeatureStatus.DependsOn []Dependency`
   - Backward-compat JSON decode: missing field → empty slice, no error

2. **Cycle detection** (new file, likely `internal/store/dag.go` or `internal/workflow/dag.go` — pick whichever aligns with existing package boundaries):
   - `AddDependency(s *Store, childSlug string, dep Dependency) error` — DFS from child through proposed edge; if head is reachable from tail, reject with `ErrCyclicDependency` listing full cycle path
   - Companion: `RemoveDependency(s *Store, childSlug string, parentSlug string) error`
   - Kahn's algorithm helper `TopoOrder(s *Store) ([]string, error)` — deterministic tiebreaker = lexicographic slug order

3. **Validation rules** (enforce on every `AddDependency`):
   - Self-dependency → `ErrSelfDependency`
   - Unknown parent slug → `ErrUnknownParent`
   - Cycle → `ErrCyclicDependency` (with path)
   - Duplicate edge (same child+parent+kind) → `ErrDuplicateDependency`
   - Same child→parent pair with both hard AND soft → `ErrDependencyKindConflict` (reject; user must remove one)

4. **Config flag plumbing** (`internal/store/store.go`):
   - `features.dependencies: bool` in config.yaml (default `false`)
   - Helper `func (c *Config) DependenciesEnabled() bool` — all DAG code paths MUST call this helper
   - Any `AddDependency` call on a repo with the flag off returns `ErrDependenciesDisabled`
   - Any `status.json` read that encounters `depends_on` with flag off: log warning, ignore field (forward-compat)

5. **Tests** (`internal/store/dag_test.go`):
   - Cycle detection fixtures: 2-cycle, 3-cycle, deep-chain-no-cycle, diamond-no-cycle, self-loop
   - Each of the 5 validation rules → one positive + one negative test
   - `TopoOrder` determinism: same graph produces same order, 10 runs
   - Backward-compat decode: status.json without `depends_on` field still valid
   - Flag-gated: `AddDependency` with flag off returns `ErrDependenciesDisabled`

## Out of Scope for M14.1

- CLI flag surface (`tpatch define --depends-on`) — M14.2
- Apply gate + `created_by` recipe op — M14.2
- Reconcile topological traversal + composable labels — M14.3
- `status --dag` + skill updates — M14.4

M14.1 ships as internal infrastructure only. No user-facing behavior change. No skill changes.

## Architectural Constraints (from ADR-011)

1. `depends_on` lives in `status.json` only — no `feature.yaml` field, no migration.
2. DFS for cycle detection (gives actionable path), Kahn for operator traversal (deterministic, easy tiebreaker).
3. `features.dependencies` flag gates the entire code path until v0.6.0 atomic flip.
4. Errors carry full cycle paths — operator must be able to act on the message alone.

## Validation Gates

- `gofmt -l .` empty
- `go build ./cmd/tpatch` ok
- `go test ./...` all green
- No new external Go dependencies
- Parity guard still passes (assets untouched in M14.1)

## Tranche D Roadmap

| Milestone | Scope | Status |
|---|---|---|
| ADR-011 | 9 decisions locked | ✅ committed `765542c` |
| M14.1 | Data model + validation (~300 LOC) | 🔨 **this** |
| M14.2 | Apply gate + `created_by` + 6-skill rollout (~250 LOC) | blocked on M14.1 |
| M14.3 | Reconcile topo traversal + composable labels + compound verdict (~500 LOC) | blocked on M14.2 |
| M14.4 | `status --dag` + skills + v0.6.0 release (~300 LOC) | blocked on M14.3 |

M14.3 will extend `workflow.AcceptShadow` (shipped in v0.5.2) for the `blocked-by-parent-and-needs-resolution` compound verdict. C2 correctness baseline is stable.

## Registered follow-ups (not in any tranche yet)

- `feat-ephemeral-mode` — one-shot add-feature with no tracking artifacts; depends on `feat-feature-import` + `feat-delivery-modes`
- `feat-feature-reorder` — flip parent-child in DAG; depends on `feat-feature-dependencies`
- `feat-resolver-dag-context` — parent-patch to M12 resolver
- `feat-feature-autorebase` — auto-rebase child on parent drift
- `feat-amend-dependent-warning` — stale-parent-* labels
- `feat-skills-apply-auto-default` — 6 skills still reference `--mode prepare/execute/done`; v0.5.1 flip not documented
- `bug-record-roundtrip-false-positive-markdown` — shipped `--lenient` fallback only; needs live repro for root-cause fix
- `chore-gitignore-tpatch-binary` — trivial one-liner; bundle into next release

## Context for Next Agent

- ADR-011 is the single authority for design decisions. Read it before editing. PRD §3.4 has a terminology drift between "verdicts" and "labels" — the ADR normalizes in favor of **labels**.
- The `workflow.AcceptShadow` helper shipped in v0.5.2 is the primitive M14.3 will reuse. Do not re-extract; read `internal/workflow/accept.go` first.
- Parity guard (`assets/assets_test.go`) is touch-sensitive. M14.1 should NOT edit assets — if you do, every skill format needs a synchronized update.
- Follow AGENTS.md handoff cadence: update `CURRENT.md` Session Summary + Files Changed at every phase transition, not only at end.
