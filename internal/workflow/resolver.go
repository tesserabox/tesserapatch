// Phase-3.5 conflict resolver.
//
// The resolver is the core value prop of tpatch: when forward-apply
// produces 3-way conflicts, we ask the provider to produce a merged
// version of each conflicted file, validate it, and stage it in a
// shadow worktree. The shadow outlives the call so the user can
// review or `--accept`/`--reject` atomically.
//
// This file is deliberately free of git-refs plumbing. Callers
// (currently reconcile wiring in b2-reconcile-wiring) fetch base /
// ours / theirs for each conflicted file and hand them in as
// ConflictInput. Tests can exercise the whole flow with a stub
// provider and no git at all.
//
// Design notes (ADR-010 D3, D4, D5, D7):
//   - Sequential per-file calls in v0.5.0 — parallelism is a v0.5.x
//     follow-up (`feat-resolver-parallel`).
//   - Hard caps: MaxConflicts (default 10) and MaxFileBytes (default
//     200 KB). Files over the byte cap are marked skipped-too-large;
//     too many conflicts short-circuits the whole session.
//   - On every path we emit resolution-session.json to
//     `.tpatch/features/<slug>/artifacts/` and a human-readable
//     resolution-report.md inside the shadow root. (v0.5.3 split:
//     resolver owns resolution-session.json; the outer reconcile
//     pipeline owns reconcile-session.json — see saveReconcileArtifacts
//     in reconcile.go. Previously both wrote the same path and
//     reconcile's overwrite clobbered the resolver's outcomes[],
//     breaking manual `reconcile --accept`.)
//   - The resolver never touches the real working tree. Accept flow
//     lives in b2-derived-refresh.

package workflow

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/tesseracode/tesserapatch/internal/gitutil"
	"github.com/tesseracode/tesserapatch/internal/provider"
	"github.com/tesseracode/tesserapatch/internal/safety"
	"github.com/tesseracode/tesserapatch/internal/store"
)

// Verdicts for the resolver session as a whole. These are distinct
// from the existing reconcile verdicts because phase 3.5 sits before
// the final forward-apply decision.
const (
	ResolveVerdictShadowAwaiting          = "shadow-awaiting"
	ResolveVerdictBlockedTooManyConflicts = "blocked-too-many-conflicts"
	ResolveVerdictBlockedRequiresHuman    = "blocked-requires-human"
	ResolveVerdictAutoAccepted            = "auto-accepted"
)

// Per-file outcome statuses.
const (
	FileStatusResolved         = "resolved"
	FileStatusValidationFailed = "validation-failed"
	FileStatusSkippedTooLarge  = "skipped-too-large"
	FileStatusProviderError    = "provider-error"
)

// Default caps per ADR-010 D7. Overridable via ResolveOptions.
const (
	DefaultMaxConflicts = 10
	DefaultMaxFileBytes = 200 * 1024
)

// ConflictInput carries one conflicted file's three-way content. The
// caller is responsible for extracting these from git; the resolver
// treats them as opaque bytes.
type ConflictInput struct {
	Path   string // repo-relative
	Base   []byte // file at baseCommit (upstream-before-reconcile)
	Ours   []byte // patched version (feature-applied on baseCommit)
	Theirs []byte // file at upstreamCommit (new upstream)
}

// ResolveOptions configures RunConflictResolve. Zero values produce
// the ADR-010 defaults.
type ResolveOptions struct {
	MaxConflicts   int              // default DefaultMaxConflicts
	MaxFileBytes   int64            // default DefaultMaxFileBytes
	ModelOverride  string           // override cfg.Model for this call only
	Validation     ValidationConfig // forwarded to ValidateResolvedFile / RunTestCommandInShadow
	AutoApply      bool             // if true, copy shadow → real tree on full success (--resolve --apply)
	SpecExcerpt    string           // optional override; default: slurp spec.md capped to 8k
	ExplorationCap int              // chars; default 4096
	SpecCap        int              // chars; default 8192
}

// FileOutcome is one file's result.
type FileOutcome struct {
	Path       string            `json:"path"`
	Status     string            `json:"status"`
	Bytes      int               `json:"bytes,omitempty"`
	Validation *ValidationResult `json:"validation,omitempty"`
	Error      string            `json:"error,omitempty"`
}

// ResolveResult is the session-level outcome written to
// resolution-session.json.
type ResolveResult struct {
	SessionID       string         `json:"session_id"`
	StartedAt       time.Time      `json:"started_at"`
	EndedAt         time.Time      `json:"ended_at"`
	Verdict         string         `json:"verdict"`
	UpstreamCommit  string         `json:"upstream_commit,omitempty"`
	Provider        string         `json:"provider,omitempty"`
	Model           string         `json:"model,omitempty"`
	ShadowPath      string         `json:"shadow_path,omitempty"`
	ConflictedFiles []string       `json:"conflicted_files"`
	Outcomes        []FileOutcome  `json:"outcomes"`
	TestResult      *TestRunResult `json:"test_result,omitempty"`
}

