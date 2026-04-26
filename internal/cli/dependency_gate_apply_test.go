package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tesseracode/tesserapatch/internal/store"
)

// TestApplyExecute_BlockedByHardDep_FlagOn asserts that, when the
// features_dependencies flag is enabled, `tpatch apply --mode execute`
// on a feature with an unsatisfied hard parent fails fast with the
// parent slug surfaced on stderr, and does NOT mutate working-tree files.
// (M14.2, ADR-011 D4.)
func TestApplyExecute_BlockedByHardDep_FlagOn(t *testing.T) {
	tmpDir := t.TempDir()
	gitInitTestRepo(t, tmpDir)
	runCmd("init", "--path", tmpDir)
	runCmd("add", "--path", tmpDir, "Parent feature")
	runCmd("add", "--path", tmpDir, "Child feature")
	parentSlug := "parent-feature"
	childSlug := "child-feature"

	s, err := store.Open(tmpDir)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	cfg, _ := s.LoadConfig()
	cfg.FeaturesDependencies = true
	if err := s.SaveConfig(cfg); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	parent, _ := s.LoadFeatureStatus(parentSlug)
	parent.State = store.StateAnalyzed
	s.SaveFeatureStatus(parent)

	child, _ := s.LoadFeatureStatus(childSlug)
	child.State = store.StateImplementing
	child.DependsOn = []store.Dependency{
		{Slug: parentSlug, Kind: store.DependencyKindHard},
	}
	s.SaveFeatureStatus(child)

	// Seed a recipe that would write a file if the gate were skipped.
	artDir := filepath.Join(tmpDir, ".tpatch", "features", childSlug, "artifacts")
	os.MkdirAll(artDir, 0o755)
	recipe := `{
  "feature": "` + childSlug + `",
  "operations": [
    {"type": "write-file", "path": "should-not-exist.txt", "content": "x\n"}
  ]
}
`
	if err := os.WriteFile(filepath.Join(artDir, "apply-recipe.json"), []byte(recipe), 0o644); err != nil {
		t.Fatal(err)
	}

	_, stderr, code := runCmd("apply", "--path", tmpDir, childSlug, "--mode", "execute")
	if code == 0 {
		t.Fatalf("expected non-zero exit when hard parent blocks apply; stderr=%q", stderr)
	}
	if !strings.Contains(stderr, parentSlug) {
		t.Errorf("stderr must name blocking parent %q; got %q", parentSlug, stderr)
	}
	if !strings.Contains(stderr, "hard parent dependency not applied") {
		t.Errorf("stderr must explain the gate; got %q", stderr)
	}
	if _, err := os.Stat(filepath.Join(tmpDir, "should-not-exist.txt")); err == nil {
		t.Errorf("file mutation occurred despite gate failure")
	}
	// Child state must be unchanged.
	after, _ := s.LoadFeatureStatus(childSlug)
	if after.State != store.StateImplementing {
		t.Errorf("child state mutated to %q; expected unchanged %q", after.State, store.StateImplementing)
	}
	// Sanity: status.json still parses.
	raw, _ := os.ReadFile(filepath.Join(tmpDir, ".tpatch", "features", childSlug, "status.json"))
	var fs store.FeatureStatus
	if err := json.Unmarshal(raw, &fs); err != nil {
		t.Errorf("status.json corrupted after blocked apply: %v", err)
	}
}

// TestApplyExecute_FlagOff_BypassesDependencyGate verifies that with the
// flag OFF, the gate is a strict no-op even when a hard parent is
// unsatisfied — preserving v0.5.3 behaviour (ADR-011 D9).
func TestApplyExecute_FlagOff_BypassesDependencyGate(t *testing.T) {
	tmpDir := t.TempDir()
	gitInitTestRepo(t, tmpDir)
	runCmd("init", "--path", tmpDir)
	runCmd("add", "--path", tmpDir, "Parent feature")
	runCmd("add", "--path", tmpDir, "Child feature")
	parentSlug := "parent-feature"
	childSlug := "child-feature"

	s, err := store.Open(tmpDir)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	// FeaturesDependencies stays false (default).
	parent, _ := s.LoadFeatureStatus(parentSlug)
	parent.State = store.StateAnalyzed
	s.SaveFeatureStatus(parent)

	child, _ := s.LoadFeatureStatus(childSlug)
	child.State = store.StateImplementing
	child.DependsOn = []store.Dependency{
		{Slug: parentSlug, Kind: store.DependencyKindHard},
	}
	s.SaveFeatureStatus(child)

	artDir := filepath.Join(tmpDir, ".tpatch", "features", childSlug, "artifacts")
	os.MkdirAll(artDir, 0o755)
	recipe := `{
  "feature": "` + childSlug + `",
  "operations": [
    {"type": "write-file", "path": "ok.txt", "content": "x\n"}
  ]
}
`
	os.WriteFile(filepath.Join(artDir, "apply-recipe.json"), []byte(recipe), 0o644)

	_, stderr, code := runCmd("apply", "--path", tmpDir, childSlug, "--mode", "execute")
	if code != 0 {
		t.Fatalf("flag-off apply must proceed; stderr=%q", stderr)
	}
	if _, err := os.Stat(filepath.Join(tmpDir, "ok.txt")); err != nil {
		t.Errorf("expected file written when flag is off: %v", err)
	}
}
