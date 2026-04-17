# Current Handoff

## Active Task
- **Task ID**: M11 — Native Copilot provider (ADR-005)
- **Milestone**: M11 delivered
- **Description**: First-party Go provider speaking directly to `api.githubcopilot.com`. Mirrors the copilot-api/litellm pattern: device-code OAuth → session-token exchange → editor headers.
- **Status**: Implemented; awaiting supervisor review.
- **Assigned**: 2026-04-18

## Session Summary

1. **Auth store** (`internal/provider/copilot_auth.go`) — schema
   `{version, oauth, session}`, atomic write at `$XDG_DATA_HOME/tpatch/copilot-auth.json`
   with 0600 perms, rejects symlinks + world/group-writable parent dirs, tightens
   file perms on load, `TPATCH_COPILOT_AUTH_FILE` env override for tests,
   `authStoreMu` serialises writes + refreshes.
2. **Device-code flow** (`internal/provider/copilot_login.go`) — `RequestDeviceCode`,
   `PollAccessToken` (honours `authorization_pending`, permanent `slow_down` bump,
   `expired_token`, `access_denied`, local deadline + ctx cancel, always sends
   `Accept: application/json`), `ExchangeSessionToken` (+ `…Locked` variant used
   by the provider's retry-on-401 path). Client ID `Iv1.b507a08c87ecfe98`
   matches copilot-api.
3. **Editor headers** (`internal/provider/copilot_headers.go`) — version
   constants tracking copilot-api 0.26.7, `x-request-id` uuid, `TODO(adr-005)`
   to refresh when upstream bumps.
4. **Provider impl** (`internal/provider/copilot_native.go`) — `CopilotNative`
   satisfies `Provider`. `Check` never initiates device flow (returns
   `errCopilotUnauthorized` if no auth file). `Generate` proactively refreshes
   the session 60s before expiry, retries once on 401 with a forced refresh,
   then fails. Routes via `auth.Session.Endpoints["api"]` verbatim (D5).
5. **Registry** — `provider.NewFromConfig` dispatches
   `CopilotNativeType = "copilot-native"`. `Config.Configured()` relaxed for
   copilot-native so `Model` alone is enough (`BaseURL` comes from the auth
   file). New `Config.Initiator` field plumbed through `store.ProviderConfig`,
   the YAML parser, `SaveConfig`, and `renderGlobalYAML`.
6. **Opt-in gate** — `store.AcknowledgeCopilotNativeOptIn`,
   `store.CopilotNativeOptedIn`, plus `CopilotNativeOptIn` + `…At` fields
   written to **global config only** (same class as `CopilotAUPAckAt`) so they
   don't leak via repo clones. Enforced in `providerSetCmd`, `config set`
   (`provider.type=copilot-native`), and implicitly in auto-detect (which never
   lists copilot-native as a candidate).
7. **CLI** (`internal/cli/copilot_native.go`) — `provider copilot-login`
   (enterprise prompt, device flow, AUP notice), `provider copilot-logout`
   (deletes auth file). Re-uses AUP language from M10.
8. **Config set** — `config set provider.copilot_native_optin true` routes
   to `SaveGlobalConfig` (rubber-duck #3); `config set provider.initiator`
   validates `""|user|agent`.
9. **Preset** — `--preset copilot-native` in `providerPresets` (empty
   BaseURL, default model `claude-sonnet-4`, empty AuthEnv).
10. **Version bump** — `0.4.0-dev`.
11. **Docs** — new `docs/faq.md` (macOS `~/Library/Application Support`
    caveat + `XDG_CONFIG_HOME` override + auth-file locations); harness
    doc `docs/harnesses/copilot.md` gains "Native path (experimental,
    opt-in)" section; ROADMAP M11 marked ✅.

## Files Created
- `internal/provider/copilot_auth.go`
- `internal/provider/copilot_login.go`
- `internal/provider/copilot_headers.go`
- `internal/provider/copilot_native.go`
- `internal/cli/copilot_native.go`
- `docs/faq.md`

## Files Modified
- `internal/provider/provider.go` — `Config.Initiator`, relaxed `Configured()`
- `internal/provider/anthropic.go` — `NewFromConfig` dispatches copilot-native
- `internal/store/types.go` — `CopilotNativeOptIn` + `…At`, `ProviderConfig.Initiator`, relaxed `ProviderConfig.Configured()`
- `internal/store/store.go` — YAML parse/emit for new fields
- `internal/store/global.go` — global opt-in render + merge + helpers
- `internal/cli/cobra.go` — preset, type flag, opt-in gate, config-set routing, version bump
- `internal/cli/copilot.go` — pipes `Initiator` into `provider.Config`
- `docs/harnesses/copilot.md` — native path section
- `docs/ROADMAP.md` — M11 marked ✅

## Test Results

```
$ go test ./... -count=1
ok  github.com/tesserabox/tesserapatch/assets
ok  github.com/tesserabox/tesserapatch/internal/cli
ok  github.com/tesserabox/tesserapatch/internal/provider
ok  github.com/tesserabox/tesserapatch/internal/safety
ok  github.com/tesserabox/tesserapatch/internal/store
ok  github.com/tesserabox/tesserapatch/internal/workflow
$ go build ./cmd/tpatch
# binary reports 0.4.0-dev
```

## Next Steps
1. Supervisor review per `AGENTS.md` cadence → approve → tag `v0.4.0`
   so the CI release job publishes notes.
2. Live smoke test against a real GitHub account with Copilot entitlement:
   - `tpatch config set provider.copilot_native_optin true`
   - `tpatch provider copilot-login`
   - `tpatch provider set --preset copilot-native`
   - `tpatch provider check`
   - full `tpatch cycle` of a toy feature.
3. Follow-up: add provider-level unit tests with an httptest fake for
   the device flow + session exchange + 401 retry (scaffolded but not
   included in this cut to keep the diff surgical).

## Blockers
None. Editor-header policy is a known unknown per ADR-005 OQ1; we ship
with editor headers until GitHub publishes an official compatibility
endpoint.

## Context for Next Agent
- `CopilotAuthFilePath()` returns `(string, error)` — don't call it as a
  single-value expression.
- `ExchangeSessionToken(ctx, opts, auth)` **mutates `auth` in place** and
  returns only `error`. That's intentional: the provider's retry-on-401
  path needs to refresh the in-memory struct without re-reading the file
  before writing.
- `CopilotSessionBlock.Endpoints["api"]` is the routing root. Treat it as
  opaque — don't parse or reconstruct it.
- `authStoreMu` guards **both** the file and `exchangeSessionTokenLocked`;
  always call `ExchangeSessionToken` (the public wrapper) unless you
  already hold the mutex.
- macOS + `os.UserConfigDir()` resolves to `~/Library/Application Support/tpatch/`.
  Documented in `docs/faq.md`; users who want XDG layout set
  `XDG_CONFIG_HOME`.
