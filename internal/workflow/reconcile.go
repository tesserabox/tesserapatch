package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tesserabox/tesserapatch/internal/gitutil"
	"github.com/tesserabox/tesserapatch/internal/provider"
	"github.com/tesserabox/tesserapatch/internal/store"
)

// ReconcileResult is the outcome for a single feature.
type ReconcileResult struct {
	Slug           string                 `json:"slug"`
	Title          string                 `json:"title"`
	Outcome        store.ReconcileOutcome `json:"outcome"`
	Phase          string                 `json:"phase"`
	UpstreamRef    string                 `json:"upstream_ref"`
	UpstreamCommit string                 `json:"upstream_commit"`
	Notes          []string               `json:"notes"`
	Conflicts      []string               `json:"conflicts,omitempty"`
}

// RunReconcile reconciles features against the upstream ref.
func RunReconcile(ctx context.Context, s *store.Store, slugs []string, upstreamRef string, prov provider.Provider, cfg provider.Config) ([]ReconcileResult, error) {
	// If no slugs specified, reconcile all applied/active features
	if len(slugs) == 0 {
		features, err := s.ListFeatures()
		if err != nil {
			return nil, err
		}
		for _, f := range features {
			if f.State == store.StateApplied || f.State == store.StateActive {
				slugs = append(slugs, f.Slug)
			}
		}
	}
	if len(slugs) == 0 {
		return nil, fmt.Errorf("no features to reconcile (no applied or active features found)")
	}

	// Resolve upstream commit
	upstreamCommit, err := gitutil.ResolveRef(s.Root, upstreamRef)
	if err != nil {
		return nil, fmt.Errorf("cannot resolve upstream ref %q: %w", upstreamRef, err)
	}

	results := make([]ReconcileResult, 0, len(slugs))

	// GAP 4: For multi-feature reconciliation, derive incremental patches.
	// Each feature's patch should only contain ITS changes, not prior features'.
	if len(slugs) > 1 {
		deriveIncrementalPatches(s, slugs, upstreamCommit)
	}

	for _, slug := range slugs {
		result, err := reconcileFeature(ctx, s, slug, upstreamRef, upstreamCommit, prov, cfg)
		if err != nil {
			results = append(results, ReconcileResult{
				Slug:           slug,
				Outcome:        store.ReconcileBlocked,
				Phase:          "error",
				UpstreamRef:    upstreamRef,
				UpstreamCommit: upstreamCommit,
				Notes:          []string{fmt.Sprintf("Error: %v", err)},
			})
			continue
		}
		results = append(results, *result)
	}

	// Update upstream.lock
	updateUpstreamLock(s, upstreamRef, upstreamCommit)

	return results, nil
}

