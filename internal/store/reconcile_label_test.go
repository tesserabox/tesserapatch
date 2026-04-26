package store

import (
	"os"
	"strings"
	"testing"
)

// TestReconcileSummary_EffectiveOutcome_Compound asserts the M14.3 compound
// presentation rule (ADR-011 D6): needs-human-resolution + blocked-by-parent
// label collapses to "blocked-by-parent-and-needs-resolution".
func TestReconcileSummary_EffectiveOutcome_Compound(t *testing.T) {
	r := ReconcileSummary{
		Outcome: ReconcileBlockedRequiresHuman,
		Labels:  []ReconcileLabel{LabelBlockedByParent},
	}
	if got := r.EffectiveOutcome(); got != "blocked-by-parent-and-needs-resolution" {
		t.Fatalf("EffectiveOutcome=%q, want compound", got)
	}
}

// TestReconcileSummary_EffectiveOutcome_Passthrough — every non-compound
// (Outcome, Labels) pair stringifies to the bare Outcome.
func TestReconcileSummary_EffectiveOutcome_Passthrough(t *testing.T) {
	cases := []ReconcileSummary{
		{Outcome: ReconcileReapplied, Labels: []ReconcileLabel{LabelWaitingOnParent}},
		{Outcome: ReconcileBlockedRequiresHuman, Labels: []ReconcileLabel{LabelWaitingOnParent}},
		{Outcome: ReconcileBlockedRequiresHuman, Labels: nil},
		{Outcome: ReconcileShadowAwaiting, Labels: []ReconcileLabel{LabelBlockedByParent}},
		{Outcome: ReconcileReapplied, Labels: []ReconcileLabel{LabelStaleParentApplied}},
	}
	for _, c := range cases {
		if got := c.EffectiveOutcome(); got != string(c.Outcome) {
			t.Errorf("EffectiveOutcome(%+v)=%q, want %q", c, got, c.Outcome)
		}
	}
}

// TestRoundtrip_PreM14_3StatusByteIdentity asserts that adding the Labels
// field with `omitempty` does NOT regress byte-identity round-trip against
// fixtures that pre-date M14.3. The fixture has `"reconcile": {}`; after
// load → save it must come back identical (no `"labels": null` leakage).
func TestRoundtrip_PreM14_3StatusByteIdentity(t *testing.T) {
	tmp := t.TempDir()
	s, err := Init(tmp)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if _, err := s.AddFeature(AddFeatureInput{Title: "demo-feature", Slug: "demo-feature", Request: "x"}); err != nil {
		t.Fatalf("AddFeature: %v", err)
	}
	statusPath := s.featureStatusPath("demo-feature")
	if err := os.WriteFile(statusPath, []byte(preM14StatusFixture), 0o644); err != nil {
		t.Fatal(err)
	}

	loaded, err := s.LoadFeatureStatus("demo-feature")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Reconcile.Labels != nil {
		t.Fatalf("pre-M14.3 fixture should yield nil Labels, got %v", loaded.Reconcile.Labels)
	}
	if err := s.SaveFeatureStatus(loaded); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(statusPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != preM14StatusFixture {
		t.Fatalf("byte-identity round-trip failed.\nwant:\n%s\ngot:\n%s", preM14StatusFixture, string(got))
	}
}

// TestRoundtrip_EmptyReconcileLabelsOmitted — explicit empty slice must
// also serialize without the `labels` key (omitempty treats len==0 as
// empty for slices). Guards against a future change that swaps to a
// pointer-typed field or non-omit form.
func TestRoundtrip_EmptyReconcileLabelsOmitted(t *testing.T) {
	tmp := t.TempDir()
	s, err := Init(tmp)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddFeature(AddFeatureInput{Title: "demo", Slug: "demo", Request: "x"}); err != nil {
		t.Fatal(err)
	}
	st, _ := s.LoadFeatureStatus("demo")
	st.Reconcile.Outcome = ReconcileReapplied
	st.Reconcile.Labels = []ReconcileLabel{} // explicit empty
	if err := s.SaveFeatureStatus(st); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(s.featureStatusPath("demo"))
	if strings.Contains(string(raw), "labels") {
		t.Fatalf("empty Labels must be omitted from JSON, got:\n%s", string(raw))
	}
}
