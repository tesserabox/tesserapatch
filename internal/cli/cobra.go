package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"github.com/tesseracode/tesserapatch/assets"
	"github.com/tesseracode/tesserapatch/internal/gitutil"
	"github.com/tesseracode/tesserapatch/internal/provider"
	"github.com/tesseracode/tesserapatch/internal/store"
	"github.com/tesseracode/tesserapatch/internal/workflow"
)

const version = "0.5.1"

// Execute runs the tpatch CLI root command.
func Execute() int {
	rootCmd := buildRootCmd()
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
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
		editCmd(),
		amendCmd(),
		removeCmd(),
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

			// Post-init: run a reachability probe for local endpoints (warn-continue).
			// Per ADR-004 D4 — init must never fail because the proxy is down;
			// the user may start it later. Emit a friendly pointer instead.
			postProbeCtx, cancel := context.WithTimeout(context.Background(), provider.ProbeTimeout)
			defer cancel()
			provCfg := providerConfigFromStore(s)
			maybeShowAUPWarning(cmd.OutOrStdout(), provCfg)
			warnIfUnreachable(postProbeCtx, cmd.OutOrStdout(), provCfg)

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
		Short: "Create a tracked feature request (reads stdin when no args)",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := openStoreFromCmd(cmd)
			if err != nil {
				return err
			}
			slug, _ := cmd.Flags().GetString("slug")

			var description string
			switch {
			case len(args) > 0:
				description = strings.Join(args, " ")
			case stdinIsPiped(cmd):
				raw, err := io.ReadAll(cmd.InOrStdin())
				if err != nil {
					return fmt.Errorf("read stdin: %w", err)
				}
				description = strings.TrimSpace(string(raw))
				if description == "" {
					return fmt.Errorf("empty description on stdin")
				}
			default:
				return fmt.Errorf("provide a description as arguments or pipe via stdin")
			}

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

// stdinIsPiped reports whether the command's stdin appears to be a pipe
// or redirected file rather than an interactive terminal. Uses the
// cobra-injected reader when it's an *os.File (tests using SetIn with
// a strings.Reader return true via the type-check fallback so piped
// input is always honoured).
func stdinIsPiped(cmd *cobra.Command) bool {
	in := cmd.InOrStdin()
	f, ok := in.(*os.File)
	if !ok {
		// Tests inject strings.Reader / bytes.Buffer etc.; treat as piped.
		return true
	}
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice == 0
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
					if st.State == store.StateReconcilingShadow || st.Reconcile.ShadowPath != "" {
						fmt.Fprintf(out, "  Shadow:        %s\n", st.Reconcile.ShadowPath)
						if st.Reconcile.ResolveSession != "" {
							fmt.Fprintf(out, "  Session:       %s\n", st.Reconcile.ResolveSession)
						}
						fmt.Fprintf(out, "  Files:         %d resolved, %d failed, %d skipped\n",
							st.Reconcile.ResolvedFiles, st.Reconcile.FailedFiles, st.Reconcile.SkippedFiles)
						if st.State == store.StateReconcilingShadow {
							fmt.Fprintf(out, "  Next:          tpatch reconcile --shadow-diff %s  |  --accept %s  |  --reject %s\n",
								st.Slug, st.Slug, st.Slug)
						}
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
			if isManualFlag(cmd) {
				return runManualPhase(cmd, s, args[0], "analyze")
			}
			timeout, _ := cmd.Flags().GetDuration("timeout")
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()
			prov, cfg, perr := loadAndProbeProvider(ctx, s)
			if perr != nil {
				return perr
			}
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
	addManualFlag(cmd)
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
			if isManualFlag(cmd) {
				return runManualPhase(cmd, s, args[0], "define")
			}
			timeout, _ := cmd.Flags().GetDuration("timeout")
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()
			prov, cfg, perr := loadAndProbeProvider(ctx, s)
			if perr != nil {
				return perr
			}
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
	addManualFlag(cmd)
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
			if isManualFlag(cmd) {
				return runManualPhase(cmd, s, args[0], "explore")
			}
			timeout, _ := cmd.Flags().GetDuration("timeout")
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()
			prov, cfg, perr := loadAndProbeProvider(ctx, s)
			if perr != nil {
				return perr
			}
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
	addManualFlag(cmd)
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
			if isManualFlag(cmd) {
				return runManualPhase(cmd, s, args[0], "implement")
			}
			timeout, _ := cmd.Flags().GetDuration("timeout")
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()
			prov, cfg, perr := loadAndProbeProvider(ctx, s)
			if perr != nil {
				return perr
			}
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
	addManualFlag(cmd)
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
				return runApplyPrepare(cmd, s, slug)
			case "started":
				return runApplyStarted(cmd, s, slug)
			case "execute":
				_, err := runApplyExecute(cmd, s, slug)
				return err
			case "done":
				_, _, err := runApplyDone(cmd, s, slug)
				return err
			case "auto":
				return runApplyAuto(cmd, s, slug)
			default:
				return fmt.Errorf("unknown apply mode %q (valid: auto, prepare, started, execute, done)", mode)
			}
		},
	}
	cmd.Flags().String("mode", "auto", "Apply mode: auto (default, runs prepare→execute→done), prepare, started, execute, done")
	cmd.Flags().Bool("dry-run", false, "Preview recipe execution without modifying files")
	cmd.Flags().String("note", "", "Operator notes about the apply session")
	cmd.Flags().String("validation-status", "", "Validation outcome: passed, failed, needs_review")
	cmd.Flags().String("validation-note", "", "Details about validation")
	return cmd
}

