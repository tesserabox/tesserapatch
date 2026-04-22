// Package gitutil provides git operations: diff, patch capture, reverse-apply, head commit.
package gitutil

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// HeadCommit returns the current HEAD commit hash.
func HeadCommit(repoRoot string) (string, error) {
	out, err := runGit(repoRoot, "rev-parse", "HEAD")
	if err != nil {
		return "", fmt.Errorf("git rev-parse HEAD: %w", err)
	}
	return strings.TrimSpace(out), nil
}

// RecentCommit is a one-line summary of a commit, used to suggest
// candidate --from base refs when `tpatch record` captures an empty
// diff (almost always because the user committed before recording).
type RecentCommit struct {
	SHA     string // short SHA
	When    string // "2 hours ago"
	Subject string // commit subject line
}

// RecentCommits returns up to `limit` recent commits on HEAD, newest
// first. Used by the record command to give the user concrete --from
// candidates in the "you committed before recording" diagnostic. Never
// returns an error — a bare repo / shallow clone / first commit case
// simply yields a shorter list.
func RecentCommits(repoRoot string, limit int) []RecentCommit {
	if limit <= 0 {
		limit = 10
	}
	// Use an ASCII unit separator between fields so commit subjects
	// containing tabs or pipes do not break parsing.
	sep := "\x1f"
	format := "%h" + sep + "%ar" + sep + "%s"
	out, err := runGit(repoRoot, "log", fmt.Sprintf("-n%d", limit), "--pretty=format:"+format)
	if err != nil {
		return nil
	}
	var result []RecentCommit
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, sep, 3)
		if len(parts) != 3 {
			continue
		}
		result = append(result, RecentCommit{SHA: parts[0], When: parts[1], Subject: parts[2]})
	}
	return result
}

// IsWorkingTreeDirty reports whether there are unstaged or untracked
// changes in the repo. Used by the record empty-capture diagnostic to
// distinguish the "nothing changed" case from the "you committed
// already" case.
func IsWorkingTreeDirty(repoRoot string) bool {
	out, err := runGit(repoRoot, "status", "--porcelain")
	if err != nil {
		return false
	}
	return strings.TrimSpace(out) != ""
}

// IsPathTracked reports whether `path` (relative to repoRoot) is
// tracked by git. A missing path or any git error returns false so
// callers can treat "not tracked" as the conservative default.
func IsPathTracked(repoRoot, path string) bool {
	out, err := runGit(repoRoot, "ls-files", "--", path)
	if err != nil {
		return false
	}
	return strings.TrimSpace(out) != ""
}

// ReconcilePreflight is the preflight report returned by
// PreflightReconcile. The Reconcile phase MUST NOT run when any of the
// four fields is non-empty, unless the user passes --allow-dirty.
//
// Rationale (see A10 doc-reconcile-workflow): a dirty tree or lingering
// conflict markers silently corrupt reverse/forward apply verdicts —
// reconcile reads file bytes, not git trees, so a `<<<<<<<` line inside
// a source file looks exactly like any other context line to `git apply
// --check`. We hard-refuse instead of guessing.
type ReconcilePreflight struct {
	// UnstagedFiles lists `git status --porcelain` entries with their
	// status code, e.g. " M apps/server/src/foo.ts".
	UnstagedFiles []string
	// UntrackedFiles lists files present in the tree but ignored by
	// git (separate from modified-tracked files).
	UntrackedFiles []string
	// MergeMarkerFiles lists paths that still contain `<<<<<<< `,
	// `=======`, or `>>>>>>> ` conflict markers.
	MergeMarkerFiles []string
	// LeftoverFiles lists *.orig and *.rej files — the classic "I
	// aborted a merge but forgot to clean up" footprint.
	LeftoverFiles []string
}

// Clean reports whether the preflight found zero violations.
func (p ReconcilePreflight) Clean() bool {
	return len(p.UnstagedFiles) == 0 &&
		len(p.UntrackedFiles) == 0 &&
		len(p.MergeMarkerFiles) == 0 &&
		len(p.LeftoverFiles) == 0
}

