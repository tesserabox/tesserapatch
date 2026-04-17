package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/tesserabox/tesserapatch/assets"
	"github.com/tesserabox/tesserapatch/internal/gitutil"
	"github.com/tesserabox/tesserapatch/internal/provider"
	"github.com/tesserabox/tesserapatch/internal/store"
	"github.com/tesserabox/tesserapatch/internal/workflow"
)

const version = "0.3.0-dev"

// Execute runs the tpatch CLI root command.
func Execute() int {
	rootCmd := buildRootCmd()
	if err := rootCmd.Execute(); err != nil {
		return 1
	}
	return 0
}

func buildRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "tpatch",
		Short:         "Tessera Patch — customize open-source projects with natural-language patches",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetVersionTemplate("tpatch {{.Version}}\n")

	// Persistent flags
	root.PersistentFlags().String("path", "", "Target repository path (default: current directory)")

	// Commands
	root.AddCommand(
		initCmd(),
		addCmd(),
		statusCmd(),
		analyzeCmd(),
		defineCmd(),
		exploreCmd(),
		implementCmd(),
		applyCmd(),
		recordCmd(),
		reconcileCmd(),
		providerCmd(),
		configCmd(),
		cycleCmd(),
		testCmd(),
		nextCmd(),
	)

	return root
}

// ─── init ────────────────────────────────────────────────────────────────────

func initCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init [path]",
		Short: "Initialize .tpatch/ workspace and install skill formats",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := resolveRoot(cmd, args)
			if err != nil {
				return err
			}
			s, err := store.Init(root)
			if err != nil {
				return err
			}

			installSkills(cmd, root)

			// GAP 6: Auto-detect provider
			autoDetectProvider(cmd, s)

			fmt.Fprintf(cmd.OutOrStdout(), "Initialized .tpatch/ in %s\n", s.Root)
			fmt.Fprintf(cmd.OutOrStdout(), "  config:    %s\n", filepath.Join(s.TpatchDir(), "config.yaml"))
			fmt.Fprintf(cmd.OutOrStdout(), "  features:  %s\n", filepath.Join(s.TpatchDir(), "FEATURES.md"))
			fmt.Fprintf(cmd.OutOrStdout(), "  steering:  %s\n", filepath.Join(s.TpatchDir(), "steering/"))
			return nil
		},
	}
	return cmd
}

// ─── add ─────────────────────────────────────────────────────────────────────

func addCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add <description...>",
		Short: "Create a tracked feature request",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := openStoreFromCmd(cmd)
			if err != nil {
				return err
			}
			slug, _ := cmd.Flags().GetString("slug")
			description := strings.Join(args, " ")

			status, err := s.AddFeature(store.AddFeatureInput{
				Title: description, Request: description, Slug: slug,
			})
			if err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Created feature: %s\n", status.Slug)
			fmt.Fprintf(cmd.OutOrStdout(), "  directory: .tpatch/features/%s/\n", status.Slug)
			fmt.Fprintf(cmd.OutOrStdout(), "  state:     %s\n", status.State)
			return nil
		},
	}
	cmd.Flags().String("slug", "", "Override feature slug")
	return cmd
}

// ─── status ──────────────────────────────────────────────────────────────────

func statusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status [slug]",
		Short: "Show feature status dashboard",
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := openStoreFromCmd(cmd)
			if err != nil {
				return err
			}

			features, err := s.ListFeatures()
			if err != nil {
				return err
			}
			cfg, err := s.LoadConfig()
			if err != nil {
				return err
			}

			asJSON, _ := cmd.Flags().GetBool("json")
			verbose, _ := cmd.Flags().GetBool("verbose")
			featureSlug, _ := cmd.Flags().GetString("feature")
			if featureSlug == "" && len(args) > 0 {
				featureSlug = args[0]
			}

			out := cmd.OutOrStdout()
			if asJSON {
				payload := map[string]any{
					"root": s.Root, "provider": cfg.Provider,
					"provider_configured": cfg.Provider.Configured(),
					"features":            features,
				}
				data, _ := json.MarshalIndent(payload, "", "  ")
				fmt.Fprintf(out, "%s\n", data)
				return nil
			}

			fmt.Fprintf(out, "Project: %s\n", s.Root)
			if cfg.Provider.Configured() {
				fmt.Fprintf(out, "Provider: %s (%s, model=%s)\n", cfg.Provider.Type, cfg.Provider.BaseURL, cfg.Provider.Model)
			} else {
				fmt.Fprintf(out, "Provider: not configured\n")
			}
			if len(features) == 0 {
				fmt.Fprintln(out, "Features: none")
				return nil
			}
			fmt.Fprintf(out, "Features: %d\n", len(features))
			for _, f := range features {
				fmt.Fprintf(out, "  - %s [%s] %s\n", f.Slug, f.State, f.Title)
			}
			if featureSlug != "" || verbose {
				slugs := []string{}
				if featureSlug != "" {
					slugs = append(slugs, featureSlug)
				} else {
					for _, f := range features {
						slugs = append(slugs, f.Slug)
					}
				}
				for _, sl := range slugs {
					st, err := s.LoadFeatureStatus(sl)
					if err != nil {
						continue
					}
					fmt.Fprintf(out, "\nDetail: %s\n", st.Slug)
					fmt.Fprintf(out, "  Title:         %s\n", st.Title)
					fmt.Fprintf(out, "  State:         %s\n", st.State)
					fmt.Fprintf(out, "  Compatibility: %s\n", st.Compatibility)
					fmt.Fprintf(out, "  Requested:     %s\n", st.RequestedAt)
					fmt.Fprintf(out, "  Updated:       %s\n", st.UpdatedAt)
					if st.Notes != "" {
						fmt.Fprintf(out, "  Notes:         %s\n", st.Notes)
					}
				}
			}
			return nil
		},
	}
	cmd.Flags().Bool("json", false, "Output as JSON")
	cmd.Flags().Bool("verbose", false, "Show all feature details")
	cmd.Flags().String("feature", "", "Show detail for one feature")
	return cmd
}

// ─── analyze ─────────────────────────────────────────────────────────────────

func analyzeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "analyze <slug>",
		Short: "Run analysis phase on a feature",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := openStoreFromCmd(cmd)
			if err != nil {
				return err
			}
			timeout, _ := cmd.Flags().GetDuration("timeout")
			prov, cfg := loadProviderFromStore(s)
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()
			if noRetry, _ := cmd.Flags().GetBool("no-retry"); noRetry {
				ctx = workflow.WithDisableRetry(ctx, true)
			}

			result, err := workflow.RunAnalysis(ctx, s, args[0], prov, cfg)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Analysis saved for %s\n", args[0])
			fmt.Fprintf(cmd.OutOrStdout(), "  Summary: %s\n", result.Summary)
			if result.HeuristicMode {
				fmt.Fprintln(cmd.OutOrStdout(), "  (heuristic mode — no provider connected)")
			}
			return nil
		},
	}
	cmd.Flags().Duration("timeout", 60*time.Second, "Analysis timeout")
	cmd.Flags().Bool("no-retry", false, "Disable retry-with-feedback on invalid LLM output")
	return cmd
}

// ─── define ──────────────────────────────────────────────────────────────────

func defineCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "define <slug>",
		Short: "Generate acceptance criteria and plan",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := openStoreFromCmd(cmd)
			if err != nil {
				return err
			}
			timeout, _ := cmd.Flags().GetDuration("timeout")
			prov, cfg := loadProviderFromStore(s)
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()
			if noRetry, _ := cmd.Flags().GetBool("no-retry"); noRetry {
				ctx = workflow.WithDisableRetry(ctx, true)
			}

			if err := workflow.RunDefine(ctx, s, args[0], prov, cfg); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Spec generated for %s\n", args[0])
			return nil
		},
	}
	cmd.Flags().Duration("timeout", 60*time.Second, "Timeout")
	cmd.Flags().Bool("no-retry", false, "Disable retry-with-feedback on invalid LLM output")
	return cmd
}

