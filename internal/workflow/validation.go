// Validation gate for the phase-3.5 conflict resolver.
//
// Every file the provider writes into the shadow worktree passes
// through `ValidateResolvedFile` before accept is permitted. The four
// gates are defined by ADR-010 D4:
//
//  1. No conflict markers (`<<<<<<<`, `>>>>>>>`).
//  2. Native parse — Go via `go/parser` in-tree; other extensions via
//     `ValidationConfig.SyntaxCheckCmd` with `{file}` substitution.
//  3. Identifier preservation — exported identifiers that appeared in
//     ours or theirs must appear in the resolved content. Regex-based
//     and deliberately approximate: catches accidental deletion of
//     public symbols without the maintenance cost of per-language ASTs.
//  4. Repo-wide `test_command` — optional, run in the shadow worktree
//     by `RunTestCommandInShadow`. Gated separately because it is
//     expensive; the three in-process checks run regardless.
//
// This file contains only validation logic. Shadow management lives in
// `internal/gitutil/shadow.go`; resolver orchestration will live in
// `internal/workflow/resolver.go`.

package workflow

import (
	"bytes"
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/tesseracode/tesserapatch/internal/gitutil"
)

// ValidationConfig tells the gate how to check non-Go files and whether
// to run the repo's test_command.
type ValidationConfig struct {
	// SyntaxCheckCmd is an optional shell command template. Occurrences
	// of "{file}" are replaced with the absolute path of the file to
	// validate. Empty means "skip syntax check for non-Go extensions".
	// Run with `sh -c`, so quoting is the caller's responsibility.
	SyntaxCheckCmd string

	// IdentifierCheck enables the exported-identifier-preservation gate
	// (gate 3). Default off — callers that want the stricter gate pass
	// true. Turning it off means a resolution that removes an exported
	// symbol still passes gates 1 and 2; the caller owns the policy
	// call.
	IdentifierCheck bool

	// TestCommand, when non-empty, is executed in the shadow worktree
	// by RunTestCommandInShadow (gate 4). Pure validation does not run
	// the test command — the resolver calls it separately.
	TestCommand string

	// TestTimeout bounds TestCommand. Zero means no timeout.
	TestTimeout time.Duration
}

// GateStatus captures one gate's outcome.
type GateStatus struct {
	Name    string `json:"name"` // conflict-markers | native-parse | identifier-preservation
	Passed  bool   `json:"passed"`
	Skipped bool   `json:"skipped,omitempty"` // true when no check applies (e.g. no SyntaxCheckCmd)
	Detail  string `json:"detail,omitempty"`  // short diagnostic — first error line, missing identifier, etc.
}

// ValidationResult is the aggregate verdict for a single resolved file.
type ValidationResult struct {
	Path   string       `json:"path"`
	Passed bool         `json:"passed"`
	Gates  []GateStatus `json:"gates"`
}

// FirstFailure returns the first non-skipped gate that didn't pass, or
// nil if everything passed.
func (r *ValidationResult) FirstFailure() *GateStatus {
	for i := range r.Gates {
		g := &r.Gates[i]
		if !g.Passed && !g.Skipped {
			return g
		}
	}
	return nil
}

// ValidateResolvedFile runs the three in-process gates on resolved.
// ours and theirs are only consulted for the identifier-preservation
// gate; pass empty slices to skip that dimension entirely.
//
// absPath is the absolute filesystem path of the resolved file; it is
// used both to dispatch the native-parse gate by extension and to hand
// off to SyntaxCheckCmd. The caller is expected to have written resolved
// to absPath already (writes happen in the shadow; this function reads
// the extension from the path and content from the slice).
func ValidateResolvedFile(absPath string, resolved, ours, theirs []byte, cfg ValidationConfig) ValidationResult {
	res := ValidationResult{Path: absPath, Passed: true}

	// Gate 1: conflict markers.
	g1 := GateStatus{Name: "conflict-markers", Passed: true}
	if gitutil.HasConflictMarkers(resolved) {
		g1.Passed = false
		g1.Detail = "unresolved git conflict markers present"
	}
	res.Gates = append(res.Gates, g1)

	// Gate 2: native parse.
	g2 := nativeParseGate(absPath, resolved, cfg.SyntaxCheckCmd)
	res.Gates = append(res.Gates, g2)

	// Gate 3: identifier preservation (optional).
	g3 := GateStatus{Name: "identifier-preservation", Passed: true}
	if !cfg.IdentifierCheck || (len(ours) == 0 && len(theirs) == 0) {
		g3.Skipped = true
	} else {
		if missing := missingExportedIdentifiers(resolved, ours, theirs); len(missing) > 0 {
			g3.Passed = false
			// Cap the detail line so a pathological diff doesn't blow
			// up the session JSON.
			show := missing
			if len(show) > 8 {
				show = show[:8]
			}
			g3.Detail = fmt.Sprintf("exported identifiers missing: %s", strings.Join(show, ", "))
			if len(missing) > 8 {
				g3.Detail += fmt.Sprintf(" (+%d more)", len(missing)-8)
			}
		}
	}
	res.Gates = append(res.Gates, g3)

	for _, g := range res.Gates {
		if !g.Passed && !g.Skipped {
			res.Passed = false
			break
		}
	}
	return res
}