func runApplyPrepare(cmd *cobra.Command, s *store.Store, slug string) error {
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
	fmt.Fprintf(cmd.OutOrStdout(), "Apply packet prepared for %s\n", slug)
	return nil
}

func runApplyStarted(cmd *cobra.Command, s *store.Store, slug string) error {
	if err := s.MarkFeatureState(slug, store.StateImplementing, "apply --mode started", "Implementation in progress"); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Feature %s marked as implementing\n", slug)
	return nil
}

// runApplyExecute loads the recipe, warns on stale provenance, runs the
// operations, and returns the execution result. The result is returned
// so auto-mode can roll it into a final summary without re-running.
func runApplyExecute(cmd *cobra.Command, s *store.Store, slug string) (workflow.RecipeExecResult, error) {
	out := cmd.OutOrStdout()
	recipe, err := workflow.LoadRecipe(s, slug)
	if err != nil {
		return workflow.RecipeExecResult{}, err
	}
	warnRecipeStale(cmd.ErrOrStderr(), s, slug)
	if err := s.MarkFeatureState(slug, store.StateImplementing, "apply --mode execute", "Executing recipe"); err != nil {
		return workflow.RecipeExecResult{}, err
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
		return result, nil
	}
	return result, fmt.Errorf("recipe execution failed: %d error(s)", len(result.Errors))
}

// runApplyDone captures the post-apply patch, writes apply-session.json,
// and advances the feature to state=applied. Returns the patch string
// and its byte-length so auto-mode can summarise without re-reading.
func runApplyDone(cmd *cobra.Command, s *store.Store, slug string) (patch string, patchBytes int, err error) {
	out := cmd.OutOrStdout()
	note, _ := cmd.Flags().GetString("note")
	valStatus, _ := cmd.Flags().GetString("validation-status")
	valNote, _ := cmd.Flags().GetString("validation-note")

	patch, patchErr := gitutil.CapturePatch(s.Root)
	if patchErr != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: could not capture patch: %v\n", patchErr)
	}
	if patch != "" {
		s.WriteArtifact(slug, "post-apply.patch", patch)
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

	if valNote != "" || valStatus != "" {
		vs := valStatus
		if vs == "" {
			vs = "pending"
		}
		validationMD := fmt.Sprintf("# Manual Validation\n\n**Status**: %s\n**Timestamp**: %s\n\n## Notes\n\n%s\n", vs, now, valNote)
		s.WriteArtifact(slug, "manual-validation.md", validationMD)
	}

	if err := s.MarkFeatureState(slug, store.StateApplied, "apply --mode done", "Changes applied and recorded"); err != nil {
		return patch, len(patch), err
	}
	fmt.Fprintf(out, "Feature %s marked as applied\n", slug)
	return patch, len(patch), nil
}

// runApplyAuto chains prepare → execute → done in one shot. Stops on the
// first error; surfaces it as-is. On success, prints a consolidated
// summary naming each phase.
func runApplyAuto(cmd *cobra.Command, s *store.Store, slug string) error {
	if err := runApplyPrepare(cmd, s, slug); err != nil {
		return err
	}
	execResult, err := runApplyExecute(cmd, s, slug)
	if err != nil {
		return err
	}
	_, patchBytes, err := runApplyDone(cmd, s, slug)
	if err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(),
		"Feature %s: prepared → executed → recorded (%d ops, %d bytes patch)\n",
		slug, execResult.Operations, patchBytes)
	return nil
}

