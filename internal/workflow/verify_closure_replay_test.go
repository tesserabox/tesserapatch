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

// ── Slice C revision-1: recipe-absent + patch-present matrix ───────────
//
// PRD-verify-freshness §5 (line 524) edge case: when apply-recipe.json
// is absent BUT post-apply.patch is present, V7 is skipped (recipe
// precondition not met) yet V8 must still run against the closure-
// replayed baseline. The original Slice C wiring short-circuited BOTH
// V7 and V8 on missing recipe, masking patch drift. These four tests
// pin the four matrix cells of recipe×patch ∈ {present, absent}² plus
// the parent-replay fail-fast interaction when recipe is absent.

// validNewFilePatch is a minimal, hand-rolled unified diff that creates
// a new top-level file. It applies cleanly to any baseline that does
// not already have rev1-added.txt.
const validNewFilePatch = `diff --git a/rev1-added.txt b/rev1-added.txt
new file mode 100644
--- /dev/null
+++ b/rev1-added.txt
@@ -0,0 +1 @@
+rev1
`

// TestRunVerify_RecipeAbsent_PatchPresent_V8RunsAgainstClosureBaseline
// is the happy-path matrix cell: no recipe, valid patch, no parents.
// V7 must be skipped (recipe absent), V8 must pass (patch applies).
func TestRunVerify_RecipeAbsent_PatchPresent_V8RunsAgainstClosureBaseline(t *testing.T) {
	slug := "rev1-happy"
	s := setupVerifyFeature(t, slug)
	writeIntent(t, s, slug)
	if err := s.WriteArtifact(slug, "post-apply.patch", validNewFilePatch); err != nil {
		t.Fatalf("write patch: %v", err)
	}

	report, err := RunVerify(s, slug, VerifyOptions{NoWrite: true})
	if err != nil {
		t.Fatalf("RunVerify: %v", err)
	}

	v7 := findCheck(t, report, CheckRecipeReplayClean)
	if !v7.Passed || !v7.Skipped {
		t.Errorf("V7 must be skipped when recipe absent; got %+v", v7)
	}
	if !strings.Contains(v7.Reason, "no apply-recipe.json") {
		t.Errorf("V7 skip reason must cite missing recipe; got %q", v7.Reason)
	}
	v8 := findCheck(t, report, CheckPostApplyPatchReplayClean)
	if !v8.Passed || v8.Skipped {
		t.Errorf("V8 must run and pass with valid patch + closure-replayed baseline; got %+v", v8)
	}
	if report.Verdict != "passed" {
		t.Errorf("verdict must be passed; got %q", report.Verdict)
	}
	assertNoShadowFor(t, s, slug)
}

// TestRunVerify_RecipeAbsent_PatchPresent_V8FailsOnInvalidPatch is the
// REGRESSION TEST for the supervisor's HIGH finding: with no recipe
// and an INVALID patch, Slice C as shipped reported V8 as
// {passed:true, skipped:true} and overall verdict=passed. The fix
// must produce V8 fail with the verbatim PRD §3.1.2 remediation and
// verdict=failed.
func TestRunVerify_RecipeAbsent_PatchPresent_V8FailsOnInvalidPatch(t *testing.T) {
	slug := "rev1-bug"
	s := setupVerifyFeature(t, slug)
	writeIntent(t, s, slug)
	if err := s.WriteArtifact(slug, "post-apply.patch", "this is not a valid patch\n"); err != nil {
		t.Fatalf("write patch: %v", err)
	}

	report, err := RunVerify(s, slug, VerifyOptions{NoWrite: true})
	if err != nil {
		t.Fatalf("RunVerify: %v", err)
	}

	v7 := findCheck(t, report, CheckRecipeReplayClean)
	if !v7.Passed || !v7.Skipped {
		t.Errorf("V7 must be skipped (recipe absent); got %+v", v7)
	}
	v8 := findCheck(t, report, CheckPostApplyPatchReplayClean)
	if v8.Passed || v8.Skipped {
		t.Fatalf("V8 must FAIL on invalid patch (the bug-repro path); got %+v", v8)
	}
	wantRem := "post-apply.patch no longer applies to closure-replayed baseline; run tpatch reconcile " + slug
	if v8.Remediation != wantRem {
		t.Errorf("V8 remediation must be PRD §3.1.2 verbatim\n want: %q\n  got: %q", wantRem, v8.Remediation)
	}
	if report.Verdict != "failed" {
		t.Errorf("verdict must be failed; got %q", report.Verdict)
	}
	assertNoShadowFor(t, s, slug)
}

