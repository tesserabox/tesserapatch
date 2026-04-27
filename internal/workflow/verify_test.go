package workflow

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tesseracode/tesserapatch/internal/store"
)

// helper: init a store with a feature in `applied` state.
func setupVerifyFeature(t *testing.T, slug string) *store.Store {
	t.Helper()
	tmp := t.TempDir()
	s, err := store.Init(tmp)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if _, err := s.AddFeature(store.AddFeatureInput{Title: slug, Slug: slug, Request: "x"}); err != nil {
		t.Fatalf("AddFeature: %v", err)
	}
	if err := s.MarkFeatureState(slug, store.StateApplied, "apply", ""); err != nil {
		t.Fatalf("MarkFeatureState: %v", err)
	}
	return s
}

// helper: write spec.md so V1 passes.
func writeSpec(t *testing.T, s *store.Store, slug, body string) {
	t.Helper()
	if err := s.WriteFeatureFile(slug, "spec.md", body); err != nil {
		t.Fatalf("write spec.md: %v", err)
	}
}

// helper: write apply-recipe.json so V2 has a recipe to parse.
func writeVerifyRecipe(t *testing.T, s *store.Store, slug string, recipe ApplyRecipe) {
	t.Helper()
	data, err := json.Marshal(recipe)
	if err != nil {
		t.Fatalf("marshal recipe: %v", err)
	}
	if err := s.WriteArtifact(slug, "apply-recipe.json", string(data)); err != nil {
		t.Fatalf("write recipe: %v", err)
	}
}

// ── V0 / V1 / V2 pass ───────────────────────────────────────────────────

func TestRunVerify_V0V1V2_AllPass(t *testing.T) {
	slug := "demo"
	s := setupVerifyFeature(t, slug)
	writeSpec(t, s, slug, "intent text")
	writeVerifyRecipe(t, s, slug, ApplyRecipe{Feature: slug, Operations: []RecipeOperation{
		{Type: "ensure-directory", Path: "src"},
	}})

	report, err := RunVerify(s, slug, VerifyOptions{})
	if err != nil {
		t.Fatalf("RunVerify: %v", err)
	}
	if len(report.Checks) != 10 {
		t.Fatalf("expected 10-check array, got %d", len(report.Checks))
	}
	must := func(id string) store.VerifyCheckResult {
		for _, c := range report.Checks {
			if c.ID == id {
				return c
			}
		}
		t.Fatalf("missing check %s", id)
		return store.VerifyCheckResult{}
	}
	for _, id := range []string{CheckStatusLoaded, CheckIntentFilesPresent, CheckRecipeParses, CheckRecipeOpTargetsResolve} {
		c := must(id)
		if !c.Passed || c.Skipped {
			t.Errorf("%s expected real pass; got %+v", id, c)
		}
	}
	if report.Verdict != "passed" || report.ExitCode != 0 {
		t.Errorf("verdict=%q exit=%d", report.Verdict, report.ExitCode)
	}
	// Persisted record was written. Reload and check.
	loaded, err := s.LoadFeatureStatus(slug)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Verify == nil {
		t.Fatal("expected Verify record persisted")
	}
	if !loaded.Verify.Passed {
		t.Error("persisted Passed=true expected")
	}
	if loaded.Verify.RecipeHashAtVerify == "" {
		t.Error("expected non-empty recipe hash")
	}
}

// ── V1 fail: spec.md missing ────────────────────────────────────────────

func TestRunVerify_V1_FailsWhenSpecMissing(t *testing.T) {
	slug := "no-spec"
	s := setupVerifyFeature(t, slug)
	// no spec.md
	report, err := RunVerify(s, slug, VerifyOptions{})
	if err != nil {
		t.Fatalf("RunVerify: %v", err)
	}
	for _, c := range report.Checks {
		if c.ID == CheckIntentFilesPresent {
			if c.Passed || c.Skipped {
				t.Errorf("expected V1 to fail, got %+v", c)
			}
			break
		}
	}
	if report.Verdict != "failed" {
		t.Errorf("expected failed verdict, got %s", report.Verdict)
	}
}

// ── V1 fail: empty spec.md ──────────────────────────────────────────────

