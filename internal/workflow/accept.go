// Shared "accept a resolved shadow into the live tree" helper.
//
// Extracted in Tranche C2 / v0.5.2 (finding #1) so the same code path
// serves both:
//
//   - the manual `tpatch reconcile --accept <slug>` CLI command, and
//   - the auto-apply leg of `tpatch reconcile --resolve --apply` which
//     previously set `ReconcileReapplied` WITHOUT ever copying shadow
//     content onto the real tree. See ADR-010 §"caller responsibility"
//     for the original design intent; the v0.5.1-and-prior reconcile
//     wiring dropped that step.
//
// The helper is the single source of truth for the accept transition:
//
//  1. Apply non-conflicted hunks of the original post-apply patch
//     (resolved paths excluded so git never writes conflict markers).
//  2. Copy resolved files from the shadow worktree onto the real tree
//     (CopyShadowToReal uses ensureSafeRepoPath internally).
//  3. Refresh derived artifacts via RefreshAfterAccept (post-apply.patch
//     + patches/NNN-reconcile.patch).
//  4. Mark the feature state as applied with human-readable notes.
//  5. Prune the shadow worktree and clear the shadow pointer from
//     status.json so the audit trail ends cleanly.
//
// On mid-flight failure, the helper returns a structured error and
// does NOT prune the shadow — the user needs the shadow to investigate
// and retry. Callers map the error to a blocked verdict.

package workflow

import (
	"fmt"
	"time"

	"github.com/tesseracode/tesserapatch/internal/gitutil"
	"github.com/tesseracode/tesserapatch/internal/store"
)

// AcceptOptions controls non-essential side effects. Zero value is the
// classical `reconcile --accept` behavior: refresh artifacts, flip
// state to applied, prune shadow.
type AcceptOptions struct {
	// ResolveSessionID, when non-empty, is preserved in the feature
	// status as an audit record. Defaults to the existing value.
	ResolveSessionID string

	// Phase is recorded in feature notes (useful to distinguish
	// "reconcile --accept" from "reconcile --resolve --apply" in the
	// audit trail). Defaults to "reconcile --accept".
	Phase string
}

// AcceptResult is what the helper did.
type AcceptResult struct {
	// AcceptedFiles is the list of resolved paths copied from the
	// shadow worktree onto the real tree (step 2 above).
	AcceptedFiles []string

	// Pruned is true if the shadow worktree was removed at the end.
	Pruned bool

	// RefreshWarning, if non-empty, is a best-effort message that
	// the derived-artifact refresh failed. The accept still
	// completed (working tree reflects the new state); the user
	// should run `tpatch record` to refresh manually.
	RefreshWarning string
}

// AcceptShadow runs the full "shadow → real tree" accept transition
// for a feature. See the package doc comment for the sequence.
//
// Preconditions (caller's responsibility when called from
// `reconcile --resolve --apply`):
//   - The resolver has already produced a complete shadow (all files
//     in `files` exist at the correct paths inside the shadow).
//   - The feature has a recorded post-apply.patch.
//
// Preconditions when called from `reconcile --accept`:
//   - The feature is in state `reconciling-shadow` (caller checks).
//
// On error, the shadow is preserved so the user can recover.
func AcceptShadow(s *store.Store, slug string, files []string, upstreamCommit string, opts AcceptOptions) (*AcceptResult, error) {
	if len(files) == 0 {
		return nil, fmt.Errorf("accept: no resolved files to copy for %q", slug)
	}

	originalPatch, err := s.ReadFeatureFile(slug, "artifacts/post-apply.patch")
	if err != nil {
		return nil, fmt.Errorf("accept: read post-apply.patch: %w", err)
	}

	// Step 1: apply non-conflicted hunks while leaving resolved paths
	// to be overwritten in step 2.
	if originalPatch != "" {
		if err := gitutil.ForwardApplyExcluding(s.Root, originalPatch, files); err != nil {
			return nil, fmt.Errorf("accept: apply non-conflicted hunks: %w", err)
		}
	}

	// Step 2: overlay resolved content from shadow.
	if err := gitutil.CopyShadowToReal(s.Root, slug, files); err != nil {
		return nil, fmt.Errorf("accept: copy shadow → real: %w", err)
	}

	res := &AcceptResult{AcceptedFiles: append([]string{}, files...)}

	// Step 3: refresh derived artifacts. NOT fatal — staleness here
	// does not corrupt the working tree; users can rerun `tpatch record`.
	if upstreamCommit != "" {
		if rerr := RefreshAfterAccept(s, slug, upstreamCommit, originalPatch); rerr != nil {
			res.RefreshWarning = fmt.Sprintf("derived-artifact refresh failed: %v (accepted files are on disk; run `tpatch record` to refresh manually)", rerr)
		}
	} else {
		res.RefreshWarning = "no upstream commit recorded; skipping derived-artifact refresh (run `tpatch record` to refresh manually)"
	}

	// Step 4: feature state → applied.
	phase := opts.Phase
	if phase == "" {
		phase = "reconcile --accept"
	}
	notes := fmt.Sprintf("%s: %d file(s) accepted from shadow; derived artifacts refreshed", phase, len(files))
	if err := s.MarkFeatureState(slug, store.StateApplied, phase, notes); err != nil {
		return res, fmt.Errorf("accept: mark state: %w", err)
	}

	// Step 5: prune shadow + clear pointer. Both are best-effort.
	if err := gitutil.PruneShadow(s.Root, slug); err != nil {
		// Prune failure is non-fatal — the shadow is noise in the
		// worktree list but the feature is correctly applied.
		// Append to refresh warning so callers get a single surface.
		if res.RefreshWarning == "" {
			res.RefreshWarning = fmt.Sprintf("prune shadow failed: %v", err)
		} else {
			res.RefreshWarning = res.RefreshWarning + "; prune shadow failed: " + err.Error()
		}
	} else {
		res.Pruned = true
	}

	if err := clearShadowPointerAndStamp(s, slug, opts.ResolveSessionID); err != nil {
		if res.RefreshWarning == "" {
			res.RefreshWarning = fmt.Sprintf("clear shadow pointer failed: %v", err)
		} else {
			res.RefreshWarning = res.RefreshWarning + "; clear shadow pointer failed: " + err.Error()
		}
	}

	return res, nil
}

// clearShadowPointerAndStamp mirrors the CLI's clearShadowPointer but
// lives in workflow/ so the shared helper does not import cli. Also
// preserves an explicit ResolveSessionID when the caller passes one
// (the auto-apply path wants to stamp the resolver session id even
// though the shadow pointer is gone).
func clearShadowPointerAndStamp(s *store.Store, slug, sessionID string) error {
	st, err := s.LoadFeatureStatus(slug)
	if err != nil {
		return err
	}
	st.Reconcile.ShadowPath = ""
	st.Reconcile.ResolvedFiles = 0
	st.Reconcile.FailedFiles = 0
	st.Reconcile.SkippedFiles = 0
	if sessionID != "" {
		st.Reconcile.ResolveSession = sessionID
	}
	st.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	return s.SaveFeatureStatus(st)
}
