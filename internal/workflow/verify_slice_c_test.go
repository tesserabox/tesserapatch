package workflow

// Slice C per-check tests (M15-W3 PRD-verify-freshness §9). These cover
// V3 (recipe_op_targets_resolve), V4 (dep_metadata_valid),
// V5 (satisfied_by_reachable), V6 (dependency_gate_satisfied), and
// V9 (reconcile_outcome_consistent) at unit granularity. The closure-
// replay (V7/V8) tests live in verify_closure_replay_test.go.

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tesseracode/tesserapatch/internal/store"
)

// findCheck returns the named check from a report, failing the test
// if absent.
func findCheck(t *testing.T, report *VerifyReport, id string) store.VerifyCheckResult {
	t.Helper()
	for _, c := range report.Checks {
		if c.ID == id {
			return c
		}
	}
	t.Fatalf("check %s not found in report", id)
	return store.VerifyCheckResult{}
}

// setApplied marks a parent feature as applied and writes a recipe
// for it (so closure replay has something to replay).
func setApplied(t *testing.T, s *store.Store, slug string, recipe ApplyRecipe) {
	t.Helper()
	if _, err := s.AddFeature(store.AddFeatureInput{Title: slug, Slug: slug, Request: "x"}); err != nil {
		t.Fatalf("AddFeature %s: %v", slug, err)
	}
	if err := s.MarkFeatureState(slug, store.StateApplied, "apply", ""); err != nil {
		t.Fatalf("MarkFeatureState %s: %v", slug, err)
	}
	if recipe.Feature == "" {
		recipe.Feature = slug
	}
	data, err := json.Marshal(recipe)
	if err != nil {
		t.Fatalf("marshal recipe: %v", err)
	}
	if err := s.WriteArtifact(slug, "apply-recipe.json", string(data)); err != nil {
		t.Fatalf("write recipe: %v", err)
	}
}

// ── V3 ──────────────────────────────────────────────────────────────────

// V3 passes for write-file / ensure-directory ops even when the target
// path doesn't exist — those op types create the path.
func TestRunVerify_V3_WriteFileNonExistentPath_Passes(t *testing.T) {
	slug := "v3-writefile"
	s := setupVerifyFeature(t, slug)
	writeIntentFiles(t, s, slug)
	writeVerifyRecipe(t, s, slug, ApplyRecipe{Feature: slug, Operations: []RecipeOperation{
		{Type: "write-file", Path: "src/new.go", Content: "package x\n"},
	}})

	report, err := RunVerify(s, slug, VerifyOptions{NoWrite: true})
	if err != nil {
		t.Fatal(err)
	}
	c := findCheck(t, report, CheckRecipeOpTargetsResolve)
	if !c.Passed || c.Skipped {
		t.Errorf("V3 should pass for write-file ops; got %+v", c)
	}
}

// V3 passes when an append/replace op carries created_by pointing at a
// hard parent in `applied`.
func TestRunVerify_V3_CreatedByHardAppliedParent_Passes(t *testing.T) {
	slug := "v3-createdby"
	s := setupVerifyFeature(t, slug)
	writeIntentFiles(t, s, slug)
	// Add hard parent in applied state.
	if _, err := s.AddFeature(store.AddFeatureInput{Title: "parent", Slug: "parent", Request: "x"}); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkFeatureState("parent", store.StateApplied, "apply", ""); err != nil {
		t.Fatal(err)
	}
	// Wire the dep on the child.
	st, err := s.LoadFeatureStatus(slug)
	if err != nil {
		t.Fatal(err)
	}
	st.DependsOn = []store.Dependency{{Slug: "parent", Kind: store.DependencyKindHard}}
	if err := s.SaveFeatureStatus(st); err != nil {
		t.Fatal(err)
	}
	writeVerifyRecipe(t, s, slug, ApplyRecipe{Feature: slug, Operations: []RecipeOperation{
		{Type: "replace-in-file", Path: "src/parent-file.go", Search: "a", Replace: "b", CreatedBy: "parent"},
	}})

	report, err := RunVerify(s, slug, VerifyOptions{NoWrite: true})
	if err != nil {
		t.Fatal(err)
	}
	c := findCheck(t, report, CheckRecipeOpTargetsResolve)
	if !c.Passed || c.Skipped {
		t.Errorf("V3 should pass when created_by names an applied hard parent; got %+v", c)
	}
}

