# Agent as Provider — Path B Workflow

> Companion to the shipped skill files. Read this once, then rely on
> the per-format skills for day-to-day invocation.

Tessera Patch has two equally-supported execution paths for every LLM
phase (`analyze`, `define`, `explore`, `implement`):

- **Path A — CLI-driven.** The configured provider generates the
  artifact: `tpatch <phase> <slug>`.
- **Path B — Agent-authored.** You (the agent) author the artifact by
  hand under `.tpatch/features/<slug>/`, then advance state with
  `tpatch <phase> <slug> --manual`.

Path B is **normal, not exceptional**. Prefer it whenever:

1. No provider is configured in this repository or in global config.
2. The configured provider returns empty / truncated / insufficient
   output. In practice this happens most often with `implement` — the
   LLM returns a 1-operation stub, an `ensure-directory`-only recipe,
   or truncated JSON.
3. You have strictly more context than the provider (you are mid-task,
   have already read the relevant files, and know what needs to
   change).
4. The provider output would disagree with intent you have already
   captured in `spec.md` / `exploration.md`.

Do not wait for a "better" recipe. Author the artifact and move on.

## The phase → artifact → state contract

The `--manual` flag validates this table. If the expected artifact is
missing at the canonical path (or, for `implement`, is not valid JSON),
the command refuses with a diagnostic pointing at the exact file; state
does **not** advance.

| phase       | artifact                                    | advances state to |
|-------------|---------------------------------------------|-------------------|
| `analyze`   | `.tpatch/features/<slug>/analysis.md`       | `analyzed`        |
| `define`    | `.tpatch/features/<slug>/spec.md`           | `defined`         |
| `explore`   | `.tpatch/features/<slug>/exploration.md`    | `defined`         |
| `implement` | `.tpatch/features/<slug>/artifacts/apply-recipe.json` | `implementing` |

On success, `status.json.notes` records:

```
Phase advanced manually (--manual); artifact authored at <path>
```

so the audit trail distinguishes Path B transitions from provider
output.

## apply-recipe.json schema (authoritative)

The recipe is the deterministic script that `tpatch apply --mode execute`
replays against the current upstream snapshot.

```json
{
  "version": 1,
  "operations": [
    { "type": "ensure-directory", "path": "src/feature/" },
    { "type": "write-file",
      "path": "src/feature/index.ts",
      "content": "export const x = 1;\n" },
    { "type": "replace-in-file",
      "path": "src/registry.ts",
      "search": "export * from \"./legacy\";\n",
      "replace": "export * from \"./legacy\";\nexport * from \"./feature\";\n" },
    { "type": "append-file",
      "path": "src/changelog.md",
      "content": "\n- added feature/\n" }
  ]
}
```

Operation semantics:

- `ensure-directory` — `mkdir -p`. Idempotent.
- `write-file` — creates or overwrites the whole file. Use for new
  files or full rewrites.
- `replace-in-file` — locates the first occurrence of `search` and
  substitutes `replace`. Errors if `search` is absent.
  - `search` is a **literal string match, not a regex**. Paste the
    exact text, including leading/trailing whitespace. Escape quotes
    and backslashes per JSON rules, not regex rules.
  - Include surrounding lines for uniqueness — one-line anchors
    collide.
  - Exactly one occurrence is replaced per op. To replace several
    copies, emit several ops; each targets the next occurrence as the
    prior op has already rewritten the file.
- `append-file` — appends `content` to an existing file. Errors if
  the file is missing.

There is no `delete-file` or `rename-file` op in the current schema.
To delete or rename a file, use Path B: `apply --mode started`,
`git rm <path>` (or `git mv`), `apply --mode done`, `record`. Richer
op support is tracked in `feat-recipe-schema-expansion`.

Path safety:

- All `path` values are repo-relative.
- `../` traversal, absolute paths, and symlinks that escape the repo
  abort `apply --mode execute` via `EnsureSafeRepoPath`. This is
  enforced per-operation, not just on the recipe as a whole.

Ordering:

- Operations execute sequentially. Later operations can assume earlier
  operations succeeded (e.g. `ensure-directory` before `write-file`
  into that directory, or `write-file` before `replace-in-file` on the
  same file).

## Patch vs recipe — mental model

`.tpatch/features/<slug>/artifacts/` contains two representations of
the same change:

| file | role |
|------|------|
| `post-apply.patch` | authoritative `git diff`. **The patch captures intent.** |
| `apply-recipe.json` | deterministic script targeting a specific upstream snapshot. |

When they disagree — e.g. a `replace-in-file` anchor is no longer
present because upstream edited the surrounding lines — **trust the
patch**, regenerate the recipe. The patch is what `tpatch reconcile`
evaluates against new upstream, what `tpatch record` writes on every
capture, and what survives a feature being rebuilt from scratch.

The recipe is a performance optimisation: it lets `apply --mode
execute` run without invoking a provider on a clean snapshot. It is
not the source of truth.

## The 3WayConflicts playbook

When `tpatch reconcile` returns `3WayConflicts` for a feature, the
phase-4 three-way merge detected textual conflicts between your
feature's diff and the new upstream. The stash created by reconcile
holds your **pre-reconcile** tree (index, working tree, and — crucially
— the `.tpatch/` metadata). Do not pop it.

1. **Never pop the stash.** If you `git stash pop`, you roll the
   repository back to pre-reconcile state and lose the new upstream
   you were trying to reconcile against.
