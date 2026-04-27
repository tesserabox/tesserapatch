package workflow

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/tesseracode/tesserapatch/internal/provider"
	"github.com/tesseracode/tesserapatch/internal/store"
)

// TestAcceptShadow_FlagOn_RefreshesLabels — after the manual accept
// path stamps Outcome=reapplied, ComposeLabels must run again so labels
// reflect the CURRENT parent state. Parents may have shifted between
// the resolver staging the shadow and the user accepting it.
func TestAcceptShadow_FlagOn_RefreshesLabels(t *testing.T) {
	s, slug := buildConflictFixture(t)

	// Enable DAG flag.
	cfg, _ := s.LoadConfig()
	cfg.FeaturesDependencies = true
	if err := s.SaveConfig(cfg); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	// Seed a hard parent in a transient state.
	if _, err := s.AddFeature(store.AddFeatureInput{Title: "wip-parent", Slug: "wip-parent", Request: "x"}); err != nil {
		t.Fatalf("AddFeature parent: %v", err)
	}
	parent, _ := s.LoadFeatureStatus("wip-parent")
	parent.State = store.StateAnalyzed
	if err := s.SaveFeatureStatus(parent); err != nil {
		t.Fatalf("save parent: %v", err)
	}
	child, _ := s.LoadFeatureStatus(slug)
	child.DependsOn = []store.Dependency{{Slug: "wip-parent", Kind: store.DependencyKindHard}}
	if err := s.SaveFeatureStatus(child); err != nil {
		t.Fatalf("save child deps: %v", err)
	}

	// Run a resolve to stage a shadow (parent transient → child carries
	// LabelWaitingOnParent).
	prov := &scriptedProvider{
		responses: []string{`{"decision":"unclear","reasoning":"x"}`},
		keyed:     map[string]string{"shared.txt": "a\nB-merged\nc\n"},
	}
	provCfg := provider.Config{Type: "openai-compatible", BaseURL: "http://x", Model: "m", AuthEnv: "X"}
	results, err := RunReconcile(context.Background(), s, []string{slug}, "HEAD", prov, provCfg,
		ReconcileOptions{Resolve: true})
	if err != nil {
		t.Fatalf("RunReconcile: %v", err)
	}

	// Find the child result; its Reconcile.Labels should already include
	// waiting-on-parent (parent transient).
	stPre, _ := s.LoadFeatureStatus(slug)
	if !hasLabel(stPre.Reconcile.Labels, store.LabelWaitingOnParent) {
		t.Fatalf("pre-accept: expected waiting-on-parent on child; got labels=%v outcome=%s results=%+v",
			stPre.Reconcile.Labels, stPre.Reconcile.Outcome, results)
	}

	// Mid-flight: parent flips to applied (it just reconciled cleanly).
	parent2, _ := s.LoadFeatureStatus("wip-parent")
	parent2.State = store.StateApplied
	parent2.Reconcile.Outcome = store.ReconcileReapplied
	parent2.UpdatedAt = "2020-01-01T00:00:00Z" // earlier than child accept stamp
	if err := s.SaveFeatureStatus(parent2); err != nil {
		t.Fatalf("flip parent: %v", err)
	}

	// Find resolved files for accept.
	resolution, _ := s.ReadFeatureFile(slug, filepath.Join("artifacts", "resolution-session.json"))
	_ = resolution // sanity touch
	resolved := []string{"shared.txt"}

	if _, err := AcceptShadow(s, slug, resolved, stPre.Reconcile.UpstreamCommit, AcceptOptions{
		Phase:            "reconcile --accept",
		ResolveSessionID: stPre.Reconcile.ResolveSession,
	}); err != nil {
		t.Fatalf("AcceptShadow: %v", err)
	}

	stPost, _ := s.LoadFeatureStatus(slug)
	if stPost.Reconcile.Outcome != store.ReconcileReapplied {
		t.Errorf("post-accept Outcome=%s, want reapplied", stPost.Reconcile.Outcome)
	}
	// Parent is now satisfied → no labels.
	if len(stPost.Reconcile.Labels) != 0 {
		t.Errorf("post-accept labels should be empty (parent satisfied), got %v", stPost.Reconcile.Labels)
	}
}

// TestAcceptShadow_FlagOff_LabelsRemainNil — flag-off byte-identity
// guard. AcceptShadow must NOT touch Reconcile.Labels when DAG is off,
// and a fresh save must round-trip without producing a "labels" key.
func TestAcceptShadow_FlagOff_LabelsRemainNil(t *testing.T) {
	s, slug := buildConflictFixture(t)

	// Flag must be off for this byte-identity guard. v0.6.0 default
	// flipped to true, so opt out explicitly.
	cfg, _ := s.LoadConfig()
	cfg.FeaturesDependencies = false
	if err := s.SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	cfg, _ = s.LoadConfig()
	if cfg.DAGEnabled() {
		t.Fatal("flag must be off after explicit opt-out")
	}

	prov := &scriptedProvider{
		responses: []string{`{"decision":"unclear","reasoning":"x"}`},
		keyed:     map[string]string{"shared.txt": "a\nB-merged\nc\n"},
	}
	provCfg := provider.Config{Type: "openai-compatible", BaseURL: "http://x", Model: "m", AuthEnv: "X"}
	if _, err := RunReconcile(context.Background(), s, []string{slug}, "HEAD", prov, provCfg,
		ReconcileOptions{Resolve: true}); err != nil {
		t.Fatalf("RunReconcile: %v", err)
	}

	stPre, _ := s.LoadFeatureStatus(slug)
	if stPre.Reconcile.Labels != nil {
		t.Fatalf("flag off: pre-accept labels must be nil, got %v", stPre.Reconcile.Labels)
	}
	resolved := []string{"shared.txt"}
	if _, err := AcceptShadow(s, slug, resolved, stPre.Reconcile.UpstreamCommit, AcceptOptions{
		Phase:            "reconcile --accept",
		ResolveSessionID: stPre.Reconcile.ResolveSession,
	}); err != nil {
		t.Fatalf("AcceptShadow: %v", err)
	}

	stPost, _ := s.LoadFeatureStatus(slug)
	if stPost.Reconcile.Labels != nil {
		t.Fatalf("flag off: post-accept labels must be nil, got %v", stPost.Reconcile.Labels)
	}
	// On-disk JSON must NOT carry the "labels" key (omitempty guard).
	statusPath := filepath.Join(s.TpatchDir(), "features", slug, "status.json")
	raw, err := os.ReadFile(statusPath)
	if err != nil {
		t.Fatalf("read status.json: %v", err)
	}
	if containsBytesLiteral(raw, []byte(`"labels"`)) {
		t.Fatalf("flag off: status.json must NOT contain \"labels\" key, got:\n%s", string(raw))
	}
}

// containsBytesLiteral is a local helper to avoid importing test
// utilities from the store package. Plain substring check.
func containsBytesLiteral(haystack, needle []byte) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		match := true
		for j := 0; j < len(needle); j++ {
			if haystack[i+j] != needle[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
