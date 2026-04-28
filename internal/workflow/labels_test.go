package workflow

import (
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/tesseracode/tesserapatch/internal/store"
)

// setParentState mutates a feature's State + (optionally) its
// Reconcile.Outcome and bumps UpdatedAt to a deterministic stamp later
// than the child's prior reconcile (so the stale-check fires when we
// want it to).
func setParentState(t *testing.T, s *store.Store, slug string, state store.FeatureState, outcome store.ReconcileOutcome, updatedAt string) {
	t.Helper()
	st, err := s.LoadFeatureStatus(slug)
	if err != nil {
		t.Fatalf("LoadFeatureStatus %s: %v", slug, err)
	}
	st.State = state
	if outcome != "" {
		st.Reconcile.Outcome = outcome
	}
	if updatedAt != "" {
		st.UpdatedAt = updatedAt
	}
	if err := s.SaveFeatureStatus(st); err != nil {
		t.Fatalf("SaveFeatureStatus %s: %v", slug, err)
	}
}

func setChildReconcile(t *testing.T, s *store.Store, slug, attemptedAt string) {
	t.Helper()
	st, err := s.LoadFeatureStatus(slug)
	if err != nil {
		t.Fatalf("LoadFeatureStatus %s: %v", slug, err)
	}
	st.Reconcile.AttemptedAt = attemptedAt
	if err := s.SaveFeatureStatus(st); err != nil {
		t.Fatalf("SaveFeatureStatus %s: %v", slug, err)
	}
}

func TestComposeLabels_FlagOff_AlwaysEmpty(t *testing.T) {
	s := planTestEnv(t, false)
	addPlanFeature(t, s, "parent", nil)
	addPlanFeature(t, s, "child", []store.Dependency{
		{Slug: "parent", Kind: store.DependencyKindHard},
	})
	setParentState(t, s, "parent", store.StateAnalyzed, "", "")
	got, err := ComposeLabels(s, "child")
	if err != nil {
		t.Fatalf("ComposeLabels: %v", err)
	}
	if got != nil {
		t.Fatalf("flag off must yield nil labels, got %v", got)
	}
}

func TestComposeLabels_NoDeps_Empty(t *testing.T) {
	s := planTestEnv(t, true)
	addPlanFeature(t, s, "loner", nil)
	got, err := ComposeLabels(s, "loner")
	if err != nil {
		t.Fatalf("ComposeLabels: %v", err)
	}
	// Slice B: a feature with no Verify record always carries
	// `never-verified`, even when it has no deps.
	want := []store.ReconcileLabel{store.LabelNeverVerified}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("no deps must yield freshness label only, got %v want %v", got, want)
	}
}

