# Harness Integration — GitHub Copilot CLI

GitHub Copilot CLI (`copilot`) is an agentic terminal client powered by the same harness as GitHub's Copilot coding agent. It ships with an MCP-capable tool layer and a shell-execution tool. Like codex, it is *not* a tpatch provider — it is a harness that calls tpatch to drive the feature lifecycle.

## Prerequisites

- `copilot` installed and authenticated with an active Copilot subscription.
- `tpatch` ≥ `0.3.0-dev`.
- A configured tpatch provider. If you use copilot-api for both copilot-cli and tpatch, the same token works for both:
  ```bash
  tpatch provider set --preset copilot --auth-env GITHUB_TOKEN
  ```
- A `.github/copilot/cli/skills/` entry for tpatch (we ship one via `tpatch init` to `.tpatch/steering/`; copy or symlink the `assets/skills/copilot/` skill file into the repo-level skill directory if you want Copilot CLI to discover it automatically).

## Handshake

Copilot CLI follows the same `tpatch next --format harness-json` protocol as every other harness. The difference is that copilot already ships with MCP and skill-file discovery, so the contract is declared once in a skill and re-used across sessions.

```
┌──────────┐      ┌─────────────────┐     ┌────────────────┐
│  Human   │──────▶│ copilot (agent) │────▶│ tpatch (tool)  │
└──────────┘      └─────────────────┘     └────────────────┘
```

## One-time repo setup

Run `tpatch init` inside the repo. This drops:

- `.tpatch/steering/copilot/tessera-patch.md` — repo-scoped skill
- `.tpatch/steering/copilot/prompts/tessera-patch-apply.prompt.md` — prompt template

Copy the skill to Copilot CLI's expected location:

```bash
mkdir -p .github/copilot/cli/skills/tessera-patch
cp .tpatch/steering/copilot/tessera-patch.md .github/copilot/cli/skills/tessera-patch/SKILL.md
```

The skill teaches Copilot CLI:

1. The 15 tpatch commands and their ordering.
2. The `tpatch next --format harness-json` protocol for deciding the next action.
3. The invariant that `.tpatch/` artifacts are the single source of truth for feature state.

## Example session

```bash
copilot "Drive tpatch to completion for feature 'fix-model-id-translation'.
 Use tpatch next --format harness-json between steps and honor the on_complete
 field. Stop once phase is done."
```

Copilot will:

1. Discover the tpatch skill from `.github/copilot/cli/skills/tessera-patch/SKILL.md`.
2. Call `tpatch next fix-model-id-translation --format harness-json`.
3. Read the JSON payload and execute the `on_complete` command via its shell tool.
4. Loop until the payload returns `phase: "done"`.

Every step is visible in the Copilot CLI transcript. Artifacts persist under `.tpatch/features/<slug>/` regardless of whether the session is resumed in a new terminal.

## MCP option (advanced)

Copilot CLI supports custom MCP servers. A future tpatch release may ship an MCP frontend (`tpatch mcp serve`) that exposes the same state machine as structured tool calls. Until then, the shell-via-JSON contract is the supported integration path. Track this under M10.

## Recommended configuration

In your repo or `~/.config/copilot/settings.json`:

```json
{
  "tools": {
    "shell": {
      "allowList": ["tpatch *", "git status", "git diff"]
    }
  }
}
```

Allow-listing `tpatch *` keeps the loop non-interactive while still blocking arbitrary shell.

## What *not* to do

- **Do not** ask Copilot CLI to re-implement the analyze/define/explore/implement phases inside its own agent loop. They are already implemented in `tpatch` with validator-backed retry. The harness should call, not replicate.
- **Do not** commit `.tpatch/` contents unless your team has agreed to share feature histories. Keep the folder untracked by default; `tpatch init` writes a `.gitignore` entry for generated artifacts.
- **Do not** point Copilot CLI at a different provider than tpatch for the workflow phases. Drift between the two produces confusing, inconsistent plans.

## Troubleshooting

| Symptom | Likely cause | Fix |
|---------|-------------|-----|
| Copilot can't find the skill | Skill file not copied to `.github/copilot/cli/skills/` | Re-run the `cp` step from "One-time repo setup" |
| `tpatch next` returns `phase: analyze` on every call | Analyze step failing silently | Inspect `.tpatch/features/<slug>/artifacts/raw-analyze-response-*.txt` |
| Shell tool prompts for approval every call | Allow list not set | Add `tpatch *` to the allow list as shown above |
| Provider auth errors | Same env var shared by copilot-cli and tpatch got rotated | `tpatch provider check` to verify, re-export the new token |
