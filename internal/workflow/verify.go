package workflow

// Slice A of the freshness-overlay design (PRD-verify-freshness §9, ADR-013).
//
// Scope:
//   - `tpatch verify <slug>` cobra shell wires through `RunVerify`.
//   - V0 (status_loaded), V1 (intent_files_present), V2 (recipe_parses /
//     recipe_op_targets_resolve) are real implementations.
//   - V3–V9 are stubs that return passed: true, skipped: true with a
//     reason naming the slice that will land them. The full 10-check
//     array is emitted on `--json` stdout so the report shape is
//     reviewable in Slice A.
//   - The persisted `Verify` record carries only `verified_at`, `passed`,
//     `recipe_hash_at_verify`, `patch_hash_at_verify`, `parent_snapshot`
//     (Reviewer Note 1, M15-W3 APPROVED WITH NOTES at 3c122aa). The full
//     check array is built in-memory only.
//   - V7/V8 closure-replay and the freshness-derivation `ComposeLabels`
//     hook are deliberately deferred to Slices C / B respectively.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/tesseracode/tesserapatch/internal/store"
)

// Verify check IDs. Frozen vocabulary; consumers may switch on these.
const (
	CheckStatusLoaded               = "status_loaded"
	CheckIntentFilesPresent         = "intent_files_present"
	CheckRecipeParses               = "recipe_parses"
	CheckRecipeOpTargetsResolve     = "recipe_op_targets_resolve"
	CheckDepMetadataValid           = "dep_metadata_valid"
	CheckSatisfiedByReachable       = "satisfied_by_reachable"
	CheckDependencyGateSatisfied    = "dependency_gate_satisfied"
	CheckRecipeReplayClean          = "recipe_replay_clean"
	CheckPostApplyPatchReplayClean  = "post_apply_patch_replay_clean"
	CheckReconcileOutcomeConsistent = "reconcile_outcome_consistent"
)

// Severity vocabulary (ADR-013 / PRD §3.2).
const (
	SeverityBlock      = "block"
	SeverityBlockAbort = "block-abort"
	SeverityWarn       = "warn"
)

// verifySchemaVersion is the PRD §4.3 schema_version field for the
// `--json` report. Bumping is a breaking change for harness consumers.
const verifySchemaVersion = "1.0"

// VerifyOptions controls a single `RunVerify` invocation.
type VerifyOptions struct {
	NoWrite bool // when true, skip persistence of the Verify record.
}

// VerifyReport is the full in-memory result of a verify run. The
// `Checks` field carries all ten check rows; `Persisted` carries the
// minimal field set actually written to status.json (Reviewer Note 1).
type VerifyReport struct {
	SchemaVersion      string                        `json:"schema_version"`
	Slug               string                        `json:"slug"`
	VerifiedAt         string                        `json:"verified_at"`
	Verdict            string                        `json:"verdict"` // "passed" | "failed"
	ExitCode           int                           `json:"exit_code"`
	Checks             []store.VerifyCheckResult     `json:"checks"`
	LifecycleState     store.FeatureState            `json:"lifecycle_state"`
	RecipeHashAtVerify string                        `json:"recipe_hash_at_verify,omitempty"`
	PatchHashAtVerify  string                        `json:"patch_hash_at_verify,omitempty"`
	ParentSnapshot     map[string]store.FeatureState `json:"parent_snapshot,omitempty"`

	// Persisted is the trimmed record that gets written to status.json.
	// It is NOT a separate JSON field on the report — RunVerify uses it
	// internally to call `store.WriteVerifyRecord`. Tests inspect it
	// directly.
	Persisted store.VerifyRecord `json:"-"`
}