// PreflightReconcile inspects the working tree for the four conditions
// that make reconcile verdicts unreliable. It is read-only — it never
// modifies files. See ReconcilePreflight for the contract.
func PreflightReconcile(repoRoot string) (ReconcilePreflight, error) {
	var p ReconcilePreflight

	// git status --porcelain: first two columns are the status code,
	// remainder is the path. We split tracked-modified from untracked.
	out, err := runGit(repoRoot, "status", "--porcelain")
	if err != nil {
		return p, fmt.Errorf("git status: %w", err)
	}
	for _, line := range strings.Split(out, "\n") {
		if len(line) < 4 {
			continue
		}
		code, path := line[:2], strings.TrimSpace(line[3:])
		if code == "??" {
			p.UntrackedFiles = append(p.UntrackedFiles, path)
		} else {
			p.UnstagedFiles = append(p.UnstagedFiles, line)
		}
	}

	// Conflict markers. `git grep -lE '^<<<<<<< |^=======$|^>>>>>>> '`
	// scans tracked files only, which is what we want — untracked
	// noise is already reported above.
	if mark, _ := runGit(repoRoot, "grep", "-lE", "^<<<<<<< |^=======$|^>>>>>>> "); strings.TrimSpace(mark) != "" {
		for _, f := range strings.Split(strings.TrimSpace(mark), "\n") {
			p.MergeMarkerFiles = append(p.MergeMarkerFiles, f)
		}
	}

	// *.orig and *.rej leftovers anywhere in the tree (walk, cheap).
	_ = filepath.Walk(repoRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil {
			return nil
		}
		// Skip .git/ entirely.
		if info.IsDir() && info.Name() == ".git" {
			return filepath.SkipDir
		}
		if info.IsDir() {
			return nil
		}
		name := info.Name()
		if strings.HasSuffix(name, ".orig") || strings.HasSuffix(name, ".rej") {
			rel, rerr := filepath.Rel(repoRoot, path)
			if rerr != nil {
				rel = path
			}
			p.LeftoverFiles = append(p.LeftoverFiles, rel)
		}
		return nil
	})

	sort.Strings(p.UnstagedFiles)
	sort.Strings(p.UntrackedFiles)
	sort.Strings(p.MergeMarkerFiles)
	sort.Strings(p.LeftoverFiles)
	return p, nil
}

// CaptureDiffStat returns `git diff --stat` output.
func CaptureDiffStat(repoRoot string) (string, error) {
	out, err := runGit(repoRoot, "diff", "--stat")
	if err != nil {
		return "", err
	}
	return out, nil
}

// CapturePatch captures a unified diff including tracked modifications and untracked new files.
// It excludes .tpatch/, .claude/skills/, .github/skills/, .github/prompts/, .cursor/rules/.
func CapturePatch(repoRoot string) (string, error) {
	excludePatterns := []string{
		":(exclude).tpatch",
		":(exclude).claude/skills",
		":(exclude).github/skills",
		":(exclude).github/prompts",
		":(exclude).cursor/rules",
		":(exclude).windsurfrules",
	}

	skipPrefixes := []string{".tpatch/", ".claude/skills/", ".github/skills/", ".github/prompts/", ".cursor/rules/", ".windsurfrules"}

	// Stage untracked files with --intent-to-add so they appear in git diff
	untrackedFiles, _ := runGit(repoRoot, "ls-files", "--others", "--exclude-standard")
	var stagedNewFiles []string
	for _, file := range strings.Split(strings.TrimSpace(untrackedFiles), "\n") {
		file = strings.TrimSpace(file)
		if file == "" {
			continue
		}
		skip := false
		for _, prefix := range skipPrefixes {
			if strings.HasPrefix(file, prefix) || file == strings.TrimSuffix(prefix, "/") {
				skip = true
				break
			}
		}
		if skip {
			continue
		}
		// Stage as intent-to-add (makes new files visible to git diff)
		if _, err := runGit(repoRoot, "add", "--intent-to-add", file); err == nil {
			stagedNewFiles = append(stagedNewFiles, file)
		}
	}

	// Capture unified diff (now includes tracked changes AND intent-to-add new files)
	args := append([]string{"diff", "--"}, excludePatterns...)
	patch, err := runGit(repoRoot, args...)
	if err != nil {
		patch = ""
	}

	// Unstage the intent-to-add files to leave the working tree clean
	for _, file := range stagedNewFiles {
		runGit(repoRoot, "reset", "--", file)
	}

	result := strings.TrimSpace(patch)
	if result != "" {
		result += "\n" // git patches must end with a newline
	}
	return result, nil
}

