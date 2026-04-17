# Harness Integration — OpenAI Codex CLI

Codex (`codex exec`) is an agentic CLI that runs a reasoning loop with shell-tool access. Tessera Patch is *not* a provider for codex — it is a sibling tool that codex invokes to drive the analyze → define → explore → implement → apply → record lifecycle. This guide shows how to wire them together.

## Prerequisites

- `codex` ≥ the version that supports `codex exec <prompt>` (non-interactive mode).
- `tpatch` ≥ `0.3.0-dev` with the `cycle`, `test`, `next` commands.
- A configured provider for `tpatch` itself (e.g. `tpatch provider set --preset copilot`).
- `AGENTS.md` at the repo root so codex picks up repo-specific instructions.

## Handshake

```
┌──────────┐  codex exec "..."   ┌──────────────┐
│  Human   │────────────────────▶│   codex      │
└──────────┘                     │   (LLM loop) │
                                 └──────┬───────┘
                                        │ shell tool
                                        ▼
                                 ┌──────────────┐
                                 │   tpatch     │
                                 │ next / apply │
                                 └──────┬───────┘
                                        │ writes
                                        ▼
                                 ┌──────────────┐
                                 │   .tpatch/   │
                                 └──────────────┘
```

1. Codex is the harness. It chooses *when* to run a shell command.
2. Tpatch is the engine. It decides *what* the next action is and produces artifacts.
3. The handshake is `tpatch next <slug> --format harness-json` — codex parses the payload, picks the `on_complete` command, runs it, and re-runs `tpatch next` to see the new state.

## One-time repo setup

Add an `AGENTS.md` block (or create it if absent):

```markdown
## Using tpatch

This repo uses Tessera Patch for feature lifecycle management. To work on a
feature, follow this loop:

1. Run `tpatch next <slug> --format harness-json`.
2. Execute the `on_complete` shell command from the JSON payload.
3. Read the artifacts listed in `context_files` before proposing edits.
4. Re-run step 1 until the payload reports `phase: done`.

Never edit files outside the repo root. Never commit anything inside `.tpatch/`
unless the user asks.
```

Codex reads `AGENTS.md` on every session start, so new tasks inherit this contract automatically.

## Example session

```bash
# 1. Seed a feature.
tpatch add "Support nested enum translation in model router"
# → slug: support-nested-enum-translation-in-model-router

# 2. Delegate the whole cycle to codex.
codex exec "Drive tpatch to completion for the feature
 'support-nested-enum-translation-in-model-router'.
 Use 'tpatch next <slug> --format harness-json' at each step.
 Stop when next reports phase: done."
```

Codex will then iterate:

```json
// After `tpatch next support-nested-enum-translation-in-model-router --format harness-json`
{
  "phase": "analyze",
  "slug": "support-nested-enum-translation-in-model-router",
  "state": "requested",
  "instructions": "Run analysis on the feature request...",
  "context_files": [".tpatch/features/.../request.md"],
  "on_complete": "tpatch analyze support-nested-enum-translation-in-model-router"
}
```

Codex runs the `on_complete` command, then repeats. Each step is visible in the codex transcript and every artifact is under `.tpatch/features/<slug>/`.

## Recommended codex configuration

In `~/.codex/config.toml`:

```toml
[sandbox]
# tpatch writes to .tpatch/ and to tracked source files — both must be writable.
workspace_write = true

[approvals]
# tpatch reports intent before any destructive step. Allow shell commands that
# match the tpatch prefix without prompting.
allow = ["tpatch *"]
```

Adjust to your security posture. At minimum, `tpatch next` and `tpatch status` are safe to allowlist because they never write outside `.tpatch/`.

## What *not* to do

- **Do not** set codex up to call LLMs directly for analyze/define/explore/implement. Those phases are already implemented inside tpatch with retry, validation, and heuristic fallback. Duplicating them in the harness produces two competing prompts and two competing schemas.
- **Do not** run `tpatch apply ... --mode execute` inside codex without first reviewing the `apply-recipe.json`. The recipe is the human-review boundary. Use `--skip-execute` in cycle mode if you want codex to stop there.
- **Do not** embed provider keys in codex config — tpatch already reads them from the env vars named in `.tpatch/config.yaml` (secret-by-reference).

## Troubleshooting

| Symptom | Likely cause | Fix |
|---------|-------------|-----|
| Codex re-reads an old recipe | `tpatch next` told it to re-run `apply prepare` but the feature state is stale | `tpatch status <slug>` and reset via `tpatch implement <slug>` |
| Codex refuses the shell command | Approval policy blocks `tpatch` | Add `tpatch *` to the allow list or run `--ask-for-approval=never` for trusted loops |
| Codex loops forever | `next` keeps returning the same phase because execution failed silently | Inspect `.tpatch/features/<slug>/apply-session.json` and `artifacts/raw-<phase>-response-*.txt` |