// ─── explore ─────────────────────────────────────────────────────────────────

func exploreCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "explore <slug>",
		Short: "Read codebase, find minimal changeset",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := openStoreFromCmd(cmd)
			if err != nil {
				return err
			}
			timeout, _ := cmd.Flags().GetDuration("timeout")
			prov, cfg := loadProviderFromStore(s)
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()
			if noRetry, _ := cmd.Flags().GetBool("no-retry"); noRetry {
				ctx = workflow.WithDisableRetry(ctx, true)
			}

			if err := workflow.RunExplore(ctx, s, args[0], prov, cfg); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Exploration saved for %s\n", args[0])
			return nil
		},
	}
	cmd.Flags().Duration("timeout", 60*time.Second, "Timeout")
	cmd.Flags().Bool("no-retry", false, "Disable retry-with-feedback on invalid LLM output")
	return cmd
}

// ─── implement ───────────────────────────────────────────────────────────────

func implementCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "implement <slug>",
		Short: "Generate deterministic apply recipe",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := openStoreFromCmd(cmd)
			if err != nil {
				return err
			}
			timeout, _ := cmd.Flags().GetDuration("timeout")
			prov, cfg := loadProviderFromStore(s)
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()
			if noRetry, _ := cmd.Flags().GetBool("no-retry"); noRetry {
				ctx = workflow.WithDisableRetry(ctx, true)
			}

			if err := workflow.RunImplement(ctx, s, args[0], prov, cfg); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Implementation recipe generated for %s\n", args[0])
			return nil
		},
	}
	cmd.Flags().Duration("timeout", 90*time.Second, "Timeout")
	cmd.Flags().Bool("no-retry", false, "Disable retry-with-feedback on invalid LLM output")
	return cmd
}

// ─── apply ───────────────────────────────────────────────────────────────────

func applyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "apply <slug>",
		Short: "Execute apply recipe or record session",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			slug := args[0]
			s, err := openStoreFromCmd(cmd)
			if err != nil {
				return err
			}
			mode, _ := cmd.Flags().GetString("mode")
			dryRun, _ := cmd.Flags().GetBool("dry-run")
			out := cmd.OutOrStdout()

			// Handle --dry-run: preview recipe operations without modifying anything
			if dryRun {
				recipe, err := workflow.LoadRecipe(s, slug)
				if err != nil {
					return err
				}
				result := workflow.DryRunRecipe(s.Root, recipe)
				fmt.Fprintf(out, "Dry-run for %s (%d operations):\n", slug, result.Operations)
				for _, msg := range result.Messages {
					fmt.Fprintf(out, "  ✓ %s\n", msg)
				}
				for _, e := range result.Errors {
					fmt.Fprintf(out, "  ✗ %s\n", e)
				}
				if result.Success {
					fmt.Fprintln(out, "All operations would succeed.")
				} else {
					fmt.Fprintf(out, "%d error(s) — recipe would fail.\n", len(result.Errors))
				}
				return nil
			}

			switch mode {
			case "prepare":
				request, _ := s.ReadFeatureFile(slug, "request.md")
				spec, _ := s.ReadFeatureFile(slug, "spec.md")
				exploration, _ := s.ReadFeatureFile(slug, "exploration.md")
				packet := fmt.Sprintf("# Apply Packet: %s\n\n## Request\n%s\n\n## Spec\n%s\n\n## Exploration\n%s\n",
					slug, request, spec, exploration)
				if err := s.WriteArtifact(slug, "apply-packet.md", packet); err != nil {
					return err
				}
				if err := s.MarkFeatureState(slug, store.StateImplementing, "apply --mode prepare", "Agent packet ready"); err != nil {
					return err
				}
				fmt.Fprintf(out, "Apply packet prepared for %s\n", slug)

			case "started":
				if err := s.MarkFeatureState(slug, store.StateImplementing, "apply --mode started", "Implementation in progress"); err != nil {
					return err
				}
				fmt.Fprintf(out, "Feature %s marked as implementing\n", slug)

			case "execute":
				// GAP 8: Execute the recipe with path safety checks
				recipe, err := workflow.LoadRecipe(s, slug)
				if err != nil {
					return err
				}
				if err := s.MarkFeatureState(slug, store.StateImplementing, "apply --mode execute", "Executing recipe"); err != nil {
					return err
				}
				result := workflow.ExecuteRecipe(s.Root, recipe)
				for _, msg := range result.Messages {
					fmt.Fprintf(out, "  %s\n", msg)
				}
				for _, e := range result.Errors {
					fmt.Fprintf(cmd.ErrOrStderr(), "  ERROR: %s\n", e)
				}
				if result.Success {
					fmt.Fprintf(out, "Recipe executed: %d/%d operations succeeded\n", result.Applied, result.Operations)
				} else {
					return fmt.Errorf("recipe execution failed: %d error(s)", len(result.Errors))
				}

			case "done":
				note, _ := cmd.Flags().GetString("note")
				valStatus, _ := cmd.Flags().GetString("validation-status")
				valNote, _ := cmd.Flags().GetString("validation-note")

				patch, patchErr := gitutil.CapturePatch(s.Root)
				if patchErr != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "warning: could not capture patch: %v\n", patchErr)
				}
				if patch != "" {
					s.WriteArtifact(slug, "post-apply.patch", patch)
					// GAP 7: Also write sequential patch
					patchName, _ := s.WritePatch(slug, "apply", patch)
					if patchName != "" {
						fmt.Fprintf(out, "  Saved patch: patches/%s\n", patchName)
					}
				}
				diffStat, _ := gitutil.CaptureDiffStat(s.Root)
				if diffStat != "" {
					s.WriteArtifact(slug, "post-apply-diff.txt", diffStat)
				}

				now := time.Now().UTC().Format(time.RFC3339)
				commit, _ := gitutil.HeadCommit(s.Root)
				status, _ := s.LoadFeatureStatus(slug)
				status.Apply.BaseCommit = commit
				status.Apply.CompletedAt = now
				status.Apply.HasPatch = patch != ""
				s.SaveFeatureStatus(status)

				// GAP 2: Write apply-session.json
				session := store.ApplySession{
					Slug:             slug,
					PreparedAt:       status.Apply.PreparedAt,
					StartedAt:        status.Apply.StartedAt,
					CompletedAt:      now,
					BaseCommit:       commit,
					HasPatch:         patch != "",
					OperatorNotes:    note,
					ValidationStatus: valStatus,
					ValidationNotes:  valNote,
				}
				s.SaveApplySession(slug, session)

				// GAP 5: Write manual-validation.md if validation notes provided
				if valNote != "" || valStatus != "" {
					vs := valStatus
					if vs == "" {
						vs = "pending"
					}
					validationMD := fmt.Sprintf("# Manual Validation\n\n**Status**: %s\n**Timestamp**: %s\n\n## Notes\n\n%s\n", vs, now, valNote)
					s.WriteArtifact(slug, "manual-validation.md", validationMD)
				}

				if err := s.MarkFeatureState(slug, store.StateApplied, "apply --mode done", "Changes applied and recorded"); err != nil {
					return err
				}
				fmt.Fprintf(out, "Feature %s marked as applied\n", slug)

			default:
				return fmt.Errorf("unknown apply mode %q (valid: prepare, started, execute, done)", mode)
			}
			return nil
		},
	}
	cmd.Flags().String("mode", "prepare", "Apply mode: prepare, started, execute, done")
	cmd.Flags().Bool("dry-run", false, "Preview recipe execution without modifying files")
	cmd.Flags().String("note", "", "Operator notes about the apply session")
	cmd.Flags().String("validation-status", "", "Validation outcome: passed, failed, needs_review")
	cmd.Flags().String("validation-note", "", "Details about validation")
	return cmd
}

// ─── record ──────────────────────────────────────────────────────────────────

func recordCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "record <slug>",
		Short: "Capture patches (tracked + untracked files)",
		Long:  "Capture the current diff as a patch. If --from is specified, captures the diff between that commit and HEAD instead of the working tree.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			slug := args[0]
			s, err := openStoreFromCmd(cmd)
			if err != nil {
				return err
			}

			fromRef, _ := cmd.Flags().GetString("from")
			var patch string
			if fromRef != "" {
				patch, err = gitutil.CapturePatchFromCommits(s.Root, fromRef, "HEAD")
			} else {
				patch, err = gitutil.CapturePatch(s.Root)
			}
			if err != nil {
				return fmt.Errorf("cannot capture patch: %w", err)
			}
			if patch == "" {
				fmt.Fprintln(cmd.OutOrStdout(), "No changes to record")
				return nil
			}

			// Write post-apply.patch (backwards compat) + sequential patch (GAP 7)
			if err := s.WriteArtifact(slug, "post-apply.patch", patch); err != nil {
				return err
			}
			patchName, _ := s.WritePatch(slug, "record", patch)
			if patchName != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "  Saved patch: patches/%s\n", patchName)
			}

			// Automated patch validation
			cfg, _ := s.LoadConfig()
			strategy := cfg.MergeStrategy
			if strategy == "" {
				strategy = "3way"
			}
			if valErr := gitutil.ValidatePatch(s.Root, patch, strategy); valErr != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: patch validation failed: %v\n", valErr)
				fmt.Fprintf(cmd.ErrOrStderr(), "  The recorded patch may not apply cleanly during reconciliation.\n")
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "  Patch validated: applies cleanly\n")
			}

			diffStat, _ := gitutil.CaptureDiffStat(s.Root)
			if diffStat != "" {
				s.WriteArtifact(slug, "post-apply-diff.txt", diffStat)
			}

			// GAP 3: Generate record.md
			filesChanged := countPatchFiles(patch)
			recordMD := generateRecordMD(slug, filesChanged, len(patch), diffStat, fromRef)
			s.WriteFeatureFile(slug, "record.md", recordMD)

			status, _ := s.LoadFeatureStatus(slug)
			status.Apply.HasPatch = true
			commit, _ := gitutil.HeadCommit(s.Root)
			if commit != "" {
				status.Apply.BaseCommit = commit
			}
			s.SaveFeatureStatus(status)

			if err := s.MarkFeatureState(slug, store.StateApplied, "record", "Patch recorded"); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Recorded patch for %s (%d bytes, %d files)\n", slug, len(patch), filesChanged)
			return nil
		},
	}
	cmd.Flags().String("from", "", "Base commit to diff from (captures committed diff instead of working tree)")
	return cmd
}

func countPatchFiles(patch string) int {
	count := 0
	for _, line := range strings.Split(patch, "\n") {
		if strings.HasPrefix(line, "diff --git") {
			count++
		}
	}
	return count
}

func generateRecordMD(slug string, filesChanged, patchBytes int, diffStat, fromRef string) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("# Implementation Record: %s\n\n", slug))
	b.WriteString(fmt.Sprintf("**Recorded**: %s\n", time.Now().UTC().Format(time.RFC3339)))
	b.WriteString(fmt.Sprintf("**Files changed**: %d\n", filesChanged))
	b.WriteString(fmt.Sprintf("**Patch size**: %d bytes\n\n", patchBytes))

	if diffStat != "" {
		b.WriteString("## Change Summary\n\n```\n")
		b.WriteString(diffStat)
		b.WriteString("```\n\n")
	}

	b.WriteString("## Replay Instructions\n\n")
	b.WriteString("To re-apply this feature to a clean checkout:\n\n")
	b.WriteString("```bash\n")
	b.WriteString("# From the feature's artifacts directory:\n")
	b.WriteString(fmt.Sprintf("git apply .tpatch/features/%s/artifacts/post-apply.patch\n", slug))
	b.WriteString("```\n\n")
	if fromRef != "" {
		b.WriteString(fmt.Sprintf("*Patch was captured as a committed diff from `%s` to `HEAD`.*\n", fromRef))
	}

	return b.String()
}

