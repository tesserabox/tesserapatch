package workflow

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/tesseracode/tesserapatch/internal/store"
)

// Slice B (ADR-013 / PRD-verify-freshness §3.4.2): assert that
// DeriveFreshnessLabel returns exactly one of the four freshness
// labels for every (Verify, recipe-hash, patch-hash, parent-snapshot)
// quadrant of the truth table.

func sha256HexBytes(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func writeRecipeBytes(t *testing.T, s *store.Store, slug, body string) string {
	t.Helper()
	if err := s.WriteArtifact(slug, "apply-recipe.json", body); err != nil {
		t.Fatalf("WriteArtifact recipe: %v", err)
	}
	return sha256HexBytes([]byte(body))
}

func writePatchBytes(t *testing.T, s *store.Store, slug, body string) string {
	t.Helper()
	if err := s.WriteArtifact(slug, "post-apply.patch", body); err != nil {
		t.Fatalf("WriteArtifact patch: %v", err)
	}
	return sha256HexBytes([]byte(body))
}

func TestDeriveFreshnessLabel_NeverVerified_NoVerifyRecord(t *testing.T) {
	s := planTestEnv(t, true)
	addPlanFeature(t, s, "x", nil)
	st, _ := s.LoadFeatureStatus("x")
	if got := DeriveFreshnessLabel(s, st); got != store.LabelNeverVerified {
		t.Fatalf("expected never-verified, got %q", got)
	}
}

func TestDeriveFreshnessLabel_VerifyFailed_PassedFalse(t *testing.T) {
	s := planTestEnv(t, true)
	addPlanFeature(t, s, "x", nil)
	st, _ := s.LoadFeatureStatus("x")
	st.Verify = &store.VerifyRecord{VerifiedAt: "2025-01-01T00:00:00Z", Passed: false}
	if err := s.SaveFeatureStatus(st); err != nil {
		t.Fatal(err)
	}
	st, _ = s.LoadFeatureStatus("x")
	if got := DeriveFreshnessLabel(s, st); got != store.LabelVerifyFailed {
		t.Fatalf("expected verify-failed, got %q", got)
	}
}

func TestDeriveFreshnessLabel_VerifiedFresh_AllInvariantsHold(t *testing.T) {
	s := planTestEnv(t, true)
	addPlanFeature(t, s, "p", nil)
	addPlanFeature(t, s, "c", []store.Dependency{{Slug: "p", Kind: store.DependencyKindHard}})
	setParentState(t, s, "p", store.StateApplied, store.ReconcileReapplied, "2025-03-01T00:00:00Z")

	rh := writeRecipeBytes(t, s, "c", `{"ops":[]}`)
	ph := writePatchBytes(t, s, "c", "diff --git\n")

	st, _ := s.LoadFeatureStatus("c")
	st.Verify = &store.VerifyRecord{
		VerifiedAt:         "2025-03-02T00:00:00Z",
		Passed:             true,
		RecipeHashAtVerify: rh,
		PatchHashAtVerify:  ph,
		ParentSnapshot:     map[string]store.FeatureState{"p": store.StateApplied},
	}
	if err := s.SaveFeatureStatus(st); err != nil {
		t.Fatal(err)
	}
	st, _ = s.LoadFeatureStatus("c")
	if got := DeriveFreshnessLabel(s, st); got != store.LabelVerifiedFresh {
		t.Fatalf("expected verified-fresh, got %q", got)
	}
}

func TestDeriveFreshnessLabel_VerifiedStale_RecipeHashDrift(t *testing.T) {
	s := planTestEnv(t, true)
	addPlanFeature(t, s, "c", nil)
	_ = writeRecipeBytes(t, s, "c", `{"ops":[]}`)
	ph := writePatchBytes(t, s, "c", "p\n")

	st, _ := s.LoadFeatureStatus("c")
	st.Verify = &store.VerifyRecord{
		VerifiedAt:         "t",
		Passed:             true,
		RecipeHashAtVerify: "deadbeef", // mismatched
		PatchHashAtVerify:  ph,
	}
	_ = s.SaveFeatureStatus(st)
	st, _ = s.LoadFeatureStatus("c")
	if got := DeriveFreshnessLabel(s, st); got != store.LabelVerifiedStale {
		t.Fatalf("expected verified-stale (recipe drift), got %q", got)
	}
}

func TestDeriveFreshnessLabel_VerifiedStale_PatchHashDrift(t *testing.T) {
	s := planTestEnv(t, true)
	addPlanFeature(t, s, "c", nil)
	rh := writeRecipeBytes(t, s, "c", `{"ops":[]}`)
	_ = writePatchBytes(t, s, "c", "p\n")

	st, _ := s.LoadFeatureStatus("c")
	st.Verify = &store.VerifyRecord{
		VerifiedAt:         "t",
		Passed:             true,
		RecipeHashAtVerify: rh,
		PatchHashAtVerify:  "deadbeef",
	}
	_ = s.SaveFeatureStatus(st)
	st, _ = s.LoadFeatureStatus("c")
	if got := DeriveFreshnessLabel(s, st); got != store.LabelVerifiedStale {
		t.Fatalf("expected verified-stale (patch drift), got %q", got)
	}
}

func TestDeriveFreshnessLabel_VerifiedStale_ParentRegression(t *testing.T) {
	s := planTestEnv(t, true)
	addPlanFeature(t, s, "p", nil)
	addPlanFeature(t, s, "c", []store.Dependency{{Slug: "p", Kind: store.DependencyKindHard}})
	// Parent currently in `requested` (default).
	rh := writeRecipeBytes(t, s, "c", `{"ops":[]}`)
	ph := writePatchBytes(t, s, "c", "p\n")

	st, _ := s.LoadFeatureStatus("c")
	st.Verify = &store.VerifyRecord{
		VerifiedAt:         "t",
		Passed:             true,
		RecipeHashAtVerify: rh,
		PatchHashAtVerify:  ph,
		ParentSnapshot:     map[string]store.FeatureState{"p": store.StateApplied},
	}
	_ = s.SaveFeatureStatus(st)
	st, _ = s.LoadFeatureStatus("c")
	if got := DeriveFreshnessLabel(s, st); got != store.LabelVerifiedStale {
		t.Fatalf("expected verified-stale (parent regressed below applied), got %q", got)
	}
}

func TestDeriveFreshnessLabel_VerifiedStale_ParentMissing(t *testing.T) {
	s := planTestEnv(t, true)
	addPlanFeature(t, s, "c", nil)
	rh := writeRecipeBytes(t, s, "c", `{}`)
	ph := writePatchBytes(t, s, "c", "")

	st, _ := s.LoadFeatureStatus("c")
	st.Verify = &store.VerifyRecord{
		VerifiedAt:         "t",
		Passed:             true,
		RecipeHashAtVerify: rh,
		PatchHashAtVerify:  ph,
		ParentSnapshot:     map[string]store.FeatureState{"ghost": store.StateApplied},
	}
	_ = s.SaveFeatureStatus(st)
	st, _ = s.LoadFeatureStatus("c")
	if got := DeriveFreshnessLabel(s, st); got != store.LabelVerifiedStale {
		t.Fatalf("expected verified-stale (parent gone), got %q", got)
	}
}

// State-or-better invariant: applied snapshot accepts upstream_merged
// current.
func TestDeriveFreshnessLabel_StateOrBetter_AppliedAcceptsUpstreamMerged(t *testing.T) {
	s := planTestEnv(t, true)
	addPlanFeature(t, s, "p", nil)
	addPlanFeature(t, s, "c", []store.Dependency{{Slug: "p", Kind: store.DependencyKindHard}})
	setParentState(t, s, "p", store.StateUpstreamMerged, store.ReconcileUpstreamed, "2025-03-01T00:00:00Z")

	rh := writeRecipeBytes(t, s, "c", `{}`)
	ph := writePatchBytes(t, s, "c", "")
	st, _ := s.LoadFeatureStatus("c")
	st.Verify = &store.VerifyRecord{
		VerifiedAt:         "t",
		Passed:             true,
		RecipeHashAtVerify: rh,
		PatchHashAtVerify:  ph,
		ParentSnapshot:     map[string]store.FeatureState{"p": store.StateApplied},
	}
	_ = s.SaveFeatureStatus(st)
	st, _ = s.LoadFeatureStatus("c")
	if got := DeriveFreshnessLabel(s, st); got != store.LabelVerifiedFresh {
		t.Fatalf("applied snapshot must accept upstream_merged current; got %q", got)
	}
}

// State-or-better: upstream_merged snapshot is terminal — current must
// be exactly upstream_merged.
func TestDeriveFreshnessLabel_StateOrBetter_UpstreamMergedRequiresExact(t *testing.T) {
	s := planTestEnv(t, true)
	addPlanFeature(t, s, "p", nil)
	addPlanFeature(t, s, "c", []store.Dependency{{Slug: "p", Kind: store.DependencyKindHard}})
	setParentState(t, s, "p", store.StateApplied, store.ReconcileReapplied, "2025-03-01T00:00:00Z")

	rh := writeRecipeBytes(t, s, "c", `{}`)
	ph := writePatchBytes(t, s, "c", "")
	st, _ := s.LoadFeatureStatus("c")
	st.Verify = &store.VerifyRecord{
		VerifiedAt:         "t",
		Passed:             true,
		RecipeHashAtVerify: rh,
		PatchHashAtVerify:  ph,
		ParentSnapshot:     map[string]store.FeatureState{"p": store.StateUpstreamMerged},
	}
	_ = s.SaveFeatureStatus(st)
	st, _ = s.LoadFeatureStatus("c")
	if got := DeriveFreshnessLabel(s, st); got != store.LabelVerifiedStale {
		t.Fatalf("upstream_merged snapshot must be exact; got %q", got)
	}
}

// Empty-recorded + empty-current artifact bytes is a match (mirrors
// verify writer's sha256Hex(nil) → "").
func TestDeriveFreshnessLabel_EmptyHashes_MatchAsAbsent(t *testing.T) {
	s := planTestEnv(t, true)
	addPlanFeature(t, s, "c", nil)
	// Do NOT write recipe/patch artifacts.
	st, _ := s.LoadFeatureStatus("c")
	st.Verify = &store.VerifyRecord{
		VerifiedAt:         "t",
		Passed:             true,
		RecipeHashAtVerify: "",
		PatchHashAtVerify:  "",
	}
	_ = s.SaveFeatureStatus(st)
	st, _ = s.LoadFeatureStatus("c")
	if got := DeriveFreshnessLabel(s, st); got != store.LabelVerifiedFresh {
		t.Fatalf("absent-recorded + absent-file must be fresh; got %q", got)
	}
}

// IsFreshnessLabel / StripFreshnessLabels purity.
func TestStripFreshnessLabels_RemovesOnlyFreshness(t *testing.T) {
	in := []store.ReconcileLabel{
		store.LabelBlockedByParent,
		store.LabelNeverVerified,
		store.LabelStaleParentApplied,
		store.LabelVerifiedFresh,
		store.LabelVerifyFailed,
		store.LabelVerifiedStale,
	}
	got := StripFreshnessLabels(in)
	if len(got) != 2 || got[0] != store.LabelBlockedByParent || got[1] != store.LabelStaleParentApplied {
		t.Fatalf("StripFreshnessLabels left non-freshness labels intact only; got %v", got)
	}
	for _, l := range got {
		if IsFreshnessLabel(l) {
			t.Fatalf("freshness label leaked through strip: %q", l)
		}
	}
}
