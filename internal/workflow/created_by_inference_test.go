package workflow

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/tesseracode/tesserapatch/internal/store"
)

// inferTestEnv builds a store with the DAG flag set and registers a
// child feature that depends on the supplied parents. Each parent is
// also registered. The caller is responsible for writing each parent's
// `artifacts/post-apply.patch` with the bytes that should (or should
// not) match the op.Search.
func inferTestEnv(t *testing.T, dagEnabled bool, childSlug string, parents []store.Dependency) *store.Store {
	t.Helper()
	s := planTestEnv(t, dagEnabled)
	for _, p := range parents {
		addPlanFeature(t, s, p.Slug, nil)
	}
	addPlanFeature(t, s, childSlug, parents)
	return s
}

// writeParentPostApplyPatch seeds a parent's post-apply.patch artifact
// with arbitrary bytes — enough for `bytes.Contains` checks. The
// content is not parsed; the inference matcher only does substring
// search on the raw bytes.
func writeParentPostApplyPatch(t *testing.T, s *store.Store, parentSlug, body string) {
	t.Helper()
	if err := s.WriteArtifact(parentSlug, "post-apply.patch", body); err != nil {
		t.Fatalf("WriteArtifact post-apply.patch for %s: %v", parentSlug, err)
	}
}

// recipeSnapshot returns a deep copy of a recipe's Operations slice so
// tests can prove the matcher did not mutate the input.
func recipeSnapshot(r ApplyRecipe) []RecipeOperation {
	out := make([]RecipeOperation, len(r.Operations))
	copy(out, r.Operations)
	return out
}

// TestCreatedByInference_SuggestsHardParent — single hard parent's
// post-apply.patch contains the Search bytes; pristine working tree
// does not have the file. Expect: a single ℹ suggestion line + summary,
// recipe operations untouched.
func TestCreatedByInference_SuggestsHardParent(t *testing.T) {
	s := inferTestEnv(t, true, "child", []store.Dependency{
		{Slug: "parent", Kind: store.DependencyKindHard},
	})
	writeParentPostApplyPatch(t, s, "parent",
		"+++ b/src/auth.ts\n@@ -1,1 +1,3 @@\n+function login() { return TOKEN_PLACEHOLDER; }\n")

	recipe := ApplyRecipe{
		Feature: "child",
		Operations: []RecipeOperation{
			{Type: "replace-in-file", Path: "src/auth.ts", Search: "TOKEN_PLACEHOLDER", Replace: "secret"},
		},
	}
	before := recipeSnapshot(recipe)

	read, restore := captureWarn(t)
	defer restore()
	if err := inferCreatedBy(context.Background(), s, "child", recipe); err != nil {
		t.Fatalf("inferCreatedBy: %v", err)
	}
	got := read()

	if !strings.Contains(got, "suggest created_by: parent") {
		t.Errorf("expected suggestion mentioning parent; got:\n%s", got)
	}
	if !strings.Contains(got, "op #0") {
		t.Errorf("expected op index in suggestion; got:\n%s", got)
	}
	if !strings.Contains(got, "src/auth.ts") {
		t.Errorf("expected target path in suggestion; got:\n%s", got)
	}
	if !strings.Contains(got, "1 created_by suggestion(s)") {
		t.Errorf("expected summary line; got:\n%s", got)
	}
	if !reflect.DeepEqual(before, recipe.Operations) {
		t.Errorf("recipe was mutated; before=%+v after=%+v", before, recipe.Operations)
	}
}

// TestCreatedByInference_RespectsExistingAnnotation — when the op
// already declares created_by, the matcher must not second-guess it.
// No suggestion, no scan output, even when a parent's patch matches.
func TestCreatedByInference_RespectsExistingAnnotation(t *testing.T) {
	s := inferTestEnv(t, true, "child", []store.Dependency{
		{Slug: "parent", Kind: store.DependencyKindHard},
	})
	writeParentPostApplyPatch(t, s, "parent", "TOKEN_PLACEHOLDER")

	recipe := ApplyRecipe{
		Feature: "child",
		Operations: []RecipeOperation{
			{Type: "replace-in-file", Path: "src/auth.ts", Search: "TOKEN_PLACEHOLDER", Replace: "secret", CreatedBy: "parent"},
		},
	}

	read, restore := captureWarn(t)
	defer restore()
	if err := inferCreatedBy(context.Background(), s, "child", recipe); err != nil {
		t.Fatalf("inferCreatedBy: %v", err)
	}
	if got := read(); got != "" {
		t.Errorf("expected silent output for already-annotated op; got:\n%s", got)
	}
}

