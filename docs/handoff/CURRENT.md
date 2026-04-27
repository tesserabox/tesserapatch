# Current Handoff

## Active Task

- **Task ID**: `M15-W2` (Wave 2 — Path B correctness and ergonomics)
- **Milestone**: M15 stream — v0.6.x stabilization
- **Status**: Dispatching implementer
- **Assigned**: 2026-04-26

## Session Summary

**M15-W1 closed (APPROVED WITH NOTES).** Three polish items shipped + one review-note follow-up:

| SHA | Item |
|---|---|
| `aa0f93e` | `feat-satisfied-by-reachability` — `git merge-base --is-ancestor` validation |
| `d5f934f` | `chore-skill-frontmatter` — Copilot + Claude SKILL.md frontmatter |
| `99ee60e` | `feat-define-spec-alias` — cobra alias `spec` → `define` |
| `192935b` | M15-W1 impl handoff |
| `76fcfef` | M15-W1 reviewer verdict |
| `57bf1ab` | review-note follow-up: validation-layer git-error path tests |

## Wave 2 Dispatch Contract — 4 items

Run in order. Each item validates with focused tests, then `gofmt -l .` → `go test ./...` → `go build ./cmd/tpatch && rm -f tpatch`.

### Item 1 — `bug-test-command-shell-selection`

**Goal**: Stop hardcoding `sh -c` for the project test command. Windows/WSL flows fail today on the wrapper, not on the project's actual test command.

**Probe**: `internal/workflow/validation.go` and `internal/cli/phase2.go` — search for `exec.Command("sh"` or similar. Identify all sites that shell out to run user-configured commands (test command + shadow validation hook).

**Implementation**:
- Build a small `internal/safety` (or `internal/workflow`) helper `RunUserShellCommand(ctx, dir, command string) error` that picks shell per `runtime.GOOS`:
  - Unix-like → `sh -c <command>`
  - Windows → `cmd /C <command>`
- Optional: respect a `shell` field in `config.yaml` if present (string, full path or shell name); fall back to OS default.
- Update all sites that previously used raw `sh -c`.
- Tests: cover both branches via `runtime.GOOS` injection (or stub var) + a smoke test that the helper actually invokes the configured shell. Don't write Windows-specific tests that require a Windows runner; verify behavior via the shell-selection function in isolation.

**Constraint**: Default behavior on Unix must be byte-identical to today's `sh -c` (no surprise behavior changes for the existing user base).

### Item 2 — `feat-record-autogen-recipe`

**Goal**: When `tpatch record` runs and no `apply-recipe.json` exists for the feature (Path B / manual flow), auto-generate a minimal recipe from the captured patch so future replay/inspection works.

**Probe**:
- `internal/cli/cobra.go` `recordCmd` — current capture path.
- `internal/workflow/refresh.go` (if exists) — recipe persistence helpers.
- `internal/workflow/recipe.go` — recipe schema + executors.

**Implementation**:
- After patch capture in `recordCmd`, check if `feature/apply-recipe.json` exists. If missing, derive a minimal recipe from the captured patch:
  - Parse the unified diff
  - For each file touched, emit a recipe op (likely `replace-in-file` or `apply-patch` op type — pick whichever is already supported in the schema for "this is a patch capture")
  - Serialize the recipe and write it next to the patch
- The recipe is **for replay/inspection** — `artifacts/post-apply.patch` remains the **source-of-truth for reconcile** (do NOT invert this).
- New behavior should be opt-out via `--no-recipe-autogen` flag. Default: on (closes a real Path B gap).
- Tests: feature without recipe + record → recipe materialized; feature with existing recipe → no overwrite; `--no-recipe-autogen` honored.

**Constraint**: Recipe ops produced must validate against the existing recipe-op JSON schema enforced by the parity guard. If the schema doesn't support a clean "patch-derived" op, surface the constraint and propose a minimal schema extension — but DO NOT silently extend.

### Item 3 — `bug-recipe-stale-after-manual-flow`

