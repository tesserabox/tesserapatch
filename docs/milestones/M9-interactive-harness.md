# M9 — Interactive Mode & Harness Integration

**Status**: ⬜ Not started  
**Depends on**: M8

## Overview

This milestone adds two interaction modes that transform tpatch from a batch CLI into an interactive patching experience:

1. **Plain interactive mode** (`--interactive`): The CLI itself pauses between phases, shows results, and asks the user to confirm/edit before proceeding. No external dependencies — pure stdin/stdout.

2. **Harness-backed interactive mode**: Instead of tpatch driving the interaction, a coding agent harness (Claude Code, GitHub Copilot CLI, OpenCode, Cursor, etc.) drives the workflow using tpatch as a tool. The skill files already teach the harness the methodology — this milestone makes the handoff seamless.

## Phase 1: Plain Interactive Mode

A `tpatch cycle <slug> --interactive` command that runs the full lifecycle with pauses:

```
$ tpatch cycle my-feature --interactive

[1/7] Analyzing feature...
  Summary: Fix model ID translation bug
  Compatibility: compatible
  ▸ Continue to define phase? [Y/n/edit]

[2/7] Defining acceptance criteria...
  Generated 4 criteria in spec.md
  ▸ Review spec.md now? [Y/n]
  (opens $EDITOR or prints to stdout)
  ▸ Accept and continue to explore? [Y/n/edit]

[3/7] Exploring codebase...
  Found 5 relevant files
  ▸ Continue to implement? [Y/n]

[4/7] Generating apply recipe...
  Recipe has 3 operations
  ▸ Dry-run the recipe? [Y/n]
  ▸ Execute the recipe? [Y/n]

[5/7] Running tests...
  (runs project test command from config or PATCHING.md)
  ▸ Mark as passed? [Y/n/failed]

[6/7] Recording changes...
  Captured patch (5636 bytes, 4 files)
  Patch validated: applies cleanly
  ▸ Add operator notes? [text/skip]

[7/7] Complete.
  Feature my-feature is now in state: applied
```

### Tasks

- [ ] M9.1 — Implement `tpatch cycle <slug>` command (non-interactive batch mode first)
- [ ] M9.2 — Add `--interactive` flag with confirmation prompts between phases
- [ ] M9.3 — Add `--editor` flag to open `$EDITOR` for spec.md review
- [ ] M9.4 — Add `test_command` to config.yaml (e.g., `bun test`, `go test ./...`)
- [ ] M9.5 — Implement `tpatch test <slug>` — run the configured test command and record result

## Phase 2: Harness Integration Architecture

The key insight: tpatch already works with coding agent harnesses via the skill files installed by `tpatch init`. The harness reads the skill, calls tpatch commands, and the human reviews in the harness's native UI.

But we can go deeper. Instead of the harness calling `tpatch` commands one by one, tpatch can emit **structured instructions** that a harness can consume:

### Harness Protocol (Proposal)

```bash
# Emit a structured task for the harness to execute
tpatch next <slug> --format harness-json
```

Output:
```json
{
  "phase": "implement",
  "slug": "my-feature",
  "instructions": "Read the spec at .tpatch/features/my-feature/spec.md and implement the changes described. Focus on the files listed in exploration.md.",
  "context_files": [
    ".tpatch/features/my-feature/spec.md",
    ".tpatch/features/my-feature/exploration.md",
    ".tpatch/features/my-feature/artifacts/apply-recipe.json"
  ],
  "on_complete": "tpatch apply my-feature --mode done",
  "on_abort": "tpatch apply my-feature --mode started"
}
```

This allows harnesses to:
1. Read `tpatch next` to know what phase to do
2. Load the context files into their agent context
3. Do the work (the harness is the agent)
4. Call the `on_complete` command when done

### Candidate Harnesses

| Harness | How it would use tpatch | Integration depth |
|---------|------------------------|-------------------|
| **Claude Code** | Already supported via `.claude/skills/` — skill teaches methodology. `tpatch next --format harness-json` would give structured tasks. | Deep — claude code can call CLI tools natively |
| **GitHub Copilot CLI** | Via `.github/skills/` — same skill-based approach. Could use `tpatch next` for structured workflow. | Medium — copilot can run terminal commands |
| **OpenCode** | OpenCode supports custom tools/commands. `tpatch` could register as an OpenCode tool. | Deep — tool registration API |
| **Cursor** | Via `.cursor/rules/` — rules file already installed. Agent mode can call tpatch commands. | Medium — rules + terminal |
| **Windsurf** | Via `.windsurfrules` — same approach. | Medium — rules + terminal |
| **Custom / headless** | `tpatch cycle --no-interactive` for CI/CD. Full batch mode. | Shallow — just CLI |

### Tasks

- [ ] M9.6 — Implement `tpatch next <slug> [--format text|harness-json]` — emit next action
- [ ] M9.7 — Design harness protocol JSON schema
- [ ] M9.8 — Add `harness` field to config.yaml (none, claude-code, copilot, opencode, cursor, windsurf)
- [ ] M9.9 — When harness is set, `tpatch cycle --interactive` delegates to harness-specific prompts
- [ ] M9.10 — Write integration guide: "Using tpatch with Claude Code" (as a doc, not a skill — the skill already exists)
- [ ] M9.11 — Write integration guide: "Using tpatch with OpenCode"

## Acceptance Criteria

- `tpatch cycle <slug>` runs the full lifecycle in batch mode
- `tpatch cycle <slug> --interactive` pauses between phases with confirmation
- `tpatch next <slug>` emits the next action for harness consumption
- `tpatch test <slug>` runs configured test command and records result
- At least one harness integration guide is written and tested

## Design Notes

### Why not make tpatch a harness plugin?

tpatch is intentionally **harness-agnostic**. It's a CLI that works with any harness via skills/rules files. Making it a native plugin for one harness would reduce portability.

The `tpatch next` protocol is the bridge: it tells any harness what to do next in a structured format, without coupling to a specific harness's plugin API.

### Interactive vs Harness-backed

| Aspect | Plain `--interactive` | Harness-backed |
|--------|----------------------|----------------|
| **Who drives** | tpatch CLI (prompts user) | Harness (reads tpatch instructions) |
| **Who implements** | User types code or uses editor | Harness agent writes code |
| **Review** | User reads stdout/editor | User reviews in harness UI |
| **Best for** | Simple changes, manual workflow | Complex changes, AI-assisted |
