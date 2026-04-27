// Package store provides the .tpatch/ data model, file I/O, and state management.
package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/tesseracode/tesserapatch/internal/safety"
)

// Store provides read/write access to the .tpatch/ workspace.
type Store struct {
	Root string // absolute path to the project root
}

// FindProjectRoot walks up from start looking for a .tpatch/ directory.
func FindProjectRoot(start string) (string, error) {
	current, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	for {
		if fileExists(filepath.Join(current, ".tpatch")) {
			return current, nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	return "", errors.New("could not find .tpatch in this directory or any parent")
}

// Init creates a new .tpatch/ workspace at root.
func Init(root string) (*Store, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}

	store := &Store{Root: absRoot}
	if fileExists(store.tpatchDir()) {
		return nil, fmt.Errorf("%s already exists — already initialized", store.tpatchDir())
	}

	// Create directory structure
	dirs := []string{
		store.featuresDir(),
		store.steeringDir(),
		store.workflowsDir(),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}

	// Write config.yaml
	configContent := `# Tessera Patch configuration
provider:
  type: openai-compatible  # openai-compatible | anthropic
  base_url: ""
  model: ""
  auth_env: ""  # env var name containing auth token (NOT the secret itself)

# Merge strategy for applying patches: "3way" (default) or "rebase"
merge_strategy: 3way

# Max LLM validation retries when output fails to parse (0 disables retry)
max_retries: 2

# Max output tokens for the implement phase (default 16384). Bump higher
# for features that emit many large file bodies inline.
max_tokens_implement: 16384

# Shell command run by ` + "`tpatch test <slug>`" + ` (e.g. "go test ./...", "bun test")
test_command: ""

# Feature dependency DAG (ADR-011). Default true from v0.6.0.
# Set to false to opt back into v0.5.x byte-identity behaviour.
features_dependencies: true
`
	if err := writeFile(store.configPath(), configContent); err != nil {
		return nil, err
	}

	// Write FEATURES.md
	featuresContent := "# Tracked Features\n\n*No features yet. Run `tpatch add <description>` to add one.*\n"
	if err := writeFile(store.featuresIndexPath(), featuresContent); err != nil {
		return nil, err
	}

	// Write upstream.lock
	lockContent := `# Upstream Lock
# Updated automatically by tpatch reconcile.
remote: ""
branch: ""
commit: ""
url: ""
`
	if err := writeFile(store.upstreamLockPath(), lockContent); err != nil {
		return nil, err
	}

	// Write steering files
	localSteering := "# Local Steering\n\n<!-- Add custom instructions for patching this project here. -->\n"
	if err := writeFile(filepath.Join(store.steeringDir(), "local.md"), localSteering); err != nil {
		return nil, err
	}

	upstreamSteering := "# Upstream Steering\n\n<!-- Cached PATCHING.md from upstream, if available. -->\n"
	if err := writeFile(filepath.Join(store.steeringDir(), "upstream.md"), upstreamSteering); err != nil {
		return nil, err
	}

	return store, nil
}

// Open loads an existing Store from root.
func Open(root string) (*Store, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	store := &Store{Root: absRoot}
	if !fileExists(store.tpatchDir()) {
		return nil, fmt.Errorf("%s is not initialized with tpatch — run 'tpatch init' first", absRoot)
	}
	return store, nil
}

// AddFeature creates a new feature directory with request.md and status.json.
func (s *Store) AddFeature(input AddFeatureInput) (FeatureStatus, error) {
	title := strings.TrimSpace(input.Title)
	request := strings.TrimSpace(input.Request)
	if title == "" {
		return FeatureStatus{}, errors.New("feature title is required")
	}
	if request == "" {
		request = title
	}

	slug := Slugify(input.Slug)
	if slug == "" {
		slug = Slugify(title)
	}
	if slug == "" {
		return FeatureStatus{}, errors.New("could not derive a valid feature slug")
	}

	featureDir := s.featureDir(slug)
	if fileExists(featureDir) {
		return FeatureStatus{}, fmt.Errorf("feature %q already exists", slug)
	}

	// Create feature directories
	if err := os.MkdirAll(s.featureArtifactsDir(slug), 0o755); err != nil {
		return FeatureStatus{}, err
	}
	if err := os.MkdirAll(s.featureReconciliationDir(slug), 0o755); err != nil {
		return FeatureStatus{}, err
	}

	now := nowStamp()
	status := FeatureStatus{
		ID:            slug,
		Slug:          slug,
		Title:         title,
		State:         StateRequested,
		Compatibility: CompatibilityUnknown,
		RequestedAt:   now,
		UpdatedAt:     now,
		LastCommand:   "add",
	}

	// Write request.md
	requestContent := fmt.Sprintf("# Feature Request: %s\n\n**Slug**: `%s`\n**Created**: %s\n\n## Description\n\n%s\n", title, slug, now, request)
	if err := writeFile(s.featureRequestPath(slug), requestContent); err != nil {
		return FeatureStatus{}, err
	}

	// Write status.json
	if err := s.SaveFeatureStatus(status); err != nil {
		return FeatureStatus{}, err
	}

	// Update FEATURES.md
	if err := s.RefreshFeaturesIndex(); err != nil {
		return FeatureStatus{}, err
	}

	return status, nil
}

// ListFeatures returns all tracked features sorted by slug.
func (s *Store) ListFeatures() ([]FeatureStatus, error) {
	entries, err := os.ReadDir(s.featuresDir())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}

	features := make([]FeatureStatus, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		status, err := s.LoadFeatureStatus(entry.Name())
		if err != nil {
			continue // skip features without valid status.json
		}
		features = append(features, status)
	}

	sort.Slice(features, func(i, j int) bool {
		return features[i].Slug < features[j].Slug
	})

	return features, nil
}

