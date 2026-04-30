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
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/tesseracode/tesserapatch/internal/gitutil"
	"github.com/tesseracode/tesserapatch/internal/safety"
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

	// FailedAt and ParentSlug are populated by V7's hard-parent
	// closure replay when a parent fails to reconstruct (PRD §3.4.3
	// fail-fast semantics). Both omitempty so the never-failed JSON
	// shape is unchanged.
	FailedAt   string `json:"failed_at,omitempty"`
	ParentSlug string `json:"parent_slug,omitempty"`

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

	// V2 — recipe_parses (severity: block).
	parseCheck, recipe, recipeBytes := checkRecipeParses(s, slug)
	recipePresent := parseCheck.Passed && !parseCheck.Skipped
	report.Checks = append(report.Checks, parseCheck)

	// V3 — recipe_op_targets_resolve (severity: block).
	report.Checks = append(report.Checks, checkRecipeOpTargetsResolve(s, status, recipe, recipePresent))

	// V4 — dep_metadata_valid (severity: block).
	report.Checks = append(report.Checks, checkDepMetadataValid(s, slug, status))

	// V5 — satisfied_by_reachable (severity: block).
	report.Checks = append(report.Checks, checkSatisfiedByReachable(s, slug, status))

	// V6 — dependency_gate_satisfied (severity: warn, gated on
	// Config.DAGEnabled).
	report.Checks = append(report.Checks, checkDependencyGateSatisfied(s, slug, status))

	// V7 + V8 — closure replay (severity: block, dynamic). Skip if
	// any earlier static block-severity check failed so we don't
	// allocate a shadow we can't trust the inputs of.
	v7v8Skip := anyBlockFailed(report.Checks)
	if v7v8Skip {
		report.Checks = append(report.Checks,
			store.VerifyCheckResult{ID: CheckRecipeReplayClean, Severity: SeverityBlock, Passed: true, Skipped: true, Reason: "skipped: an earlier block-severity static check failed"},
			store.VerifyCheckResult{ID: CheckPostApplyPatchReplayClean, Severity: SeverityBlock, Passed: true, Skipped: true, Reason: "skipped: V7 (recipe_replay_clean) skipped"},
		)
	} else {
		patchPath := filepath.Join(s.Root, ".tpatch", "features", slug, "artifacts", "post-apply.patch")
		patchPresent := false
		if fi, statErr := os.Stat(patchPath); statErr == nil && !fi.IsDir() {
			patchPresent = true
		}
		cr := runClosureReplay(s, slug, status, recipe, recipePresent, patchPresent)
		report.Checks = append(report.Checks, cr.v7, cr.v8)
		if cr.failedAt != "" {
			report.FailedAt = cr.failedAt
			report.ParentSlug = cr.parentSlug
		}
	}

	// V9 — reconcile_outcome_consistent (severity: warn). Reads
	// status.Reconcile.Outcome ONLY (ADR-013 D6).
	report.Checks = append(report.Checks, checkReconcileOutcomeConsistent(status))

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
		CheckDepMetadataValid,
		CheckSatisfiedByReachable,
	} {
		out = append(out, store.VerifyCheckResult{
			ID:       id,
			Severity: SeverityBlock,
			Passed:   true,
			Skipped:  true,
			Reason:   "skipped: V0 (status_loaded) aborted the run",
		})
	}
	out = append(out, store.VerifyCheckResult{
		ID:       CheckDependencyGateSatisfied,
		Severity: SeverityWarn,
		Passed:   true,
		Skipped:  true,
		Reason:   "skipped: V0 (status_loaded) aborted the run",
	})
	for _, id := range []string{
		CheckRecipeReplayClean,
		CheckPostApplyPatchReplayClean,
	} {
		out = append(out, store.VerifyCheckResult{
			ID:       id,
			Severity: SeverityBlock,
			Passed:   true,
			Skipped:  true,
			Reason:   "skipped: V0 (status_loaded) aborted the run",
		})
	}
	out = append(out, store.VerifyCheckResult{
		ID:       CheckReconcileOutcomeConsistent,
		Severity: SeverityWarn,
		Passed:   true,
		Skipped:  true,
		Reason:   "skipped: V0 (status_loaded) aborted the run",
	})
	return out
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