// ─── reconcile ───────────────────────────────────────────────────────────────

func reconcileCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "reconcile [slug...]",
		Short: "Reconcile features against upstream",
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := openStoreFromCmd(cmd)
			if err != nil {
				return err
			}
			upstreamRef, _ := cmd.Flags().GetString("upstream-ref")
			timeout, _ := cmd.Flags().GetDuration("timeout")
			prov, cfg := loadProviderFromStore(s)
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()

			results, err := workflow.RunReconcile(ctx, s, args, upstreamRef, prov, cfg)
			if err != nil {
				return err
			}

			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Reconciled %d feature(s) against %s\n", len(results), upstreamRef)
			for _, result := range results {
				fmt.Fprintf(out, "  - %s [%s] (%s) %s\n", result.Slug, result.Outcome, result.Phase, result.Title)
				for _, note := range result.Notes {
					fmt.Fprintf(out, "    %s\n", note)
				}
			}
			return nil
		},
	}
	cmd.Flags().String("upstream-ref", "upstream/main", "Upstream ref to reconcile against")
	cmd.Flags().Duration("timeout", 120*time.Second, "Reconciliation timeout")
	return cmd
}

// ─── provider ────────────────────────────────────────────────────────────────

func providerCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "provider",
		Short: "Manage LLM provider settings",
	}
	cmd.AddCommand(providerCheckCmd(), providerSetCmd())
	return cmd
}

func providerCheckCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "check",
		Short: "Validate provider endpoint",
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := openStoreFromCmd(cmd)
			if err != nil {
				return err
			}
			_, cfg := loadProviderFromStore(s)
			if !cfg.Configured() {
				return fmt.Errorf("provider is not configured — run 'tpatch config set provider.base_url <url>' and 'tpatch config set provider.model <model>'")
			}
			timeout, _ := cmd.Flags().GetDuration("timeout")
			prov := provider.NewFromConfig(cfg)
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()

			health, err := prov.Check(ctx, cfg)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Provider OK: %s\n", health.Endpoint)
			fmt.Fprintf(cmd.OutOrStdout(), "Models: %s\n", strings.Join(health.Models, ", "))
			return nil
		},
	}
	cmd.Flags().Duration("timeout", 15*time.Second, "Request timeout")
	return cmd
}

func providerSetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set",
		Short: "Configure provider endpoint",
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := openStoreFromCmd(cmd)
			if err != nil {
				return err
			}
			cfg, err := s.LoadConfig()
			if err != nil {
				return err
			}
			if preset, _ := cmd.Flags().GetString("preset"); preset != "" {
				p, ok := providerPresets[strings.ToLower(preset)]
				if !ok {
					return fmt.Errorf("unknown preset %q (valid: copilot, openai, openrouter, anthropic, ollama)", preset)
				}
				cfg.Provider.Type = p.Type
				cfg.Provider.BaseURL = p.BaseURL
				cfg.Provider.Model = p.Model
				cfg.Provider.AuthEnv = p.AuthEnv
			}
			if v, _ := cmd.Flags().GetString("type"); v != "" {
				if v != "openai-compatible" && v != "anthropic" {
					return fmt.Errorf("invalid provider type %q (valid: openai-compatible, anthropic)", v)
				}
				cfg.Provider.Type = v
			}
			if v, _ := cmd.Flags().GetString("base-url"); v != "" {
				cfg.Provider.BaseURL = v
			}
			if v, _ := cmd.Flags().GetString("model"); v != "" {
				cfg.Provider.Model = v
			}
			if v, _ := cmd.Flags().GetString("auth-env"); v != "" {
				cfg.Provider.AuthEnv = v
			}
			if err := s.SaveConfig(cfg); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Provider configured: type=%s url=%s model=%s\n",
				cfg.Provider.Type, cfg.Provider.BaseURL, cfg.Provider.Model)
			return nil
		},
	}
	cmd.Flags().String("preset", "", "Preset: copilot | openai | openrouter | anthropic | ollama")
	cmd.Flags().String("type", "", "Provider type: openai-compatible | anthropic")
	cmd.Flags().String("base-url", "", "Provider base URL")
	cmd.Flags().String("model", "", "Default model")
	cmd.Flags().String("auth-env", "", "Environment variable name for auth token")
	return cmd
}