// TestCreatedByInference_AmbiguousMultipleParents — two hard parents
// each contain the Search bytes. The matcher must emit an ambiguity
// notice (no auto-suggestion) and must not include a summary line
// (the summary counts only single-match suggestions).
func TestCreatedByInference_AmbiguousMultipleParents(t *testing.T) {
	s := inferTestEnv(t, true, "child", []store.Dependency{
		{Slug: "parent-a", Kind: store.DependencyKindHard},
		{Slug: "parent-b", Kind: store.DependencyKindHard},
	})
	writeParentPostApplyPatch(t, s, "parent-a", "MARKER_TEXT in patch a")
	writeParentPostApplyPatch(t, s, "parent-b", "patch b also has MARKER_TEXT")

	recipe := ApplyRecipe{
		Feature: "child",
		Operations: []RecipeOperation{
			{Type: "replace-in-file", Path: "src/x.go", Search: "MARKER_TEXT", Replace: "X"},
		},
	}

	read, restore := captureWarn(t)
	defer restore()
	if err := inferCreatedBy(context.Background(), s, "child", recipe); err != nil {
		t.Fatalf("inferCreatedBy: %v", err)
	}
	got := read()
	if !strings.Contains(got, "ambiguous") {
		t.Errorf("expected ambiguity notice; got:\n%s", got)
	}
	if !strings.Contains(got, "parent-a") || !strings.Contains(got, "parent-b") {
		t.Errorf("expected both candidate parents listed; got:\n%s", got)
	}
	if strings.Contains(got, "suggest created_by:") {
		t.Errorf("must not auto-suggest when ambiguous; got:\n%s", got)
	}
	if strings.Contains(got, "created_by suggestion(s)") {
		t.Errorf("ambiguity-only output must not produce a suggestion summary; got:\n%s", got)
	}
}

// TestCreatedByInference_SkipsSoftParents — only a soft parent's patch
// contains the Search bytes; the lone hard parent does not. Expect:
// no suggestion (PRD: "Inference only suggests hard parents").
func TestCreatedByInference_SkipsSoftParents(t *testing.T) {
	s := inferTestEnv(t, true, "child", []store.Dependency{
		{Slug: "soft-parent", Kind: store.DependencyKindSoft},
		{Slug: "hard-parent", Kind: store.DependencyKindHard},
	})
	writeParentPostApplyPatch(t, s, "soft-parent", "MARKER_TEXT in soft parent")
	writeParentPostApplyPatch(t, s, "hard-parent", "unrelated content")

	recipe := ApplyRecipe{
		Feature: "child",
		Operations: []RecipeOperation{
			{Type: "replace-in-file", Path: "src/x.go", Search: "MARKER_TEXT", Replace: "X"},
		},
	}

	read, restore := captureWarn(t)
	defer restore()
	if err := inferCreatedBy(context.Background(), s, "child", recipe); err != nil {
		t.Fatalf("inferCreatedBy: %v", err)
	}
	if got := read(); got != "" {
		t.Errorf("expected silence (only soft parent matched); got:\n%s", got)
	}
}

