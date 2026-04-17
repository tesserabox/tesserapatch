package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/tesserabox/tesserapatch/internal/gitutil"
	"github.com/tesserabox/tesserapatch/internal/store"
	"github.com/tesserabox/tesserapatch/internal/workflow"
)

// ─── cycle ───────────────────────────────────────────────────────────────────

func cycleCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cycle <slug>",
		Short: "Run analyze → define → explore → implement → apply → record in sequence",
		Long: `Run the full feature lifecycle in one command.

By default, runs in batch mode without prompts. Use --interactive to pause
between phases and confirm before continuing. Use --skip-execute to stop
before the apply execute step (useful for agent-driven workflows where the
agent implements the code).`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			slug := args[0]
			s, err := openStoreFromCmd(cmd)
			if err != nil {
				return err
			}

			interactive, _ := cmd.Flags().GetBool("interactive")
			skipExecute, _ := cmd.Flags().GetBool("skip-execute")
			editor, _ := cmd.Flags().GetBool("editor")
			timeout, _ := cmd.Flags().GetDuration("timeout")

			prov, provCfg := loadProviderFromStore(s)
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()

			out := cmd.OutOrStdout()
			in := cmd.InOrStdin()
			reader := bufio.NewReader(in)

			status, err := s.LoadFeatureStatus(slug)
			if err != nil {
				return fmt.Errorf("feature %q not found — run 'tpatch add' first: %w", slug, err)
			}

			// [1/6] analyze
			fmt.Fprintf(out, "[1/6] Analyzing %s...\n", slug)
			result, err := workflow.RunAnalysis(ctx, s, slug, prov, provCfg)
			if err != nil {
				return err
			}
			fmt.Fprintf(out, "  Summary: %s\n", truncate(result.Summary, 120))
			fmt.Fprintf(out, "  Compatibility: %s\n", result.Compatibility.Status)
			if !confirm(interactive, reader, out, "Continue to define phase?") {
				return nil
			}

			// [2/6] define
			fmt.Fprintf(out, "[2/6] Defining acceptance criteria...\n")
			if err := workflow.RunDefine(ctx, s, slug, prov, provCfg); err != nil {
				return err
			}
			fmt.Fprintf(out, "  Spec written to .tpatch/features/%s/spec.md\n", slug)
			if editor && interactive {
				openInEditor(out, filepath.Join(s.Root, ".tpatch", "features", slug, "spec.md"))
			}
			if !confirm(interactive, reader, out, "Continue to explore phase?") {
				return nil
			}

			// [3/6] explore
			fmt.Fprintf(out, "[3/6] Exploring codebase...\n")
			if err := workflow.RunExplore(ctx, s, slug, prov, provCfg); err != nil {
				return err
			}
			fmt.Fprintf(out, "  Exploration written to .tpatch/features/%s/exploration.md\n", slug)
			if !confirm(interactive, reader, out, "Continue to implement phase?") {
				return nil
			}

			// [4/6] implement
			fmt.Fprintf(out, "[4/6] Generating apply recipe...\n")
			if err := workflow.RunImplement(ctx, s, slug, prov, provCfg); err != nil {
				return err
			}
			recipe, loadErr := workflow.LoadRecipe(s, slug)
			if loadErr == nil {
				fmt.Fprintf(out, "  Recipe has %d operation(s)\n", len(recipe.Operations))
			}
			if skipExecute {
				fmt.Fprintf(out, "  --skip-execute set; stopping before recipe execution.\n")
				fmt.Fprintf(out, "  To apply manually: tpatch apply %s --mode execute\n", slug)
				return nil
			}
			if !confirm(interactive, reader, out, "Execute recipe now?") {
				return nil
			}

			// [5/6] apply --mode execute
			fmt.Fprintf(out, "[5/6] Executing recipe...\n")
			if loadErr != nil {
				return loadErr
			}
			if err := s.MarkFeatureState(slug, store.StateImplementing, "cycle", "Executing recipe"); err != nil {
				return err
			}
			execResult := workflow.ExecuteRecipe(s.Root, recipe)
			for _, msg := range execResult.Messages {
				fmt.Fprintf(out, "  %s\n", msg)
			}
			for _, e := range execResult.Errors {
				fmt.Fprintf(cmd.ErrOrStderr(), "  ERROR: %s\n", e)
			}
			if !execResult.Success {
				return fmt.Errorf("recipe execution failed: %d error(s)", len(execResult.Errors))
			}
			fmt.Fprintf(out, "  %d/%d operations succeeded\n", execResult.Applied, execResult.Operations)

			// [6/6] record
			if !confirm(interactive, reader, out, "Record captured patch?") {
				return nil
			}
			fmt.Fprintf(out, "[6/6] Recording patch...\n")
			patch, patchErr := gitutil.CapturePatch(s.Root)
			if patchErr != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "  warning: capture failed: %v\n", patchErr)
			}
			if patch != "" {
				s.WriteArtifact(slug, "post-apply.patch", patch)
				if name, _ := s.WritePatch(slug, "cycle", patch); name != "" {
					fmt.Fprintf(out, "  Saved patch: patches/%s\n", name)
				}
			}
			now := time.Now().UTC().Format(time.RFC3339)
			commit, _ := gitutil.HeadCommit(s.Root)
			status.Apply.BaseCommit = commit
			status.Apply.CompletedAt = now
			status.Apply.HasPatch = patch != ""
			s.SaveFeatureStatus(status)
			if err := s.MarkFeatureState(slug, store.StateApplied, "cycle", "Cycle complete"); err != nil {
				return err
			}
			fmt.Fprintf(out, "Feature %s is now in state: applied\n", slug)
			return nil
		},
	}
	cmd.Flags().Bool("interactive", false, "Pause between phases with confirmation prompts")
	cmd.Flags().Bool("editor", false, "Open $EDITOR on spec.md during interactive cycle")
	cmd.Flags().Bool("skip-execute", false, "Stop before executing the apply recipe")
	cmd.Flags().Duration("timeout", 5*time.Minute, "Total timeout for all LLM phases")
	return cmd
}

