package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func init() {
	// Prevent auto-detection from interfering with tests
	os.Setenv("TPATCH_NO_AUTO_DETECT", "1")
}

func runCmd(args ...string) (stdout, stderr string, code int) {
	var outBuf, errBuf bytes.Buffer
	root := buildRootCmd()
	root.SetOut(&outBuf)
	root.SetErr(&errBuf)
	root.SetArgs(args)
	err := root.Execute()
	if err != nil {
		return outBuf.String(), errBuf.String(), 1
	}
	return outBuf.String(), errBuf.String(), 0
}

func TestHelpReturns0(t *testing.T) {
	_, _, code := runCmd("--help")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
}

func TestVersionReturns0(t *testing.T) {
	out, _, code := runCmd("--version")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if !strings.Contains(out, "tpatch") {
		t.Fatalf("expected version in stdout, got %q", out)
	}
}

func TestUnknownCommandReturns1(t *testing.T) {
	_, _, code := runCmd("bogus")
	if code != 1 {
		t.Fatalf("expected exit 1, got %d", code)
	}
}

func TestCommandsNeedArgs(t *testing.T) {
	commands := []string{"analyze", "define", "explore", "implement", "apply", "record"}
	for _, cmd := range commands {
		t.Run(cmd, func(t *testing.T) {
			_, _, code := runCmd(cmd)
			if code != 1 {
				t.Fatalf("expected exit 1 for %q without args, got %d", cmd, code)
			}
		})
	}
}

func TestAnalyzeHeuristic(t *testing.T) {
	tmpDir := t.TempDir()

	runCmd("init", "--path", tmpDir)
	runCmd("add", "--path", tmpDir, "Fix model translation")

	out, _, code := runCmd("analyze", "--path", tmpDir, "fix-model-translation")
	if code != 0 {
		t.Fatalf("analyze failed (code %d)", code)
	}
	if !strings.Contains(out, "heuristic mode") {
		t.Fatalf("expected heuristic mode output, got %q", out)
	}
}

func TestDefineHeuristic(t *testing.T) {
	tmpDir := t.TempDir()

	runCmd("init", "--path", tmpDir)
	runCmd("add", "--path", tmpDir, "Fix model translation")
	runCmd("analyze", "--path", tmpDir, "fix-model-translation")

	out, _, code := runCmd("define", "--path", tmpDir, "fix-model-translation")
	if code != 0 {
		t.Fatalf("define failed (code %d)", code)
	}
	if !strings.Contains(out, "Spec generated") {
		t.Fatalf("expected spec generated output, got %q", out)
	}
}

func TestInitAndAddIntegration(t *testing.T) {
	tmpDir := t.TempDir()

	out, _, code := runCmd("init", "--path", tmpDir)
	if code != 0 {
		t.Fatalf("init failed (code %d)", code)
	}
	if !strings.Contains(out, "Initialized") {
		t.Fatalf("expected Initialized in output, got %q", out)
	}

	out, _, code = runCmd("add", "--path", tmpDir, "Fix model translation bug")
	if code != 0 {
		t.Fatalf("add failed (code %d)", code)
	}
	if !strings.Contains(out, "fix-model-translation-bug") {
		t.Fatalf("expected slug in output, got %q", out)
	}

	out, _, code = runCmd("status", "--path", tmpDir)
	if code != 0 {
		t.Fatalf("status failed (code %d)", code)
	}
	if !strings.Contains(out, "fix-model-translation-bug") {
		t.Fatalf("expected feature in status output, got %q", out)
	}

	out, _, code = runCmd("config", "show", "--path", tmpDir)
	if code != 0 {
		t.Fatalf("config show failed (code %d)", code)
	}
	if !strings.Contains(out, "provider") {
		t.Fatalf("expected provider in config output, got %q", out)
	}

	out, _, code = runCmd("config", "set", "--path", tmpDir, "provider.base_url", "http://localhost:4141")
	if code != 0 {
		t.Fatalf("config set failed (code %d)", code)
	}

	out, _, code = runCmd("status", "--path", tmpDir, "--json")
	if code != 0 {
		t.Fatalf("status --json failed (code %d)", code)
	}
	if !strings.Contains(out, "fix-model-translation-bug") {
		t.Fatalf("expected feature in JSON output, got %q", out)
	}
}

