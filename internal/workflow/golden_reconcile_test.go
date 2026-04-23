package workflow

// Golden scenarios for `tpatch reconcile` (M12 / ADR-010 PRD answer #6).
//
// Each scenario exercises RunReconcile end-to-end against a fixture git
// repo wired to a known upstream divergence. Together they cover the
// five verdicts the phase-3.5 pipeline can produce:
//
//	1. clean-reapply      — no phase 3.5 (ForwardApplyStrict / 3WayClean)
//	2. shadow-awaiting    — conflict + provider resolves successfully
//	3. validation-failed  — provider returns content with conflict markers
//	4. too-many-conflicts — conflict count exceeds MaxConflicts
//	5. blocked-no-provider — --resolve set but provider not configured
//
// The fixture builders (`buildCleanReapplyFixture`, `buildConflictFixture`,
// `buildMultiConflictFixture`) capture real `git diff --cached HEAD`
// output so the patch has the index/blob refs that `git apply --3way`
// needs to locate the base blob — hand-crafted patches fail that check.

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tesseracode/tesserapatch/internal/provider"
	"github.com/tesseracode/tesserapatch/internal/store"
)

// buildCleanReapplyFixture: shared.txt exists with one line; feature adds
// a new line; upstream stays at original. Reapply is trivial (strict or
// 3-way clean), no conflict.
func buildCleanReapplyFixture(t *testing.T) (*store.Store, string) {
	t.Helper()
	tmp := t.TempDir()
	setupGitRepo(t, tmp)
	os.WriteFile(filepath.Join(tmp, "shared.txt"), []byte("one\n"), 0o644)
	gitAdd(t, tmp, "shared.txt")
	gitCommit(t, tmp, "add shared")

	// Feature adds a second line.
	os.WriteFile(filepath.Join(tmp, "shared.txt"), []byte("one\ntwo\n"), 0o644)
	gitAdd(t, tmp, "shared.txt")
	diffCmd := exec.Command("git", "diff", "--cached", "HEAD")
	diffCmd.Dir = tmp
	patchBytes, err := diffCmd.Output()
	if err != nil {
		t.Fatalf("git diff: %v", err)
	}
	gitCommit(t, tmp, "feature applied")
	// Upstream does NOT diverge on this path — leave HEAD where it is
	// so reapply against HEAD is trivial.

	s, _ := store.Init(tmp)
	s.AddFeature(store.AddFeatureInput{Title: "Clean", Request: "r"})
	s.MarkFeatureState("clean", store.StateApplied, "apply", "")
	s.WriteArtifact("clean", "post-apply.patch", string(patchBytes))
	return s, "clean"
}

// buildConflictFixture: shared.txt, feature and upstream change the
// same line. Triggers ForwardApply3WayConflicts.
func buildConflictFixture(t *testing.T) (*store.Store, string) {
	t.Helper()
	tmp := t.TempDir()
	setupGitRepo(t, tmp)
	os.WriteFile(filepath.Join(tmp, "shared.txt"), []byte("a\nb\nc\n"), 0o644)
	gitAdd(t, tmp, "shared.txt")
	gitCommit(t, tmp, "add shared")

	os.WriteFile(filepath.Join(tmp, "shared.txt"), []byte("a\nB-local\nc\n"), 0o644)
	gitAdd(t, tmp, "shared.txt")
	diffCmd := exec.Command("git", "diff", "--cached", "HEAD")
	diffCmd.Dir = tmp
	patchBytes, err := diffCmd.Output()
	if err != nil {
		t.Fatalf("git diff: %v", err)
	}
	gitCommit(t, tmp, "feature applied")

	os.WriteFile(filepath.Join(tmp, "shared.txt"), []byte("a\nB-upstream\nc\n"), 0o644)
	gitAdd(t, tmp, "shared.txt")
	gitCommit(t, tmp, "upstream diverges")

	s, _ := store.Init(tmp)
	s.AddFeature(store.AddFeatureInput{Title: "Feature", Request: "r"})
	s.MarkFeatureState("feature", store.StateApplied, "apply", "")
	s.WriteArtifact("feature", "post-apply.patch", string(patchBytes))
	return s, "feature"
}

