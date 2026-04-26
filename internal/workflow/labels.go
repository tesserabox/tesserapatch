// Composable reconcile labels — M14.3 / ADR-011 D3 + D6.
//
// ComposeLabels reads a child feature's hard-dependency declarations,
// loads each parent's FeatureStatus, and derives the overlay labels per
// PRD-feature-dependencies §3.5. Labels are computed at READ time and
// overlay on top of the child's intrinsic Reconcile.Outcome — they are
// NOT persisted as new ReconcileOutcome enum values.
//
// AUTHORITATIVE SOURCE GUARD (ADR-010 D5, ADR-011 D6):
//
//   This function reads parent verdicts via store.LoadFeatureStatus —
//   specifically status.Reconcile.Outcome. It MUST NEVER consult
//   artifacts/reconcile-session.json. The session artifact is an audit
//   record of one RunReconcile invocation; status.json is the source of
//   current truth post-accept. Any future change here that adds a path
//   reading session artifacts is a behavioural regression.
//
// Soft dependencies never produce labels (ADR-011 D4). Multiple labels
// can stack (e.g. one parent waiting + another stale → two labels).
// Output is deduplicated and sorted alphabetically for deterministic
// JSON serialization.

package workflow

import (
	"sort"

	"github.com/tesseracode/tesserapatch/internal/store"
)

// hasLabel reports whether labels contains the target label. Linear scan
// — label slices are typically 0-3 elements so a map would be overkill.
func hasLabel(labels []store.ReconcileLabel, target store.ReconcileLabel) bool {
	for _, l := range labels {
		if l == target {
			return true
		}
	}
	return false
}

// LabelWaitingOnParent. The set covers every pre-applied lifecycle state
// plus the in-flight reconcile states.
var transientStates = map[store.FeatureState]struct{}{
	store.StateRequested:         {},
	store.StateAnalyzed:          {},
	store.StateDefined:           {},
	store.StateImplementing:      {},
	store.StateReconciling:       {},
	store.StateReconcilingShadow: {},
}

// appliedSatisfyingStates: hard parents in any of these states are
// considered "applied" for label purposes — only stale and
// blocked-by-parent labels apply, never waiting-on-parent.
var appliedSatisfyingStates = map[store.FeatureState]struct{}{
	store.StateApplied:        {},
	store.StateActive:         {},
	store.StateUpstreamMerged: {},
}

// blockedReconcileOutcomes: a hard parent in StateApplied/Active whose
// last reconcile produced one of these verdicts is treated as blocked
// for label-composition purposes. The parent's working tree may be
// usable, but the operator has unresolved upstream work owed.
var blockedReconcileOutcomes = map[store.ReconcileOutcome]struct{}{
	store.ReconcileBlockedRequiresHuman:    {},
	store.ReconcileBlockedTooManyConflicts: {},
	store.ReconcileBlocked:                 {},
	store.ReconcileShadowAwaiting:          {},
}

// ComposeLabels returns the M14.3 overlay labels for a child feature
// based on its hard-parent set. Soft parents are skipped per ADR-011 D4.
//
// Returns an empty (nil) slice when:
//   - Config.DAGEnabled() is false (gate per ADR-011 D9).
//   - The child has no dependencies.
//   - No hard parent's state warrants a label.
//
// Errors only when the child's own status cannot be loaded — parent
// load failures are silently treated as LabelBlockedByParent (a missing
// parent is, by definition, not satisfying the child's dependency).
func ComposeLabels(s *store.Store, slug string) ([]store.ReconcileLabel, error) {
	cfg, err := s.LoadConfig()
	if err != nil {
		return nil, err
	}
	if !cfg.DAGEnabled() {
		return nil, nil
	}

	child, err := s.LoadFeatureStatus(slug)
	if err != nil {
		return nil, err
	}
	return composeLabelsFromStatus(s, child), nil
}

// composeLabelsAt is like ComposeLabels but uses asOf as the
// child's effective reconcile baseline for the staleness check, instead
// of the on-disk child.Reconcile.AttemptedAt. Callers inside
// RunReconcile use this to ensure persisted Labels reflect the
// AttemptedAt about to be written, not the previous run's value
// (M14 fix-pass F2).
func composeLabelsAt(s *store.Store, slug string, asOf string) ([]store.ReconcileLabel, error) {
	cfg, err := s.LoadConfig()
	if err != nil {
		return nil, err
	}
	if !cfg.DAGEnabled() {
		return nil, nil
	}
	child, err := s.LoadFeatureStatus(slug)
	if err != nil {
		return nil, err
	}
	if asOf != "" {
		child.Reconcile.AttemptedAt = asOf
	}
	return composeLabelsFromStatus(s, child), nil
}

// composeLabelsFromStatus is the body shared by ComposeLabels and
// composeLabelsAt. It accepts an already-loaded FeatureStatus so the
// caller can override fields (e.g. AttemptedAt) prior to label
// composition without round-tripping through disk.
func composeLabelsFromStatus(s *store.Store, child store.FeatureStatus) []store.ReconcileLabel {
	if len(child.DependsOn) == 0 {
		return nil
	}

	set := make(map[store.ReconcileLabel]struct{})

	for _, dep := range child.DependsOn {
		if dep.Kind != store.DependencyKindHard {
			continue // ADR-011 D4: soft deps never contribute to labels.
		}
		// CRITICAL — read parent verdict from status.json (ADR-010 D5).
		// Do NOT read artifacts/reconcile-session.json from any code
		// path reachable here; the adversarial test enforces this.
		parent, perr := s.LoadFeatureStatus(dep.Slug)
		if perr != nil {
			// Missing parent acts as a hard blocker.
			set[store.LabelBlockedByParent] = struct{}{}
			continue
		}

		// State-level classification first.
		if parent.State == store.StateBlocked {
			set[store.LabelBlockedByParent] = struct{}{}
			continue
		}
		if _, transient := transientStates[parent.State]; transient {
			set[store.LabelWaitingOnParent] = struct{}{}
			continue
		}
		if _, applied := appliedSatisfyingStates[parent.State]; applied {
			// upstream_merged with valid satisfied_by is fully retired:
			// the parent's changes are part of upstream, so neither
			// blocked-by-parent nor stale-parent-applied apply (the
			// child has no live local parent to drift against).
			if parent.State == store.StateUpstreamMerged {
				continue
			}
			// Reconcile-level overlay on applied/active parents.
			if _, blocked := blockedReconcileOutcomes[parent.Reconcile.Outcome]; blocked {
				set[store.LabelBlockedByParent] = struct{}{}
				continue
			}
			// Stale check: parent has been updated since the child's
			// last reconcile. We only flag this when the child has a
			// prior AttemptedAt — without a baseline timestamp there is
			// nothing to be "stale" against.
			if child.Reconcile.AttemptedAt != "" && parent.UpdatedAt != "" &&
				parent.UpdatedAt > child.Reconcile.AttemptedAt {
				set[store.LabelStaleParentApplied] = struct{}{}
			}
			continue
		}
		// Unknown / unhandled state — be conservative and treat as
		// blocked rather than silently dropping a label.
		set[store.LabelBlockedByParent] = struct{}{}
	}

	if len(set) == 0 {
		return nil
	}
	out := make([]store.ReconcileLabel, 0, len(set))
	for l := range set {
		out = append(out, l)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
