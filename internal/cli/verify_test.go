package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// runVerifyForExitCode invokes `tpatch verify` through the real cobra
// root and returns the unwrapped error so tests can assert on
// *ExitCodeError directly. The package-level `runCmd` helper collapses
// every error to exit code 1, which would mask the very plumbing these
// regression tests are guarding (M15-W3-SLICE-A revision 3 — supervisor
// reproduced exit-1 leaks for missing-slug and non-tpatch-workspace).
func runVerifyForExitCode(args ...string) error {
	root := buildRootCmd()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs(append([]string{"verify"}, args...))
	return root.Execute()
}

// TestVerify_MissingSlug_ExitsTwo locks in PRD-verify-freshness §5
// "feature not found" → exit 2. Before revision 3 the missing-slug
// surface fell through `LoadFeatureStatus` → plain error → exit 1.
func TestVerify_MissingSlug_ExitsTwo(t *testing.T) {
	tmp := t.TempDir()
	gitInitTestRepo(t, tmp)
	if _, _, code := runCmd("init", "--path", tmp); code != 0 {
		t.Fatalf("init failed: %d", code)
	}

	err := runVerifyForExitCode("--path", tmp, "nope")
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	var ec *ExitCodeError
	if !errors.As(err, &ec) {
		t.Fatalf("expected *ExitCodeError, got %T: %v", err, err)
	}
	if ec.Code != 2 {
		t.Errorf("missing slug must exit 2 (PRD §5), got %d", ec.Code)
	}
}

// TestVerify_NonTpatchWorkspace_ExitsTwo locks in PRD-verify-freshness §5
// "verify inside a non-tpatch-init repo → exit 2 — not a tpatch workspace".
// The error originates in `openStoreFromCmd` and previously bypassed the
// typed exit-code wrap.
func TestVerify_NonTpatchWorkspace_ExitsTwo(t *testing.T) {
	tmp := t.TempDir()

	err := runVerifyForExitCode("--path", tmp, "nope")
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	var ec *ExitCodeError
	if !errors.As(err, &ec) {
		t.Fatalf("expected *ExitCodeError, got %T: %v", err, err)
	}
	if ec.Code != 2 {
		t.Errorf("non-tpatch workspace must exit 2 (PRD §5), got %d", ec.Code)
	}
}

// TestVerify_V0AbortFromRunVerify_ExitsTwo locks in the V0 abort path:
// a feature whose status.json is corrupt cannot be loaded, so RunVerify
// returns the report + a non-refusal error. PRD §5 + §6 Q7 bind that to
// exit 2 ("internal error").
func TestVerify_V0AbortFromRunVerify_ExitsTwo(t *testing.T) {
	tmp := t.TempDir()
	gitInitTestRepo(t, tmp)
	if _, _, code := runCmd("init", "--path", tmp); code != 0 {
		t.Fatalf("init failed: %d", code)
	}
	if _, _, code := runCmd("add", "--path", tmp, "--slug", "demo", "demo"); code != 0 {
		t.Fatalf("add failed: %d", code)
	}
	statusPath := filepath.Join(tmp, ".tpatch", "features", "demo", "status.json")
	if err := os.WriteFile(statusPath, []byte("{not valid json"), 0o644); err != nil {
		t.Fatalf("corrupt status.json: %v", err)
	}

	err := runVerifyForExitCode("--path", tmp, "demo")
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	var ec *ExitCodeError
	if !errors.As(err, &ec) {
		t.Fatalf("expected *ExitCodeError, got %T: %v", err, err)
	}
	if ec.Code != 2 {
		t.Errorf("V0 abort must exit 2 (PRD §5 / §6 Q7), got %d", ec.Code)
	}
}
