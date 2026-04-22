package gitutil

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreateShadowAndResolve(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)

	head, err := HeadCommit(dir)
	if err != nil {
		t.Fatal(err)
	}

	path, err := CreateShadow(dir, "demo-feature", head)
	if err != nil {
		t.Fatalf("CreateShadow: %v", err)
	}

	// Shadow should live under .tpatch/shadow/demo-feature-<ts>/
	rel, err := filepath.Rel(dir, path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(filepath.ToSlash(rel), ".tpatch/shadow/demo-feature-") {
		t.Errorf("shadow path outside expected dir: %s", rel)
	}
	// Shadow should contain a checkout of HEAD.
	if _, err := os.Stat(filepath.Join(path, "hello.txt")); err != nil {
		t.Errorf("expected hello.txt in shadow: %v", err)
	}

	sh, err := ResolveShadow(dir, "demo-feature")
	if err != nil {
		t.Fatalf("ResolveShadow: %v", err)
	}
	if sh == nil {
		t.Fatal("expected a shadow to be resolved")
	}
	if sh.Slug != "demo-feature" {
		t.Errorf("slug = %q, want demo-feature", sh.Slug)
	}
	if sh.Path != path {
		t.Errorf("path = %q, want %q", sh.Path, path)
	}
}

func TestCreateShadowReapsPrior(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)

	head, _ := HeadCommit(dir)

	first, err := CreateShadow(dir, "demo", head)
	if err != nil {
		t.Fatalf("first CreateShadow: %v", err)
	}
	// Sleep inside the test would slow CI; instead assert by directly
	// writing a second shadow dir (simulate a leftover from a crash)
	// and verify the next CreateShadow reaps it.
	stale := filepath.Join(dir, ".tpatch/shadow/demo-2020-01-01T00-00-00.000000Z")
	if err := os.MkdirAll(stale, 0o755); err != nil {
		t.Fatal(err)
	}

	// CreateShadow must prune both `first` and the stale dir.
	second, err := CreateShadow(dir, "demo", head)
	if err != nil {
		t.Fatalf("second CreateShadow: %v", err)
	}
	if first == second {
		t.Error("second shadow should have a distinct path")
	}
	if _, err := os.Stat(first); !os.IsNotExist(err) {
		t.Errorf("first shadow should have been reaped, stat err = %v", err)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Errorf("stale shadow should have been reaped, stat err = %v", err)
	}
}

func TestPruneShadowIdempotent(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)

	if err := PruneShadow(dir, "nonexistent"); err != nil {
		t.Errorf("PruneShadow on missing slug should be nil, got %v", err)
	}

	head, _ := HeadCommit(dir)
	path, err := CreateShadow(dir, "demo", head)
	if err != nil {
		t.Fatal(err)
	}
	if err := PruneShadow(dir, "demo"); err != nil {
		t.Fatalf("PruneShadow: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("shadow should be gone, stat err = %v", err)
	}
	// Prune again — still no error.
	if err := PruneShadow(dir, "demo"); err != nil {
		t.Errorf("second PruneShadow should be nil, got %v", err)
	}
}

func TestCopyShadowToReal(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)

	head, _ := HeadCommit(dir)
	shadowPath, err := CreateShadow(dir, "demo", head)
	if err != nil {
		t.Fatal(err)
	}

	// Write a "resolved" version of hello.txt in the shadow.
	resolved := "hello, resolved!\n"
	if err := os.WriteFile(filepath.Join(shadowPath, "hello.txt"), []byte(resolved), 0o644); err != nil {
		t.Fatal(err)
	}
	// And a new subdir file that didn't exist in upstream.
	subdir := filepath.Join(shadowPath, "sub")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subdir, "new.txt"), []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := CopyShadowToReal(dir, "demo", []string{"hello.txt", "sub/new.txt"}); err != nil {
		t.Fatalf("CopyShadowToReal: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(dir, "hello.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != resolved {
		t.Errorf("hello.txt not copied: got %q", string(got))
	}
	if _, err := os.Stat(filepath.Join(dir, "sub/new.txt")); err != nil {
		t.Errorf("sub/new.txt not copied: %v", err)
	}
}

func TestCopyShadowToRealRejectsUnsafePaths(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)

	head, _ := HeadCommit(dir)
	if _, err := CreateShadow(dir, "demo", head); err != nil {
		t.Fatal(err)
	}

	cases := []string{
		"../escape.txt",
		"/abs/path.txt",
		"sub/../../escape.txt",
		".git/config",
	}
	for _, bad := range cases {
		err := CopyShadowToReal(dir, "demo", []string{bad})
		if err == nil {
			t.Errorf("CopyShadowToReal(%q) should refuse, got nil", bad)
		}
	}
}

func TestCopyShadowToRealNoShadow(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)

	err := CopyShadowToReal(dir, "demo", []string{"hello.txt"})
	if err == nil {
		t.Fatal("expected error when no shadow exists")
	}
}

func TestShadowDiffDetectsChange(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)

	head, _ := HeadCommit(dir)
	shadowPath, err := CreateShadow(dir, "demo", head)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(shadowPath, "hello.txt"), []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	diff, err := ShadowDiff(dir, "demo", []string{"hello.txt"})
	if err != nil {
		t.Fatalf("ShadowDiff: %v", err)
	}
	if !strings.Contains(diff, "changed") {
		t.Errorf("diff should reference the change, got %q", diff)
	}

	// A clean file returns empty output.
	if err := os.WriteFile(filepath.Join(shadowPath, "hello.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	diff2, err := ShadowDiff(dir, "demo", []string{"hello.txt"})
	if err != nil {
		t.Fatalf("ShadowDiff clean: %v", err)
	}
	if diff2 != "" {
		t.Errorf("expected empty diff, got %q", diff2)
	}
}