func TestRunVerify_V1_FailsWhenSpecEmpty(t *testing.T) {
	slug := "empty-spec"
	s := setupVerifyFeature(t, slug)
	writeSpec(t, s, slug, "")
	report, err := RunVerify(s, slug, VerifyOptions{})
	if err != nil {
		t.Fatalf("RunVerify: %v", err)
	}
	for _, c := range report.Checks {
		if c.ID == CheckIntentFilesPresent {
			if c.Passed {
				t.Errorf("expected V1 to fail on empty spec, got %+v", c)
			}
			break
		}
	}
}

// ── V2 absent recipe — Note 2 contract ──────────────────────────────────

func TestRunVerify_V2_AbsentRecipe_SkippedNotFailed(t *testing.T) {
	slug := "no-recipe"
	s := setupVerifyFeature(t, slug)
	writeSpec(t, s, slug, "intent")
	// no apply-recipe.json
	report, err := RunVerify(s, slug, VerifyOptions{})
	if err != nil {
		t.Fatalf("RunVerify: %v", err)
	}
	var parse, ops store.VerifyCheckResult
	for _, c := range report.Checks {
		switch c.ID {
		case CheckRecipeParses:
			parse = c
		case CheckRecipeOpTargetsResolve:
			ops = c
		}
	}
	for _, c := range []store.VerifyCheckResult{parse, ops} {
		if !c.Passed || !c.Skipped {
			t.Errorf("%s: expected passed:true skipped:true on absent recipe, got %+v", c.ID, c)
		}
		if c.Reason == "" {
			t.Errorf("%s: expected non-empty skip reason", c.ID)
		}
	}
	if report.Verdict != "passed" {
		t.Errorf("absent recipe must not flip verdict to failed; got %s", report.Verdict)
	}
}

// ── V2 fail: malformed JSON ─────────────────────────────────────────────

func TestRunVerify_V2_MalformedJSONFails(t *testing.T) {
	slug := "bad-recipe"
	s := setupVerifyFeature(t, slug)
	writeSpec(t, s, slug, "intent")
	if err := s.WriteArtifact(slug, "apply-recipe.json", "{not valid json"); err != nil {
		t.Fatal(err)
	}
	report, err := RunVerify(s, slug, VerifyOptions{})
	if err != nil {
		t.Fatalf("RunVerify: %v", err)
	}
	for _, c := range report.Checks {
		if c.ID == CheckRecipeParses && (c.Passed || c.Skipped) {
			t.Errorf("expected recipe_parses to fail on malformed JSON, got %+v", c)
		}
	}
	if report.Verdict != "failed" {
		t.Errorf("expected failed verdict, got %s", report.Verdict)
	}
}

// ── V2 fail: replace-in-file target missing ─────────────────────────────

func TestRunVerify_V2_OpTargetMissingFails(t *testing.T) {
	slug := "missing-target"
	s := setupVerifyFeature(t, slug)
	writeSpec(t, s, slug, "intent")
	writeVerifyRecipe(t, s, slug, ApplyRecipe{Feature: slug, Operations: []RecipeOperation{
		{Type: "replace-in-file", Path: "src/does-not-exist.go", Search: "a", Replace: "b"},
	}})
	report, err := RunVerify(s, slug, VerifyOptions{})
	if err != nil {
		t.Fatalf("RunVerify: %v", err)
	}
	for _, c := range report.Checks {
		if c.ID == CheckRecipeOpTargetsResolve {
			if c.Passed || c.Skipped {
				t.Errorf("expected op-targets-resolve to fail, got %+v", c)
			}
			if !strings.Contains(c.Remediation, "src/does-not-exist.go") {
				t.Errorf("expected remediation to name the missing path, got %q", c.Remediation)
			}
		}
	}
}

// ── --no-write does not persist ─────────────────────────────────────────

func TestRunVerify_NoWriteDoesNotPersist(t *testing.T) {
	slug := "nowrite"
	s := setupVerifyFeature(t, slug)
	writeSpec(t, s, slug, "intent")
	writeVerifyRecipe(t, s, slug, ApplyRecipe{Feature: slug, Operations: nil})

	if _, err := RunVerify(s, slug, VerifyOptions{NoWrite: true}); err != nil {
		t.Fatal(err)
	}
	loaded, err := s.LoadFeatureStatus(slug)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Verify != nil {
		t.Errorf("--no-write must not persist Verify, got %+v", loaded.Verify)
	}

	// After a write run, the record is present.
	if _, err := RunVerify(s, slug, VerifyOptions{}); err != nil {
		t.Fatal(err)
	}
	loaded, err = s.LoadFeatureStatus(slug)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Verify == nil {
		t.Fatal("expected Verify after a normal run")
	}
}

