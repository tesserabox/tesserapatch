package store

import "strings"

// FeatureState represents the lifecycle state of a tracked feature.
type FeatureState string

const (
	StateRequested         FeatureState = "requested"
	StateAnalyzed          FeatureState = "analyzed"
	StateDefined           FeatureState = "defined"
	StateImplementing      FeatureState = "implementing"
	StateApplied           FeatureState = "applied"
	StateActive            FeatureState = "active"
	StateReconciling       FeatureState = "reconciling"
	StateReconcilingShadow FeatureState = "reconciling-shadow"
	StateBlocked           FeatureState = "blocked"
	StateUpstreamMerged    FeatureState = "upstream_merged"
)

// CompatibilityStatus describes how compatible a feature is with the base project.
type CompatibilityStatus string

const (
	CompatibilityUnknown    CompatibilityStatus = "unknown"
	CompatibilityCompatible CompatibilityStatus = "compatible"
	CompatibilityConflict   CompatibilityStatus = "conflict"
	CompatibilityUnclear    CompatibilityStatus = "unclear"
)

// ReconcileOutcome describes the result of reconciliation.
type ReconcileOutcome string

const (
	ReconcileUpstreamed  ReconcileOutcome = "upstreamed"
	ReconcileReapplied   ReconcileOutcome = "reapplied"
	ReconcileStillNeeded ReconcileOutcome = "still_needed"
	ReconcileBlocked     ReconcileOutcome = "blocked"

	// Phase-3.5 (M12 / ADR-010) verdicts. Shadow-awaiting means the
	// provider-assisted resolver succeeded and staged resolved files in
	// a shadow worktree for human acceptance. The two blocked-* forms
	// distinguish honest blockers (too many conflicts; validation/provider
	// failure) from the catch-all ReconcileBlocked used by earlier phases.
	ReconcileShadowAwaiting          ReconcileOutcome = "shadow-awaiting"
	ReconcileBlockedTooManyConflicts ReconcileOutcome = "blocked-too-many-conflicts"
	ReconcileBlockedRequiresHuman    ReconcileOutcome = "blocked-requires-human"
)

// ReconcileLabel is a derived overlay on top of Reconcile.Outcome that
// describes the DAG context (M14.3 / ADR-011 D3 + D6). Labels are computed
// on demand from parent state; they are NOT new ReconcileOutcome values
// and they are NOT persisted as enum values on Reconcile.Outcome.
//
// Multiple labels may stack on a single feature (e.g. a child waiting on
// one parent and stale relative to another). They are stored sorted
// alphabetically for deterministic JSON output.
//
// Authoritative source for the parent verdict that drives label
// composition: read parent.Reconcile.Outcome via store.LoadFeatureStatus
// — never artifacts/reconcile-session.json (ADR-010 D5).
type ReconcileLabel string

const (
	// LabelWaitingOnParent — at least one hard parent is not yet applied
	// (state is requested/analyzed/defined/implementing/reconciling/
	// reconciling-shadow). The child cannot meaningfully reconcile until
	// the parent reaches applied/active/upstream_merged.
	LabelWaitingOnParent ReconcileLabel = "waiting-on-parent"

	// LabelBlockedByParent — at least one hard parent is in a terminal
	// failure verdict (Outcome=blocked-* or shadow-awaiting on an applied
	// parent, or State=blocked). Combined with the child's own
	// needs-human-resolution this produces the compound presentation
	// "blocked-by-parent-and-needs-resolution" (see EffectiveOutcome).
	LabelBlockedByParent ReconcileLabel = "blocked-by-parent"

	// LabelStaleParentApplied — at least one applied hard parent has been
	// updated since the child's last reconcile attempt. The child's
	// recorded baseline may no longer reflect the parent's current state.
	LabelStaleParentApplied ReconcileLabel = "stale-parent-applied"

	// Slice B — freshness overlay (ADR-013 D5, PRD-verify-freshness §3.4.2).
	// Exactly one of these four labels is derived for every FeatureStatus
	// at READ time by composeLabelsFromStatus. They are NEVER persisted to
	// status.json (D4 byte-identity contract): persistence call sites
	// strip them before writing Reconcile.Labels.

	// LabelNeverVerified — `status.Verify == nil`. The feature has never
	// been through `tpatch verify`. Default freshness label for v0.6.1
	// fixtures.
	LabelNeverVerified ReconcileLabel = "never-verified"

	// LabelVerifiedFresh — `status.Verify.Passed == true` AND the recipe
	// hash + patch hash + every parent_snapshot entry currently match
	// (state-or-better invariant). The harness can trust the verify
	// claim against the current world.
	LabelVerifiedFresh ReconcileLabel = "verified-fresh"

	// LabelVerifiedStale — `status.Verify.Passed == true` but at least
	// one freshness condition has drifted (recipe rewrite, patch
	// rewrite, or a parent transitioned to a state the snapshot does
	// not allow). The persisted record is preserved unchanged; only
	// the derived label flips.
	LabelVerifiedStale ReconcileLabel = "verified-stale"

	// LabelVerifyFailed — `status.Verify.Passed == false`. The most
	// recent verify run reported a blocker. `amend (recipe-touching)`
	// per ADR-013 D3 may also clear `Passed` to surface invalidation
	// at write time.
	LabelVerifyFailed ReconcileLabel = "verify-failed"
)

