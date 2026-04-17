# PRD — Native GitHub Copilot Auth as a tpatch Provider

**Status**: Draft
**Date**: 2026-04-17
**Owner**: tpatch core
**Target milestones**: M10 (managed proxy UX), M11 (direct PAT provider — opt-in)

## 1. Problem

Today, `tpatch provider set --preset copilot` points at `http://localhost:4141`, which is the [`copilot-api`](https://github.com/tesserabox/copilot-api) proxy — a reverse-engineered translator that exposes GitHub Copilot through an OpenAI-compatible (and now Anthropic-compatible) surface. Users must:

1. Install the proxy themselves (`bun install`, `npm i -g copilot-api`, or Docker).
2. Run it as a background process before tpatch works.
3. Manage OAuth separately (either interactively through the proxy or by supplying `GH_TOKEN`).

Users ask: *"Can I just use the same GitHub account I use with `copilot` CLI?"* The answer today is "yes, but through the proxy." This PRD evaluates what a more native integration would cost, benefit, and risk.

## 2. Research — What is officially supported?

### 2.1 copilot-api (the current dependency)

Directly from the `tesserabox/copilot-api` README:

> **This is a reverse-engineered proxy of GitHub Copilot API. It is not supported by GitHub, and may break unexpectedly. Use at your own risk.**
>
> GitHub Security Notice: Excessive automated or scripted use of Copilot […] may trigger GitHub's abuse-detection systems. You may receive a warning from GitHub Security, and further anomalous activity could result in temporary suspension of your Copilot access.

So copilot-api is **unsupported** and places the user at some risk of account action. It is, however, the most mature working option today.

### 2.2 github/copilot-cli (the official client)

The official Copilot CLI repository (`github/copilot-cli`) contains only `README.md`, `install.sh`, `changelog.md`, and `LICENSE.md`. **No source code is published** — the CLI is distributed as a closed-source binary via Homebrew, npm (`@github/copilot`), WinGet, or the install script. We cannot read its transport logic.

The README documents the official auth options:

1. **Interactive OAuth** via the `/login` slash command.
2. **Personal Access Token** with the `"Copilot Requests"` permission, supplied through `GH_TOKEN` or `GITHUB_TOKEN` env vars.

Neither option exposes a documented OpenAI-compatible endpoint that a third-party Go process can call. The CLI is an interactive TUI; it is not a protocol.

### 2.3 Conclusion

There is, **as of 2026-04**, no officially documented HTTP endpoint that a tool like tpatch can call to reach GitHub Copilot. Every "native Copilot" path for a non-GitHub tool is either:

- A reverse-engineered endpoint (copilot-api's approach), or
- Shelling out to the `copilot` binary (not a provider, an agent).

## 3. Options Evaluated

| Option | What it is | Officially supported? | Implementation cost | Operational risk | UX |
|--------|-----------|-----------------------|---------------------|------------------|----|
| **A. Status quo** | User runs copilot-api, tpatch connects to localhost:4141 | No | Zero (shipped) | Medium — abuse detection applies | Requires external setup |
| **B. Managed proxy** | tpatch auto-installs & manages copilot-api lifecycle | No (same as A) | Medium | Same as A | One-command |
| **C. Native PAT provider** | tpatch calls `api.githubcopilot.com` directly with a PAT | No (endpoint undocumented) | High (reimplement proxy in Go) | Same as A but without Node dep | One-command, single binary |
| **D. Shell out to `copilot` CLI** | tpatch spawns `copilot -p <prompt>` per phase | Official | Low | Low — sanctioned, but quota-hungry | Burns premium requests; structured output fragile; copilot re-runs its own agent loop |
| **E. MCP-based** | If Copilot CLI publishes an MCP server mode, tpatch is a client | Speculative | Depends | Low once available | Clean |

## 4. Recommendation

**Two-phase rollout, gated by milestones.**

### Phase 1 — M10: Managed Proxy UX (Option B)

Make the copilot-api path feel native without pretending it is.

- Ship `tpatch provider copilot-start` that:
  1. Checks whether `copilot-api` is runnable (`copilot-api --version` or `npx copilot-api@latest --version`).
  2. If missing, prints install instructions and exits with a helpful error (do **not** silently install — the user must consent to pulling an unsupported proxy).
  3. If present, spawns it as a background process with `--port 4141 --rate-limit 30 --wait` (default rate-limit friendly to GitHub ToS).
  4. Writes the PID to `.tpatch/provider-runtime.json` so `tpatch provider copilot-stop` can cleanly shut it down.
  5. Runs `tpatch provider set --preset copilot` to wire the config.
- Add a **visible warning** on every `tpatch provider copilot-start` run linking to GitHub's Acceptable Use Policy and the copilot-api abuse-detection notice.
- Document in `docs/harnesses/copilot.md` that this is the supported "easy" path.

**Why this first**: it keeps tpatch honest (we don't own the endpoint), reduces setup friction by ~80%, and does not require us to reimplement anything GitHub might change.

### Phase 2 — M11: Native PAT provider (Option C) — feature-flagged, opt-in

For users who don't want a Node proxy running:

- Add `internal/provider/copilot_native.go` implementing the `Provider` interface against `api.githubcopilot.com` directly.
- Config: `type: copilot-native`, `auth_env: GITHUB_TOKEN`, PAT scope "Copilot Requests".
- Headers replicated from copilot-api's known-good set (Editor-Version, Editor-Plugin-Version, Copilot-Integration-Id, OpenAI-Organization, etc.).
- **Opt-in gate**: must be enabled via `tpatch config set provider.copilot_native_optin true` *and* requires a non-empty warning acknowledgement. First-run prints the full AUP quote.
- Document as experimental; may break; user assumes risk.

**Why opt-in**: we replicate the same unsupported surface as copilot-api. The only material gain is "no Node/Bun dependency." That is not enough to justify on-by-default.

### Explicitly **not** doing

- **Option D (shell out to `copilot` CLI)**: rejected. Each tpatch phase (analyze/define/explore/implement) would burn a premium request; `copilot` re-enters its own agentic loop which will diverge from our prompt schemas; output parsing on top of a TUI-oriented binary is brittle.
- **Option E (MCP)**: will be reconsidered if/when Copilot CLI or the coding agent publishes an MCP server endpoint. Tracked as a watch item, not work.

## 5. Scope details

### 5.1 New CLI surface

```
tpatch provider copilot-start [--port 4141] [--rate-limit 30]
tpatch provider copilot-stop
tpatch provider copilot-status
```

All three operate on the proxy lifecycle recorded in `.tpatch/provider-runtime.json`. They fail gracefully (exit code, clear message) when:
- `copilot-api` is not installed.
- The managed PID is no longer running.
- Port 4141 is occupied by something else.

### 5.2 `provider-runtime.json` schema

```json
{
  "managed": "copilot-api",
  "pid": 12345,
  "port": 4141,
  "started_at": "2026-04-17T06:55:00Z",
  "started_by": "tpatch/0.3.0",
  "flags": ["--rate-limit", "30", "--wait"]
}
```

Written into `.tpatch/` so it moves with the project. Not tracked in git (added to `.gitignore` by `tpatch init`).

### 5.3 `Provider` interface stability

Unchanged. Both the managed proxy path (Phase 1) and the native PAT path (Phase 2) use the existing `Provider` interface — Phase 1 keeps using `OpenAICompatible`, Phase 2 adds a sibling struct. `NewFromConfig` dispatches on `cfg.Type`.

## 6. Risks & Mitigations

| Risk | Mitigation |
|------|-----------|
| GitHub policy action against users driving Copilot from tpatch | Default rate-limit of 30s on managed proxy; prominent AUP warning on every start; require explicit opt-in for Phase 2 native provider |
| copilot-api breaks because GitHub changes its internal API | Pin a tested copilot-api version in the docs; add a `tpatch provider check` diagnostic that detects transport failures and tells the user to update |
| User installs the wrong copilot-api (upstream vs fork) | `copilot-start` checks `copilot-api --version` and warns if not the Tesserabox fork |
| Phase 2 native PAT provider drifts from copilot-api's header set | Mirror copilot-api's known-good headers in a single constants file with links to the upstream source; ship integration tests against `httptest` fixtures |
| Abuse-detection flag on the user's Copilot account | Mandatory AUP acknowledgement on first run; no concurrent in-flight requests (workflow phases are serial anyway) |

## 7. Success Criteria

Phase 1 ships when:

- `tpatch provider copilot-start` goes from "nothing configured" to "working analyze/define/explore/implement" in one command on a machine with copilot-api already installed.
- A user without copilot-api gets a clear, actionable install instruction (not a silent failure).
- `tpatch provider copilot-stop` reliably terminates the managed process.
- Integration test covers the happy path with a mocked `copilot-api` binary on PATH.

Phase 2 ships when:

- `tpatch provider check` against `copilot-native` type returns a non-empty model list via GitHub endpoint directly (no Node proxy running).
- The full analyze → define → explore → implement cycle works against the native provider.
- Explicit opt-in flag must be set; first run displays the AUP warning block.

## 8. Open Questions

1. **Can we legally ship a header set that identifies tpatch as an "editor" to GitHub's Copilot endpoint?** Needs a quick legal/policy check. If "no", Phase 2 is blocked and we stay on Phase 1 indefinitely. (ADR-004 when answered.)
2. **Does GitHub plan to publish an official OpenAI-compatible Copilot endpoint?** If yes, Phase 2 gets dropped in favor of the official surface. Worth asking on GitHub's Copilot Discussions before building.
3. **Should `tpatch provider copilot-start` also offer Docker-backed invocation?** A Docker path is lower-friction on Linux servers but heavier on macOS. Probably out of scope for M10; revisit if requested.

## 9. Out of Scope

- Embedding copilot-api as a Go port (rewriting a Node proxy in Go is a separate project — option C's *internal implementation*, not a separate product).
- Token refresh UX beyond what copilot-api already provides.
- Streaming responses (tpatch workflow is request/response; not a TUI).