func confirm(interactive bool, reader *bufio.Reader, out io.Writer, prompt string) bool {
	if !interactive {
		return true
	}
	fmt.Fprintf(out, "  ▸ %s [Y/n] ", prompt)
	line, err := reader.ReadString('\n')
	if err != nil {
		return false
	}
	line = strings.ToLower(strings.TrimSpace(line))
	if line == "" || line == "y" || line == "yes" {
		return true
	}
	return false
}

func openInEditor(out io.Writer, path string) {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		fmt.Fprintf(out, "  (set $EDITOR to review %s in your editor)\n", path)
		return
	}
	c := exec.Command(editor, path)
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	_ = c.Run()
}

func truncate(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// ─── test ────────────────────────────────────────────────────────────────────

func testCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "test <slug>",
		Short: "Run the configured test command and record the result",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			slug := args[0]
			s, err := openStoreFromCmd(cmd)
			if err != nil {
				return err
			}
			cfg, err := s.LoadConfig()
			if err != nil {
				return err
			}
			testCmdStr := strings.TrimSpace(cfg.TestCommand)
			if v, _ := cmd.Flags().GetString("command"); v != "" {
				testCmdStr = v
			}
			if testCmdStr == "" {
				return fmt.Errorf("no test_command configured — run 'tpatch config set test_command <cmd>' or pass --command")
			}

			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Running: %s\n", testCmdStr)

			timeout, _ := cmd.Flags().GetDuration("timeout")
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()

			c := exec.CommandContext(ctx, "sh", "-c", testCmdStr)
			c.Dir = s.Root
			output, runErr := c.CombinedOutput()
			fmt.Fprint(out, string(output))

			outputArtifact := fmt.Sprintf("# Test run — %s\n# Command: %s\n# Directory: %s\n\n%s",
				time.Now().UTC().Format(time.RFC3339), testCmdStr, s.Root, string(output))
			s.WriteArtifact(slug, "test-output.txt", outputArtifact)

			status, _ := s.LoadFeatureStatus(slug)
			valStatus := "passed"
			if runErr != nil {
				valStatus = "failed"
			}
			status.Apply.CompletedAt = time.Now().UTC().Format(time.RFC3339)

			session := store.ApplySession{
				Slug:             slug,
				PreparedAt:       status.Apply.PreparedAt,
				StartedAt:        status.Apply.StartedAt,
				CompletedAt:      status.Apply.CompletedAt,
				BaseCommit:       status.Apply.BaseCommit,
				HasPatch:         status.Apply.HasPatch,
				ValidationStatus: valStatus,
				ValidationNotes:  fmt.Sprintf("tpatch test — command: %s", testCmdStr),
			}
			s.SaveApplySession(slug, session)
			s.SaveFeatureStatus(status)

			if runErr != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "Test command failed: %v\n", runErr)
				return fmt.Errorf("tests failed")
			}
			fmt.Fprintf(out, "Tests passed for %s\n", slug)
			return nil
		},
	}
	cmd.Flags().String("command", "", "Override the configured test command")
	cmd.Flags().Duration("timeout", 10*time.Minute, "Test command timeout")
	return cmd
}

