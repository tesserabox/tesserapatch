# FAQ

## Where is my config stored?

tpatch uses two config locations:

- **Global** (user-level): defaults, opt-in flags, Copilot AUP
  acknowledgement.
- **Repo** (`.tpatch/config.yaml`): per-repository overrides.

The repo values win over the global ones, except for a small set of
"global-only" keys (e.g. `provider.copilot_native_optin`) that must stay
user-wide so they don't accidentally follow a clone.

### Global config path by OS

| OS      | Resolved path                                               |
|---------|-------------------------------------------------------------|
| Linux   | `$XDG_CONFIG_HOME/tpatch/config.yaml` → `~/.config/tpatch/` |
| macOS   | `~/Library/Application Support/tpatch/config.yaml`          |
| Windows | `%AppData%\tpatch\config.yaml`                              |

### Why is macOS not `~/.config/`?

Go's standard library (`os.UserConfigDir()`) follows Apple's
conventions, which map user config to
`~/Library/Application Support/`, not `~/.config/`. This is intentional
— macOS apps have always lived there, and Spotlight/Time Machine honour
it.

If you prefer the XDG layout on macOS (e.g. you manage dotfiles across
Linux + macOS and want parity), set:

```sh
export XDG_CONFIG_HOME="$HOME/.config"
```

tpatch honours `XDG_CONFIG_HOME` on every platform. With that set,
macOS too will read/write `~/.config/tpatch/config.yaml`.

## Where is my Copilot auth stored?

The native Copilot provider stores a long-lived OAuth token at:

| OS      | Path                                                                 |
|---------|----------------------------------------------------------------------|
| Linux   | `$XDG_DATA_HOME/tpatch/copilot-auth.json` → `~/.local/share/tpatch/` |
| macOS   | `~/Library/Application Support/tpatch/copilot-auth.json` *†*         |
| Windows | not yet supported — use `copilot-api` proxy instead                  |

*†* macOS currently uses the same prefix because `os.UserConfigDir` and
`os.UserHomeDir`-based XDG fallback both land there. Set
`XDG_DATA_HOME=$HOME/.local/share` to force the Linux-style location.
The env var `TPATCH_COPILOT_AUTH_FILE` overrides the path entirely
(used by tests and unusual deployments).

The file is written with `0600` permissions. Run
`tpatch provider copilot-logout` to delete it.

## How do I opt in to the native Copilot provider?

```
tpatch config set provider.copilot_native_optin true
tpatch provider copilot-login
tpatch provider set --preset copilot-native
```

See [ADR-005](adrs/ADR-005-m11-native-copilot-provider.md) for the
security and policy rationale.

## How do I verify my provider works?

```
tpatch provider check
```

prints the configured endpoint and the list of models the server
reports. For the native provider this also triggers the session
refresh if needed.
