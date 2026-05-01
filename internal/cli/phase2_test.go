package cli

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestMain isolates the package's global config IO from the user's real
// ~/.config/tpatch/config.yaml. All tests in this package redirect
// XDG_CONFIG_HOME to a per-run temp dir so `provider set` (default
// global scope after bug-provider-set-global) cannot clobber the
// developer's machine config.
func TestMain(m *testing.M) {
	tmp, err := os.MkdirTemp("", "tpatch-cli-xdg-*")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(tmp)
	_ = os.Setenv("XDG_CONFIG_HOME", tmp)
	os.Exit(m.Run())
}

func TestCycleBatchHeuristic(t *testing.T) {
	tmpDir := t.TempDir()
	runCmd("init", "--path", tmpDir)
	runCmd("add", "--path", tmpDir, "Fix model translation")

	out, _, code := runCmd("cycle", "--path", tmpDir, "fix-model-translation", "--skip-execute")
	if code != 0 {
		t.Fatalf("cycle failed (code %d): %s", code, out)
	}
	for _, want := range []string{"[1/6] Analyzing", "[2/6] Defining", "[3/6] Exploring", "[4/6] Generating apply recipe"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in cycle output:\n%s", want, out)
		}
	}
	if !strings.Contains(out, "--skip-execute set") {
		t.Errorf("expected --skip-execute acknowledgment, got:\n%s", out)
	}

	// Regression for bug-cycle-state-mismatch: after cycle --skip-execute
	// the feature must be in state `implementing` with last_command=`implement`.
	// Previously, RunImplement set state=defined, so status lied about
	// where the lifecycle was.
	statusPath := filepath.Join(tmpDir, ".tpatch", "features", "fix-model-translation", "status.json")
	raw, err := os.ReadFile(statusPath)
	if err != nil {
		t.Fatalf("read status.json: %v", err)
	}
	var status struct {
		State       string `json:"state"`
		LastCommand string `json:"last_command"`
	}
	if err := json.Unmarshal(raw, &status); err != nil {
		t.Fatalf("parse status.json: %v", err)
	}
	if status.State != "implementing" {
		t.Errorf("post-cycle state: want %q, got %q (last_command=%q)",
			"implementing", status.State, status.LastCommand)
	}
	if status.LastCommand != "implement" {
		t.Errorf("post-cycle last_command: want %q, got %q",
			"implement", status.LastCommand)
	}
}

func TestNextEmitsHarnessJSON(t *testing.T) {
	tmpDir := t.TempDir()
	runCmd("init", "--path", tmpDir)
	runCmd("add", "--path", tmpDir, "Fix model translation")

	out, _, code := runCmd("next", "--path", tmpDir, "fix-model-translation", "--format", "harness-json")
	if code != 0 {
		t.Fatalf("next failed (code %d): %s", code, out)
	}
	var task struct {
		Phase        string   `json:"phase"`
		Slug         string   `json:"slug"`
		State        string   `json:"state"`
		Instructions string   `json:"instructions"`
		ContextFiles []string `json:"context_files"`
		OnComplete   string   `json:"on_complete"`
	}
	if err := json.Unmarshal([]byte(out), &task); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, out)
	}
	if task.Phase != "analyze" {
		t.Errorf("expected phase 'analyze' for newly requested feature, got %q", task.Phase)
	}
	if task.Slug != "fix-model-translation" {
		t.Errorf("unexpected slug %q", task.Slug)
	}
	if task.OnComplete == "" {
		t.Error("expected non-empty on_complete")
	}
}

func TestNextProgressesWithState(t *testing.T) {
	tmpDir := t.TempDir()
	runCmd("init", "--path", tmpDir)
	runCmd("add", "--path", tmpDir, "Fix model translation")
	runCmd("analyze", "--path", tmpDir, "fix-model-translation")

	out, _, _ := runCmd("next", "--path", tmpDir, "fix-model-translation", "--format", "harness-json")
	if !strings.Contains(out, `"phase": "define"`) {
		t.Fatalf("expected phase define after analyze; got:\n%s", out)
	}
}

