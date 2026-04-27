package workflow

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tesseracode/tesserapatch/internal/safety"
	"github.com/tesseracode/tesserapatch/internal/store"
)

// RecipeExecResult is the result of executing or dry-running a recipe.
type RecipeExecResult struct {
	Success    bool     `json:"success"`
	Operations int      `json:"operations"`
	Applied    int      `json:"applied"`
	Skipped    int      `json:"skipped"`
	Errors     []string `json:"errors,omitempty"`
	Messages   []string `json:"messages,omitempty"`
	// Warnings collects non-fatal advisories emitted during dry-run
	// (e.g. C5 F2: hard-parent created_by + missing target downgrades
	// from E to W per PRD §4.3). Execute-mode never populates this —
	// the same condition aborts apply with ErrPathCreatedByParent.
	Warnings []string `json:"warnings,omitempty"`
}

// DryRunRecipe validates a recipe against the codebase without modifying anything.
//
// Takes the store so the M14 created_by apply-time gate (ADR-011 D4) can
// classify each op against the child feature's declared dependencies.
// When Config.FeaturesDependencies is false the gate is a no-op and
// behaviour is byte-identical to v0.5.3.
//
// C5 F2: per PRD §4.3, dry-run downgrades hard-parent created_by misses
// to a warning (the op is reported as "would succeed once parent is
// applied" rather than aborting the recipe). Execute-mode keeps the
// hard error to abort apply. Recipe-shape validation errors (created_by
// names a feature outside depends_on) remain hard errors in both modes.
func DryRunRecipe(s *store.Store, recipe ApplyRecipe) RecipeExecResult {
	result := RecipeExecResult{Operations: len(recipe.Operations)}
	for _, op := range recipe.Operations {
		msg, warn, err := dryRunOperation(s, recipe.Feature, op)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("[%s] %s: %v", op.Type, op.Path, err))
			continue
		}
		result.Applied++
		result.Messages = append(result.Messages, msg)
		if warn != "" {
			result.Warnings = append(result.Warnings, fmt.Sprintf("[%s] %s: %s", op.Type, op.Path, warn))
		}
	}
	result.Success = len(result.Errors) == 0
	return result
}

// ExecuteRecipe applies recipe operations to the codebase with path safety checks.
//
// See DryRunRecipe re: the created_by apply-time gate.
func ExecuteRecipe(s *store.Store, recipe ApplyRecipe) RecipeExecResult {
	result := RecipeExecResult{Operations: len(recipe.Operations)}
	for _, op := range recipe.Operations {
		if err := executeOperation(s, recipe.Feature, op); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("[%s] %s: %v", op.Type, op.Path, err))
		} else {
			result.Applied++
			result.Messages = append(result.Messages, fmt.Sprintf("[%s] %s: OK", op.Type, op.Path))
		}
	}
	result.Success = len(result.Errors) == 0
	return result
}

// LoadRecipe reads and parses apply-recipe.json for a feature.
func LoadRecipe(s *store.Store, slug string) (ApplyRecipe, error) {
	data, err := s.ReadFeatureFile(slug, filepath.Join("artifacts", "apply-recipe.json"))
	if err != nil {
		return ApplyRecipe{}, fmt.Errorf("no recipe found — run 'tpatch implement %s' first", slug)
	}
	var recipe ApplyRecipe
	if err := json.Unmarshal([]byte(data), &recipe); err != nil {
		return ApplyRecipe{}, fmt.Errorf("invalid recipe JSON: %w", err)
	}
	return recipe, nil
}

