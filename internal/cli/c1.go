package cli

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tesseracode/tesserapatch/internal/store"
)

// featureDirPath constructs the absolute path to a feature's directory
// under .tpatch/features/<slug>/. Kept local to the c1 commands (edit/
// amend/remove) rather than exported from store.
func featureDirPath(s *store.Store, slug string) string {
	return filepath.Join(s.TpatchDir(), "features", slug)
}

// resolveArtifactPath probes both the feature root and its artifacts/
// subdir and returns the first path that exists. Some files (request.md,
// spec.md, apply-recipe.json, analysis.md, exploration.md, record.md) live
// directly under .tpatch/features/<slug>/; others (post-apply.patch,
// apply-session.json, etc.) live under artifacts/. When neither exists,
// the top-level path is returned with exists=false so callers can surface
// a "does not exist" error that still points at the canonical location.
func resolveArtifactPath(s *store.Store, slug, artifact string) (path string, exists bool) {
	topLevel := filepath.Join(featureDirPath(s, slug), artifact)
	inArtifacts := filepath.Join(featureDirPath(s, slug), "artifacts", artifact)
	if _, err := os.Stat(topLevel); err == nil {
		return topLevel, true
	}
	if _, err := os.Stat(inArtifacts); err == nil {
		return inArtifacts, true
	}
	return topLevel, false
}

// defaultEditArtifact picks the most relevant artifact for a given
// feature state. Falls back to request.md for unknown / edge states.
func defaultEditArtifact(state store.FeatureState) string {
	switch state {
	case store.StateRequested:
		return "request.md"
	case store.StateAnalyzed, store.StateDefined:
		return "spec.md"
	case store.StateImplementing:
		return "apply-recipe.json"
	case store.StateApplied:
		return "post-apply.patch"
	default:
		return "request.md"
	}
}

// ─── edit ────────────────────────────────────────────────────────────────────

func editCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "edit <slug> [artifact]",
		Short: "Open a feature artifact in $EDITOR (defaulted by state)",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			slug := args[0]
			s, err := openStoreFromCmd(cmd)
			if err != nil {
				return err
			}
			if _, err := os.Stat(featureDirPath(s, slug)); err != nil {
				return fmt.Errorf("feature %s does not exist", slug)
			}

			artifact := ""
			if len(args) == 2 {
				artifact = args[1]
			} else {
				status, _ := s.LoadFeatureStatus(slug)
				artifact = defaultEditArtifact(status.State)
			}

			path, exists := resolveArtifactPath(s, slug, artifact)
			if !exists {
				return fmt.Errorf("artifact %q does not exist for feature %s", artifact, slug)
			}
			openInEditor(cmd.OutOrStdout(), path)
			return nil
		},
	}
	return cmd
}

// ─── amend ───────────────────────────────────────────────────────────────────

func amendCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "amend <slug> [description...]",
		Short: "Replace a feature's request.md (use --append to add instead of replace)",
		Long: `Replace (default) or append to a feature's request.md.

Default behavior REPLACES the existing request.md with the new description.
Use --append to concatenate the new description onto the existing request
(separated by a blank line).

--append and --reset are mutually exclusive: a state reset alongside an
append makes no sense (you are reopening the intent while pretending to
preserve it).

Reads the description from positional args, or from stdin when none are
provided.`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			cmd.SilenceErrors = true
			slug := args[0]

			// Slice B (ADR-013 D3 / PRD-verify-freshness §9):
			// `amend --state tested` is rejected with exit 2. The
			// `tested` lifecycle state does not exist under the
			// freshness-overlay model — verify is an overlay, not a
			// state. Validate before doing any other work so we never
			// touch disk on a refused command.
			if stateFlag, _ := cmd.Flags().GetString("state"); stateFlag != "" {
				if err := validateAmendStateFlag(stateFlag); err != nil {
					return err
				}
			}

			s, err := openStoreFromCmd(cmd)
			if err != nil {
				return err
			}
			if _, err := os.Stat(featureDirPath(s, slug)); err != nil {
				return fmt.Errorf("feature %s does not exist", slug)
			}

			appendMode, _ := cmd.Flags().GetBool("append")
			reset, _ := cmd.Flags().GetBool("reset")
			if appendMode && reset {
				return fmt.Errorf("amend: --append and --reset are mutually exclusive")
			}

			depAdds, _ := cmd.Flags().GetStringArray("depends-on")
			depRms, _ := cmd.Flags().GetStringArray("remove-depends-on")
			depsOnly := (len(depAdds) > 0 || len(depRms) > 0) && len(args) == 1 && !stdinIsPiped(cmd)

			var description string
			if !depsOnly {
				switch {
				case len(args) > 1:
					description = strings.Join(args[1:], " ")
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
					return fmt.Errorf("provide a new description as arguments or pipe via stdin")
				}
			}

			// Capture the recipe's pre-amend bytes so we can detect a
			// recipe-touching amend by either path:
			//   1. amend itself rewrites apply-recipe.json (no current
			//      flag does this, but future flags might).
			//   2. the on-disk recipe already differs from what the
			//      persisted Verify record was computed against (manual
			//      edits between `tpatch verify` and `tpatch amend`).
			// We invalidate Verify if either is true. The producer-set
			// rule (ADR-013 D3): amend asserts authorship on the
			// feature; a Verify whose recorded recipe hash no longer
			// matches the live recipe is stale and must be cleared.
			recipeBefore := readRecipeBytes(s, slug)

			if !depsOnly {
				var newBody string
				if appendMode {
					// Blind concat with a blank-line separator. A future enhancement
					// could parse request.md section headers and append to a canonical
					// "Additional requirements" block; documented in help text so
					// users are not surprised by the current straightforward model.
					existing, _ := s.ReadFeatureFile(slug, "request.md")
					existing = strings.TrimRight(existing, "\n")
					if existing == "" {
						newBody = description + "\n"
					} else {
						newBody = existing + "\n\n" + description + "\n"
					}
				} else {
					newBody = description + "\n"
				}

				if err := s.WriteFeatureFile(slug, "request.md", newBody); err != nil {
					return err
				}

				if reset {
					if err := s.MarkFeatureState(slug, store.StateRequested, "amend --reset", "Request replaced; state reset"); err != nil {
						return err
					}
				}
			}

			// Apply any --depends-on / --remove-depends-on edits.
			if len(depAdds) > 0 || len(depRms) > 0 {
				if !dependencyConfigEnabled(s) {
					return fmt.Errorf("amend --depends-on requires features_dependencies: true in .tpatch/config.yaml")
				}
				if err := applyAmendDependsOn(cmd, s, slug); err != nil {
					return err
				}
			}

			// Slice B (ADR-013 D3): a recipe-touching amend invalidates
			// the existing Verify record. Two trigger conditions, EITHER
			// of which causes the clear:
			//   (a) amend mutated the recipe in-flight (pre/post bytes
			//       differ — future-proofing for flags that rewrite
			//       apply-recipe.json directly).
			//   (b) the on-disk recipe differs from what the persisted
			//       Verify recorded (`Verify.RecipeHashAtVerify`).
			//       External edits between `tpatch verify` and
			//       `tpatch amend` are caught here — the case the
			//       external supervisor reproduced for revision-3.
			recipeAfter := readRecipeBytes(s, slug)
			if !bytes.Equal(recipeBefore, recipeAfter) || recipeDiffersFromVerify(s, slug, recipeAfter) {
				if err := clearVerifyForAmend(s, slug); err != nil {
					return err
				}
			}

			status, _ := s.LoadFeatureStatus(slug)
			verb := "Amended"
			if appendMode {
				verb = "Appended to"
			}
			if depsOnly {
				verb = "Updated dependencies on"
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s feature %s (state: %s)\n", verb, slug, status.State)
			return nil
		},
	}
	cmd.Flags().Bool("reset", false, "Reset feature state to \"requested\"")
	cmd.Flags().Bool("append", false, "Append to request.md instead of replacing it")
	cmd.Flags().StringArray("depends-on", nil, "Add or upgrade a depends_on edge (parent[:hard|:soft], repeatable). Requires features_dependencies.")
	cmd.Flags().StringArray("remove-depends-on", nil, "Remove a depends_on edge (parent slug, repeatable). Requires features_dependencies.")
	// Slice B (ADR-013 D3 / PRD-verify-freshness §9 — explicit reject
	// path for `amend --state tested`). The flag is wired ONLY to
	// surface a friendly error; no state value is currently accepted
	// by amend (lifecycle transitions are owned by other verbs).
	cmd.Flags().String("state", "", "Reserved — amend does not accept arbitrary state transitions")
	return cmd
}

// validAmendStates is the empty set: amend deliberately accepts no
// `--state` values. The flag exists to surface a clean exit-2 error
// for `amend --state tested` (and any other invented state) per
// ADR-013 D3 / PRD-verify-freshness §9.
var validAmendStates = map[string]struct{}{}

// validateAmendStateFlag returns an *ExitCodeError{Code:2} for any
// `--state <value>` invocation. `tested` in particular is called out
// because the freshness-overlay redesign (ADR-013) deliberately did
// NOT add a `tested` lifecycle state — verify is an overlay, not a
// state transition.
func validateAmendStateFlag(value string) error {
	if _, ok := validAmendStates[value]; ok {
		return nil
	}
	return &ExitCodeError{
		Code:    2,
		Message: fmt.Sprintf("amend: no such state %q. Lifecycle states are owned by other verbs (add/analyze/define/explore/implement/apply/reconcile). The freshness overlay (`tpatch verify`) is not a lifecycle state.", value),
	}
}