// anyBlockFailed returns true when any non-skipped block / block-abort
// check in checks has Passed=false. Used to short-circuit V7/V8 (the
// dynamic phase) so we don't allocate a shadow when static inputs are
// already broken.
func anyBlockFailed(checks []store.VerifyCheckResult) bool {
	for _, c := range checks {
		if c.Skipped || c.Passed {
			continue
		}
		if c.Severity == SeverityBlock || c.Severity == SeverityBlockAbort {
			return true
		}
	}
	return false
}

// ── V3 — recipe_op_targets_resolve ──────────────────────────────────────
//
// Per PRD §3.1 V3, every op's Path must exist OR carry a `created_by`
// whose parent is a declared hard dep currently in `applied` /
// `upstream_merged`. Mirrors the apply-time `created_by` gate
// (`internal/workflow/created_by_gate.go:57`) — only ops whose semantics
// require a pre-existing target (replace-in-file, append-file) trigger
// the existence check; write-file and ensure-directory create their
// targets and pass trivially.
func checkRecipeOpTargetsResolve(s *store.Store, status store.FeatureStatus, recipe ApplyRecipe, recipePresent bool) store.VerifyCheckResult {
	if !recipePresent {
		return store.VerifyCheckResult{
			ID:       CheckRecipeOpTargetsResolve,
			Severity: SeverityBlock,
			Passed:   true,
			Skipped:  true,
			Reason:   "no apply-recipe.json (precondition not met)",
		}
	}

	hardParentState := map[string]store.FeatureState{}
	for _, dep := range status.DependsOn {
		if dep.Kind != store.DependencyKindHard {
			continue
		}
		ps, err := s.LoadFeatureStatus(dep.Slug)
		if err != nil {
			continue
		}
		hardParentState[dep.Slug] = ps.State
	}

	for i, op := range recipe.Operations {
		switch op.Type {
		case "replace-in-file", "append-file":
			// fall through — pre-existing target required
		default:
			continue
		}
		target := filepath.Join(s.Root, op.Path)
		if _, err := os.Stat(target); err == nil {
			continue
		}
		if op.CreatedBy != "" {
			st, ok := hardParentState[op.CreatedBy]
			if ok && (st == store.StateApplied || st == store.StateUpstreamMerged) {
				continue
			}
		}
		// PRD §3.1.2 V3 — verbatim template.
		return store.VerifyCheckResult{
			ID:          CheckRecipeOpTargetsResolve,
			Severity:    SeverityBlock,
			Passed:      false,
			Remediation: fmt.Sprintf("recipe op #%d path '%s' missing and created_by empty; declare created_by=<parent> or apply <parent>", i+1, op.Path),
		}
	}

	return store.VerifyCheckResult{
		ID:       CheckRecipeOpTargetsResolve,
		Severity: SeverityBlock,
		Passed:   true,
	}
}

// ── V4 — dep_metadata_valid ─────────────────────────────────────────────
//
// PRD §3.1 V4 wraps `store.ValidateDependencies(s, slug, status.DependsOn)`
// (`internal/store/validation.go:66`). Per §3.1.2 the remediation
// surfaces the validation sentinel verbatim.
func checkDepMetadataValid(s *store.Store, slug string, status store.FeatureStatus) store.VerifyCheckResult {
	if err := store.ValidateDependencies(s, slug, status.DependsOn); err != nil {
		return store.VerifyCheckResult{
			ID:          CheckDepMetadataValid,
			Severity:    SeverityBlock,
			Passed:      false,
			Remediation: err.Error(),
		}
	}
	return store.VerifyCheckResult{
		ID:       CheckDepMetadataValid,
		Severity: SeverityBlock,
		Passed:   true,
	}
}