// nativeParseGate dispatches by file extension. Go files go through
// `go/parser` in-tree. Everything else defers to SyntaxCheckCmd if
// configured; otherwise the gate is skipped.
func nativeParseGate(absPath string, content []byte, syntaxCheckCmd string) GateStatus {
	g := GateStatus{Name: "native-parse", Passed: true}
	ext := strings.ToLower(filepath.Ext(absPath))

	if ext == ".go" {
		// `parser.ParseFile` with AllErrors surfaces the first error's
		// message in a single string; good enough for the gate detail.
		fset := token.NewFileSet()
		if _, err := parser.ParseFile(fset, absPath, content, parser.AllErrors); err != nil {
			g.Passed = false
			g.Detail = firstLine(err.Error())
		}
		return g
	}

	if syntaxCheckCmd == "" {
		g.Skipped = true
		return g
	}

	cmdline := strings.ReplaceAll(syntaxCheckCmd, "{file}", shellQuote(absPath))
	cmd := exec.Command("sh", "-c", cmdline)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		g.Passed = false
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		g.Detail = firstLine(msg)
	}
	return g
}

// exportedIdentRe matches Go-style exported identifiers (leading
// uppercase ASCII letter, followed by letters/digits/underscore). It
// is deliberately language-agnostic at the regex level — most curly-
// brace languages use `PascalCase` for public API, so this catches
// the common "provider deleted our public method" case across Go,
// TypeScript, Java, Kotlin, C#, Rust. Not a substitute for an AST;
// it is the floor, not the ceiling.
var exportedIdentRe = regexp.MustCompile(`\b[A-Z][A-Za-z0-9_]{2,}\b`)

// missingExportedIdentifiers returns the set of exported-looking
// identifiers present in ours or theirs but absent from resolved.
// Results are sorted for deterministic output.
func missingExportedIdentifiers(resolved, ours, theirs []byte) []string {
	present := extractIdents(resolved)
	union := map[string]struct{}{}
	for id := range extractIdents(ours) {
		union[id] = struct{}{}
	}
	for id := range extractIdents(theirs) {
		union[id] = struct{}{}
	}
	var missing []string
	for id := range union {
		if _, ok := present[id]; !ok {
			missing = append(missing, id)
		}
	}
	sort.Strings(missing)
	return missing
}

func extractIdents(data []byte) map[string]struct{} {
	out := map[string]struct{}{}
	for _, m := range exportedIdentRe.FindAll(data, -1) {
		out[string(m)] = struct{}{}
	}
	return out
}

// firstLine returns the first non-empty line of s, trimmed. Used to
// keep gate detail messages single-line for session JSON.
func firstLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}
	return ""
}

// shellQuote single-quotes p for safe substitution into `sh -c`. The
// shadow paths come from tpatch itself and never contain quotes in
// practice, but robustness is cheap.
func shellQuote(p string) string {
	return "'" + strings.ReplaceAll(p, "'", `'\''`) + "'"
}

// TestRunResult records a test_command invocation in the shadow.
type TestRunResult struct {
	Ran        bool          `json:"ran"`
	Passed     bool          `json:"passed"`
	DurationMs int64         `json:"duration_ms"`
	ExitCode   int           `json:"exit_code"`
	TimedOut   bool          `json:"timed_out,omitempty"`
	Stderr     string        `json:"stderr,omitempty"`
	Duration   time.Duration `json:"-"`
}

// RunTestCommandInShadow executes cfg.TestCommand with CWD set to the
// shadow path. Returns (_, nil) even on non-zero exit: "test failed"
// is a valid business outcome, not an infrastructure error. An error
// is returned only when we could not launch the command at all.
func RunTestCommandInShadow(shadowPath string, cfg ValidationConfig) (TestRunResult, error) {
	if cfg.TestCommand == "" {
		return TestRunResult{Ran: false}, nil
	}
	if _, err := os.Stat(shadowPath); err != nil {
		return TestRunResult{}, fmt.Errorf("shadow path unavailable: %w", err)
	}

	cmd := exec.Command("sh", "-c", cfg.TestCommand)
	cmd.Dir = shadowPath
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	// Best-effort timeout. A zero TestTimeout disables it; we use a
	// goroutine-free approach with exec.Cmd.Wait and a timer signal.
	start := time.Now()
	if cfg.TestTimeout <= 0 {
		err := cmd.Run()
		res := TestRunResult{
			Ran:        true,
			DurationMs: time.Since(start).Milliseconds(),
			Duration:   time.Since(start),
			Stderr:     trimStderr(stderr.String()),
		}
		if err == nil {
			res.Passed = true
			return res, nil
		}
		if ee, ok := err.(*exec.ExitError); ok {
			res.ExitCode = ee.ExitCode()
			return res, nil
		}
		// Genuine launch failure: surface it.
		return res, fmt.Errorf("run test_command: %w", err)
	}

	// Timed path.
	if err := cmd.Start(); err != nil {
		return TestRunResult{Ran: true}, fmt.Errorf("start test_command: %w", err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case err := <-done:
		res := TestRunResult{
			Ran:        true,
			DurationMs: time.Since(start).Milliseconds(),
			Duration:   time.Since(start),
			Stderr:     trimStderr(stderr.String()),
		}
		if err == nil {
			res.Passed = true
			return res, nil
		}
		if ee, ok := err.(*exec.ExitError); ok {
			res.ExitCode = ee.ExitCode()
			return res, nil
		}
		return res, fmt.Errorf("run test_command: %w", err)
	case <-time.After(cfg.TestTimeout):
		_ = cmd.Process.Kill()
		<-done // drain Wait
		return TestRunResult{
			Ran:        true,
			TimedOut:   true,
			DurationMs: time.Since(start).Milliseconds(),
			Duration:   time.Since(start),
			Stderr:     trimStderr(stderr.String()),
		}, nil
	}
}

// trimStderr keeps test output short for the session JSON.
func trimStderr(s string) string {
	s = strings.TrimSpace(s)
	const cap = 2000
	if len(s) <= cap {
		return s
	}
	return s[len(s)-cap:]
}