// providerPresets are vetted one-line configurations for common providers.
// Each preset matches a widely used endpoint that speaks either the
// OpenAI chat completions API or the Anthropic Messages API — the two
// protocols implemented in internal/provider.
var providerPresets = map[string]struct {
	Type, BaseURL, Model, AuthEnv string
}{
	"copilot":    {"openai-compatible", "http://localhost:4141", "claude-sonnet-4", "GITHUB_TOKEN"},
	"openai":     {"openai-compatible", "https://api.openai.com", "gpt-4o", "OPENAI_API_KEY"},
	"openrouter": {"openai-compatible", "https://openrouter.ai/api", "anthropic/claude-sonnet-4", "OPENROUTER_API_KEY"},
	"anthropic":  {"anthropic", "https://api.anthropic.com", "claude-sonnet-4-5", "ANTHROPIC_API_KEY"},
	"ollama":     {"openai-compatible", "http://localhost:11434", "llama3.2", ""},
}

// ─── config ──────────────────────────────────────────────────────────────────

func configCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage configuration",
	}
	cmd.AddCommand(configShowCmd(), configSetCmd())
	return cmd
}

func configShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Display configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := openStoreFromCmd(cmd)
			if err != nil {
				return err
			}
			data, err := os.ReadFile(s.ConfigPath())
			if err != nil {
				return err
			}
			fmt.Fprint(cmd.OutOrStdout(), string(data))
			return nil
		},
	}
}

func configSetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set <key> <value>",
		Short: "Set a configuration value",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			key, value := args[0], args[1]
			s, err := openStoreFromCmd(cmd)
			if err != nil {
				return err
			}
			cfg, err := s.LoadConfig()
			if err != nil {
				return err
			}
			switch key {
			case "provider.type":
				cfg.Provider.Type = value
			case "provider.base_url":
				cfg.Provider.BaseURL = value
			case "provider.model":
				cfg.Provider.Model = value
			case "provider.auth_env":
				cfg.Provider.AuthEnv = value
			case "merge_strategy":
				if value != "3way" && value != "rebase" {
					return fmt.Errorf("invalid merge_strategy %q (valid: 3way, rebase)", value)
				}
				cfg.MergeStrategy = value
			case "max_retries":
				var n int
				if _, err := fmt.Sscanf(value, "%d", &n); err != nil || n < 0 {
					return fmt.Errorf("invalid max_retries %q (must be non-negative integer)", value)
				}
				cfg.MaxRetries = n
			case "test_command":
				cfg.TestCommand = value
			default:
				return fmt.Errorf("unknown config key %q (valid: provider.type, provider.base_url, provider.model, provider.auth_env, merge_strategy, max_retries, test_command)", key)
			}
			if err := s.SaveConfig(cfg); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Set %s = %s\n", key, value)
			return nil
		},
	}
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func installSkills(cmd *cobra.Command, root string) {
	skillInstalls := []struct {
		src, dst, name string
	}{
		{"skills/claude/tessera-patch/SKILL.md", filepath.Join(root, ".claude", "skills", "tessera-patch", "SKILL.md"), "Claude skill"},
		{"skills/copilot/tessera-patch/SKILL.md", filepath.Join(root, ".github", "skills", "tessera-patch", "SKILL.md"), "Copilot skill"},
		{"prompts/copilot/tessera-patch-apply.prompt.md", filepath.Join(root, ".github", "prompts", "tessera-patch-apply.prompt.md"), "Copilot prompt"},
		{"skills/cursor/tessera-patch.mdc", filepath.Join(root, ".cursor", "rules", "tessera-patch.mdc"), "Cursor rules"},
		{"skills/windsurf/windsurfrules", filepath.Join(root, ".windsurfrules"), "Windsurf rules"},
		{"workflows/tessera-patch-generic.md", filepath.Join(root, ".tpatch", "workflows", "tessera-patch-generic.md"), "Generic workflow"},
	}
	for _, si := range skillInstalls {
		data, err := assets.Skills.ReadFile(si.src)
		if err != nil {
			continue
		}
		os.MkdirAll(filepath.Dir(si.dst), 0o755)
		if err := os.WriteFile(si.dst, data, 0o644); err != nil {
			continue
		}
		fmt.Fprintf(cmd.OutOrStdout(), "  Installed %s\n", si.name)
	}
}

