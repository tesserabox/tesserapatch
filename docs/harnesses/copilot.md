# Harness Integration — GitHub Copilot CLI

GitHub Copilot CLI (`copilot`) is an agentic terminal client powered by the same harness as GitHub's Copilot coding agent. It ships with an MCP-capable tool layer and a shell-execution tool. Like codex, it is *not* a tpatch provider — it is a harness that calls tpatch to drive the feature lifecycle.

## Prerequisites

- `copilot` installed and authenticated with an active Copilot subscription.
- `tpatch` ≥ `0.3.1`.
- A configured tpatch provider. The local copilot-api proxy strips and
  replaces inbound auth headers with its own session token (see
  `lib/api-config.ts:copilotHeaders` in copilot-api), so you do **not**
  need to set `--auth-env`:
  ```bash
  tpatch provider set --preset copilot
  ```
  If you want to forward `GITHUB_TOKEN` for visibility in the proxy
  logs, you can still pass `--auth-env GITHUB_TOKEN`; it's a no-op on
  the upstream call.
- A `.github/copilot/cli/skills/` entry for tpatch (we ship one via `tpatch init` to `.tpatch/steering/`; copy or symlink the `assets/skills/copilot/` skill file into the repo-level skill directory if you want Copilot CLI to discover it automatically).

## The copilot-api proxy (M10)

The `--preset copilot` flag configures tpatch to talk to `copilot-api`
(https://github.com/ericc-ch/copilot-api) running on `localhost:4141`. tpatch
**does not supervise** the proxy — you start and stop it yourself:

```bash
npm install -g copilot-api
copilot-api start

# or, no install:
npx copilot-api@latest start
```

On the first `tpatch provider set --preset copilot` (or matching auto-detect),
tpatch prints an Acceptable Use Policy warning once. The acknowledgement is
persisted in the global config:

- Linux / `$XDG_CONFIG_HOME` set: `$XDG_CONFIG_HOME/tpatch/config.yaml`
- Linux default: `~/.config/tpatch/config.yaml`
- **macOS default: `~/Library/Application Support/tpatch/config.yaml`** (set
  `XDG_CONFIG_HOME=$HOME/.config` if you prefer the XDG path)
- Windows: `%AppData%\tpatch\config.yaml`

Per-repo values in `.tpatch/config.yaml` override the global values field-by-field;
empty fields fall back to the global config.

If the proxy is not reachable at `localhost:4141`:

- `tpatch init` and `tpatch provider set` warn but continue.
- `tpatch analyze|define|explore|implement|cycle` hard-fail with an install
  pointer before starting the LLM call. This keeps heuristic fallbacks
  explicit rather than silent.

### Smart endpoint routing (M10+)

The copilot-api proxy advertises per-model `supported_endpoints` on
`/v1/models`. tpatch reads that metadata during the reachability probe
and transparently picks the matching wire format:

| Model advertises | Provider used | Wire route |
|------------------|---------------|------------|
| `/v1/messages` (Claude `opus-4.6`, `sonnet-4`, ...) | Anthropic Messages | `POST /v1/messages` |
| `/responses` only (GPT-5.x, `o1` family) | *(not yet supported — see below)* | — |
| `/chat/completions` (GPT-4o, legacy models) | OpenAI Chat Completions | `POST /v1/chat/completions` |

This means `--type anthropic` is no longer required for Claude models on
the proxy — `--preset copilot --model claude-opus-4.6` Just Works. If
you set `TPATCH_NO_PROBE=1` to skip the reachability probe, smart
routing is also skipped and tpatch falls back to whatever wire type the
preset configured.

#### `/responses`-only models

Models that *only* advertise `/responses` (today: `gpt-5.5`, `gpt-5.4`,
`o1-mini`, etc.) currently surface a `ProxyUpstreamAbortedError`. The
copilot-api proxy does route the request to its `/responses` handler,
but the upstream Copilot fetch is aborted before a response arrives.
There is no client-side workaround — pick a model that supports
`/v1/messages` or `/chat/completions` until the upstream proxy fix
ships.

> **Experimental opt-in.** A `ResponsesProvider` is wired into
> `internal/provider/responses.go` behind the
> `TPATCH_ENABLE_RESPONSES_PROVIDER=1` environment variable. When the
> upstream proxy fix lands, set the env var (or remove the gate in
> `PickProvider`) to route `/responses`-only models through it
> directly. The wire format is already correct — see ADR-014.

The proxy is reverse-engineered, not supported by GitHub, and may trigger
abuse-detection if hit too aggressively. See ADR-004 for the UX rationale and
ADR-005 for the plan to ship a first-party `copilot` provider that removes
this dependency.

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

## Native path (experimental, opt-in — see ADR-005)

As of v0.4.0-dev, tpatch can talk directly to `api.githubcopilot.com`
using the same editor-style OAuth flow that VS Code's Copilot
extension uses, without running the `copilot-api` proxy. This is
useful when you want a single binary with no sidecar.

```sh
tpatch config set provider.copilot_native_optin true
tpatch provider copilot-login                  # runs the device-code flow
tpatch provider set --preset copilot-native --model claude-sonnet-4
tpatch provider check
```

Session tokens live for ~25 minutes and are refreshed automatically
before each call. The long-lived OAuth token is written to
`~/.local/share/tpatch/copilot-auth.json` (Linux) or
`~/Library/Application Support/tpatch/copilot-auth.json` (macOS) with
`0600` permissions. Run `tpatch provider copilot-logout` to delete it.

This path is **not endorsed by GitHub**; you are responsible for
compliance with the Acceptable Use Policies. If GitHub ships an
official compatibility endpoint, we will switch to it.

For most users, the managed `copilot-api` proxy (preset: `copilot`) is
still the recommended setup because it has a broader install base and
receives faster upstream patches.
