package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// gitCommitFile writes a file under dir, stages it, commits it, and returns
// the resulting HEAD SHA. Used by the record committed-range tests.
func gitCommitFile(t *testing.T, dir, relPath, contents, msg string) string {
	t.Helper()
	full := filepath.Join(dir, relPath)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"add", relPath},
		{"-c", "commit.gpgsign=false", "commit", "-q", "-m", msg},
	} {
		c := exec.Command("git", args...)
		c.Dir = dir
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s: %v", args, out, err)
		}
	}
	c := exec.Command("git", "rev-parse", "HEAD")
	c.Dir = dir
	out, err := c.CombinedOutput()
	if err != nil {
		t.Fatalf("git rev-parse: %s: %v", out, err)
	}
	return strings.TrimSpace(string(out))
}

// setupRecordRangeFixture builds a fixture for committed-range record tests:
//
//	gitInitTestRepo  (commit 0 — README.md)
//	commit A         (touches src/a.txt + src/b.txt)
//	commit B         (touches src/b.txt + noise.txt)
//
// Returns (tmpDir, baseSha, shaA, shaB).
func setupRecordRangeFixture(t *testing.T) (tmpDir, baseSha, shaA, shaB string) {
	t.Helper()
	tmpDir = t.TempDir()
	gitInitTestRepo(t, tmpDir)
	baseSha = gitHead(t, tmpDir)

	// Commit A — touches two files under src/.
	if err := os.MkdirAll(filepath.Join(tmpDir, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(tmpDir, "src", "a.txt"), []byte("a v1\n"), 0o644)
	os.WriteFile(filepath.Join(tmpDir, "src", "b.txt"), []byte("b v1\n"), 0o644)
	for _, args := range [][]string{
		{"add", "."},
		{"-c", "commit.gpgsign=false", "commit", "-q", "-m", "A"},
	} {
		c := exec.Command("git", args...)
		c.Dir = tmpDir
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s: %v", args, out, err)
		}
	}
	shaA = gitHead(t, tmpDir)

	// Commit B — touches src/b.txt + a top-level noise.txt.
	os.WriteFile(filepath.Join(tmpDir, "src", "b.txt"), []byte("b v2\n"), 0o644)
	os.WriteFile(filepath.Join(tmpDir, "noise.txt"), []byte("noise\n"), 0o644)
	for _, args := range [][]string{
		{"add", "."},
		{"-c", "commit.gpgsign=false", "commit", "-q", "-m", "B"},
	} {
		c := exec.Command("git", args...)
		c.Dir = tmpDir
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s: %v", args, out, err)
		}
	}
	shaB = gitHead(t, tmpDir)
	return
}

// TestRecordCmd_FromAndFiles_Compatible verifies the headline fix: --files
// and --from can be combined to scope a committed-range capture to specific
// paths.
func TestRecordCmd_FromAndFiles_Compatible(t *testing.T) {
	tmpDir, baseSha, _, _ := setupRecordRangeFixture(t)

	runCmd("init", "--path", tmpDir)
	runCmd("add", "--path", tmpDir, "From and files compat")
	slug := "from-and-files-compat"

	_, stderr, code := runCmd("record", "--path", tmpDir, slug,
		"--from", baseSha, "--files", "src/b.txt", "--lenient")
	if code != 0 {
		t.Fatalf("record --from --files failed: stderr=%q", stderr)
	}

	patchPath := filepath.Join(tmpDir, ".tpatch", "features", slug, "artifacts", "post-apply.patch")
	got, err := os.ReadFile(patchPath)
	if err != nil {
		t.Fatalf("read post-apply.patch: %v", err)
	}
	patch := string(got)
	if !strings.Contains(patch, "src/b.txt") {
		t.Errorf("scoped committed-range capture missing in-scope file:\n%s", patch)
	}
	if strings.Contains(patch, "src/a.txt") {
		t.Errorf("scoped capture leaked src/a.txt:\n%s", patch)
	}
	if strings.Contains(patch, "noise.txt") {
		t.Errorf("scoped capture leaked noise.txt:\n%s", patch)
	}
}

// TestRecordCmd_CommitRangeAndFiles_Compatible verifies the explicit
// --commit-range form works with --files.
func TestRecordCmd_CommitRangeAndFiles_Compatible(t *testing.T) {
	tmpDir, baseSha, _, shaB := setupRecordRangeFixture(t)

	runCmd("init", "--path", tmpDir)
	runCmd("add", "--path", tmpDir, "Commit range and files")
	slug := "commit-range-and-files"

	_, stderr, code := runCmd("record", "--path", tmpDir, slug,
		"--commit-range", baseSha+".."+shaB, "--files", "src/b.txt", "--lenient")
	if code != 0 {
		t.Fatalf("record --commit-range --files failed: stderr=%q", stderr)
	}

	patchPath := filepath.Join(tmpDir, ".tpatch", "features", slug, "artifacts", "post-apply.patch")
	got, err := os.ReadFile(patchPath)
	if err != nil {
		t.Fatalf("read post-apply.patch: %v", err)
	}
	patch := string(got)
	if !strings.Contains(patch, "src/b.txt") {
		t.Errorf("scoped --commit-range capture missing src/b.txt:\n%s", patch)
	}
	if strings.Contains(patch, "src/a.txt") || strings.Contains(patch, "noise.txt") {
		t.Errorf("scoped --commit-range capture leaked out-of-scope files:\n%s", patch)
	}
}