// CapturePatchFromCommits captures the diff between two commits, excluding tpatch artifacts.
func CapturePatchFromCommits(repoRoot, fromRef, toRef string) (string, error) {
	excludePatterns := []string{
		":(exclude).tpatch",
		":(exclude).claude/skills",
		":(exclude).github/skills",
		":(exclude).github/prompts",
		":(exclude).cursor/rules",
		":(exclude).windsurfrules",
	}
	args := append([]string{"diff", fromRef, toRef, "--"}, excludePatterns...)
	out, err := runGit(repoRoot, args...)
	if err != nil {
		return "", err
	}
	result := strings.TrimSpace(out)
	if result != "" {
		result += "\n" // git patches must end with a newline
	}
	return result, nil
}

// ValidatePatch runs `git apply --check` to verify a patch is well-formed and can be applied.
// It checks against the given repoRoot (which should be at the target baseline).
// Returns nil if valid. Tries strict first, then 3-way if strategy is "3way".
func ValidatePatch(repoRoot, patch, strategy string) error {
	if patch == "" {
		return fmt.Errorf("empty patch")
	}
	// Strict check first
	cmd := exec.Command("git", "apply", "--check", "-")
	cmd.Dir = repoRoot
	cmd.Stdin = strings.NewReader(patch)
	if cmd.Run() == nil {
		return nil
	}
	if strategy == "3way" || strategy == "" {
		cmd = exec.Command("git", "apply", "--3way", "--check", "-")
		cmd.Dir = repoRoot
		cmd.Stdin = strings.NewReader(patch)
		if cmd.Run() == nil {
			return nil
		}
	}
	return fmt.Errorf("patch validation failed: patch cannot be applied cleanly")
}

// ReverseApplyCheck tests if a patch can be reverse-applied (already present in the tree).
func ReverseApplyCheck(repoRoot, patch string) (bool, error) {
	cmd := exec.Command("git", "apply", "--reverse", "--check", "-")
	cmd.Dir = repoRoot
	cmd.Stdin = strings.NewReader(patch)
	err := cmd.Run()
	return err == nil, nil
}

// ValidatePatchReverse runs `git apply --reverse --check` against the
// current working tree. This is the correct semantic for record-time
// validation: the patch was just applied, so the working tree contains
// its result. A successful reverse-apply proves the recorded patch
// round-trips against what is on disk — i.e. it is well-formed and
// describes the changes accurately.
//
// Compare with ValidatePatch (forward `git apply --check`) which is
// correct for reconcile/rebase-time validation against an upstream
// baseline that does NOT yet contain the patch.
//
// Returns nil on success. On failure, surfaces git's stderr so users
// can see the precise reason (line-ending mismatch, binary file
// without index, untracked-file collision, etc).
func ValidatePatchReverse(repoRoot, patch string) error {
	if patch == "" {
		return fmt.Errorf("empty patch")
	}
	cmd := exec.Command("git", "apply", "--reverse", "--check", "-")
	cmd.Dir = repoRoot
	cmd.Stdin = strings.NewReader(patch)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("patch does not round-trip against working tree: %s", msg)
	}
	return nil
}

// ForwardApplyCheck tests if a patch can be applied cleanly.
// Tries strict apply first, then falls back to 3-way merge check.
func ForwardApplyCheck(repoRoot, patch string) (bool, error) {
	// Try strict apply first
	cmd := exec.Command("git", "apply", "--check", "-")
	cmd.Dir = repoRoot
	cmd.Stdin = strings.NewReader(patch)
	if cmd.Run() == nil {
		return true, nil
	}
	// Fall back to 3-way merge check (handles context mismatches)
	cmd = exec.Command("git", "apply", "--3way", "--check", "-")
	cmd.Dir = repoRoot
	cmd.Stdin = strings.NewReader(patch)
	return cmd.Run() == nil, nil
}

