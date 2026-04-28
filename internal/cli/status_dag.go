// status --dag rendering (M14.4 / Chunk A).
//
// Renders the feature dependency DAG as an ASCII tree, or as JSON for
// harness consumption. Reads only FeatureStatus.Reconcile.Outcome via
// store.LoadFeatureStatus — never artifacts/reconcile-session.json
// (ADR-010 D5 + ADR-011 source-truth guard).
//
// Hard deps render as `─►`; soft deps render as `┄►`. Each node prints
// `<slug> [<state>] <effective-outcome>` with composable labels suffixed
// in `(label, label)`.
//
// Cycle-safe: the renderer uses DetectCycles + a per-walk visited set so
// it can never recurse infinitely on a malformed graph. When cycles are
// present the tree falls back to a flat slug list and the cycle path is
// surfaced as a warning.
//
// Determinism: roots and child arrays are sorted by slug so output is
// stable across runs (golden-test friendly).

package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tesseracode/tesserapatch/internal/store"
	"github.com/tesseracode/tesserapatch/internal/workflow"
)

// dagJSONNode is the per-feature record emitted by `status --dag --json`.
type dagJSONNode struct {
	Slug           string                 `json:"slug"`
	State          store.FeatureState     `json:"state"`
	Outcome        string                 `json:"outcome,omitempty"`
	Effective      string                 `json:"effective_outcome,omitempty"`
	Labels         []store.ReconcileLabel `json:"labels,omitempty"`
	FreshnessLabel store.ReconcileLabel   `json:"freshness_label,omitempty"`
	Verify         *store.VerifyRecord    `json:"verify,omitempty"`
	DependsOn      []dagJSONEdge          `json:"depends_on,omitempty"`
	Dependents     []dagJSONEdge          `json:"dependents,omitempty"`
}

type dagJSONEdge struct {
	Slug        string `json:"slug"`
	Kind        string `json:"kind"`
	SatisfiedBy string `json:"satisfied_by,omitempty"`
}

// dagChildEdge is the per-parent adjacency entry used by the tree
// renderer. The kind comes from the *child's* depends_on declaration.
type dagChildEdge struct {
	slug string
	kind string
}

// dagJSONPayload is the top-level shape for `--dag --json`.
type dagJSONPayload struct {
	Scope    string        `json:"scope"`
	Features []dagJSONNode `json:"features"`
	Roots    []string      `json:"roots"`
	Warnings []string      `json:"warnings,omitempty"`
	Cycle    []string      `json:"cycle,omitempty"`
}

// runStatusDAG is the entry point invoked from statusCmd when --dag is set.
// scopeSlug "" means "render whole DAG"; non-empty narrows to that feature
// plus its full transitive parent and child sets.
func runStatusDAG(out io.Writer, s *store.Store, scopeSlug string, asJSON bool) error {
	features, err := s.ListFeatures()
	if err != nil {
		return err
	}
	if scopeSlug != "" {
		// Validate the slug exists before scoping.
		if _, err := s.LoadFeatureStatus(scopeSlug); err != nil {
			return fmt.Errorf("feature %q not found", scopeSlug)
		}
	}

	graph := make(map[string][]store.Dependency, len(features))
	byslug := make(map[string]store.FeatureStatus, len(features))
	for _, f := range features {
		graph[f.Slug] = f.DependsOn
		byslug[f.Slug] = f
	}

	cyc, _ := store.DetectCycles(graph)
	scoped := scopeSet(graph, scopeSlug)

	if asJSON {
		return writeDAGJSON(out, s, features, byslug, graph, scoped, scopeSlug, cyc)
	}
	return writeDAGTree(out, s, features, byslug, graph, scoped, scopeSlug, cyc)
}