func reconcileFeature(ctx context.Context, s *store.Store, slug, upstreamRef, upstreamCommit string, prov provider.Provider, cfg provider.Config) (*ReconcileResult, error) {
	status, err := s.LoadFeatureStatus(slug)
	if err != nil {
		return nil, err
	}

	// Load the recorded patch — prefer incremental patch if available (GAP 4)
	patch, err := s.ReadFeatureFile(slug, filepath.Join("artifacts", "incremental.patch"))
	if err != nil {
		patch, err = s.ReadFeatureFile(slug, filepath.Join("artifacts", "post-apply.patch"))
		if err != nil {
			return nil, fmt.Errorf("no recorded patch for feature %q — run 'tpatch record' first", slug)
		}
	}

	result := &ReconcileResult{
		Slug:           slug,
		Title:          status.Title,
		UpstreamRef:    upstreamRef,
		UpstreamCommit: upstreamCommit,
	}

	// Phase 1: Reverse-apply check (fast, free)
	reverseOK, _ := gitutil.ReverseApplyCheck(s.Root, patch)
	if reverseOK {
		result.Outcome = store.ReconcileUpstreamed
		result.Phase = "phase-1-reverse-apply"
		result.Notes = append(result.Notes, "Patch is already present in upstream (reverse-apply succeeded)")
		saveReconcileArtifacts(s, slug, result)
		updateFeatureState(s, slug, result)
		return result, nil
	}

	// Phase 2: Operation-level evaluation (if apply-recipe.json exists)
	recipeData, recipeErr := s.ReadFeatureFile(slug, filepath.Join("artifacts", "apply-recipe.json"))
	if recipeErr == nil && recipeData != "" {
		var recipe ApplyRecipe
		if err := json.Unmarshal([]byte(recipeData), &recipe); err == nil && len(recipe.Operations) > 0 {
			opResult := evaluateRecipeOperations(s.Root, recipe.Operations)
			if opResult.allPresent {
				result.Outcome = store.ReconcileUpstreamed
				result.Phase = "phase-2-operation-level"
				result.Notes = append(result.Notes, "All recipe operations already present in upstream")
				saveReconcileArtifacts(s, slug, result)
				updateFeatureState(s, slug, result)
				return result, nil
			}
			if opResult.hasConflicts {
				result.Notes = append(result.Notes, fmt.Sprintf("Operation-level: %d present, %d applicable, %d conflicts",
					opResult.presentCount, opResult.applicableCount, opResult.conflictCount))
			}
		}
	}

	// Phase 3: Provider-assisted semantic check (if provider available)
	if prov != nil && cfg.Configured() {
		request, _ := s.ReadFeatureFile(slug, "request.md")
		spec, _ := s.ReadFeatureFile(slug, "spec.md")

		// Extract affected files from patch and read their current upstream content
		upstreamContext := extractUpstreamContext(s.Root, patch)

		semanticResult, err := providerSemanticCheck(ctx, prov, cfg, request, spec, patch, upstreamRef, upstreamContext)
		if err == nil {
			if semanticResult == "upstreamed" {
				result.Outcome = store.ReconcileUpstreamed
				result.Phase = "phase-3-provider-semantic"
				result.Notes = append(result.Notes, "Provider determined upstream satisfies acceptance criteria")
				saveReconcileArtifacts(s, slug, result)
				updateFeatureState(s, slug, result)
				return result, nil
			}
			result.Notes = append(result.Notes, fmt.Sprintf("Provider semantic check: %s", semanticResult))
		} else {
			result.Notes = append(result.Notes, fmt.Sprintf("Provider semantic check error: %v", err))
		}
	}

	// Phase 4: Forward-apply attempt (safety net)
	forwardOK, _ := gitutil.ForwardApplyCheck(s.Root, patch)
	if forwardOK {
		result.Outcome = store.ReconcileReapplied
		result.Phase = "phase-4-forward-apply"
		result.Notes = append(result.Notes, "Patch can be re-applied cleanly to upstream")
		saveReconcileArtifacts(s, slug, result)
		updateFeatureState(s, slug, result)
		return result, nil
	}

	// All phases exhausted — blocked
	result.Outcome = store.ReconcileBlocked
	result.Phase = "phase-4-blocked"
	result.Notes = append(result.Notes, "Patch cannot be applied cleanly — manual intervention needed")
	result.Conflicts = append(result.Conflicts, "Forward-apply failed — check for merge conflicts")
	saveReconcileArtifacts(s, slug, result)
	updateFeatureState(s, slug, result)
	return result, nil
}

// Operation-level evaluation

type operationEvalResult struct {
	allPresent      bool
	hasConflicts    bool
	presentCount    int
	applicableCount int
	conflictCount   int
}

func evaluateRecipeOperations(repoRoot string, ops []RecipeOperation) operationEvalResult {
	result := operationEvalResult{}

	for _, op := range ops {
		switch op.Type {
		case "replace-in-file":
			filePath := filepath.Join(repoRoot, op.Path)
			content, err := os.ReadFile(filePath)
			if err != nil {
				result.conflictCount++
				result.hasConflicts = true
				continue
			}
			contentStr := string(content)
			if strings.Contains(contentStr, op.Replace) {
				result.presentCount++
			} else if strings.Contains(contentStr, op.Search) {
				result.applicableCount++
			} else {
				result.conflictCount++
				result.hasConflicts = true
			}

		case "write-file":
			filePath := filepath.Join(repoRoot, op.Path)
			content, err := os.ReadFile(filePath)
			if err == nil && strings.TrimSpace(string(content)) == strings.TrimSpace(op.Content) {
				result.presentCount++
			} else if err != nil {
				result.applicableCount++ // file doesn't exist, can be created
			} else {
				result.conflictCount++
				result.hasConflicts = true
			}

		case "ensure-directory":
			result.presentCount++ // directories are always fine

		default:
			result.applicableCount++
		}
	}

	result.allPresent = result.presentCount > 0 && result.conflictCount == 0 && result.applicableCount == 0
	return result
}

