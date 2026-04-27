// Implement-time `created_by` auto-inference (PRD §4.3.1, M15.1).
//
// This file is the IMPLEMENT-time advisory companion to the apply-time
// gate in created_by_gate.go. The gate enforces — at apply — that an op
// declaring a hard-parent `created_by` only runs once that parent has
// landed. The inference scanner here runs at IMPLEMENT time, *after*
// the recipe has been parsed but *before* it is written to disk, and
// *suggests* a `created_by` value when the operator forgot to set one.
//
// PRD §4.3.1 contract (authoritative):
//
//   - For each `replace-in-file` op in the recipe whose CreatedBy is
//     empty AND whose Search is non-empty, scan the child's hard
//     parents (status.json.depends_on, kind=="hard"). If exactly one
//     hard parent's artifacts/post-apply.patch contains the Search
//     bytes AND the same Search bytes are NOT already present in the
//     pristine working tree at op.Path, emit an advisory line on stderr
//     suggesting `created_by: <parent>`.
//
//   - The recipe is NEVER mutated. The operator reviews the advisory
//     and edits apply-recipe.json by hand. This preserves operator
//     authority per PRD §8 risk note.
//
//   - When DAGEnabled() is false, the scanner is inert (byte-identical
//     v0.5.x behaviour). When opted out via `--no-created-by-infer`
//     (ctx flag), the scanner short-circuits before any I/O.
//
// Soft parents are intentionally skipped (ADR-011 D4 — soft deps never
// gate apply, so a `created_by` suggestion against them would be
// misleading at apply time). Inference is also non-transitive: only
// direct parents declared in `depends_on` are scanned.
//
// Output channel: WarnWriter (stderr by default), so suggestions show
// up alongside the implement-phase progress without polluting stdout
// (which downstream tooling may parse).

package workflow

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"github.com/tesseracode/tesserapatch/internal/store"
)

// ctxKeyDisableCreatedByInfer is the context key for the
// `--no-created-by-infer` opt-out. Lives next to ctxKeyDisableRetry in
// retry.go's contextKey type so all workflow context keys share the
// same private type (one chokepoint per package).
const ctxKeyDisableCreatedByInfer contextKey = 1

// WithDisableCreatedByInference returns a context that suppresses the
// implement-time `created_by` advisory scan. Used by the
// `--no-created-by-infer` CLI flag.
func WithDisableCreatedByInference(ctx context.Context, disable bool) context.Context {
	return context.WithValue(ctx, ctxKeyDisableCreatedByInfer, disable)
}

func createdByInferenceDisabled(ctx context.Context) bool {
	v, _ := ctx.Value(ctxKeyDisableCreatedByInfer).(bool)
	return v
}

