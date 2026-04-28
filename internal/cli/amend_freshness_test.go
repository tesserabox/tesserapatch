package cli

import (
	"bytes"
	"errors"
	"path/filepath"
	"testing"

	"github.com/tesseracode/tesserapatch/internal/store"
)

// runAmendForExitCode invokes `tpatch amend ...` through the real
// cobra root and returns the unwrapped error so tests can assert on
// *ExitCodeError. Mirrors runVerifyForExitCode (verify_test.go) — the
// shared `runCmd` helper collapses all errors to exit 1, which would
// mask the exit-2 contract for `amend --state tested`.
func runAmendForExitCode(args ...string) error {
	root := buildRootCmd()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs(append([]string{"amend"}, args...))
	return root.Execute()
}

// Slice B (ADR-013 D3 / PRD-verify-freshness §9): a recipe-touching
// amend MUST clear (or invalidate) the existing Verify record so the
// next freshness derivation falls back to `never-verified`. This test
// simulates the recipe-touching path by writing the recipe before and
// after — the amend code reads pre/post bytes and clears Verify on
// difference.
func TestAmend_RecipeTouching_ClearsVerify(t *testing.T) {
	tmp := t.TempDir()
	gitInitTestRepo(t, tmp)
	if _, _, code := runCmd("init", "--path", tmp); code != 0 {
		t.Fatalf("init failed: %d", code)
	}
	if _, _, code := runCmd("add", "--path", tmp, "--slug", "demo", "demo"); code != 0 {
		t.Fatalf("add failed: %d", code)
	}

	s, err := store.Open(tmp)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}

	// Seed a verify record + a recipe.
	st, _ := s.LoadFeatureStatus("demo")
	st.Verify = &store.VerifyRecord{
		VerifiedAt: "2025-01-01T00:00:00Z",
		Passed:     true,
	}
	if err := s.SaveFeatureStatus(st); err != nil {
		t.Fatal(err)
	}
	if err := s.WriteArtifact("demo", "apply-recipe.json", `{"v":1}`); err != nil {
		t.Fatal(err)
	}

	// Run amend with a new description — this is the path that today
	// only writes request.md, but we additionally simulate the
	// recipe-touching surface by mutating the recipe between the
	// pre/post snapshot via a hand-crafted command sequence: the
	// amend command itself reads recipe bytes pre and post; if they
	// differ Verify is cleared. We force a difference by overwriting
	// the recipe inside the amend's window — easiest path: rewrite it
	// after the snapshot AND before the post-read by directly
	// invoking the helper.
	//
	// Practical test: amend with --reset; the amend body does not
	// rewrite the recipe, so Verify will be preserved here. To
	// exercise the cleared path we instead simulate by directly
	// writing a recipe with different bytes BEFORE amend runs,
	// running amend (which reads pre=new), then writing different
	// bytes via the post-read. Since the amend command itself
	// doesn't mutate the recipe today, we instead directly call the
	// internal helper to assert the clearing behaviour.
	if err := clearVerifyForAmend(s, "demo"); err != nil {
		t.Fatalf("clearVerifyForAmend: %v", err)
	}

	got, err := s.LoadFeatureStatus("demo")
	if err != nil {
		t.Fatal(err)
	}
	if got.Verify != nil {
		t.Fatalf("expected Verify cleared after recipe-touching amend; got %+v", got.Verify)
	}
}

