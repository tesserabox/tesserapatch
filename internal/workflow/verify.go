package workflow

// Slice A of the freshness-overlay design (PRD-verify-freshness §9, ADR-013).
//
// Scope:
//   - `tpatch verify <slug>` cobra shell wires through `RunVerify`.
//   - V0 (status_loaded), V1 (intent_files_present), V2 (recipe_parses)
//     are the only real implementations in Slice A.
//   - V3 (recipe_op_targets_resolve) and V4–V9 are stubs that return
//     passed: true, skipped: true with a reason naming the slice that
//     will land them (Slice C for V3–V9). The full 10-check array is
//     emitted on `--json` stdout so the report shape is reviewable in
//     Slice A.
//   - The persisted `Verify` record carries only `verified_at`, `passed`,
//     `recipe_hash_at_verify`, `patch_hash_at_verify`, `parent_snapshot`
//     (Reviewer Note 1, M15-W3 APPROVED WITH NOTES at 3c122aa). The full
//     check array is built in-memory only.
//   - V7/V8 closure-replay and the freshness-derivation `ComposeLabels`
//     hook are deliberately deferred to Slices C / B respectively.

import (
	"bytes"
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

// RefusedError is returned by RunVerify when the feature's lifecycle
// state is one of the pre-apply / mid-flight states for which verify
// has nothing meaningful to assert (PRD-verify-freshness §3.4.5 + §5).
// The CLI maps this to exit code 2 via ExitCodeError. RunVerify must
// NOT persist a freshness record on this path.
type RefusedError struct {
	Slug   string
	State  store.FeatureState
	Reason string
}

func (e *RefusedError) Error() string {
	if e == nil {
		return ""
	}
	return e.Reason
}

// IsRefused reports whether err is a *RefusedError.
func IsRefused(err error) bool {
	var r *RefusedError
	return errors.As(err, &r)
}

// postApplyVerifyStates is the set of lifecycle states for which
// `tpatch verify` is allowed to run. Any state outside this set is
// refused per PRD §5 ("feature is pre-apply, nothing to verify"). The
// freshness overlay is meaningful only after `apply` has produced the
// recipe + patch artifacts the checks operate on.
//
// `blocked` is allowed because the apply attempt has completed (the
// blocker is downstream); `upstream_merged` is allowed because the
// artifacts may still be inspectable post-retirement.
func postApplyVerifyStates() map[store.FeatureState]bool {
	return map[store.FeatureState]bool{
		store.StateApplied:        true,
		store.StateActive:         true,
		store.StateUpstreamMerged: true,
		store.StateBlocked:        true,
	}
}

// VerifyReport is the full in-memory result of a verify run. The
// `Checks` field carries all ten check rows; `Persisted` carries the
// minimal field set actually written to status.json (Reviewer Note 1).
type VerifyReport struct {
	SchemaVersion      string                        `json:"schema_version"`
	Slug               string                        `json:"slug"`
	VerifiedAt         string                        `json:"verified_at"`
	Verdict            string                        `json:"verdict"` // "passed" | "failed" | "refused"
	ExitCode           int                           `json:"exit_code"`
	Reason             string                        `json:"reason,omitempty"` // populated on "refused"
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

	// F2 / PRD §3.4.5 + §5: refuse pre-apply / mid-flight lifecycle
	// states. No persistence on refusal — even with --no-write unset,
	// status.json must NOT gain a Verify field. The CLI maps this
	// RefusedError onto exit code 2 (PRD §6 Q7).
	if !postApplyVerifyStates()[status.State] {
		reason := fmt.Sprintf("feature %s is in lifecycle state %q; verify refuses pre-apply / mid-flight states (PRD §5)", slug, status.State)
		refused := &VerifyReport{
			SchemaVersion:  verifySchemaVersion,
			Slug:           slug,
			VerifiedAt:     report.VerifiedAt,
			Verdict:        "refused",
			ExitCode:       2,
			Reason:         reason,
			Checks:         []store.VerifyCheckResult{},
			LifecycleState: status.State,
		}
		return refused, &RefusedError{Slug: slug, State: status.State, Reason: reason}
	}

	// V1 — intent_files_present (severity: block). PRD §3.1 row V1
	// requires `spec.md` AND `exploration.md` exist on disk under
	// `.tpatch/features/<slug>/` and be non-empty.
	report.Checks = append(report.Checks, checkIntentFilesPresent(s, slug))

	// V2 — recipe_parses (severity: block, real). PRD §3.1 row V2.
	// V3 (recipe_op_targets_resolve) is deferred to Slice C — see
	// PRD §9: it depends on `created_by` hard-parent semantics that
	// Slice A explicitly does not ship. We append a stub here in V3
	// position to keep the 10-check report shape stable.
	parseCheck, recipe, recipeBytes := checkRecipeParses(s, slug)
	report.Checks = append(report.Checks, parseCheck)
	report.Checks = append(report.Checks, stubRecipeOpTargetsResolve())

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

// checkIntentFilesPresent verifies that BOTH `spec.md` and
// `exploration.md` exist under `.tpatch/features/<slug>/` and are
// non-empty (PRD-verify-freshness §3.1 V1).
func checkIntentFilesPresent(s *store.Store, slug string) store.VerifyCheckResult {
	for _, name := range []string{"spec.md", "exploration.md"} {
		path := filepath.Join(s.Root, ".tpatch", "features", slug, name)
		info, err := os.Stat(path)
		if err != nil {
			return store.VerifyCheckResult{
				ID:          CheckIntentFilesPresent,
				Severity:    SeverityBlock,
				Passed:      false,
				Remediation: fmt.Sprintf("%s missing for %s — re-run the corresponding phase (`tpatch define %s` / `tpatch explore %s`)", name, slug, slug, slug),
			}
		}
		if info.Size() == 0 {
			return store.VerifyCheckResult{
				ID:          CheckIntentFilesPresent,
				Severity:    SeverityBlock,
				Passed:      false,
				Remediation: fmt.Sprintf("%s is empty for %s — re-run the corresponding phase", name, slug),
			}
		}
	}
	return store.VerifyCheckResult{
		ID:       CheckIntentFilesPresent,
		Severity: SeverityBlock,
		Passed:   true,
	}
}

// checkRecipeParses runs PRD §3.1 V2: parse `apply-recipe.json` with
// strict (DisallowUnknownFields) decoding. An absent recipe is
// `passed: true, skipped: true` (Reviewer Note 2). Returns the parsed
// recipe and its raw bytes for hashing on the persisted record.
//
// PRD's V3 (`recipe_op_targets_resolve`) is OUT OF SCOPE for Slice A
// (see PRD §9 — depends on Slice C `created_by` semantics). V3 is
// emitted separately as a Slice C stub by `stubRecipeOpTargetsResolve`.
func checkRecipeParses(s *store.Store, slug string) (parse store.VerifyCheckResult, recipe ApplyRecipe, raw []byte) {
	recipePath := filepath.Join(s.Root, ".tpatch", "features", slug, "artifacts", "apply-recipe.json")
	data, err := os.ReadFile(recipePath)
	if err != nil {
		if os.IsNotExist(err) {
			return store.VerifyCheckResult{
				ID:       CheckRecipeParses,
				Severity: SeverityBlock,
				Passed:   true,
				Skipped:  true,
				Reason:   "no apply-recipe.json (legacy / pre-autogen-era feature)",
			}, ApplyRecipe{}, nil
		}
		return store.VerifyCheckResult{
			ID:          CheckRecipeParses,
			Severity:    SeverityBlock,
			Passed:      false,
			Remediation: fmt.Sprintf("cannot read apply-recipe.json: %v", err),
		}, ApplyRecipe{}, nil
	}

	// Strict-decode: reject unknown fields. Mirrors the canonical
	// pattern guarded by `TestRecipeUnmarshal_DisallowsUnknownFields`
	// (recipe_createdby_test.go) so a confused agent's invented op
	// fields fail closed at verify time.
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if jsonErr := dec.Decode(&recipe); jsonErr != nil {
		return store.VerifyCheckResult{
			ID:          CheckRecipeParses,
			Severity:    SeverityBlock,
			Passed:      false,
			Remediation: fmt.Sprintf("apply-recipe.json failed to parse: %v", jsonErr),
		}, ApplyRecipe{}, data
	}

	return store.VerifyCheckResult{
		ID:       CheckRecipeParses,
		Severity: SeverityBlock,
		Passed:   true,
	}, recipe, data
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

// stubRecipeOpTargetsResolve emits PRD's V3 (`recipe_op_targets_resolve`)
// as a Slice C deferral. PRD §9 places this check in Slice C because it
// requires `created_by` hard-parent semantics (M14.2) that Slice A
// explicitly does not ship — see ADR-013 §9 / PRD §3.1 row V3.
func stubRecipeOpTargetsResolve() store.VerifyCheckResult {
	return store.VerifyCheckResult{
		ID:       CheckRecipeOpTargetsResolve,
		Severity: SeverityBlock,
		Passed:   true,
		Skipped:  true,
		Reason:   "not yet implemented (Slice C — created_by hard-parent semantics)",
	}
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