// warnRecipeStale prints a stderr warning when the recipe-provenance
// sidecar indicates the recipe was generated at a commit different
// from current HEAD. No-op when the sidecar is missing (old recipes)
// or HEAD is unreadable — this keeps the guard backward-compatible.
func warnRecipeStale(w io.Writer, s *store.Store, slug string) {
	raw, err := s.ReadFeatureFile(slug, filepath.Join("artifacts", "recipe-provenance.json"))
	if err != nil || strings.TrimSpace(raw) == "" {
		return
	}
	var prov workflow.RecipeProvenance
	if err := json.Unmarshal([]byte(raw), &prov); err != nil || prov.BaseCommit == "" {
		return
	}
	head, err := gitutil.HeadCommit(s.Root)
	if err != nil || head == "" || head == prov.BaseCommit {
		return
	}
	short := func(sha string) string {
		if len(sha) > 7 {
			return sha[:7]
		}
		return sha
	}
	fmt.Fprintf(w, "warning: recipe was generated at commit %s but HEAD is now %s — results may differ\n",
		short(prov.BaseCommit), short(head))
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
				// bug / footgun A8 doc-record-timing: previously we
				// wrote a no-op patch and reported success, letting
				// the feature advance to state=applied with zero
				// recorded bytes. The common cause is "user committed
				// their edits before running record"; working tree is
				// then clean and CapturePatch (diff HEAD) returns "".
				// Refuse the empty capture and surface --from candidates.
				if fromRef != "" {
					// User explicitly chose --from <ref>..HEAD and it
					// produced no diff. That is a legitimate "nothing
					// changed in that range" — keep the old success
					// semantic so harness scripts are not broken.
					fmt.Fprintln(cmd.OutOrStdout(), "No changes to record in the specified range")
					return nil
				}
				w := cmd.ErrOrStderr()
				fmt.Fprintln(w, "tpatch record captured 0 bytes — nothing unstaged or untracked in the working tree.")
				if gitutil.IsWorkingTreeDirty(s.Root) {
					fmt.Fprintln(w, "  (working tree is dirty, but no textual diff was produced — possibly mode-only or binary changes)")
				} else {
					fmt.Fprintln(w, "  If you already committed your feature edits, rerun with --from <base>:")
					fmt.Fprintln(w, "    tpatch record "+slug+" --from <base-commit-or-ref>")
					commits := gitutil.RecentCommits(s.Root, 10)
					if len(commits) > 0 {
						fmt.Fprintln(w, "  Recent commits on this branch (candidates for --from base):")
						for _, c := range commits {
							fmt.Fprintf(w, "    %s  %s  %s\n", c.SHA, c.When, c.Subject)
						}
					}
				}
				return fmt.Errorf("empty capture — see diagnostic above")
			}

			// Write post-apply.patch (backwards compat) + sequential patch (GAP 7)
			if err := s.WriteArtifact(slug, "post-apply.patch", patch); err != nil {
				return err
			}
			patchName, _ := s.WritePatch(slug, "record", patch)
			if patchName != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "  Saved patch: patches/%s\n", patchName)
				// A9 doc-patches-vs-artifacts: patches/ is append-only
				// audit trail; surface a one-liner once the directory
				// gets crowded so users know cleanup is an option (not
				// a silent footgun). NextPatchNumber is cheap (ReadDir).
				if nextN := s.NextPatchNumber(slug); nextN > 6 {
					fmt.Fprintf(cmd.OutOrStdout(),
						"  note: %d patches accumulated under .tpatch/features/%s/patches/ — patches/NNN-*.patch is historical audit only; for replay use artifacts/post-apply.patch.\n",
						nextN-1, slug)
				}
			}

			// Automated patch validation. At record-time the working
			// tree already contains the patch, so a forward `git apply
			// --check` would always fail (cannot apply something that
			// is already present). The correct semantic here is
			// reverse-apply: prove the recorded patch round-trips
			// against the tree we just captured it from. Forward
			// validation against an upstream baseline happens at
			// reconcile-time, not here.
			lenient, _ := cmd.Flags().GetBool("lenient")
			if lenient {
				fmt.Fprintln(cmd.ErrOrStderr(), "warning: --lenient: skipping patch round-trip validation")
			} else if valErr := gitutil.ValidatePatchReverse(s.Root, patch); valErr != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %v\n", valErr)
				fmt.Fprintf(cmd.ErrOrStderr(), "  The recorded patch may not represent the on-disk changes accurately.\n")
				fmt.Fprintf(cmd.ErrOrStderr(), "  Common causes: line-ending differences, binary files without --binary, or post-apply edits.\n")
				fmt.Fprintf(cmd.ErrOrStderr(), "  To silence this check (e.g. for whitespace-sensitive markdown), rerun with --lenient.\n")
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "  Patch validated: round-trips cleanly against working tree\n")
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
	cmd.Flags().Bool("lenient", false, "Skip reverse-apply round-trip validation (use for whitespace-sensitive files)")
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
			// Phase-3.5 (M12) terminal operations. These act on the
			// shadow state left behind by a prior `reconcile --resolve`
			// and do NOT re-run the reconcile pipeline. They are
			// mutually exclusive with each other and with --resolve.
			acceptSlug, _ := cmd.Flags().GetString("accept")
			rejectSlug, _ := cmd.Flags().GetString("reject")
			shadowDiffSlug, _ := cmd.Flags().GetString("shadow-diff")
			resolve, _ := cmd.Flags().GetBool("resolve")
			apply, _ := cmd.Flags().GetBool("apply")
			maxConflicts, _ := cmd.Flags().GetInt("max-conflicts")
			modelOverride, _ := cmd.Flags().GetString("model")

			if err := validateReconcileFlags(acceptSlug, rejectSlug, shadowDiffSlug, resolve, apply); err != nil {
				return err
			}

			s, err := openStoreFromCmd(cmd)
			if err != nil {
				return err
			}

			if acceptSlug != "" {
				return runReconcileAccept(cmd, s, acceptSlug)
			}
			if rejectSlug != "" {
				return runReconcileReject(cmd, s, rejectSlug)
			}
			if shadowDiffSlug != "" {
				return runReconcileShadowDiff(cmd, s, shadowDiffSlug)
			}

			// A10 doc-reconcile-workflow: hard-refuse dirty trees /
			// lingering conflict markers. See docs/reconcile.md for
			// the rationale — silent corruption beats loud failure.
			allowDirty, _ := cmd.Flags().GetBool("allow-dirty")
			preflight, pfErr := gitutil.PreflightReconcile(s.Root)
			if pfErr != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: reconcile preflight failed: %v\n", pfErr)
			} else if !preflight.Clean() {
				printReconcilePreflight(cmd.ErrOrStderr(), preflight, allowDirty)
				if !allowDirty {
					return fmt.Errorf("reconcile refused — see preflight diagnostic above")
				}
			}
			preflightOnly, _ := cmd.Flags().GetBool("preflight")
			if preflightOnly {
				if preflight.Clean() {
					fmt.Fprintln(cmd.OutOrStdout(), "Preflight: clean. Reconcile is safe to run.")
				}
				return nil
			}

			upstreamRef, _ := cmd.Flags().GetString("upstream-ref")
			timeout, _ := cmd.Flags().GetDuration("timeout")
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()
			prov, cfg, perr := loadAndProbeProvider(ctx, s)
			if perr != nil {
				return perr
			}

			opts := workflow.ReconcileOptions{
				Resolve:      resolve,
				Apply:        apply,
				Model:        modelOverride,
				MaxConflicts: maxConflicts,
			}
			results, err := workflow.RunReconcile(ctx, s, args, upstreamRef, prov, cfg, opts)
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
				if result.ShadowPath != "" {
					fmt.Fprintf(out, "    shadow:   %s\n", result.ShadowPath)
					fmt.Fprintf(out, "    files:    %d resolved, %d failed, %d skipped\n",
						len(result.ResolvedFiles), len(result.FailedFiles), len(result.SkippedFiles))
					if result.Outcome == store.ReconcileShadowAwaiting {
						fmt.Fprintf(out, "    next:     tpatch reconcile --shadow-diff %s  |  --accept %s  |  --reject %s\n",
							result.Slug, result.Slug, result.Slug)
					}
				}
			}

			// Tip: if .tpatch/ is untracked the user's feature state will
			// not travel with their branch. Cheap to check post-run.
			if isTpatchUntracked(s.Root) {
				fmt.Fprintln(out, "tip: .tpatch/ is not tracked; consider `git add .tpatch/` so feature state travels with your branch.")
			}
			return nil
		},
	}
	cmd.Flags().String("upstream-ref", "upstream/main", "Upstream ref to reconcile against")
	cmd.Flags().Duration("timeout", 120*time.Second, "Reconciliation timeout")
	cmd.Flags().Bool("preflight", false, "Only run the preflight checks and exit (does not reconcile)")
	cmd.Flags().Bool("allow-dirty", false, "Bypass the clean-tree requirement (verdicts may be wrong — not recommended)")
	// M12 / ADR-010 phase-3.5 flags.
	cmd.Flags().Bool("resolve", false, "Enable provider-assisted conflict resolution (phase 3.5) on 3-way conflicts")
	cmd.Flags().Bool("apply", false, "With --resolve, auto-copy the shadow worktree onto the real tree on full success (skips human review)")
	cmd.Flags().Int("max-conflicts", 0, "With --resolve, cap the number of conflicted files per feature (0 = workflow default)")
	cmd.Flags().String("model", "", "With --resolve, override the provider model for phase-3.5 calls only")
	cmd.Flags().String("accept", "", "Accept a shadow worktree: copy resolved files onto the real tree and transition state → applied")
	cmd.Flags().String("reject", "", "Reject a shadow worktree: prune it and roll feature state back to applied")
	cmd.Flags().String("shadow-diff", "", "Emit a unified diff between shadow and real tree for a feature (review without accepting)")
	return cmd
}