2. Restore only the tpatch metadata so you can see the feature's
   recorded intent without disturbing the working tree:

   ```
   git checkout stash@{0}^3 -- .tpatch/
   ```

   The `^3` parent of a stash commit is the staged-files tree; stash
   created by tpatch includes `.tpatch/` there specifically for this
   flow.
3. Read the feature's intent:
   - `.tpatch/features/<slug>/spec.md` — what the feature is supposed
     to do.
   - `.tpatch/features/<slug>/exploration.md` — which files and
     symbols are load-bearing.
   - `.tpatch/features/<slug>/artifacts/post-apply.patch` — the exact
     diff applied last time.
4. Read the **new** upstream version of each conflicted file (they
   are in the working tree now).
5. Hand-author a resolution that preserves **both** intents — yours
   and the upstream change that caused the conflict. Do not blindly
   prefer one side; reconcile exists to merge.
6. Finish the feature via Path B:

   ```
   tpatch apply <slug> --mode done
   tpatch record <slug>
   ```

   This captures a fresh `post-apply.patch` against the new upstream
   and writes a new `apply-recipe.json`.

### Worked example

Given conflict on `src/registry.ts`:

- Feature `add-telemetry` added `export * from "./telemetry";` after
  `export * from "./legacy";`.
- Upstream renamed `./legacy` to `./deprecated`.

Resolution: edit the new upstream file to add
`export * from "./telemetry";` after the renamed
`export * from "./deprecated";` line. Both intents preserved.

## Provider-assisted automation (v0.5.0 — shipped)

ADR-010 (`docs/adrs/ADR-010-provider-conflict-resolver.md`) locks the
design; v0.5.0 ships it as **Phase 3.5** of `tpatch reconcile`.

### Flow

```
tpatch reconcile --resolve            # provider merges each conflicted file
                                      # into a shadow worktree at
                                      # .tpatch/shadow/<slug>-<ts>/
                                      # real tree is never touched

tpatch reconcile --shadow-diff <slug> # preview resolver output (read-only)
tpatch reconcile --accept <slug>      # apply non-conflicting hunks + copy
                                      # shadow files onto real tree,
                                      # regenerate post-apply.patch,
                                      # snapshot patches/NNN-reconcile.patch
tpatch reconcile --reject <slug>      # discard shadow, no tree changes
```

The four terminal flags (`--accept` / `--reject` / `--shadow-diff`, plus
`--apply` for auto-accept when every file is `resolved`) take a slug as
the flag value, not a positional arg. They are mutually exclusive.

### Verdicts

| verdict | meaning |
|---|---|
| `shadow-awaiting` | All conflicted files resolved; shadow ready for `--accept`. Feature state: `reconciling-shadow`. |
| `blocked-requires-human` | At least one file failed validation (still contains `<<<<<<<`, corrupted content, or no provider configured). ADR-010 D9: there is no heuristic fallback. |
| `blocked-too-many-conflicts` | Conflict count exceeded `--max-conflicts` (default 10); provider was never called. |

### reconcile-session.json

Each resolver attempt writes
`.tpatch/features/<slug>/reconciliation/reconcile-session.json`:

```json
{
  "slug": "<slug>",
  "timestamp": "2026-04-22T12:00:00Z",
  "upstream_commit": "<sha>",
  "shadow_path": ".tpatch/shadow/<slug>-<ts>/",
  "files": [
    { "path": "src/a.ts", "status": "resolved", "model": "gpt-4o-mini", "validation": "ok" },
    { "path": "src/b.ts", "status": "failed",   "model": "gpt-4o-mini", "validation": "markers" }
  ],
  "verdict": "shadow-awaiting"
}
```

This file is the source of truth for agents acting as the provider in
Path B: read it, edit the shadow files directly (they live under
`.tpatch/shadow/<slug>-<ts>/`), then run `tpatch reconcile --accept
<slug>` to commit the resolution without re-invoking the LLM.

### Accept flow (authoritative)

`--accept` is not a simple copy. It does, atomically:

1. Reads the original `artifacts/post-apply.patch` and extracts the set
   of files touched by the feature.
2. Applies non-conflicting hunks to the real tree with
   `git apply --3way --exclude=<resolved-paths>` so unchanged upstream
   regions land correctly.
3. Copies the resolved files from the shadow worktree over the real
   tree (overlay — these are the files that had conflicts).
4. Regenerates `artifacts/post-apply.patch` as
   `git diff <upstreamCommit> -- <touched files>` (new files are
   intent-to-added first so they appear in the diff).
5. Snapshots the resolution delta into
   `patches/NNN-reconcile.patch` for audit.
6. Marks feature state `applied` and prunes the shadow worktree.

`apply-recipe.json` is NOT auto-regenerated — whole-diff to op-list is
lossy. Re-run `tpatch implement` or `tpatch record` if the recipe
matters to you.

### Path B fallback

If the provider is unavailable or returns `blocked-requires-human`, you
can still use the shadow as a scratch space: edit
`.tpatch/shadow/<slug>-<ts>/<path>` directly, update the file's status
in `reconcile-session.json` to `"resolved"`, then `tpatch reconcile
--accept <slug>`. The accept flow trusts the shadow contents; it does
not re-validate that the provider wrote them.

The manual git-stash playbook above is still fully supported and
remains the fastest path when `--resolve` is not wanted or the
conflict count exceeds `--max-conflicts`.