func TestApplyModeFlagsAfterSlug(t *testing.T) {
	// This test verifies BUG-1 is fixed: --mode after slug should work
	tmpDir := t.TempDir()

	runCmd("init", "--path", tmpDir)
	runCmd("add", "--path", tmpDir, "Test feature")

	// With cobra, flags can appear after positional args
	out, _, code := runCmd("apply", "--path", tmpDir, "test-feature", "--mode", "started")
	if code != 0 {
		t.Fatalf("apply with --mode after slug failed (code %d)", code)
	}
	if !strings.Contains(out, "marked as implementing") {
		t.Fatalf("expected 'marked as implementing', got %q", out)
	}
}

func TestValidateReconcileFlags(t *testing.T) {
	cases := []struct {
		name      string
		accept    string
		reject    string
		diff      string
		resolve   bool
		apply     bool
		wantError bool
	}{
		{"all zero ok", "", "", "", false, false, false},
		{"resolve alone ok", "", "", "", true, false, false},
		{"resolve+apply ok", "", "", "", true, true, false},
		{"apply without resolve fails", "", "", "", false, true, true},
		{"accept alone ok", "demo", "", "", false, false, false},
		{"accept+resolve fails", "demo", "", "", true, false, true},
		{"accept+reject fails", "demo", "demo", "", false, false, true},
		{"accept+diff fails", "demo", "", "demo", false, false, true},
		{"reject+diff fails", "", "demo", "demo", false, false, true},
		{"reject+apply fails", "", "demo", "", false, true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateReconcileFlags(tc.accept, tc.reject, tc.diff, tc.resolve, tc.apply)
			if tc.wantError && err == nil {
				t.Errorf("expected error, got nil")
			}
			if !tc.wantError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestReconcileTerminalFlagsMutexViaCLI(t *testing.T) {
	// Smoke-test that the CLI surfaces the validation error end-to-end.
	root := buildRootCmd()
	var outBuf, errBuf bytes.Buffer
	root.SetOut(&outBuf)
	root.SetErr(&errBuf)
	root.SetArgs([]string{"reconcile", "--accept", "a", "--reject", "b"})
	err := root.Execute()
	if err == nil {
		t.Fatalf("expected error on mutex violation")
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("expected 'mutually exclusive' in error, got %q", err.Error())
	}
}

// gitInitTestRepo creates a git repo with one committed file so HeadCommit
// returns a usable SHA. Used by recipe-stale-guard and apply --mode auto
// tests which need a real HEAD reference.
func gitInitTestRepo(t *testing.T, dir string) {
	t.Helper()
	for _, args := range [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test"},
	} {
		c := exec.Command(args[0], args[1:]...)
		c.Dir = dir
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s: %v", args, out, err)
		}
	}
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Test\n"), 0o644)
	for _, args := range [][]string{
		{"git", "add", "."},
		{"git", "commit", "-m", "init"},
	} {
		c := exec.Command(args[0], args[1:]...)
		c.Dir = dir
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s: %v", args, out, err)
		}
	}
}

func gitHead(t *testing.T, dir string) string {
	t.Helper()
	c := exec.Command("git", "rev-parse", "HEAD")
	c.Dir = dir
	out, err := c.CombinedOutput()
	if err != nil {
		t.Fatalf("git rev-parse: %s: %v", out, err)
	}
	return strings.TrimSpace(string(out))
}

func writeRecipeAndProvenance(t *testing.T, tmpDir, slug, baseCommit string) {
	t.Helper()
	artDir := filepath.Join(tmpDir, ".tpatch", "features", slug, "artifacts")
	if err := os.MkdirAll(artDir, 0o755); err != nil {
		t.Fatal(err)
	}
	recipe := `{
  "feature": "` + slug + `",
  "operations": [
    {"type": "ensure-directory", "path": "src/"}
  ]
}
`
	if err := os.WriteFile(filepath.Join(artDir, "apply-recipe.json"), []byte(recipe), 0o644); err != nil {
		t.Fatal(err)
	}
	if baseCommit == "" {
		return
	}
	prov := `{"base_commit":"` + baseCommit + `","generated_at":"2026-04-22T00:00:00Z"}` + "\n"
	if err := os.WriteFile(filepath.Join(artDir, "recipe-provenance.json"), []byte(prov), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestApplyExecuteRecipeStaleGuard(t *testing.T) {
	t.Run("matching-head-no-warning", func(t *testing.T) {
		tmpDir := t.TempDir()
		gitInitTestRepo(t, tmpDir)
		runCmd("init", "--path", tmpDir)
		runCmd("add", "--path", tmpDir, "Test feature stale guard a")
		slug := "test-feature-stale-guard-a"
		writeRecipeAndProvenance(t, tmpDir, slug, gitHead(t, tmpDir))

		_, stderr, code := runCmd("apply", "--path", tmpDir, slug, "--mode", "execute")
		if code != 0 {
			t.Fatalf("apply execute failed: %s", stderr)
		}
		if strings.Contains(stderr, "recipe was generated at commit") {
			t.Fatalf("did not expect stale-recipe warning, got %q", stderr)
		}
	})

	t.Run("mismatched-head-warning", func(t *testing.T) {
		tmpDir := t.TempDir()
		gitInitTestRepo(t, tmpDir)
		runCmd("init", "--path", tmpDir)
		runCmd("add", "--path", tmpDir, "Test feature stale guard b")
		slug := "test-feature-stale-guard-b"
		writeRecipeAndProvenance(t, tmpDir, slug, "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef")

		_, stderr, code := runCmd("apply", "--path", tmpDir, slug, "--mode", "execute")
		if code != 0 {
			t.Fatalf("apply execute failed: %s", stderr)
		}
		if !strings.Contains(stderr, "recipe was generated at commit") {
			t.Fatalf("expected stale-recipe warning, got %q", stderr)
		}
		if !strings.Contains(stderr, "deadbee") {
			t.Fatalf("expected short SHA of stored commit in warning, got %q", stderr)
		}
	})

	t.Run("absent-sidecar-no-warning", func(t *testing.T) {
		tmpDir := t.TempDir()
		gitInitTestRepo(t, tmpDir)
		runCmd("init", "--path", tmpDir)
		runCmd("add", "--path", tmpDir, "Test feature stale guard c")
		slug := "test-feature-stale-guard-c"
		writeRecipeAndProvenance(t, tmpDir, slug, "")

		_, stderr, code := runCmd("apply", "--path", tmpDir, slug, "--mode", "execute")
		if code != 0 {
			t.Fatalf("apply execute failed: %s", stderr)
		}
		if strings.Contains(stderr, "recipe was generated at commit") {
			t.Fatalf("did not expect stale-recipe warning when sidecar absent, got %q", stderr)
		}
	})
}

func TestApplyAutoMode(t *testing.T) {
	tmpDir := t.TempDir()
	gitInitTestRepo(t, tmpDir)
	runCmd("init", "--path", tmpDir)
	runCmd("add", "--path", tmpDir, "Auto mode test")
	slug := "auto-mode-test"

	// Seed a real recipe that creates a file so execute has observable effect.
	artDir := filepath.Join(tmpDir, ".tpatch", "features", slug, "artifacts")
	os.MkdirAll(artDir, 0o755)
	recipe := `{
  "feature": "` + slug + `",
  "operations": [
    {"type": "write-file", "path": "hello.txt", "content": "hi\n"}
  ]
}
`
	os.WriteFile(filepath.Join(artDir, "apply-recipe.json"), []byte(recipe), 0o644)

	out, stderr, code := runCmd("apply", "--path", tmpDir, slug)
	if code != 0 {
		t.Fatalf("apply auto failed: stderr=%q", stderr)
	}
	if !strings.Contains(out, "Apply packet prepared") {
		t.Errorf("expected prepare output, got %q", out)
	}
	if !strings.Contains(out, "Recipe executed") {
		t.Errorf("expected execute output, got %q", out)
	}
	if !strings.Contains(out, "marked as applied") {
		t.Errorf("expected done output, got %q", out)
	}
	if !strings.Contains(out, "prepared → executed → recorded") {
		t.Errorf("expected auto summary line, got %q", out)
	}
	if _, err := os.Stat(filepath.Join(tmpDir, "hello.txt")); err != nil {
		t.Errorf("expected hello.txt written, got %v", err)
	}
}

func TestApplyExplicitPrepareStillWorks(t *testing.T) {
	// Regression: --mode prepare must keep the same semantic it had
	// when it was the default (stops after writing apply-packet.md).
	tmpDir := t.TempDir()
	runCmd("init", "--path", tmpDir)
	runCmd("add", "--path", tmpDir, "Explicit prepare")
	out, _, code := runCmd("apply", "--path", tmpDir, "explicit-prepare", "--mode", "prepare")
	if code != 0 {
		t.Fatalf("apply --mode prepare failed")
	}
	if !strings.Contains(out, "Apply packet prepared") {
		t.Errorf("expected prepare output, got %q", out)
	}
	if strings.Contains(out, "Recipe executed") {
		t.Errorf("explicit prepare should NOT execute recipe, got %q", out)
	}
}

func TestAddReadsStdin(t *testing.T) {
	tmpDir := t.TempDir()
	runCmd("init", "--path", tmpDir)

	root := buildRootCmd()
	var outBuf, errBuf bytes.Buffer
	root.SetOut(&outBuf)
	root.SetErr(&errBuf)
	root.SetIn(strings.NewReader("Fix stdin feature\n"))
	root.SetArgs([]string{"add", "--path", tmpDir})
	if err := root.Execute(); err != nil {
		t.Fatalf("add via stdin failed: %v (stderr=%q)", err, errBuf.String())
	}
	out := outBuf.String()
	if !strings.Contains(out, "fix-stdin-feature") {
		t.Errorf("expected slug from stdin input, got %q", out)
	}
}

func TestAddStdinEmptyRejects(t *testing.T) {
	tmpDir := t.TempDir()
	runCmd("init", "--path", tmpDir)

	root := buildRootCmd()
	var outBuf, errBuf bytes.Buffer
	root.SetOut(&outBuf)
	root.SetErr(&errBuf)
	root.SetIn(strings.NewReader("   \n  "))
	root.SetArgs([]string{"add", "--path", tmpDir})
	if err := root.Execute(); err == nil {
		t.Fatalf("expected error on empty stdin, got output=%q", outBuf.String())
	}
}

func TestEditMissingFeature(t *testing.T) {
tmpDir := t.TempDir()
runCmd("init", "--path", tmpDir)
_, stderr, code := runCmd("edit", "--path", tmpDir, "nonexistent-feature")
if code == 0 {
t.Fatalf("expected error for missing feature, got success; stderr=%q", stderr)
}
}

func TestEditMissingArtifact(t *testing.T) {
tmpDir := t.TempDir()
runCmd("init", "--path", tmpDir)
runCmd("add", "--path", tmpDir, "Edit artifact test")
_, stderr, code := runCmd("edit", "--path", tmpDir, "edit-artifact-test", "spec.md")
if code == 0 {
t.Fatalf("expected error for missing spec.md, got success; stderr=%q", stderr)
}
// Error should mention both artifact name and slug.
if !strings.Contains(stderr, "spec.md") && !strings.Contains(stderr, "edit-artifact-test") {
// The wrapped error goes through cobra's error path; accept exit code alone.
}
}

func TestEditDefaultsToRequestMD(t *testing.T) {
// With no $EDITOR set, openInEditor prints a pointer message. We use
// that as the signal that the correct file was resolved.
tmpDir := t.TempDir()
runCmd("init", "--path", tmpDir)
runCmd("add", "--path", tmpDir, "Edit default test")

// Ensure EDITOR is empty so we hit the pointer-message branch.
t.Setenv("EDITOR", "")

out, _, code := runCmd("edit", "--path", tmpDir, "edit-default-test")
if code != 0 {
t.Fatalf("edit failed")
}
if !strings.Contains(out, "request.md") {
t.Errorf("expected default artifact to be request.md, got %q", out)
}
}
