// Package workflow orchestrates the 7-phase lifecycle:
// analyse → define → explore → implement → test → record → reconcile.
package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tesserabox/tesserapatch/internal/provider"
	"github.com/tesserabox/tesserapatch/internal/store"
)

// AnalysisResult is the structured output of the analysis phase.
type AnalysisResult struct {
	Summary             string              `json:"summary"`
	Compatibility       CompatibilityResult `json:"compatibility"`
	AffectedAreas       []string            `json:"affected_areas"`
	AcceptanceCriteria  []string            `json:"acceptance_criteria"`
	ImplementationNotes []string            `json:"implementation_notes"`
	UnresolvedQuestions []string            `json:"unresolved_questions"`
	HeuristicMode       bool                `json:"heuristic_mode"`
}

// CompatibilityResult describes compatibility assessment.
type CompatibilityResult struct {
	Status    store.CompatibilityStatus `json:"status"`
	Reasoning string                    `json:"reasoning"`
}

// RunAnalysis executes the analysis phase for a feature.
func RunAnalysis(ctx context.Context, s *store.Store, slug string, prov provider.Provider, cfg provider.Config) (*AnalysisResult, error) {
	// Load feature request
	request, err := s.ReadFeatureFile(slug, "request.md")
	if err != nil {
		return nil, fmt.Errorf("cannot read feature request: %w", err)
	}

	// Gather workspace context
	fileTree := captureFileTree(s.Root, 3)
	guidance := readGuidanceFiles(s.Root)

	var result *AnalysisResult

	if prov != nil && cfg.Configured() {
		// LLM-assisted analysis
		systemPrompt := buildAnalysisSystemPrompt()
		userPrompt := buildAnalysisUserPrompt(request, fileTree, guidance)

		storeCfg, _ := s.LoadConfig()
		var tmp AnalysisResult
		response, err := GenerateWithRetry(ctx, prov, cfg, provider.GenerateRequest{
			SystemPrompt: systemPrompt,
			UserPrompt:   userPrompt,
			MaxTokens:    4096,
			Temperature:  0.1,
		}, RetryOptions{
			MaxRetries: storeCfg.MaxRetries,
			Validate:   JSONObjectValidator(&tmp),
			LogPrefix:  "analyze",
			Slug:       slug,
			Store:      s,
		})
		if err != nil {
			// Fall back to heuristic on provider/validation error
			result = heuristicAnalysis(request, slug)
			result.UnresolvedQuestions = append(result.UnresolvedQuestions, fmt.Sprintf("Provider error: %v", err))
			if response != "" {
				s.WriteArtifact(slug, "raw-analysis-response.txt", response)
			}
		} else {
			result, err = parseAnalysisResponse(response)
			if err != nil {
				// Should be unreachable (validator already parsed), but be defensive.
				result = heuristicAnalysis(request, slug)
				result.ImplementationNotes = append(result.ImplementationNotes, "Raw LLM response available in artifacts")
				s.WriteArtifact(slug, "raw-analysis-response.txt", response)
			}
		}
	} else {
		// Heuristic mode
		result = heuristicAnalysis(request, slug)
	}

	// Save artifacts
	analysisJSON, _ := json.MarshalIndent(result, "", "  ")
	if err := s.WriteArtifact(slug, "analysis.json", string(analysisJSON)+"\n"); err != nil {
		return nil, err
	}

	// Save analysis.md
	analysisMD := renderAnalysisMD(result, slug)
	if err := s.WriteFeatureFile(slug, "analysis.md", analysisMD); err != nil {
		return nil, err
	}

	// Update state
	state := store.StateAnalyzed
	notes := result.Summary
	if err := s.MarkFeatureState(slug, state, "analyze", notes); err != nil {
		return nil, err
	}

	return result, nil
}

// RunDefine generates acceptance criteria and implementation plan.
func RunDefine(ctx context.Context, s *store.Store, slug string, prov provider.Provider, cfg provider.Config) error {
	request, err := s.ReadFeatureFile(slug, "request.md")
	if err != nil {
		return fmt.Errorf("cannot read feature request: %w", err)
	}

	analysisJSON, err := s.ReadFeatureFile(slug, filepath.Join("artifacts", "analysis.json"))
	analysisTxt := ""
	if err == nil {
		analysisTxt = analysisJSON
	}

	analysisMD, _ := s.ReadFeatureFile(slug, "analysis.md")

	var specContent string
	if prov != nil && cfg.Configured() {
		systemPrompt := "You are a senior software engineer. Generate acceptance criteria and an implementation plan for the following feature request. Output as markdown with ## Acceptance Criteria (numbered list) and ## Implementation Plan sections."
		userPrompt := fmt.Sprintf("# Feature Request\n\n%s\n\n# Analysis\n\n%s\n%s", request, analysisMD, analysisTxt)

		storeCfg, _ := s.LoadConfig()
		response, err := GenerateWithRetry(ctx, prov, cfg, provider.GenerateRequest{
			SystemPrompt: systemPrompt,
			UserPrompt:   userPrompt,
			MaxTokens:    4096,
		}, RetryOptions{
			MaxRetries: storeCfg.MaxRetries,
			Validate:   NonEmptyValidator(),
			LogPrefix:  "define",
			Slug:       slug,
			Store:      s,
		})
		if err != nil {
			specContent = heuristicDefine(slug, request)
		} else {
			specContent = fmt.Sprintf("# Specification: %s\n\n%s\n", slug, response)
		}
	} else {
		specContent = heuristicDefine(slug, request)
	}

	if err := s.WriteFeatureFile(slug, "spec.md", specContent); err != nil {
		return err
	}

	return s.MarkFeatureState(slug, store.StateDefined, "define", "Acceptance criteria and plan generated")
}

