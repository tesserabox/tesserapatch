# Current Handoff

## Active Task

- **Task ID**: Tranche C2 / v0.5.2 — correctness fix pass (**IN FLIGHT**)
- **Status**: 🔨 Implementation sub-agent dispatched (see `docs/supervisor/LOG.md` for verification trail)
- **Blocks**: M14.1 — cannot start data model work until reconcile `--resolve --apply` is truthful and the `refresh.go` path no longer dirties the user's index (M14.3 extends both)
- **Next on deck after C2**: ADR-011 ✅ done → M14.1 data model + validation

### C2 fix scope (7 items, verified real)

| ID | Severity | Finding |
|---|---|---|
| c2-resolve-apply-truthful | 🔴 Silent correctness bug | `--resolve --apply` sets `ReconcileReapplied` without copying shadow → real tree |
| c2-refresh-index-clean | 🟡 UX bug | `DiffFromCommitForPaths` leaves `git add -N` intent-to-add entries in user's index |
| c2-recipe-hash-provenance | 🟡 Guard incomplete | Stale guard catches HEAD drift but not recipe content drift |
| c2-remove-piped-stdin | 🟡 Contract drift | `printf y\| tpatch remove` refuses despite shipped contract saying piped stdin skips confirm |
| c2-amend-append-flag | 🟢 Feature add | Lock replace-default, add explicit `--append`, mutex with `--reset` |
| c2-max-conflicts-drift | 🟢 Doc drift | 6 sites claim default 3; code is 10 (CHANGELOG, agent-as-provider, 4 shipped skill/prompt files) |
| c2-release-v0.5.2 | supervisor | Tag after code-review sub-agent approves |

### Why before M14.1

1. Finding #1 is silent correctness on the v0.5.0 headline feature (`--resolve --apply`). Building DAG on top compounds the bug × N features in M14.3's Kahn traversal.
2. M14.3 extends `refresh.go` (finding #2's code) — fix the temp-index leak once, inherit clean plumbing.
3. The **shared accept-helper** extraction (finding #1's preferred fix) is the exact primitive M14.3's `blocked-by-parent-and-needs-resolution` compound verdict will need.
4. Skills max-conflicts drift will be re-touched by M14.2/M14.4 parity-guard rollouts anyway — cleaner to fix drift before the DAG adds 3 new label strings to the same skill files.

### Deferred decisions locked in PRD (for M14 reference)

1. `depends_on` in `status.json` only (no new `feature.yaml`, no migration)
2. DFS for cycle detection, Kahn's algorithm for operator traversal
3. `waiting-on-parent` + `blocked-by-parent` are **composable derived labels** (not states)
4. `created_by` recipe op gated by **hard deps only**
5. `upstream_merged` satisfies hard deps
6. Child's own reconcile verdict **always computed first**; parent labels overlay clean verdicts
7. `remove --cascade` required to delete parents with dependents — `--force` alone does NOT bypass
8. Parent-patch context **NOT** passed to M12 resolver in v0.6 (deferred to `feat-resolver-dag-context`)
9. All gated by `features.dependencies` config flag until v0.6.0 atomic flip

See `docs/adrs/ADR-011-feature-dependencies.md` for full rationale.

### Tranche D scope (v0.6.0, after C2)

| Milestone | Scope | Est. LOC |
|---|---|---|
| M14.1 | Data model + validation (Dependency struct, cycle DFS, 5 rules) | ~300 |
| M14.2 | Apply gate + `created_by` recipe op + 6-skill parity-guard rollout | ~250 |
| M14.3 | Reconcile topological traversal + composable labels + compound verdict | ~500 |
| M14.4 | `status --dag` + skills + release v0.6.0 | ~300 |

### Registered follow-ups (not in any tranche yet)

- `feat-ephemeral-mode` — one-shot add-feature mode with no tracking artifacts; depends on `feat-feature-import` + `feat-delivery-modes`
- `feat-feature-reorder` — flip parent-child in DAG; depends on `feat-feature-dependencies`
- `feat-resolver-dag-context` — parent-patch to M12 resolver
- `feat-feature-autorebase` — auto-rebase child on parent drift
- `feat-amend-dependent-warning` — stale-parent-* labels
- `feat-skills-apply-auto-default` — 6 skills still reference `--mode prepare/execute/done`; v0.5.1 flip not documented
- `bug-record-roundtrip-false-positive-markdown` — shipped `--lenient` fallback only; needs live repro for root-cause fix
- `chore-gitignore-tpatch-binary` — trivial one-liner; bundle into next release

## Session Summary — 2026-04-23 — PRD approved, C2 fix pass opened

Supervisor-driven: after ADR-011 shipped, reviewer session surfaced 4 confirmed bugs + 2 doc drifts. Verified findings #1, #2, #6 via direct code inspection (resolver.go:218-222 comment is explicit; gitutil.go:689-697 leaks intent-to-add; 6 skill/doc sites claim max-conflicts default 3 against code's 10). Registered 7 C2 todos with dependencies; M14.1 blocked behind v0.5.2 release.

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
