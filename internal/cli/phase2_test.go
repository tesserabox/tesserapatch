package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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

	_, _, code := runCmd("provider", "set", "--path", tmpDir, "--type", "anthropic",
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
	_, _, code = runCmd("provider", "set", "--path", tmpDir, "--type", "bogus")
	if code == 0 {
		t.Errorf("expected failure for invalid provider type")
	}
}

func TestProviderSetPreset(t *testing.T) {
	tmpDir := t.TempDir()
	runCmd("init", "--path", tmpDir)

	if _, _, code := runCmd("provider", "set", "--path", tmpDir, "--preset", "openrouter"); code != 0 {
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
	if _, _, code := runCmd("provider", "set", "--path", tmpDir, "--preset", "anthropic", "--model", "claude-opus-4"); code != 0 {
		t.Fatalf("provider set --preset anthropic --model failed")
	}
	out, _, _ = runCmd("config", "show", "--path", tmpDir)
	if !strings.Contains(out, "type: anthropic") || !strings.Contains(out, "claude-opus-4") {
		t.Errorf("expected anthropic type + claude-opus-4 model, got:\n%s", out)
	}

	// Unknown preset should fail.
	if _, _, code := runCmd("provider", "set", "--path", tmpDir, "--preset", "bogus"); code == 0 {
		t.Errorf("expected failure for unknown preset")
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