// RunConflictResolve is the entry point for phase 3.5. It is
// synchronous and sequential; returns as soon as all files have been
// attempted (or the too-many-conflicts cap triggers an early exit).
//
// The caller owns git plumbing: inputs must be fully populated
// (Base/Ours/Theirs) and upstreamCommit must be the ref at which the
// shadow worktree should be checked out.
func RunConflictResolve(
	ctx context.Context,
	s *store.Store,
	slug string,
	prov provider.Provider,
	cfg provider.Config,
	inputs []ConflictInput,
	upstreamCommit string,
	opts ResolveOptions,
) (*ResolveResult, error) {
	applyDefaults(&opts)

	start := time.Now().UTC()
	res := &ResolveResult{
		SessionID:       newSessionID(),
		StartedAt:       start,
		UpstreamCommit:  upstreamCommit,
		Provider:        cfg.Type,
		Model:           effectiveModel(cfg, opts),
		ConflictedFiles: pathsOf(inputs),
	}

	// Pre-flight: cap on number of conflicts.
	if len(inputs) > opts.MaxConflicts {
		res.Verdict = ResolveVerdictBlockedTooManyConflicts
		res.EndedAt = time.Now().UTC()
		if err := persistSession(s, slug, res); err != nil {
			return res, fmt.Errorf("persist session: %w", err)
		}
		return res, nil
	}

	// Provider-required. The resolver has no heuristic fallback per
	// ADR-010 D9 (feat-resolver-heuristic-fallback is logged for
	// later opt-in support).
	if prov == nil || !cfg.Configured() {
		res.Verdict = ResolveVerdictBlockedRequiresHuman
		res.EndedAt = time.Now().UTC()
		for _, in := range inputs {
			res.Outcomes = append(res.Outcomes, FileOutcome{
				Path:   in.Path,
				Status: FileStatusProviderError,
				Error:  "provider not configured",
			})
		}
		if err := persistSession(s, slug, res); err != nil {
			return res, fmt.Errorf("persist session: %w", err)
		}
		return res, nil
	}

	// Create the shadow. Reaps any prior shadow for the slug.
	shadowPath, err := gitutil.CreateShadow(s.Root, slug, upstreamCommit)
	if err != nil {
		return res, fmt.Errorf("create shadow: %w", err)
	}
	res.ShadowPath = shadowPath

	specExcerpt, explorationExcerpt := gatherExcerpts(s, slug, opts)
	callCfg := cfg
	if opts.ModelOverride != "" {
		callCfg.Model = opts.ModelOverride
	}

	allResolved := true
	for _, in := range inputs {
		outcome := resolveOne(ctx, prov, callCfg, in, shadowPath, specExcerpt, explorationExcerpt, opts)
		res.Outcomes = append(res.Outcomes, outcome)
		if outcome.Status != FileStatusResolved {
			allResolved = false
		}
	}

	// Optional test_command gate (ADR-010 D4 gate 4).
	if allResolved && opts.Validation.TestCommand != "" {
		tr, terr := RunTestCommandInShadow(shadowPath, opts.Validation)
		if terr != nil {
			// Launch failure — treat as blocked.
			tr.Ran = true
			tr.Stderr = terr.Error()
			allResolved = false
		}
		res.TestResult = &tr
		if !tr.Passed {
			allResolved = false
		}
	}

	switch {
	case !allResolved:
		res.Verdict = ResolveVerdictBlockedRequiresHuman
	case opts.AutoApply:
		// Actual file copy is the caller's job (b2-derived-refresh).
		// The resolver records the intent here so the verdict is
		// auditable; callers flip the bit to AutoAccepted on success.
		res.Verdict = ResolveVerdictAutoAccepted
	default:
		res.Verdict = ResolveVerdictShadowAwaiting
	}

	res.EndedAt = time.Now().UTC()

	if err := writeResolutionReport(shadowPath, res); err != nil {
		return res, fmt.Errorf("write report: %w", err)
	}
	if err := persistSession(s, slug, res); err != nil {
		return res, fmt.Errorf("persist session: %w", err)
	}
	return res, nil
}

