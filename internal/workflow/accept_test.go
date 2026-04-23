package workflow

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tesseracode/tesserapatch/internal/gitutil"
	"github.com/tesseracode/tesserapatch/internal/store"
)

// TestAcceptShadowCopiesResolvedContentToRealTree guards the v0.5.2
// finding #1 fix: reconcile --resolve --apply (and the manual
// reconcile --accept path) must actually copy resolved content from
// the shadow onto the real tree before reporting success.
//
// Setup: a feature that patched README.md (tracked) and added new.txt
// (untracked). We create a shadow, write resolved content there, then
// call AcceptShadow. Assertions:
//   - real tree now contains the resolved README.md and new.txt
//   - feature state → applied
//   - shadow worktree is pruned
//   - refreshed post-apply.patch exists
//   - status.json ShadowPath is cleared
func TestAcceptShadowCopiesResolvedContentToRealTree(t *testing.T) {
	tmpDir := t.TempDir()
	setupGitRepo(t, tmpDir)
	s, err := store.Init(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddFeature(store.AddFeatureInput{Title: "Accept demo", Request: "r"}); err != nil {
		t.Fatal(err)
	}
	slug := "accept-demo"

	upstream, err := gitutil.HeadCommit(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	// Seed post-apply.patch describing a README.md edit + new.txt add.
	originalPatch := `diff --git a/README.md b/README.md
--- a/README.md
+++ b/README.md
@@ -1 +1,2 @@
 # Test
+pre-resolved line (original patch)
diff --git a/new.txt b/new.txt
new file mode 100644
--- /dev/null
+++ b/new.txt
@@ -0,0 +1 @@
+pre-resolved new (original patch)
`
	if err := s.WriteArtifact(slug, "post-apply.patch", originalPatch); err != nil {
		t.Fatal(err)
	}

	// Create shadow and stage resolved content. The resolved content
	// is what the provider's conflict resolver would produce.
	shadowPath, err := gitutil.CreateShadow(tmpDir, slug, upstream)
	if err != nil {
		t.Fatalf("CreateShadow: %v", err)
	}
	if err := os.WriteFile(filepath.Join(shadowPath, "README.md"),
		[]byte("# Test\nRESOLVED content from shadow\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(shadowPath, "new.txt"),
		[]byte("RESOLVED new content from shadow\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Set feature state to reconciling-shadow as if phase 3.5 landed.
	if err := s.MarkFeatureState(slug, store.StateReconcilingShadow, "reconcile", "staged"); err != nil {
		t.Fatal(err)
	}
	st, _ := s.LoadFeatureStatus(slug)
	st.Reconcile.ShadowPath = shadowPath
	st.Reconcile.UpstreamCommit = upstream
	st.Reconcile.ResolveSession = "sess-123"
	if err := s.SaveFeatureStatus(st); err != nil {
		t.Fatal(err)
	}

	res, err := AcceptShadow(s, slug, []string{"README.md", "new.txt"}, upstream, AcceptOptions{
		Phase:            "reconcile --resolve --apply",
		ResolveSessionID: "sess-123",
	})
	if err != nil {
		t.Fatalf("AcceptShadow: %v (refreshWarning=%q)", err, func() string {
			if res != nil {
				return res.RefreshWarning
			}
			return ""
		}())
	}

	// 1. Real tree contains resolved content (NOT pre-resolved or
	//    stale shadow content).
	readme, err := os.ReadFile(filepath.Join(tmpDir, "README.md"))
	if err != nil {
		t.Fatalf("read README.md: %v", err)
	}
	if !strings.Contains(string(readme), "RESOLVED content from shadow") {
		t.Errorf("README.md not updated from shadow; got %q", string(readme))
	}
	newtxt, err := os.ReadFile(filepath.Join(tmpDir, "new.txt"))
	if err != nil {
		t.Fatalf("read new.txt: %v", err)
	}
	if !strings.Contains(string(newtxt), "RESOLVED new content from shadow") {
		t.Errorf("new.txt not written from shadow; got %q", string(newtxt))
	}

	// 2. Feature state → applied.
	st2, err := s.LoadFeatureStatus(slug)
	if err != nil {
		t.Fatal(err)
	}
	if st2.State != store.StateApplied {
		t.Errorf("feature state = %q, want %q", st2.State, store.StateApplied)
	}

	// 3. Shadow pointer is cleared from status.json; session id preserved.
	if st2.Reconcile.ShadowPath != "" {
		t.Errorf("status.Reconcile.ShadowPath = %q, want empty", st2.Reconcile.ShadowPath)
	}
	if st2.Reconcile.ResolveSession != "sess-123" {
		t.Errorf("status.Reconcile.ResolveSession = %q, want sess-123 preserved as audit",
			st2.Reconcile.ResolveSession)
	}

	// 4. Shadow worktree pruned.
	if !res.Pruned {
		t.Errorf("expected shadow pruned, res=%+v", res)
	}
	if sh, _ := gitutil.ResolveShadow(tmpDir, slug); sh != nil {
		t.Errorf("shadow still present at %s", sh.Path)
	}

	// 5. post-apply.patch refreshed.
	newPatch, err := s.ReadFeatureFile(slug, "artifacts/post-apply.patch")
	if err != nil {
		t.Fatalf("read refreshed patch: %v", err)
	}
	if !strings.Contains(newPatch, "RESOLVED content from shadow") {
		t.Errorf("refreshed post-apply.patch missing resolved README content:\n%s", newPatch)
	}

	// 6. AcceptedFiles matches input.
	if len(res.AcceptedFiles) != 2 {
		t.Errorf("AcceptedFiles = %v, want 2 entries", res.AcceptedFiles)
	}
}

// TestAcceptShadowPreservesShadowOnFailure guards the v0.5.2 design
// rule: if AcceptShadow returns an error mid-flight, the shadow is
// NOT pruned so the user can investigate.
//
// Simulation: call AcceptShadow without ever creating a shadow → the
// CopyShadowToReal step fails. Assert the function errors and any
// (nonexistent) shadow is not pruned-as-success.
func TestAcceptShadowErrorsWithoutShadow(t *testing.T) {
	tmpDir := t.TempDir()
	setupGitRepo(t, tmpDir)
	s, err := store.Init(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddFeature(store.AddFeatureInput{Title: "Accept fail", Request: "r"}); err != nil {
		t.Fatal(err)
	}
	slug := "accept-fail"

	upstream, _ := gitutil.HeadCommit(tmpDir)

	// Seed a post-apply.patch so step 1 passes, but DO NOT create the
	// shadow — CopyShadowToReal in step 2 must fail.
	_ = s.WriteArtifact(slug, "post-apply.patch", "")

	_, err = AcceptShadow(s, slug, []string{"nonexistent.txt"}, upstream, AcceptOptions{})
	if err == nil {
		t.Fatal("expected error when shadow is missing, got nil")
	}
	if !strings.Contains(err.Error(), "copy shadow") {
		t.Errorf("expected 'copy shadow' in error, got %v", err)
	}

	// State must NOT have been advanced to applied.
	st, _ := s.LoadFeatureStatus(slug)
	if st.State == store.StateApplied {
		t.Errorf("feature state wrongly advanced to applied after failure")
	}
}