func resolveRoot(cmd *cobra.Command, args []string) (string, error) {
	pathFlag, _ := cmd.Flags().GetString("path")
	if pathFlag != "" {
		return filepath.Abs(pathFlag)
	}
	if len(args) > 0 {
		return filepath.Abs(args[0])
	}
	return filepath.Abs(".")
}

func openStoreFromCmd(cmd *cobra.Command) (*store.Store, error) {
	pathFlag, _ := cmd.Flags().GetString("path")
	start := pathFlag
	if start == "" {
		start = "."
	}
	root, err := store.FindProjectRoot(start)
	if err != nil {
		return nil, err
	}
	return store.Open(root)
}

func loadProviderFromStore(s *store.Store) (provider.Provider, provider.Config) {
	cfg, err := s.LoadConfig()
	if err != nil {
		return nil, provider.Config{}
	}
	provCfg := provider.Config{
		Type:    cfg.Provider.Type,
		BaseURL: cfg.Provider.BaseURL,
		Model:   cfg.Provider.Model,
		AuthEnv: cfg.Provider.AuthEnv,
	}
	if !provCfg.Configured() {
		return nil, provCfg
	}
	return provider.NewFromConfig(provCfg), provCfg
}

// autoDetectProvider probes known provider endpoints and auto-configures if found.
func autoDetectProvider(cmd *cobra.Command, s *store.Store) {
	// Skip auto-detection in test environments
	if os.Getenv("TPATCH_NO_AUTO_DETECT") != "" {
		return
	}

	cfg, _ := s.LoadConfig()
	if cfg.Provider.Configured() {
		return // already configured
	}

	type candidate struct {
		name   string
		preset struct{ Type, BaseURL, Model, AuthEnv string }
	}

	candidates := []candidate{
		{"copilot-api (localhost:4141)", providerPresets["copilot"]},
		{"Ollama (localhost:11434)", providerPresets["ollama"]},
	}

	// Also check env vars for direct API keys
	if os.Getenv("ANTHROPIC_API_KEY") != "" {
		candidates = append(candidates, candidate{"Anthropic (from ANTHROPIC_API_KEY)", providerPresets["anthropic"]})
	}
	if os.Getenv("OPENAI_API_KEY") != "" {
		candidates = append(candidates, candidate{"OpenAI (from OPENAI_API_KEY)", providerPresets["openai"]})
	}
	if os.Getenv("OPENROUTER_API_KEY") != "" {
		candidates = append(candidates, candidate{"OpenRouter (from OPENROUTER_API_KEY)", providerPresets["openrouter"]})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	for _, c := range candidates {
		provCfg := provider.Config{Type: c.preset.Type, BaseURL: c.preset.BaseURL, Model: c.preset.Model, AuthEnv: c.preset.AuthEnv}
		prov := provider.NewFromConfig(provCfg)
		if _, err := prov.Check(ctx, provCfg); err == nil {
			cfg.Provider.Type = c.preset.Type
			cfg.Provider.BaseURL = c.preset.BaseURL
			cfg.Provider.Model = c.preset.Model
			cfg.Provider.AuthEnv = c.preset.AuthEnv
			s.SaveConfig(cfg)
			fmt.Fprintf(cmd.OutOrStdout(), "  Auto-detected provider: %s\n", c.name)
			return
		}
	}
}
