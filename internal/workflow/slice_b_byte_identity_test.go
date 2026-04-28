package workflow

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/tesseracode/tesserapatch/internal/store"
)

// Slice B byte-identity (ADR-013 D4 / v0.6.1 contract):
// status.json with no Verify field must round-trip byte-identically
// through Save → Load → Save. Freshness labels must NEVER appear in
// the persisted Reconcile.Labels array.
func TestSliceB_ByteIdentity_NoVerifyField(t *testing.T) {
	s := planTestEnv(t, true)
	addPlanFeature(t, s, "demo", nil)

	statusPath := filepath.Join(s.TpatchDir(), "features", "demo", "status.json")
	pre, err := os.ReadFile(statusPath)
	if err != nil {
		t.Fatalf("read status.json: %v", err)
	}

	// Round-trip: load → save → reread.
	st, err := s.LoadFeatureStatus("demo")
	if err != nil {
		t.Fatalf("LoadFeatureStatus: %v", err)
	}
	if st.Verify != nil {
		t.Fatalf("default-seeded feature must have Verify==nil; got %+v", st.Verify)
	}
	if err := s.SaveFeatureStatus(st); err != nil {
		t.Fatalf("SaveFeatureStatus: %v", err)
	}
	post, err := os.ReadFile(statusPath)
	if err != nil {
		t.Fatalf("re-read status.json: %v", err)
	}
	if !bytes.Equal(pre, post) {
		t.Fatalf("byte-identity violated:\npre = %s\npost = %s", pre, post)
	}

	// JSON must not contain a "verify" key (omitempty guard).
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(post, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, present := raw["verify"]; present {
		t.Fatalf("status.json must omit \"verify\" key when Verify==nil")
	}

	// Freshness derivation produces a label, but persistence sites
	// strip it. Verify ComposeLabels is purely additive at read time.
	labels := composeLabelsFromStatus(s, st)
	sawNeverVerified := false
	for _, l := range labels {
		if l == store.LabelNeverVerified {
			sawNeverVerified = true
		}
	}
	if !sawNeverVerified {
		t.Fatalf("expected never-verified in derived labels for Verify==nil; got %v", labels)
	}

	// And StripFreshnessLabels removes it.
	stripped := StripFreshnessLabels(labels)
	for _, l := range stripped {
		if IsFreshnessLabel(l) {
			t.Fatalf("StripFreshnessLabels left freshness label %q in result", l)
		}
	}
}

// Persistence sites must not write freshness labels into status.json.
// We exercise this by directly invoking the same persistence helpers
// the reconcile / accept paths use, with a status whose composed
// labels include a freshness entry.
func TestSliceB_PersistedLabels_NeverContainFreshness(t *testing.T) {
	s := planTestEnv(t, true)
	addPlanFeature(t, s, "demo", nil)

	st, _ := s.LoadFeatureStatus("demo")
	// Synthesize a label set that mixes M14.3 + freshness.
	st.Reconcile.Labels = StripFreshnessLabels([]store.ReconcileLabel{
		store.LabelStaleParentApplied,
		store.LabelNeverVerified,
		store.LabelVerifiedFresh,
	})
	if err := s.SaveFeatureStatus(st); err != nil {
		t.Fatal(err)
	}

	statusPath := filepath.Join(s.TpatchDir(), "features", "demo", "status.json")
	raw, err := os.ReadFile(statusPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, freshness := range []string{
		"never-verified", "verified-fresh", "verified-stale", "verify-failed",
	} {
		if bytes.Contains(raw, []byte(freshness)) {
			t.Fatalf("freshness label %q leaked into persisted status.json:\n%s",
				freshness, raw)
		}
	}
}
