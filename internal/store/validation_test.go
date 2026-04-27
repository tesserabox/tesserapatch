package store

import (
	"errors"
	"testing"
)

// helper: init store and add bare-bones features at known states.
func newStoreWith(t *testing.T, features map[string]FeatureState) *Store {
	t.Helper()
	tmp := t.TempDir()
	s, err := Init(tmp)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	for slug, state := range features {
		if _, err := s.AddFeature(AddFeatureInput{Title: slug, Slug: slug, Request: slug}); err != nil {
			t.Fatalf("AddFeature %s: %v", slug, err)
		}
		if state != "" && state != StateRequested {
			st, err := s.LoadFeatureStatus(slug)
			if err != nil {
				t.Fatalf("LoadFeatureStatus %s: %v", slug, err)
			}
			st.State = state
			if err := s.SaveFeatureStatus(st); err != nil {
				t.Fatalf("SaveFeatureStatus %s: %v", slug, err)
			}
		}
	}
	return s
}

func TestValidateDependencies_SelfDependency(t *testing.T) {
	s := newStoreWith(t, map[string]FeatureState{"alpha": StateApplied})
	err := ValidateDependencies(s, "alpha", []Dependency{{Slug: "alpha", Kind: DependencyKindHard}})
	if !errors.Is(err, ErrSelfDependency) {
		t.Fatalf("want ErrSelfDependency, got %v", err)
	}
}

func TestValidateDependencies_AllowsValid(t *testing.T) {
	s := newStoreWith(t, map[string]FeatureState{
		"parent": StateApplied,
		"child":  StateRequested,
	})
	if err := ValidateDependencies(s, "child", []Dependency{{Slug: "parent", Kind: DependencyKindHard}}); err != nil {
		t.Fatalf("want clean validation, got %v", err)
	}
}

func TestValidateDependencies_DanglingRef(t *testing.T) {
	s := newStoreWith(t, map[string]FeatureState{"child": StateRequested})
	err := ValidateDependencies(s, "child", []Dependency{{Slug: "ghost", Kind: DependencyKindHard}})
	if !errors.Is(err, ErrDanglingDependency) {
		t.Fatalf("want ErrDanglingDependency, got %v", err)
	}
}

func TestValidateDependencies_KindConflict(t *testing.T) {
	s := newStoreWith(t, map[string]FeatureState{
		"parent": StateApplied,
		"child":  StateRequested,
	})
	deps := []Dependency{
		{Slug: "parent", Kind: DependencyKindHard},
		{Slug: "parent", Kind: DependencyKindSoft},
	}
	err := ValidateDependencies(s, "child", deps)
	if !errors.Is(err, ErrKindConflict) {
		t.Fatalf("want ErrKindConflict, got %v", err)
	}
}

func TestValidateDependencies_DuplicateSameKindAllowed(t *testing.T) {
	s := newStoreWith(t, map[string]FeatureState{
		"parent": StateApplied,
		"child":  StateRequested,
	})
	deps := []Dependency{
		{Slug: "parent", Kind: DependencyKindHard},
		{Slug: "parent", Kind: DependencyKindHard},
	}
	if err := ValidateDependencies(s, "child", deps); err != nil {
		t.Fatalf("duplicate same-kind should be allowed, got %v", err)
	}
}

func TestValidateDependencies_Cycle(t *testing.T) {
	s := newStoreWith(t, map[string]FeatureState{
		"alpha": StateApplied,
		"beta":  StateApplied,
	})
	// Make alpha depend on beta first (clean).
	st, _ := s.LoadFeatureStatus("alpha")
	st.DependsOn = []Dependency{{Slug: "beta", Kind: DependencyKindHard}}
	if err := s.SaveFeatureStatus(st); err != nil {
		t.Fatal(err)
	}
	// Now propose beta → alpha — completes cycle.
	err := ValidateDependencies(s, "beta", []Dependency{{Slug: "alpha", Kind: DependencyKindHard}})
	if !errors.Is(err, ErrCycle) {
		t.Fatalf("want ErrCycle, got %v", err)
	}
}

func TestValidateDependencies_AcyclicAdditionAllowed(t *testing.T) {
	s := newStoreWith(t, map[string]FeatureState{
		"alpha": StateApplied,
		"beta":  StateApplied,
		"gamma": StateRequested,
	})
	// gamma → beta → alpha is a clean linear chain.
	st, _ := s.LoadFeatureStatus("beta")
	st.DependsOn = []Dependency{{Slug: "alpha", Kind: DependencyKindHard}}
	if err := s.SaveFeatureStatus(st); err != nil {
		t.Fatal(err)
	}
	if err := ValidateDependencies(s, "gamma", []Dependency{{Slug: "beta", Kind: DependencyKindHard}}); err != nil {
		t.Fatalf("clean chain should validate, got %v", err)
	}
}

