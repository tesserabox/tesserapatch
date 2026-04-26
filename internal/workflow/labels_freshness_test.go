package workflow

import (
	"reflect"
	"testing"

	"github.com/tesseracode/tesserapatch/internal/store"
)

// TestRunReconcile_ClearsStaleLabel_WhenChildRefreshed pins M14 fix-pass
// finding F2: previously, saveReconcileArtifacts called ComposeLabels
// which read the child's OLD AttemptedAt (loaded from disk) and computed
// stale-parent-applied; updateFeatureState then overwrote AttemptedAt
// with time.Now(). Result: status.json persisted with new AttemptedAt
// AND a stale-parent-applied label that referred to the timestamp the
// child no longer has.
//
// Post-fix invariant: when persisting FeatureReconcile, Labels reflect
// the same AttemptedAt that's about to be written.
func TestRunReconcile_ClearsStaleLabel_WhenChildRefreshed(t *testing.T) {
	s := planTestEnv(t, true)
	addPlanFeature(t, s, "parent", nil)
	addPlanFeature(t, s, "child", []store.Dependency{
		{Slug: "parent", Kind: store.DependencyKindHard},
	})

	// Simulate the pre-reconcile world: parent updated AT a recent time,
	// child reconciled at an older time, and (crucially) the persisted
	// child status already carries a stale-parent-applied label.
	setParentState(t, s, "parent", store.StateApplied, store.ReconcileReapplied, "2025-03-01T00:00:00Z")
	child, _ := s.LoadFeatureStatus("child")
	child.Reconcile.AttemptedAt = "2025-01-01T00:00:00Z"
	child.Reconcile.Labels = []store.ReconcileLabel{store.LabelStaleParentApplied}
	if err := s.SaveFeatureStatus(child); err != nil {
		t.Fatalf("SaveFeatureStatus child: %v", err)
	}

	// Run the persistence path that RunReconcile uses on every reconcile
	// completion. The intrinsic outcome is Reapplied (clean) — we are
	// testing label freshness, not phase logic.
	result := &ReconcileResult{
		Slug:    "child",
		Outcome: store.ReconcileReapplied,
		Phase:   "phase-2-forward-apply",
	}
	saveReconcileArtifacts(s, "child", result)
	updateFeatureState(s, "child", result)

	// Reload from disk: AttemptedAt must be fresh, Labels must NOT
	// contain stale-parent-applied (parent.UpdatedAt is now < the new
	// child.AttemptedAt that just landed).
	got, err := s.LoadFeatureStatus("child")
	if err != nil {
		t.Fatalf("LoadFeatureStatus: %v", err)
	}
	if got.Reconcile.AttemptedAt == "" || got.Reconcile.AttemptedAt == "2025-01-01T00:00:00Z" {
		t.Fatalf("AttemptedAt was not refreshed; got %q", got.Reconcile.AttemptedAt)
	}
	for _, l := range got.Reconcile.Labels {
		if l == store.LabelStaleParentApplied {
			t.Fatalf("stale-parent-applied persisted across a clean reconcile — F2 invariant violated. Labels=%v, AttemptedAt=%s, parent.UpdatedAt=2025-03-01T00:00:00Z",
				got.Reconcile.Labels, got.Reconcile.AttemptedAt)
		}
	}
}

// TestRunReconcile_PreservesOtherLabels_WhenStaleResolved — when the
// stale label resolves on a refresh but other labels still apply
// (e.g. a different parent is in a non-applied state), those other
// labels MUST remain. F2 must clear only what is no longer true; it
// must not flush the entire label set.
func TestRunReconcile_PreservesOtherLabels_WhenStaleResolved(t *testing.T) {
	s := planTestEnv(t, true)
	addPlanFeature(t, s, "p-stale", nil)
	addPlanFeature(t, s, "p-pending", nil)
	addPlanFeature(t, s, "child", []store.Dependency{
		{Slug: "p-stale", Kind: store.DependencyKindHard},
		{Slug: "p-pending", Kind: store.DependencyKindHard},
	})

	// p-stale: applied + UpdatedAt in 2025-01 (will be < child's new AttemptedAt → not stale post-fix).
	setParentState(t, s, "p-stale", store.StateApplied, store.ReconcileReapplied, "2025-01-01T00:00:00Z")
	// p-pending: still in analyze state → waiting-on-parent.
	setParentState(t, s, "p-pending", store.StateAnalyzed, "", "")

	// Pre-reconcile: child has both labels persisted.
	child, _ := s.LoadFeatureStatus("child")
	child.Reconcile.AttemptedAt = "2024-06-01T00:00:00Z"
	child.Reconcile.Labels = []store.ReconcileLabel{
		store.LabelStaleParentApplied,
		store.LabelWaitingOnParent,
	}
	if err := s.SaveFeatureStatus(child); err != nil {
		t.Fatalf("SaveFeatureStatus child: %v", err)
	}

	result := &ReconcileResult{
		Slug:    "child",
		Outcome: store.ReconcileReapplied,
		Phase:   "phase-2-forward-apply",
	}
	saveReconcileArtifacts(s, "child", result)
	updateFeatureState(s, "child", result)

	got, _ := s.LoadFeatureStatus("child")
	want := []store.ReconcileLabel{store.LabelWaitingOnParent}
	if !reflect.DeepEqual(got.Reconcile.Labels, want) {
		t.Fatalf("labels: got %v, want %v (stale must clear, waiting-on-parent must remain)",
			got.Reconcile.Labels, want)
	}
}
