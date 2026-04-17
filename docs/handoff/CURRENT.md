# Current Handoff

## Active Task
- **Task ID**: Native Copilot Auth — research + PRD
- **Milestone**: Planning for M10 (managed proxy UX) + M11 (native PAT provider)
- **Description**: User asked: "plan what would it take to have a 'native' copilot auth as a provider." Researched copilot-api and github/copilot-cli to determine what's officially supported.
- **Status**: PRD written, awaiting supervisor review
- **Assigned**: 2026-04-17

## Session Summary

1. **Committed Phase 2 work** as commit `dc42718` ("Phase 2 (v0.3.0): providers, validation, interactive/harness, distribution"). Includes all M7/M8/M9, refinement, and distribution changes.
2. **Released v0.3.0** — bumped version constant from `0.3.0-dev` to `0.3.0`, committed as `305781d`, tagged `v0.3.0` with a full release note. Tag is local; repo owner still needs to `git push origin main --tags`.
3. **Researched Copilot auth options**:
   - Pulled `tesserabox/copilot-api` README — explicitly "reverse-engineered proxy… not supported by GitHub… may trigger abuse-detection systems."
   - Pulled `github/copilot-cli` README and repo root listing — **not open source** (only README, install.sh, changelog, LICENSE published; the CLI is a closed-source binary on Homebrew/npm/WinGet). Official auth paths: `/login` OAuth or `GH_TOKEN`/`GITHUB_TOKEN` with "Copilot Requests" PAT permission.
   - Conclusion: **GitHub does not publish a public OpenAI-compatible Copilot endpoint.** Every third-party integration (copilot-api, Claude Code via proxy, tpatch) is on reverse-engineered surface.
4. **Wrote PRD** (`docs/prds/PRD-native-copilot-auth.md`) with 5 options evaluated and a two-phase recommendation: M10 managed-proxy UX (`copilot-start` / `copilot-stop` / `copilot-status`), then M11 opt-in native PAT provider calling `api.githubcopilot.com` directly. Shelling out to `copilot` CLI explicitly rejected (burns premium requests, re-runs its own agent loop).

## Files Created
- `docs/prds/PRD-native-copilot-auth.md`

## Files Changed
- `internal/cli/cobra.go` — version `0.3.0-dev` → `0.3.0` (committed)

## Git State
- `dc42718` — Phase 2 feature commit
- `305781d` — "Release v0.3.0" (version bump)
- `v0.3.0` — tag on 305781d
- **Not yet pushed.** Repo owner needs `git push origin main && git push origin v0.3.0`.

## Test Results
- `gofmt -l .` clean
- `go test ./...` — all 7 packages pass
- `tpatch --version` → `tpatch 0.3.0`

## Key Decisions in PRD
- **Reject** shelling out to `copilot` CLI (Option D): not a provider, each prompt burns a premium request, re-runs its own agent loop.
- **Phase 1 (M10)**: auto-manage copilot-api proxy. Requires user to have the proxy installed (we print install instructions, do not silently install). Defaults to rate-limit 30s + wait. Writes `.tpatch/provider-runtime.json` for lifecycle management.
- **Phase 2 (M11) opt-in only**: native PAT provider (`type: copilot-native`) calling `api.githubcopilot.com` directly. Same unsupported endpoint as copilot-api but no Node/Bun dep. Gated by `provider.copilot_native_optin: true` + mandatory AUP warning on first run.
- **Open question** captured: does GitHub's ToS permit a third-party tool to identify as an editor against their Copilot endpoint? Needs answer before M11 lands.

## Blockers
- None for the PRD itself.
- M11 (native provider) is soft-blocked on the "can we ship the editor header set?" legal question noted in the PRD.

## Next Steps
1. **Repo owner**: `git push origin main && git push origin v0.3.0` to publish.
2. **Repo owner**: make repo public (if not already) so CI + `go install` work.
3. **Next agent session**: implement M10 (managed proxy) per the PRD. Estimated scope: `provider copilot-start/stop/status` subcommands + `provider-runtime.json` + `.gitignore` entry + integration test with a mocked `copilot-api` binary.
4. **Before M11**: answer open question 1 in the PRD (editor-headers policy). If permissible, implement `CopilotNativeProvider`.

## Context for Next Agent
- PRD lives at `docs/prds/PRD-native-copilot-auth.md`. It includes the full options matrix and the rejection rationale for each alternative.
- The `Provider` interface is stable and Phase 1 does not need to touch it at all — the managed proxy still routes through the existing `OpenAICompatible` code path. Phase 2 adds a sibling struct.
- `docs/harnesses/copilot.md` already documents the current manual setup; update it when M10 lands.
- GitHub has explicitly warned users in copilot-api's README about abuse-detection. Our UX for M10/M11 must surface that warning prominently.