func TestValidateDependencies_SatisfiedByRequiresUpstream(t *testing.T) {
	s := newStoreWith(t, map[string]FeatureState{
		"parent": StateApplied, // not upstream_merged
		"child":  StateRequested,
	})
	deps := []Dependency{{Slug: "parent", Kind: DependencyKindHard, SatisfiedBy: "deadbeef"}}
	err := ValidateDependencies(s, "child", deps)
	if !errors.Is(err, ErrSatisfiedByRequiresUpstream) {
		t.Fatalf("want ErrSatisfiedByRequiresUpstream, got %v", err)
	}
}

func TestValidateDependencies_SatisfiedByOnUpstreamMerged(t *testing.T) {
	defer stubIsAncestor(t, true, nil)()
	s := newStoreWith(t, map[string]FeatureState{
		"parent": StateUpstreamMerged,
		"child":  StateRequested,
	})
	deps := []Dependency{{Slug: "parent", Kind: DependencyKindHard, SatisfiedBy: "deadbeef"}}
	if err := ValidateDependencies(s, "child", deps); err != nil {
		t.Fatalf("satisfied_by on upstream_merged parent must be allowed, got %v", err)
	}
}

// stubIsAncestor swaps the package-level isAncestor hook for tests so
// validation can be exercised without standing up a real git repo. The
// returned func restores the original (use with t.Cleanup or defer).
func stubIsAncestor(t *testing.T, ok bool, err error) func() {
	t.Helper()
	prev := isAncestor
	isAncestor = func(repoRoot, ancestor, descendant string) (bool, error) {
		return ok, err
	}
	t.Cleanup(func() { isAncestor = prev })
	return func() { isAncestor = prev }
}

func TestValidateDependencies_SatisfiedByReachable(t *testing.T) {
	defer stubIsAncestor(t, true, nil)()
	s := newStoreWith(t, map[string]FeatureState{
		"parent": StateUpstreamMerged,
		"child":  StateRequested,
	})
	deps := []Dependency{{Slug: "parent", Kind: DependencyKindHard, SatisfiedBy: "abc1234"}}
	if err := ValidateDependencies(s, "child", deps); err != nil {
		t.Fatalf("reachable satisfied_by SHA must validate, got %v", err)
	}
}

func TestValidateDependencies_SatisfiedByUnreachable(t *testing.T) {
	defer stubIsAncestor(t, false, nil)()
	s := newStoreWith(t, map[string]FeatureState{
		"parent": StateUpstreamMerged,
		"child":  StateRequested,
	})
	deps := []Dependency{{Slug: "parent", Kind: DependencyKindHard, SatisfiedBy: "fabricated"}}
	err := ValidateDependencies(s, "child", deps)
	if !errors.Is(err, ErrSatisfiedBySHANotReachable) {
		t.Fatalf("want ErrSatisfiedBySHANotReachable, got %v", err)
	}
}

func TestValidateAllFeatures_SatisfiedByUnreachable(t *testing.T) {
	defer stubIsAncestor(t, false, nil)()
	s := newStoreWith(t, map[string]FeatureState{
		"parent": StateUpstreamMerged,
		"child":  StateRequested,
	})
	st, _ := s.LoadFeatureStatus("child")
	st.DependsOn = []Dependency{{Slug: "parent", Kind: DependencyKindHard, SatisfiedBy: "fabricated"}}
	if err := s.SaveFeatureStatus(st); err != nil {
		t.Fatal(err)
	}
	errs := ValidateAllFeatures(s)
	found := false
	for _, e := range errs {
		if errors.Is(e, ErrSatisfiedBySHANotReachable) {
			found = true
		}
	}
	if !found {
		t.Fatalf("want ErrSatisfiedBySHANotReachable in %v", errs)
	}
}