// ── --json shape: PRD §3.2 / §4.3 ───────────────────────────────────────

func TestRunVerify_JSONShape(t *testing.T) {
	slug := "shape"
	s := setupVerifyFeature(t, slug)
	writeSpec(t, s, slug, "intent")
	writeVerifyRecipe(t, s, slug, ApplyRecipe{Feature: slug, Operations: nil})

	report, err := RunVerify(s, slug, VerifyOptions{NoWrite: true})
	if err != nil {
		t.Fatal(err)
	}

	var buf strings.Builder
	if err := report.WriteJSONReport(&buf); err != nil {
		t.Fatal(err)
	}

	// Round-trip into a generic map and check the required keys.
	var m map[string]any
	if err := json.Unmarshal([]byte(buf.String()), &m); err != nil {
		t.Fatalf("json: %v\n%s", err, buf.String())
	}
	for _, key := range []string{"schema_version", "slug", "verified_at", "verdict", "exit_code", "checks", "lifecycle_state"} {
		if _, ok := m[key]; !ok {
			t.Errorf("JSON report missing required key %q", key)
		}
	}
	if m["schema_version"] != "1.0" {
		t.Errorf("schema_version=1.0 expected, got %v", m["schema_version"])
	}
	checks, ok := m["checks"].([]any)
	if !ok || len(checks) != 10 {
		t.Errorf("expected 10-check array in JSON, got %v entries", len(checks))
	}
	// All ten check IDs present, in order.
	wantIDs := []string{
		CheckStatusLoaded, CheckIntentFilesPresent, CheckRecipeParses, CheckRecipeOpTargetsResolve,
		CheckDepMetadataValid, CheckSatisfiedByReachable, CheckDependencyGateSatisfied,
		CheckRecipeReplayClean, CheckPostApplyPatchReplayClean, CheckReconcileOutcomeConsistent,
	}
	for i, want := range wantIDs {
		gotID := checks[i].(map[string]any)["id"]
		if gotID != want {
			t.Errorf("check[%d].id = %v, want %s", i, gotID, want)
		}
	}
}

// ── V3–V9 stubs are passed:true skipped:true with reason ────────────────

func TestRunVerify_StubsCarrySliceReason(t *testing.T) {
	slug := "stubs"
	s := setupVerifyFeature(t, slug)
	writeSpec(t, s, slug, "intent")

	report, err := RunVerify(s, slug, VerifyOptions{NoWrite: true})
	if err != nil {
		t.Fatal(err)
	}
	stubIDs := map[string]bool{
		CheckDepMetadataValid: true, CheckSatisfiedByReachable: true,
		CheckDependencyGateSatisfied: true, CheckRecipeReplayClean: true,
		CheckPostApplyPatchReplayClean: true, CheckReconcileOutcomeConsistent: true,
	}
	for _, c := range report.Checks {
		if !stubIDs[c.ID] {
			continue
		}
		if !c.Passed || !c.Skipped {
			t.Errorf("stub %s expected passed+skipped, got %+v", c.ID, c)
		}
		if !strings.Contains(c.Reason, "Slice") {
			t.Errorf("stub %s reason should name a slice, got %q", c.ID, c.Reason)
		}
	}
}

// ── V0 abort: status.json read failure ──────────────────────────────────

func TestRunVerify_V0_AbortsWhenStatusUnreadable(t *testing.T) {
	slug := "abort"
	s := setupVerifyFeature(t, slug)
	// Corrupt status.json into a directory so ReadFile returns an error.
	statusPath := filepath.Join(s.Root, ".tpatch", "features", slug, "status.json")
	if err := os.Remove(statusPath); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(statusPath, 0o755); err != nil {
		t.Fatal(err)
	}
	report, err := RunVerify(s, slug, VerifyOptions{NoWrite: true})
	if err == nil {
		t.Fatal("expected RunVerify to surface an error when V0 aborts")
	}
	if report == nil {
		t.Fatal("report should still be produced for shape stability")
	}
	if len(report.Checks) != 10 {
		t.Errorf("expected 10-check array even on V0 abort, got %d", len(report.Checks))
	}
	if report.Checks[0].ID != CheckStatusLoaded || report.Checks[0].Passed {
		t.Errorf("V0 should be the first failed check, got %+v", report.Checks[0])
	}
}