**Goal**: After a manual `tpatch implement <slug> --manual` flow, the existing `apply-recipe.json` becomes stale (it doesn't reflect what the manual edits actually did). Today no warning surfaces.

**Implementation**:
- After Path B `--manual` implement OR after `record` updates the patch, detect drift between `apply-recipe.json` and the captured patch:
  - Re-run recipe in dry-run against a clean shadow → produce a synthetic patch.
  - Compare to the recorded patch (file list, hunk count, or hash).
  - On drift, EITHER (a) regenerate the recipe via the Item 2 autogen path, OR (b) mark the recipe stale (e.g., add a `stale: true` field + warn the user).
- Pick approach (a) by default if the autogen path lands cleanly in Item 2; pick approach (b) if approach (a) risks data loss.
- The fix MUST resolve stale/misleading recipes — not just suppress the warning.

**Constraint**: This is tightly coupled with Item 2. Land them as a coherent pair (one or two commits). Tests cover: manual flow → recipe regenerated/marked stale; recipe-only flow (no manual) → no false-positive drift.

### Item 4 — `feat-record-scoped-files`

**Goal**: `tpatch record <slug>` currently captures the whole working-tree diff, which causes cross-feature pollution when multiple features have edits in flight.

**Implementation**:
- Add `--files=<glob,glob,...>` (or `--scope=<file>`) flag to `recordCmd`.
- When set: `git diff` is invoked with explicit pathspec arguments → only changes under those paths land in the captured patch.
- When unset: behavior unchanged (full-tree capture).
- Tests: scoped capture vs. full-tree capture against a multi-file dirty tree.
- Integration: after Item 2 lands, the recipe-autogen must respect the scope (only generate ops for files that ended up in the captured patch).

**Constraint**: Default behavior must be byte-identical to today's full-capture.

## Validation Cadence

- After each item: focused tests for the touched package(s).
- After all 4: `gofmt -l .` empty, `go test ./...` green, `go build ./cmd/tpatch && rm -f tpatch`, `git status` clean.
- Commit one per item OR a tightly coupled pair (Items 2+3 may share a commit if intertwined). Push after each commit.

## Out of Scope for Wave 2

- `feat-verify-command`, `feat-feature-tested-state`, `feat-reconcile-code-presence-verdicts`, `feat-reconcile-fresh-branch-mode` — Wave 3+, gated behind a supervisor review pause.
- ROADMAP / CHANGELOG / version bumps — supervisor handles after Wave 2 review.

## Current State

- HEAD `57bf1ab` on `origin/main` (locally — push happens after this handoff is committed).
- Build clean, full test suite green, parity guard holds.
- 64 → 61 pending todos in SQL after Wave 1 (3 marked done by archive sweep).

## Blockers

None.

## Context for Next Agent

- **`tpatch` binary at the repo root is NOT gitignored.** Bare `tpatch` ignore would shadow `cmd/tpatch/`. Always `rm -f tpatch` after `go build ./cmd/tpatch` BEFORE staging. This has slipped multiple times.
- **Commit trailer mandatory**: `Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>`. Use `git -c commit.gpgsign=false`.
- **Source-truth guard (ADR-011 D6)**: any DAG/label/status code reads `status.Reconcile.Outcome` via `store.LoadFeatureStatus`, NEVER `artifacts/reconcile-session.json`.
- **Recipe vs patch authority**: `artifacts/post-apply.patch` is the reconcile source of truth. Recipes serve replay/inspection. Do NOT invert this in Wave 2 work.
- **Skill parity guard** (`assets/assets_test.go`) enforces required CLI-command anchors and the recipe-op JSON schema. Treat as a real reviewer.
- **`git push` is slow** (60+ s typical).
- **Wave 1 architectural pattern**: hookable package-level `var` for git/external-call sites lets unit tests stay environment-free. Use the same pattern for shell selection in Item 1 if helpful.
- **Wave 1 test pattern**: shared `contains(haystack, needle string) bool` helper already exists in `internal/store/store_test.go` — don't redeclare in the same package.