func TestTestCommandMissing(t *testing.T) {
	tmpDir := t.TempDir()
	runCmd("init", "--path", tmpDir)
	runCmd("add", "--path", tmpDir, "Demo")

	_, _, code := runCmd("test", "--path", tmpDir, "demo")
	if code == 0 {
		t.Fatalf("expected non-zero exit when test_command missing")
	}
}

func TestTestCommandRuns(t *testing.T) {
	tmpDir := t.TempDir()
	runCmd("init", "--path", tmpDir)
	runCmd("add", "--path", tmpDir, "Demo")
	runCmd("config", "--path", tmpDir, "set", "test_command", "echo hello-tests")

	out, _, code := runCmd("test", "--path", tmpDir, "demo")
	if code != 0 {
		t.Fatalf("test command failed (code %d): %s", code, out)
	}
	if !strings.Contains(out, "hello-tests") {
		t.Errorf("expected command output, got:\n%s", out)
	}
	if !strings.Contains(out, "Tests passed") {
		t.Errorf("expected 'Tests passed', got:\n%s", out)
	}
	// test-output.txt artifact should exist
	p := filepath.Join(tmpDir, ".tpatch", "features", "demo", "artifacts", "test-output.txt")
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("missing test-output.txt: %v", err)
	}
	if !strings.Contains(string(data), "hello-tests") {
		t.Errorf("artifact missing command output: %s", string(data))
	}
}

func TestProviderSetType(t *testing.T) {
	tmpDir := t.TempDir()
	runCmd("init", "--path", tmpDir)

	_, _, code := runCmd("provider", "set", "--path", tmpDir, "--repo", "--type", "anthropic",
		"--base-url", "https://api.anthropic.com", "--model", "claude-sonnet-4-5",
		"--auth-env", "ANTHROPIC_API_KEY")
	if code != 0 {
		t.Fatalf("provider set --type failed")
	}
	out, _, _ := runCmd("config", "show", "--path", tmpDir)
	if !strings.Contains(out, "type: anthropic") {
		t.Errorf("expected type: anthropic in config, got:\n%s", out)
	}

	// Invalid type should fail
	_, _, code = runCmd("provider", "set", "--path", tmpDir, "--repo", "--type", "bogus")
	if code == 0 {
		t.Errorf("expected failure for invalid provider type")
	}
}

func TestProviderSetPreset(t *testing.T) {
	tmpDir := t.TempDir()
	runCmd("init", "--path", tmpDir)

	if _, _, code := runCmd("provider", "set", "--path", tmpDir, "--repo", "--preset", "openrouter"); code != 0 {
		t.Fatalf("provider set --preset openrouter failed")
	}
	out, _, _ := runCmd("config", "show", "--path", tmpDir)
	if !strings.Contains(out, "openrouter.ai") {
		t.Errorf("expected openrouter base URL, got:\n%s", out)
	}
	if !strings.Contains(out, "OPENROUTER_API_KEY") {
		t.Errorf("expected OPENROUTER_API_KEY auth env, got:\n%s", out)
	}

	// Preset + model override should compose.
	if _, _, code := runCmd("provider", "set", "--path", tmpDir, "--repo", "--preset", "anthropic", "--model", "claude-opus-4"); code != 0 {
		t.Fatalf("provider set --preset anthropic --model failed")
	}
	out, _, _ = runCmd("config", "show", "--path", tmpDir)
	if !strings.Contains(out, "type: anthropic") || !strings.Contains(out, "claude-opus-4") {
		t.Errorf("expected anthropic type + claude-opus-4 model, got:\n%s", out)
	}

	// Unknown preset should fail.
	if _, _, code := runCmd("provider", "set", "--path", tmpDir, "--repo", "--preset", "bogus"); code == 0 {
		t.Errorf("expected failure for unknown preset")
	}
}

// TestCopilotPresetNoAuthEnv pins the contract that `--preset copilot`
// produces a config with no auth_env. The local copilot-api proxy
// strips and replaces inbound auth headers with its own session token
// (lib/api-config.ts:copilotHeaders), so requiring users to set
// GITHUB_TOKEN was theatre that polluted the proxy logs without
// affecting upstream behaviour.
func TestCopilotPresetNoAuthEnv(t *testing.T) {
	tmpDir := t.TempDir()
	runCmd("init", "--path", tmpDir)

	if _, _, code := runCmd("provider", "set", "--path", tmpDir, "--repo", "--preset", "copilot"); code != 0 {
		t.Fatalf("provider set --preset copilot failed")
	}
	out, _, _ := runCmd("config", "show", "--path", tmpDir)
	if !strings.Contains(out, "localhost:4141") {
		t.Errorf("expected localhost:4141 base URL, got:\n%s", out)
	}
	// auth_env should be empty or absent — never GITHUB_TOKEN.
	if strings.Contains(out, "GITHUB_TOKEN") {
		t.Errorf("copilot preset should not set GITHUB_TOKEN auth_env, got:\n%s", out)
	}
}

