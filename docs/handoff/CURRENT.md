# Current Handoff

## Active Task

- **Task ID**: (idle — awaiting next tranche scope from supervisor)
- **Status**: ✅ Idle — Tranche C1 / v0.5.1 **APPROVED, pushed, live on `origin/main`**
- **Milestone**: (no active milestone — M13 closed)
- **Previous**: M13 / Tranche C1 / v0.5.1 — archived in `HISTORY.md`; review verdict in `docs/supervisor/LOG.md`

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