// validateReconcileFlags refuses nonsensical combinations of the phase-3.5
// flags. Terminal operations (--accept/--reject/--shadow-diff) are
// mutually exclusive with each other and with the pipeline flags
// (--resolve/--apply).
func validateReconcileFlags(accept, reject, shadowDiff string, resolve, apply bool) error {
	terminals := 0
	if accept != "" {
		terminals++
	}
	if reject != "" {
		terminals++
	}
	if shadowDiff != "" {
		terminals++
	}
	if terminals > 1 {
		return fmt.Errorf("reconcile: --accept, --reject, --shadow-diff are mutually exclusive")
	}
	if terminals == 1 && (resolve || apply) {
		return fmt.Errorf("reconcile: --accept/--reject/--shadow-diff cannot be combined with --resolve/--apply")
	}
	if apply && !resolve {
		return fmt.Errorf("reconcile: --apply requires --resolve")
	}
	return nil
}

// runReconcileAccept applies the feature's non-conflicted hunks to the
// real tree, copies the resolved conflicted files from the shadow
// worktree on top, regenerates derived artifacts, and transitions the
// feature state to `applied`. Refuses if the feature is not in
// `reconciling-shadow` state.
//
// Order of operations matters for correctness:
//  1. Read post-apply.patch (needed to know which files are involved
//     and to re-apply non-conflicted hunks).
//  2. `git apply --3way --exclude=<resolved>` on post-apply.patch so
//     non-conflicted hunks land cleanly while resolved paths are left
//     for step 3.
//  3. CopyShadowToReal to overwrite resolved paths with shadow content.
//  4. RefreshAfterAccept regenerates post-apply.patch + numbered
//     patches/NNN-reconcile.patch.
//  5. MarkFeatureState → applied; prune shadow; clear status pointer.
func runReconcileAccept(cmd *cobra.Command, s *store.Store, slug string) error {
	st, err := s.LoadFeatureStatus(slug)
	if err != nil {
		return fmt.Errorf("reconcile --accept: load status: %w", err)
	}
	if st.State != store.StateReconcilingShadow {
		return fmt.Errorf("reconcile --accept: feature %q is in state %q; --accept only valid in %q",
			slug, st.State, store.StateReconcilingShadow)
	}

	files, err := loadResolvedFiles(s, slug)
	if err != nil {
		return fmt.Errorf("reconcile --accept: %w", err)
	}
	if len(files) == 0 {
		return fmt.Errorf("reconcile --accept: no resolved files recorded for %q (reconcile-session.json missing or empty)", slug)
	}

	// Pull the original post-apply.patch before we touch anything so
	// step 4 can restrict the regenerated diff to the same paths.
	originalPatch, err := s.ReadFeatureFile(slug, "artifacts/post-apply.patch")
	if err != nil {
		return fmt.Errorf("reconcile --accept: read post-apply.patch: %w", err)
	}

	// Step 2: apply non-conflicted hunks. Resolved paths are excluded
	// so git never produces conflict markers for them.
	if originalPatch != "" {
		if err := gitutil.ForwardApplyExcluding(s.Root, originalPatch, files); err != nil {
			return fmt.Errorf("reconcile --accept: apply non-conflicted hunks: %w", err)
		}
	}

	// Step 3: overlay resolved content.
	if err := gitutil.CopyShadowToReal(s.Root, slug, files); err != nil {
		return fmt.Errorf("reconcile --accept: copy shadow → real: %w", err)
	}

	// Step 4: refresh derived artifacts. Non-fatal — staleness here
	// does not corrupt the working tree; users can rerun `tpatch record`.
	if st.Reconcile.UpstreamCommit != "" {
		if rerr := workflow.RefreshAfterAccept(s, slug, st.Reconcile.UpstreamCommit, originalPatch); rerr != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "warning: reconcile --accept: derived-artifact refresh failed: %v\n", rerr)
			fmt.Fprintln(cmd.ErrOrStderr(), "  (accepted files are on disk; run `tpatch record` to refresh manually)")
		}
	} else {
		fmt.Fprintln(cmd.ErrOrStderr(), "warning: reconcile --accept: no upstream commit recorded; skipping derived-artifact refresh")
		fmt.Fprintln(cmd.ErrOrStderr(), "  (run `tpatch record` to refresh manually)")
	}

	notes := fmt.Sprintf("reconcile --accept: %d file(s) accepted from shadow; derived artifacts refreshed", len(files))
	if err := s.MarkFeatureState(slug, store.StateApplied, "reconcile --accept", notes); err != nil {
		return fmt.Errorf("reconcile --accept: mark state: %w", err)
	}

	// Prune the shadow after a successful accept so the audit trail
	// ends cleanly. The session JSON stays as the historical record.
	if err := gitutil.PruneShadow(s.Root, slug); err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: reconcile --accept: prune shadow: %v\n", err)
	}

	// Clear the shadow pointer from status.json; session id stays as
	// the audit record.
	if freshErr := clearShadowPointer(s, slug); freshErr != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: reconcile --accept: clear shadow pointer: %v\n", freshErr)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Accepted %d file(s) for %s; state → applied\n", len(files), slug)
	return nil
}