// TestRecordCmd_ToRefCaps verifies --from + --to caps the upper bound.
func TestRecordCmd_ToRefCaps(t *testing.T) {
	tmpDir, baseSha, shaA, _ := setupRecordRangeFixture(t)

	runCmd("init", "--path", tmpDir)
	runCmd("add", "--path", tmpDir, "To ref caps")
	slug := "to-ref-caps"

	_, stderr, code := runCmd("record", "--path", tmpDir, slug,
		"--from", baseSha, "--to", shaA, "--lenient")
	if code != 0 {
		t.Fatalf("record --from --to failed: stderr=%q", stderr)
	}

	patchPath := filepath.Join(tmpDir, ".tpatch", "features", slug, "artifacts", "post-apply.patch")
	got, err := os.ReadFile(patchPath)
	if err != nil {
		t.Fatalf("read post-apply.patch: %v", err)
	}
	patch := string(got)
	// Range base..A should include A's two files but not B's noise.txt.
	if !strings.Contains(patch, "src/a.txt") || !strings.Contains(patch, "src/b.txt") {
		t.Errorf("base..A capture missing A's files:\n%s", patch)
	}
	if strings.Contains(patch, "noise.txt") {
		t.Errorf("--to capped at A but capture includes B's noise.txt:\n%s", patch)
	}
	// b.txt's content at A is "b v1", not B's "b v2".
	if strings.Contains(patch, "b v2") {
		t.Errorf("--to capped at A but capture includes B's b.txt content:\n%s", patch)
	}
}

// TestRecordCmd_CommitRange_RejectsWithFrom asserts mutex.
func TestRecordCmd_CommitRange_RejectsWithFrom(t *testing.T) {
	tmpDir, baseSha, _, shaB := setupRecordRangeFixture(t)
	runCmd("init", "--path", tmpDir)
	runCmd("add", "--path", tmpDir, "Range plus from")
	slug := "range-plus-from"

	root := buildRootCmd()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"record", "--path", tmpDir, slug,
		"--commit-range", baseSha + ".." + shaB, "--from", baseSha})
	err := root.Execute()
	if err == nil {
		t.Fatalf("expected error combining --commit-range and --from")
	}
	if !strings.Contains(err.Error(), "--commit-range is mutually exclusive with --from") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestRecordCmd_CommitRange_RejectsWithTo asserts mutex.
func TestRecordCmd_CommitRange_RejectsWithTo(t *testing.T) {
	tmpDir, baseSha, _, shaB := setupRecordRangeFixture(t)
	runCmd("init", "--path", tmpDir)
	runCmd("add", "--path", tmpDir, "Range plus to")
	slug := "range-plus-to"

	root := buildRootCmd()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"record", "--path", tmpDir, slug,
		"--commit-range", baseSha + ".." + shaB, "--to", shaB})
	err := root.Execute()
	if err == nil {
		t.Fatalf("expected error combining --commit-range and --to")
	}
	if !strings.Contains(err.Error(), "--commit-range is mutually exclusive with --to") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestRecordCmd_WorkingTreeFilesUnchanged verifies the original working-tree
// --files behaviour is unregressed (no --from / --to / --commit-range).
func TestRecordCmd_WorkingTreeFilesUnchanged(t *testing.T) {
	tmpDir := t.TempDir()
	gitInitTestRepo(t, tmpDir)
	runCmd("init", "--path", tmpDir)
	runCmd("add", "--path", tmpDir, "Working tree files unchanged")
	slug := "working-tree-files-unchanged"

	if err := os.MkdirAll(filepath.Join(tmpDir, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(tmpDir, "src", "auth.go"), []byte("package src\n"), 0o644)
	os.WriteFile(filepath.Join(tmpDir, "noise.txt"), []byte("unrelated\n"), 0o644)

	_, stderr, code := runCmd("record", "--path", tmpDir, slug, "--files", "src/", "--lenient")
	if code != 0 {
		t.Fatalf("record --files failed: %s", stderr)
	}
	patchPath := filepath.Join(tmpDir, ".tpatch", "features", slug, "artifacts", "post-apply.patch")
	got, err := os.ReadFile(patchPath)
	if err != nil {
		t.Fatalf("read post-apply.patch: %v", err)
	}
	patch := string(got)
	if !strings.Contains(patch, "src/auth.go") {
		t.Errorf("working-tree scoped capture missing src/auth.go:\n%s", patch)
	}
	if strings.Contains(patch, "noise.txt") {
		t.Errorf("working-tree scoped capture leaked noise.txt:\n%s", patch)
	}
}

// silence unused-import warning if helpers go unreferenced.
var _ = gitCommitFile