func dryRunOperation(s *store.Store, slug string, op RecipeOperation) (string, string, error) {
	repoRoot := s.Root
	target := filepath.Join(repoRoot, op.Path)
	if err := safety.EnsureSafeRepoPath(repoRoot, target); err != nil {
		return "", "", fmt.Errorf("path safety: %w", err)
	}

	switch op.Type {
	case "write-file":
		if _, err := os.Stat(filepath.Dir(target)); os.IsNotExist(err) {
			return "", "", fmt.Errorf("parent directory does not exist")
		}
		return fmt.Sprintf("[write-file] would write %s (%d bytes)", op.Path, len(op.Content)), "", nil

	case "replace-in-file":
		// M14 created_by gate (ADR-011 D4): classify the op before the
		// bare not-found error so a hard-parent miss surfaces with
		// actionable context. Soft parents emit a warning then fall
		// through to the existing not-found path.
		_, statErr := os.Stat(target)
		targetExists := statErr == nil
		if gateErr := checkCreatedByGate(s, slug, op, targetExists); gateErr != nil {
			// C5 F2 / PRD §4.3: dry-run downgrades a hard-parent
			// created_by miss from E to W. The op is reported as "would
			// succeed once parent is applied" so the recipe-level
			// summary shows it among Applied (not Errors). Recipe-shape
			// validation errors (parent-not-in-depends_on, unknown kind)
			// keep their hard-error status in both dry-run and execute.
			if errors.Is(gateErr, ErrPathCreatedByParent) {
				warn := fmt.Sprintf("warning: path %s will be created by parent feature %s; apply %s before executing", op.Path, op.CreatedBy, op.CreatedBy)
				return fmt.Sprintf("[replace-in-file] %s (deferred — created_by parent %s)", op.Path, op.CreatedBy), warn, nil
			}
			return "", "", gateErr
		}
		content, err := os.ReadFile(target)
		if err != nil {
			return "", "", fmt.Errorf("file not found: %w", err)
		}
		idx := strings.Index(string(content), op.Search)
		if idx < 0 {
			return "", "", fmt.Errorf("search text not found in %s", op.Path)
		}
		line := strings.Count(string(content[:idx]), "\n") + 1
		return fmt.Sprintf("[replace-in-file] would replace in %s (match at line %d)", op.Path, line), "", nil

	case "append-file":
		_, statErr := os.Stat(target)
		targetExists := statErr == nil
		if gateErr := checkCreatedByGate(s, slug, op, targetExists); gateErr != nil {
			if errors.Is(gateErr, ErrPathCreatedByParent) {
				warn := fmt.Sprintf("warning: path %s will be created by parent feature %s; apply %s before executing", op.Path, op.CreatedBy, op.CreatedBy)
				return fmt.Sprintf("[append-file] %s (deferred — created_by parent %s)", op.Path, op.CreatedBy), warn, nil
			}
			return "", "", gateErr
		}
		if !targetExists {
			return "", "", fmt.Errorf("file not found: %s", op.Path)
		}
		return fmt.Sprintf("[append-file] would append to %s (%d bytes)", op.Path, len(op.Content)), "", nil

	case "ensure-directory":
		if info, err := os.Stat(target); err == nil && info.IsDir() {
			return fmt.Sprintf("[ensure-directory] %s already exists", op.Path), "", nil
		}
		return fmt.Sprintf("[ensure-directory] would create %s", op.Path), "", nil

	default:
		return "", "", fmt.Errorf("unknown operation type %q", op.Type)
	}
}

func executeOperation(s *store.Store, slug string, op RecipeOperation) error {
	repoRoot := s.Root
	target := filepath.Join(repoRoot, op.Path)
	if err := safety.EnsureSafeRepoPath(repoRoot, target); err != nil {
		return fmt.Errorf("path safety: %w", err)
	}

	switch op.Type {
	case "write-file":
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, []byte(op.Content), 0o644)

	case "replace-in-file":
		_, statErr := os.Stat(target)
		targetExists := statErr == nil
		if gateErr := checkCreatedByGate(s, slug, op, targetExists); gateErr != nil {
			return gateErr
		}
		content, err := os.ReadFile(target)
		if err != nil {
			return fmt.Errorf("file not found: %w", err)
		}
		text := string(content)
		if !strings.Contains(text, op.Search) {
			return fmt.Errorf("search text not found in %s", op.Path)
		}
		replaced := strings.Replace(text, op.Search, op.Replace, 1)
		return os.WriteFile(target, []byte(replaced), 0o644)

	case "append-file":
		_, statErr := os.Stat(target)
		targetExists := statErr == nil
		if gateErr := checkCreatedByGate(s, slug, op, targetExists); gateErr != nil {
			return gateErr
		}
		f, err := os.OpenFile(target, os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = f.WriteString(op.Content)
		return err

	case "ensure-directory":
		return os.MkdirAll(target, 0o755)

	default:
		return fmt.Errorf("unknown operation type %q", op.Type)
	}
}