// runReconcileReject discards the shadow worktree for a feature and
// rolls state back to `applied` (the pre-reconcile state). Safe to run
// from any state that has a shadow registered — the shadow is always
// off-to-the-side, never on the real tree.
func runReconcileReject(cmd *cobra.Command, s *store.Store, slug string) error {
	st, err := s.LoadFeatureStatus(slug)
	if err != nil {
		return fmt.Errorf("reconcile --reject: load status: %w", err)
	}
	if st.Reconcile.ShadowPath == "" {
		return fmt.Errorf("reconcile --reject: no shadow recorded for %q", slug)
	}

	if err := gitutil.PruneShadow(s.Root, slug); err != nil {
		return fmt.Errorf("reconcile --reject: prune shadow: %w", err)
	}
	if err := clearShadowPointer(s, slug); err != nil {
		return fmt.Errorf("reconcile --reject: clear shadow pointer: %w", err)
	}

	// Only roll the state back if we were parked in reconciling-shadow.
	// Other states (applied, active) mean the user ran --reject as a
	// cleanup after having already manually accepted or abandoned; do
	// not clobber those.
	if st.State == store.StateReconcilingShadow {
		if err := s.MarkFeatureState(slug, store.StateApplied, "reconcile --reject", "shadow rejected; rolled back to applied"); err != nil {
			return fmt.Errorf("reconcile --reject: mark state: %w", err)
		}
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Rejected shadow for %s; state → applied\n", slug)
	return nil
}

// runReconcileShadowDiff streams a unified diff of the shadow's
// resolved files vs the real tree to stdout. Non-destructive.
func runReconcileShadowDiff(cmd *cobra.Command, s *store.Store, slug string) error {
	st, err := s.LoadFeatureStatus(slug)
	if err != nil {
		return fmt.Errorf("reconcile --shadow-diff: load status: %w", err)
	}
	if st.Reconcile.ShadowPath == "" {
		return fmt.Errorf("reconcile --shadow-diff: no shadow recorded for %q", slug)
	}

	files, err := loadResolvedFiles(s, slug)
	if err != nil {
		return fmt.Errorf("reconcile --shadow-diff: %w", err)
	}
	if len(files) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "(no resolved files recorded in reconcile-session.json)")
		return nil
	}

	diff, err := gitutil.ShadowDiff(s.Root, slug, files)
	if err != nil {
		return fmt.Errorf("reconcile --shadow-diff: %w", err)
	}
	if diff == "" {
		fmt.Fprintln(cmd.OutOrStdout(), "(shadow matches real tree — nothing to review)")
		return nil
	}
	fmt.Fprint(cmd.OutOrStdout(), diff)
	return nil
}