// RunExplore reads relevant codebase files and produces an exploration log.
func RunExplore(ctx context.Context, s *store.Store, slug string, prov provider.Provider, cfg provider.Config) error {
	request, err := s.ReadFeatureFile(slug, "request.md")
	if err != nil {
		return fmt.Errorf("cannot read feature request: %w", err)
	}

	analysisMD, _ := s.ReadFeatureFile(slug, "analysis.md")
	specMD, _ := s.ReadFeatureFile(slug, "spec.md")

	fileTree := captureFileTree(s.Root, 4)

	var explorationContent string
	if prov != nil && cfg.Configured() {
		systemPrompt := "You are a senior software engineer exploring a codebase. Identify the specific files and code sections relevant to implementing the requested feature. Output as markdown with ## Relevant Files (list with paths and descriptions) and ## Minimal Changeset (what needs to change)."
		userPrompt := fmt.Sprintf("# Feature\n%s\n\n# Analysis\n%s\n\n# Spec\n%s\n\n# File Tree\n```\n%s\n```", request, analysisMD, specMD, fileTree)

		storeCfg, _ := s.LoadConfig()
		response, err := GenerateWithRetry(ctx, prov, cfg, provider.GenerateRequest{
			SystemPrompt: systemPrompt,
			UserPrompt:   userPrompt,
			MaxTokens:    4096,
		}, RetryOptions{
			MaxRetries: storeCfg.MaxRetries,
			Validate:   NonEmptyValidator(),
			LogPrefix:  "explore",
			Slug:       slug,
			Store:      s,
		})
		if err != nil {
			explorationContent = heuristicExplore(slug, fileTree)
		} else {
			explorationContent = fmt.Sprintf("# Exploration: %s\n\n%s\n", slug, response)
		}
	} else {
		explorationContent = heuristicExplore(slug, fileTree)
	}

	if err := s.WriteFeatureFile(slug, "exploration.md", explorationContent); err != nil {
		return err
	}

	return s.MarkFeatureState(slug, store.StateDefined, "explore", "Exploration complete")
}

// Heuristic fallbacks for offline mode

func heuristicAnalysis(request, slug string) *AnalysisResult {
	return &AnalysisResult{
		Summary:       fmt.Sprintf("Heuristic analysis for feature '%s'. Manual review recommended.", slug),
		HeuristicMode: true,
		Compatibility: CompatibilityResult{
			Status:    store.CompatibilityUnknown,
			Reasoning: "Heuristic mode — no LLM available for compatibility assessment",
		},
		AffectedAreas:       []string{"(manual identification needed)"},
		AcceptanceCriteria:  []string{"Feature works as described in request", "Existing tests pass", "No regressions introduced", "Changes documented"},
		ImplementationNotes: []string{"Analysis generated in heuristic mode — connect a provider for detailed analysis"},
		UnresolvedQuestions: []string{"Detailed compatibility assessment pending provider connection"},
	}
}

func heuristicDefine(slug, request string) string {
	return fmt.Sprintf(`# Specification: %s

## Acceptance Criteria

1. Feature works as described in the request
2. All existing tests continue to pass
3. No regressions introduced
4. Changes are documented

## Implementation Plan

1. Review the feature request and analysis
2. Identify affected files and code areas
3. Implement the changes
4. Run tests and verify acceptance criteria
5. Record the changes

*Generated in heuristic mode — connect a provider for detailed analysis.*
`, slug)
}

func heuristicExplore(slug, fileTree string) string {
	return fmt.Sprintf(`# Exploration: %s

## File Tree

`+"```"+`
%s
`+"```"+`

## Relevant Files

*(Manual identification needed — connect a provider for automated exploration.)*

## Minimal Changeset

*(Pending detailed analysis.)*
`, slug, fileTree)
}

// Capture file tree up to maxDepth levels deep.
func captureFileTree(root string, maxDepth int) string {
	var b strings.Builder
	walkTree(&b, root, "", 0, maxDepth)
	return b.String()
}