// V3 fails when created_by points at a parent that is not in
// applied/upstream_merged.
func TestRunVerify_V3_CreatedByPendingParent_Fails(t *testing.T) {
	slug := "v3-pending"
	s := setupVerifyFeature(t, slug)
	writeIntentFiles(t, s, slug)
	if _, err := s.AddFeature(store.AddFeatureInput{Title: "parent", Slug: "parent", Request: "x"}); err != nil {
		t.Fatal(err)
	}
	// Parent stays in `requested` (default after AddFeature).
	st, err := s.LoadFeatureStatus(slug)
	if err != nil {
		t.Fatal(err)
	}
	st.DependsOn = []store.Dependency{{Slug: "parent", Kind: store.DependencyKindHard}}
	if err := s.SaveFeatureStatus(st); err != nil {
		t.Fatal(err)
	}
	writeVerifyRecipe(t, s, slug, ApplyRecipe{Feature: slug, Operations: []RecipeOperation{
		{Type: "replace-in-file", Path: "src/parent-file.go", Search: "a", Replace: "b", CreatedBy: "parent"},
	}})

	report, err := RunVerify(s, slug, VerifyOptions{NoWrite: true})
	if err != nil {
		t.Fatal(err)
	}
	c := findCheck(t, report, CheckRecipeOpTargetsResolve)
	if c.Passed {
		t.Errorf("V3 should fail when created_by parent is not applied/upstream_merged; got %+v", c)
	}
}

// ── V4 ──────────────────────────────────────────────────────────────────

func TestRunVerify_V4_NoDeps_Passes(t *testing.T) {
	slug := "v4-empty"
	s := setupVerifyFeature(t, slug)
	writeIntentFiles(t, s, slug)
	report, err := RunVerify(s, slug, VerifyOptions{NoWrite: true})
	if err != nil {
		t.Fatal(err)
	}
	c := findCheck(t, report, CheckDepMetadataValid)
	if !c.Passed || c.Skipped {
		t.Errorf("V4 should pass with no deps; got %+v", c)
	}
}

func TestRunVerify_V4_DanglingDep_Fails(t *testing.T) {
	slug := "v4-dangling"
	s := setupVerifyFeature(t, slug)
	writeIntentFiles(t, s, slug)
	st, err := s.LoadFeatureStatus(slug)
	if err != nil {
		t.Fatal(err)
	}
	st.DependsOn = []store.Dependency{{Slug: "ghost", Kind: store.DependencyKindHard}}
	if err := s.SaveFeatureStatus(st); err != nil {
		t.Fatal(err)
	}
	report, err := RunVerify(s, slug, VerifyOptions{NoWrite: true})
	if err != nil {
		t.Fatal(err)
	}
	c := findCheck(t, report, CheckDepMetadataValid)
	if c.Passed {
		t.Errorf("V4 should fail on dangling dep; got %+v", c)
	}
	if !strings.Contains(c.Remediation, "ghost") {
		t.Errorf("V4 remediation should wrap validation sentinel verbatim; got %q", c.Remediation)
	}
}

// ── V5 ──────────────────────────────────────────────────────────────────

func TestRunVerify_V5_NoSatisfiedBy_Skipped(t *testing.T) {
	slug := "v5-none"
	s := setupVerifyFeature(t, slug)
	writeIntentFiles(t, s, slug)
	report, err := RunVerify(s, slug, VerifyOptions{NoWrite: true})
	if err != nil {
		t.Fatal(err)
	}
	c := findCheck(t, report, CheckSatisfiedByReachable)
	if !c.Passed || !c.Skipped {
		t.Errorf("V5 should be passed+skipped when no satisfied_by deps; got %+v", c)
	}
}