// ── parentSnapshot ──────────────────────────────────────────────────────

// TestParentSnapshot_MissingParentOmitted locks in the M15-W3-SLICE-A
// revision: when a hard parent slug doesn't have an on-disk feature
// directory, parentSnapshot must omit the key entirely (not record an
// empty-string FeatureState, which is invalid and would crash Slice B's
// satisfies_state_or_better derivation).
func TestParentSnapshot_MissingParentOmitted(t *testing.T) {
	tmp := t.TempDir()
	s, err := store.Init(tmp)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	// Parent A exists and is applied. Parent B is never created.
	if _, err := s.AddFeature(store.AddFeatureInput{Title: "parent-a", Slug: "parent-a", Request: "x"}); err != nil {
		t.Fatalf("AddFeature parent-a: %v", err)
	}
	if err := s.MarkFeatureState("parent-a", store.StateApplied, "apply", ""); err != nil {
		t.Fatalf("MarkFeatureState parent-a: %v", err)
	}

	child := store.FeatureStatus{
		Slug: "child",
		DependsOn: []store.Dependency{
			{Slug: "parent-a", Kind: store.DependencyKindHard},
			{Slug: "parent-b", Kind: store.DependencyKindHard}, // missing on disk
		},
	}

	snap := parentSnapshot(s, child)
	if len(snap) != 1 {
		t.Fatalf("expected exactly one entry, got %d: %+v", len(snap), snap)
	}
	if got, ok := snap["parent-a"]; !ok || got != store.StateApplied {
		t.Errorf("parent-a missing or wrong state: ok=%v state=%q", ok, got)
	}
	if v, ok := snap["parent-b"]; ok {
		t.Errorf("parent-b must NOT be a key (got %q); empty-string sentinel is the bug being fixed", v)
	}
}

// TestParentSnapshot_AllParentsMissingReturnsNil documents the chosen
// behavior when every hard parent is missing on disk: parentSnapshot
// returns nil so the omitempty-tagged field stays absent from JSON,
// preserving byte-identical round-trip with the never-verified baseline
// (ADR-013 D4).
func TestParentSnapshot_AllParentsMissingReturnsNil(t *testing.T) {
	tmp := t.TempDir()
	s, err := store.Init(tmp)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	child := store.FeatureStatus{
		Slug: "child",
		DependsOn: []store.Dependency{
			{Slug: "ghost-a", Kind: store.DependencyKindHard},
			{Slug: "ghost-b", Kind: store.DependencyKindHard},
		},
	}

	snap := parentSnapshot(s, child)
	if snap != nil {
		t.Fatalf("expected nil for all-missing parents, got %+v", snap)
	}
}

// TestParentSnapshot_SoftDepsExcluded preserves the existing contract
// that soft deps are never part of the snapshot — the freshness overlay
// only tracks the hard-dep closure (ADR-013 D5).
func TestParentSnapshot_SoftDepsExcluded(t *testing.T) {
	tmp := t.TempDir()
	s, err := store.Init(tmp)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if _, err := s.AddFeature(store.AddFeatureInput{Title: "soft-parent", Slug: "soft-parent", Request: "x"}); err != nil {
		t.Fatalf("AddFeature soft-parent: %v", err)
	}
	if err := s.MarkFeatureState("soft-parent", store.StateApplied, "apply", ""); err != nil {
		t.Fatalf("MarkFeatureState soft-parent: %v", err)
	}

	child := store.FeatureStatus{
		Slug: "child",
		DependsOn: []store.Dependency{
			{Slug: "soft-parent", Kind: store.DependencyKindSoft},
		},
	}
	snap := parentSnapshot(s, child)
	if snap != nil {
		t.Fatalf("soft-only deps must yield nil snapshot, got %+v", snap)
	}
}