// scopeSet returns the slug set to render. When scopeSlug == "" it
// returns nil (meaning "every feature is in scope"). Otherwise the
// returned set is the union of scopeSlug, all transitive parents, and
// all transitive children — visited via BFS so cycles cannot loop.
func scopeSet(graph map[string][]store.Dependency, scopeSlug string) map[string]struct{} {
	if scopeSlug == "" {
		return nil
	}
	out := map[string]struct{}{scopeSlug: {}}

	// Upward (parents): follow depends_on.
	queue := []string{scopeSlug}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, d := range graph[cur] {
			if _, seen := out[d.Slug]; seen {
				continue
			}
			out[d.Slug] = struct{}{}
			queue = append(queue, d.Slug)
		}
	}

	// Downward (children): reverse adjacency.
	rev := make(map[string][]string, len(graph))
	for child, deps := range graph {
		for _, d := range deps {
			rev[d.Slug] = append(rev[d.Slug], child)
		}
	}
	queue = []string{scopeSlug}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, child := range rev[cur] {
			if _, seen := out[child]; seen {
				continue
			}
			out[child] = struct{}{}
			queue = append(queue, child)
		}
	}
	return out
}

// inScope reports whether slug should be rendered.
func inScope(scoped map[string]struct{}, slug string) bool {
	if scoped == nil {
		return true
	}
	_, ok := scoped[slug]
	return ok
}

// writeDAGTree renders the ASCII tree.
func writeDAGTree(
	out io.Writer,
	s *store.Store,
	features []store.FeatureStatus,
	byslug map[string]store.FeatureStatus,
	graph map[string][]store.Dependency,
	scoped map[string]struct{},
	scopeSlug string,
	cycle []string,
) error {
	if scopeSlug != "" {
		fmt.Fprintf(out, "DAG (scope: %s)\n", scopeSlug)
	} else {
		fmt.Fprintln(out, "DAG (all features)")
	}

	// Pre-compute child adjacency in render order (sorted by slug). Each
	// entry includes the kind from the child's depends_on declaration.
	children := make(map[string][]dagChildEdge, len(graph))
	for child, deps := range graph {
		for _, d := range deps {
			children[d.Slug] = append(children[d.Slug], dagChildEdge{slug: child, kind: d.Kind})
		}
	}
	for parent := range children {
		sort.Slice(children[parent], func(i, j int) bool {
			return children[parent][i].slug < children[parent][j].slug
		})
	}

	// Cycle handling: when DetectCycles returned a path, fall back to a
	// flat slug list and surface the warning. Don't attempt to recurse —
	// the caller may have a malformed graph and we must never hang.
	if len(cycle) > 0 {
		fmt.Fprintf(out, "  ⚠ cycle detected: %s\n", strings.Join(cycle, " -> "))
		fmt.Fprintln(out, "  (showing flat list; resolve cycle to render tree)")
		slugs := make([]string, 0, len(features))
		for _, f := range features {
			if !inScope(scoped, f.Slug) {
				continue
			}
			slugs = append(slugs, f.Slug)
		}
		sort.Strings(slugs)
		for _, sl := range slugs {
			fmt.Fprintf(out, "  - %s\n", renderNodeLine(s, byslug[sl]))
		}
		return nil
	}

	// Roots: in-scope slugs whose parents are entirely out-of-scope (or
	// who have no parents at all). For the unscoped render this is the
	// natural "no depends_on" set; for a scoped render rooted at a slug
	// the upward chain becomes the single root for that subtree.
	roots := computeRoots(features, graph, scoped)
	if len(roots) == 0 && scoped != nil {
		// Defensive — scopeSet always includes scopeSlug so we should
		// always have at least one root. But if not, list flat.
		fmt.Fprintln(out, "  (no root nodes in scope)")
		return nil
	}
	if len(roots) == 0 {
		fmt.Fprintln(out, "  (no features)")
		return nil
	}

	visited := make(map[string]struct{}, len(features))
	for _, root := range roots {
		walkTree(out, s, byslug, children, scoped, visited, root, "", "", true)
	}

	// Surface unreached in-scope slugs (e.g. orphan children of out-of-
	// scope parents) so the user sees every feature exactly once.
	var orphans []string
	for _, f := range features {
		if !inScope(scoped, f.Slug) {
			continue
		}
		if _, ok := visited[f.Slug]; !ok {
			orphans = append(orphans, f.Slug)
		}
	}
	sort.Strings(orphans)
	if len(orphans) > 0 {
		fmt.Fprintln(out, "")
		fmt.Fprintln(out, "Orphans (parents out of scope):")
		for _, o := range orphans {
			fmt.Fprintf(out, "  - %s\n", renderNodeLine(s, byslug[o]))
		}
	}
	return nil
}