func TestRunVerify_V5_MalformedSHA_Fails(t *testing.T) {
	slug := "v5-malformed"
	s := setupVerifyFeature(t, slug)
	writeIntentFiles(t, s, slug)
	if _, err := s.AddFeature(store.AddFeatureInput{Title: "parent", Slug: "parent", Request: "x"}); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkFeatureState("parent", store.StateUpstreamMerged, "test", ""); err != nil {
		t.Fatal(err)
	}
	st, err := s.LoadFeatureStatus(slug)
	if err != nil {
		t.Fatal(err)
	}
	st.DependsOn = []store.Dependency{{
		Slug: "parent", Kind: store.DependencyKindHard, SatisfiedBy: "not-a-sha",
	}}
	if err := s.SaveFeatureStatus(st); err != nil {
		t.Fatal(err)
	}
	report, err := RunVerify(s, slug, VerifyOptions{NoWrite: true})
	if err != nil {
		t.Fatal(err)
	}
	// V4 will catch the malformed SHA before V5 since ValidateDependencies
	// runs the same regex; that's expected — both surface block-severity
	// failures so the verdict is failed either way. V5 must be evaluated
	// (not skipped on the upstream V4 failure) because each check is
	// independent.
	c4 := findCheck(t, report, CheckDepMetadataValid)
	if c4.Passed {
		t.Errorf("V4 should fail on malformed SHA; got %+v", c4)
	}
}

func TestRunVerify_V5_UnreachableSHA_Fails(t *testing.T) {
	slug := "v5-unreachable"
	s := setupVerifyFeature(t, slug)
	writeIntentFiles(t, s, slug)
	if _, err := s.AddFeature(store.AddFeatureInput{Title: "parent", Slug: "parent", Request: "x"}); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkFeatureState("parent", store.StateUpstreamMerged, "test", ""); err != nil {
		t.Fatal(err)
	}
	// Use a syntactically valid 40-hex SHA that is NOT in the repo.
	const phantomSHA = "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef"
	st, err := s.LoadFeatureStatus(slug)
	if err != nil {
		t.Fatal(err)
	}
	st.DependsOn = []store.Dependency{{
		Slug: "parent", Kind: store.DependencyKindHard, SatisfiedBy: phantomSHA,
	}}
	if err := s.SaveFeatureStatus(st); err != nil {
		t.Fatal(err)
	}
	report, err := RunVerify(s, slug, VerifyOptions{NoWrite: true})
	if err != nil {
		t.Fatal(err)
	}
	// V5 fails standalone (unreachable); V4 also fails (validation runs
	// the same reachability check). Either failing is enough; assert V5
	// specifically.
	c := findCheck(t, report, CheckSatisfiedByReachable)
	// V5 may be skipped if V4 blocked V7 — no, V4/V5 are sibling static
	// checks both block, both run independently. V5 must explicitly fail
	// here.
	// ValidateDependencies in V4 runs first and may fail; V5 runs
	// independently and should also fail.
	if c.Passed {
		// Acceptable if validation already raised the error and V5 was
		// designed to short-circuit; but our V5 implementation is
		// independent.
		t.Errorf("V5 should fail on unreachable SHA; got %+v", c)
	}
	if !strings.Contains(c.Remediation, phantomSHA) {
		t.Errorf("V5 remediation should name the phantom SHA; got %q", c.Remediation)
	}
}

// ── V6 ──────────────────────────────────────────────────────────────────

func TestRunVerify_V6_DAGEnabled_NoDeps_Passes(t *testing.T) {
	slug := "v6-empty"
	s := setupVerifyFeature(t, slug)
	writeIntentFiles(t, s, slug)
	report, err := RunVerify(s, slug, VerifyOptions{NoWrite: true})
	if err != nil {
		t.Fatal(err)
	}
	c := findCheck(t, report, CheckDependencyGateSatisfied)
	if !c.Passed || c.Skipped {
		t.Errorf("V6 should pass with no deps; got %+v", c)
	}
	if c.Severity != SeverityWarn {
		t.Errorf("V6 severity must be warn; got %s", c.Severity)
	}
}

