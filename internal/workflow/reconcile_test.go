package workflow

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/tesseracode/tesserapatch/internal/provider"
	"github.com/tesseracode/tesserapatch/internal/store"
)

// setupGitRepo creates a git repo in dir with an initial commit.
func setupGitRepo(t *testing.T, dir string) {
	t.Helper()
	cmds := [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git setup %v: %s: %v", args, out, err)
		}
	}

	// Create initial file and commit
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Test\n"), 0o644)
	gitAdd(t, dir, ".")
	gitCommit(t, dir, "initial commit")
}

func gitAdd(t *testing.T, dir, path string) {
	t.Helper()
	cmd := exec.Command("git", "add", path)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git add: %s: %v", out, err)
	}
}

func gitCommit(t *testing.T, dir, msg string) {
	t.Helper()
	cmd := exec.Command("git", "commit", "-m", msg, "--allow-empty")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %s: %v", out, err)
	}
}

func TestReconcilePhase1_UpstreamedViaReverseApply(t *testing.T) {
	tmpDir := t.TempDir()
	setupGitRepo(t, tmpDir)

	// Initialize tpatch
	s, err := store.Init(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	// Add a feature and create a patch
	s.AddFeature(store.AddFeatureInput{Title: "Add greeting", Request: "Add a greeting file"})
	s.MarkFeatureState("add-greeting", store.StateApplied, "apply", "")

	// Create a file (the "change") and record as patch
	os.WriteFile(filepath.Join(tmpDir, "greeting.txt"), []byte("Hello World\n"), 0o644)
	gitAdd(t, tmpDir, "greeting.txt")
	gitCommit(t, tmpDir, "add greeting feature")

	// Create the patch artifact (diff from initial to current)
	patch := `diff --git a/greeting.txt b/greeting.txt
new file mode 100644
index 0000000..557db03
--- /dev/null
+++ b/greeting.txt
@@ -0,0 +1 @@
+Hello World
`
	s.WriteArtifact("add-greeting", "post-apply.patch", patch)

	// The patch is already applied (reverse-apply should succeed), so Phase 1 → UPSTREAMED
	results, err := RunReconcile(context.Background(), s, []string{"add-greeting"}, "HEAD", nil, provider.Config{}, ReconcileOptions{})
	if err != nil {
		t.Fatal(err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Outcome != store.ReconcileUpstreamed {
		t.Fatalf("expected upstreamed, got %s", results[0].Outcome)
	}
	if results[0].Phase != "phase-1-reverse-apply" {
		t.Fatalf("expected phase-1, got %s", results[0].Phase)
	}
}

func TestReconcilePhase4_ReappliedViaForwardApply(t *testing.T) {
	tmpDir := t.TempDir()
	setupGitRepo(t, tmpDir)

	s, err := store.Init(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	s.AddFeature(store.AddFeatureInput{Title: "Add models command", Request: "Add a models CLI subcommand"})
	s.MarkFeatureState("add-models-command", store.StateApplied, "apply", "")

	// Create a patch that adds a new file (not yet applied to the repo)
	patch := `diff --git a/models.txt b/models.txt
new file mode 100644
index 0000000..abc1234
--- /dev/null
+++ b/models.txt
@@ -0,0 +1,3 @@
+gpt-4o
+claude-opus-4.6
+claude-opus-4.6-1m
`
	s.WriteArtifact("add-models-command", "post-apply.patch", patch)

	// The patch does NOT exist yet, but can be applied cleanly → Phase 4 → REAPPLIED
	results, err := RunReconcile(context.Background(), s, []string{"add-models-command"}, "HEAD", nil, provider.Config{}, ReconcileOptions{})
	if err != nil {
		t.Fatal(err)
	}

	if results[0].Outcome != store.ReconcileReapplied {
		t.Fatalf("expected reapplied, got %s", results[0].Outcome)
	}
	// Phase name is now three-state: strict / 3way / conflicts. This
	// test adds a brand-new file so the strict verdict is expected.
	if results[0].Phase != "phase-4-forward-apply-strict" {
		t.Fatalf("expected phase-4-forward-apply-strict, got %s", results[0].Phase)
	}
}

// TestReconcilePhase4_ConflictMarkersAreBlocked reproduces
// bug-reconcile-phase4-false-positive: upstream modifies the same
// line the feature patch touches. `git apply --3way --check` used to
// return 0 (merge is *attemptable*) so reconcile falsely reported
// "reapplied". With PreviewForwardApply the 3-way merge actually runs
// in an isolated worktree; conflict markers are detected and the
// verdict is promoted to BLOCKED with the offending files surfaced.
func TestReconcilePhase4_ConflictMarkersAreBlocked(t *testing.T) {
	tmpDir := t.TempDir()
	setupGitRepo(t, tmpDir)

	// Baseline file the feature and upstream will both edit on the
	// same line — guaranteed 3-way conflict.
	if err := os.WriteFile(filepath.Join(tmpDir, "shared.txt"), []byte("line-a\nline-b\nline-c\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitAdd(t, tmpDir, "shared.txt")
	gitCommit(t, tmpDir, "add shared.txt")

	s, err := store.Init(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	s.AddFeature(store.AddFeatureInput{Title: "Shared edit", Request: "Change line-b to feature value"})
	s.MarkFeatureState("shared-edit", store.StateApplied, "apply", "")

	// Blob SHAs baked into the patch so git --3way can look up the
	// merge base. Compute them from the current HEAD.
	baseBlob := gitHashObject(t, tmpDir, "shared.txt")
	// Feature wants "line-b-feature" (recorded patch, NOT applied to tree).
	featureBlob := computeBlobSHA("line-a\nline-b-feature\nline-c\n")
	patch := "diff --git a/shared.txt b/shared.txt\n" +
		"index " + baseBlob + ".." + featureBlob + " 100644\n" +
		"--- a/shared.txt\n" +
		"+++ b/shared.txt\n" +
		"@@ -1,3 +1,3 @@\n" +
		" line-a\n" +
		"-line-b\n" +
		"+line-b-feature\n" +
		" line-c\n"
	s.WriteArtifact("shared-edit", "post-apply.patch", patch)

	// Upstream edits the same line to a different value. `git apply
	// --3way` will produce conflict markers on shared.txt.
	if err := os.WriteFile(filepath.Join(tmpDir, "shared.txt"), []byte("line-a\nline-b-upstream\nline-c\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitAdd(t, tmpDir, "shared.txt")
	gitCommit(t, tmpDir, "upstream edit")

	results, err := RunReconcile(context.Background(), s, []string{"shared-edit"}, "HEAD", nil, provider.Config{}, ReconcileOptions{})
	if err != nil {
		t.Fatal(err)
	}

	if results[0].Outcome != store.ReconcileBlocked {
		t.Fatalf("expected blocked due to conflict markers, got %s (phase=%s, notes=%v)",
			results[0].Outcome, results[0].Phase, results[0].Notes)
	}
	if results[0].Phase != "phase-4-forward-apply-conflicts" {
		t.Fatalf("expected phase-4-forward-apply-conflicts, got %s", results[0].Phase)
	}
	// The conflicted file should be reported verbatim.
	found := false
	for _, c := range results[0].Conflicts {
		if c == "shared.txt" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected shared.txt in Conflicts, got %v", results[0].Conflicts)
	}
}

func gitHashObject(t *testing.T, dir, path string) string {
	t.Helper()
	cmd := exec.Command("git", "hash-object", path)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git hash-object: %v", err)
	}
	return string(out[:len(out)-1]) // trim trailing \n
}

// computeBlobSHA returns the git blob SHA for a string, matching what
// `git hash-object -w` would produce. Used to fabricate valid index
// lines in synthetic patches for --3way to look up.
func computeBlobSHA(content string) string {
	h := sha1.New()
	header := "blob " + strconv.Itoa(len(content)) + "\x00"
	h.Write([]byte(header))
	h.Write([]byte(content))
	return hex.EncodeToString(h.Sum(nil))
}

// TestReconcilePromotesOnLiveMarkers reproduces
// bug-reconcile-reapplied-with-conflict-markers (t3code case study,
// v0.4.4). Even if PreviewForwardApply returns a clean verdict in an
// isolated worktree, the live working tree may contain unresolved
// conflict markers (leftover from a prior reconcile, a manual edit,
// or an outside merge). Returning "reapplied" in that state is a
// data-corruption risk: the user commits bad code trusting the
// verdict. The defensive ScanConflictMarkers pass must promote to
// Blocked and name the offending files.
func TestReconcilePromotesOnLiveMarkers(t *testing.T) {
	tmpDir := t.TempDir()
	setupGitRepo(t, tmpDir)

	s, err := store.Init(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	s.AddFeature(store.AddFeatureInput{Title: "Add note", Request: "Add a note file"})
	s.MarkFeatureState("add-note", store.StateApplied, "apply", "")

	// A trivially-applicable patch (new file) — would normally give
	// ForwardApplyStrict and verdict Reapplied.
	patch := `diff --git a/note.txt b/note.txt
new file mode 100644
index 0000000..abc1234
--- /dev/null
+++ b/note.txt
@@ -0,0 +1,1 @@
+hello
`
	s.WriteArtifact("add-note", "post-apply.patch", patch)

	// Plant conflict markers in an unrelated file in the live tree.
	if err := os.WriteFile(filepath.Join(tmpDir, "leftover.txt"),
		[]byte("<<<<<<< ours\nA\n=======\nB\n>>>>>>> theirs\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	results, err := RunReconcile(context.Background(), s, []string{"add-note"}, "HEAD", nil, provider.Config{}, ReconcileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if results[0].Outcome != store.ReconcileBlocked {
		t.Fatalf("expected blocked due to live conflict markers, got %s (phase=%s, notes=%v)",
			results[0].Outcome, results[0].Phase, results[0].Notes)
	}
	if results[0].Phase != "phase-4-live-conflict-markers" {
		t.Fatalf("expected phase-4-live-conflict-markers, got %s", results[0].Phase)
	}
	foundLeftover := false
	for _, f := range results[0].Conflicts {
		if f == "leftover.txt" {
			foundLeftover = true
		}
	}
	if !foundLeftover {
		t.Fatalf("expected leftover.txt in conflicts list, got %v", results[0].Conflicts)
	}
}

func TestReconcilePhase3_ProviderAssistedUpstreamed(t *testing.T) {
	tmpDir := t.TempDir()
	setupGitRepo(t, tmpDir)

	s, err := store.Init(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	s.AddFeature(store.AddFeatureInput{Title: "Fix model translation", Request: "Fix model ID translation"})
	s.MarkFeatureState("fix-model-translation", store.StateApplied, "apply", "")

	// Write spec with acceptance criteria
	s.WriteFeatureFile("fix-model-translation", "spec.md", "# Spec\n\n## Acceptance Criteria\n\n1. Model IDs are translated correctly\n")

	// Create a patch that won't reverse-apply or forward-apply (simulating structural difference)
	patch := `diff --git a/nonexistent.ts b/nonexistent.ts
--- a/nonexistent.ts
+++ b/nonexistent.ts
@@ -1,3 +1,5 @@
 old content
+new translation code
+more code
 existing
`
	s.WriteArtifact("fix-model-translation", "post-apply.patch", patch)

	// Mock provider that responds "upstreamed"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/chat/completions" {
			json.NewEncoder(w).Encode(map[string]any{
				"choices": []map[string]any{
					{"message": map[string]string{
						"content": `{"decision": "upstreamed", "reasoning": "Upstream has equivalent implementation"}`,
					}},
				},
			})
			return
		}
		w.WriteHeader(404)
	}))
	defer srv.Close()

	prov := provider.New()
	cfg := provider.Config{BaseURL: srv.URL, Model: "test-model"}

	results, err := RunReconcile(context.Background(), s, []string{"fix-model-translation"}, "HEAD", prov, cfg, ReconcileOptions{})
	if err != nil {
		t.Fatal(err)
	}

	if results[0].Outcome != store.ReconcileUpstreamed {
		t.Fatalf("expected upstreamed, got %s", results[0].Outcome)
	}
	if results[0].Phase != "phase-3-provider-semantic" {
		t.Fatalf("expected phase-3, got %s", results[0].Phase)
	}
}

func TestReconcileBlocked(t *testing.T) {
	tmpDir := t.TempDir()
	setupGitRepo(t, tmpDir)

	s, err := store.Init(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	s.AddFeature(store.AddFeatureInput{Title: "Conflicting change", Request: "A change that conflicts"})
	s.MarkFeatureState("conflicting-change", store.StateApplied, "apply", "")

	// Create a patch that modifies a non-existent file in a conflicting way
	patch := `diff --git a/src/main.ts b/src/main.ts
--- a/src/main.ts
+++ b/src/main.ts
@@ -10,6 +10,8 @@
 import { something } from './lib';
+import { newThing } from './new-module';
 
 function main() {
+  newThing();
   something();
 }
`
	s.WriteArtifact("conflicting-change", "post-apply.patch", patch)

	// No provider → phases 1-2 fail, phase 4 fails → BLOCKED
	results, err := RunReconcile(context.Background(), s, []string{"conflicting-change"}, "HEAD", nil, provider.Config{}, ReconcileOptions{})
	if err != nil {
		t.Fatal(err)
	}

	if results[0].Outcome != store.ReconcileBlocked {
		t.Fatalf("expected blocked, got %s", results[0].Outcome)
	}
}

// TestReconcilePhase35_NoProviderBlocks verifies that --resolve without
// a configured provider returns blocked-requires-human rather than
// silently falling back to heuristics (ADR-010 D9).
func TestReconcilePhase35_NoProviderBlocks(t *testing.T) {
	tmpDir := t.TempDir()
	setupGitRepo(t, tmpDir)

	// Baseline commit: shared.txt with a middle line to conflict on.
	if err := os.WriteFile(filepath.Join(tmpDir, "shared.txt"), []byte("a\nb\nc\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitAdd(t, tmpDir, "shared.txt")
	gitCommit(t, tmpDir, "add shared")

	// Feature-applied state: line 2 -> B-local. Stage then capture
	// a real git diff (includes index/blob refs so --3way can locate
	// the base blob), then commit.
	if err := os.WriteFile(filepath.Join(tmpDir, "shared.txt"), []byte("a\nB-local\nc\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitAdd(t, tmpDir, "shared.txt")
	diffCmd := exec.Command("git", "diff", "--cached", "HEAD")
	diffCmd.Dir = tmpDir
	patchBytes, dErr := diffCmd.Output()
	if dErr != nil {
		t.Fatalf("git diff: %v", dErr)
	}
	patch := string(patchBytes)
	gitCommit(t, tmpDir, "feature applied")

	// Upstream diverges: line 2 -> B-upstream. Same line conflicts.
	if err := os.WriteFile(filepath.Join(tmpDir, "shared.txt"), []byte("a\nB-upstream\nc\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitAdd(t, tmpDir, "shared.txt")
	gitCommit(t, tmpDir, "upstream diverges")

	s, err := store.Init(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	s.AddFeature(store.AddFeatureInput{Title: "Feature", Request: "r"})
	s.MarkFeatureState("feature", store.StateApplied, "apply", "")
	s.WriteArtifact("feature", "post-apply.patch", patch)

	results, err := RunReconcile(context.Background(), s, []string{"feature"}, "HEAD", nil, provider.Config{},
		ReconcileOptions{Resolve: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Outcome != store.ReconcileBlockedRequiresHuman {
		t.Fatalf("expected blocked-requires-human, got %s (phase=%s notes=%v)",
			results[0].Outcome, results[0].Phase, results[0].Notes)
	}
	if results[0].Phase != "phase-3.5-provider-resolve" {
		t.Fatalf("expected phase 3.5, got %s", results[0].Phase)
	}
}

// TestReconcilePhase35_StatusSummary verifies that blocked-requires-human
// populates the ReconcileSummary with the phase-3.5 bookkeeping fields
// (session id, shadow path) where applicable, and that counts are
// zeroed when the resolver short-circuits on "no provider".
func TestReconcilePhase35_StatusSummary(t *testing.T) {
	tmpDir := t.TempDir()
	setupGitRepo(t, tmpDir)
	os.WriteFile(filepath.Join(tmpDir, "shared.txt"), []byte("a\nb\nc\n"), 0o644)
	gitAdd(t, tmpDir, "shared.txt")
	gitCommit(t, tmpDir, "add shared")
	os.WriteFile(filepath.Join(tmpDir, "shared.txt"), []byte("a\nB-local\nc\n"), 0o644)
	gitAdd(t, tmpDir, "shared.txt")
	diffCmd := exec.Command("git", "diff", "--cached", "HEAD")
	diffCmd.Dir = tmpDir
	patchBytes, _ := diffCmd.Output()
	patch := string(patchBytes)
	gitCommit(t, tmpDir, "feature applied")
	os.WriteFile(filepath.Join(tmpDir, "shared.txt"), []byte("a\nB-upstream\nc\n"), 0o644)
	gitAdd(t, tmpDir, "shared.txt")
	gitCommit(t, tmpDir, "upstream diverges")

	s, _ := store.Init(tmpDir)
	s.AddFeature(store.AddFeatureInput{Title: "Feature", Request: "r"})
	s.MarkFeatureState("feature", store.StateApplied, "apply", "")
	s.WriteArtifact("feature", "post-apply.patch", patch)

	_, err := RunReconcile(context.Background(), s, []string{"feature"}, "HEAD", nil, provider.Config{},
		ReconcileOptions{Resolve: true})
	if err != nil {
		t.Fatal(err)
	}

	st, err := s.LoadFeatureStatus("feature")
	if err != nil {
		t.Fatal(err)
	}
	if st.State != store.StateBlocked {
		t.Fatalf("expected state blocked (no-provider short-circuit), got %s", st.State)
	}
	if st.Reconcile.Outcome != store.ReconcileBlockedRequiresHuman {
		t.Fatalf("expected outcome blocked-requires-human, got %s", st.Reconcile.Outcome)
	}
	// No shadow was created because the resolver never ran (pre-flight
	// rejection path). These fields must remain zero — otherwise a
	// fresh `tpatch status` would print a dangling shadow pointer.
	if st.Reconcile.ShadowPath != "" {
		t.Errorf("expected empty ShadowPath, got %q", st.Reconcile.ShadowPath)
	}
	if st.Reconcile.ResolveSession != "" {
		t.Errorf("expected empty ResolveSession, got %q", st.Reconcile.ResolveSession)
	}
	if st.Reconcile.ResolvedFiles != 0 || st.Reconcile.FailedFiles != 0 || st.Reconcile.SkippedFiles != 0 {
		t.Errorf("expected all file counts zero, got %d/%d/%d",
			st.Reconcile.ResolvedFiles, st.Reconcile.FailedFiles, st.Reconcile.SkippedFiles)
	}
}