// computeRoots returns the slugs to start tree rendering from. A slug is
// a root when it is in scope and has no in-scope parents.
func computeRoots(features []store.FeatureStatus, graph map[string][]store.Dependency, scoped map[string]struct{}) []string {
	var roots []string
	for _, f := range features {
		if !inScope(scoped, f.Slug) {
			continue
		}
		hasInScopeParent := false
		for _, d := range graph[f.Slug] {
			if inScope(scoped, d.Slug) {
				hasInScopeParent = true
				break
			}
		}
		if !hasInScopeParent {
			roots = append(roots, f.Slug)
		}
	}
	sort.Strings(roots)
	return roots
}

// walkTree renders one node and recurses into its children, indenting
// with `prefix`. `connector` is the inbound arrow (e.g. "─► " for hard,
// "┄► " for soft) — empty for the root. `last` controls whether the
// branch glyph is `└` vs `├`. Cycle-safe via the visited set: a slug is
// printed once; subsequent visits show "(...already shown...)".
func walkTree(
	out io.Writer,
	s *store.Store,
	byslug map[string]store.FeatureStatus,
	children map[string][]dagChildEdge,
	scoped map[string]struct{},
	visited map[string]struct{},
	slug, prefix, connector string,
	last bool,
) {
	st, ok := byslug[slug]
	if !ok {
		fmt.Fprintf(out, "%s%s%s (missing)\n", prefix, connector, slug)
		return
	}
	if _, seen := visited[slug]; seen {
		fmt.Fprintf(out, "%s%s%s (already shown above)\n", prefix, connector, slug)
		return
	}
	visited[slug] = struct{}{}

	fmt.Fprintf(out, "%s%s%s\n", prefix, connector, renderNodeLine(s, st))

	kids := children[slug]
	// Determine in-scope kids and preserve order.
	inScopeKids := kids[:0:0]
	for _, k := range kids {
		if inScope(scoped, k.slug) {
			inScopeKids = append(inScopeKids, k)
		}
	}
	for i, k := range inScopeKids {
		isLast := i == len(inScopeKids)-1
		var branch, childPrefix string
		if isLast {
			branch = "└"
			childPrefix = prefix + "  "
		} else {
			branch = "├"
			childPrefix = prefix + "│ "
		}
		// Connector reflects edge kind (hard vs soft).
		arrow := "─► "
		if k.kind == store.DependencyKindSoft {
			arrow = "┄► "
		}
		walkTree(out, s, byslug, children, scoped, visited, k.slug, childPrefix, branch+arrow, isLast)
	}
	_ = last
}

// renderNodeLine formats one feature node: `slug [state] outcome (labels)`.
// The `s` parameter is used to derive the read-time freshness label
// (Slice B / ADR-013 D5). Pass nil to skip freshness derivation (e.g.
// in tests that don't care about the freshness overlay).
func renderNodeLine(s *store.Store, st store.FeatureStatus) string {
	var freshness store.ReconcileLabel
	if s != nil {
		freshness = workflow.DeriveFreshnessLabel(s, st)
	}
	return renderNodeLineWithFreshness(st, freshness)
}

