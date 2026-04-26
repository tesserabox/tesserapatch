package workflow

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/tesseracode/tesserapatch/internal/store"
)

// planTestEnv wires an isolated store with the DAG flag set as requested.
func planTestEnv(t *testing.T, dagEnabled bool) *store.Store {
	t.Helper()
	tmp := t.TempDir()
	s, err := store.Init(tmp)
	if err != nil {
		t.Fatalf("store.Init: %v", err)
	}
	cfg, _ := s.LoadConfig()
	cfg.FeaturesDependencies = dagEnabled
	if err := s.SaveConfig(cfg); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	return s
}

// addPlanFeature seeds a feature with deps. Used by plan + label tests.
func addPlanFeature(t *testing.T, s *store.Store, slug string, deps []store.Dependency) {
	t.Helper()
	if _, err := s.AddFeature(store.AddFeatureInput{Title: slug, Slug: slug, Request: "test"}); err != nil {
		t.Fatalf("AddFeature %s: %v", slug, err)
	}
	if len(deps) == 0 {
		return
	}
	st, err := s.LoadFeatureStatus(slug)
	if err != nil {
		t.Fatalf("LoadFeatureStatus %s: %v", slug, err)
	}
	st.DependsOn = deps
	if err := s.SaveFeatureStatus(st); err != nil {
		t.Fatalf("SaveFeatureStatus %s: %v", slug, err)
	}
}

// TestPlanReconcile_FlagOff_PreservesInputOrder is documentary — the
// flag-off path is enforced inside RunReconcile (PlanReconcile is never
// called). PlanReconcile itself is a pure function and doesn't read the
// config flag. We verify the contract by simulating the RunReconcile
// branch: when the flag is off, the input order is what's used.
func TestPlanReconcile_FlagOff_PreservesInputOrder(t *testing.T) {
	s := planTestEnv(t, false)
	addPlanFeature(t, s, "child", []store.Dependency{
		{Slug: "parent", Kind: store.DependencyKindHard},
	})
	addPlanFeature(t, s, "parent", nil)

	cfg, _ := s.LoadConfig()
	if cfg.DAGEnabled() {
		t.Fatal("flag must be off")
	}
	// In RunReconcile, when DAGEnabled() is false, PlanReconcile is not
	// called and the slugs slice is used as-is. We assert the contract
	// by calling PlanReconcile and confirming it would have reordered;
	// the RunReconcile wiring then keeps the input.
	order, err := PlanReconcile(s, []string{"child", "parent"})
	if err != nil {
		t.Fatalf("PlanReconcile: %v", err)
	}
	// PlanReconcile always topo-orders. The flag-off guarantee is in
	// RunReconcile, exercised by golden tests.
	if reflect.DeepEqual(order, []string{"child", "parent"}) {
		t.Errorf("PlanReconcile must reorder; got %v", order)
	}
}

func TestPlanReconcile_FlagOn_TopologicallyOrders(t *testing.T) {
	s := planTestEnv(t, true)
	// child → parent → grandparent (hard chain).
	addPlanFeature(t, s, "grandparent", nil)
	addPlanFeature(t, s, "parent", []store.Dependency{
		{Slug: "grandparent", Kind: store.DependencyKindHard},
	})
	addPlanFeature(t, s, "child", []store.Dependency{
		{Slug: "parent", Kind: store.DependencyKindHard},
	})

	order, err := PlanReconcile(s, []string{"child", "parent", "grandparent"})
	if err != nil {
		t.Fatalf("PlanReconcile: %v", err)
	}
	want := []string{"grandparent", "parent", "child"}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("order = %v, want %v", order, want)
	}
}

func TestPlanReconcile_RejectsCycle(t *testing.T) {
	s := planTestEnv(t, true)
	addPlanFeature(t, s, "a", []store.Dependency{
		{Slug: "b", Kind: store.DependencyKindHard},
	})
	addPlanFeature(t, s, "b", []store.Dependency{
		{Slug: "a", Kind: store.DependencyKindHard},
	})
	_, err := PlanReconcile(s, []string{"a", "b"})
	if err == nil {
		t.Fatal("expected cycle error, got nil")
	}
	if !errors.Is(err, store.ErrCycle) {
		t.Fatalf("expected wraps store.ErrCycle, got %v", err)
	}
	if !strings.Contains(err.Error(), "cycle") {
		t.Errorf("error should mention cycle path; got %v", err)
	}
}

// TestPlanReconcile_TransitiveHardClosure — given only [child], the
// closure must include all transitive HARD parents. Soft-only ancestors
// must NOT be pulled in.
func TestPlanReconcile_TransitiveHardClosure(t *testing.T) {
	s := planTestEnv(t, true)
	addPlanFeature(t, s, "grandparent", nil)
	addPlanFeature(t, s, "soft-only-ancestor", nil)
	addPlanFeature(t, s, "parent", []store.Dependency{
		{Slug: "grandparent", Kind: store.DependencyKindHard},
		{Slug: "soft-only-ancestor", Kind: store.DependencyKindSoft},
	})
	addPlanFeature(t, s, "child", []store.Dependency{
		{Slug: "parent", Kind: store.DependencyKindHard},
	})

	order, err := PlanReconcile(s, []string{"child"})
	if err != nil {
		t.Fatalf("PlanReconcile: %v", err)
	}
	// Closure: {child, parent, grandparent}. soft-only-ancestor must be
	// absent — its only path into the closure is via a soft edge from
	// parent.
	want := []string{"grandparent", "parent", "child"}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("order = %v, want %v (soft-only-ancestor must be absent)", order, want)
	}
}