// TestValidateDependencies_SatisfiedByGitError pins the "bogus ref / git failure"
// branch: when isAncestor returns a real error (not just (false, nil)), the
// validator must surface a wrapped error that includes (1) the dependent slug,
// (2) the parent slug, and (3) the underlying git error — and must NOT collapse
// to ErrSatisfiedBySHANotReachable, because the two signals mean different things
// (unreachable = forged SHA; git error = malformed ref or env break).
func TestValidateDependencies_SatisfiedByGitError(t *testing.T) {
	gitErr := errors.New("fatal: bad revision 'fabricated'")
	defer stubIsAncestor(t, false, gitErr)()
	s := newStoreWith(t, map[string]FeatureState{
		"parent": StateUpstreamMerged,
		"child":  StateRequested,
	})
	deps := []Dependency{{Slug: "parent", Kind: DependencyKindHard, SatisfiedBy: "fabricated"}}
	err := ValidateDependencies(s, "child", deps)
	if err == nil {
		t.Fatal("want error from git failure path, got nil")
	}
	if errors.Is(err, ErrSatisfiedBySHANotReachable) {
		t.Fatalf("git error must NOT collapse to ErrSatisfiedBySHANotReachable, got %v", err)
	}
	if !errors.Is(err, gitErr) {
		t.Fatalf("want wrapped underlying git error, got %v", err)
	}
	msg := err.Error()
	for _, want := range []string{"child", "parent", "fabricated"} {
		if !contains(msg, want) {
			t.Fatalf("error %q must include %q for context", msg, want)
		}
	}
}

func TestValidateAllFeatures_SatisfiedByGitError(t *testing.T) {
	gitErr := errors.New("fatal: bad revision 'fabricated'")
	defer stubIsAncestor(t, false, gitErr)()
	s := newStoreWith(t, map[string]FeatureState{
		"parent": StateUpstreamMerged,
		"child":  StateRequested,
	})
	st, _ := s.LoadFeatureStatus("child")
	st.DependsOn = []Dependency{{Slug: "parent", Kind: DependencyKindHard, SatisfiedBy: "fabricated"}}
	if err := s.SaveFeatureStatus(st); err != nil {
		t.Fatal(err)
	}
	errs := ValidateAllFeatures(s)
	if len(errs) == 0 {
		t.Fatal("want at least one error from git failure path, got none")
	}
	var matched error
	for _, e := range errs {
		if errors.Is(e, gitErr) {
			matched = e
		}
		if errors.Is(e, ErrSatisfiedBySHANotReachable) {
			t.Fatalf("git error must NOT collapse to ErrSatisfiedBySHANotReachable, got %v in %v", e, errs)
		}
	}
	if matched == nil {
		t.Fatalf("want wrapped underlying git error in %v", errs)
	}
}

func TestValidateDependencies_InvalidKind(t *testing.T) {
	s := newStoreWith(t, map[string]FeatureState{
		"parent": StateApplied,
		"child":  StateRequested,
	})
	err := ValidateDependencies(s, "child", []Dependency{{Slug: "parent", Kind: "weak"}})
	if !errors.Is(err, ErrInvalidDependencyKind) {
		t.Fatalf("want ErrInvalidDependencyKind, got %v", err)
	}
}

func TestValidateAllFeatures_SurfacesAllRules(t *testing.T) {
	s := newStoreWith(t, map[string]FeatureState{
		"good":   StateApplied,
		"selfie": StateApplied,
		"badkid": StateRequested,
		"dupkid": StateRequested,
		"satkid": StateRequested,
	})
	// selfie -> selfie (self)
	st, _ := s.LoadFeatureStatus("selfie")
	st.DependsOn = []Dependency{{Slug: "selfie", Kind: DependencyKindHard}}
	if err := s.SaveFeatureStatus(st); err != nil {
		t.Fatal(err)
	}
	// badkid -> ghost (dangling)
	st, _ = s.LoadFeatureStatus("badkid")
	st.DependsOn = []Dependency{{Slug: "ghost", Kind: DependencyKindHard}}
	if err := s.SaveFeatureStatus(st); err != nil {
		t.Fatal(err)
	}
	// dupkid -> good twice with diff kinds (kind conflict)
	st, _ = s.LoadFeatureStatus("dupkid")
	st.DependsOn = []Dependency{
		{Slug: "good", Kind: DependencyKindHard},
		{Slug: "good", Kind: DependencyKindSoft},
	}
	if err := s.SaveFeatureStatus(st); err != nil {
		t.Fatal(err)
	}
	// satkid -> good with satisfied_by (good is not upstream_merged)
	st, _ = s.LoadFeatureStatus("satkid")
	st.DependsOn = []Dependency{{Slug: "good", Kind: DependencyKindHard, SatisfiedBy: "abc"}}
	if err := s.SaveFeatureStatus(st); err != nil {
		t.Fatal(err)
	}

	errs := ValidateAllFeatures(s)
	wantSentinels := []error{
		ErrSelfDependency,
		ErrDanglingDependency,
		ErrKindConflict,
		ErrSatisfiedByRequiresUpstream,
		ErrCycle, // selfie self-edge also triggers a cycle
	}
	for _, want := range wantSentinels {
		found := false
		for _, got := range errs {
			if errors.Is(got, want) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected an error wrapping %v, got %v", want, errs)
		}
	}
}