// ─── next ────────────────────────────────────────────────────────────────────

// HarnessTask is the JSON payload returned by `tpatch next --format harness-json`.
type HarnessTask struct {
	Phase        string   `json:"phase"`
	Slug         string   `json:"slug"`
	State        string   `json:"state"`
	Instructions string   `json:"instructions"`
	ContextFiles []string `json:"context_files"`
	OnComplete   string   `json:"on_complete"`
	OnAbort      string   `json:"on_abort,omitempty"`
}

func nextCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "next <slug>",
		Short: "Emit the next logical action for a feature",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			slug := args[0]
			s, err := openStoreFromCmd(cmd)
			if err != nil {
				return err
			}
			status, err := s.LoadFeatureStatus(slug)
			if err != nil {
				return fmt.Errorf("feature %q not found: %w", slug, err)
			}
			format, _ := cmd.Flags().GetString("format")

			task := nextAction(s, status)

			out := cmd.OutOrStdout()
			switch format {
			case "harness-json":
				data, _ := json.MarshalIndent(task, "", "  ")
				fmt.Fprintln(out, string(data))
			default:
				fmt.Fprintf(out, "Feature: %s\n", task.Slug)
				fmt.Fprintf(out, "State:   %s\n", task.State)
				fmt.Fprintf(out, "Phase:   %s\n", task.Phase)
				fmt.Fprintf(out, "\n%s\n", task.Instructions)
				if len(task.ContextFiles) > 0 {
					fmt.Fprintf(out, "\nContext files:\n")
					for _, f := range task.ContextFiles {
						fmt.Fprintf(out, "  - %s\n", f)
					}
				}
				if task.OnComplete != "" {
					fmt.Fprintf(out, "\nOn complete: %s\n", task.OnComplete)
				}
				if task.OnAbort != "" {
					fmt.Fprintf(out, "On abort:    %s\n", task.OnAbort)
				}
			}
			return nil
		},
	}
	cmd.Flags().String("format", "text", "Output format: text | harness-json")
	return cmd
}