// RunVerify executes the Slice A check set against the named feature
// and (unless opts.NoWrite is true) persists the freshness overlay via
// `store.WriteVerifyRecord`. Returns the in-memory report regardless of
// pass/fail; only structural failures (slug missing, status load error
// in V0) surface as a Go error — and even then a report is produced
// where possible.
func RunVerify(s *store.Store, slug string, opts VerifyOptions) (*VerifyReport, error) {
	if strings.TrimSpace(slug) == "" {
		return nil, errors.New("verify requires a feature slug")
	}

	report := &VerifyReport{
		SchemaVersion: verifySchemaVersion,
		Slug:          slug,
		VerifiedAt:    time.Now().UTC().Format(time.RFC3339),
		Checks:        make([]store.VerifyCheckResult, 0, 10),
	}

	// V0 — status_loaded (severity: block-abort). If this fails the
	// rest of the run aborts because we have no FeatureStatus to read.
	status, err := s.LoadFeatureStatus(slug)
	if err != nil {
		report.Checks = append(report.Checks, store.VerifyCheckResult{
			ID:          CheckStatusLoaded,
			Severity:    SeverityBlockAbort,
			Passed:      false,
			Remediation: fmt.Sprintf("could not load status.json: %v", err),
		})
		// Append the remaining nine stubs so the JSON shape is stable.
		for _, c := range stubChecksAfterAbort() {
			report.Checks = append(report.Checks, c)
		}
		report.Verdict = "failed"
		report.ExitCode = 2
		// Cannot persist without a status.json — return the report and
		// the error.
		return report, fmt.Errorf("verify aborted: %w", err)
	}
	report.Checks = append(report.Checks, store.VerifyCheckResult{
		ID:       CheckStatusLoaded,
		Severity: SeverityBlockAbort,
		Passed:   true,
	})
	report.LifecycleState = status.State

	// V1 — intent_files_present (severity: block). Slice A contract:
	// `spec.md` is the canonical intent file. `request.md` is created
	// by `tpatch add` and is part of the intent surface as well.
	report.Checks = append(report.Checks, checkIntentFilesPresent(s, slug))

	// V2 — split into two real checks: recipe_parses and
	// recipe_op_targets_resolve. Note 2 contract: an absent recipe is
	// `passed: true, skipped: true, reason: "..."` — never a false fail.
	parseCheck, opCheck, recipe, recipeBytes := checkRecipe(s, slug)
	report.Checks = append(report.Checks, parseCheck, opCheck)

	// V3–V9 stubs.
	report.Checks = append(report.Checks, stubV3toV9()...)

	// Hashes for the persisted record.
	report.RecipeHashAtVerify = sha256Hex(recipeBytes)
	report.PatchHashAtVerify = sha256Hex(readArtifactBytes(s, slug, "post-apply.patch"))

	// Parent snapshot: iterate hard deps and read each parent's
	// FeatureState.
	report.ParentSnapshot = parentSnapshot(s, status)

	// Verdict: failed if any non-skipped, non-warn check failed.
	report.Verdict, report.ExitCode = computeVerdict(report.Checks)

	report.Persisted = store.VerifyRecord{
		VerifiedAt:         report.VerifiedAt,
		Passed:             report.Verdict == "passed",
		RecipeHashAtVerify: report.RecipeHashAtVerify,
		PatchHashAtVerify:  report.PatchHashAtVerify,
		ParentSnapshot:     report.ParentSnapshot,
	}

	if !opts.NoWrite {
		if err := s.WriteVerifyRecord(slug, report.Persisted); err != nil {
			return report, fmt.Errorf("verify ran but persistence failed: %w", err)
		}
	}

	// Use `recipe` only to suppress the unused-var warning when no
	// fields are read — it carries semantic intent for future slices.
	_ = recipe

	return report, nil
}

// WriteJSONReport emits the report to w with stable indentation.
func (r *VerifyReport) WriteJSONReport(w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}

// WriteHumanReport emits a brief per-check summary suitable for stderr.
func (r *VerifyReport) WriteHumanReport(w io.Writer) {
	fmt.Fprintf(w, "verify %s — %s\n", r.Slug, r.Verdict)
	for _, c := range r.Checks {
		marker := "✓"
		switch {
		case c.Skipped:
			marker = "⊘"
		case !c.Passed:
			marker = "✗"
		}
		line := fmt.Sprintf("  %s [%s] %s", marker, c.Severity, c.ID)
		if c.Skipped && c.Reason != "" {
			line += " — " + c.Reason
		}
		if !c.Passed && c.Remediation != "" {
			line += " — " + c.Remediation
		}
		fmt.Fprintln(w, line)
	}
}

// ── Real checks ──────────────────────────────────────────────────────────