// renderNodeLineWithFreshness is the freshness-aware variant. Callers
// that have a *store.Store available pass the derived freshness label
// in; callers without one pass "".
func renderNodeLineWithFreshness(st store.FeatureStatus, freshness store.ReconcileLabel) string {
	out := fmt.Sprintf("%s [%s]", st.Slug, st.State)
	if eff := st.Reconcile.EffectiveOutcome(); eff != "" {
		out += " " + eff
	}
	labels := mergedLabels(st, freshness)
	if len(labels) > 0 {
		strs := make([]string, len(labels))
		for i, l := range labels {
			strs[i] = string(l)
		}
		out += " (" + strings.Join(strs, ", ") + ")"
	}
	return out
}

// mergedLabels returns the persisted M14.3 set + the freshness label
// (if non-empty), sorted alphabetically. The freshness label is NOT
// persisted (D4) — render-layer callers compose it on demand.
func mergedLabels(st store.FeatureStatus, freshness store.ReconcileLabel) []store.ReconcileLabel {
	if freshness == "" && len(st.Reconcile.Labels) == 0 {
		return nil
	}
	seen := make(map[store.ReconcileLabel]struct{}, len(st.Reconcile.Labels)+1)
	out := make([]store.ReconcileLabel, 0, len(st.Reconcile.Labels)+1)
	for _, l := range st.Reconcile.Labels {
		if _, ok := seen[l]; ok {
			continue
		}
		seen[l] = struct{}{}
		out = append(out, l)
	}
	if freshness != "" {
		if _, ok := seen[freshness]; !ok {
			out = append(out, freshness)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// writeDAGJSON renders the JSON shape consumed by harnesses and tests.
func writeDAGJSON(
	out io.Writer,
	s *store.Store,
	features []store.FeatureStatus,
	byslug map[string]store.FeatureStatus,
	graph map[string][]store.Dependency,
	scoped map[string]struct{},
	scopeSlug string,
	cycle []string,
) error {
	scope := "all"
	if scopeSlug != "" {
		scope = scopeSlug
	}
	payload := dagJSONPayload{Scope: scope}

	// dependents map: parent → []child edges.
	dependents := make(map[string][]dagJSONEdge, len(graph))
	for child, deps := range graph {
		for _, d := range deps {
			dependents[d.Slug] = append(dependents[d.Slug], dagJSONEdge{Slug: child, Kind: d.Kind})
		}
	}
	for k := range dependents {
		sort.Slice(dependents[k], func(i, j int) bool {
			return dependents[k][i].Slug < dependents[k][j].Slug
		})
	}

	for _, f := range features {
		if !inScope(scoped, f.Slug) {
			continue
		}
		// Slice B (ADR-013 D5): derive the freshness label at render
		// time and merge it into the labels array. Persisted
		// Reconcile.Labels carries only M14.3 entries.
		freshness := workflow.DeriveFreshnessLabel(s, f)
		node := dagJSONNode{
			Slug:           f.Slug,
			State:          f.State,
			Outcome:        string(f.Reconcile.Outcome),
			Effective:      f.Reconcile.EffectiveOutcome(),
			Labels:         mergedLabels(f, freshness),
			FreshnessLabel: freshness,
			Verify:         f.Verify,
		}
		for _, d := range f.DependsOn {
			node.DependsOn = append(node.DependsOn, dagJSONEdge{
				Slug:        d.Slug,
				Kind:        d.Kind,
				SatisfiedBy: d.SatisfiedBy,
			})
		}
		for _, e := range dependents[f.Slug] {
			node.Dependents = append(node.Dependents, e)
		}
		payload.Features = append(payload.Features, node)
	}

	payload.Roots = computeRoots(features, graph, scoped)
	if len(cycle) > 0 {
		payload.Cycle = cycle
		payload.Warnings = append(payload.Warnings, fmt.Sprintf("cycle detected: %s", strings.Join(cycle, " -> ")))
	}

	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "%s\n", data)
	return nil
}

// statusDagFlagWired is the boolean flag added to statusCmd. Helper kept
// in this file so the wiring stays adjacent to the renderer.
func wireStatusDagFlag(cmd *cobra.Command) {
	cmd.Flags().Bool("dag", false, "Render the feature dependency DAG (ASCII tree)")
}