func nextAction(s *store.Store, status store.FeatureStatus) HarnessTask {
	slug := status.Slug
	featureDir := filepath.Join(".tpatch", "features", slug)
	req := filepath.Join(featureDir, "request.md")
	analysis := filepath.Join(featureDir, "analysis.md")
	spec := filepath.Join(featureDir, "spec.md")
	exploration := filepath.Join(featureDir, "exploration.md")
	recipe := filepath.Join(featureDir, "artifacts", "apply-recipe.json")

	switch status.State {
	case store.StateRequested:
		return HarnessTask{
			Phase:        "analyze",
			Slug:         slug,
			State:        string(status.State),
			Instructions: "Run analysis on the feature request to assess compatibility and affected areas.",
			ContextFiles: []string{req},
			OnComplete:   fmt.Sprintf("tpatch analyze %s", slug),
		}
	case store.StateAnalyzed:
		return HarnessTask{
			Phase:        "define",
			Slug:         slug,
			State:        string(status.State),
			Instructions: "Read the analysis and generate acceptance criteria + implementation plan in spec.md.",
			ContextFiles: []string{req, analysis},
			OnComplete:   fmt.Sprintf("tpatch define %s", slug),
		}
	case store.StateDefined:
		// Defined may be between spec and exploration, or between exploration and implement.
		if !fileExistsAt(s.Root, featureDir, "exploration.md") {
			return HarnessTask{
				Phase:        "explore",
				Slug:         slug,
				State:        string(status.State),
				Instructions: "Explore the codebase and identify the minimal changeset (files, code sections) needed to implement the spec.",
				ContextFiles: []string{req, analysis, spec},
				OnComplete:   fmt.Sprintf("tpatch explore %s", slug),
			}
		}
		if !fileExistsAt(s.Root, featureDir, filepath.Join("artifacts", "apply-recipe.json")) {
			return HarnessTask{
				Phase:        "implement",
				Slug:         slug,
				State:        string(status.State),
				Instructions: "Generate the deterministic apply-recipe.json that writes/modifies the files identified in exploration.md.",
				ContextFiles: []string{req, analysis, spec, exploration},
				OnComplete:   fmt.Sprintf("tpatch implement %s", slug),
			}
		}
		return HarnessTask{
			Phase:        "apply",
			Slug:         slug,
			State:        string(status.State),
			Instructions: "Review the apply recipe, then execute it to materialize the changes in the working tree.",
			ContextFiles: []string{spec, exploration, recipe},
			OnComplete:   fmt.Sprintf("tpatch apply %s --mode execute", slug),
			OnAbort:      fmt.Sprintf("tpatch apply %s --mode started", slug),
		}
	case store.StateImplementing:
		return HarnessTask{
			Phase:        "apply",
			Slug:         slug,
			State:        string(status.State),
			Instructions: "Implementation is in progress. Execute the apply recipe or, if implementing manually, run tests and then mark the apply as done.",
			ContextFiles: []string{spec, exploration, recipe},
			OnComplete:   fmt.Sprintf("tpatch apply %s --mode done", slug),
			OnAbort:      fmt.Sprintf("tpatch apply %s --mode started", slug),
		}
	case store.StateApplied, store.StateActive:
		return HarnessTask{
			Phase:        "test",
			Slug:         slug,
			State:        string(status.State),
			Instructions: "Run the project's test command to validate the applied changes. If tests pass, the feature is complete until the next upstream reconciliation.",
			ContextFiles: []string{filepath.Join(featureDir, "record.md")},
			OnComplete:   fmt.Sprintf("tpatch test %s", slug),
		}
	case store.StateReconciling:
		return HarnessTask{
			Phase:        "reconcile",
			Slug:         slug,
			State:        string(status.State),
			Instructions: "Reconciliation in progress. Re-apply the patch against the new upstream or mark as upstream-merged.",
			ContextFiles: []string{filepath.Join(featureDir, "reconciliation")},
			OnComplete:   fmt.Sprintf("tpatch reconcile %s", slug),
		}
	case store.StateBlocked:
		return HarnessTask{
			Phase:        "review",
			Slug:         slug,
			State:        string(status.State),
			Instructions: "Feature is blocked. Review the last command output and notes, resolve the blocker, then re-run the appropriate phase.",
			ContextFiles: []string{req},
		}
	case store.StateUpstreamMerged:
		return HarnessTask{
			Phase:        "done",
			Slug:         slug,
			State:        string(status.State),
			Instructions: "Feature has been merged upstream. No further action required.",
		}
	default:
		return HarnessTask{
			Phase:        "review",
			Slug:         slug,
			State:        string(status.State),
			Instructions: "Feature is in an unrecognized state. Review status.json manually.",
			ContextFiles: []string{filepath.Join(featureDir, "status.json")},
		}
	}
}

func fileExistsAt(root, dir, name string) bool {
	_, err := os.Stat(filepath.Join(root, dir, name))
	return err == nil
}