// TestGoldenReconcile_CleanReapply — scenario 1. --resolve is harmless
// when no conflict exists; classical phase 4 handles it.
func TestGoldenReconcile_CleanReapply(t *testing.T) {
	s, slug := buildCleanReapplyFixture(t)
	results, err := RunReconcile(context.Background(), s, []string{slug}, "HEAD", nil, provider.Config{},
		ReconcileOptions{Resolve: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("len=%d", len(results))
	}
	got := results[0].Outcome
	if got != store.ReconcileReapplied && got != store.ReconcileUpstreamed {
		t.Errorf("expected reapplied/upstreamed, got %s (phase=%s)", got, results[0].Phase)
	}
	if results[0].ShadowPath != "" {
		t.Errorf("clean reapply should not create a shadow; got %q", results[0].ShadowPath)
	}
}

// TestGoldenReconcile_ShadowAwaiting — scenario 2. Provider returns a
// clean merged file; expected verdict is shadow-awaiting with one
// resolved file and a shadow path populated.
func TestGoldenReconcile_ShadowAwaiting(t *testing.T) {
	s, slug := buildConflictFixture(t)

	// Set a provider (any non-empty config activates the resolver).
	cfg := provider.Config{Type: "openai-compatible", BaseURL: "http://x", Model: "m", AuthEnv: "X"}
	// Phase 3 may run a semantic check first (different prompt shape),
	// phase 3.5 then per-file. Keyed routes every resolver call to the
	// clean merged content; a positional fallback answers the phase-3
	// semantic probe with an empty verdict so phase 3.5 proceeds.
	prov := &scriptedProvider{
		responses: []string{`{"verdict":"unclear"}`},
		keyed:     map[string]string{"shared.txt": "a\nB-merged\nc\n"},
	}

	results, err := RunReconcile(context.Background(), s, []string{slug}, "HEAD", prov, cfg,
		ReconcileOptions{Resolve: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("len=%d", len(results))
	}
	r := results[0]
	if r.Outcome != store.ReconcileShadowAwaiting {
		t.Errorf("expected shadow-awaiting, got %s (phase=%s notes=%v)", r.Outcome, r.Phase, r.Notes)
	}
	if r.ShadowPath == "" {
		t.Errorf("expected shadow path populated")
	}
	if len(r.ResolvedFiles) != 1 || r.ResolvedFiles[0] != "shared.txt" {
		t.Errorf("expected [shared.txt] resolved, got %v", r.ResolvedFiles)
	}
	if len(r.FailedFiles) != 0 {
		t.Errorf("expected no failures, got %v", r.FailedFiles)
	}
}

// TestGoldenReconcile_ValidationFailed — scenario 3. Provider returns
// content that still contains conflict markers. Validation rejects it
// and the session is blocked-requires-human.
func TestGoldenReconcile_ValidationFailed(t *testing.T) {
	s, slug := buildConflictFixture(t)
	cfg := provider.Config{Type: "openai-compatible", BaseURL: "http://x", Model: "m", AuthEnv: "X"}

	// A response that looks merged but still carries `<<<<<<<` markers.
	bad := "a\n<<<<<<< ours\nB-local\n=======\nB-upstream\n>>>>>>> theirs\nc\n"
	prov := &scriptedProvider{
		responses: []string{`{"verdict":"unclear"}`},
		keyed:     map[string]string{"shared.txt": bad},
	}

	results, err := RunReconcile(context.Background(), s, []string{slug}, "HEAD", prov, cfg,
		ReconcileOptions{Resolve: true})
	if err != nil {
		t.Fatal(err)
	}
	r := results[0]
	if r.Outcome != store.ReconcileBlockedRequiresHuman {
		t.Errorf("expected blocked-requires-human, got %s (notes=%v)", r.Outcome, r.Notes)
	}
	if len(r.FailedFiles) != 1 {
		t.Errorf("expected 1 failed file, got %v", r.FailedFiles)
	}
}

// TestGoldenReconcile_TooManyConflicts — scenario 4. Cap at 0
// immediately short-circuits the resolver. (The reconcile wiring uses
// DefaultMaxConflicts=10 when MaxConflicts is 0, so pass 1 here and
// rely on the conflict fixture only having 1 file; instead we use a
// synthetic cap of -1 which falls below len(conflictFiles)=1.)
// The easiest reliable trigger is a MaxConflicts that the resolver
// respects verbatim and is less than the number of conflict files.
// Since opts.MaxConflicts=0 means "use default", use 1 with a
// multi-file fixture... but our buildConflictFixture has 1 file.
// Trick: pass a non-default value by using the resolver directly via
// RunReconcile and a fixture with 2 conflict files. Build one inline.
func TestGoldenReconcile_TooManyConflicts(t *testing.T) {
	tmp := t.TempDir()
	setupGitRepo(t, tmp)
	os.WriteFile(filepath.Join(tmp, "a.txt"), []byte("x\n"), 0o644)
	os.WriteFile(filepath.Join(tmp, "b.txt"), []byte("x\n"), 0o644)
	gitAdd(t, tmp, ".")
	gitCommit(t, tmp, "add")

	os.WriteFile(filepath.Join(tmp, "a.txt"), []byte("A-local\n"), 0o644)
	os.WriteFile(filepath.Join(tmp, "b.txt"), []byte("B-local\n"), 0o644)
	gitAdd(t, tmp, ".")
	diffCmd := exec.Command("git", "diff", "--cached", "HEAD")
	diffCmd.Dir = tmp
	patchBytes, _ := diffCmd.Output()
	gitCommit(t, tmp, "feature")

	os.WriteFile(filepath.Join(tmp, "a.txt"), []byte("A-upstream\n"), 0o644)
	os.WriteFile(filepath.Join(tmp, "b.txt"), []byte("B-upstream\n"), 0o644)
	gitAdd(t, tmp, ".")
	gitCommit(t, tmp, "upstream")

	s, _ := store.Init(tmp)
	s.AddFeature(store.AddFeatureInput{Title: "Multi", Request: "r"})
	s.MarkFeatureState("multi", store.StateApplied, "apply", "")
	s.WriteArtifact("multi", "post-apply.patch", string(patchBytes))

	cfg := provider.Config{Type: "openai-compatible", BaseURL: "http://x", Model: "m", AuthEnv: "X"}
	// MaxConflicts=1 with 2 conflict files triggers the cap.
	prov := &scriptedProvider{}
	results, err := RunReconcile(context.Background(), s, []string{"multi"}, "HEAD", prov, cfg,
		ReconcileOptions{Resolve: true, MaxConflicts: 1})
	if err != nil {
		t.Fatal(err)
	}
	r := results[0]
	if r.Outcome != store.ReconcileBlockedTooManyConflicts {
		t.Errorf("expected blocked-too-many-conflicts, got %s (phase=%s notes=%v)",
			r.Outcome, r.Phase, r.Notes)
	}
	if prov.calls != 0 {
		t.Errorf("provider should not be called on too-many-conflicts short-circuit; calls=%d", prov.calls)
	}
}

// TestGoldenReconcile_NoProviderBlocks — scenario 5. --resolve without
// a configured provider yields blocked-requires-human. (Already covered
// by TestReconcilePhase35_NoProviderBlocks; kept here as a golden label
// so `go test -run GoldenReconcile` is a complete ADR-010 acceptance suite.)
func TestGoldenReconcile_NoProviderBlocks(t *testing.T) {
	s, slug := buildConflictFixture(t)
	results, err := RunReconcile(context.Background(), s, []string{slug}, "HEAD", nil, provider.Config{},
		ReconcileOptions{Resolve: true})
	if err != nil {
		t.Fatal(err)
	}
	r := results[0]
	if r.Outcome != store.ReconcileBlockedRequiresHuman {
		t.Errorf("expected blocked-requires-human, got %s", r.Outcome)
	}
	if r.ShadowPath != "" {
		t.Errorf("no-provider path should not create a shadow; got %q", r.ShadowPath)
	}
}

// TestGoldenReconcile_ResolveApplyTruthful — v0.5.2 C2 finding #1.
// Scenario 6: --resolve --apply with a provider that returns a clean
// merged file. Expected end-state:
//   - Outcome = ReconcileReapplied
//   - real working tree contains the merged content (NOT the pre-patch
//     upstream content, NOT the local feature content, NOT conflict
//     markers)
//   - shadow worktree is pruned
//   - feature state = applied
//
// Before v0.5.2 this path set ReconcileReapplied without actually
// copying shadow → real tree, so the real working tree kept the
// upstream content and the "truthful" claim was silently wrong.
func TestGoldenReconcile_ResolveApplyTruthful(t *testing.T) {
	s, slug := buildConflictFixture(t)

	cfg := provider.Config{Type: "openai-compatible", BaseURL: "http://x", Model: "m", AuthEnv: "X"}
	// Clean merged resolution for the one conflicted file (shared.txt).
	// `unclear` response for the phase-3 semantic probe so we progress
	// into phase 3.5.
	prov := &scriptedProvider{
		responses: []string{`{"verdict":"unclear"}`},
		keyed:     map[string]string{"shared.txt": "a\nB-merged\nc\n"},
	}

	results, err := RunReconcile(context.Background(), s, []string{slug}, "HEAD", prov, cfg,
		ReconcileOptions{Resolve: true, Apply: true})
	if err != nil {
		t.Fatalf("RunReconcile: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("len(results)=%d, want 1", len(results))
	}
	r := results[0]
	if r.Outcome != store.ReconcileReapplied {
		t.Fatalf("Outcome = %s, want reapplied (phase=%s notes=%v)", r.Outcome, r.Phase, r.Notes)
	}

	// Real working tree must contain the MERGED content — the truthful
	// bit. The pre-v0.5.2 bug would have left upstream's "B-upstream"
	// on disk here because shadow → real copy was skipped.
	got, err := os.ReadFile(filepath.Join(s.Root, "shared.txt"))
	if err != nil {
		t.Fatalf("read shared.txt: %v", err)
	}
	gotStr := string(got)
	if !strings.Contains(gotStr, "B-merged") {
		t.Errorf("shared.txt missing merged content; got %q (pre-v0.5.2 bug leaves B-upstream here)", gotStr)
	}
	if strings.Contains(gotStr, "B-upstream") || strings.Contains(gotStr, "B-local") {
		t.Errorf("shared.txt contains pre-merge content; got %q", gotStr)
	}
	if strings.Contains(gotStr, "<<<<<<<") || strings.Contains(gotStr, ">>>>>>>") {
		t.Errorf("shared.txt contains conflict markers; got %q", gotStr)
	}

	// Feature state must be applied.
	st, _ := s.LoadFeatureStatus(slug)
	if st.State != store.StateApplied {
		t.Errorf("feature state = %s, want applied", st.State)
	}

	// Shadow pointer cleared from status.json (the resolver session id
	// is preserved as an audit record).
	if st.Reconcile.ShadowPath != "" {
		t.Errorf("Reconcile.ShadowPath = %q, want empty after auto-apply", st.Reconcile.ShadowPath)
	}
	if st.Reconcile.ResolveSession == "" {
		t.Errorf("Reconcile.ResolveSession should be preserved; got empty")
	}
}
