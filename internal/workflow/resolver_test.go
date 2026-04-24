package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tesseracode/tesserapatch/internal/provider"
	"github.com/tesseracode/tesserapatch/internal/store"
)

// scriptedProvider returns responses[i] for the i-th call. If keyed
// is non-nil, it matches the input file name in the user prompt and
// returns keyed[path] instead of the positional response.
type scriptedProvider struct {
	responses []string
	keyed     map[string]string
	err       error
	calls     int
}

func (p *scriptedProvider) Check(ctx context.Context, cfg provider.Config) (*provider.Health, error) {
	return &provider.Health{}, nil
}

func (p *scriptedProvider) Generate(ctx context.Context, cfg provider.Config, req provider.GenerateRequest) (string, error) {
	if p.err != nil {
		return "", p.err
	}
	if p.keyed != nil {
		for path, resp := range p.keyed {
			if strings.Contains(req.UserPrompt, "# File: "+path) {
				p.calls++
				return resp, nil
			}
		}
	}
	if p.calls >= len(p.responses) {
		return "", errors.New("scripted: out of responses")
	}
	resp := p.responses[p.calls]
	p.calls++
	return resp, nil
}

func setupResolverStore(t *testing.T) (*store.Store, string) {
	t.Helper()
	tmp := t.TempDir()
	setupGitRepo(t, tmp)

	s, err := store.Init(tmp)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddFeature(store.AddFeatureInput{Title: "resolver-demo", Request: "test"}); err != nil {
		t.Fatal(err)
	}
	_ = s.WriteFeatureFile("resolver-demo", "spec.md", "# Spec\nKeep Foo intact.\n")
	_ = s.WriteFeatureFile("resolver-demo", "exploration.md", "# Exploration\nNotes.\n")
	head, err := headCommit(t, tmp)
	if err != nil {
		t.Fatal(err)
	}
	return s, head
}

// headCommit returns the current HEAD sha for dir.
func headCommit(t *testing.T, dir string) (string, error) {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}

func TestResolveTooManyConflictsBlocks(t *testing.T) {
	s, head := setupResolverStore(t)

	inputs := []ConflictInput{
		{Path: "a"}, {Path: "b"}, {Path: "c"},
	}
	cfg := provider.Config{BaseURL: "x", Model: "y", Type: "openai-compatible"}
	res, err := RunConflictResolve(context.Background(), s, "resolver-demo",
		&scriptedProvider{}, cfg, inputs, head,
		ResolveOptions{MaxConflicts: 2})
	if err != nil {
		t.Fatal(err)
	}
	if res.Verdict != ResolveVerdictBlockedTooManyConflicts {
		t.Errorf("verdict = %q, want %q", res.Verdict, ResolveVerdictBlockedTooManyConflicts)
	}
	// Should still persist the session.
	assertSessionFile(t, s, "resolver-demo", res.SessionID)
}