// ForwardApplyVerdict is what phase-4 of reconcile now consumes. It
// distinguishes a clean re-apply from a 3-way merge that will leave
// conflict markers in the tree — the latter used to masquerade as
// "reapplied" because `git apply --3way --check` returns 0 whenever
// the 3-way machinery *could attempt* the merge, even if the final
// files contain conflict markers.
type ForwardApplyVerdict int

const (
	// ForwardApplyStrict means `git apply --check` (without --3way)
	// succeeds. Safe to auto-apply.
	ForwardApplyStrict ForwardApplyVerdict = iota
	// ForwardApply3WayClean means strict failed but a real 3-way merge
	// in an isolated worktree completes without conflict markers.
	ForwardApply3WayClean
	// ForwardApply3WayConflicts means the 3-way merge runs but leaves
	// conflict markers — the user must resolve them. ConflictFiles on
	// the ForwardApplyPreview lists the affected paths.
	ForwardApply3WayConflicts
	// ForwardApplyBlocked means neither strict nor 3-way can even
	// attempt the apply.
	ForwardApplyBlocked
)

// ForwardApplyPreview is the structured result of PreviewForwardApply.
// Verdict is always set; ConflictFiles is non-nil only when Verdict ==
// ForwardApply3WayConflicts. Stderr carries git's diagnostic output for
// the final attempt and is surfaced in reconcile notes.
type ForwardApplyPreview struct {
	Verdict       ForwardApplyVerdict
	ConflictFiles []string
	Stderr        string
}

// PreviewForwardApply gives an authoritative phase-4 verdict without
// mutating repoRoot. The algorithm:
//  1. Strict `git apply --check` — if it passes, return ForwardApplyStrict.
//  2. Create a temporary linked worktree at HEAD (`git worktree add --detach`).
//  3. Actually run `git apply --3way` in the worktree.
//  4. Scan the worktree for conflict markers (`<<<<<<<`, `=======`, `>>>>>>>`).
//     - No markers + apply exit 0 ⇒ ForwardApply3WayClean.
//     - Markers present        ⇒ ForwardApply3WayConflicts (+ file list).
//     - Apply failed outright  ⇒ ForwardApplyBlocked.
//  5. Remove the worktree.
//
// If the worktree provisioning fails (e.g. bare repo, permissions),
// PreviewForwardApply falls back to the looser strict/--3way --check
// pair and marks the verdict ForwardApply3WayClean conservatively —
// logging the fallback reason in Stderr so callers can report it.
func PreviewForwardApply(repoRoot, patch string) (ForwardApplyPreview, error) {
	if patch == "" {
		return ForwardApplyPreview{Verdict: ForwardApplyBlocked, Stderr: "empty patch"}, nil
	}

	// Phase 4a: strict check.
	strict := exec.Command("git", "apply", "--check", "-")
	strict.Dir = repoRoot
	strict.Stdin = strings.NewReader(patch)
	if strict.Run() == nil {
		return ForwardApplyPreview{Verdict: ForwardApplyStrict}, nil
	}

	// Phase 4b: linked worktree at HEAD for a real 3-way attempt.
	wt, cleanup, wtErr := mkPreviewWorktree(repoRoot)
	if wtErr != nil {
		// Degraded path: without an isolated worktree we cannot prove
		// that a 3-way merge would be clean. `git apply --3way --check`
		// returns 0 even for merges that will leave conflict markers
		// (that's the original bug). Prefer a HONEST Blocked verdict
		// with a clear reason over an optimistic 3WayClean; reconcile
		// callers can surface the reason and the user can investigate.
		return ForwardApplyPreview{
			Verdict: ForwardApplyBlocked,
			Stderr:  fmt.Sprintf("worktree preview unavailable (%v); cannot verify 3-way merge cleanliness — refusing to guess", wtErr),
		}, nil
	}
	defer cleanup()

	apply := exec.Command("git", "apply", "--3way", "-")
	apply.Dir = wt
	apply.Stdin = strings.NewReader(patch)
	var applyErr strings.Builder
	apply.Stderr = &applyErr
	applyExit := apply.Run()

	markers := scanConflictMarkers(wt)
	stderr := strings.TrimSpace(applyErr.String())

	switch {
	case applyExit == nil && len(markers) == 0:
		return ForwardApplyPreview{Verdict: ForwardApply3WayClean, Stderr: stderr}, nil
	case len(markers) > 0:
		return ForwardApplyPreview{
			Verdict:       ForwardApply3WayConflicts,
			ConflictFiles: markers,
			Stderr:        stderr,
		}, nil
	default:
		return ForwardApplyPreview{Verdict: ForwardApplyBlocked, Stderr: stderr}, nil
	}
}