// TestCreatedByInference_OptOut — WithDisableCreatedByInference must
// short-circuit before any I/O. Even with a perfect single-parent
// match the matcher emits nothing.
func TestCreatedByInference_OptOut(t *testing.T) {
	s := inferTestEnv(t, true, "child", []store.Dependency{
		{Slug: "parent", Kind: store.DependencyKindHard},
	})
	writeParentPostApplyPatch(t, s, "parent", "TOKEN_PLACEHOLDER")

	recipe := ApplyRecipe{
		Feature: "child",
		Operations: []RecipeOperation{
			{Type: "replace-in-file", Path: "src/auth.ts", Search: "TOKEN_PLACEHOLDER", Replace: "secret"},
		},
	}

	ctx := WithDisableCreatedByInference(context.Background(), true)
	read, restore := captureWarn(t)
	defer restore()
	if err := inferCreatedBy(ctx, s, "child", recipe); err != nil {
		t.Fatalf("inferCreatedBy: %v", err)
	}
	if got := read(); got != "" {
		t.Errorf("opt-out must suppress all output; got:\n%s", got)
	}
}

// TestCreatedByInference_FlagOff — when features_dependencies is
// false the matcher is inert (byte-identical pre-v0.6 behaviour).
func TestCreatedByInference_FlagOff(t *testing.T) {
	s := inferTestEnv(t, false, "child", []store.Dependency{
		{Slug: "parent", Kind: store.DependencyKindHard},
	})
	writeParentPostApplyPatch(t, s, "parent", "TOKEN_PLACEHOLDER")

	recipe := ApplyRecipe{
		Feature: "child",
		Operations: []RecipeOperation{
			{Type: "replace-in-file", Path: "src/auth.ts", Search: "TOKEN_PLACEHOLDER", Replace: "secret"},
		},
	}

	read, restore := captureWarn(t)
	defer restore()
	if err := inferCreatedBy(context.Background(), s, "child", recipe); err != nil {
		t.Fatalf("inferCreatedBy: %v", err)
	}
	if got := read(); got != "" {
		t.Errorf("flag-off must be inert; got:\n%s", got)
	}
}

// TestCreatedByInference_PristineHasSearch_NoSuggestion — when the
// Search text is already present in the working file, no scan is
// needed and no suggestion is emitted (the op will resolve normally).
func TestCreatedByInference_PristineHasSearch_NoSuggestion(t *testing.T) {
	s := inferTestEnv(t, true, "child", []store.Dependency{
		{Slug: "parent", Kind: store.DependencyKindHard},
	})
	// Parent ALSO contains the marker — proves the pristine check
	// short-circuits before the parent scan.
	writeParentPostApplyPatch(t, s, "parent", "TOKEN_PLACEHOLDER")
	writeWorkingFile(t, s, "src/auth.ts", "// existing TOKEN_PLACEHOLDER here\n")

	recipe := ApplyRecipe{
		Feature: "child",
		Operations: []RecipeOperation{
			{Type: "replace-in-file", Path: "src/auth.ts", Search: "TOKEN_PLACEHOLDER", Replace: "secret"},
		},
	}

	read, restore := captureWarn(t)
	defer restore()
	if err := inferCreatedBy(context.Background(), s, "child", recipe); err != nil {
		t.Fatalf("inferCreatedBy: %v", err)
	}
	if got := read(); got != "" {
		t.Errorf("expected silence when pristine file contains Search; got:\n%s", got)
	}
}

// TestCreatedByInference_NoMatchSilent — pristine doesn't contain the
// text and no parent's post-apply.patch contains it either. The
// matcher must stay silent and let the apply-time gate (or the bare
// not-found error) surface the real issue.
func TestCreatedByInference_NoMatchSilent(t *testing.T) {
	s := inferTestEnv(t, true, "child", []store.Dependency{
		{Slug: "parent", Kind: store.DependencyKindHard},
	})
	writeParentPostApplyPatch(t, s, "parent", "completely unrelated patch body")

	recipe := ApplyRecipe{
		Feature: "child",
		Operations: []RecipeOperation{
			{Type: "replace-in-file", Path: "src/auth.ts", Search: "TOKEN_PLACEHOLDER", Replace: "secret"},
		},
	}

	read, restore := captureWarn(t)
	defer restore()
	if err := inferCreatedBy(context.Background(), s, "child", recipe); err != nil {
		t.Fatalf("inferCreatedBy: %v", err)
	}
	if got := read(); got != "" {
		t.Errorf("expected silence (no parent match); got:\n%s", got)
	}
}