// TestProviderSetGlobalDefault is the regression for bug-provider-set-global.
// Previously, `tpatch provider set --preset X` outside a repo failed with
// "could not find .tpatch". Provider config is user-level (same Copilot
// seat across repos), so set must default to the global config path.
func TestProviderSetGlobalDefault(t *testing.T) {
	// Run from a tmp cwd with NO .tpatch/; must not look for one.
	cwd, _ := os.Getwd()
	defer os.Chdir(cwd)
	away := t.TempDir()
	if err := os.Chdir(away); err != nil {
		t.Fatal(err)
	}

	out, _, code := runCmd("provider", "set", "--preset", "openrouter")
	if code != 0 {
		t.Fatalf("provider set (global) failed: %s", out)
	}
	if !strings.Contains(out, "global (") {
		t.Errorf("expected target label `global`, got:\n%s", out)
	}

	// Verify the global config file was written under our XDG override.
	xdg := os.Getenv("XDG_CONFIG_HOME")
	globalPath := filepath.Join(xdg, "tpatch", "config.yaml")
	data, err := os.ReadFile(globalPath)
	if err != nil {
		t.Fatalf("global config not written: %v", err)
	}
	if !strings.Contains(string(data), "openrouter.ai") {
		t.Errorf("global config missing openrouter base URL:\n%s", string(data))
	}

	// --repo must still require a .tpatch/; the repoless cwd should fail.
	if _, _, code := runCmd("provider", "set", "--repo", "--preset", "openrouter"); code == 0 {
		t.Errorf("provider set --repo without .tpatch/ should fail")
	}
}

func TestConfigSetNewKeys(t *testing.T) {
	tmpDir := t.TempDir()
	runCmd("init", "--path", tmpDir)

	if _, _, code := runCmd("config", "--path", tmpDir, "set", "max_retries", "5"); code != 0 {
		t.Fatalf("config set max_retries failed")
	}
	if _, _, code := runCmd("config", "--path", tmpDir, "set", "test_command", "go test ./..."); code != 0 {
		t.Fatalf("config set test_command failed")
	}
	out, _, _ := runCmd("config", "show", "--path", tmpDir)
	if !strings.Contains(out, "max_retries: 5") {
		t.Errorf("expected max_retries: 5 in config:\n%s", out)
	}
	if !strings.Contains(out, "test_command:") {
		t.Errorf("expected test_command: in config:\n%s", out)
	}
}

