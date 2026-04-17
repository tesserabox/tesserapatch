package store

// FeatureState represents the lifecycle state of a tracked feature.
type FeatureState string

const (
	StateRequested      FeatureState = "requested"
	StateAnalyzed       FeatureState = "analyzed"
	StateDefined        FeatureState = "defined"
	StateImplementing   FeatureState = "implementing"
	StateApplied        FeatureState = "applied"
	StateActive         FeatureState = "active"
	StateReconciling    FeatureState = "reconciling"
	StateBlocked        FeatureState = "blocked"
	StateUpstreamMerged FeatureState = "upstream_merged"
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
)

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
}

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
}

// Config holds the .tpatch/config.yaml contents.
type Config struct {
	Provider      ProviderConfig `json:"provider"`
	MergeStrategy string         `json:"merge_strategy"` // "3way" (default) or "rebase"
	MaxRetries    int            `json:"max_retries"`    // LLM validation retries (default 2)
	TestCommand   string         `json:"test_command"`   // shell command run by `tpatch test`
}

// ProviderConfig stores the LLM provider settings.
type ProviderConfig struct {
	Type    string `json:"type"`
	BaseURL string `json:"base_url"`
	Model   string `json:"model"`
	AuthEnv string `json:"auth_env"` // env var name, NOT the secret
}

// Configured returns true if the provider has enough info to attempt a connection.
func (c ProviderConfig) Configured() bool {
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
