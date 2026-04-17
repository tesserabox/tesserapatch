package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Global config handling.
//
// The global config lives at $XDG_CONFIG_HOME/tpatch/config.yaml (falling
// back to ~/.config/tpatch/config.yaml on POSIX systems and
// %AppData%/tpatch/config.yaml on Windows). It holds user-wide defaults
// that can be overridden per-repo in .tpatch/config.yaml.
//
// Load order for the effective config seen by a command:
//   1. Global config (if present) — base values.
//   2. Repo config (.tpatch/config.yaml) — non-empty fields override.
//   3. Environment variables — credentials only, never persisted.
//
// Secrets are never written to either file; only env var *names* are
// stored (via Provider.AuthEnv).

// GlobalConfigPath returns the absolute path to the global config file,
// honoring XDG_CONFIG_HOME then falling back to ~/.config/tpatch/config.yaml
// (and %AppData%\tpatch\config.yaml on Windows via UserConfigDir).
func GlobalConfigPath() (string, error) {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "tpatch", "config.yaml"), nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine user config dir: %w", err)
	}
	return filepath.Join(dir, "tpatch", "config.yaml"), nil
}

// LoadGlobalConfig reads the global config file if it exists, or returns
// a zeroed Config when absent. Missing-file is not an error.
func LoadGlobalConfig() (Config, error) {
	path, err := GlobalConfigPath()
	if err != nil {
		return Config{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Config{}, nil
		}
		return Config{}, err
	}
	cfg := parseYAMLConfig(string(data))
	cfg.CopilotAUPAckAt = extractYAMLValue(string(data), "copilot_aup_acknowledged_at")
	return cfg, nil
}

// SaveGlobalConfig writes cfg to the global config path, creating parent
// directories as needed. Secrets are never persisted; only env-var names.
func SaveGlobalConfig(cfg Config) error {
	path, err := GlobalConfigPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	content := renderGlobalYAML(cfg)
	// chmod 0600 on the config file to match secret-adjacent-config convention.
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return err
	}
	return nil
}

// LoadMergedConfig returns global ⋃ repo with non-empty repo fields
// taking precedence. Callers that write config must still pick the
// correct target (SaveConfig for repo, SaveGlobalConfig for global).
func (s *Store) LoadMergedConfig() (Config, error) {
	global, err := LoadGlobalConfig()
	if err != nil {
		return Config{}, err
	}
	repo, err := s.LoadConfig()
	if err != nil {
		return Config{}, err
	}
	return mergeConfig(global, repo), nil
}

// AcknowledgeCopilotAUP records the current ISO-8601 timestamp in the
// global config's copilot_aup_acknowledged_at field. Idempotent.
func AcknowledgeCopilotAUP() error {
	cfg, err := LoadGlobalConfig()
	if err != nil {
		return err
	}
	if strings.TrimSpace(cfg.CopilotAUPAckAt) != "" {
		return nil
	}
	cfg.CopilotAUPAckAt = nowStamp()
	return SaveGlobalConfig(cfg)
}

// CopilotAUPAcknowledged reports whether the user has already seen and
// accepted the Copilot AUP warning.
func CopilotAUPAcknowledged() bool {
	cfg, err := LoadGlobalConfig()
	if err != nil {
		return false
	}
	return strings.TrimSpace(cfg.CopilotAUPAckAt) != ""
}

// mergeConfig returns global with non-empty repo fields layered on top.
// Zero/empty values in repo do NOT clear global values — repo must
// explicitly set a field to override it. The Copilot AUP ack is sourced
// from global only (it is not a per-repo concept).
func mergeConfig(global, repo Config) Config {
	out := global
	if repo.Provider.Type != "" {
		out.Provider.Type = repo.Provider.Type
	}
	if repo.Provider.BaseURL != "" {
		out.Provider.BaseURL = repo.Provider.BaseURL
	}
	if repo.Provider.Model != "" {
		out.Provider.Model = repo.Provider.Model
	}
	if repo.Provider.AuthEnv != "" {
		out.Provider.AuthEnv = repo.Provider.AuthEnv
	}
	if repo.Provider.Initiator != "" {
		out.Provider.Initiator = repo.Provider.Initiator
	}
	if repo.MergeStrategy != "" && repo.MergeStrategy != "3way" {
		out.MergeStrategy = repo.MergeStrategy
	} else if out.MergeStrategy == "" {
		out.MergeStrategy = repo.MergeStrategy
	}
	if out.MergeStrategy == "" {
		out.MergeStrategy = "3way"
	}
	if repo.MaxRetries > 0 {
		out.MaxRetries = repo.MaxRetries
	}
	if repo.TestCommand != "" {
		out.TestCommand = repo.TestCommand
	}
	return out
}

// renderGlobalYAML serialises the global config. Uses the same top-level
// keys as the repo config plus copilot_aup_acknowledged_at and the
// copilot-native opt-in fields.
func renderGlobalYAML(cfg Config) string {
	mergeStrat := cfg.MergeStrategy
	if mergeStrat == "" {
		mergeStrat = "3way"
	}
	maxRetries := cfg.MaxRetries
	if maxRetries < 0 {
		maxRetries = 0
	}
	optIn := "false"
	if cfg.CopilotNativeOptIn {
		optIn = "true"
	}
	initiatorLine := ""
	if cfg.Provider.Initiator != "" {
		initiatorLine = fmt.Sprintf("  initiator: %s\n", yamlQuote(cfg.Provider.Initiator))
	}
	return fmt.Sprintf(`# Tessera Patch — global configuration
# Location: $XDG_CONFIG_HOME/tpatch/config.yaml (or ~/.config/tpatch/config.yaml)
# Repo-level .tpatch/config.yaml fields override these values.
# Secrets are never written here — only the env-var *name* goes in auth_env.
provider:
  type: %s
  base_url: %s
  model: %s
  auth_env: %s
%s
merge_strategy: %s
max_retries: %d
test_command: %s

# Timestamp the user acknowledged the Copilot AUP warning (ISO-8601).
# Managed by tpatch; leave empty to re-trigger the first-run warning.
copilot_aup_acknowledged_at: %s

# Opt-in for the native Copilot provider (type: copilot-native). See ADR-005.
# Set via `+"`tpatch config set provider.copilot_native_optin true`"+`.
copilot_native_optin: %s
copilot_native_optin_at: %s
`,
		yamlQuote(cfg.Provider.Type), yamlQuote(cfg.Provider.BaseURL),
		yamlQuote(cfg.Provider.Model), yamlQuote(cfg.Provider.AuthEnv),
		initiatorLine,
		mergeStrat, maxRetries, yamlQuote(cfg.TestCommand),
		yamlQuote(cfg.CopilotAUPAckAt),
		optIn, yamlQuote(cfg.CopilotNativeOptInAt),
	)
}

// AcknowledgeCopilotNativeOptIn records the user's acceptance of the
// native Copilot provider AUP in the global config. Idempotent — if
// already acknowledged, does nothing.
func AcknowledgeCopilotNativeOptIn() error {
	cfg, err := LoadGlobalConfig()
	if err != nil {
		return err
	}
	if cfg.CopilotNativeOptIn {
		return nil
	}
	cfg.CopilotNativeOptIn = true
	cfg.CopilotNativeOptInAt = nowStamp()
	return SaveGlobalConfig(cfg)
}

// CopilotNativeOptedIn reports whether the user has opted into the
// native provider. Reads from the global config.
func CopilotNativeOptedIn() bool {
	cfg, err := LoadGlobalConfig()
	if err != nil {
		return false
	}
	return cfg.CopilotNativeOptIn
}