// loadResolvedFiles reads reconcile-session.json and returns the list
// of files whose resolver status is `resolved` (i.e., worth copying).
// Skipped and failed files are intentionally excluded.
func loadResolvedFiles(s *store.Store, slug string) ([]string, error) {
	raw, err := s.ReadFeatureFile(slug, filepath.Join("artifacts", "reconcile-session.json"))
	if err != nil {
		return nil, fmt.Errorf("read reconcile-session.json: %w", err)
	}
	var session struct {
		Outcomes []struct {
			Path   string `json:"path"`
			Status string `json:"status"`
		} `json:"outcomes"`
	}
	if err := json.Unmarshal([]byte(raw), &session); err != nil {
		return nil, fmt.Errorf("parse reconcile-session.json: %w", err)
	}
	var files []string
	for _, o := range session.Outcomes {
		if o.Status == workflow.FileStatusResolved {
			files = append(files, o.Path)
		}
	}
	return files, nil
}

// clearShadowPointer resets the phase-3.5 bookkeeping fields on the
// feature's Reconcile summary. The ResolveSession id is preserved as
// an audit record.
func clearShadowPointer(s *store.Store, slug string) error {
	st, err := s.LoadFeatureStatus(slug)
	if err != nil {
		return err
	}
	st.Reconcile.ShadowPath = ""
	st.Reconcile.ResolvedFiles = 0
	st.Reconcile.FailedFiles = 0
	st.Reconcile.SkippedFiles = 0
	return s.SaveFeatureStatus(st)
}

// printReconcilePreflight renders a user-facing diagnostic from the
// preflight report. Mirrors the error-message template from the A10
// todo so the remediation is reachable without leaving the terminal.
func printReconcilePreflight(w io.Writer, p gitutil.ReconcilePreflight, allowDirty bool) {
	if allowDirty {
		fmt.Fprintln(w, "warning: --allow-dirty set; reconcile will proceed against an unclean tree.")
	} else {
		fmt.Fprintln(w, "error: reconcile requires a clean working tree. Detected:")
	}
	for _, line := range p.UnstagedFiles {
		fmt.Fprintf(w, "  modified:         %s\n", line)
	}
	for _, f := range p.UntrackedFiles {
		fmt.Fprintf(w, "  untracked:        %s\n", f)
	}
	for _, f := range p.MergeMarkerFiles {
		fmt.Fprintf(w, "  merge markers:    %s\n", f)
	}
	for _, f := range p.LeftoverFiles {
		fmt.Fprintf(w, "  merge leftover:   %s\n", f)
	}
	if allowDirty {
		return
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "To recover:")
	fmt.Fprintln(w, "  - If these changes belong to an active feature, commit them first.")
	fmt.Fprintln(w, "  - If they are a half-applied merge or stash, resolve or abort first:")
	fmt.Fprintln(w, "      git merge --abort         (if mid-merge)")
	fmt.Fprintln(w, "      git reset --hard HEAD     (to discard — destructive!)")
	fmt.Fprintln(w, "      git stash                 (to set aside)")
	fmt.Fprintln(w, "  - If you understand the risks and want to proceed anyway, pass")
	fmt.Fprintln(w, "    `--allow-dirty` (not recommended; verdicts may be wrong).")
	fmt.Fprintln(w, "  - See docs/reconcile.md for the full workflow patterns.")
}

// isTpatchUntracked reports whether the `.tpatch/` directory is not
// tracked in the repo. Used as a non-fatal tip at the end of reconcile.
func isTpatchUntracked(repoRoot string) bool {
	return !gitutil.IsPathTracked(repoRoot, ".tpatch")
}

// ─── --manual / --skip-llm shared helpers ────────────────────────────────────

