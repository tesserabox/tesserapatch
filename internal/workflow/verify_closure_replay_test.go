package workflow

// V7/V8 hard-parent topological closure replay tests (PRD-verify-freshness
// §3.4.3 + §9 Slice C). Three fixtures are required by the slice spec:
//
//   - 3DeepDAG_Happy           — 3-deep hard-parent chain, all applied,
//                                target verify passes V7+V8.
//   - ParentFailMidClosure_FailFast — middle parent's recipe replay errors;
//                                expect failed_at=parent-replay,
//                                parent_slug set, V7 fail, V8 skip.
//   - UpstreamMergedParentSkipped  — middle parent in upstream_merged is
//                                skipped without error; downstream replay
//                                continues from baseline.
//
// The closure-replay primitive lives in verify.go only (ADR-010 D2).

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tesseracode/tesserapatch/internal/store"
)

// addChain wires a hard-parent chain root → mid → leaf. Each parent is
// in `applied` state by default with a write-file recipe that creates a
// distinct file. Returns the slugs in [root, mid, leaf] order.
func addChain(t *testing.T, s *store.Store) (root, mid, leaf string) {
	t.Helper()
	root = "root"
	mid = "mid"
	leaf = "leaf"

	// root: writes file A.
	setApplied(t, s, root, ApplyRecipe{Operations: []RecipeOperation{
		{Type: "write-file", Path: "src/root.txt", Content: "root\n"},
	}})

	// mid: writes file B; depends_on root (hard).
	setApplied(t, s, mid, ApplyRecipe{Operations: []RecipeOperation{
		{Type: "write-file", Path: "src/mid.txt", Content: "mid\n"},
	}})
	setHardDeps(t, s, mid, []string{root})

	// leaf: writes file C; depends_on mid (hard).
	setApplied(t, s, leaf, ApplyRecipe{Operations: []RecipeOperation{
		{Type: "write-file", Path: "src/leaf.txt", Content: "leaf\n"},
	}})
	setHardDeps(t, s, leaf, []string{mid})

	return root, mid, leaf
}

func setHardDeps(t *testing.T, s *store.Store, slug string, parents []string) {
	t.Helper()
	st, err := s.LoadFeatureStatus(slug)
	if err != nil {
		t.Fatalf("load %s: %v", slug, err)
	}
	st.DependsOn = nil
	for _, p := range parents {
		st.DependsOn = append(st.DependsOn, store.Dependency{Slug: p, Kind: store.DependencyKindHard})
	}
	if err := s.SaveFeatureStatus(st); err != nil {
		t.Fatalf("save %s: %v", slug, err)
	}
}

func writeIntent(t *testing.T, s *store.Store, slug string) {
	t.Helper()
	if err := s.WriteFeatureFile(slug, "spec.md", "spec for "+slug); err != nil {
		t.Fatalf("spec %s: %v", slug, err)
	}
	if err := s.WriteFeatureFile(slug, "exploration.md", "exploration for "+slug); err != nil {
		t.Fatalf("exploration %s: %v", slug, err)
	}
}

