package workflow

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/tesseracode/tesserapatch/internal/gitutil"
	"github.com/tesseracode/tesserapatch/internal/provider"
	"github.com/tesseracode/tesserapatch/internal/store"
)

// RecipeProvenance is the sidecar written alongside apply-recipe.json
// recording the commit the recipe was generated against and the sha256
// of the recipe bytes at generation time. The apply path reads it to
// warn both when HEAD has drifted (base_commit mismatch) and when the
// recipe file has been edited out-of-band (recipe_sha256 mismatch).
// Kept as a sidecar (not a field on ApplyRecipe) so the skill-parity
// guard's DisallowUnknownFields check does not require updates to 6
// skills.
//
// RecipeSHA256 uses a pointer so older sidecars (pre-v0.5.2) that
// omit the field decode as nil and the content-drift check is simply
// skipped — the HEAD check still runs.
type RecipeProvenance struct {
	BaseCommit   string  `json:"base_commit"`
	GeneratedAt  string  `json:"generated_at"`
	RecipeSHA256 *string `json:"recipe_sha256,omitempty"`
}

// WarnWriter receives non-fatal warnings emitted by workflow phases (e.g.
// when the implement phase falls back to a heuristic recipe because the
// LLM call failed validation). Defaults to os.Stderr; tests override it
// to capture output.
var WarnWriter io.Writer = os.Stderr

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

	// CreatedBy is an optional parent-feature slug declaring which feature
	// originated this file. It is an ordering / label hint consumed by the
	// DAG features added in M14.3+ (label composition, topo reconcile).
	//
	// In M14.2 the field is persisted but inert — no apply / reconcile /
	// record code reads it, and the implement phase does not yet populate
	// it. Wiring is deferred to M14.3 once a real consumer (label
	// composition) lands.
	//
	// `omitempty` is load-bearing: recipes that do not declare DAG
	// provenance must round-trip byte-identical to v0.5.3.
	//
	// Reading rule (ADR-010 D5): for reconcile-result decisions, callers
	// must read status.Reconcile.Outcome — never artifacts/reconcile-session.json.
	CreatedBy string `json:"created_by,omitempty"`
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
		maxTokens := storeCfg.MaxTokensImplement
		if maxTokens <= 0 {
			maxTokens = store.DefaultMaxTokensImplement
		}
		var tmp ApplyRecipe
		response, err := GenerateWithRetry(ctx, prov, cfg, provider.GenerateRequest{
			SystemPrompt: systemPrompt,
			UserPrompt:   userPrompt,
			MaxTokens:    maxTokens,
			Temperature:  0.1,
		}, RetryOptions{
			MaxRetries: storeCfg.MaxRetries,
			Validate:   JSONObjectValidator(&tmp),
			LogPrefix:  "implement",
			Slug:       slug,
			Store:      s,
		})
		if err != nil {
			fmt.Fprintf(WarnWriter,
				"warning: implement LLM call failed after %d retries (%v); "+
					"falling back to a 1-operation heuristic recipe.\n"+
					"  Inspect raw responses at .tpatch/features/%s/artifacts/raw-implement-response-*.txt\n"+
					"  Retry with a larger budget: tpatch config set max_tokens_implement 32768\n",
				storeCfg.MaxRetries, err, slug)
			recipeContent = heuristicRecipe(slug)
		} else {
			recipeContent = response
		}
	} else {
		recipeContent = heuristicRecipe(slug)
	}

	// Try to parse and re-serialize for clean formatting
	var recipe ApplyRecipe
	if err := json.Unmarshal([]byte(mustExtractJSON(recipeContent)), &recipe); err != nil {
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

	// Write provenance sidecar so `apply` can warn when HEAD has
	// drifted from the commit this recipe was generated against OR
	// when the recipe bytes themselves have been edited out-of-band.
	// Best-effort: if HEAD is unreadable (e.g. the caller is not
	// inside a git repo), skip the sidecar — the guard is
	// backward-compatible with its absence.
	if commit, err := gitutil.HeadCommit(s.Root); err == nil && commit != "" {
		// Re-read the recipe from disk so the hash matches exactly
		// what future apply invocations will hash. Writing and then
		// reading is deliberately serialised; we avoid hashing the
		// in-memory buffer because trailing-newline normalisation in
		// WriteArtifact would make the two diverge.
		recipeBytes, rerr := s.ReadFeatureFile(slug, "artifacts/apply-recipe.json")
		prov := RecipeProvenance{
			BaseCommit:  commit,
			GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		}
		if rerr == nil {
			sum := sha256.Sum256([]byte(recipeBytes))
			hex := fmt.Sprintf("%x", sum[:])
			prov.RecipeSHA256 = &hex
		}
		data, _ := json.MarshalIndent(prov, "", "  ")
		_ = s.WriteArtifact(slug, "recipe-provenance.json", string(data)+"\n")
	}

	// State advances to "implementing" — the recipe is ready but the
	// code has not been executed/applied yet. The `apply` command moves
	// it the rest of the way through implementing → applied.
	return s.MarkFeatureState(slug, store.StateImplementing, "implement", "Apply recipe generated")
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

func extractJSON(s string) string { return mustExtractJSON(s) }

// mustExtractJSON is a thin adapter over ExtractJSONObject that
// preserves the old "best-effort, never panic" contract of the legacy
// helper: on parse failure it still returns a non-empty string so the
// downstream json.Unmarshal path produces its own structured error
// (which is what the retry loop keys off of).
func mustExtractJSON(s string) string {
	out, err := ExtractJSONObject(s)
	if err != nil {
		return out
	}
	return out
}

func findIndex(s, substr string) int {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