// Provider-assisted semantic check
func providerSemanticCheck(ctx context.Context, prov provider.Provider, cfg provider.Config, request, spec, patch, upstreamRef, upstreamContext string) (string, error) {
	systemPrompt := `You are evaluating whether an upstream update has incorporated a local feature's changes.

Compare the feature's acceptance criteria against the current upstream code.
The "Current Upstream Code" section shows what the relevant files look like NOW in upstream.
If the upstream now satisfies the feature's requirements (even if implemented differently), respond with: {"decision": "upstreamed", "reasoning": "..."}
If the feature is still needed (upstream does NOT have equivalent functionality), respond with: {"decision": "still_needed", "reasoning": "..."}
If you cannot determine, respond with: {"decision": "unclear", "reasoning": "..."}

Output ONLY valid JSON.`

	userPrompt := fmt.Sprintf(`# Feature Request
%s

# Feature Specification
%s

# Recorded Patch (our local changes)
%s

# Upstream Ref: %s

# Current Upstream Code (relevant files as they exist now)
%s

Does the upstream now satisfy this feature's requirements? Compare the acceptance criteria against the current upstream code shown above.`, request, spec, patch, upstreamRef, upstreamContext)

	response, err := prov.Generate(ctx, cfg, provider.GenerateRequest{
		SystemPrompt: systemPrompt,
		UserPrompt:   userPrompt,
		MaxTokens:    1024,
		Temperature:  0.1,
	})
	if err != nil {
		return "", err
	}

	// Parse JSON response
	cleaned := response
	if idx := strings.Index(cleaned, "{"); idx >= 0 {
		cleaned = cleaned[idx:]
		if end := strings.LastIndex(cleaned, "}"); end >= 0 {
			cleaned = cleaned[:end+1]
		}
	}

	var decision struct {
		Decision  string `json:"decision"`
		Reasoning string `json:"reasoning"`
	}
	if err := json.Unmarshal([]byte(cleaned), &decision); err != nil {
		return "unclear", nil
	}

	return decision.Decision, nil
}

// Save reconciliation artifacts
func saveReconcileArtifacts(s *store.Store, slug string, result *ReconcileResult) {
	// Save reconcile-session.json
	data, _ := json.MarshalIndent(result, "", "  ")
	s.WriteArtifact(slug, "reconcile-session.json", string(data)+"\n")

	// Save reconcile.md
	var b strings.Builder
	b.WriteString(fmt.Sprintf("# Reconciliation: %s\n\n", slug))
	b.WriteString(fmt.Sprintf("**Outcome**: %s\n", result.Outcome))
	b.WriteString(fmt.Sprintf("**Phase**: %s\n", result.Phase))
	b.WriteString(fmt.Sprintf("**Upstream Ref**: %s\n", result.UpstreamRef))
	b.WriteString(fmt.Sprintf("**Upstream Commit**: %s\n", result.UpstreamCommit))
	b.WriteString(fmt.Sprintf("**Timestamp**: %s\n\n", time.Now().UTC().Format(time.RFC3339)))

	if len(result.Notes) > 0 {
		b.WriteString("## Notes\n\n")
		for _, note := range result.Notes {
			b.WriteString("- " + note + "\n")
		}
		b.WriteString("\n")
	}
	if len(result.Conflicts) > 0 {
		b.WriteString("## Conflicts\n\n")
		for _, c := range result.Conflicts {
			b.WriteString("- " + c + "\n")
		}
		b.WriteString("\n")
	}
	s.WriteArtifact(slug, "reconcile.md", b.String())

	// Save per-version log
	commitRange := fmt.Sprintf("%s-to-%s", truncateCommit(result.UpstreamCommit), "HEAD")
	s.WriteFeatureFile(slug, filepath.Join("reconciliation", commitRange+".md"), b.String())
}

