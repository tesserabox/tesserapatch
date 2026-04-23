# Current Handoff

## Active Task

- **Task ID**: M14 / Tranche D / v0.6.0 — Feature Dependencies / DAG (**scoping phase**)
- **Status**: 🔨 PRD approved (`fa4bbb6`) — next: draft ADR-011 before any M14.1 code
- **PRD**: `docs/prds/PRD-feature-dependencies.md` (736 lines, APPROVED WITH NOTES after 3 review cycles)
- **Milestone**: `docs/milestones/M14-feature-dependencies.md` (to be created)
- **Previous**: M13 / Tranche C1 / v0.5.1 shipped ✅ (archived in `HISTORY.md`)

### Next steps

1. ✅ ~~Draft `docs/adrs/ADR-011-feature-dependencies.md`~~ — done (9 decisions locked, terminology normalized per PRD §3.4 drift).
2. Create `docs/milestones/M14-feature-dependencies.md` with the 4-sub-milestone contract.
3. Begin M14.1 implementation (data model + validation).

### Tranche D scope (v0.6.0)

| Milestone | Scope | Est. LOC |
|---|---|---|
| M14.1 | Data model + validation (Dependency struct, cycle DFS, 5 rules) | ~300 |
| M14.2 | Apply gate + `created_by` recipe op + 6-skill parity-guard rollout | ~250 |
| M14.3 | Reconcile topological traversal + composable labels + compound verdict | ~500 |
| M14.4 | `status --dag` + skills + release v0.6.0 | ~300 |

SQL: `SELECT id, status FROM todos WHERE id='adr-011-feature-dependencies' OR id LIKE 'm14.%' ORDER BY id;`

### Decisions locked in PRD (to be codified in ADR-011)

1. `depends_on` in `status.json` only (no new `feature.yaml`, no migration)
2. DFS for cycle detection, Kahn's algorithm for operator traversal
3. `waiting-on-parent` + `blocked-by-parent` are **composable derived labels** (not states) — both can coexist on one feature
4. `created_by` recipe op gated by **hard deps only** (soft deps emit warnings, not errors)
5. `upstream_merged` satisfies hard deps (parent can be gone if it landed upstream)
6. Child's own reconcile verdict **always computed first**; parent labels overlay clean verdicts; intrinsic `blocked-*` never masked
7. New compound verdict `blocked-by-parent-and-needs-resolution` for `3WayConflicts + blocked parent` case
8. `remove --cascade` required to delete parents with dependents — `--force` alone does NOT bypass
9. Parent-patch context **NOT** passed to M12 conflict resolver in v0.6 (deferred to `feat-resolver-dag-context`)

### Follow-ups deferred from PRD (registered in SQL)

- `feat-resolver-dag-context` — parent-patch to M12 resolver
- `feat-feature-autorebase` — auto-rebase child on parent drift
- `feat-amend-dependent-warning` — stale-parent-* labels (implemented alongside M14.2 but tracked separately)

### Registered follow-ups (not in any tranche yet)

- `feat-skills-apply-auto-default` — 6 skills still reference `--mode prepare/execute/done`; v0.5.1 flip not documented
- `bug-record-roundtrip-false-positive-markdown` — shipped `--lenient` fallback only; needs live repro for root-cause fix
- `chore-gitignore-tpatch-binary` — trivial one-liner; bundle into next release

## Session Summary — 2026-04-23 — PRD authoring for feat-feature-dependencies

Supervisor-driven sub-agent cycle: implementation sub-agent drafted PRD v1 (453 lines) → rubber-duck review surfaced 6 critical issues → v2 revision (697 lines) addressed all 6 but introduced 4 new contradictions → rubber-duck review flagged them → v3 revision (736 lines) fixed all 4 → final review **APPROVED WITH NOTES**. 3 full review cycles, 1 minor non-blocking cleanup note (terminology normalization deferred to ADR-011).

PRD committed `fa4bbb6`. Supervisor log updated. ROADMAP M14 block populated. SQL: parent feat marked done; `adr-011-feature-dependencies` + `m14.1`-`m14.4` chain inserted with dependencies; 3 follow-ups registered.

## Files Changed

- `docs/prds/PRD-feature-dependencies.md` — NEW — 736 lines
- `docs/ROADMAP.md` — M14 section populated
- `docs/supervisor/LOG.md` — PRD review cycle entry
- `docs/handoff/CURRENT.md` — this file, flipped to M14 scoping state

## Test Results

N/A — docs-only session.

## Next Steps

1. Draft ADR-011 (can be done as a sub-agent task or directly by supervisor — small scope).
2. Create `docs/milestones/M14-feature-dependencies.md` with the 4-sub-milestone contract.
3. Launch M14.1 implementation sub-agent once ADR-011 is in place.

## Blockers

None. ADR-011 is the only gating artifact before M14.1 coding starts.

## Context for Next Agent