// TestRunVerify_RecipeAbsent_PatchAbsent_BothSkipped pins PRD §5
// line 526: when BOTH artifacts are absent, V7 and V8 are skipped and
// no shadow is allocated.
func TestRunVerify_RecipeAbsent_PatchAbsent_BothSkipped(t *testing.T) {
	slug := "rev1-both-absent"
	s := setupVerifyFeature(t, slug)
	writeIntent(t, s, slug)

	report, err := RunVerify(s, slug, VerifyOptions{NoWrite: true})
	if err != nil {
		t.Fatalf("RunVerify: %v", err)
	}

	v7 := findCheck(t, report, CheckRecipeReplayClean)
	if !v7.Passed || !v7.Skipped {
		t.Errorf("V7 must be skipped; got %+v", v7)
	}
	v8 := findCheck(t, report, CheckPostApplyPatchReplayClean)
	if !v8.Passed || !v8.Skipped {
		t.Errorf("V8 must be skipped; got %+v", v8)
	}
	if report.Verdict != "passed" {
		t.Errorf("verdict must be passed (warn-only V6 is fine); got %q", report.Verdict)
	}
	// Shadow must not have been allocated when both artifacts are
	// absent. The shadow dir may not exist at all — both states
	// (absent dir, empty dir) satisfy the contract.
	shadowDir := filepath.Join(s.Root, ".tpatch", "shadow")
	if entries, err := os.ReadDir(shadowDir); err == nil {
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), slug+"-") {
				t.Errorf("shadow allocated when both artifacts absent: %s", e.Name())
			}
		}
	}
}

// TestRunVerify_RecipeAbsent_PatchPresent_ParentReplayFailFast asserts
// that the parent-replay fail-fast still fires when recipe is absent.
// V7 must carry the parent-replay remediation (PRD §3.4.3 verbatim
// form); V8 must be skipped with the PRD §4.3.5 reason
// "skipped: parent-replay aborted before V8".
func TestRunVerify_RecipeAbsent_PatchPresent_ParentReplayFailFast(t *testing.T) {
	slug := "rev1-parentfail"
	s := setupVerifyFeature(t, slug)
	writeIntent(t, s, slug)
	if err := s.WriteArtifact(slug, "post-apply.patch", validNewFilePatch); err != nil {
		t.Fatalf("write patch: %v", err)
	}

	// Hard parent in `analyzed` state — non-replayable per
	// runClosureReplay's parent-state switch (only applied or
	// upstream_merged are accepted).
	if _, err := s.AddFeature(store.AddFeatureInput{Title: "stuck-parent", Slug: "stuck-parent", Request: "x"}); err != nil {
		t.Fatalf("AddFeature parent: %v", err)
	}
	if err := s.MarkFeatureState("stuck-parent", store.StateAnalyzed, "analyze", ""); err != nil {
		t.Fatalf("MarkFeatureState parent: %v", err)
	}
	setHardDeps(t, s, slug, []string{"stuck-parent"})

	report, err := RunVerify(s, slug, VerifyOptions{NoWrite: true})
	if err != nil {
		t.Fatalf("RunVerify: %v", err)
	}

	v7 := findCheck(t, report, CheckRecipeReplayClean)
	if v7.Passed {
		t.Errorf("V7 must fail on parent-replay even when target recipe is absent; got %+v", v7)
	}
	if !strings.Contains(v7.Remediation, "hard parent stuck-parent failed to replay in shadow") {
		t.Errorf("V7 must use parent-replay remediation form; got %q", v7.Remediation)
	}
	v8 := findCheck(t, report, CheckPostApplyPatchReplayClean)
	if !v8.Skipped {
		t.Errorf("V8 must be skipped on parent-replay fail; got %+v", v8)
	}
	if v8.Reason != "skipped: parent-replay aborted before V8" {
		t.Errorf("V8 skip reason must be PRD §4.3.5 verbatim; got %q", v8.Reason)
	}
	if report.FailedAt != "parent-replay" {
		t.Errorf("FailedAt must be 'parent-replay'; got %q", report.FailedAt)
	}
	if report.ParentSlug != "stuck-parent" {
		t.Errorf("ParentSlug must be 'stuck-parent'; got %q", report.ParentSlug)
	}
	if report.Verdict != "failed" {
		t.Errorf("verdict must be failed; got %q", report.Verdict)
	}
	assertNoShadowFor(t, s, slug)
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