// resolveOne handles a single file: cap check → prompt → provider call →
// fence stripping → shadow write → validation.
func resolveOne(
	ctx context.Context,
	prov provider.Provider,
	cfg provider.Config,
	in ConflictInput,
	shadowPath, specExcerpt, explorationExcerpt string,
	opts ResolveOptions,
) FileOutcome {
	out := FileOutcome{Path: in.Path}

	size := int64(len(in.Ours) + len(in.Theirs) + len(in.Base))
	if size > opts.MaxFileBytes {
		out.Status = FileStatusSkippedTooLarge
		out.Bytes = int(size)
		out.Error = fmt.Sprintf("combined 3-way content %d bytes exceeds max_file_bytes=%d", size, opts.MaxFileBytes)
		return out
	}

	systemPrompt := conflictResolveSystemPrompt
	userPrompt := buildConflictUserPrompt(in, specExcerpt, explorationExcerpt)

	resp, err := prov.Generate(ctx, cfg, provider.GenerateRequest{
		SystemPrompt: systemPrompt,
		UserPrompt:   userPrompt,
		MaxTokens:    16384,
		Temperature:  0.1,
	})
	if err != nil {
		out.Status = FileStatusProviderError
		out.Error = err.Error()
		return out
	}
	resolved := stripResolverFences(resp)

	// Write into the shadow, creating parent dirs as needed.
	abs := filepath.Join(shadowPath, filepath.FromSlash(in.Path))
	if err := safety.EnsureSafeRepoPath(shadowPath, abs); err != nil {
		out.Status = FileStatusProviderError
		out.Error = "unsafe shadow path: " + err.Error()
		return out
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		out.Status = FileStatusProviderError
		out.Error = "mkdir shadow: " + err.Error()
		return out
	}
	if err := os.WriteFile(abs, []byte(resolved), 0o644); err != nil {
		out.Status = FileStatusProviderError
		out.Error = "write shadow: " + err.Error()
		return out
	}
	out.Bytes = len(resolved)

	// Run in-process validation gates (markers, native-parse, identifier).
	v := ValidateResolvedFile(abs, []byte(resolved), in.Ours, in.Theirs, opts.Validation)
	out.Validation = &v
	if !v.Passed {
		out.Status = FileStatusValidationFailed
		if f := v.FirstFailure(); f != nil {
			out.Error = fmt.Sprintf("%s: %s", f.Name, f.Detail)
		}
		return out
	}
	out.Status = FileStatusResolved
	return out
}

// conflictResolveSystemPrompt is the ADR-010 D3 instruction.
const conflictResolveSystemPrompt = `You are resolving a 3-way merge conflict in a forked codebase.

You receive:
- BASE: the original upstream version of the file.
- OURS: the same file with the feature's patch applied.
- THEIRS: the new upstream version of the file.
- SPEC: the feature's intent (why OURS diverges from BASE).
- EXPLORATION: grounding notes.

Your goal is to produce a single merged file that preserves BOTH
intents: the feature's behavior (from OURS / SPEC) and the upstream
change (from THEIRS). When in doubt, prefer THEIRS for code the
feature does not care about, and OURS for code the feature introduced.

Output ONLY the resolved file content. No commentary, no markdown code
fences, no explanations. The first byte of your response must be the
first byte of the resolved file.`

func buildConflictUserPrompt(in ConflictInput, specExcerpt, explorationExcerpt string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# File: %s\n\n", in.Path)
	if specExcerpt != "" {
		b.WriteString("## SPEC\n")
		b.WriteString(specExcerpt)
		b.WriteString("\n\n")
	}
	if explorationExcerpt != "" {
		b.WriteString("## EXPLORATION\n")
		b.WriteString(explorationExcerpt)
		b.WriteString("\n\n")
	}
	b.WriteString("## BASE\n")
	b.Write(in.Base)
	b.WriteString("\n\n## OURS\n")
	b.Write(in.Ours)
	b.WriteString("\n\n## THEIRS\n")
	b.Write(in.Theirs)
	b.WriteString("\n")
	return b.String()
}

// codeFenceRe matches a provider response wrapped in markdown code
// fences (with or without a language tag). We strip them because the
// system prompt asks for raw content but many models ignore that.
// Kept distinct from jsonextract.stripCodeFences, which is lenient
// about trailing prose for the JSON case; here we want a conservative
// "only strip when the whole response is fenced" rule so we don't
// accidentally eat real file content that legitimately contains
// triple backticks (markdown docs etc.).
var codeFenceRe = regexp.MustCompile("(?s)\\A\\s*```[a-zA-Z0-9_-]*\\s*\\n(.*?)\\n?```\\s*\\z")

func stripResolverFences(s string) string {
	if m := codeFenceRe.FindStringSubmatch(s); m != nil {
		return m[1]
	}
	return s
}