// TestReconcileRefusesDirtyTree is the regression for A10
// doc-reconcile-workflow preflight. A modified tracked file must make
// `tpatch reconcile` exit non-zero without running any phase, and
// --allow-dirty must opt out of the block.
func TestReconcileRefusesDirtyTree(t *testing.T) {
	tmpDir := t.TempDir()
	if _, _, code := runCmd("init", "--path", tmpDir); code != 0 {
		t.Fatal("init failed")
	}
	// Seed a git repo with one committed file.
	mustGit := func(args ...string) {
		c := exec.Command("git", args...)
		c.Dir = tmpDir
		c.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t",
		)
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	mustGit("init", "-q", "-b", "main")
	mustGit("config", "commit.gpgsign", "false")
	if err := os.WriteFile(filepath.Join(tmpDir, "file.txt"), []byte("one\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit("add", "-A")
	mustGit("commit", "-q", "-m", "seed")

	// Dirty the tree.
	if err := os.WriteFile(filepath.Join(tmpDir, "file.txt"), []byte("two\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Reconcile must refuse with a preflight diagnostic on stderr.
	_, stderr, code := runCmd("reconcile", "--path", tmpDir)
	if code == 0 {
		t.Fatalf("reconcile must refuse dirty tree, exited 0")
	}
	if !strings.Contains(stderr, "clean working tree") {
		t.Errorf("expected `clean working tree` diagnostic, got:\n%s", stderr)
	}
	if !strings.Contains(stderr, "modified:") {
		t.Errorf("expected `modified:` violation listing, got:\n%s", stderr)
	}

	// --preflight alone should also refuse (same gate).
	_, stderr2, code2 := runCmd("reconcile", "--path", tmpDir, "--preflight")
	if code2 == 0 {
		t.Fatalf("--preflight on dirty tree must exit non-zero")
	}
	if !strings.Contains(stderr2, "modified:") {
		t.Errorf("--preflight should print violations, got:\n%s", stderr2)
	}
}

// Running `tpatch record` on a clean working tree with no --from used
// to write a 0-byte patch and mark the feature state=applied. Now it
// must refuse with exit 1 and surface --from candidates drawn from
// `git log`.
func TestRecordEmptyCaptureRefused(t *testing.T) {
	tmpDir := t.TempDir()

	// Stand up a tpatch repo with at least one real git commit so
	// RecentCommits has something to suggest.
	if _, _, code := runCmd("init", "--path", tmpDir); code != 0 {
		t.Fatalf("tpatch init failed")
	}
	if _, _, code := runCmd("add", "--path", tmpDir, "demo-feature"); code != 0 {
		t.Fatalf("tpatch add failed")
	}
	mustGit := func(args ...string) {
		c := exec.Command("git", args...)
		c.Dir = tmpDir
		c.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t",
		)
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	mustGit("init", "-q")
	mustGit("add", "-A")
	mustGit("commit", "-q", "-m", "seed: tpatch init")

	// Clean working tree, no --from → must refuse.
	_, errOut, code := runCmd("record", "--path", tmpDir, "demo-feature")
	if code == 0 {
		t.Fatalf("expected record to refuse empty capture, got exit 0")
	}
	if !strings.Contains(errOut, "captured 0 bytes") {
		t.Errorf("expected `captured 0 bytes` diagnostic, got stderr:\n%s", errOut)
	}
	if !strings.Contains(errOut, "--from") {
		t.Errorf("expected --from hint, got stderr:\n%s", errOut)
	}
	if !strings.Contains(errOut, "Recent commits") {
		t.Errorf("expected candidate commit list, got stderr:\n%s", errOut)
	}

	// Feature state must NOT have advanced — the old code wrote state=applied.
	statusPath := filepath.Join(tmpDir, ".tpatch", "features", "demo-feature", "status.json")
	raw, err := os.ReadFile(statusPath)
	if err != nil {
		t.Fatalf("read status.json: %v", err)
	}
	var status struct {
		State string `json:"state"`
	}
	_ = json.Unmarshal(raw, &status)
	if status.State == "applied" {
		t.Errorf("refused record must not advance state to applied; got %q", status.State)
	}
}

// TestManualAnalyzeAdvancesState — --manual with a hand-authored analysis.md
// advances feature state without calling the provider.
func TestManualAnalyzeAdvancesState(t *testing.T) {
	tmpDir := t.TempDir()
	runCmd("init", "--path", tmpDir)
	runCmd("add", "--path", tmpDir, "Manual analyze fixture")

	featureDir := filepath.Join(tmpDir, ".tpatch", "features", "manual-analyze-fixture")
	if err := os.WriteFile(filepath.Join(featureDir, "analysis.md"), []byte("# Manual Analysis\n\nhand-authored\n"), 0o644); err != nil {
		t.Fatalf("write analysis.md: %v", err)
	}

	out, _, code := runCmd("analyze", "--path", tmpDir, "manual-analyze-fixture", "--manual")
	if code != 0 {
		t.Fatalf("analyze --manual failed (code %d): %s", code, out)
	}
	if !strings.Contains(out, "advanced manually") || !strings.Contains(out, "manual mode") {
		t.Errorf("expected manual-mode acknowledgment, got:\n%s", out)
	}

	raw, err := os.ReadFile(filepath.Join(featureDir, "status.json"))
	if err != nil {
		t.Fatalf("read status.json: %v", err)
	}
	var status struct {
		State       string `json:"state"`
		LastCommand string `json:"last_command"`
		Notes       string `json:"notes"`
	}
	_ = json.Unmarshal(raw, &status)
	if status.State != "analyzed" {
		t.Errorf("state: want analyzed, got %q", status.State)
	}
	if status.LastCommand != "analyze" {
		t.Errorf("last_command: want analyze, got %q", status.LastCommand)
	}
	if !strings.Contains(status.Notes, "manually") {
		t.Errorf("expected notes to record manual transition; got %q", status.Notes)
	}
}

// TestManualRefusesMissingArtifact — --manual without the expected artifact
// must fail and point at the exact path the agent should author.
func TestManualRefusesMissingArtifact(t *testing.T) {
	tmpDir := t.TempDir()
	runCmd("init", "--path", tmpDir)
	runCmd("add", "--path", tmpDir, "Manual no artifact")

	_, _, code := runCmd("define", "--path", tmpDir, "manual-no-artifact", "--manual")
	if code == 0 {
		t.Fatalf("define --manual should refuse when spec.md is missing")
	}
	// state must not advance
	statusPath := filepath.Join(tmpDir, ".tpatch", "features", "manual-no-artifact", "status.json")
	raw, err := os.ReadFile(statusPath)
	if err != nil {
		t.Fatalf("read status.json: %v", err)
	}
	var status struct {
		State string `json:"state"`
	}
	_ = json.Unmarshal(raw, &status)
	if status.State != "requested" {
		t.Errorf("refused --manual must not advance state; got %q", status.State)
	}
}

// TestManualImplementValidatesJSON — --manual on implement refuses when the
// recipe file is not valid JSON.
func TestManualImplementValidatesJSON(t *testing.T) {
	tmpDir := t.TempDir()
	runCmd("init", "--path", tmpDir)
	runCmd("add", "--path", tmpDir, "Manual bad recipe")

	featureDir := filepath.Join(tmpDir, ".tpatch", "features", "manual-bad-recipe")
	if err := os.MkdirAll(filepath.Join(featureDir, "artifacts"), 0o755); err != nil {
		t.Fatalf("mkdir artifacts: %v", err)
	}
	if err := os.WriteFile(filepath.Join(featureDir, "artifacts", "apply-recipe.json"), []byte("not json at all"), 0o644); err != nil {
		t.Fatalf("write bad recipe: %v", err)
	}

	_, _, code := runCmd("implement", "--path", tmpDir, "manual-bad-recipe", "--manual")
	if code == 0 {
		t.Fatalf("implement --manual should refuse invalid JSON")
	}

	// valid JSON → should succeed
	if err := os.WriteFile(filepath.Join(featureDir, "artifacts", "apply-recipe.json"), []byte(`{"operations":[]}`), 0o644); err != nil {
		t.Fatalf("write good recipe: %v", err)
	}
	out, _, code := runCmd("implement", "--path", tmpDir, "manual-bad-recipe", "--manual")
	if code != 0 {
		t.Fatalf("implement --manual should succeed with valid JSON; out: %s", out)
	}
	raw, _ := os.ReadFile(filepath.Join(featureDir, "status.json"))
	var status struct {
		State string `json:"state"`
	}
	_ = json.Unmarshal(raw, &status)
	if status.State != "implementing" {
		t.Errorf("state: want implementing, got %q", status.State)
	}
}

// TestManualSkipLLMAlias — --skip-llm is an alias for --manual.
func TestManualSkipLLMAlias(t *testing.T) {
	tmpDir := t.TempDir()
	runCmd("init", "--path", tmpDir)
	runCmd("add", "--path", tmpDir, "Alias check")

	featureDir := filepath.Join(tmpDir, ".tpatch", "features", "alias-check")
	if err := os.WriteFile(filepath.Join(featureDir, "exploration.md"), []byte("# Exploration\n"), 0o644); err != nil {
		t.Fatalf("write exploration.md: %v", err)
	}

	out, _, code := runCmd("explore", "--path", tmpDir, "alias-check", "--skip-llm")
	if code != 0 {
		t.Fatalf("explore --skip-llm failed (code %d): %s", code, out)
	}
	if !strings.Contains(out, "advanced manually") {
		t.Errorf("expected manual acknowledgment via --skip-llm; got:\n%s", out)
	}
}
