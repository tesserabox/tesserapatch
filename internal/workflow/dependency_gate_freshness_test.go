package workflow

import (
	"testing"

	"github.com/tesseracode/tesserapatch/internal/store"
)

// ADR-013 D2 invariant: the apply gate (CheckDependencyGate) does NOT
// consider freshness state. A parent that is `applied` with a stale
// Verify record (or no Verify record at all) MUST satisfy the gate
// for its children. Freshness is a read-time overlay; the apply gate
// remains a pure state-driven check.
//
// This test pins that invariant. If a future refactor accidentally
// teaches CheckDependencyGate to look at parent.Verify, this test
// fails and the regression is caught.
func TestDependencyGate_IgnoresParentVerifyStaleness(t *testing.T) {
	s := gateTestEnv(t, true)

	// Parent is `applied`. Attach a Verify record that is structurally
	// stale (Passed=true but with hashes that won't match any current
	// artifact, since we never wrote one). The freshness derivation
	// would label this `verified-stale`; the gate must not care.
	addFeature(t, s, "parent", store.StateApplied, nil)
	pst, _ := s.LoadFeatureStatus("parent")
	pst.Verify = &store.VerifyRecord{
		VerifiedAt:         "2025-01-01T00:00:00Z",
		Passed:             true,
		RecipeHashAtVerify: "deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef",
		PatchHashAtVerify:  "deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef",
	}
	if err := s.SaveFeatureStatus(pst); err != nil {
		t.Fatalf("SaveFeatureStatus parent: %v", err)
	}

	// Sanity: the freshness overlay does mark the parent stale (so we
	// know the test scenario is actually exercising the divergence
	// between freshness and the apply gate).
	if got := DeriveFreshnessLabel(s, pst); got != store.LabelVerifiedStale {
		t.Fatalf("scenario assumes parent freshness is verified-stale; got %q", got)
	}

	// Child has a hard dep on parent. Gate must pass.
	addFeature(t, s, "child", store.StateImplementing, []store.Dependency{
		{Slug: "parent", Kind: store.DependencyKindHard},
	})
	if err := CheckDependencyGate(s, "child"); err != nil {
		t.Fatalf("apply gate must ignore parent freshness (ADR-013 D2); got: %v", err)
	}
}

// Sibling: parent is applied but has Verify=nil (never-verified). Gate
// still passes.
func TestDependencyGate_IgnoresParentNeverVerified(t *testing.T) {
	s := gateTestEnv(t, true)
	addFeature(t, s, "parent", store.StateApplied, nil)
	pst, _ := s.LoadFeatureStatus("parent")
	if pst.Verify != nil {
		t.Fatalf("expected parent.Verify nil by default; got %+v", pst.Verify)
	}
	addFeature(t, s, "child", store.StateImplementing, []store.Dependency{
		{Slug: "parent", Kind: store.DependencyKindHard},
	})
	if err := CheckDependencyGate(s, "child"); err != nil {
		t.Fatalf("apply gate must ignore never-verified parents (ADR-013 D2); got: %v", err)
	}
}
