package store

import (
	"os"
	"reflect"
	"testing"
)

// preM14StatusFixture is a minimal, valid status.json from before M14.1.
// It has NO depends_on field. After load → save, the byte output must be
// identical to this (modulo the trailing newline our writeJSON appends).
//
// This guards the omitempty annotation on FeatureStatus.DependsOn — if it
// regresses to a non-omit form the JSON would gain `"depends_on": null`
// and break every existing on-disk fixture.
const preM14StatusFixture = `{
  "id": "demo-feature",
  "slug": "demo-feature",
  "title": "Demo Feature",
  "state": "applied",
  "compatibility": "compatible",
  "requested_at": "2025-01-01T00:00:00Z",
  "updated_at": "2025-01-02T00:00:00Z",
  "last_command": "apply",
  "apply": {
    "prepared_at": "2025-01-02T00:00:00Z",
    "started_at": "2025-01-02T00:00:01Z",
    "completed_at": "2025-01-02T00:00:02Z",
    "base_commit": "abc123",
    "has_patch": true,
    "has_recipe": true
  },
  "reconcile": {}
}
`

func TestRoundtrip_PreM14StatusByteIdentity(t *testing.T) {
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
		t.Fatalf("LoadFeatureStatus: %v", err)
	}
	if loaded.DependsOn != nil {
		t.Fatalf("pre-M14 fixture should yield nil DependsOn, got %v", loaded.DependsOn)
	}
	// Save back without mutation.
	if err := s.SaveFeatureStatus(loaded); err != nil {
		t.Fatalf("SaveFeatureStatus: %v", err)
	}
	got, err := os.ReadFile(statusPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != preM14StatusFixture {
		t.Fatalf("byte-identity round-trip failed.\nwant:\n%s\ngot:\n%s", preM14StatusFixture, string(got))
	}
}

func TestRoundtrip_EmptyDependsOnOmitted(t *testing.T) {
	tmp := t.TempDir()
	s, err := Init(tmp)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddFeature(AddFeatureInput{Title: "demo", Slug: "demo", Request: "x"}); err != nil {
		t.Fatal(err)
	}
	st, _ := s.LoadFeatureStatus("demo")
	// Explicit empty (not nil) should still omit (omitempty treats len==0 as empty for slices).
	st.DependsOn = []Dependency{}
	if err := s.SaveFeatureStatus(st); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(s.featureStatusPath("demo"))
	if containsLiteral(string(raw), "depends_on") {
		t.Fatalf("empty depends_on must be omitted from JSON, got:\n%s", string(raw))
	}
}

func TestRoundtrip_DependsOnPreserved(t *testing.T) {
	tmp := t.TempDir()
	s, err := Init(tmp)
	if err != nil {
		t.Fatal(err)
	}
	for _, slug := range []string{"parent-a", "parent-b", "child"} {
		if _, err := s.AddFeature(AddFeatureInput{Title: slug, Slug: slug, Request: "x"}); err != nil {
			t.Fatal(err)
		}
	}
	st, _ := s.LoadFeatureStatus("child")
	st.DependsOn = []Dependency{
		{Slug: "parent-a", Kind: DependencyKindHard},
		{Slug: "parent-b", Kind: DependencyKindSoft, SatisfiedBy: ""},
	}
	if err := s.SaveFeatureStatus(st); err != nil {
		t.Fatal(err)
	}
	round, err := s.LoadFeatureStatus("child")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(round.DependsOn, st.DependsOn) {
		t.Fatalf("depends_on round-trip mismatch:\n want %#v\n got  %#v", st.DependsOn, round.DependsOn)
	}
}

func TestConfig_FeaturesDependenciesRoundtrip(t *testing.T) {
	tmp := t.TempDir()
	s, err := Init(tmp)
	if err != nil {
		t.Fatal(err)
	}
	cfg, _ := s.LoadConfig()
	// v0.6.0 default flip: fresh init should report DAGEnabled.
	if !cfg.DAGEnabled() {
		t.Fatal("default DAGEnabled must be true from v0.6.0")
	}
	cfg.FeaturesDependencies = false
	if err := s.SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	got, err := s.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if got.DAGEnabled() {
		t.Fatalf("expected DAGEnabled false after explicit opt-out, got cfg=%+v", got)
	}
}

func containsLiteral(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