// ── V5 — satisfied_by_reachable ─────────────────────────────────────────
//
// PRD §3.1 V5: every dep with `satisfied_by` set must match the 40-hex
// SHA regex AND `gitutil.IsAncestor(repoRoot, sha, "HEAD")` must return
// true. Skipped (passed) when no dep carries satisfied_by.
func checkSatisfiedByReachable(s *store.Store, slug string, status store.FeatureStatus) store.VerifyCheckResult {
	var checked int
	for _, dep := range status.DependsOn {
		if dep.SatisfiedBy == "" {
			continue
		}
		checked++
		if !satisfiedBySHA.MatchString(dep.SatisfiedBy) {
			return store.VerifyCheckResult{
				ID:          CheckSatisfiedByReachable,
				Severity:    SeverityBlock,
				Passed:      false,
				Remediation: fmt.Sprintf("satisfied_by SHA %s for parent %s is no longer reachable from HEAD; re-run tpatch amend %s --remove-depends-on %s --depends-on %s", dep.SatisfiedBy, dep.Slug, slug, dep.Slug, dep.Slug),
			}
		}
		ok, err := gitutil.IsAncestor(s.Root, dep.SatisfiedBy, "HEAD")
		if err != nil || !ok {
			return store.VerifyCheckResult{
				ID:          CheckSatisfiedByReachable,
				Severity:    SeverityBlock,
				Passed:      false,
				Remediation: fmt.Sprintf("satisfied_by SHA %s for parent %s is no longer reachable from HEAD; re-run tpatch amend %s --remove-depends-on %s --depends-on %s", dep.SatisfiedBy, dep.Slug, slug, dep.Slug, dep.Slug),
			}
		}
	}
	if checked == 0 {
		return store.VerifyCheckResult{
			ID:       CheckSatisfiedByReachable,
			Severity: SeverityBlock,
			Passed:   true,
			Skipped:  true,
			Reason:   "no satisfied_by deps to check",
		}
	}
	return store.VerifyCheckResult{
		ID:       CheckSatisfiedByReachable,
		Severity: SeverityBlock,
		Passed:   true,
	}
}

// ── V6 — dependency_gate_satisfied ──────────────────────────────────────
//
// PRD §3.1 V6 (warn). Gated on `Config.DAGEnabled()` — when the flag is
// off, V6 is a passed+skipped no-op. Otherwise calls
// `workflow.CheckDependencyGate` (`internal/workflow/dependency_gate.go:42`)
// and reports the first hard parent in a non-{applied,upstream_merged}
// state per PRD §3.1.2 V6.
func checkDependencyGateSatisfied(s *store.Store, slug string, status store.FeatureStatus) store.VerifyCheckResult {
	cfg, err := s.LoadConfig()
	if err != nil {
		return store.VerifyCheckResult{
			ID:       CheckDependencyGateSatisfied,
			Severity: SeverityWarn,
			Passed:   true,
			Skipped:  true,
			Reason:   fmt.Sprintf("cannot load config: %v", err),
		}
	}
	if !cfg.DAGEnabled() {
		return store.VerifyCheckResult{
			ID:       CheckDependencyGateSatisfied,
			Severity: SeverityWarn,
			Passed:   true,
			Skipped:  true,
			Reason:   "DAG disabled in config",
		}
	}
	if gateErr := CheckDependencyGate(s, slug); gateErr != nil {
		// Locate the first hard parent that fails the apply-gate so the
		// remediation can name slug + state.
		for _, dep := range status.DependsOn {
			if dep.Kind != store.DependencyKindHard {
				continue
			}
			ps, perr := s.LoadFeatureStatus(dep.Slug)
			label := "<missing>"
			if perr == nil {
				if ps.State == store.StateApplied || ps.State == store.StateUpstreamMerged {
					continue
				}
				label = string(ps.State)
			}
			return store.VerifyCheckResult{
				ID:          CheckDependencyGateSatisfied,
				Severity:    SeverityWarn,
				Passed:      false,
				Remediation: fmt.Sprintf("hard parent %s in state=%s (warn-only at verify time)", dep.Slug, label),
			}
		}
		// Fallback if no specific parent could be identified.
		return store.VerifyCheckResult{
			ID:          CheckDependencyGateSatisfied,
			Severity:    SeverityWarn,
			Passed:      false,
			Remediation: gateErr.Error(),
		}
	}
	return store.VerifyCheckResult{
		ID:       CheckDependencyGateSatisfied,
		Severity: SeverityWarn,
		Passed:   true,
	}
}

// ── V9 — reconcile_outcome_consistent ───────────────────────────────────
//
// PRD §3.1 V9 (warn). ADR-013 D6 is binding: this check reads
// `status.Reconcile.Outcome` and ONLY that field. It must not stat or
// open any file under `artifacts/` (the recipe and post-apply patch are
// touched by V2/V3/V7/V8 only) and must never read
// `reconcile-session.json`.
func checkReconcileOutcomeConsistent(status store.FeatureStatus) store.VerifyCheckResult {
	outcome := status.Reconcile.Outcome
	if outcome == "" {
		return store.VerifyCheckResult{
			ID:       CheckReconcileOutcomeConsistent,
			Severity: SeverityWarn,
			Passed:   true,
			Skipped:  true,
			Reason:   "no Reconcile.Outcome set",
		}
	}
	switch outcome {
	case store.ReconcileReapplied, store.ReconcileUpstreamed, store.ReconcileStillNeeded:
		return store.VerifyCheckResult{
			ID:       CheckReconcileOutcomeConsistent,
			Severity: SeverityWarn,
			Passed:   true,
		}
	}
	return store.VerifyCheckResult{
		ID:          CheckReconcileOutcomeConsistent,
		Severity:    SeverityWarn,
		Passed:      false,
		Remediation: fmt.Sprintf("reconcile outcome is %s; verify cannot vouch for reconcile health (warn-only)", outcome),
	}
}