- PRD review had **3 passes** and every pass improved the artifact materially — this is the pattern for non-trivial features. Budget review cycles, don't treat first-pass approval as the norm.
- Rubber-duck agent is highly effective at catching self-introduced contradictions in revisions. Always re-review after revisions.
- `m14.1-data-model` must not start until ADR-011 is committed — it's a repo rule per AGENTS.md.
- PRD has ONE non-blocking cleanup note: §3.4 still uses enum-style `ReconcileWaitingOnParent` / `ReconcileBlockedByParent` verdicts while §4.5 locks label semantics. ADR-011 should normalize (labels win).

### Post-release user testing

User did manual testing after release — no bugs reported. Removed the stray `tpatch` build artifact from repo root manually.

### Registered follow-ups (not in any tranche yet)

- **Skill-asset refresh for apply default flip** — all 6 skill formats + `docs/agent-as-provider.md` still reference `apply --mode prepare/execute/done` explicitly. New `--mode auto` default is not documented there. Low-priority polish; cluster with next skill touch.
- **`bug-record-roundtrip-false-positive-markdown`** — shipped `--lenient` fallback only. Real repro needed to root-cause. Re-open if a user reports live.
- **`.gitignore /tpatch`** — bare binary at repo root from `go build ./cmd/tpatch` is not gitignored. Trivial one-line fix bundled into next tranche.

## Session Summary — 2026-04-22 — Tranche C1 / v0.5.1 shipped

10 commits on `main`, pushed to `origin`. Tag `v0.5.1` pushed. All tests green. No new Go deps.

| # | Item | Commit |
|---|---|---|
| 1 | c1-recipe-stale-guard | `4f49c76` |
| 2 | c1-apply-default-execute | `3a12b2e` |
| 3 | c1-add-stdin | `d727ea2` |
| 4 | c1-progress-indicator | `5dba3b4` |
| 5 | c1-edit-flag | `1dbc812` |
| 6 | c1-feature-amend | `36587c9` |
| 7 | c1-feature-removal | `958e6d0` |
| 8 | c1-record-lenient | `5dae00b` |
| 9 | release(v0.5.1) | `e069cd8` + tag `v0.5.1` |
| 10 | supervisor log: C1 review — APPROVED | `c4cccb3` |

### Breaking UX

- `tpatch apply` default mode flipped from `prepare` to `auto`. Users relying on the previous behavior must pass `--mode prepare` explicitly.

### Notes for next agent

- **Item 8 shipped as fallback, not root-cause fix.** Three synthetic repros of `bug-record-roundtrip-false-positive-markdown` (trailing whitespace, new untracked markdown with `--intent-to-add`, modified tracked markdown) all passed reverse-apply cleanly. Without a live fixture, I shipped the documented `--lenient` escape hatch instead of a speculative `--ignore-whitespace` fix. If the bug resurfaces with a real repro, revisit.
- **Recipe provenance is a sidecar** (`artifacts/recipe-provenance.json`), not a field on `apply-recipe.json` — avoids changing all 6 skill formats + failing the strict `DisallowUnknownFields` parity guard.
- **Spinner lives at the single `GenerateWithRetry` choke point.** Any new LLM-calling code path gets the spinner for free if it goes through that function.
- **`.gitignore` does NOT ignore a bare `tpatch` binary at repo root.** Don't `go build ./cmd/tpatch` from the root — it writes a binary that gets picked up by `git add -A`. Use `go vet + go test` only.
- **Stdin detection pattern**: `stdinIsPiped` (permissive — true for tests that use `cmd.SetIn(strings.NewReader(...))`) for input; `canPromptForConfirmation` (inverse, requires real TTY) for destructive ops.

## Files Changed (tranche C1 aggregate)

- `internal/cli/cobra.go` — version bump, apply default mode flip, addCmd stdin, stale-guard, record --lenient, c1 subcommand registrations.
- `internal/cli/c1.go` — NEW — edit/amend/remove commands.
- `internal/cli/cobra_test.go` — tests for all C1 items + shared helpers.
- `internal/workflow/implement.go` — `RecipeProvenance` sidecar.
- `internal/workflow/spinner.go` (NEW) + `spinner_test.go` (NEW).
- `internal/workflow/retry.go` — spinner wired in `GenerateWithRetry`.
- `internal/store/store.go` — `RemoveFeature`.
- `CHANGELOG.md` — v0.5.1 section.
- `docs/ROADMAP.md` — M13 status flipped to ✅.
- `docs/handoff/CURRENT.md` + `docs/handoff/HISTORY.md` — archived.

## Test Results

- `gofmt -l .` — clean.
- `go vet ./...` — clean.
- `go test ./...` — all packages green.

## Next Steps

1. ✅ Supervisor review of C1 commits — APPROVED (see `docs/supervisor/LOG.md`).
2. ✅ Pushed `main` + tag `v0.5.1` to `origin`.
3. ⏭️ Pick next tranche from ROADMAP M14+ backlog (see supervisor proposal in latest chat turn).

## Blockers

None.

## Context for Next Agent

- All C1 commits are single-purpose and can be reverted individually if any one item is rejected in review.
- `--mode prepare` → `--mode auto` default flip is the only user-visible regression risk. Skill assets were NOT updated in this tranche (still say "apply --mode prepare/started/done") — worth a follow-up touch if the new default sticks.