// Update feature state based on reconciliation outcome
func updateFeatureState(s *store.Store, slug string, result *ReconcileResult) {
	status, err := s.LoadFeatureStatus(slug)
	if err != nil {
		return
	}

	status.Reconcile = store.ReconcileSummary{
		AttemptedAt:    time.Now().UTC().Format(time.RFC3339),
		UpstreamRef:    result.UpstreamRef,
		UpstreamCommit: result.UpstreamCommit,
		Outcome:        result.Outcome,
	}
	status.LastCommand = "reconcile"
	status.UpdatedAt = time.Now().UTC().Format(time.RFC3339)

	switch result.Outcome {
	case store.ReconcileUpstreamed:
		status.State = store.StateUpstreamMerged
		status.Notes = "Feature adopted by upstream — local patch retired"
	case store.ReconcileReapplied:
		status.State = store.StateApplied
		status.Notes = "Patch re-applied cleanly to new upstream"
	case store.ReconcileStillNeeded:
		status.State = store.StateApplied
		status.Notes = "Feature still needed — partial upstream adoption"
	case store.ReconcileBlocked:
		status.State = store.StateBlocked
		status.Notes = "Reconciliation blocked — manual intervention needed"
	}

	s.SaveFeatureStatus(status)
}

func updateUpstreamLock(s *store.Store, ref, commit string) {
	content := fmt.Sprintf(`# Upstream Lock
# Updated by tpatch reconcile at %s
remote: upstream
branch: %s
commit: %s
url: ""
`, time.Now().UTC().Format(time.RFC3339), ref, commit)
	lockPath := filepath.Join(s.TpatchDir(), "upstream.lock")
	os.WriteFile(lockPath, []byte(content), 0o644)
}

func truncateCommit(commit string) string {
	if len(commit) > 8 {
		return commit[:8]
	}
	return commit
}

// deriveIncrementalPatches computes per-feature incremental patches for multi-feature scenarios.
// When features A and B are applied sequentially, B's cumulative patch includes A's changes.
// This function derives the delta (incremental) patch for each feature and saves it alongside
// the cumulative patch so reconciliation uses only the feature's own changes.
func deriveIncrementalPatches(s *store.Store, slugs []string, baseCommit string) {
	var prevCumulative string

	for _, slug := range slugs {
		currentPatch, err := s.ReadFeatureFile(slug, filepath.Join("artifacts", "post-apply.patch"))
		if err != nil {
			prevCumulative = currentPatch
			continue
		}

		if prevCumulative == "" {
			// First feature: incremental = cumulative
			s.WriteArtifact(slug, "incremental.patch", currentPatch)
		} else {
			// Derive delta between previous cumulative and current cumulative
			incremental, err := gitutil.DeriveIncrementalPatch(s.Root, baseCommit, prevCumulative, currentPatch)
			if err != nil || incremental == "" {
				// Fallback: use the full patch if derivation fails
				s.WriteArtifact(slug, "incremental.patch", currentPatch)
			} else {
				s.WriteArtifact(slug, "incremental.patch", incremental)
			}
		}

		prevCumulative = currentPatch
	}
}

// extractUpstreamContext reads the current contents of files affected by the patch.
// This gives the LLM the actual upstream code to compare against acceptance criteria.
func extractUpstreamContext(repoRoot, patch string) string {
	var files []string
	seen := make(map[string]bool)
	for _, line := range strings.Split(patch, "\n") {
		if strings.HasPrefix(line, "+++ b/") {
			file := strings.TrimPrefix(line, "+++ b/")
			if !seen[file] && file != "/dev/null" {
				seen[file] = true
				files = append(files, file)
			}
		} else if strings.HasPrefix(line, "--- a/") {
			file := strings.TrimPrefix(line, "--- a/")
			if !seen[file] && file != "/dev/null" {
				seen[file] = true
				files = append(files, file)
			}
		}
	}

	var b strings.Builder
	for _, file := range files {
		content, err := os.ReadFile(filepath.Join(repoRoot, file))
		if err != nil {
			b.WriteString(fmt.Sprintf("## %s\n(file not present in upstream)\n\n", file))
			continue
		}
		// Truncate large files to keep prompt manageable
		text := string(content)
		if len(text) > 4000 {
			text = text[:4000] + "\n... (truncated)"
		}
		b.WriteString(fmt.Sprintf("## %s\n```\n%s\n```\n\n", file, text))
	}
	return b.String()
}