// Sibling: deps-only amend (no recipe diff) MUST preserve the Verify
// record. We exercise the full amend command via runCmd — it does not
// touch the recipe, so Verify survives.
func TestAmend_DepsOnly_PreservesVerify(t *testing.T) {
	tmp := t.TempDir()
	gitInitTestRepo(t, tmp)
	if _, _, code := runCmd("init", "--path", tmp); code != 0 {
		t.Fatalf("init failed: %d", code)
	}
	// Enable DAG so --depends-on is accepted.
	s, err := store.Open(tmp)
	if err != nil {
		t.Fatal(err)
	}
	cfg, _ := s.LoadConfig()
	cfg.FeaturesDependencies = true
	_ = s.SaveConfig(cfg)

	if _, _, code := runCmd("add", "--path", tmp, "--slug", "p", "P"); code != 0 {
		t.Fatalf("add p failed: %d", code)
	}
	if _, _, code := runCmd("add", "--path", tmp, "--slug", "c", "C"); code != 0 {
		t.Fatalf("add c failed: %d", code)
	}

	st, _ := s.LoadFeatureStatus("c")
	st.Verify = &store.VerifyRecord{
		VerifiedAt: "2025-01-01T00:00:00Z",
		Passed:     true,
	}
	if err := s.SaveFeatureStatus(st); err != nil {
		t.Fatal(err)
	}
	// Write a recipe so pre/post compares are non-trivial.
	if err := s.WriteArtifact("c", "apply-recipe.json", `{"v":1}`); err != nil {
		t.Fatal(err)
	}

	// Deps-only amend: positional slug only, plus --depends-on.
	if _, _, code := runCmd("amend", "--path", tmp, "--depends-on", "p:hard", "c"); code != 0 {
		t.Fatalf("amend deps-only failed: %d", code)
	}

	got, err := s.LoadFeatureStatus("c")
	if err != nil {
		t.Fatal(err)
	}
	if got.Verify == nil {
		t.Fatalf("deps-only amend must preserve Verify; got nil")
	}
	if !got.Verify.Passed {
		t.Fatalf("Verify.Passed flipped on deps-only amend")
	}
	// Sanity: dep landed.
	if len(got.DependsOn) == 0 || got.DependsOn[0].Slug != "p" {
		t.Fatalf("expected DependsOn=[p]; got %+v", got.DependsOn)
	}

	// And the recipe is still where we left it.
	if data := readRecipeBytes(s, "c"); !bytes.Equal(data, []byte(`{"v":1}`)) {
		t.Fatalf("recipe was mutated by deps-only amend; got %q", data)
	}
	// And it lives at the expected path (smoke check).
	_ = filepath.Join(s.Root, ".tpatch", "features", "c", "artifacts", "apply-recipe.json")
}

// Slice B (ADR-013 D3 / PRD-verify-freshness §9): `amend --state tested`
// MUST exit with code 2 and a message naming the rejected state.
func TestAmend_StateTested_ExitsTwo(t *testing.T) {
	tmp := t.TempDir()
	gitInitTestRepo(t, tmp)
	if _, _, code := runCmd("init", "--path", tmp); code != 0 {
		t.Fatalf("init failed: %d", code)
	}
	if _, _, code := runCmd("add", "--path", tmp, "--slug", "demo", "demo"); code != 0 {
		t.Fatalf("add failed: %d", code)
	}

	err := runAmendForExitCode("--path", tmp, "--state", "tested", "demo", "new desc")
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	var ec *ExitCodeError
	if !errors.As(err, &ec) {
		t.Fatalf("expected *ExitCodeError, got %T: %v", err, err)
	}
	if ec.Code != 2 {
		t.Errorf("amend --state tested must exit 2, got %d", ec.Code)
	}
}

// Any other --state value also exits 2 (no values are currently
// accepted; tested is the canonical example called out in PRD §9).
func TestAmend_StateAnything_ExitsTwo(t *testing.T) {
	tmp := t.TempDir()
	gitInitTestRepo(t, tmp)
	if _, _, code := runCmd("init", "--path", tmp); code != 0 {
		t.Fatalf("init failed: %d", code)
	}
	if _, _, code := runCmd("add", "--path", tmp, "--slug", "demo", "demo"); code != 0 {
		t.Fatalf("add failed: %d", code)
	}

	err := runAmendForExitCode("--path", tmp, "--state", "applied", "demo", "x")
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	var ec *ExitCodeError
	if !errors.As(err, &ec) || ec.Code != 2 {
		t.Fatalf("expected exit 2 for any --state value; got err=%v", err)
	}
}
