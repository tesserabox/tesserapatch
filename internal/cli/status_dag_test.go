package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tesseracode/tesserapatch/internal/store"
)

// helper: init+enable flag, return tmpDir.
func newDAGTestRepo(t *testing.T) (string, *store.Store) {
	t.Helper()
	tmp := t.TempDir()
	gitInitTestRepo(t, tmp)
	runCmd("init", "--path", tmp)
	s, err := store.Open(tmp)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	cfg, _ := s.LoadConfig()
	cfg.FeaturesDependencies = true
	if err := s.SaveConfig(cfg); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	return tmp, s
}

func TestStatusDag_EmptyRepo(t *testing.T) {
	tmp, _ := newDAGTestRepo(t)
	out, _, code := runCmd("status", "--dag", "--path", tmp)
	if code != 0 {
		t.Fatalf("exit %d, stdout=%q", code, out)
	}
	if !strings.Contains(out, "DAG (all features)") {
		t.Fatalf("missing DAG header: %q", out)
	}
	if !strings.Contains(out, "(no features)") {
		t.Fatalf("expected '(no features)' on empty DAG, got %q", out)
	}
}

func TestStatusDag_RendersHardAndSoftEdges(t *testing.T) {
	tmp, s := newDAGTestRepo(t)
	runCmd("add", "--path", tmp, "--slug", "alpha", "Alpha root")
	runCmd("add", "--path", tmp, "--slug", "beta", "Beta middle")
	runCmd("add", "--path", tmp, "--slug", "gamma", "Gamma leaf")

	beta, _ := s.LoadFeatureStatus("beta")
	beta.DependsOn = []store.Dependency{{Slug: "alpha", Kind: store.DependencyKindHard}}
	if err := s.SaveFeatureStatus(beta); err != nil {
		t.Fatal(err)
	}
	gamma, _ := s.LoadFeatureStatus("gamma")
	gamma.DependsOn = []store.Dependency{{Slug: "beta", Kind: store.DependencyKindSoft}}
	if err := s.SaveFeatureStatus(gamma); err != nil {
		t.Fatal(err)
	}

	out, _, code := runCmd("status", "--dag", "--path", tmp)
	if code != 0 {
		t.Fatalf("exit %d, out=%q", code, out)
	}
	if !strings.Contains(out, "alpha [requested]") {
		t.Fatalf("missing alpha node line: %q", out)
	}
	if !strings.Contains(out, "─► beta") {
		t.Fatalf("expected hard arrow ─► to beta, got %q", out)
	}
	if !strings.Contains(out, "┄► gamma") {
		t.Fatalf("expected soft arrow ┄► to gamma, got %q", out)
	}
}

func TestStatusDag_CycleSafe(t *testing.T) {
	// Bypass validation by writing depends_on on disk directly so a
	// cycle exists. The renderer must NOT hang and must surface a
	// warning + flat list.
	tmp, s := newDAGTestRepo(t)
	runCmd("add", "--path", tmp, "--slug", "a", "A")
	runCmd("add", "--path", tmp, "--slug", "b", "B")

	a, _ := s.LoadFeatureStatus("a")
	a.DependsOn = []store.Dependency{{Slug: "b", Kind: store.DependencyKindHard}}
	if err := s.SaveFeatureStatus(a); err != nil {
		t.Fatal(err)
	}
	b, _ := s.LoadFeatureStatus("b")
	b.DependsOn = []store.Dependency{{Slug: "a", Kind: store.DependencyKindHard}}
	if err := s.SaveFeatureStatus(b); err != nil {
		t.Fatal(err)
	}

	out, _, code := runCmd("status", "--dag", "--path", tmp)
	if code != 0 {
		t.Fatalf("exit %d, out=%q", code, out)
	}
	if !strings.Contains(out, "cycle detected") {
		t.Fatalf("expected cycle warning, got %q", out)
	}
}

func TestStatusDag_ScopedSubset(t *testing.T) {
	tmp, s := newDAGTestRepo(t)
	runCmd("add", "--path", tmp, "--slug", "root1", "Root one")
	runCmd("add", "--path", tmp, "--slug", "root2", "Root two")
	runCmd("add", "--path", tmp, "--slug", "child", "Child")

	child, _ := s.LoadFeatureStatus("child")
	child.DependsOn = []store.Dependency{{Slug: "root1", Kind: store.DependencyKindHard}}
	if err := s.SaveFeatureStatus(child); err != nil {
		t.Fatal(err)
	}

	out, _, code := runCmd("status", "--dag", "--path", tmp, "child")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(out, "root1") || !strings.Contains(out, "child") {
		t.Fatalf("scoped DAG missing expected slugs: %q", out)
	}
	if strings.Contains(out, "root2") {
		t.Fatalf("scoped DAG must not include unrelated root2: %q", out)
	}
}

