package workflow

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tesserabox/tesserapatch/internal/safety"
	"github.com/tesserabox/tesserapatch/internal/store"
)

// RecipeExecResult is the result of executing or dry-running a recipe.
type RecipeExecResult struct {
	Success    bool     `json:"success"`
	Operations int      `json:"operations"`
	Applied    int      `json:"applied"`
	Skipped    int      `json:"skipped"`
	Errors     []string `json:"errors,omitempty"`
	Messages   []string `json:"messages,omitempty"`
}

// DryRunRecipe validates a recipe against the codebase without modifying anything.
func DryRunRecipe(repoRoot string, recipe ApplyRecipe) RecipeExecResult {
	result := RecipeExecResult{Operations: len(recipe.Operations)}
	for _, op := range recipe.Operations {
		msg, err := dryRunOperation(repoRoot, op)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("[%s] %s: %v", op.Type, op.Path, err))
		} else {
			result.Applied++
			result.Messages = append(result.Messages, msg)
		}
	}
	result.Success = len(result.Errors) == 0
	return result
}

// ExecuteRecipe applies recipe operations to the codebase with path safety checks.
func ExecuteRecipe(repoRoot string, recipe ApplyRecipe) RecipeExecResult {
	result := RecipeExecResult{Operations: len(recipe.Operations)}
	for _, op := range recipe.Operations {
		if err := executeOperation(repoRoot, op); err != nil {
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

func dryRunOperation(repoRoot string, op RecipeOperation) (string, error) {
	target := filepath.Join(repoRoot, op.Path)
	if err := safety.EnsureSafeRepoPath(repoRoot, target); err != nil {
		return "", fmt.Errorf("path safety: %w", err)
	}

	switch op.Type {
	case "write-file":
		if _, err := os.Stat(filepath.Dir(target)); os.IsNotExist(err) {
			return "", fmt.Errorf("parent directory does not exist")
		}
		return fmt.Sprintf("[write-file] would write %s (%d bytes)", op.Path, len(op.Content)), nil

	case "replace-in-file":
		content, err := os.ReadFile(target)
		if err != nil {
			return "", fmt.Errorf("file not found: %w", err)
		}
		idx := strings.Index(string(content), op.Search)
		if idx < 0 {
			return "", fmt.Errorf("search text not found in %s", op.Path)
		}
		line := strings.Count(string(content[:idx]), "\n") + 1
		return fmt.Sprintf("[replace-in-file] would replace in %s (match at line %d)", op.Path, line), nil

	case "append-file":
		if _, err := os.Stat(target); os.IsNotExist(err) {
			return "", fmt.Errorf("file not found: %s", op.Path)
		}
		return fmt.Sprintf("[append-file] would append to %s (%d bytes)", op.Path, len(op.Content)), nil

	case "ensure-directory":
		if info, err := os.Stat(target); err == nil && info.IsDir() {
			return fmt.Sprintf("[ensure-directory] %s already exists", op.Path), nil
		}
		return fmt.Sprintf("[ensure-directory] would create %s", op.Path), nil

	default:
		return "", fmt.Errorf("unknown operation type %q", op.Type)
	}
}

func executeOperation(repoRoot string, op RecipeOperation) error {
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
