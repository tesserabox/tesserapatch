package workflow

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tesseracode/tesserapatch/internal/gitutil"
	"github.com/tesseracode/tesserapatch/internal/store"
)

func TestFilesInPatch(t *testing.T) {
	patch := `diff --git a/foo.txt b/foo.txt
index 111..222 100644
--- a/foo.txt
+++ b/foo.txt
@@ -1 +1 @@
-hi
+hello
diff --git a/bar/baz.go b/bar/baz.go
new file mode 100644
--- /dev/null
+++ b/bar/baz.go
@@ -0,0 +1 @@
+package baz
diff --git a/foo.txt b/foo.txt
`
	got := gitutil.FilesInPatch(patch)
	want := []string{"foo.txt", "bar/baz.go"}
	if len(got) != len(want) {
		t.Fatalf("len=%d want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("at %d: got %q want %q", i, got[i], want[i])
		}
	}
}

func TestFilesInPatchEmpty(t *testing.T) {
	if got := gitutil.FilesInPatch(""); len(got) != 0 {
		t.Errorf("expected no files, got %v", got)
	}
}

func TestRefreshAfterAcceptRegeneratesArtifacts(t *testing.T) {
	tmpDir := t.TempDir()
	setupGitRepo(t, tmpDir)
	s, err := store.Init(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	s.AddFeature(store.AddFeatureInput{Title: "Demo", Request: "demo"})
	slug := "demo"

	// Record the upstream commit (the single "initial commit" setupGitRepo makes).
	upstream, err := gitutil.HeadCommit(tmpDir)
	if err != nil {
		t.Fatalf("head: %v", err)
	}

	// Simulate the post-accept working tree: feature touched a new file
	// and modified README.md. The ORIGINAL patch (pre-accept) is what
	// determines which paths the refresh will regenerate against.
	if err := os.WriteFile(filepath.Join(tmpDir, "feature.txt"), []byte("feature content\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "README.md"), []byte("# Test\nupdated line\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Also create an unrelated untracked file — refresh must NOT pull it in.
	if err := os.WriteFile(filepath.Join(tmpDir, "unrelated.log"), []byte("noise\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	originalPatch := `diff --git a/feature.txt b/feature.txt
new file mode 100644
--- /dev/null
+++ b/feature.txt
@@ -0,0 +1 @@
+feature content
diff --git a/README.md b/README.md
--- a/README.md
+++ b/README.md
@@ -1 +1,2 @@
 # Test
+updated line
`
	if err := s.WriteArtifact(slug, "post-apply.patch", originalPatch); err != nil {
		t.Fatal(err)
	}

	if err := RefreshAfterAccept(s, slug, upstream, originalPatch); err != nil {
		t.Fatalf("RefreshAfterAccept: %v", err)
	}

	// 1. post-apply.patch was rewritten and reflects both touched files.
	newPatch, err := s.ReadFeatureFile(slug, "artifacts/post-apply.patch")
	if err != nil {
		t.Fatalf("read new patch: %v", err)
	}
	if !strings.Contains(newPatch, "feature.txt") {
		t.Errorf("refreshed patch missing feature.txt:\n%s", newPatch)
	}
	if !strings.Contains(newPatch, "README.md") {
		t.Errorf("refreshed patch missing README.md:\n%s", newPatch)
	}
	if strings.Contains(newPatch, "unrelated.log") {
		t.Errorf("refreshed patch leaked untracked file:\n%s", newPatch)
	}

	// 2. A numbered reconcile patch exists in patches/.
	entries, err := os.ReadDir(filepath.Join(tmpDir, ".tpatch", "features", slug, "patches"))
	if err != nil {
		t.Fatalf("readdir patches: %v", err)
	}
	found := false
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), "-reconcile.patch") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected a patches/NNN-reconcile.patch snapshot; got %v", entries)
	}
}

func TestForwardApplyExcluding(t *testing.T) {
	tmpDir := t.TempDir()
	setupGitRepo(t, tmpDir)
	// Two-file patch: one we'll exclude, one should land.
	patch := `diff --git a/a.txt b/a.txt
new file mode 100644
--- /dev/null
+++ b/a.txt
@@ -0,0 +1 @@
+A
diff --git a/b.txt b/b.txt
new file mode 100644
--- /dev/null
+++ b/b.txt
@@ -0,0 +1 @@
+B
`
	if err := gitutil.ForwardApplyExcluding(tmpDir, patch, []string{"a.txt"}); err != nil {
		t.Fatalf("ForwardApplyExcluding: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmpDir, "a.txt")); !os.IsNotExist(err) {
		t.Errorf("a.txt should have been excluded, but it exists (err=%v)", err)
	}
	if content, err := os.ReadFile(filepath.Join(tmpDir, "b.txt")); err != nil || strings.TrimSpace(string(content)) != "B" {
		t.Errorf("b.txt should have been applied; got content=%q err=%v", content, err)
	}
}