// DefaultMaxTokensImplement is the fallback budget for the implement-phase
// LLM response when Config.MaxTokensImplement is unset or non-positive.
// Bumped from the previous hard-coded 8192 to reduce mid-JSON truncation
// for features that emit many large file bodies inline.
const DefaultMaxTokensImplement = 16384

// FeatureStatus is the machine-readable status of a tracked feature (status.json).
type FeatureStatus struct {
	ID            string              `json:"id"`
	Slug          string              `json:"slug"`
	Title         string              `json:"title"`
	State         FeatureState        `json:"state"`
	Compatibility CompatibilityStatus `json:"compatibility"`
	RequestedAt   string              `json:"requested_at"`
	UpdatedAt     string              `json:"updated_at"`
	LastCommand   string              `json:"last_command"`
	Notes         string              `json:"notes,omitempty"`
	Apply         ApplySummary        `json:"apply"`
	Reconcile     ReconcileSummary    `json:"reconcile"`

	// DependsOn lists the parent features this feature depends on, forming
	// the feature DAG. Edges flow child → parent (a child's DependsOn lists
	// its parents). See ADR-011 + docs/prds/PRD-feature-dependencies.md.
	//
	// Behaviour gate: this field is only read/written when
	// Config.FeaturesDependencies is true (default false until v0.6.0).
	// `omitempty` is load-bearing — when the flag is OFF and no deps are
	// declared, status.json must round-trip byte-for-byte identical to
	// pre-M14.1 fixtures.
	//
	// Authoritative source for derived reconcile decisions: read
	// status.Reconcile.Outcome — never read artifacts/reconcile-session.json
	// for DAG decisions. The session artifact is an audit record of one
	// RunReconcile invocation; status.json is the source of current truth
	// post-accept (see ADR-010 D5).
	DependsOn []Dependency `json:"depends_on,omitempty"`

	// Verify is the freshness-overlay sub-record written by the explicit
	// `tpatch verify` verb (ADR-013 D1 / D5). It is NOT a lifecycle state;
	// `FeatureState` is never mutated by verify. The pointer is
	// `omitempty`-marshalled so v0.6.1 fixtures that never run verify
	// round-trip byte-identical (ADR-013 D4).
	//
	// The persisted record carries the minimum needed to derive the four
	// freshness labels (`never-verified`, `verified-fresh`,
	// `verified-stale`, `verify-failed`) at read time in `ComposeLabels`
	// (Slice B); it deliberately does NOT persist the per-check array —
	// the full 10-check report is emitted on `tpatch verify --json`
	// stdout only (Reviewer Note 1, M15-W3 APPROVED WITH NOTES at 3c122aa).
	//
	// Read paths must NOT mutate this field. Only the `verify` and
	// `amend` (recipe-touching) verbs may rewrite it (ADR-013 D3).
	Verify *VerifyRecord `json:"verify,omitempty"`
}

// VerifyRecord is the persisted freshness overlay produced by
// `tpatch verify <slug>` (ADR-013 D1). Slice A populates the minimal field
// set; the full per-check array (`VerifyCheckResult`) is emitted only on
// `--json` stdout, never written to status.json (Reviewer Note 1).
//
// Hash fields are SHA-256 hex of the canonical bytes of `apply-recipe.json`
// and `artifacts/post-apply.patch` respectively at verify time. An empty
// string means the file was absent at verify time — see Reviewer Note 2
// for the absent-recipe contract.
//
// `ParentSnapshot` is keyed by parent slug; values are the parent's
// `FeatureState` literal at verify time. Slice B's freshness derivation
// reads this against current parent state to flip `verified-fresh` →
// `verified-stale` at READ time without rewriting status.json.
type VerifyRecord struct {
	VerifiedAt         string                  `json:"verified_at"`
	Passed             bool                    `json:"passed"`
	RecipeHashAtVerify string                  `json:"recipe_hash_at_verify,omitempty"`
	PatchHashAtVerify  string                  `json:"patch_hash_at_verify,omitempty"`
	ParentSnapshot     map[string]FeatureState `json:"parent_snapshot,omitempty"`
}