// ── V7 + V8 — hard-parent topological closure replay ────────────────────
//
// PRD §3.4.3 spec. ONE shadow is allocated for the run; V7 replays the
// hard-parent closure into it (parents in topological order) and then
// applies the target's recipe; V8 then `git apply --check`s the
// target's `post-apply.patch` against the same shadow tree. Shadow is
// pruned via deferred call regardless of pass/fail (ADR-013 D7).
//
// The closure-replay primitive lives ONLY in this file (ADR-010 D2 +
// ADR-013 §3.4.3 "Why this is verify-only"). Do not factor out into a
// shared helper without an ADR amendment.
type closureReplayResult struct {
	v7         store.VerifyCheckResult
	v8         store.VerifyCheckResult
	failedAt   string
	parentSlug string
}

func runClosureReplay(s *store.Store, slug string, status store.FeatureStatus, recipe ApplyRecipe, recipePresent, patchPresent bool) closureReplayResult {
	// Edge case (PRD-verify-freshness §5, line 526): both apply-recipe.json
	// and post-apply.patch absent → skip V7 and V8, do NOT allocate the
	// shadow. The shadow is only spun up when at least one of the two
	// dynamic checks has an artifact to validate.
	if !recipePresent && !patchPresent {
		return closureReplayResult{
			v7: store.VerifyCheckResult{ID: CheckRecipeReplayClean, Severity: SeverityBlock, Passed: true, Skipped: true, Reason: "no apply-recipe.json (precondition not met)"},
			v8: store.VerifyCheckResult{ID: CheckPostApplyPatchReplayClean, Severity: SeverityBlock, Passed: true, Skipped: true, Reason: "no post-apply.patch (precondition not met)"},
		}
	}

	// V7 skipped reason when recipe is absent but we still proceed to
	// allocate the shadow (because patch is present and V8 must run
	// against the closure-replayed baseline — PRD §5 line 524).
	v7SkipRecipeAbsent := store.VerifyCheckResult{
		ID:       CheckRecipeReplayClean,
		Severity: SeverityBlock,
		Passed:   true,
		Skipped:  true,
		Reason:   "no apply-recipe.json (precondition not met)",
	}

	// 1. Compute hard-parent closure (BFS over DependencyKindHard).
	closure := map[string][]store.Dependency{}
	closure[slug] = filterHardDeps(status.DependsOn)
	queue := append([]string(nil), depSlugsHard(status.DependsOn)...)
	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]
		if _, seen := closure[curr]; seen {
			continue
		}
		st, err := s.LoadFeatureStatus(curr)
		if err != nil {
			closure[curr] = nil
			continue
		}
		hd := filterHardDeps(st.DependsOn)
		closure[curr] = hd
		for _, d := range hd {
			queue = append(queue, d.Slug)
		}
	}

	// 2. Topological order over hard-only sub-DAG.
	order, err := store.TopologicalOrder(closure)
	if err != nil {
		return closureReplayResult{
			v7: store.VerifyCheckResult{
				ID:          CheckRecipeReplayClean,
				Severity:    SeverityBlock,
				Passed:      false,
				Remediation: fmt.Sprintf("hard-parent closure topology failed: %v; investigate or re-run tpatch implement %s", err, slug),
			},
			v8: skipV8Because("V7 (recipe_replay_clean) failed: topology"),
		}
	}

	// 3. Allocate ONE shadow for the run, defer prune.
	head, err := gitutil.HeadCommit(s.Root)
	if err != nil {
		return closureReplayResult{
			v7: store.VerifyCheckResult{
				ID:          CheckRecipeReplayClean,
				Severity:    SeverityBlock,
				Passed:      false,
				Remediation: fmt.Sprintf("cannot resolve HEAD for shadow allocation: %v", err),
			},
			v8: skipV8Because("V7 (recipe_replay_clean) failed: HEAD unresolved"),
		}
	}
	shadowPath, err := gitutil.CreateShadow(s.Root, slug, head)
	if err != nil {
		return closureReplayResult{
			v7: store.VerifyCheckResult{
				ID:          CheckRecipeReplayClean,
				Severity:    SeverityBlock,
				Passed:      false,
				Remediation: fmt.Sprintf("cannot allocate shadow worktree: %v", err),
			},
			v8: skipV8Because("V7 (recipe_replay_clean) failed: shadow allocation"),
		}
	}
	defer func() {
		// ADR-013 D7: shadow is pruned before verify exits, regardless
		// of pass/fail. Single defer guards every return path.
		_ = gitutil.PruneShadow(s.Root, slug)
	}()

	// 4. Replay parents in topo order, skipping target. On any
	// parent-replay failure: V7 carries the parent-replay remediation
	// (PRD §3.4.3 verbatim form) and V8 is skipped with the
	// "parent-replay aborted before V8" reason (PRD §4.3.5). This
	// holds even when recipePresent is false — the parent-replay
	// failure is still reported on V7.
	for _, parent := range order {
		if parent == slug {
			continue
		}
		pst, err := s.LoadFeatureStatus(parent)
		if err != nil {
			return closureReplayResult{
				v7:         parentReplayFail(parent, fmt.Errorf("cannot load parent status: %w", err)),
				v8:         skipV8Because("skipped: parent-replay aborted before V8"),
				failedAt:   "parent-replay",
				parentSlug: parent,
			}
		}
		switch pst.State {
		case store.StateUpstreamMerged:
			// Skip — parent's changes are already on the baseline.
			continue
		case store.StateApplied:
			pr, prerr := loadParentRecipe(s, parent)
			if prerr != nil {
				return closureReplayResult{
					v7:         parentReplayFail(parent, prerr),
					v8:         skipV8Because("skipped: parent-replay aborted before V8"),
					failedAt:   "parent-replay",
					parentSlug: parent,
				}
			}
			if _, rerr := replayRecipeOpsInShadow(shadowPath, pr.Operations); rerr != nil {
				return closureReplayResult{
					v7:         parentReplayFail(parent, rerr),
					v8:         skipV8Because("skipped: parent-replay aborted before V8"),
					failedAt:   "parent-replay",
					parentSlug: parent,
				}
			}
		default:
			return closureReplayResult{
				v7:         parentReplayFail(parent, fmt.Errorf("parent state is %q (need applied or upstream_merged)", pst.State)),
				v8:         skipV8Because("skipped: parent-replay aborted before V8"),
				failedAt:   "parent-replay",
				parentSlug: parent,
			}
		}
	}

	// 5. V7 — apply target's recipe in the same shadow, OR skip if
	// recipe is absent (PRD §5 line 524: V7 skipped when recipe absent
	// but V8 still runs against the closure-replayed baseline).
	var v7 store.VerifyCheckResult
	if recipePresent {
		if opIdx, rerr := replayRecipeOpsInShadow(shadowPath, recipe.Operations); rerr != nil {
			return closureReplayResult{
				v7: store.VerifyCheckResult{
					ID:          CheckRecipeReplayClean,
					Severity:    SeverityBlock,
					Passed:      false,
					Remediation: fmt.Sprintf("recipe op #%d failed in shadow replay: %v; investigate or re-run tpatch implement %s", opIdx, rerr, slug),
				},
				v8: skipV8Because("V7 (recipe_replay_clean) failed"),
			}
		}
		v7 = store.VerifyCheckResult{ID: CheckRecipeReplayClean, Severity: SeverityBlock, Passed: true}
	} else {
		v7 = v7SkipRecipeAbsent
	}

	// 6. V8 — git apply --check post-apply.patch against the shadow
	// (which now contains the closure-replayed baseline, plus the
	// target recipe if recipePresent). Skip if the patch is absent.
	if !patchPresent {
		return closureReplayResult{
			v7: v7,
			v8: store.VerifyCheckResult{
				ID:       CheckPostApplyPatchReplayClean,
				Severity: SeverityBlock,
				Passed:   true,
				Skipped:  true,
				Reason:   "no post-apply.patch (precondition not met)",
			},
		}
	}
	patchPath := filepath.Join(s.Root, ".tpatch", "features", slug, "artifacts", "post-apply.patch")
	cmd := exec.Command("git", "apply", "--check", patchPath)
	cmd.Dir = shadowPath
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return closureReplayResult{
			v7: v7,
			v8: store.VerifyCheckResult{
				ID:          CheckPostApplyPatchReplayClean,
				Severity:    SeverityBlock,
				Passed:      false,
				Remediation: fmt.Sprintf("post-apply.patch no longer applies to closure-replayed baseline; run tpatch reconcile %s", slug),
			},
		}
	}
	return closureReplayResult{
		v7: v7,
		v8: store.VerifyCheckResult{ID: CheckPostApplyPatchReplayClean, Severity: SeverityBlock, Passed: true},
	}
}