func checkIntentFilesPresent(s *store.Store, slug string) store.VerifyCheckResult {
	specPath := filepath.Join(s.Root, ".tpatch", "features", slug, "spec.md")
	info, err := os.Stat(specPath)
	if err != nil {
		return store.VerifyCheckResult{
			ID:          CheckIntentFilesPresent,
			Severity:    SeverityBlock,
			Passed:      false,
			Remediation: fmt.Sprintf("spec.md missing for %s — run `tpatch define %s` first", slug, slug),
		}
	}
	if info.Size() == 0 {
		return store.VerifyCheckResult{
			ID:          CheckIntentFilesPresent,
			Severity:    SeverityBlock,
			Passed:      false,
			Remediation: fmt.Sprintf("spec.md is empty for %s — re-run `tpatch define %s`", slug, slug),
		}
	}
	return store.VerifyCheckResult{
		ID:       CheckIntentFilesPresent,
		Severity: SeverityBlock,
		Passed:   true,
	}
}

// checkRecipe runs the V2 split: parse + op-target resolution. Returns
// the parsed recipe and its raw bytes for hashing. An absent recipe is
// `passed: true, skipped: true` (Reviewer Note 2).
func checkRecipe(s *store.Store, slug string) (parse, ops store.VerifyCheckResult, recipe ApplyRecipe, raw []byte) {
	recipePath := filepath.Join(s.Root, ".tpatch", "features", slug, "artifacts", "apply-recipe.json")
	data, err := os.ReadFile(recipePath)
	if err != nil {
		if os.IsNotExist(err) {
			skipReason := "no apply-recipe.json (legacy / pre-autogen-era feature)"
			parse = store.VerifyCheckResult{
				ID:       CheckRecipeParses,
				Severity: SeverityBlock,
				Passed:   true,
				Skipped:  true,
				Reason:   skipReason,
			}
			ops = store.VerifyCheckResult{
				ID:       CheckRecipeOpTargetsResolve,
				Severity: SeverityBlock,
				Passed:   true,
				Skipped:  true,
				Reason:   skipReason,
			}
			return parse, ops, ApplyRecipe{}, nil
		}
		// Read error other than ENOENT — surface as a parse failure.
		parse = store.VerifyCheckResult{
			ID:          CheckRecipeParses,
			Severity:    SeverityBlock,
			Passed:      false,
			Remediation: fmt.Sprintf("cannot read apply-recipe.json: %v", err),
		}
		ops = store.VerifyCheckResult{
			ID:       CheckRecipeOpTargetsResolve,
			Severity: SeverityBlock,
			Passed:   true,
			Skipped:  true,
			Reason:   "skipped: recipe could not be read",
		}
		return parse, ops, ApplyRecipe{}, nil
	}

	if jsonErr := json.Unmarshal(data, &recipe); jsonErr != nil {
		parse = store.VerifyCheckResult{
			ID:          CheckRecipeParses,
			Severity:    SeverityBlock,
			Passed:      false,
			Remediation: fmt.Sprintf("apply-recipe.json does not parse as JSON: %v", jsonErr),
		}
		ops = store.VerifyCheckResult{
			ID:       CheckRecipeOpTargetsResolve,
			Severity: SeverityBlock,
			Passed:   true,
			Skipped:  true,
			Reason:   "skipped: recipe did not parse",
		}
		return parse, ops, ApplyRecipe{}, data
	}

	parse = store.VerifyCheckResult{
		ID:       CheckRecipeParses,
		Severity: SeverityBlock,
		Passed:   true,
	}

	// Op-target resolution: every op's `path` resolves to a present file
	// in the working tree. `write-file` and `ensure-directory` ops may
	// legitimately reference paths that do not yet exist (they create);
	// `replace-in-file` and `append-file` require the target to exist
	// at verify time.
	var failures []string
	for i, op := range recipe.Operations {
		switch op.Type {
		case "replace-in-file", "append-file":
			target := filepath.Join(s.Root, op.Path)
			if _, statErr := os.Stat(target); statErr != nil {
				failures = append(failures, fmt.Sprintf("op #%d (%s) target %q does not resolve: %v", i, op.Type, op.Path, statErr))
			}
		}
	}
	if len(failures) > 0 {
		ops = store.VerifyCheckResult{
			ID:          CheckRecipeOpTargetsResolve,
			Severity:    SeverityBlock,
			Passed:      false,
			Remediation: strings.Join(failures, "; "),
		}
		return parse, ops, recipe, data
	}
	ops = store.VerifyCheckResult{
		ID:       CheckRecipeOpTargetsResolve,
		Severity: SeverityBlock,
		Passed:   true,
	}
	return parse, ops, recipe, data
}

// ── Stubs ────────────────────────────────────────────────────────────────

