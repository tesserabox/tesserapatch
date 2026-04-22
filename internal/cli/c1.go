package cli

import (
	"fmt"
	"os"
	"path/filepath"

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
