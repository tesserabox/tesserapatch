package workflow

import (
	"reflect"
	"testing"

	"github.com/tesseracode/tesserapatch/internal/store"
)

// TestComposeLabels_UpstreamedChild_NoLabels — F3: once a child's own
// reconcile outcome is ReconcileUpstreamed it is being retired and
// parent-derived labels are misleading. ComposeLabels must early-return
// nil regardless of parent state.
func TestComposeLabels_UpstreamedChild_NoLabels(t *testing.T) {
	s := planTestEnv(t, true)
	addPlanFeature(t, s, "parent", nil)
	addPlanFeature(t, s, "child", []store.Dependency{
		{Slug: "parent", Kind: store.DependencyKindHard},
	})

	// Parent in a state that would normally produce blocked-by-parent.
	setParentState(t, s, "parent", store.StateBlocked, store.ReconcileBlocked, "")

	// Child is upstreamed: per ADR-011 / F3, NO labels.
	child, _ := s.LoadFeatureStatus("child")
	child.State = store.StateUpstreamMerged
	child.Reconcile.Outcome = store.ReconcileUpstreamed
	if err := s.SaveFeatureStatus(child); err != nil {
		t.Fatalf("SaveFeatureStatus: %v", err)
	}

	got, err := ComposeLabels(s, "child")
	if err != nil {
		t.Fatalf("ComposeLabels: %v", err)
	}
	if got != nil {
		t.Fatalf("upstreamed child must surface no labels (F3); got %v", got)
	}
}

// TestComposeLabels_NonSuppressedOutcome_StillProducesLabels — F3 must
// NOT over-suppress. A child whose own outcome is something other than
// the retired-set (e.g. ReconcileReapplied) must still produce
// parent-derived labels when applicable.
func TestComposeLabels_NonSuppressedOutcome_StillProducesLabels(t *testing.T) {
	s := planTestEnv(t, true)
	addPlanFeature(t, s, "parent", nil)
	addPlanFeature(t, s, "child", []store.Dependency{
		{Slug: "parent", Kind: store.DependencyKindHard},
	})

	// Parent recently changed → stale-parent-applied applies.
	setChildReconcile(t, s, "child", "2025-01-01T00:00:00Z")
	setParentState(t, s, "parent", store.StateApplied, store.ReconcileReapplied, "2025-02-01T00:00:00Z")

	// Child intrinsic outcome: Reapplied (NOT retired).
	child, _ := s.LoadFeatureStatus("child")
	child.Reconcile.Outcome = store.ReconcileReapplied
	if err := s.SaveFeatureStatus(child); err != nil {
		t.Fatalf("SaveFeatureStatus: %v", err)
	}

	got, _ := ComposeLabels(s, "child")
	// Slice B: non-retired child surfaces M14.3 labels AND the
	// freshness overlay (`never-verified` for unverified children).
	want := []store.ReconcileLabel{store.LabelNeverVerified, store.LabelStaleParentApplied}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("non-retired child must surface labels; got %v, want %v", got, want)
	}
}