// LoadFeatureStatus reads the status.json for a feature.
func (s *Store) LoadFeatureStatus(slug string) (FeatureStatus, error) {
	data, err := os.ReadFile(s.featureStatusPath(slug))
	if err != nil {
		return FeatureStatus{}, err
	}
	var status FeatureStatus
	if err := json.Unmarshal(data, &status); err != nil {
		return FeatureStatus{}, err
	}
	return status, nil
}

// SaveFeatureStatus writes status.json for a feature and refreshes FEATURES.md
// so the human-readable index stays in sync with every state transition.
// Errors refreshing the index are swallowed: status.json is the source of
// truth and must land even if the derived index can't be rewritten (e.g.
// read-only FS, concurrent writer). The next SaveFeatureStatus call retries.
func (s *Store) SaveFeatureStatus(status FeatureStatus) error {
	if status.UpdatedAt == "" {
		status.UpdatedAt = nowStamp()
	}
	if err := writeJSON(s.featureStatusPath(status.Slug), status); err != nil {
		return err
	}
	_ = s.RefreshFeaturesIndex()
	return nil
}

// MarkFeatureState updates a feature's state and metadata.
func (s *Store) MarkFeatureState(slug string, state FeatureState, command, notes string) error {
	status, err := s.LoadFeatureStatus(slug)
	if err != nil {
		return err
	}
	status.State = state
	status.LastCommand = command
	status.UpdatedAt = nowStamp()
	status.Notes = strings.TrimSpace(notes)
	return s.SaveFeatureStatus(status)
}

// WriteVerifyRecord persists the freshness overlay produced by the
// explicit `tpatch verify` verb (ADR-013 D1 / D5). This is the ONLY
// store-level entry point for setting `FeatureStatus.Verify`; read paths
// (`LoadFeatureStatus`, `ComposeLabels`, status rendering) must NOT call
// it. Slice A: `tpatch verify` is the sole caller; Slice B will add
// `tpatch amend` (recipe-touching) as the second producer.
//
// `LastCommand = "verify"` and `UpdatedAt` are bumped via SaveFeatureStatus.
// `FeatureState` is left untouched — verify is a freshness overlay, not a
// lifecycle transition (ADR-013 D1).
func (s *Store) WriteVerifyRecord(slug string, record VerifyRecord) error {
	status, err := s.LoadFeatureStatus(slug)
	if err != nil {
		return err
	}
	rec := record
	status.Verify = &rec
	status.LastCommand = "verify"
	status.UpdatedAt = nowStamp()
	return s.SaveFeatureStatus(status)
}

// ReadFeatureFile reads a named file from the feature directory.
func (s *Store) ReadFeatureFile(slug, name string) (string, error) {
	data, err := os.ReadFile(filepath.Join(s.featureDir(slug), name))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// WriteFeatureFile writes a named file to the feature directory.
func (s *Store) WriteFeatureFile(slug, name, content string) error {
	target := filepath.Join(s.featureDir(slug), name)
	if err := safety.EnsureSafeRepoPath(s.Root, target); err != nil {
		return fmt.Errorf("unsafe path in WriteFeatureFile: %w", err)
	}
	return writeFile(target, content)
}

// WriteArtifact writes a file to the feature's artifacts directory.
func (s *Store) WriteArtifact(slug, name, content string) error {
	target := s.featureArtifactPath(slug, name)
	if err := safety.EnsureSafeRepoPath(s.Root, target); err != nil {
		return fmt.Errorf("unsafe path in WriteArtifact: %w", err)
	}
	return writeFile(target, content)
}

// LoadConfig reads the YAML config (parsed as simple key extraction for zero-dep).
func (s *Store) LoadConfig() (Config, error) {
	data, err := os.ReadFile(s.configPath())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Config{}, nil
		}
		return Config{}, err
	}
	return parseYAMLConfig(string(data)), nil
}