func TestResolveNoProviderBlocks(t *testing.T) {
	s, head := setupResolverStore(t)

	res, err := RunConflictResolve(context.Background(), s, "resolver-demo",
		nil, provider.Config{}, []ConflictInput{{Path: "a"}}, head,
		ResolveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Verdict != ResolveVerdictBlockedRequiresHuman {
		t.Errorf("verdict = %q", res.Verdict)
	}
	if len(res.Outcomes) != 1 || res.Outcomes[0].Status != FileStatusProviderError {
		t.Errorf("outcome = %+v", res.Outcomes)
	}
}

func TestResolveHappyPath(t *testing.T) {
	s, head := setupResolverStore(t)

	resolvedContent := "package main\n\nfunc Foo() int { return 42 }\n"
	prov := &scriptedProvider{
		keyed: map[string]string{"foo.go": resolvedContent},
	}
	cfg := provider.Config{BaseURL: "x", Model: "y", Type: "openai-compatible"}

	inputs := []ConflictInput{{
		Path:   "foo.go",
		Base:   []byte("package main\n\nfunc Foo() int { return 1 }\n"),
		Ours:   []byte("package main\n\nfunc Foo() int { return 2 }\n"),
		Theirs: []byte("package main\n\nfunc Foo() int { return 3 }\n"),
	}}
	res, err := RunConflictResolve(context.Background(), s, "resolver-demo",
		prov, cfg, inputs, head, ResolveOptions{})
	if err != nil {
		t.Fatalf("RunConflictResolve: %v", err)
	}
	if res.Verdict != ResolveVerdictShadowAwaiting {
		t.Errorf("verdict = %q, want %q", res.Verdict, ResolveVerdictShadowAwaiting)
	}
	if len(res.Outcomes) != 1 || res.Outcomes[0].Status != FileStatusResolved {
		t.Fatalf("outcome = %+v", res.Outcomes)
	}

	// Shadow should contain the resolved file + resolution-report.md.
	shadow := res.ShadowPath
	got, err := os.ReadFile(filepath.Join(shadow, "foo.go"))
	if err != nil {
		t.Fatalf("shadow file: %v", err)
	}
	if string(got) != resolvedContent {
		t.Errorf("shadow content = %q, want %q", string(got), resolvedContent)
	}
	if _, err := os.Stat(filepath.Join(shadow, "resolution-report.md")); err != nil {
		t.Errorf("resolution-report.md missing: %v", err)
	}
	assertSessionFile(t, s, "resolver-demo", res.SessionID)
}

func TestResolveValidationFailureBlocks(t *testing.T) {
	s, head := setupResolverStore(t)

	// Provider returns content with unresolved conflict markers.
	bad := "<<<<<<< ours\nfoo\n=======\nbar\n>>>>>>> theirs\n"
	prov := &scriptedProvider{responses: []string{bad}}
	cfg := provider.Config{BaseURL: "x", Model: "y", Type: "openai-compatible"}

	inputs := []ConflictInput{{
		Path: "foo.txt", Base: []byte("x"), Ours: []byte("y"), Theirs: []byte("z"),
	}}
	res, err := RunConflictResolve(context.Background(), s, "resolver-demo",
		prov, cfg, inputs, head, ResolveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Verdict != ResolveVerdictBlockedRequiresHuman {
		t.Errorf("verdict = %q", res.Verdict)
	}
	if res.Outcomes[0].Status != FileStatusValidationFailed {
		t.Errorf("status = %q", res.Outcomes[0].Status)
	}
}

func TestResolveStripsCodeFences(t *testing.T) {
	s, head := setupResolverStore(t)

	fenced := "```go\npackage main\n\nfunc Foo() int { return 1 }\n```"
	prov := &scriptedProvider{responses: []string{fenced}}
	cfg := provider.Config{BaseURL: "x", Model: "y", Type: "openai-compatible"}

	inputs := []ConflictInput{{
		Path:   "foo.go",
		Base:   []byte("package main\n"),
		Ours:   []byte("package main\n"),
		Theirs: []byte("package main\n"),
	}}
	res, err := RunConflictResolve(context.Background(), s, "resolver-demo",
		prov, cfg, inputs, head, ResolveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Outcomes[0].Status != FileStatusResolved {
		t.Fatalf("expected resolved, got %+v", res.Outcomes[0])
	}
	got, _ := os.ReadFile(filepath.Join(res.ShadowPath, "foo.go"))
	if strings.HasPrefix(string(got), "```") {
		t.Errorf("fences not stripped: %q", string(got))
	}
	if !strings.Contains(string(got), "func Foo()") {
		t.Errorf("content missing: %q", string(got))
	}
}

func TestResolveSkipsTooLarge(t *testing.T) {
	s, head := setupResolverStore(t)

	big := strings.Repeat("x", 300*1024)
	prov := &scriptedProvider{responses: []string{"ok"}}
	cfg := provider.Config{BaseURL: "x", Model: "y", Type: "openai-compatible"}

	inputs := []ConflictInput{{
		Path:   "big.txt",
		Base:   []byte(big),
		Ours:   []byte(big),
		Theirs: []byte(big),
	}}
	res, err := RunConflictResolve(context.Background(), s, "resolver-demo",
		prov, cfg, inputs, head, ResolveOptions{MaxFileBytes: 200 * 1024})
	if err != nil {
		t.Fatal(err)
	}
	if res.Outcomes[0].Status != FileStatusSkippedTooLarge {
		t.Errorf("status = %q", res.Outcomes[0].Status)
	}
	if prov.calls != 0 {
		t.Errorf("provider should not be called for too-large file, got %d calls", prov.calls)
	}
}

func assertSessionFile(t *testing.T, s *store.Store, slug, wantID string) {
	t.Helper()
	got, err := s.ReadFeatureFile(slug, filepath.Join("artifacts", "resolution-session.json"))
	if err != nil {
		t.Fatalf("resolution-session.json missing: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(got), &parsed); err != nil {
		t.Fatalf("resolution-session.json invalid: %v", err)
	}
	if id, _ := parsed["session_id"].(string); id != wantID {
		t.Errorf("session_id = %q, want %q", id, wantID)
	}
}
