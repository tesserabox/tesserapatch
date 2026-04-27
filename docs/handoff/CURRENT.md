# Current Handoff

## Active Task

- **Task ID**: `M15-W1` (Wave 1: `feat-satisfied-by-reachability`, `chore-skill-frontmatter`, `feat-define-spec-alias`)
- **Milestone**: M15 stream — v0.6.x stabilization and Path B follow-through
- **Status**: Implementation complete — review pending
- **Assigned**: 2026-04-26

## Session Summary

Three Wave 1 polish items landed as three focused commits, each with its own tests.

1. **`aa0f93e` — `feat(validation): verify satisfied_by SHA reachability via git merge-base`**
   Closes the deliberate M14.1 limitation where any well-formed hex string was accepted as `satisfied_by` provenance as long as the parent state was `upstream_merged`. Adds `gitutil.IsAncestor` (exit-code-aware wrapper around `git merge-base --is-ancestor`: exit 0 → reachable, exit 1 → unreachable, otherwise an error). Wires the check into both `ValidateDependencies` and `ValidateAllFeatures`, gated on the parent already being `upstream_merged` (no double-fail with the requires-upstream rule). Introduces a hookable `isAncestor` package var so unit tests stay git-free.

2. **`d5f934f` — `chore(skills): add YAML frontmatter to Copilot + Claude SKILL.md`**
   Prepends a `name: tessera-patch` / `description: …` YAML block to `assets/skills/copilot/tessera-patch/SKILL.md` and `assets/skills/claude/tessera-patch/SKILL.md` so the Copilot CLI / Claude Code skill loaders accept the file. Cursor `.mdc` (already has its own frontmatter format) and Windsurf rules (no frontmatter convention) audited and left as-is. Parity guard untouched — frontmatter doesn't remove any required CLI-command anchor.

3. **`99ee60e` — `feat(cli): add `spec` as an alias for `define`**
   `Aliases: []string{"spec"}` on `defineCmd()`. Alias only — same RunE, same flags, identical semantics. Two new tests (`TestSpecAliasResolvesToDefine`, `TestSpecAliasRunsDefine`). One small parenthetical doc note in `docs/feature-layout.md`. Skills + parity guard left untouched: `tpatch define` remains the canonical anchor.

## Current State

- HEAD `99ee60e`, two commits ahead of origin/main locally pre-push (push happens after this handoff is committed).
- Build clean, full test suite green, parity guard holds.
- ROADMAP unchanged (supervisor handles release/milestone box flips).
- Wave 2 (`bug-test-command-shell-selection`, `feat-record-autogen-recipe`, `bug-recipe-stale-after-manual-flow`, `feat-record-scoped-files`) intentionally NOT started — supervisor dispatches separately after Wave 1 is reviewed.

## Files Changed

- `internal/gitutil/gitutil.go` — new `IsAncestor` helper.
- `internal/gitutil/gitutil_test.go` — `TestIsAncestor` covering reachable / unreachable / bogus-ref.
- `internal/store/validation.go` — new `ErrSatisfiedBySHANotReachable`, `isAncestor` hook, reachability checks in both validators.
- `internal/store/validation_test.go` — `stubIsAncestor` helper, three new tests, existing `…OnUpstreamMerged` test updated to stub.
- `assets/skills/copilot/tessera-patch/SKILL.md` — YAML frontmatter prepended.
- `assets/skills/claude/tessera-patch/SKILL.md` — YAML frontmatter prepended.
- `internal/cli/cobra.go` — `Aliases: []string{"spec"}` on `defineCmd()`.
- `internal/cli/cobra_test.go` — `TestSpecAliasResolvesToDefine` + `TestSpecAliasRunsDefine`.
- `docs/feature-layout.md` — alias parenthetical on the `spec.md` row.

## Test Results

```
ok  github.com/tesseracode/tesserapatch/assets
?   github.com/tesseracode/tesserapatch/cmd/tpatch[no test files]
ok  github.com/tesseracode/tesserapatch/internal/cli
ok  github.com/tesseracode/tesserapatch/internal/gitutil
ok  github.com/tesseracode/tesserapatch/internal/provider
ok  github.com/tesseracode/tesserapatch/internal/safety
ok  github.com/tesseracode/tesserapatch/internal/store
ok  github.com/tesseracode/tesserapatch/internal/workflow
```

`gofmt -l .` clean. `go build ./cmd/tpatch` succeeded; root binary removed.

## Next Steps

Awaiting reviewer dispatch on M15-W1. Wave 2 holds until M15-W1 APPROVED.

## Blockers

None.

## Context for Next Agent

- **Reachability check is gated on `parent.State == StateUpstreamMerged`.** This is intentional: when the parent is in any other state, `ErrSatisfiedByRequiresUpstream` already fires and the reachability rule would just produce a noisier double-error. ADR-011 D5 still holds — `satisfied_by` is provenance metadata; runtime semantics are unchanged.
- **`isAncestor` is a package-level `var` hook in `internal/store`.** Tests stub it via `stubIsAncestor(t, ok, err)` which restores via `t.Cleanup`. If a future test creates a real git repo and wants the live behavior, just don't call the stub.
- **The `gitutil.IsAncestor` failure path returns `(false, err)` only for non-zero, non-1 exits** (e.g., bogus SHA, repo missing). Callers must NOT treat the error as "unreachable" — they may want to surface it as a configuration problem.
- **`spec` is alias-only.** Do not bulk-rewrite skills/docs to mention it — `tpatch define` remains the canonical CLI-command anchor enforced by the parity guard. The doc touch in `docs/feature-layout.md` is a single parenthetical and intentionally minimal.
- **Frontmatter prepend used only `name` + `description`.** No `globs`, no `alwaysApply` — Copilot/Claude loaders require frontmatter but don't consume those Cursor-specific keys, and adding them would be cargo-cult. Cursor's existing `.mdc` keeps its own keys.
- **`tpatch` binary at the repo root is NOT gitignored.** Bare `tpatch` ignore would shadow `cmd/tpatch/`. Always `rm -f tpatch` after `go build ./cmd/tpatch` BEFORE staging.
- **Commit trailer mandatory**: `Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>`. Use `git -c commit.gpgsign=false`.
