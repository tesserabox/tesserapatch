# Current Handoff

## Active Task

- **Task ID**: `M15-W2` (Wave 2 — Path B correctness and ergonomics)
- **Milestone**: M15 stream — v0.6.x stabilization
- **Status**: Implementation complete — review pending
- **Assigned**: 2026-04-26

## Session Summary

All four Wave 2 items shipped across three commits:

| SHA | Item |
|---|---|
| `e7f524d` | `bug-test-command-shell-selection` — OS-aware shell helper (`workflow.UserShell`) routes the three former `sh -c` call sites; Unix path byte-identical |
| `dbd44c2` | `feat-record-autogen-recipe` + `bug-recipe-stale-after-manual-flow` — patch-derived autogen of `apply-recipe.json` when missing; `recipe-stale.json` sidecar on drift; `--no-recipe-autogen` opt-out, `--regenerate-recipe` to overwrite |
| `d402653` | `feat-record-scoped-files` — `--files=<pathspec,...>` flag on `tpatch record` with `CapturePatchScoped` helper; default unset preserves full-tree capture byte-for-byte |

## Files Changed

**Item 1 — shell selection**

- `internal/workflow/shell.go` (new)
- `internal/workflow/shell_test.go` (new)
- `internal/workflow/validation.go` (two `sh -c` sites → `UserShell`)
- `internal/cli/phase2.go` (one `sh -c` site → `UserShell`)

**Items 2 + 3 — recipe autogen + stale detection**

- `internal/workflow/recipe_autogen.go` (new) — `RecipeFromPatch`, `AutogenRecipeForRecord`, `RecipeStaleness` sidecar type, file-set drift compare
- `internal/workflow/recipe_autogen_test.go` (new) — 9 tests (parse, generate, skip-if-off, noop, stale-marker, regenerate, clear-on-realign, schema allowlist)
- `internal/cli/cobra.go` (`recordCmd` wiring + new flags)

**Item 4 — scoped capture**

- `internal/gitutil/gitutil.go` (refactor `CapturePatch` → thin wrapper over new `CapturePatchScoped`)
- `internal/gitutil/capture_scoped_test.go` (new) — default parity, narrowing, multi-pathspec
- `internal/cli/cobra.go` (`--files` flag + `--from` clash guard)
- `internal/cli/cobra_test.go` (two integration tests)

## Test Results

```
ok  	github.com/tesseracode/tesserapatch/assets
ok  	github.com/tesseracode/tesserapatch/internal/cli         5.391s
ok  	github.com/tesseracode/tesserapatch/internal/gitutil     4.120s
ok  	github.com/tesseracode/tesserapatch/internal/provider
ok  	github.com/tesseracode/tesserapatch/internal/safety
ok  	github.com/tesseracode/tesserapatch/internal/store       0.534s
ok  	github.com/tesseracode/tesserapatch/internal/workflow    9.720s
```

`gofmt -l .` empty. `go build ./cmd/tpatch && rm -f tpatch` clean. Working
tree clean before push.

## Reviewer Attention Points

- **Recipe schema gap (deletions)**: `RecipeFromPatch` skips deleted files
  with a stderr warning because the recipe-op schema (parity guard)
  defines no `delete-file` op. This is a documented gap, NOT a silent
  schema extension. If reviewer wants delete coverage, that requires a
  schema-extension ADR + parity-guard update — flagged for Wave 3+.
- **Stale resolution (Item 3 design choice)**: when an existing recipe
  drifts from the captured patch, the default action is to write a
  `recipe-stale.json` sidecar and warn — the existing recipe is **not**
  overwritten, because a provider-generated recipe may carry richer
  `replace-in-file` context or `created_by` edges that a patch-derived
  recipe cannot reproduce. `--regenerate-recipe` is the explicit user
  action to overwrite; the sidecar self-clears once the recipe matches
  the captured patch again.
- **Drift detection scope**: file-set comparison only (path inclusion).
  Same files but altered content does not trigger drift. Sufficient for
  the manual-edit-after-implement scenario but a deliberate floor; a
  hash-based deep compare is a follow-up if needed.
- **`--files` + `--from` rejection**: explicit error rather than
  silently-ignored pathspecs. Prevents a misleading "captured nothing"
  outcome.
- **Source-of-truth invariant preserved**: `artifacts/post-apply.patch`
  remains the reconcile authority everywhere. Recipes serve replay /
  inspection only, even after autogen.

## Next Steps

Awaiting reviewer dispatch on M15-W2. Wave 3 holds for supervisor
review pause (verify command, tested state, reconcile semantics).

## Blockers

None.

## Constraints (still valid for next agent)

- Only edit files inside `tpatch/`.
- `tpatch` binary at repo root is **not** gitignored — always
  `rm -f tpatch` after `go build ./cmd/tpatch` BEFORE staging.
- Commit trailer mandatory: `Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>`.
- Source-truth guard (ADR-011 D6): label/status code reads
  `status.Reconcile.Outcome` via `store.LoadFeatureStatus`, never
  `artifacts/reconcile-session.json`.
- Recipe vs patch authority: `artifacts/post-apply.patch` is the
  reconcile source of truth.
- Skill parity guard (`assets/assets_test.go`) — recipe-op schema is
  enforced; the autogen path emits only `write-file` ops to stay
  inside the allowlist.
- `git push` is slow (60+s typical).

## Out of Scope for Wave 2 (still gated for Wave 3)

- `feat-verify-command`, `feat-feature-tested-state`,
  `feat-reconcile-code-presence-verdicts`, `feat-reconcile-fresh-branch-mode`.
- ROADMAP / CHANGELOG / version bumps.

## Context for Next Agent

- `RecipeStaleness` is held as a sidecar (`recipe-stale.json`), not a
  field on `ApplyRecipe`, so the parity guard's
  `DisallowUnknownFields` decoder against skill JSON examples stays
  green without touching the 6 skill assets.
- `userShellFor(goos)` is the testable seam for shell selection — same
  hookable-var pattern Wave 1 used for `gitutil.IsAncestor`.
- The autogen + stale path runs unconditionally after a successful
  patch capture in `recordCmd`, after `MarkFeatureState`. Failures
  inside the autogen helper are reported as warnings on stderr and do
  not fail the record itself.