// addManualFlag installs the --manual flag and the --skip-llm alias on a
// phase command. Both flags are boolean and hidden behind the same state in
// cobra's flag set; tests and callers read --manual.
func addManualFlag(cmd *cobra.Command) {
	cmd.Flags().Bool("manual", false, "Advance feature state from a hand-authored artifact; do not call the provider")
	cmd.Flags().Bool("skip-llm", false, "Alias for --manual")
}

// isManualFlag returns true if either --manual or --skip-llm is set.
func isManualFlag(cmd *cobra.Command) bool {
	m, _ := cmd.Flags().GetBool("manual")
	if m {
		return true
	}
	alias, _ := cmd.Flags().GetBool("skip-llm")
	return alias
}

// runManualPhase validates the expected phase artifact exists and advances
// feature state without invoking the provider. It is the single entry point
// shared by analyze/define/explore/implement when --manual is set.
func runManualPhase(cmd *cobra.Command, s *store.Store, slug, phase string) error {
	if err := s.AdvanceStateManually(slug, phase); err != nil {
		return err
	}
	m, _ := store.ManualPhase(phase)
	fmt.Fprintf(cmd.OutOrStdout(), "Phase %s advanced manually for %s (artifact: %s; state: %s)\n", phase, slug, m.Path, m.State)
	fmt.Fprintln(cmd.OutOrStdout(), "  (manual mode — provider not called)")
	return nil
}

// ─── provider ────────────────────────────────────────────────────────────────

func providerCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "provider",
		Short: "Manage LLM provider settings",
	}
	cmd.AddCommand(
		providerCheckCmd(),
		providerSetCmd(),
		providerCopilotLoginCmd(),
		providerCopilotLogoutCmd(),
	)
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
		Short: "Configure provider endpoint (global by default; --repo to override per-repo)",
		RunE: func(cmd *cobra.Command, args []string) error {
			repoScope, _ := cmd.Flags().GetBool("repo")

			// Load whichever config we are targeting so preset/flag merges
			// layer onto the existing values (same UX as before the global
			// default). Repo mode: require .tpatch. Global mode: load from
			// disk or start empty.
			var (
				cfg    store.Config
				s      *store.Store
				target string
			)
			if repoScope {
				var err error
				s, err = openStoreFromCmd(cmd)
				if err != nil {
					return err
				}
				cfg, err = s.LoadConfig()
				if err != nil {
					return err
				}
				target = "repo (" + s.ConfigPath() + ")"
			} else {
				var err error
				cfg, err = store.LoadGlobalConfig()
				if err != nil {
					return err
				}
				path, err := store.GlobalConfigPath()
				if err != nil {
					return err
				}
				target = "global (" + path + ")"
			}

			if preset, _ := cmd.Flags().GetString("preset"); preset != "" {
				p, ok := providerPresets[strings.ToLower(preset)]
				if !ok {
					return fmt.Errorf("unknown preset %q (valid: copilot, copilot-native, openai, openrouter, anthropic, ollama)", preset)
				}
				cfg.Provider.Type = p.Type
				cfg.Provider.BaseURL = p.BaseURL
				cfg.Provider.Model = p.Model
				cfg.Provider.AuthEnv = p.AuthEnv
			}
			if v, _ := cmd.Flags().GetString("type"); v != "" {
				if v != "openai-compatible" && v != "anthropic" && v != provider.CopilotNativeType {
					return fmt.Errorf("invalid provider type %q (valid: openai-compatible, anthropic, copilot-native)", v)
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
			// Enforce the copilot-native opt-in gate before persisting.
			// Per rubber-duck #2: this is the first of three activation
			// paths (set, auto-detect, config-set); all three must gate.
			if cfg.Provider.Type == provider.CopilotNativeType && !store.CopilotNativeOptedIn() {
				fmt.Fprint(cmd.OutOrStdout(), copilotNativeOptInPrompt())
				return fmt.Errorf("copilot-native requires opt-in; run `tpatch config set provider.copilot_native_optin true`")
			}

			if repoScope {
				if err := s.SaveConfig(cfg); err != nil {
					return err
				}
			} else {
				if err := store.SaveGlobalConfig(cfg); err != nil {
					return err
				}
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Provider configured [%s]: type=%s url=%s model=%s\n",
				target, cfg.Provider.Type, cfg.Provider.BaseURL, cfg.Provider.Model)

			// Show the AUP warning the first time a Copilot-flavoured config
			// is selected (once per user, recorded in global config).
			provCfg := provider.Config{
				Type:      cfg.Provider.Type,
				BaseURL:   cfg.Provider.BaseURL,
				Model:     cfg.Provider.Model,
				AuthEnv:   cfg.Provider.AuthEnv,
				Initiator: cfg.Provider.Initiator,
			}
			maybeShowAUPWarning(cmd.OutOrStdout(), provCfg)
			return nil
		},
	}
	cmd.Flags().String("preset", "", "Preset: copilot | copilot-native | openai | openrouter | anthropic | ollama")
	cmd.Flags().String("type", "", "Provider type: openai-compatible | anthropic | copilot-native")
	cmd.Flags().String("base-url", "", "Provider base URL")
	cmd.Flags().String("model", "", "Default model")
	cmd.Flags().String("auth-env", "", "Environment variable name for auth token")
	cmd.Flags().Bool("repo", false, "Write to the repo-level .tpatch/config.yaml instead of the global config")
	return cmd
}