// parentReplayFail formats the V7 result for a parent-replay failure
// per PRD §3.1.2 (parent-replay variant).
func parentReplayFail(parentSlug string, err error) store.VerifyCheckResult {
	return store.VerifyCheckResult{
		ID:          CheckRecipeReplayClean,
		Severity:    SeverityBlock,
		Passed:      false,
		Remediation: fmt.Sprintf("hard parent %s failed to replay in shadow: %v; re-run tpatch verify %s on the parent first", parentSlug, err, parentSlug),
	}
}

func skipV8Because(reason string) store.VerifyCheckResult {
	return store.VerifyCheckResult{
		ID:       CheckPostApplyPatchReplayClean,
		Severity: SeverityBlock,
		Passed:   true,
		Skipped:  true,
		Reason:   reason,
	}
}

func filterHardDeps(deps []store.Dependency) []store.Dependency {
	var out []store.Dependency
	for _, d := range deps {
		if d.Kind == store.DependencyKindHard {
			out = append(out, d)
		}
	}
	return out
}

func depSlugsHard(deps []store.Dependency) []string {
	var out []string
	for _, d := range deps {
		if d.Kind == store.DependencyKindHard {
			out = append(out, d.Slug)
		}
	}
	return out
}

func loadParentRecipe(s *store.Store, parent string) (ApplyRecipe, error) {
	raw, err := s.ReadFeatureFile(parent, filepath.Join("artifacts", "apply-recipe.json"))
	if err != nil {
		return ApplyRecipe{}, fmt.Errorf("read parent recipe: %w", err)
	}
	var pr ApplyRecipe
	if err := json.Unmarshal([]byte(raw), &pr); err != nil {
		return ApplyRecipe{}, fmt.Errorf("parse parent recipe: %w", err)
	}
	return pr, nil
}