// gatherExcerpts reads spec.md and exploration.md, capped to avoid
// blowing context window. Caps are defensive — a 50-page spec would
// drown the conflicted file content.
func gatherExcerpts(s *store.Store, slug string, opts ResolveOptions) (string, string) {
	if opts.SpecCap == 0 {
		opts.SpecCap = 8192
	}
	if opts.ExplorationCap == 0 {
		opts.ExplorationCap = 4096
	}
	spec := opts.SpecExcerpt
	if spec == "" {
		if content, err := s.ReadFeatureFile(slug, "spec.md"); err == nil {
			spec = capStr(content, opts.SpecCap)
		}
	}
	var exploration string
	if content, err := s.ReadFeatureFile(slug, "exploration.md"); err == nil {
		exploration = capStr(content, opts.ExplorationCap)
	}
	return spec, exploration
}

func capStr(s string, n int) string {
	if n <= 0 || len(s) <= n {
		return s
	}
	return s[:n] + "\n...[truncated]"
}

func applyDefaults(opts *ResolveOptions) {
	if opts.MaxConflicts <= 0 {
		opts.MaxConflicts = DefaultMaxConflicts
	}
	if opts.MaxFileBytes <= 0 {
		opts.MaxFileBytes = DefaultMaxFileBytes
	}
}

func effectiveModel(cfg provider.Config, opts ResolveOptions) string {
	if opts.ModelOverride != "" {
		return opts.ModelOverride
	}
	return cfg.Model
}

func pathsOf(inputs []ConflictInput) []string {
	out := make([]string, 0, len(inputs))
	for _, in := range inputs {
		out = append(out, in.Path)
	}
	sort.Strings(out)
	return out
}

func newSessionID() string {
	var buf [6]byte
	_, _ = rand.Read(buf[:])
	return fmt.Sprintf("rec-%s-%s", time.Now().UTC().Format("2006-01-02T15-04-05Z"), hex.EncodeToString(buf[:]))
}

// persistSession writes resolution-session.json into the feature's
// artifacts directory. This is the auditable record of the phase-3.5
// resolver run (per-file outcomes + validation) referenced by
// `tpatch status`, the accept flow (loadResolvedFiles), and the
// v0.5.0 resolution-report. Split from reconcile-session.json in
// v0.5.3 — reconcile-session.json is now owned exclusively by
// saveReconcileArtifacts and holds the high-level ReconcileResult.
func persistSession(s *store.Store, slug string, res *ResolveResult) error {
	data, err := json.MarshalIndent(res, "", "  ")
	if err != nil {
		return err
	}
	return s.WriteArtifact(slug, "resolution-session.json", string(data)+"\n")
}

// writeResolutionReport emits a human-readable companion to the JSON
// session record. Lives inside the shadow so `--shadow-diff` and the
// accept flow can surface it.
func writeResolutionReport(shadowPath string, res *ResolveResult) error {
	if err := safety.EnsureSafeRepoPath(shadowPath, filepath.Join(shadowPath, "resolution-report.md")); err != nil {
		return err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# Resolution Report\n\n")
	fmt.Fprintf(&b, "- Session: `%s`\n", res.SessionID)
	fmt.Fprintf(&b, "- Verdict: **%s**\n", res.Verdict)
	fmt.Fprintf(&b, "- Provider: %s\n", res.Provider)
	fmt.Fprintf(&b, "- Model: %s\n", res.Model)
	fmt.Fprintf(&b, "- Upstream commit: %s\n", res.UpstreamCommit)
	fmt.Fprintf(&b, "- Started: %s\n", res.StartedAt.Format(time.RFC3339))
	fmt.Fprintf(&b, "- Ended: %s\n\n", res.EndedAt.Format(time.RFC3339))

	fmt.Fprintf(&b, "## Files (%d)\n\n", len(res.Outcomes))
	for _, o := range res.Outcomes {
		fmt.Fprintf(&b, "- `%s` — **%s**", o.Path, o.Status)
		if o.Error != "" {
			fmt.Fprintf(&b, " — %s", o.Error)
		}
		fmt.Fprintln(&b)
	}
	if res.TestResult != nil {
		fmt.Fprintf(&b, "\n## Test command\n\n- Ran: %v\n- Passed: %v\n- Exit: %d\n- Duration: %dms\n",
			res.TestResult.Ran, res.TestResult.Passed, res.TestResult.ExitCode, res.TestResult.DurationMs)
		if res.TestResult.TimedOut {
			fmt.Fprintf(&b, "- **Timed out**\n")
		}
	}
	return os.WriteFile(filepath.Join(shadowPath, "resolution-report.md"), []byte(b.String()), 0o644)
}