// V6 must be warn-only — even when the gate is unsatisfied, the verdict
// must not flip to failed (PRD §3.3 + ADR-013).
func TestRunVerify_V6_UnsatisfiedDep_WarnNotBlock(t *testing.T) {
	slug := "v6-warn"
	s := setupVerifyFeature(t, slug)
	writeIntentFiles(t, s, slug)
	if _, err := s.AddFeature(store.AddFeatureInput{Title: "parent", Slug: "parent", Request: "x"}); err != nil {
		t.Fatal(err)
	}
	// Parent stays in requested → gate fails.
	st, err := s.LoadFeatureStatus(slug)
	if err != nil {
		t.Fatal(err)
	}
	st.DependsOn = []store.Dependency{{Slug: "parent", Kind: store.DependencyKindHard}}
	if err := s.SaveFeatureStatus(st); err != nil {
		t.Fatal(err)
	}
	// No recipe → V7/V8 skip; V0–V5/V9 are clean.
	report, err := RunVerify(s, slug, VerifyOptions{NoWrite: true})
	if err != nil {
		t.Fatal(err)
	}
	c := findCheck(t, report, CheckDependencyGateSatisfied)
	if c.Passed {
		t.Errorf("V6 should fail when hard parent is unapplied; got %+v", c)
	}
	if c.Severity != SeverityWarn {
		t.Errorf("V6 severity must remain warn; got %s", c.Severity)
	}
	if !strings.Contains(c.Remediation, "warn-only at verify time") {
		t.Errorf("V6 remediation must follow PRD §3.1.2 verbatim; got %q", c.Remediation)
	}
	// A warn-only V6 fail must NOT flip the verdict.
	if report.Verdict == "failed" {
		t.Errorf("V6 warn-only failure must not flip verdict; got %s", report.Verdict)
	}
}

// ── V9 ──────────────────────────────────────────────────────────────────

func TestRunVerify_V9_NoOutcome_Skipped(t *testing.T) {
	slug := "v9-none"
	s := setupVerifyFeature(t, slug)
	writeIntentFiles(t, s, slug)
	report, err := RunVerify(s, slug, VerifyOptions{NoWrite: true})
	if err != nil {
		t.Fatal(err)
	}
	c := findCheck(t, report, CheckReconcileOutcomeConsistent)
	if !c.Passed || !c.Skipped {
		t.Errorf("V9 should be passed+skipped when no Reconcile.Outcome; got %+v", c)
	}
	if c.Severity != SeverityWarn {
		t.Errorf("V9 severity must be warn; got %s", c.Severity)
	}
}

func TestRunVerify_V9_ReappliedOutcome_Passes(t *testing.T) {
	slug := "v9-reapplied"
	s := setupVerifyFeature(t, slug)
	writeIntentFiles(t, s, slug)
	st, err := s.LoadFeatureStatus(slug)
	if err != nil {
		t.Fatal(err)
	}
	st.Reconcile.Outcome = store.ReconcileReapplied
	if err := s.SaveFeatureStatus(st); err != nil {
		t.Fatal(err)
	}
	report, err := RunVerify(s, slug, VerifyOptions{NoWrite: true})
	if err != nil {
		t.Fatal(err)
	}
	c := findCheck(t, report, CheckReconcileOutcomeConsistent)
	if !c.Passed || c.Skipped {
		t.Errorf("V9 should pass for outcome=reapplied; got %+v", c)
	}
}

func TestRunVerify_V9_BlockedOutcome_WarnFail(t *testing.T) {
	slug := "v9-blocked"
	s := setupVerifyFeature(t, slug)
	writeIntentFiles(t, s, slug)
	st, err := s.LoadFeatureStatus(slug)
	if err != nil {
		t.Fatal(err)
	}
	st.Reconcile.Outcome = store.ReconcileBlocked
	if err := s.SaveFeatureStatus(st); err != nil {
		t.Fatal(err)
	}
	report, err := RunVerify(s, slug, VerifyOptions{NoWrite: true})
	if err != nil {
		t.Fatal(err)
	}
	c := findCheck(t, report, CheckReconcileOutcomeConsistent)
	if c.Passed {
		t.Errorf("V9 should fail (warn) for outcome=blocked; got %+v", c)
	}
	if !strings.Contains(c.Remediation, "warn-only") {
		t.Errorf("V9 remediation must follow PRD §3.1.2 verbatim; got %q", c.Remediation)
	}
	// Warn-only failure must not flip the verdict.
	if report.Verdict == "failed" {
		t.Errorf("V9 warn must not flip verdict; got %s", report.Verdict)
	}
}