// recipeDiffersFromVerify returns true when the persisted Verify
// record's recorded recipe hash no longer matches the on-disk recipe.
// Used by amend to invalidate stale Verify records on the producer-set
// path (ADR-013 D3): amend takes ownership of the feature; if the
// recipe has drifted from the verified-against state, the Verify is
// no longer authoritative and must be cleared. Returns false when
// there is no Verify record (nothing to invalidate) or when both the
// recorded hash and the on-disk recipe are absent (mirrors the verify
// writer's both-absent-is-match semantic).
func recipeDiffersFromVerify(s *store.Store, slug string, recipeBytes []byte) bool {
	status, err := s.LoadFeatureStatus(slug)
	if err != nil || status.Verify == nil {
		return false
	}
	currentHash := ""
	if len(recipeBytes) > 0 {
		h := sha256.Sum256(recipeBytes)
		currentHash = hex.EncodeToString(h[:])
	}
	return currentHash != status.Verify.RecipeHashAtVerify
}

// readRecipeBytes returns the raw bytes of the feature's
// `apply-recipe.json` artifact, or nil if the file is absent /
// unreadable. Used by amend to detect recipe-touching amends.
func readRecipeBytes(s *store.Store, slug string) []byte {
	p := filepath.Join(s.TpatchDir(), "features", slug, "artifacts", "apply-recipe.json")
	data, err := os.ReadFile(p)
	if err != nil {
		return nil
	}
	return data
}

// clearVerifyForAmend invalidates the Verify sub-record on a recipe-
// touching amend per ADR-013 D3. We CLEAR the field (set to nil) rather
// than flip Passed=false so the next ComposeLabels derives
// `never-verified` (truthful) instead of `verify-failed` (which would
// imply a verify run had failed). The producer-set rule: amend is a
// producer of `Verify == nil`; verify is the only producer of a
// non-nil record.
func clearVerifyForAmend(s *store.Store, slug string) error {
	status, err := s.LoadFeatureStatus(slug)
	if err != nil {
		return err
	}
	if status.Verify == nil {
		return nil
	}
	status.Verify = nil
	return s.SaveFeatureStatus(status)
}

// ─── remove ──────────────────────────────────────────────────────────────────

func removeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove <slug>",
		Short: "Delete a feature directory and all its artifacts",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			slug := args[0]
			s, err := openStoreFromCmd(cmd)
			if err != nil {
				return err
			}
			if _, err := os.Stat(featureDirPath(s, slug)); err != nil {
				return fmt.Errorf("feature %s does not exist", slug)
			}

			force, _ := cmd.Flags().GetBool("force")
			cascade, _ := cmd.Flags().GetBool("cascade")

			// PRD §3.7 / ADR-011 D7: --force never bypasses the dep
			// integrity gate. Only --cascade may opt into removing a
			// feature with downstream dependents.
			if !cascade {
				if err := checkRemoveDependents(s, slug); err != nil {
					return err
				}
			}

			if cascade {
				return runRemoveWithCascade(cmd, s, slug, force)
			}

			// Contract (v0.5.1 shipped):
			//   --force       → always skip confirmation
			//   TTY stdin     → prompt [y/N]
			//   piped stdin / redirected / no TTY → skip confirmation (auto-yes)
			// The last branch is what lets scripts like
			//   printf 'y\n' | tpatch remove <slug>
			// and unattended CI steps succeed without --force.
			if !force && canPromptForConfirmation(cmd) {
				fmt.Fprintf(cmd.OutOrStdout(), "Remove feature %s and all its artifacts? [y/N] ", slug)
				reader := bufio.NewReader(cmd.InOrStdin())
				line, _ := reader.ReadString('\n')
				line = strings.TrimSpace(line)
				if line != "y" && line != "Y" {
					fmt.Fprintln(cmd.OutOrStdout(), "aborted")
					return nil
				}
			}

			if err := s.RemoveFeature(slug); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Removed feature %s\n", slug)
			return nil
		},
	}
	cmd.Flags().Bool("force", false, "Skip confirmation prompt (does NOT bypass dependency check)")
	cmd.Flags().Bool("cascade", false, "Also remove every dependent of this feature (reverse-topo order)")
	return cmd
}

// canPromptForConfirmation reports whether it is safe to ask the user
// a y/N question on the command's input stream. Returns true when:
//   - stdin is a terminal (regular interactive run), OR
//   - stdin has been replaced by cobra's SetIn (tests / scripted input).
//
// Returns false when stdin is a redirected file or pipe from an
// unattended context — the classical case where we must refuse to
// destructively act without an explicit --force.
func canPromptForConfirmation(cmd *cobra.Command) bool {
	in := cmd.InOrStdin()
	f, ok := in.(*os.File)
	if !ok {
		return true // tests / scripts using SetIn
	}
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}