// SaveConfig writes the YAML config.
func (s *Store) SaveConfig(cfg Config) error {
	mergeStrat := cfg.MergeStrategy
	if mergeStrat == "" {
		mergeStrat = "3way"
	}
	maxRetries := cfg.MaxRetries
	if maxRetries < 0 {
		maxRetries = 0
	}
	maxTokensImplement := cfg.MaxTokensImplement
	if maxTokensImplement <= 0 {
		maxTokensImplement = DefaultMaxTokensImplement
	}
	featuresDeps := "false"
	if cfg.FeaturesDependencies {
		featuresDeps = "true"
	}
	initiatorLine := ""
	if cfg.Provider.Initiator != "" {
		initiatorLine = fmt.Sprintf("  initiator: %s\n", yamlQuote(cfg.Provider.Initiator))
	}
	content := fmt.Sprintf(`# Tessera Patch configuration
provider:
  type: %s
  base_url: %s
  model: %s
  auth_env: %s
%s
# Merge strategy for applying patches: "3way" (default) or "rebase"
merge_strategy: %s

# Max LLM validation retries when output fails to parse (0 disables retry)
max_retries: %d

# Max output tokens for the implement phase (default 16384). Bump higher
# for features that emit many large file bodies inline.
max_tokens_implement: %d

# Shell command run by `+"`tpatch test <slug>`"+` (e.g. "go test ./...", "bun test")
test_command: %s

# Feature dependency DAG (ADR-011). Default false until v0.6.0.
features_dependencies: %s
`, yamlQuote(cfg.Provider.Type), yamlQuote(cfg.Provider.BaseURL),
		yamlQuote(cfg.Provider.Model), yamlQuote(cfg.Provider.AuthEnv),
		initiatorLine, mergeStrat,
		maxRetries, maxTokensImplement, yamlQuote(cfg.TestCommand),
		featuresDeps)
	return writeFile(s.configPath(), content)
}

// RemoveFeature deletes the feature directory (including artifacts,
// patches, reconciliation, status.json) and refreshes FEATURES.md.
// Returns an error when the slug does not exist.
func (s *Store) RemoveFeature(slug string) error {
	dir := s.featureDir(slug)
	if _, err := os.Stat(dir); err != nil {
		return fmt.Errorf("feature %s does not exist", slug)
	}
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("remove %s: %w", dir, err)
	}
	return s.RefreshFeaturesIndex()
}

// HasPatchingInstructions checks for a PATCHING.md in the project root.
func (s *Store) HasPatchingInstructions() bool {
	return fileExists(filepath.Join(s.Root, "PATCHING.md"))
}

// RefreshFeaturesIndex rebuilds FEATURES.md from current feature state.
func (s *Store) RefreshFeaturesIndex() error {
	features, err := s.ListFeatures()
	if err != nil {
		return err
	}

	var b strings.Builder
	b.WriteString("# Tracked Features\n\n")
	if len(features) == 0 {
		b.WriteString("*No features yet. Run `tpatch add <description>` to add one.*\n")
	} else {
		b.WriteString("| Slug | Title | State | Compatibility |\n")
		b.WriteString("|------|-------|-------|---------------|\n")
		for _, f := range features {
			b.WriteString(fmt.Sprintf("| `%s` | %s | %s | %s |\n", f.Slug, f.Title, f.State, f.Compatibility))
		}
	}

	return writeFile(s.featuresIndexPath(), b.String())
}

// Path accessors

func (s *Store) tpatchDir() string             { return filepath.Join(s.Root, ".tpatch") }
func (s *Store) configPath() string            { return filepath.Join(s.tpatchDir(), "config.yaml") }
func (s *Store) featuresIndexPath() string     { return filepath.Join(s.tpatchDir(), "FEATURES.md") }
func (s *Store) upstreamLockPath() string      { return filepath.Join(s.tpatchDir(), "upstream.lock") }
func (s *Store) steeringDir() string           { return filepath.Join(s.tpatchDir(), "steering") }
func (s *Store) workflowsDir() string          { return filepath.Join(s.tpatchDir(), "workflows") }
func (s *Store) featuresDir() string           { return filepath.Join(s.tpatchDir(), "features") }
func (s *Store) featureDir(slug string) string { return filepath.Join(s.featuresDir(), slug) }
func (s *Store) featureArtifactsDir(slug string) string {
	return filepath.Join(s.featureDir(slug), "artifacts")
}
func (s *Store) featureReconciliationDir(slug string) string {
	return filepath.Join(s.featureDir(slug), "reconciliation")
}
func (s *Store) featureRequestPath(slug string) string {
	return filepath.Join(s.featureDir(slug), "request.md")
}
func (s *Store) featureStatusPath(slug string) string {
	return filepath.Join(s.featureDir(slug), "status.json")
}
func (s *Store) featureArtifactPath(slug, name string) string {
	return filepath.Join(s.featureArtifactsDir(slug), name)
}