// mkPreviewWorktree provisions a detached linked worktree at HEAD and
// returns its path plus a cleanup func. Safe to call concurrently
// because each invocation uses a unique temp directory.
func mkPreviewWorktree(repoRoot string) (string, func(), error) {
	dir, err := os.MkdirTemp("", "tpatch-preview-*")
	if err != nil {
		return "", nil, err
	}
	add := exec.Command("git", "worktree", "add", "--detach", "-q", dir, "HEAD")
	add.Dir = repoRoot
	var addErr strings.Builder
	add.Stderr = &addErr
	if err := add.Run(); err != nil {
		os.RemoveAll(dir)
		return "", nil, fmt.Errorf("git worktree add: %v: %s", err, strings.TrimSpace(addErr.String()))
	}
	cleanup := func() {
		rm := exec.Command("git", "worktree", "remove", "--force", dir)
		rm.Dir = repoRoot
		_ = rm.Run()
		os.RemoveAll(dir)
	}
	return dir, cleanup, nil
}

// scanConflictMarkers walks the worktree looking for files that contain
// `<<<<<<<` at the start of a line (the canonical git merge marker).
// Returns repo-relative paths sorted alphabetically.
// ScanConflictMarkers walks root looking for files that contain git
// conflict markers (`<<<<<<<` and `>>>>>>>` on line starts). Skips
// `.git`, files larger than 5MB (binary-ish), and any read errors.
// Returns repo-relative paths, sorted. Safe to call on the main
// working tree as a defensive last-line check; reconcile uses it to
// detect a conflict-markers-but-reapplied false positive.
func ScanConflictMarkers(root string) []string {
	return scanConflictMarkers(root)
}

func scanConflictMarkers(root string) []string {
	var out []string
	_ = filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			if info.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if info.Size() > 5*1024*1024 { // skip > 5MB binaries/assets
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return nil
		}
		if bytesHasLine(data, "<<<<<<<") && bytesHasLine(data, ">>>>>>>") {
			rel, relErr := filepath.Rel(root, p)
			if relErr == nil {
				out = append(out, rel)
			}
		}
		return nil
	})
	sort.Strings(out)
	return out
}

// HasConflictMarkers reports whether data contains both `<<<<<<<`
// and `>>>>>>>` at the start of any line — the canonical signature of
// an unresolved git merge. Exported so the phase-3.5 validation gate
// can check a single file's in-memory content without walking a tree.
func HasConflictMarkers(data []byte) bool {
	return bytesHasLine(data, "<<<<<<<") && bytesHasLine(data, ">>>>>>>")
}

// bytesHasLine reports whether data contains prefix at the start of any
// line. Avoids allocating a string for large files.
func bytesHasLine(data []byte, prefix string) bool {
	if len(data) == 0 || len(prefix) == 0 {
		return false
	}
	p := []byte(prefix)
	// Start of file.
	if len(data) >= len(p) && bytesEq(data[:len(p)], p) {
		return true
	}
	for i := 0; i+1+len(p) <= len(data); i++ {
		if data[i] == '\n' && bytesEq(data[i+1:i+1+len(p)], p) {
			return true
		}
	}
	return false
}

