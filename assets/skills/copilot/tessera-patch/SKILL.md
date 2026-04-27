---
name: tessera-patch
description: Customize open-source projects through natural-language patches while maintaining upstream compatibility.
---

# Tessera Patch — GitHub Copilot Skill

## Overview

Tessera Patch customizes open-source projects through natural-language patches while maintaining upstream compatibility.

## Invocation

`tpatch` is a compiled Go binary on PATH. Invoke it directly — do NOT wrap it:

- ✓ `tpatch <command>`
- ✗ `npx tpatch …` (not a Node package)
- ✗ `npm run tpatch …` (not an npm script)
- ✗ `python -m tpatch …` (not a Python module)

Always run from the repository root (where `.tpatch/` exists). Do not `cd` to speculative paths — use the current working directory.

## Phase Ordering

```
requested    → tpatch analyze    → analyzed
analyzed     → tpatch define     → defined
defined      → tpatch explore    → defined (exploration.md enriched)
defined      → tpatch implement  → implementing (apply-recipe.json ready)
implementing → tpatch apply --mode execute                          → applied
             OR tpatch apply --mode started / edit / --mode done    → applied
applied      → tpatch record     → active
active       → tpatch reconcile  → active | upstream_merged | blocked
```

Never skip a phase. Never go backwards without `tpatch reconcile`.

## Before You Run Anything

1. `tpatch status <slug>` — see current state and last command.
2. `tpatch next <slug>` — get the exact next command (add `--format harness-json` for structured output).
3. Only then proceed. Do not guess the next phase from file presence.
4. Run tpatch record <slug> BEFORE git commit. If you already committed, use tpatch record <slug> --from <base> — a clean working tree without --from is refused.
5. Run tpatch reconcile only on a CLEAN working tree at the target upstream state. Commit or stash first; reconcile refuses dirty trees, conflict markers, and .orig/.rej leftovers. See docs/reconcile.md for the workflow patterns.

## CLI Commands

- `tpatch init` — Initialize workspace
- `tpatch add <description>` — Create feature request
- `tpatch status` — Feature dashboard
- `tpatch analyze <slug>` — Run analysis
- `tpatch define <slug>` — Generate acceptance criteria
- `tpatch explore <slug>` — Explore codebase
- `tpatch implement <slug>` — Generate apply recipe
- `tpatch apply <slug>` — Execute recipe or record session
- `tpatch record <slug>` — Capture patches
- `tpatch reconcile` — Reconcile with upstream
- `tpatch provider check` — Validate provider
- `tpatch config show|set` — Manage config
- `tpatch cycle <slug>` — Run analyze→define→explore→implement→apply→record in sequence (batch or `--interactive`)
- `tpatch test <slug>` — Run the configured `test_command` and record the outcome
- `tpatch verify <slug>` — Run integrity checks against a feature's recipe and dependencies (EXPERIMENTAL — Slice A: V0/V1/V2 only; full check set in later slices)
- `tpatch next <slug>` — Emit the next action for a feature (`--format harness-json` for structured consumption)

## Lifecycle

analyse → define → explore → implement → test → record → reconcile

## Data Model

Features are tracked in `.tpatch/features/<slug>/` with `status.json`, `request.md`, `analysis.md`, `spec.md`, and `artifacts/`.

## Reconciliation

4-phase decision tree: reverse-apply → operation-level → provider-semantic → forward-apply.

Outcomes: `upstream_merged`, `reapplied`, `blocked`.

## You Are the Provider

Every LLM phase has two paths:

- **Path A — CLI-driven**: `tpatch <phase> <slug>` — configured provider generates the artifact.
- **Path B — Agent-authored**: author the artifact yourself under `.tpatch/features/<slug>/`, then `tpatch <phase> <slug> --manual` to advance feature state without calling the provider.

You are the provider when no provider is configured, the provider returns empty/truncated/insufficient output (common with implement — 1-op stubs, ensure-directory-only, truncated JSON), or you have more context than it does. Path B is normal, not exceptional — do not wait for a better recipe.

Phase → artifact → state contract (the `--manual` flag validates this):

| phase | artifact | advances state to |
|---|---|---|
| analyze | `analysis.md` | `analyzed` |
| define | `spec.md` | `defined` |
| explore | `exploration.md` | `defined` |
| implement | `artifacts/apply-recipe.json` (JSON-validated) | `implementing` |

## apply-recipe.json schema

```json
{
  "version": 1,
  "operations": [
    { "type": "ensure-directory", "path": "src/feature/" },
    { "type": "write-file", "path": "src/a.ts", "content": "export const x = 1;\n" },
    { "type": "replace-in-file", "path": "src/b.ts",
      "search": "export * from \"./legacy\";\n",
      "replace": "export * from \"./legacy\";\nexport * from \"./feature/a\";\n" },
    { "type": "append-file", "path": "src/changelog.md",
      "content": "\n- added feature/a\n" }
  ]
}
```

Semantics:

- Ops: `ensure-directory`, `write-file { path, content }`, `replace-in-file { path, search, replace }`, `append-file { path, content }`. No `delete-file` / `rename-file` yet — use Path B + `git rm` for deletes.
- Optional `created_by` (string, parent feature slug) on any op — from v0.6.0 a **live apply-time gate**: `apply --mode execute` rejects ops whose `created_by` parent is missing from `depends_on` (hard-parent miss fails in execute, warns in `--dry-run`). Omit unless the recipe declares feature-DAG provenance.
- `replace-in-file.search` is a **literal string match, not a regex**. Paste the exact text, include surrounding lines for uniqueness.
- `replace-in-file` replaces exactly one occurrence per op. Emit multiple ops to replace several copies.
- All `path` values are repo-relative. `../`, absolute paths, or symlinks outside the repo abort `apply --mode execute` (`EnsureSafeRepoPath`).
- Operations execute in order; later ops may depend on earlier ops.