// TpatchDir returns the path to .tpatch/.
func (s *Store) TpatchDir() string { return s.tpatchDir() }

// ConfigPath returns the path to config.yaml.
func (s *Store) ConfigPath() string { return s.configPath() }

// NextPatchNumber returns the next sequential patch number for a feature.
func (s *Store) NextPatchNumber(slug string) int {
	patchDir := filepath.Join(s.featureDir(slug), "patches")
	entries, err := os.ReadDir(patchDir)
	if err != nil {
		return 1
	}
	max := 0
	for _, e := range entries {
		name := e.Name()
		if len(name) >= 3 {
			var n int
			if _, err := fmt.Sscanf(name[:3], "%03d", &n); err == nil && n > max {
				max = n
			}
		}
	}
	return max + 1
}

// WritePatch writes a patch to the sequential patches/ directory.
func (s *Store) WritePatch(slug, label, content string) (string, error) {
	num := s.NextPatchNumber(slug)
	filename := fmt.Sprintf("%03d-%s.patch", num, label)
	patchDir := filepath.Join(s.featureDir(slug), "patches")
	if err := os.MkdirAll(patchDir, 0o755); err != nil {
		return "", err
	}
	target := filepath.Join(patchDir, filename)
	if err := safety.EnsureSafeRepoPath(s.Root, target); err != nil {
		return "", err
	}
	return filename, writeFile(target, content)
}

// SaveApplySession writes the apply-session.json artifact.
func (s *Store) SaveApplySession(slug string, session ApplySession) error {
	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return err
	}
	return s.WriteArtifact(slug, "apply-session.json", string(data)+"\n")
}

// Helpers

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return writeFile(path, string(data)+"\n")
}

func writeFile(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func nowStamp() string {
	return time.Now().UTC().Format(time.RFC3339)
}

// parseYAMLConfig does minimal YAML parsing for our known config structure.
// This avoids adding a yaml dependency — our config format is flat and simple.
func parseYAMLConfig(content string) Config {
	cfg := Config{}
	cfg.Provider.Type = extractYAMLValue(content, "type")
	cfg.Provider.BaseURL = extractYAMLValue(content, "base_url")
	cfg.Provider.Model = extractYAMLValue(content, "model")
	cfg.Provider.AuthEnv = extractYAMLValue(content, "auth_env")
	cfg.Provider.Initiator = extractYAMLValue(content, "initiator")
	cfg.MergeStrategy = extractYAMLValue(content, "merge_strategy")
	if cfg.MergeStrategy == "" {
		cfg.MergeStrategy = "3way"
	}
	if v := extractYAMLValue(content, "max_retries"); v != "" {
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil && n >= 0 {
			cfg.MaxRetries = n
		}
	} else {
		cfg.MaxRetries = 2
	}
	if v := extractYAMLValue(content, "max_tokens_implement"); v != "" {
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil && n > 0 {
			cfg.MaxTokensImplement = n
		}
	}
	cfg.TestCommand = extractYAMLValue(content, "test_command")
	if v := extractYAMLValue(content, "copilot_native_optin"); v == "true" {
		cfg.CopilotNativeOptIn = true
	}
	cfg.CopilotNativeOptInAt = extractYAMLValue(content, "copilot_native_optin_at")
	if v := extractYAMLValue(content, "features_dependencies"); v != "" {
		// Explicit value wins. Default (when key is absent) is true
		// from v0.6.0 onward — see ADR-011 D9 + PRD-feature-dependencies.
		cfg.FeaturesDependencies = v != "false"
	} else {
		cfg.FeaturesDependencies = true
	}
	return cfg
}

func extractYAMLValue(content, key string) string {
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, key+":") {
			val := strings.TrimPrefix(trimmed, key+":")
			val = strings.TrimSpace(val)
			// Remove surrounding quotes
			val = strings.Trim(val, "\"'")
			// Remove inline comments
			if idx := strings.Index(val, " #"); idx >= 0 {
				val = strings.TrimSpace(val[:idx])
				val = strings.Trim(val, "\"'")
			}
			return val
		}
	}
	return ""
}

func yamlQuote(s string) string {
	if s == "" {
		return `""`
	}
	// Quote if it contains special chars
	if strings.ContainsAny(s, ": #{}[]|>&*!%@`") {
		return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
	}
	return s
}
