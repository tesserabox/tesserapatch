package workflow

// Derived-artifact refresh after a successful `tpatch reconcile --accept`.
//
// The accept flow produces a new working-tree state:
//
//	[new upstream commit] + [feature's non-conflict hunks (applied cleanly)]
//	                      + [phase-3.5 resolved files (copied from shadow)]
//
// That new state is what future reconciles should start from, so the
// feature's derived artifacts must be refreshed to reflect it:
//
//   - artifacts/post-apply.patch — regenerated as `git diff <upstreamCommit>`
//     restricted to the files the ORIGINAL post-apply.patch touched. Other
//     working-tree dirt (untracked files, unrelated edits) is excluded.
//   - patches/NNN-reconcile.patch — numbered snapshot of the new
//     post-apply.patch, serves as the audit trail of what changed.
//
// What this function does NOT refresh (deferred; callers should emit a
// note pointing users at `tpatch record` or an ADR-scoped followup):
//
//   - artifacts/apply-recipe.json — regenerating the op-level recipe
//     from a raw diff is lossy. Left stale for now; `tpatch record`
//     remains the authoritative path.
//   - artifacts/incremental.patch — reconcile.go already consumes
//     post-apply.patch as the fallback, so staleness here is cosmetic.

import (
	"fmt"

	"github.com/tesseracode/tesserapatch/internal/gitutil"
	"github.com/tesseracode/tesserapatch/internal/store"
)

// RefreshAfterAccept regenerates the feature's derived artifacts after
// the working tree has been updated via `reconcile --accept`.
// originalPatch is the pre-accept contents of post-apply.patch, used
// to determine which file paths to restrict the new diff to.
// upstreamCommit is the ref the regenerated diff is taken against.
//
// On error the caller should treat derived artifacts as potentially
// stale but should NOT roll back the working-tree changes — those are
// already reflected in the new state. The safe recovery path is to
// re-run `tpatch record` which uses the same plumbing.
func RefreshAfterAccept(s *store.Store, slug, upstreamCommit, originalPatch string) error {
	files := gitutil.FilesInPatch(originalPatch)
	newPatch, err := gitutil.DiffFromCommitForPaths(s.Root, upstreamCommit, files)
	if err != nil {
		return fmt.Errorf("refresh: regenerate post-apply.patch: %w", err)
	}

	// post-apply.patch is the source of truth for future reconciles.
	if err := s.WriteArtifact(slug, "post-apply.patch", newPatch); err != nil {
		return fmt.Errorf("refresh: write post-apply.patch: %w", err)
	}

	// Audit snapshot into patches/. The label "reconcile" matches the
	// ADR-010 design doc so future tooling can filter by it.
	if _, err := s.WritePatch(slug, "reconcile", newPatch); err != nil {
		return fmt.Errorf("refresh: write numbered reconcile patch: %w", err)
	}

	return nil
}