func stubChecksAfterAbort() []store.VerifyCheckResult {
	// Used when V0 fails: we still emit the remaining nine entries so
	// the report shape is byte-stable for harness consumers.
	out := make([]store.VerifyCheckResult, 0, 9)
	for _, id := range []string{
		CheckIntentFilesPresent,
		CheckRecipeParses,
		CheckRecipeOpTargetsResolve,
	} {
		out = append(out, store.VerifyCheckResult{
			ID:       id,
			Severity: SeverityBlock,
			Passed:   true,
			Skipped:  true,
			Reason:   "skipped: V0 (status_loaded) aborted the run",
		})
	}
	out = append(out, stubV3toV9()...)
	return out
}

func stubV3toV9() []store.VerifyCheckResult {
	return []store.VerifyCheckResult{
		{ID: CheckDepMetadataValid, Severity: SeverityBlock, Passed: true, Skipped: true, Reason: "not yet implemented (Slice C)"},
		{ID: CheckSatisfiedByReachable, Severity: SeverityBlock, Passed: true, Skipped: true, Reason: "not yet implemented (Slice C)"},
		{ID: CheckDependencyGateSatisfied, Severity: SeverityWarn, Passed: true, Skipped: true, Reason: "not yet implemented (Slice C)"},
		{ID: CheckRecipeReplayClean, Severity: SeverityBlock, Passed: true, Skipped: true, Reason: "not yet implemented (Slice C — closure replay)"},
		{ID: CheckPostApplyPatchReplayClean, Severity: SeverityBlock, Passed: true, Skipped: true, Reason: "not yet implemented (Slice C — closure replay)"},
		{ID: CheckReconcileOutcomeConsistent, Severity: SeverityWarn, Passed: true, Skipped: true, Reason: "not yet implemented (Slice C)"},
	}
}

// ── Helpers ──────────────────────────────────────────────────────────────

func sha256Hex(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func readArtifactBytes(s *store.Store, slug, name string) []byte {
	p := filepath.Join(s.Root, ".tpatch", "features", slug, "artifacts", name)
	data, err := os.ReadFile(p)
	if err != nil {
		return nil
	}
	return data
}

// parentSnapshot returns a deterministic map of parent slug → current
// FeatureState for every hard dep declared on `status`. Soft deps are
// not part of the snapshot — the freshness overlay only tracks the
// closure that the apply gate enforces (ADR-013 D5).
//
// Missing parents (slug declared as a hard dep but no
// `.tpatch/features/<slug>/` on disk — typo, manual deletion, never
// created) are omitted from the map entirely. Recording an empty
// string would not be a valid FeatureState enum and would defer a
// crash to the freshness derivation's satisfies_state_or_better
// comparison. Detecting a structurally missing parent is the job of
// `tpatch status` / dependency validation, not the freshness layer.
//
// Note on shape: the field is tagged `omitempty`, so an empty result
// (zero hard deps, or all hard parents missing) serializes as an
// absent key rather than `"parent_snapshot": {}`. We return nil in
// that case to keep the JSON byte-identical to the never-verified
// baseline (ADR-013 D4).
func parentSnapshot(s *store.Store, status store.FeatureStatus) map[string]store.FeatureState {
	if len(status.DependsOn) == 0 {
		return nil
	}
	keys := make([]string, 0, len(status.DependsOn))
	for _, dep := range status.DependsOn {
		if dep.Kind != store.DependencyKindHard {
			continue
		}
		keys = append(keys, dep.Slug)
	}
	sort.Strings(keys)
	out := map[string]store.FeatureState{}
	for _, slug := range keys {
		ps, err := s.LoadFeatureStatus(slug)
		if err != nil {
			// Parent missing — omit from snapshot. See function doc.
			continue
		}
		out[slug] = ps.State
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// computeVerdict mirrors PRD §3.2 + §4.3: any non-skipped check whose
// severity is `block` or `block-abort` and `passed=false` flips the
// verdict to "failed" with exit 2. Warn-severity failures do not change
// the verdict.
func computeVerdict(checks []store.VerifyCheckResult) (string, int) {
	for _, c := range checks {
		if c.Skipped {
			continue
		}
		if c.Passed {
			continue
		}
		if c.Severity == SeverityBlock || c.Severity == SeverityBlockAbort {
			return "failed", 2
		}
	}
	return "passed", 0
}