## Patch vs recipe — mental model

- `artifacts/post-apply.patch` — authoritative git diff. **The patch captures intent.**
- `artifacts/apply-recipe.json` — deterministic script targeting a specific upstream snapshot.

When they disagree (e.g. the recipe's `replace-in-file` can no longer find its anchor because upstream edited the line), trust the patch. Regenerate the recipe afterward.

## If reconcile returns 3WayConflicts

1. **Never pop the stash.** It holds your pre-reconcile tree.
2. Restore only the tpatch metadata so you can see the feature's intent:
   `git checkout stash@{0}^3 -- .tpatch/`
3. Read `.tpatch/features/<slug>/spec.md` (intent), `.tpatch/features/<slug>/artifacts/post-apply.patch` (diff), and the new upstream version of each conflicted file.
4. Hand-author a resolution that preserves **both** intents.
5. `tpatch apply <slug> --mode done && tpatch record <slug>`.

## Feature dependencies (v0.6.0+)

Tessera Patch tracks a dependency DAG between features. Declare parents in `status.json` `depends_on`, or via the CLI:

- `tpatch feature deps <slug>` — print depends_on + dependents.
- `tpatch feature deps <slug> add <parent>[:hard|:soft]` — add an edge (defaults to hard).
- `tpatch feature deps <slug> remove <parent>` — remove an edge (atomic).
- `tpatch amend <slug> --depends-on <parent>[:hard|:soft]` — same, in batch with other edits.
- `tpatch amend <slug> --remove-depends-on <parent>` — same, in batch.
- `tpatch feature deps --validate-all` — global validation (cycles, dangling, kind conflict).
- `tpatch status --dag` (add `--json` for harnesses) — render the DAG tree. Add a slug to scope to one feature's parents + children.

Edge kinds:

- **hard** (default) — `tpatch apply <child>` is blocked until every hard parent reaches state `applied` or `upstream_merged`.
- **soft** — ordering hint only; never gates apply.

Composable reconcile labels overlay on `Reconcile.Outcome`:

- `waiting-on-parent` — at least one hard parent has not yet been applied.
- `blocked-by-parent` — at least one hard parent is in a terminal-failure verdict.
- `stale-parent-applied` — a hard parent was updated after the child's last reconcile.
- Compound: when the child's own outcome is `blocked-requires-human` AND `blocked-by-parent` is set, `EffectiveOutcome` reports `blocked-by-parent-and-needs-resolution` (display-only — programmatic decisions still read `Outcome` and `Labels` separately).

Recipe operations may set `created_by: "<parent-slug>"` to declare DAG provenance. From v0.6.0 this is a **live apply-time gate**: `tpatch apply --mode execute` rejects an operation whose `created_by` parent is missing from `depends_on` (hard fail in execute, downgraded to a warning in `--dry-run` per PRD §4.3).

Removing a feature with downstream dependents requires `--cascade`:

- `tpatch remove <slug>` — refuses if any dependent exists.
- `tpatch remove <slug> --cascade` — TTY confirms, then removes leaves first (reverse-topological order).
- `tpatch remove <slug> --cascade --force` — required for non-TTY use.
- **`--force` alone never bypasses the dep-integrity gate** — it only suppresses the TTY confirm prompt (PRD §3.7, ADR-011 D7).

Toggle the whole feature with `features_dependencies: true|false` in `.tpatch/config.yaml` (default `true` from v0.6.0).

## Reconcile Phase 3.5 — Provider-assisted conflict resolution (v0.5.0)

On 3-way conflict, `tpatch reconcile --resolve` asks the provider to merge each conflicted file inside a **shadow worktree** (`.tpatch/shadow/<slug>-<ts>/`). The real working tree is never touched until you accept.

Flags:
- `--resolve` — enable phase 3.5 (off by default; no heuristic fallback — ADR-010 D9).
- `--apply` — auto-accept when every file is `resolved`. Requires `--resolve`.
- `--max-conflicts N` — abort before calling the provider if conflicts > N (default 10).
- `--model <name>` — override resolver model.
- `--accept <slug>` / `--reject <slug>` / `--shadow-diff <slug>` — terminal ops on a pending shadow session (mutually exclusive; slug is the flag value, not a positional arg).

Verdicts: `shadow-awaiting` (all files resolved; feature state `reconciling-shadow`), `blocked-requires-human` (validation failed or no provider), `blocked-too-many-conflicts` (count > `--max-conflicts`).

Each resolver run writes `.tpatch/features/<slug>/artifacts/resolution-session.json` — per-file status, validation reasons, shadow path. Agents acting as the provider (Path B) can edit the shadow files and then run `tpatch reconcile --accept <slug>`.

On `--accept`, tpatch applies non-conflicting hunks of `post-apply.patch` via 3-way merge (excluding resolved files), copies resolved files from shadow → real tree, regenerates `post-apply.patch`, snapshots the delta as `patches/NNN-reconcile.patch`, and marks the feature `applied`. `apply-recipe.json` is NOT auto-regenerated — re-run `tpatch implement` or `tpatch record` if the recipe matters to you.

Full design: `docs/adrs/ADR-010-provider-conflict-resolver.md`.