func TestStatusDag_RendersLabelsAndOutcome(t *testing.T) {
	tmp, s := newDAGTestRepo(t)
	runCmd("add", "--path", tmp, "--slug", "p", "Parent")
	runCmd("add", "--path", tmp, "--slug", "c", "Child")
	c, _ := s.LoadFeatureStatus("c")
	c.DependsOn = []store.Dependency{{Slug: "p", Kind: store.DependencyKindHard}}
	c.Reconcile.Outcome = store.ReconcileBlockedRequiresHuman
	c.Reconcile.Labels = []store.ReconcileLabel{store.LabelBlockedByParent}
	if err := s.SaveFeatureStatus(c); err != nil {
		t.Fatal(err)
	}

	out, _, code := runCmd("status", "--dag", "--path", tmp)
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(out, "blocked-by-parent-and-needs-resolution") {
		t.Fatalf("expected compound outcome, got %q", out)
	}
	if !strings.Contains(out, "blocked-by-parent, never-verified") {
		t.Fatalf("expected merged label suffix, got %q", out)
	}
}

func TestStatusDag_JSONShape(t *testing.T) {
	tmp, s := newDAGTestRepo(t)
	runCmd("add", "--path", tmp, "--slug", "p", "P")
	runCmd("add", "--path", tmp, "--slug", "c", "C")
	c, _ := s.LoadFeatureStatus("c")
	c.DependsOn = []store.Dependency{{Slug: "p", Kind: store.DependencyKindHard}}
	if err := s.SaveFeatureStatus(c); err != nil {
		t.Fatal(err)
	}

	out, _, code := runCmd("status", "--dag", "--json", "--path", tmp)
	if code != 0 {
		t.Fatalf("exit %d, out=%q", code, out)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if payload["scope"] != "all" {
		t.Fatalf("expected scope=all, got %v", payload["scope"])
	}
	feats, ok := payload["features"].([]any)
	if !ok || len(feats) != 2 {
		t.Fatalf("expected 2 features, got %v", payload["features"])
	}
	roots, _ := payload["roots"].([]any)
	if len(roots) != 1 || roots[0] != "p" {
		t.Fatalf("expected roots=[p], got %v", roots)
	}
}

func TestStatusDag_EnsureNoStaleConfigSideEffects(t *testing.T) {
	// Sanity: --dag must not crash when status.json files are
	// missing for some directories (defensive).
	tmp, _ := newDAGTestRepo(t)
	runCmd("add", "--path", tmp, "--slug", "ok", "OK")
	// Create a junk feature dir without status.json.
	if err := os.MkdirAll(filepath.Join(tmp, ".tpatch", "features", "junk"), 0o755); err != nil {
		t.Fatal(err)
	}
	out, _, code := runCmd("status", "--dag", "--path", tmp)
	if code != 0 {
		t.Fatalf("exit %d, out=%q", code, out)
	}
	if !strings.Contains(out, "ok") {
		t.Fatalf("expected ok in output: %q", out)
	}
}

func TestStatus_SurfacesDanglingDepWarning(t *testing.T) {
	tmp, s := newDAGTestRepo(t)
	runCmd("add", "--path", tmp, "--slug", "child", "C")
	c, _ := s.LoadFeatureStatus("child")
	c.DependsOn = []store.Dependency{{Slug: "ghost", Kind: store.DependencyKindHard}}
	if err := s.SaveFeatureStatus(c); err != nil {
		t.Fatal(err)
	}
	out, _, code := runCmd("status", "--path", tmp)
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(out, "DAG warnings") || !strings.Contains(out, "ghost") {
		t.Fatalf("expected dangling-dep warning, got %q", out)
	}
}

func TestStatus_SurfacesCycleWarning(t *testing.T) {
	tmp, s := newDAGTestRepo(t)
	runCmd("add", "--path", tmp, "--slug", "a", "A")
	runCmd("add", "--path", tmp, "--slug", "b", "B")
	a, _ := s.LoadFeatureStatus("a")
	a.DependsOn = []store.Dependency{{Slug: "b", Kind: store.DependencyKindHard}}
	if err := s.SaveFeatureStatus(a); err != nil {
		t.Fatal(err)
	}
	b, _ := s.LoadFeatureStatus("b")
	b.DependsOn = []store.Dependency{{Slug: "a", Kind: store.DependencyKindHard}}
	if err := s.SaveFeatureStatus(b); err != nil {
		t.Fatal(err)
	}
	out, _, code := runCmd("status", "--path", tmp)
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(out, "cycle") {
		t.Fatalf("expected cycle warning in plain status, got %q", out)
	}
}