func TestComposeLabels_HardParentNotApplied_AddsWaitingOnParent(t *testing.T) {
	s := planTestEnv(t, true)
	addPlanFeature(t, s, "parent", nil)
	addPlanFeature(t, s, "child", []store.Dependency{
		{Slug: "parent", Kind: store.DependencyKindHard},
	})
	setParentState(t, s, "parent", store.StateAnalyzed, "", "")
	got, _ := ComposeLabels(s, "child")
	// Slice B: freshness label composes orthogonally with M14.3.
	want := []store.ReconcileLabel{store.LabelNeverVerified, store.LabelWaitingOnParent}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestComposeLabels_HardParentBlocked_AddsBlockedByParent(t *testing.T) {
	s := planTestEnv(t, true)
	addPlanFeature(t, s, "parent", nil)
	addPlanFeature(t, s, "child", []store.Dependency{
		{Slug: "parent", Kind: store.DependencyKindHard},
	})
	setParentState(t, s, "parent", store.StateApplied, store.ReconcileBlockedRequiresHuman, "")
	got, _ := ComposeLabels(s, "child")
	want := []store.ReconcileLabel{store.LabelBlockedByParent, store.LabelNeverVerified}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestComposeLabels_HardParentApplied_NoLabel(t *testing.T) {
	s := planTestEnv(t, true)
	addPlanFeature(t, s, "parent", nil)
	addPlanFeature(t, s, "child", []store.Dependency{
		{Slug: "parent", Kind: store.DependencyKindHard},
	})
	setParentState(t, s, "parent", store.StateApplied, store.ReconcileReapplied, "2025-01-01T00:00:00Z")
	setChildReconcile(t, s, "child", "2025-02-01T00:00:00Z") // child reconciled AFTER parent updated
	got, _ := ComposeLabels(s, "child")
	// Slice B: clean applied parent yields no M14.3 labels, but the
	// freshness overlay always contributes one.
	want := []store.ReconcileLabel{store.LabelNeverVerified}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("clean applied parent must yield only freshness label, got %v want %v", got, want)
	}
}

func TestComposeLabels_HardParentRecentlyChanged_AddsStaleParentApplied(t *testing.T) {
	s := planTestEnv(t, true)
	addPlanFeature(t, s, "parent", nil)
	addPlanFeature(t, s, "child", []store.Dependency{
		{Slug: "parent", Kind: store.DependencyKindHard},
	})
	// Child reconciled at T1; parent updated at T2 > T1.
	setChildReconcile(t, s, "child", "2025-01-01T00:00:00Z")
	setParentState(t, s, "parent", store.StateApplied, store.ReconcileReapplied, "2025-02-01T00:00:00Z")
	got, _ := ComposeLabels(s, "child")
	want := []store.ReconcileLabel{store.LabelNeverVerified, store.LabelStaleParentApplied}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestComposeLabels_SoftParentNeverProducesLabel(t *testing.T) {
	s := planTestEnv(t, true)
	addPlanFeature(t, s, "soft-parent", nil)
	addPlanFeature(t, s, "child", []store.Dependency{
		{Slug: "soft-parent", Kind: store.DependencyKindSoft},
	})
	// Try to provoke every state — soft must never emit an M14.3 label.
	want := []store.ReconcileLabel{store.LabelNeverVerified}
	for _, state := range []store.FeatureState{
		store.StateAnalyzed, store.StateBlocked, store.StateApplied,
	} {
		setParentState(t, s, "soft-parent", state, store.ReconcileBlockedRequiresHuman, "2099-01-01T00:00:00Z")
		got, _ := ComposeLabels(s, "child")
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("state=%s: soft parent must produce only freshness label, got %v want %v", state, got, want)
		}
	}
}

func TestComposeLabels_MultipleParentsStackLabels(t *testing.T) {
	s := planTestEnv(t, true)
	addPlanFeature(t, s, "p-pending", nil)
	addPlanFeature(t, s, "p-blocked", nil)
	addPlanFeature(t, s, "p-stale", nil)
	addPlanFeature(t, s, "child", []store.Dependency{
		{Slug: "p-pending", Kind: store.DependencyKindHard},
		{Slug: "p-blocked", Kind: store.DependencyKindHard},
		{Slug: "p-stale", Kind: store.DependencyKindHard},
	})
	setChildReconcile(t, s, "child", "2025-01-01T00:00:00Z")
	setParentState(t, s, "p-pending", store.StateAnalyzed, "", "")
	setParentState(t, s, "p-blocked", store.StateApplied, store.ReconcileBlockedRequiresHuman, "")
	setParentState(t, s, "p-stale", store.StateApplied, store.ReconcileReapplied, "2025-02-01T00:00:00Z")

	got, _ := ComposeLabels(s, "child")
	want := []store.ReconcileLabel{
		store.LabelBlockedByParent,
		store.LabelNeverVerified,
		store.LabelStaleParentApplied,
		store.LabelWaitingOnParent,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v (alphabetical)", got, want)
	}
	// Confirm sort is alphabetical.
	if !sort.SliceIsSorted(got, func(i, j int) bool { return got[i] < got[j] }) {
		t.Errorf("labels must be sorted alphabetically; got %v", got)
	}
}

func TestComposeLabels_DeterministicOrder(t *testing.T) {
	s := planTestEnv(t, true)
	addPlanFeature(t, s, "p1", nil)
	addPlanFeature(t, s, "p2", nil)
	addPlanFeature(t, s, "p3", nil)
	addPlanFeature(t, s, "child", []store.Dependency{
		{Slug: "p1", Kind: store.DependencyKindHard},
		{Slug: "p2", Kind: store.DependencyKindHard},
		{Slug: "p3", Kind: store.DependencyKindHard},
	})
	setChildReconcile(t, s, "child", "2025-01-01T00:00:00Z")
	setParentState(t, s, "p1", store.StateAnalyzed, "", "")
	setParentState(t, s, "p2", store.StateApplied, store.ReconcileBlockedRequiresHuman, "")
	setParentState(t, s, "p3", store.StateApplied, store.ReconcileReapplied, "2025-02-01T00:00:00Z")

	first, _ := ComposeLabels(s, "child")
	for i := 0; i < 50; i++ {
		got, _ := ComposeLabels(s, "child")
		if !reflect.DeepEqual(got, first) {
			t.Fatalf("iter %d: order non-deterministic — first %v, got %v", i, first, got)
		}
	}
}

// TestComposeLabels_ReadsStatusJsonNotSessionArtifact is the LOAD-BEARING
// adversarial test for the M14.3 reviewer guard (ADR-010 D5, ADR-011 D6).
//
// We seed:
//   - Parent's status.json:        Outcome=ReconcileReapplied (clean)
//   - Parent's reconcile-session.json: outcome="blocked-requires-human"
//
// A correct implementation reads status.json, sees a clean parent, and
// emits NO labels. A buggy implementation that consults the session
// artifact would emit LabelBlockedByParent. The two artifacts can drift
// post-accept (status flips to reapplied while the session artifact
// preserves the pre-accept verdict), so this test is the canary for
// "wrong source of truth" regressions.
func TestComposeLabels_ReadsStatusJsonNotSessionArtifact(t *testing.T) {
	s := planTestEnv(t, true)
	addPlanFeature(t, s, "parent", nil)
	addPlanFeature(t, s, "child", []store.Dependency{
		{Slug: "parent", Kind: store.DependencyKindHard},
	})
	// status.json: clean reapplied parent. UpdatedAt before child's
	// reconcile so stale-parent-applied does NOT fire.
	setParentState(t, s, "parent", store.StateApplied, store.ReconcileReapplied, "2025-01-01T00:00:00Z")
	setChildReconcile(t, s, "child", "2025-02-01T00:00:00Z")

	// Adversarial session artifact: a misleading "blocked" verdict.
	// If ComposeLabels ever reads this file, the test fails.
	misleading := `{
  "slug": "parent",
  "outcome": "blocked-requires-human",
  "phase": "phase-3.5-provider-resolve",
  "notes": ["this is a stale audit artifact; status.json is authoritative"]
}
`
	if err := s.WriteArtifact("parent", "reconcile-session.json", misleading); err != nil {
		t.Fatalf("seed adversarial session artifact: %v", err)
	}
	// Sanity: artifact really exists.
	if _, err := s.ReadFeatureFile("parent", filepath.Join("artifacts", "reconcile-session.json")); err != nil {
		t.Fatalf("session artifact missing: %v", err)
	}

	got, err := ComposeLabels(s, "child")
	if err != nil {
		t.Fatalf("ComposeLabels: %v", err)
	}
	// Slice B: clean parent yields no M14.3 labels; the only label
	// surfaced is the freshness overlay (`never-verified`). If
	// composeLabelsFromStatus ever consulted the session artifact, the
	// child would carry LabelBlockedByParent in addition to the
	// freshness label.
	want := []store.ReconcileLabel{store.LabelNeverVerified}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ComposeLabels read the SESSION ARTIFACT, not status.json. "+
			"Per ADR-010 D5 / ADR-011 D6, parent verdicts must come from "+
			"status.Reconcile.Outcome only. Got labels=%v (expected %v).", got, want)
	}
}

// TestComposeLabels_MissingParent_BlockedByParent — a hard parent that
// can't be loaded (e.g. removed without --cascade) is treated as a
// blocker, mirroring the dependency-gate behaviour.
func TestComposeLabels_MissingParent_BlockedByParent(t *testing.T) {
	s := planTestEnv(t, true)
	addPlanFeature(t, s, "child", []store.Dependency{
		{Slug: "ghost-parent", Kind: store.DependencyKindHard},
	})
	got, _ := ComposeLabels(s, "child")
	want := []store.ReconcileLabel{store.LabelBlockedByParent, store.LabelNeverVerified}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}
