package gitutil

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// commitFile writes a file under dir, stages it, and commits with the given message.
// Returns the resulting HEAD SHA.
func commitFile(t *testing.T, dir, relPath, contents, msg string) string {
	t.Helper()
	full := filepath.Join(dir, relPath)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := runGit(dir, "add", relPath); err != nil {
		t.Fatalf("git add %s: %v", relPath, err)
	}
	if _, err := runGit(dir, "commit", "-q", "-m", msg); err != nil {
		t.Fatalf("git commit %q: %v", msg, err)
	}
	sha, err := runGit(dir, "rev-parse", "HEAD")
	if err != nil {
		t.Fatalf("rev-parse: %v", err)
	}
	return strings.TrimSpace(sha)
}

// TestCapturePatchFromCommitsScoped_FilesScoping pins the headline use case:
// a committed range that touches multiple files can be narrowed to a subset
// via pathspecs, and out-of-scope files are dropped from the patch.
func TestCapturePatchFromCommitsScoped_FilesScoping(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)

	// Baseline: a, b, c all exist with initial content.
	for _, p := range []string{"a.txt", "b.txt", "c.txt"} {
		if err := os.WriteFile(filepath.Join(dir, p), []byte("v0\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := runGit(dir, "add", "."); err != nil {
		t.Fatal(err)
	}
	baseSha := ""
	if _, err := runGit(dir, "commit", "-q", "-m", "baseline"); err != nil {
		t.Fatal(err)
	}
	if out, err := runGit(dir, "rev-parse", "HEAD"); err != nil {
		t.Fatal(err)
	} else {
		baseSha = strings.TrimSpace(out)
	}

	// Commit A touches a.txt + b.txt.
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("vA\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.txt"), []byte("vA\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := runGit(dir, "add", "."); err != nil {
		t.Fatal(err)
	}
	if _, err := runGit(dir, "commit", "-q", "-m", "A"); err != nil {
		t.Fatal(err)
	}

	// Commit B touches b.txt + c.txt.
	if err := os.WriteFile(filepath.Join(dir, "b.txt"), []byte("vB\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "c.txt"), []byte("vB\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := runGit(dir, "add", "."); err != nil {
		t.Fatal(err)
	}
	if _, err := runGit(dir, "commit", "-q", "-m", "B"); err != nil {
		t.Fatal(err)
	}

	scoped, err := CapturePatchFromCommitsScoped(dir, baseSha, "HEAD", []string{"b.txt"})
	if err != nil {
		t.Fatalf("CapturePatchFromCommitsScoped: %v", err)
	}
	if !strings.Contains(scoped, "b.txt") {
		t.Errorf("scoped capture missing in-scope file b.txt:\n%s", scoped)
	}
	if strings.Contains(scoped, "a.txt") {
		t.Errorf("scoped capture leaked out-of-scope file a.txt:\n%s", scoped)
	}
	if strings.Contains(scoped, "c.txt") {
		t.Errorf("scoped capture leaked out-of-scope file c.txt:\n%s", scoped)
	}
}

// TestCapturePatchFromCommitsScoped_ToRefCaps verifies the upper bound is honored.
func TestCapturePatchFromCommitsScoped_ToRefCaps(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)

	for _, p := range []string{"a.txt", "b.txt"} {
		if err := os.WriteFile(filepath.Join(dir, p), []byte("v0\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := runGit(dir, "add", "."); err != nil {
		t.Fatal(err)
	}
	if _, err := runGit(dir, "commit", "-q", "-m", "baseline"); err != nil {
		t.Fatal(err)
	}
	baseShaOut, _ := runGit(dir, "rev-parse", "HEAD")
	baseSha := strings.TrimSpace(baseShaOut)

	// Commit A touches a.txt.
	shaA := commitFile(t, dir, "a.txt", "vA\n", "A touches a.txt")
	// Commit B touches b.txt — must NOT appear when toRef caps at A.
	commitFile(t, dir, "b.txt", "vB\n", "B touches b.txt")

	scoped, err := CapturePatchFromCommitsScoped(dir, baseSha, shaA, []string{"a.txt", "b.txt"})
	if err != nil {
		t.Fatalf("CapturePatchFromCommitsScoped: %v", err)
	}
	if !strings.Contains(scoped, "a.txt") {
		t.Errorf("range base..A should include a.txt:\n%s", scoped)
	}
	if strings.Contains(scoped, "vB") || strings.Contains(scoped, "b.txt") {
		t.Errorf("range capped at A must not include B's b.txt change:\n%s", scoped)
	}
}

// TestCapturePatchFromCommitsScoped_ExcludesArtifacts verifies that scoped
// commit-range capture still strips .tpatch/ etc. from the diff, matching the
// historical CapturePatchFromCommits exclusion behaviour.
func TestCapturePatchFromCommitsScoped_ExcludesArtifacts(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)

	if _, err := runGit(dir, "rev-parse", "HEAD"); err != nil {
		t.Fatal(err)
	}
	baseShaOut, _ := runGit(dir, "rev-parse", "HEAD")
	baseSha := strings.TrimSpace(baseShaOut)

	// Touch a real file AND a .tpatch artifact in the same commit.
	if err := os.MkdirAll(filepath.Join(dir, ".tpatch"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".tpatch", "noise.txt"), []byte("artifact\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "real.txt"), []byte("real\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := runGit(dir, "add", "-A"); err != nil {
		t.Fatal(err)
	}
	if _, err := runGit(dir, "commit", "-q", "-m", "mixed"); err != nil {
		t.Fatal(err)
	}

	scoped, err := CapturePatchFromCommitsScoped(dir, baseSha, "HEAD", nil)
	if err != nil {
		t.Fatalf("CapturePatchFromCommitsScoped: %v", err)
	}
	if !strings.Contains(scoped, "real.txt") {
		t.Errorf("expected real.txt in capture:\n%s", scoped)
	}
	if strings.Contains(scoped, ".tpatch/noise.txt") {
		t.Errorf("scoped capture leaked excluded .tpatch/ artifact:\n%s", scoped)
	}
}

// TestCapturePatchFromCommits_DefaultMatchesScoped pins the byte-for-byte
// backwards-compat guarantee: the legacy unscoped wrapper must produce
// identical output to CapturePatchFromCommitsScoped(..., nil).
func TestCapturePatchFromCommits_DefaultMatchesScoped(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)

	baseShaOut, _ := runGit(dir, "rev-parse", "HEAD")
	baseSha := strings.TrimSpace(baseShaOut)
	commitFile(t, dir, "x.txt", "x\n", "add x")

	legacy, err := CapturePatchFromCommits(dir, baseSha, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	scoped, err := CapturePatchFromCommitsScoped(dir, baseSha, "HEAD", nil)
	if err != nil {
		t.Fatal(err)
	}
	if legacy != scoped {
		t.Errorf("legacy wrapper diverges from scoped(nil).\n--- legacy ---\n%s\n--- scoped(nil) ---\n%s", legacy, scoped)
	}
}