// VerifyCheckResult is the per-check entry in the in-memory `--json`
// report. NOT persisted to status.json (see VerifyRecord doc).
type VerifyCheckResult struct {
	ID          string `json:"id"`
	Severity    string `json:"severity"` // "block" | "block-abort" | "warn"
	Passed      bool   `json:"passed"`
	Skipped     bool   `json:"skipped,omitempty"`
	Reason      string `json:"reason,omitempty"`
	Remediation string `json:"remediation,omitempty"`
}

// Dependency declares a relationship from a child feature to a parent feature
// in the feature DAG. See ADR-011 + docs/prds/PRD-feature-dependencies.md.
//
// Hard vs soft semantics (ADR-011 D4):
//   - hard: parent must be applied; child apply is gated on parent state.
//   - soft: ordering hint only; does NOT gate apply (warn-only at apply time).
//
// `SatisfiedBy` is set only when the parent has reached state
// "upstream_merged" (ADR-011 D5); it carries the commit sha that absorbed
// the parent so the dependency edge retains its provenance even if the
// parent feature is later removed.
type Dependency struct {
	Slug        string `json:"slug"`
	Kind        string `json:"kind"`
	SatisfiedBy string `json:"satisfied_by,omitempty"`
}

// Dependency kind constants. See ADR-011 D4.
const (
	DependencyKindHard = "hard"
	DependencyKindSoft = "soft"
)

// ApplySummary tracks apply session state.
type ApplySummary struct {
	PreparedAt  string `json:"prepared_at,omitempty"`
	StartedAt   string `json:"started_at,omitempty"`
	CompletedAt string `json:"completed_at,omitempty"`
	BaseCommit  string `json:"base_commit,omitempty"`
	HasPatch    bool   `json:"has_patch,omitempty"`
	HasRecipe   bool   `json:"has_recipe,omitempty"`
}

// ReconcileSummary tracks reconciliation state.
type ReconcileSummary struct {
	AttemptedAt    string           `json:"attempted_at,omitempty"`
	UpstreamRef    string           `json:"upstream_ref,omitempty"`
	UpstreamCommit string           `json:"upstream_commit,omitempty"`
	Outcome        ReconcileOutcome `json:"outcome,omitempty"`

	// Phase-3.5 (M12 / ADR-010) fields. Populated only when the
	// resolver runs; remain zero on the classical phases 1-4 paths.
	ShadowPath     string `json:"shadow_path,omitempty"`
	ResolveSession string `json:"resolve_session_id,omitempty"`
	ResolvedFiles  int    `json:"resolved_files,omitempty"`
	FailedFiles    int    `json:"failed_files,omitempty"`
	SkippedFiles   int    `json:"skipped_files,omitempty"`

	// Labels is the M14.3 composable-label overlay computed from the DAG
	// (ADR-011 D3 + D6, PRD-feature-dependencies §3.5). Populated only
	// when Config.DAGEnabled() is true and at least one label applies.
	// `omitempty` is load-bearing for byte-identity round-trips against
	// pre-M14.3 fixtures: when the flag is off the field must round-trip
	// to the empty/absent form.
	Labels []ReconcileLabel `json:"labels,omitempty"`
}

// EffectiveOutcome returns the compound presentation of (Outcome, Labels)
// per ADR-011 D6 + PRD §3.5. Labels overlay on top of Outcome at READ
// time; the persisted Outcome is never the compound string.
//
// Compound rule (M14.3): when Outcome=blocked-requires-human (the
// "needs-human-resolution" intrinsic verdict) AND Labels contains
// LabelBlockedByParent, the compound presentation is
// "blocked-by-parent-and-needs-resolution". This signals to operators
// that the child's own resolution is also gated on a broken parent.
//
// All other (Outcome, Labels) combinations stringify to the bare Outcome.
// Programmatic decisions (e.g. apply gating, planner ordering) MUST read
// Outcome and Labels separately — the compound string is for display only.
func (r ReconcileSummary) EffectiveOutcome() string {
	if r.Outcome == ReconcileBlockedRequiresHuman {
		for _, l := range r.Labels {
			if l == LabelBlockedByParent {
				return "blocked-by-parent-and-needs-resolution"
			}
		}
	}
	return string(r.Outcome)
}

