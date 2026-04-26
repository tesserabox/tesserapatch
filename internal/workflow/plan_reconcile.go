// Reconcile planner — M14.3 / ADR-011.
//
// PlanReconcile expands an input slug set into the order in which
// `tpatch reconcile` should run them. The expansion has two parts:
//
//  1. Transitive HARD-parent closure. If the operator asks to reconcile
//     `[child]` and child hard-depends on `parent`, parent must reconcile
//     first; if parent itself hard-depends on `grandparent`, grandparent
//     comes first too. Soft parents do NOT force themselves into the
//     closure — they are ordering hints, not gates (ADR-011 D4).
//
//  2. Deterministic topological order over the closure. We use
//     store.TopologicalOrder, which respects BOTH hard and soft edges
//     when sequencing nodes that are already in the closure (per the
//     handoff: "soft deps still contribute to ordering"). Tie-break is
//     lexicographic by slug for stable output.
//
// Flag gating (ADR-011 D9): RunReconcile only calls PlanReconcile when
// Config.DAGEnabled() is true. With the flag off, RunReconcile preserves
// pre-M14.3 input-order behaviour byte-for-byte.

package workflow

import (
	"fmt"

	"github.com/tesseracode/tesserapatch/internal/store"
)

// PlanReconcile returns the topologically-ordered slug list that
// RunReconcile should iterate when the dependency DAG is enabled.
//
// The returned slice is the transitive HARD-parent closure of the input
// slugs (parents that must be reconciled before any input slug can be
// reconciled meaningfully), plus the input slugs themselves. Soft parents
// of slugs already in the closure contribute to ordering but are NOT
// pulled into the closure transitively.
//
// Errors:
//   - wraps store.ErrCycle when the closure contains a cycle. The error
//     message includes the cycle path produced by store.DetectCycles so
//     operators can surface the offending edge in CLI output.
//   - returns an error if any input slug is unknown to the store.
//
// Authoritative source for parent state: this function reads
// FeatureStatus via store.LoadFeatureStatus. It does NOT consult any
// reconcile-session.json artifact (ADR-010 D5); planning is purely a
// function of the dependency declarations + which features exist.
func PlanReconcile(s *store.Store, slugs []string) ([]string, error) {
	if len(slugs) == 0 {
		return nil, nil
	}

	all, err := s.ListFeatures()
	if err != nil {
		return nil, fmt.Errorf("plan reconcile: list features: %w", err)
	}

	// Index every feature's dependency declaration. Any slug missing
	// from this map is unknown to the store (caller-error / typo).
	allDeps := make(map[string][]store.Dependency, len(all))
	for _, f := range all {
		allDeps[f.Slug] = f.DependsOn
	}

	for _, slug := range slugs {
		if _, ok := allDeps[slug]; !ok {
			return nil, fmt.Errorf("plan reconcile: unknown feature %q", slug)
		}
	}

	// Transitive HARD-parent closure. Walk parents iteratively; each
	// step expands the closure by hard parents only.
	closure := make(map[string]struct{}, len(slugs))
	stack := make([]string, 0, len(slugs))
	for _, slug := range slugs {
		closure[slug] = struct{}{}
		stack = append(stack, slug)
	}
	for len(stack) > 0 {
		head := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		for _, dep := range allDeps[head] {
			if dep.Kind != store.DependencyKindHard {
				continue
			}
			if _, seen := closure[dep.Slug]; seen {
				continue
			}
			if _, exists := allDeps[dep.Slug]; !exists {
				// Hard parent declared but not present in the store. The
				// dependency-gate logic surfaces this as a blocker; here
				// we silently skip it — topology cannot order an absent
				// node, and the gate will refuse the apply downstream.
				continue
			}
			closure[dep.Slug] = struct{}{}
			stack = append(stack, dep.Slug)
		}
	}

	// Build the focus subgraph: only nodes in the closure, with their
	// FULL dependency list (hard + soft). store.TopologicalOrder filters
	// dangling parents, so soft deps to nodes outside the closure are
	// dropped from ordering automatically.
	subgraph := make(map[string][]store.Dependency, len(closure))
	for slug := range closure {
		subgraph[slug] = allDeps[slug]
	}

	order, err := store.TopologicalOrder(subgraph)
	if err != nil {
		// Augment the topology error with the cycle path for operator
		// debugging. DetectCycles is cheap on a graph this small.
		if cycle, _ := store.DetectCycles(subgraph); len(cycle) > 0 {
			return nil, fmt.Errorf("plan reconcile: %w (cycle: %v)", err, cycle)
		}
		return nil, fmt.Errorf("plan reconcile: %w", err)
	}
	return order, nil
}