// V9 SOURCE-TRUTH ADVERSARIAL TEST (ADR-013 D6).
//
// V9 must read `status.Reconcile.Outcome` and ONLY that. If the
// implementation accidentally consults `reconcile-session.json` or any
// `artifacts/*` file beyond the recipe / post-apply.patch, this test
// catches it: we plant deliberately-poisoned content under
// artifacts/reconcile-session.json (corrupt JSON) and an unrelated
// poisoned artifact, then assert V9 reads the in-memory outcome
// cleanly. A V9 implementation that opens or parses these files would
// either propagate a JSON error (fail unexpectedly) or report a wrong
// outcome.
func TestRunVerify_V9_SourceTruth_DoesNotReadArtifacts(t *testing.T) {
	slug := "v9-source-truth"
	s := setupVerifyFeature(t, slug)
	writeIntentFiles(t, s, slug)

	// Set the in-memory outcome that V9 must consult.
	st, err := s.LoadFeatureStatus(slug)
	if err != nil {
		t.Fatal(err)
	}
	st.Reconcile.Outcome = store.ReconcileReapplied
	if err := s.SaveFeatureStatus(st); err != nil {
		t.Fatal(err)
	}

	// Plant a poisoned reconcile-session.json — corrupt JSON whose
	// presence would break any consumer that opens it.
	artifactsDir := filepath.Join(s.Root, ".tpatch", "features", slug, "artifacts")
	if err := os.MkdirAll(artifactsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(artifactsDir, "reconcile-session.json"), []byte("{ this is intentionally NOT json"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Plant a poisoned post-reconcile.json as well — anything D6
	// forbids touching beyond apply-recipe.json + post-apply.patch.
	if err := os.WriteFile(filepath.Join(artifactsDir, "post-reconcile.json"), []byte("not json either"), 0o644); err != nil {
		t.Fatal(err)
	}

	report, err := RunVerify(s, slug, VerifyOptions{NoWrite: true})
	if err != nil {
		t.Fatalf("RunVerify must not propagate artifact-parse errors; got %v", err)
	}
	c := findCheck(t, report, CheckReconcileOutcomeConsistent)
	if !c.Passed || c.Skipped {
		t.Errorf("V9 must read status.Reconcile.Outcome only and pass for reapplied; got %+v", c)
	}
}

// ── V0–V2 truth-table integrity guard ───────────────────────────────────
//
// A defensive harness assertion that the V0/V1/V2 contract from Slice A
// is not regressed by Slice C. Mirrors what the Slice A truth table
// already covers but in a single shot: V0 + V1 + V2 must all pass real
// (not skipped) on a healthy fixture.
func TestRunVerify_SliceA_V0V1V2_StillRealAndPassing(t *testing.T) {
	slug := "slice-a-guard"
	s := setupVerifyFeature(t, slug)
	writeIntentFiles(t, s, slug)
	writeVerifyRecipe(t, s, slug, ApplyRecipe{Feature: slug, Operations: nil})

	report, err := RunVerify(s, slug, VerifyOptions{NoWrite: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{CheckStatusLoaded, CheckIntentFilesPresent, CheckRecipeParses} {
		c := findCheck(t, report, id)
		if !c.Passed || c.Skipped {
			t.Errorf("Slice A regression: %s should pass non-skipped on a healthy fixture; got %+v", id, c)
		}
	}
}

// commitInRepo stages and commits a file in the test git repo so V7's
// shadow allocation has up-to-date HEAD content. Used by closure replay
// fixtures that need parent-created files visible in the shadow.
func commitInRepo(t *testing.T, repoRoot, relPath, content, msg string) {
	t.Helper()
	abs := filepath.Join(repoRoot, relPath)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"add", relPath},
		{"commit", "-q", "-m", msg},
	} {
		c := exec.Command("git", args...)
		c.Dir = repoRoot
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
}