// Config holds the .tpatch/config.yaml contents.
type Config struct {
	Provider      ProviderConfig `json:"provider"`
	MergeStrategy string         `json:"merge_strategy"` // "3way" (default) or "rebase"
	MaxRetries    int            `json:"max_retries"`    // LLM validation retries (default 2)
	TestCommand   string         `json:"test_command"`   // shell command run by `tpatch test`

	// MaxTokensImplement caps the LLM response budget for the implement
	// phase. The implement phase emits whole-file content inline, so it
	// truncates more aggressively than the other phases. Default 16384
	// (set when zero/negative). Override per-repo or globally via
	// `max_tokens_implement:` in config.yaml.
	MaxTokensImplement int `json:"max_tokens_implement,omitempty"`

	// CopilotAUPAckAt is the ISO-8601 timestamp at which the user acknowledged
	// the GitHub Copilot Acceptable Use Policy warning. Written only to the
	// global config (~/.config/tpatch/config.yaml). Empty means "never
	// acknowledged"; non-empty suppresses the first-run warning.
	CopilotAUPAckAt string `json:"copilot_aup_acknowledged_at,omitempty"`

	// CopilotNativeOptIn records the user's opt-in for the native Copilot
	// provider (type: copilot-native). Global-only. When false, commands
	// that would activate copilot-native print the AUP and refuse.
	CopilotNativeOptIn bool `json:"copilot_native_optin,omitempty"`
	// CopilotNativeOptInAt is the ISO-8601 timestamp at opt-in.
	CopilotNativeOptInAt string `json:"copilot_native_optin_at,omitempty"`

	// FeaturesDependencies gates the feature dependency DAG (ADR-011 D9).
	// Default true from v0.6.0. Set explicitly to false to opt back into
	// pre-v0.6.0 byte-identity behaviour. Wired via flat YAML key
	// `features_dependencies: true|false` (the existing parser does not
	// support nested maps without a rewrite).
	FeaturesDependencies bool `json:"features_dependencies,omitempty"`
}

// DAGEnabled reports whether feature-dependency DAG behaviour is active for
// this config. All callers that gate on the dependency feature should use
// this helper rather than reading the field directly, so the gate has a
// single chokepoint (ADR-011 D9).
func (c Config) DAGEnabled() bool {
	return c.FeaturesDependencies
}

// ProviderConfig stores the LLM provider settings.
type ProviderConfig struct {
	Type      string `json:"type"`
	BaseURL   string `json:"base_url"`
	Model     string `json:"model"`
	AuthEnv   string `json:"auth_env"`            // env var name, NOT the secret
	Initiator string `json:"initiator,omitempty"` // x-initiator header ("", "user", "agent") for copilot-native
}

// Configured returns true if the provider has enough info to attempt a connection.
// copilot-native relies on the auth file for its base URL, so only Model is required.
func (c ProviderConfig) Configured() bool {
	if strings.EqualFold(strings.TrimSpace(c.Type), "copilot-native") {
		return c.Model != ""
	}
	return c.BaseURL != "" && c.Model != ""
}

// UpstreamLock tracks the upstream repository state.
type UpstreamLock struct {
	Remote string `json:"remote"`
	Branch string `json:"branch"`
	Commit string `json:"commit"`
	URL    string `json:"url"`
}

// AddFeatureInput is the input to Store.AddFeature.
type AddFeatureInput struct {
	Title   string
	Request string
	Slug    string
}

// ApplySession is the structured apply-session.json artifact.
type ApplySession struct {
	Slug             string `json:"slug"`
	PreparedAt       string `json:"prepared_at,omitempty"`
	StartedAt        string `json:"started_at,omitempty"`
	CompletedAt      string `json:"completed_at"`
	BaseCommit       string `json:"base_commit,omitempty"`
	HasPatch         bool   `json:"has_patch"`
	OperatorNotes    string `json:"operator_notes,omitempty"`
	ValidationStatus string `json:"validation_status,omitempty"` // passed, failed, needs_review
	ValidationNotes  string `json:"validation_notes,omitempty"`
}
