# Current Handoff

## Active Task
- **Task ID**: Distribution Setup — go install + CI workflow
- **Milestone**: Follow-up to Phase 2 refinement (operational)
- **Description**: Make `go install github.com/tesserabox/tesserapatch/cmd/tpatch@latest` work and add a free CI workflow.
- **Status**: Complete, awaiting supervisor review
- **Assigned**: 2026-04-17

## Session Summary

Two operational follow-ups:

1. **Module path fixed to match repo** — `go.mod` said `github.com/tesserabox/tpatch` while the GitHub repo is `tesserabox/tesserapatch`. That mismatch blocks `go install`. Renamed the module and all imports to `github.com/tesserabox/tesserapatch` (user-selected option). The binary is still called `tpatch` because Go names installed binaries after the final path segment (`cmd/tpatch`).
2. **CI workflow added** — `.github/workflows/ci.yml` runs on push and PR to `main`. It sets up Go via `go-version-file: go.mod` (so CI tracks local dev), checks formatting with `gofmt`, runs `go vet`, builds, tests, and runs an install smoke test. Matrix on `ubuntu-latest` + `macos-latest`. Concurrency group cancels superseded runs to save minutes. Free for public repos.
3. **README install block updated** — now points to the correct module path.

## Files Changed
- `go.mod` — `module github.com/tesserabox/tesserapatch`.
- All `.go` files under `cmd/`, `internal/`, `assets/` — import paths rewritten.
- `.github/workflows/ci.yml` — new CI workflow.
- `README.md` — install instructions updated.

## Test Results
- `gofmt -l .` — clean
- `go test ./... -count=1` — **ALL PASS** across 7 packages
- `go build -o tpatch ./cmd/tpatch` — OK
- `./tpatch --version` → `tpatch 0.3.0-dev`

## Post-Merge Checklist (for the repo owner)
1. Make the repo public (required for `go install` without auth and for free unlimited Actions minutes).
2. Push to `main`; CI should pass on both ubuntu + macOS.
3. Tag a release: `git tag v0.3.0 && git push origin v0.3.0`. `go install ...@latest` will then resolve to that tag.
4. Verify from a clean machine: `go install github.com/tesserabox/tesserapatch/cmd/tpatch@latest`.

## Provider Preset Clarification
`tpatch provider set --preset copilot` targets `http://localhost:4141` with `auth_env: GITHUB_TOKEN`. That is the **copilot-api proxy** endpoint, not the Copilot CLI auth itself. To use the same Copilot subscription as `copilot-cli`:

- Install and run `copilot-api` locally (it does the GitHub OAuth and exposes an OpenAI-compatible endpoint on 4141).
- Then `tpatch provider set --preset copilot` just works.

There is no direct-to-GitHub-Copilot path today because GitHub has not published a public OpenAI-compatible Copilot endpoint. If that changes, we add another preset.

## Blockers
None.

## Next Steps
1. Push + make repo public + tag v0.3.0.
2. Confirm CI green on first main push.
3. Optional: add a `release.yml` workflow with goreleaser for prebuilt binaries (not required for `go install`).