func walkTree(b *strings.Builder, path, prefix string, depth, maxDepth int) {
	if depth >= maxDepth {
		return
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return
	}
	for _, entry := range entries {
		name := entry.Name()
		// Skip common non-essential directories
		if entry.IsDir() && (name == "node_modules" || name == ".git" || name == ".tpatch" || name == "dist" || name == "build" || name == "__pycache__" || name == ".next") {
			continue
		}
		b.WriteString(prefix + name)
		if entry.IsDir() {
			b.WriteString("/\n")
			walkTree(b, filepath.Join(path, name), prefix+"  ", depth+1, maxDepth)
		} else {
			b.WriteString("\n")
		}
	}
}

// Read guidance files (PATCHING.md, CONTRIBUTING.md, etc.)
func readGuidanceFiles(root string) string {
	candidates := []string{"PATCHING.md", "CONTRIBUTING.md", "AGENTS.md", "CLAUDE.md"}
	var parts []string
	for _, name := range candidates {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err == nil && len(data) > 0 {
			parts = append(parts, fmt.Sprintf("### %s\n\n%s", name, string(data)))
		}
	}
	return strings.Join(parts, "\n\n---\n\n")
}

// Prompt builders

func buildAnalysisSystemPrompt() string {
	return `You are a senior software engineer analyzing a feature request for a forked open-source project.

Analyze the request and produce a JSON response with these fields:
{
  "summary": "one-paragraph summary of the feature",
  "compatibility": {
    "status": "compatible|conflict|unclear",
    "reasoning": "why this assessment"
  },
  "affected_areas": ["list of code areas/files affected"],
  "acceptance_criteria": ["testable criteria that must be met"],
  "implementation_notes": ["key implementation considerations"],
  "unresolved_questions": ["questions that need answers"]
}

Be specific about file paths when you can identify them from the file tree. Output ONLY valid JSON.`
}

func buildAnalysisUserPrompt(request, fileTree, guidance string) string {
	var b strings.Builder
	b.WriteString("# Feature Request\n\n")
	b.WriteString(request)
	b.WriteString("\n\n# Project File Tree\n\n```\n")
	b.WriteString(fileTree)
	b.WriteString("```\n")
	if guidance != "" {
		b.WriteString("\n# Project Guidance Files\n\n")
		b.WriteString(guidance)
	}
	return b.String()
}

func parseAnalysisResponse(response string) (*AnalysisResult, error) {
	// Try to extract JSON from the response (may be wrapped in markdown code block)
	cleaned := response
	if idx := strings.Index(cleaned, "```json"); idx >= 0 {
		cleaned = cleaned[idx+7:]
		if end := strings.Index(cleaned, "```"); end >= 0 {
			cleaned = cleaned[:end]
		}
	} else if idx := strings.Index(cleaned, "```"); idx >= 0 {
		cleaned = cleaned[idx+3:]
		if end := strings.Index(cleaned, "```"); end >= 0 {
			cleaned = cleaned[:end]
		}
	}
	cleaned = strings.TrimSpace(cleaned)

	var result AnalysisResult
	if err := json.Unmarshal([]byte(cleaned), &result); err != nil {
		return nil, fmt.Errorf("cannot parse analysis response as JSON: %w", err)
	}
	return &result, nil
}

func renderAnalysisMD(result *AnalysisResult, slug string) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("# Analysis: %s\n\n", slug))

	if result.HeuristicMode {
		b.WriteString("*Generated in heuristic mode (no provider). Connect a provider for detailed analysis.*\n\n")
	}

	b.WriteString("## Summary\n\n")
	b.WriteString(result.Summary + "\n\n")

	b.WriteString("## Compatibility\n\n")
	b.WriteString(fmt.Sprintf("**Status**: %s\n\n", result.Compatibility.Status))
	if result.Compatibility.Reasoning != "" {
		b.WriteString(result.Compatibility.Reasoning + "\n\n")
	}

	if len(result.AffectedAreas) > 0 {
		b.WriteString("## Affected Areas\n\n")
		for _, area := range result.AffectedAreas {
			b.WriteString("- " + area + "\n")
		}
		b.WriteString("\n")
	}

	if len(result.AcceptanceCriteria) > 0 {
		b.WriteString("## Acceptance Criteria\n\n")
		for i, c := range result.AcceptanceCriteria {
			b.WriteString(fmt.Sprintf("%d. %s\n", i+1, c))
		}
		b.WriteString("\n")
	}

	if len(result.ImplementationNotes) > 0 {
		b.WriteString("## Implementation Notes\n\n")
		for _, note := range result.ImplementationNotes {
			b.WriteString("- " + note + "\n")
		}
		b.WriteString("\n")
	}

	if len(result.UnresolvedQuestions) > 0 {
		b.WriteString("## Unresolved Questions\n\n")
		for _, q := range result.UnresolvedQuestions {
			b.WriteString("- " + q + "\n")
		}
		b.WriteString("\n")
	}

	return b.String()
}
