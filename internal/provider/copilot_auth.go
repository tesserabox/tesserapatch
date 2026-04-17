package provider

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// copilotAuthFileVersion is the on-disk format version. Bump + migrate
// when the schema changes.
const copilotAuthFileVersion = 1

// CopilotAuth is the on-disk schema for Copilot auth state. See
// ADR-005 D2. Stored at $XDG_DATA_HOME/tpatch/copilot-auth.json with
// 0600 perms on the file and 0700 on the parent dir.
type CopilotAuth struct {
	Version int                 `json:"version"`
	OAuth   CopilotOAuthBlock   `json:"oauth"`
	Session CopilotSessionBlock `json:"session"`
}

// CopilotOAuthBlock holds the long-lived GitHub OAuth token obtained
// from the device-code flow. EnterpriseURL is the full GHE host
// (e.g. "company.ghe.com") when applicable; empty for github.com.
type CopilotOAuthBlock struct {
	AccessToken   string `json:"access_token"`
	ObtainedAt    string `json:"obtained_at"`
	EnterpriseURL string `json:"enterprise_url,omitempty"`
}

// CopilotSessionBlock holds the short-lived (~25 min) Copilot session
// token exchanged via api.github.com/copilot_internal/v2/token. The
// Endpoints map is copied verbatim from the exchange response so
// routing stays faithful to whatever GitHub says (especially for
// enterprise deployments where api.<account-type>.githubcopilot.com
// may differ).
type CopilotSessionBlock struct {
	Token       string            `json:"token,omitempty"`
	ExpiresAt   string            `json:"expires_at,omitempty"`
	Endpoints   map[string]string `json:"endpoints,omitempty"`
	RefreshedAt string            `json:"refreshed_at,omitempty"`
}

// authStoreMu serialises on-disk auth-file writes and session refreshes
// so concurrent workflow phases cannot race the session-token exchange.
// Process-scoped; acceptable for tpatch's one-shot CLI model. If a
// long-running daemon mode lands later, switch to flock on the file.
var authStoreMu sync.Mutex

// CopilotAuthFilePath returns the absolute path to the auth JSON file.
// Test callers can override via TPATCH_COPILOT_AUTH_FILE. Production
// callers use $XDG_DATA_HOME or ~/.local/share as the base dir.
func CopilotAuthFilePath() (string, error) {
	if p := os.Getenv("TPATCH_COPILOT_AUTH_FILE"); p != "" {
		return p, nil
	}
	base := os.Getenv("XDG_DATA_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("cannot determine home dir: %w", err)
		}
		base = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(base, "tpatch", "copilot-auth.json"), nil
}

// LoadCopilotAuth reads and validates the on-disk auth state. Returns
// os.ErrNotExist (wrapped) when the user has never run copilot-login.
// Performs safety checks per rubber-duck review D4:
//   - rejects symlinked auth files (prevents attacker-placed symlinks)
//   - fails when parent dir is world/group-writable
//   - tightens 0600 on the file if it was wider
func LoadCopilotAuth() (*CopilotAuth, error) {
	path, err := CopilotAuthFilePath()
	if err != nil {
		return nil, err
	}

	// Reject symlinks — an attacker-controlled symlink can redirect
	// reads/writes to an arbitrary path the process can access.
	if lstat, err := os.Lstat(path); err == nil {
		if lstat.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("auth file %s is a symlink; refusing to open", path)
		}
	}

	// Parent dir must not be world/group-writable.
	if err := checkParentDirPerms(filepath.Dir(path)); err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// Tighten perms in place if they were wider than 0600.
	if info, err := os.Stat(path); err == nil {
		if mode := info.Mode().Perm(); mode&0o177 != 0 {
			_ = os.Chmod(path, 0o600)
		}
	}

	var auth CopilotAuth
	if err := json.Unmarshal(data, &auth); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if auth.Version != copilotAuthFileVersion {
		return nil, fmt.Errorf("unsupported auth file version %d (expected %d) at %s",
			auth.Version, copilotAuthFileVersion, path)
	}
	return &auth, nil
}

// SaveCopilotAuth writes the auth state atomically (temp file + rename)
// so partial writes and concurrent refreshes cannot leave a corrupt
// file. Sets 0600 perms on the payload and ensures the parent dir is
// 0700.
func SaveCopilotAuth(auth *CopilotAuth) error {
	authStoreMu.Lock()
	defer authStoreMu.Unlock()

	path, err := CopilotAuthFilePath()
	if err != nil {
		return err
	}
	if auth.Version == 0 {
		auth.Version = copilotAuthFileVersion
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	// Tighten dir perms if MkdirAll returned early because the dir
	// existed with wider perms.
	_ = os.Chmod(dir, 0o700)

	// Refuse to write through a symlink — same threat model as read.
	if lstat, err := os.Lstat(path); err == nil {
		if lstat.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("auth file %s is a symlink; refusing to write", path)
		}
	}

	payload, err := json.MarshalIndent(auth, "", "  ")
	if err != nil {
		return err
	}

	tmp, err := os.CreateTemp(dir, "copilot-auth-*.json.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if err := os.Chmod(tmpName, 0o600); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if _, err := tmp.Write(payload); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return err
	}
	return nil
}

// DeleteCopilotAuth removes the auth file. Idempotent — no error when
// the file doesn't exist. Called by `tpatch provider copilot-logout`.
func DeleteCopilotAuth() error {
	authStoreMu.Lock()
	defer authStoreMu.Unlock()
	path, err := CopilotAuthFilePath()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

// SessionExpired reports whether the cached session token is missing,
// malformed, or within the refresh window (60s before expiry).
func (a *CopilotAuth) SessionExpired(refreshBuffer time.Duration) bool {
	if a == nil || a.Session.Token == "" || a.Session.ExpiresAt == "" {
		return true
	}
	exp, err := time.Parse(time.RFC3339, a.Session.ExpiresAt)
	if err != nil {
		return true
	}
	return time.Now().Add(refreshBuffer).After(exp)
}

// APIEndpoint returns the authoritative Copilot API base URL from the
// session response's endpoints map, falling back to the canonical
// github.com endpoint when absent. Per ADR-005 D4 + rubber-duck #2:
// always trust the server-returned URL verbatim rather than pinning.
func (a *CopilotAuth) APIEndpoint() string {
	if a != nil && a.Session.Endpoints != nil {
		if ep, ok := a.Session.Endpoints["api"]; ok && ep != "" {
			return strings.TrimRight(ep, "/")
		}
	}
	return "https://api.githubcopilot.com"
}

// checkParentDirPerms fails when the parent directory is writable by
// group or others. Intentionally permissive on the owner bits so users
// can run tpatch as themselves.
func checkParentDirPerms(dir string) error {
	info, err := os.Stat(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("auth-file parent %s is not a directory", dir)
	}
	if mode := info.Mode().Perm(); mode&0o022 != 0 {
		return fmt.Errorf("auth-file parent %s has unsafe perms %04o (expected 0700)", dir, mode)
	}
	return nil
}