// replayRecipeOpsInShadow applies recipe ops directly against the
// shadow worktree. It deliberately does NOT call ExecuteRecipe — that
// path consults `s.LoadConfig()` and the M14 `created_by` apply-time
// gate, both of which assume a real `.tpatch/` workspace at the store
// root. The shadow is a bare `git worktree` checkout of HEAD with no
// `.tpatch/` of its own.
//
// Returns (opIndex, err) where opIndex is 1-based and 0 on success.
func replayRecipeOpsInShadow(shadowRoot string, ops []RecipeOperation) (int, error) {
	for i, op := range ops {
		if err := replayOpInShadow(shadowRoot, op); err != nil {
			return i + 1, fmt.Errorf("[%s %s] %w", op.Type, op.Path, err)
		}
	}
	return 0, nil
}

func replayOpInShadow(shadowRoot string, op RecipeOperation) error {
	target := filepath.Join(shadowRoot, op.Path)
	if err := safety.EnsureSafeRepoPath(shadowRoot, target); err != nil {
		return fmt.Errorf("path safety: %w", err)
	}
	switch op.Type {
	case "write-file":
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, []byte(op.Content), 0o644)
	case "replace-in-file":
		content, err := os.ReadFile(target)
		if err != nil {
			return fmt.Errorf("file not found: %w", err)
		}
		text := string(content)
		if !strings.Contains(text, op.Search) {
			return fmt.Errorf("search text not found")
		}
		replaced := strings.Replace(text, op.Search, op.Replace, 1)
		return os.WriteFile(target, []byte(replaced), 0o644)
	case "append-file":
		f, err := os.OpenFile(target, os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = f.WriteString(op.Content)
		return err
	case "ensure-directory":
		return os.MkdirAll(target, 0o755)
	default:
		return fmt.Errorf("unknown operation type %q", op.Type)
	}
}