// TestRunVerify_ClosureReplay_3DeepDAG_Happy is the §3.4.3 happy path:
// root → mid → leaf, all applied with write-file recipes. Verifying
// the leaf must replay root then mid then leaf in the shadow and pass
// V7 cleanly (V8 skipped because no post-apply.patch is on disk for
// the leaf).
func TestRunVerify_ClosureReplay_3DeepDAG_Happy(t *testing.T) {
	s := setupVerifyFeature(t, "scratch")
	// scratch was the placeholder feature for the gitInit'd repo; we
	// don't actually use it. Build the chain on the same store.
	root, mid, leaf := addChain(t, s)
	_ = root
	_ = mid

	// Intent files for the leaf so V1 passes.
	writeIntent(t, s, leaf)
	writeVerifyRecipe(t, s, leaf, ApplyRecipe{Feature: leaf, Operations: []RecipeOperation{
		{Type: "write-file", Path: "src/leaf-final.txt", Content: "final\n"},
	}})

	report, err := RunVerify(s, leaf, VerifyOptions{NoWrite: true})
	if err != nil {
		t.Fatalf("RunVerify: %v", err)
	}

	v7 := findCheck(t, report, CheckRecipeReplayClean)
	if !v7.Passed || v7.Skipped {
		t.Errorf("V7 should pass on 3-deep happy DAG; got %+v", v7)
	}
	v8 := findCheck(t, report, CheckPostApplyPatchReplayClean)
	// No post-apply.patch on disk → V8 must be skipped.
	if !v8.Passed || !v8.Skipped {
		t.Errorf("V8 should be skipped without post-apply.patch; got %+v", v8)
	}
	if report.FailedAt != "" {
		t.Errorf("happy path should not set FailedAt; got %q", report.FailedAt)
	}
}

// TestRunVerify_ClosureReplay_ParentFailMidClosure_FailFast asserts the
// fail-fast contract: the middle parent's recipe references a search
// string that doesn't exist in the shadow → replayRecipeOpsInShadow
// returns an error → V7 fails with "hard parent <slug> failed to replay
// in shadow: …" and `failed_at: "parent-replay"` is set. V8 is skipped.
func TestRunVerify_ClosureReplay_ParentFailMidClosure_FailFast(t *testing.T) {
	s := setupVerifyFeature(t, "scratch")

	// root: clean write-file.
	setApplied(t, s, "root", ApplyRecipe{Operations: []RecipeOperation{
		{Type: "write-file", Path: "src/root.txt", Content: "root\n"},
	}})

	// mid: a `replace-in-file` against a path that doesn't exist in the
	// shadow → replay error.
	setApplied(t, s, "mid", ApplyRecipe{Operations: []RecipeOperation{
		{Type: "replace-in-file", Path: "src/non-existent.txt", Search: "x", Replace: "y"},
	}})
	setHardDeps(t, s, "mid", []string{"root"})

	// leaf: depends on mid.
	setApplied(t, s, "leaf", ApplyRecipe{Operations: []RecipeOperation{
		{Type: "write-file", Path: "src/leaf.txt", Content: "leaf\n"},
	}})
	setHardDeps(t, s, "leaf", []string{"mid"})

	writeIntent(t, s, "leaf")
	writeVerifyRecipe(t, s, "leaf", ApplyRecipe{Feature: "leaf", Operations: []RecipeOperation{
		{Type: "write-file", Path: "src/leaf-final.txt", Content: "final\n"},
	}})

	report, err := RunVerify(s, "leaf", VerifyOptions{NoWrite: true})
	if err != nil {
		t.Fatalf("RunVerify: %v", err)
	}

	v7 := findCheck(t, report, CheckRecipeReplayClean)
	if v7.Passed {
		t.Errorf("V7 must fail when mid parent fails to replay; got %+v", v7)
	}
	if !strings.Contains(v7.Remediation, "hard parent mid failed to replay in shadow") {
		t.Errorf("V7 remediation must follow PRD §3.4.3 verbatim parent-replay form; got %q", v7.Remediation)
	}
	if !strings.Contains(v7.Remediation, "re-run tpatch verify mid on the parent first") {
		t.Errorf("V7 remediation must include the parent re-verify hint verbatim; got %q", v7.Remediation)
	}

	v8 := findCheck(t, report, CheckPostApplyPatchReplayClean)
	if !v8.Skipped {
		t.Errorf("V8 must be skipped on V7 parent-replay failure; got %+v", v8)
	}

	if report.FailedAt != "parent-replay" {
		t.Errorf("FailedAt must be 'parent-replay'; got %q", report.FailedAt)
	}
	if report.ParentSlug != "mid" {
		t.Errorf("ParentSlug must be 'mid'; got %q", report.ParentSlug)
	}
}