// inferCreatedBy walks `recipe.Operations`, scans each replace-in-file
// op with empty CreatedBy and non-empty Search, and writes advisory
// suggestion lines to WarnWriter when a unique hard-parent owner is
// found. The recipe value is read-only — never mutated.
//
// Returns nil on every non-fatal condition. Errors loading the child
// status are surfaced because they indicate a corrupted store and
// would cause the apply-time gate to fail anyway. Errors reading a
// parent's post-apply.patch are silently skipped (the parent may not
// have been applied yet — that's exactly the case we want to surface
// guidance for, but only when SOME parent matches).
func inferCreatedBy(ctx context.Context, s *store.Store, childSlug string, recipe ApplyRecipe) error {
	if createdByInferenceDisabled(ctx) {
		return nil
	}
	cfg, err := s.LoadConfig()
	if err != nil {
		return err
	}
	if !cfg.DAGEnabled() {
		// Flag-off invariant: byte-identical pre-v0.6 behaviour. No
		// I/O, no output.
		return nil
	}

	// Collect candidate ops up front so we can avoid loading the child
	// status when there is nothing to do (cheap fast-path).
	type candidate struct {
		index int
		op    RecipeOperation
	}
	var candidates []candidate
	for i, op := range recipe.Operations {
		if op.Type != "replace-in-file" {
			continue
		}
		if op.CreatedBy != "" {
			continue
		}
		if op.Search == "" {
			continue
		}
		candidates = append(candidates, candidate{index: i, op: op})
	}
	if len(candidates) == 0 {
		return nil
	}

	child, err := s.LoadFeatureStatus(childSlug)
	if err != nil {
		return fmt.Errorf("created_by inference: cannot load feature status for %q: %w", childSlug, err)
	}

	// Filter to hard parents (ADR-011 D4 — soft parents do not gate
	// apply, so a suggestion against them would be misleading).
	var hardParents []string
	for _, dep := range child.DependsOn {
		if dep.Kind == store.DependencyKindHard {
			hardParents = append(hardParents, dep.Slug)
		}
	}
	if len(hardParents) == 0 {
		return nil
	}

	// Read each hard parent's post-apply.patch once and cache the
	// bytes — the same patch may be checked against many ops in a
	// large recipe.
	parentPatches := make(map[string][]byte, len(hardParents))
	for _, p := range hardParents {
		raw, rerr := s.ReadFeatureFile(p, "artifacts/post-apply.patch")
		if rerr != nil {
			// Parent hasn't been applied yet (or patch capture
			// failed) — skip silently. We can't infer ownership
			// without the patch, but the apply-time gate will
			// surface the real issue.
			continue
		}
		parentPatches[p] = []byte(raw)
	}
	if len(parentPatches) == 0 {
		return nil
	}

	suggestions := 0
	for _, c := range candidates {
		// (a) If the Search text is already present in the pristine
		// working tree, no inference is needed — the op will resolve
		// against the working file.
		if pristineHasSearch(s.Root, c.op.Path, c.op.Search) {
			continue
		}

		// (b) Find every hard parent whose post-apply.patch contains
		// the Search bytes.
		var matches []string
		searchBytes := []byte(c.op.Search)
		for _, p := range hardParents {
			patch, ok := parentPatches[p]
			if !ok {
				continue
			}
			if bytes.Contains(patch, searchBytes) {
				matches = append(matches, p)
			}
		}
		sort.Strings(matches) // deterministic output

		switch len(matches) {
		case 0:
			// Silent — no parent claims this text. The eventual
			// apply-time gate (or the bare not-found error) will
			// surface the real problem.
		case 1:
			fmt.Fprintf(WarnWriter,
				"ℹ created_by inference: op #%d (replace-in-file %s) → suggest created_by: %s\n"+
					"   reason: Search text is missing in pristine checkout but present in %s/artifacts/post-apply.patch\n",
				c.index, c.op.Path, matches[0], matches[0])
			suggestions++
		default:
			fmt.Fprintf(WarnWriter,
				"ℹ created_by inference: op #%d (replace-in-file %s) → ambiguous: multiple hard parents contain this text: %v; please set created_by manually\n",
				c.index, c.op.Path, matches)
		}
	}

	if suggestions > 0 {
		fmt.Fprintf(WarnWriter,
			"ℹ %d created_by suggestion(s) — review and edit apply-recipe.json before tpatch apply\n",
			suggestions)
	}

	return nil
}

// pristineHasSearch reports whether the file at `repoRoot/relPath`
// exists and contains `search` as a byte substring. A missing file is
// not an error — that's the case where we DO want to scan parents.
func pristineHasSearch(repoRoot, relPath, search string) bool {
	if relPath == "" || search == "" {
		return false
	}
	full := filepath.Join(repoRoot, relPath)
	data, err := os.ReadFile(full)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false
		}
		// Any other read error (permissions, IO) — be conservative
		// and treat it as "not found" so we still try the parent
		// scan. The apply-time gate will catch the real I/O issue.
		return false
	}
	return bytes.Contains(data, []byte(search))
}