// providerPresets are vetted one-line configurations for common providers.
// Each preset matches a widely used endpoint that speaks either the
// OpenAI chat completions API or the Anthropic Messages API — the two
// protocols implemented in internal/provider.
var providerPresets = map[string]struct {
	Type, BaseURL, Model, AuthEnv string
}{
	"copilot":        {"openai-compatible", "http://localhost:4141", "claude-sonnet-4", "GITHUB_TOKEN"},
	"copilot-native": {provider.CopilotNativeType, "", "claude-sonnet-4", ""},
	"openai":         {"openai-compatible", "https://api.openai.com", "gpt-4o", "OPENAI_API_KEY"},
	"openrouter":     {"openai-compatible", "https://openrouter.ai/api", "anthropic/claude-sonnet-4", "OPENROUTER_API_KEY"},
	"anthropic":      {"anthropic", "https://api.anthropic.com", "claude-sonnet-4-5", "ANTHROPIC_API_KEY"},
	"ollama":         {"openai-compatible", "http://localhost:11434", "llama3.2", ""},
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

			// Global-only keys bypass the repo store. Per rubber-duck
			// review #3: opt-in state must persist across repos.
			switch key {
			case "provider.copilot_native_optin":
				if strings.ToLower(value) != "true" {
					return fmt.Errorf("opt-in must be set to `true`; see ADR-005")
				}
				if err := store.AcknowledgeCopilotNativeOptIn(); err != nil {
					return err
				}
				fmt.Fprint(cmd.OutOrStdout(), copilotNativeAUPNotice())
				fmt.Fprintf(cmd.OutOrStdout(), "\nOpt-in recorded in global config.\n")
				return nil
			}

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
				if value == provider.CopilotNativeType && !store.CopilotNativeOptedIn() {
					fmt.Fprint(cmd.OutOrStdout(), copilotNativeOptInPrompt())
					return fmt.Errorf("copilot-native requires opt-in; run `tpatch config set provider.copilot_native_optin true`")
				}
				cfg.Provider.Type = value
			case "provider.base_url":
				cfg.Provider.BaseURL = value
			case "provider.model":
				cfg.Provider.Model = value
			case "provider.auth_env":
				cfg.Provider.AuthEnv = value
			case "provider.initiator":
				if value != "" && value != "user" && value != "agent" {
					return fmt.Errorf("provider.initiator must be empty, \"user\", or \"agent\"")
				}
				cfg.Provider.Initiator = value
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
				return fmt.Errorf("unknown config key %q (valid: provider.type, provider.base_url, provider.model, provider.auth_env, provider.initiator, provider.copilot_native_optin, merge_strategy, max_retries, test_command)", key)
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

// probedEndpoints tracks base URLs already probed this process so the
// reachability check only runs once per run, not per workflow phase.
var (
	probedEndpoints   = map[string]error{}
	probedEndpointsMu sync.Mutex
)

func loadProviderFromStore(s *store.Store) (provider.Provider, provider.Config) {
	cfg, err := s.LoadMergedConfig()
	if err != nil {
		return nil, provider.Config{}
	}
	provCfg := provider.Config{
		Type:      cfg.Provider.Type,
		BaseURL:   cfg.Provider.BaseURL,
		Model:     cfg.Provider.Model,
		AuthEnv:   cfg.Provider.AuthEnv,
		Initiator: cfg.Provider.Initiator,
	}
	if !provCfg.Configured() {
		return nil, provCfg
	}
	return provider.NewFromConfig(provCfg), provCfg
}

// loadAndProbeProvider is loadProviderFromStore + a one-time reachability
// probe for local endpoints (cached per-process). Workflow commands use
// this to hard-fail with an install hint when a local proxy is expected
// but not running. Returns (nil, cfg, nil) if the provider is not
// configured (heuristic fallback path is preserved).
func loadAndProbeProvider(ctx context.Context, s *store.Store) (provider.Provider, provider.Config, error) {
	prov, cfg := loadProviderFromStore(s)
	if prov == nil || !provider.IsLocalEndpoint(cfg) || os.Getenv("TPATCH_NO_PROBE") != "" {
		return prov, cfg, nil
	}
	probedEndpointsMu.Lock()
	cached, seen := probedEndpoints[cfg.BaseURL]
	probedEndpointsMu.Unlock()
	if seen {
		return prov, cfg, cached
	}
	probeErr := ensureProviderReachable(ctx, cfg)
	probedEndpointsMu.Lock()
	probedEndpoints[cfg.BaseURL] = probeErr
	probedEndpointsMu.Unlock()
	if probeErr != nil {
		return nil, cfg, probeErr
	}
	return prov, cfg, nil
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
			maybeShowAUPWarning(cmd.OutOrStdout(), provCfg)
			return
		}
	}
}