// TestRunVerify_ClosureReplay_UpstreamMergedParentSkipped: when a middle
// parent is in upstream_merged its recipe is NOT replayed (changes are
// already in the baseline). Downstream replay continues normally.
func TestRunVerify_ClosureReplay_UpstreamMergedParentSkipped(t *testing.T) {
	s := setupVerifyFeature(t, "scratch")

	setApplied(t, s, "root", ApplyRecipe{Operations: []RecipeOperation{
		{Type: "write-file", Path: "src/root.txt", Content: "root\n"},
	}})

	// mid in upstream_merged. Its recipe — if executed — would error
	// (replace against a non-existent file). Skipping it is the only
	// way the run passes.
	setApplied(t, s, "mid", ApplyRecipe{Operations: []RecipeOperation{
		{Type: "replace-in-file", Path: "src/never-exists.txt", Search: "a", Replace: "b"},
	}})
	if err := s.MarkFeatureState("mid", store.StateUpstreamMerged, "test", ""); err != nil {
		t.Fatal(err)
	}
	setHardDeps(t, s, "mid", []string{"root"})

	setApplied(t, s, "leaf", ApplyRecipe{Operations: []RecipeOperation{
		{Type: "write-file", Path: "src/leaf.txt", Content: "leaf\n"},
	}})
	setHardDeps(t, s, "leaf", []string{"mid"})

	writeIntent(t, s, "leaf")
	writeVerifyRecipe(t, s, "leaf", ApplyRecipe{Feature: "leaf", Operations: []RecipeOperation{
		{Type: "write-file", Path: "src/leaf-final.txt", Content: "final\n"},
	}})

	report, err := RunVerify(s, "leaf", VerifyOptions{NoWrite: true})
	if err != nil {
		t.Fatalf("RunVerify: %v", err)
	}

	v7 := findCheck(t, report, CheckRecipeReplayClean)
	if !v7.Passed || v7.Skipped {
		t.Errorf("V7 must pass when middle upstream_merged parent is skipped; got %+v", v7)
	}
	if report.FailedAt != "" {
		t.Errorf("FailedAt must be empty on a clean upstream_merged-skip path; got %q", report.FailedAt)
	}
}

// TestRunVerify_ClosureReplay_PrunesShadowOnExit pins ADR-013 D7: the
// shadow allocated for V7/V8 must be pruned before verify returns,
// regardless of pass/fail. We call RunVerify on a 3-deep DAG that
// passes V7, then assert no shadow lingers under .tpatch/shadow/.
func TestRunVerify_ClosureReplay_PrunesShadowOnExit(t *testing.T) {
	s := setupVerifyFeature(t, "scratch")
	root, mid, leaf := addChain(t, s)
	_ = root
	_ = mid

	writeIntent(t, s, leaf)
	writeVerifyRecipe(t, s, leaf, ApplyRecipe{Feature: leaf, Operations: []RecipeOperation{
		{Type: "write-file", Path: "src/final.txt", Content: "x\n"},
	}})

	if _, err := RunVerify(s, leaf, VerifyOptions{NoWrite: true}); err != nil {
		t.Fatal(err)
	}

	// .tpatch/shadow/<leaf>-* should be gone.
	assertNoShadowFor(t, s, leaf)
}

// assertNoShadowFor fails the test if any shadow directory for slug
// still exists under .tpatch/shadow/.
func assertNoShadowFor(t *testing.T, s *store.Store, slug string) {
	t.Helper()
	shadowDir := filepath.Join(s.Root, ".tpatch", "shadow")
	entries, err := os.ReadDir(shadowDir)
	if err != nil {
		if os.IsNotExist(err) {
			return
		}
		t.Fatalf("read shadow dir: %v", err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), slug+"-") {
			t.Errorf("shadow not pruned: %s", e.Name())
		}
	}
}
