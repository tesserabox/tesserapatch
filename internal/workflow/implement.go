package workflow

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/tesserabox/tesserapatch/internal/provider"
	"github.com/tesserabox/tesserapatch/internal/store"
)

// ApplyRecipe is the deterministic operation format for applying changes.
type ApplyRecipe struct {
	Feature    string            `json:"feature"`
	Operations []RecipeOperation `json:"operations"`
}

// RecipeOperation is a single operation in an apply recipe.
type RecipeOperation struct {
	Type    string `json:"type"`    // write-file, replace-in-file, append-file, ensure-directory
	Path    string `json:"path"`    // target file path (relative to repo root)
	Content string `json:"content"` // for write-file, append-file
	Search  string `json:"search"`  // for replace-in-file: text to find
	Replace string `json:"replace"` // for replace-in-file: replacement text
}

// RunImplement generates a deterministic apply recipe for a feature.
func RunImplement(ctx context.Context, s *store.Store, slug string, prov provider.Provider, cfg provider.Config) error {
	request, err := s.ReadFeatureFile(slug, "request.md")
	if err != nil {
		return fmt.Errorf("cannot read feature request: %w", err)
	}

	spec, _ := s.ReadFeatureFile(slug, "spec.md")
	exploration, _ := s.ReadFeatureFile(slug, "exploration.md")

	var recipeContent string
	if prov != nil && cfg.Configured() {
		systemPrompt := `You are a senior software engineer. Generate an apply recipe (a JSON array of file operations) to implement the requested feature.

Each operation has:
- "type": one of "write-file", "replace-in-file", "append-file", "ensure-directory"
- "path": target file path relative to repo root
- "content": file content (for write-file, append-file)
- "search": text to find (for replace-in-file)
- "replace": replacement text (for replace-in-file)

Output ONLY valid JSON: {"feature": "<slug>", "operations": [...]}`

		userPrompt := fmt.Sprintf("# Feature: %s\n\n## Request\n%s\n\n## Spec\n%s\n\n## Exploration\n%s",
			slug, request, spec, exploration)

		storeCfg, _ := s.LoadConfig()
		var tmp ApplyRecipe
		response, err := GenerateWithRetry(ctx, prov, cfg, provider.GenerateRequest{
			SystemPrompt: systemPrompt,
			UserPrompt:   userPrompt,
			MaxTokens:    8192,
			Temperature:  0.1,
		}, RetryOptions{
			MaxRetries: storeCfg.MaxRetries,
			Validate:   JSONObjectValidator(&tmp),
			LogPrefix:  "implement",
			Slug:       slug,
			Store:      s,
		})
		if err != nil {
			recipeContent = heuristicRecipe(slug)
		} else {
			recipeContent = response
		}
	} else {
		recipeContent = heuristicRecipe(slug)
	}

	// Try to parse and re-serialize for clean formatting
	var recipe ApplyRecipe
	if err := json.Unmarshal([]byte(extractJSON(recipeContent)), &recipe); err != nil {
		// Save raw content if not valid JSON
		if err := s.WriteArtifact(slug, "apply-recipe.json", recipeContent); err != nil {
			return err
		}
	} else {
		data, _ := json.MarshalIndent(recipe, "", "  ")
		if err := s.WriteArtifact(slug, "apply-recipe.json", string(data)+"\n"); err != nil {
			return err
		}
	}

	return s.MarkFeatureState(slug, store.StateDefined, "implement", "Apply recipe generated")
}

func heuristicRecipe(slug string) string {
	recipe := ApplyRecipe{
		Feature: slug,
		Operations: []RecipeOperation{
			{
				Type:    "ensure-directory",
				Path:    "src/",
				Content: "",
			},
		},
	}
	data, _ := json.MarshalIndent(recipe, "", "  ")
	return string(data)
}

func extractJSON(s string) string {
	// Try to find JSON in the string (may be wrapped in markdown)
	if idx := findIndex(s, "```json"); idx >= 0 {
		s = s[idx+7:]
		if end := findIndex(s, "```"); end >= 0 {
			s = s[:end]
		}
	} else if idx := findIndex(s, "{"); idx >= 0 {
		s = s[idx:]
	}
	return s
}

func findIndex(s, substr string) int {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
