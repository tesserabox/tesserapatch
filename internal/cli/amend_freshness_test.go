package cli

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
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
// amend MUST clear the Verify record so the next freshness derivation
// derives `never-verified` (truthful — no fresh verify run exists).
//
// This is the live Case C reproduction from the external supervisor's
// Slice B revision-3 review: seed a passed Verify record that was
// computed against recipe v1, overwrite apply-recipe.json with recipe
// v2 (simulating a manual edit between verify and amend), then run
// the REAL `tpatch amend` command via the cobra root. Verify must be
// cleared. The previous helper-only test missed this because it
// bypassed amendCmd entirely.
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

	// Seed recipe v1, then compute its hash and persist a Verify
	// record claiming we verified against v1.
	recipeV1 := []byte(`{"v":1}`)
	if err := s.WriteArtifact("demo", "apply-recipe.json", string(recipeV1)); err != nil {
		t.Fatal(err)
	}
	v1Sum := sha256.Sum256(recipeV1)
	v1Hash := hex.EncodeToString(v1Sum[:])

	st, _ := s.LoadFeatureStatus("demo")
	st.Verify = &store.VerifyRecord{
		VerifiedAt:         "2025-01-01T00:00:00Z",
		Passed:             true,
		RecipeHashAtVerify: v1Hash,
	}
	if err := s.SaveFeatureStatus(st); err != nil {
		t.Fatal(err)
	}

	// Overwrite the recipe with v2 — drift between verify and amend.
	if err := s.WriteArtifact("demo", "apply-recipe.json", `{"v":2}`); err != nil {
		t.Fatal(err)
	}

	// Run amend through the real cobra root (the supervisor's live
	// Case C reproduction).
	if _, _, code := runCmd("amend", "--path", tmp, "demo", "new desc"); code != 0 {
		t.Fatalf("amend failed: %d", code)
	}

	got, err := s.LoadFeatureStatus("demo")
	if err != nil {
		t.Fatal(err)
	}
	if got.Verify != nil {
		t.Fatalf("recipe-touching amend must clear Verify (live CLI path); got %+v", got.Verify)
	}
}

// Recipe IDENTITY (no drift) preserves Verify. This pins the negative
// path: amend that does not touch the recipe and finds the on-disk
// recipe matching the persisted Verify hash leaves Verify alone.
func TestAmend_RecipeIdentity_PreservesVerify(t *testing.T) {
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

	recipe := []byte(`{"v":1}`)
	if err := s.WriteArtifact("demo", "apply-recipe.json", string(recipe)); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(recipe)
	hash := hex.EncodeToString(sum[:])

	st, _ := s.LoadFeatureStatus("demo")
	st.Verify = &store.VerifyRecord{
		VerifiedAt:         "2025-01-01T00:00:00Z",
		Passed:             true,
		RecipeHashAtVerify: hash,
	}
	if err := s.SaveFeatureStatus(st); err != nil {
		t.Fatal(err)
	}

	if _, _, code := runCmd("amend", "--path", tmp, "demo", "new desc"); code != 0 {
		t.Fatalf("amend failed: %d", code)
	}

	got, err := s.LoadFeatureStatus("demo")
	if err != nil {
		t.Fatal(err)
	}
	if got.Verify == nil {
		t.Fatalf("recipe-identity amend must preserve Verify; got nil")
	}
	if got.Verify.RecipeHashAtVerify != hash {
		t.Fatalf("Verify.RecipeHashAtVerify mutated: got %q want %q", got.Verify.RecipeHashAtVerify, hash)
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
	// Seed a recipe that the persisted Verify is consistent with;
	// otherwise the new producer-set rule (recipe-differs-from-Verify
	// → clear) would fire here, and we'd be testing the wrong path.
	recipe := []byte(`{"v":1}`)
	if err := s.WriteArtifact("c", "apply-recipe.json", string(recipe)); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(recipe)
	hash := hex.EncodeToString(sum[:])
	st.Verify = &store.VerifyRecord{
		VerifiedAt:         "2025-01-01T00:00:00Z",
		Passed:             true,
		RecipeHashAtVerify: hash,
	}
	if err := s.SaveFeatureStatus(st); err != nil {
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