func bytesEq(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// ForwardApply applies a patch. Uses 3-way merge if strict apply fails.
func ForwardApply(repoRoot, patch string) error {
	cmd := exec.Command("git", "apply", "-")
	cmd.Dir = repoRoot
	cmd.Stdin = strings.NewReader(patch)
	out, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}
	// Retry with 3-way merge
	cmd = exec.Command("git", "apply", "--3way", "-")
	cmd.Dir = repoRoot
	cmd.Stdin = strings.NewReader(patch)
	out, err = cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git apply failed: %s: %w", string(out), err)
	}
	return nil
}

// FetchUpstream fetches from a remote ref.
func FetchUpstream(repoRoot, remote string) error {
	_, err := runGit(repoRoot, "fetch", remote)
	return err
}

// DiffBetween returns the diff between two refs.
func DiffBetween(repoRoot, fromRef, toRef string) (string, error) {
	return runGit(repoRoot, "diff", fromRef, toRef)
}

// ResolveRef resolves a ref to its commit hash.
func ResolveRef(repoRoot, ref string) (string, error) {
	out, err := runGit(repoRoot, "rev-parse", ref)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// FileAtCommit returns the contents of a repo-relative path as it
// existed at a given commit. Used by phase 3.5 to build ConflictInput
// slices (base/theirs). If the file did not exist at that commit, the
// returned bytes are nil and the error is nil — callers treat a missing
// side as empty for three-way reconciliation purposes (git does the
// same). Any other git failure is returned verbatim.
func FileAtCommit(repoRoot, commit, relPath string) ([]byte, error) {
	// `git show <commit>:<path>` prints the blob to stdout.
	cmd := exec.Command("git", "show", commit+":"+relPath)
	cmd.Dir = repoRoot
	out, err := cmd.Output()
	if err != nil {
		// "exists on disk, but not in <commit>" or "does not exist in
		// <commit>" manifest as non-zero exit — treat as absent.
		if _, ok := err.(*exec.ExitError); ok {
			return nil, nil
		}
		return nil, err
	}
	return out, nil
}

// MergeBase returns the merge-base (common ancestor) of two commits.
// Used as the "base" side of the three-way reconciliation triple. If
// no common ancestor exists (disjoint histories), the returned commit
// is empty and err is non-nil.
func MergeBase(repoRoot, a, b string) (string, error) {
	out, err := runGit(repoRoot, "merge-base", a, b)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// FilesInPatch returns the unique set of repo-relative file paths
// touched by a unified diff, parsed from `diff --git a/<path> b/<path>`
// headers. Paths are returned in first-seen order so the output is
// stable. Used by `reconcile --accept` to know which paths to restrict
// the regenerated post-apply.patch to.
func FilesInPatch(patch string) []string {
	seen := map[string]bool{}
	var out []string
	for _, line := range strings.Split(patch, "\n") {
		if !strings.HasPrefix(line, "diff --git ") {
			continue
		}
		// `diff --git a/<p1> b/<p2>` — we take the b-side since new
		// files have /dev/null on the a-side. Handle quoted paths
		// loosely; git doesn't quote unless the path needs it.
		parts := strings.SplitN(line, " b/", 2)
		if len(parts) != 2 {
			continue
		}
		p := strings.TrimSpace(parts[1])
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	return out
}

// ForwardApplyExcluding applies a patch but skips the listed
// repo-relative paths entirely. Uses `git apply --3way` with
// `--exclude=<path>` so that non-excluded hunks land cleanly (or via
// 3-way merge) while the excluded paths are left untouched for a
// subsequent explicit overwrite step (e.g., CopyShadowToReal after
// phase 3.5). Returns an error only if the non-excluded portion of
// the patch fails to apply.
func ForwardApplyExcluding(repoRoot, patch string, excludePaths []string) error {
	args := []string{"apply", "--3way"}
	for _, p := range excludePaths {
		args = append(args, "--exclude="+p)
	}
	args = append(args, "-")
	cmd := exec.Command("git", args...)
	cmd.Dir = repoRoot
	cmd.Stdin = strings.NewReader(patch)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git apply (excluding %d path(s)) failed: %s: %w",
			len(excludePaths), strings.TrimSpace(string(out)), err)
	}
	return nil
}

// DiffFromCommitForPaths returns `git diff <commit> -- <paths...>`
// against the working tree. Empty paths slice means the full diff.
// Before running the diff, untracked paths in the provided list are
// marked via `git add -N` (intent-to-add) so they appear in the diff
// output; this mirrors how `tpatch record` handles newly created
// files. Used by the reconcile --accept derived-refresh flow to
// regenerate post-apply.patch after the accepted resolution.
func DiffFromCommitForPaths(repoRoot, commit string, paths []string) (string, error) {
	// Intent-to-add any paths git doesn't yet know about. `git add -N`
	// is silent on already-tracked paths, so we can pass the full list.
	if len(paths) > 0 {
		addArgs := append([]string{"add", "-N", "--"}, paths...)
		if _, err := runGit(repoRoot, addArgs...); err != nil {
			// Non-fatal: the intent-to-add only helps NEW files; if
			// git refuses (e.g., file doesn't exist), the diff call
			// below will still see tracked-file changes. Continue.
			_ = err
		}
	}
	args := []string{"diff", commit}
	if len(paths) > 0 {
		args = append(args, "--")
		args = append(args, paths...)
	}
	out, err := runGit(repoRoot, args...)
	if err != nil {
		return "", err
	}
	result := strings.TrimSpace(out)
	if result != "" {
		result += "\n"
	}
	return result, nil
}

// DeriveIncrementalPatch computes the diff that only contains one feature's changes,
// given the cumulative patches for the previous features and the current feature.
// prevCumulativePatch = everything up to (but not including) this feature.
// currentCumulativePatch = everything up to and including this feature.
// Returns only this feature's changes (the delta).
func DeriveIncrementalPatch(repoRoot, baseCommit, prevCumulativePatch, currentCumulativePatch string) (string, error) {
	// Create temp dirs
	tmpDir, err := os.MkdirTemp("", "tpatch-incremental-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmpDir)

	prevDir := filepath.Join(tmpDir, "prev")
	currDir := filepath.Join(tmpDir, "curr")

	// Clone base state into both dirs
	for _, dir := range []string{prevDir, currDir} {
		if _, err := runGit(".", "clone", "--no-checkout", repoRoot, dir); err != nil {
			return "", fmt.Errorf("clone failed: %w", err)
		}
		if _, err := runGit(dir, "checkout", baseCommit); err != nil {
			return "", fmt.Errorf("checkout %s failed: %w", baseCommit, err)
		}
	}

	// Apply previous features' cumulative patch to prevDir
	if prevCumulativePatch != "" {
		cmd := exec.Command("git", "apply", "--3way", "-")
		cmd.Dir = prevDir
		cmd.Stdin = strings.NewReader(prevCumulativePatch)
		cmd.Run() // best-effort
	}

	// Apply current features' cumulative patch to currDir
	if currentCumulativePatch != "" {
		cmd := exec.Command("git", "apply", "--3way", "-")
		cmd.Dir = currDir
		cmd.Stdin = strings.NewReader(currentCumulativePatch)
		cmd.Run() // best-effort
	}

	// Diff the two: this gives only the incremental changes for this feature
	cmd := exec.Command("diff", "-ruN", prevDir, currDir)
	out, _ := cmd.Output()
	result := string(out)

	// Fix paths: replace temp dir paths with relative paths
	result = strings.ReplaceAll(result, prevDir+"/", "a/")
	result = strings.ReplaceAll(result, currDir+"/", "b/")

	trimmed := strings.TrimSpace(result)
	if trimmed != "" {
		trimmed += "\n"
	}
	return trimmed, nil
}

func runGit(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return string(out), fmt.Errorf("git %s: %s", strings.Join(args, " "), string(exitErr.Stderr))
		}
		return string(out), err
	}
	return string(out), nil
}
