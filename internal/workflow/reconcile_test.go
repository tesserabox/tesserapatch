package workflow

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/tesserabox/tesserapatch/internal/provider"
	"github.com/tesserabox/tesserapatch/internal/store"
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
	results, err := RunReconcile(context.Background(), s, []string{"add-greeting"}, "HEAD", nil, provider.Config{})
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
	results, err := RunReconcile(context.Background(), s, []string{"add-models-command"}, "HEAD", nil, provider.Config{})
	if err != nil {
		t.Fatal(err)
	}

	if results[0].Outcome != store.ReconcileReapplied {
		t.Fatalf("expected reapplied, got %s", results[0].Outcome)
	}
	if results[0].Phase != "phase-4-forward-apply" {
		t.Fatalf("expected phase-4, got %s", results[0].Phase)
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

	results, err := RunReconcile(context.Background(), s, []string{"fix-model-translation"}, "HEAD", prov, cfg)
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
	results, err := RunReconcile(context.Background(), s, []string{"conflicting-change"}, "HEAD", nil, provider.Config{})
	if err != nil {
		t.Fatal(err)
	}

	if results[0].Outcome != store.ReconcileBlocked {
		t.Fatalf("expected blocked, got %s", results[0].Outcome)
	}
}
