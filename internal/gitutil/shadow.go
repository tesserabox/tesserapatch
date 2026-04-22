// Shadow worktree plumbing for phase 3.5 of reconcile.
//
// A shadow is a throwaway `git worktree` at `.tpatch/shadow/<slug>-<ts>/`
// where the provider's conflict-resolution output is staged. Nothing
// touches the real working tree until an explicit `CopyShadowToReal`
// call (driven by `tpatch reconcile --accept`), at which point only
// the explicitly listed feature files are copied.
//
// Design (ADR-010 D2):
//   - One shadow per feature slug. Any prior shadow for the slug is
//     reaped before a new one is created — no fan-out, no cross-session
//     ambiguity.
//   - Shadow lifetime outlives a single reconcile call so a human can
//     `--shadow-diff` and then accept/reject. `PruneShadow` is the only
//     terminal operation.
//   - All writes are validated via `safety.EnsureSafeRepoPath` against
//     the repo root — a shadow sits inside `.tpatch/`, well within the
//     repo, but nothing reachable from a shadow path may escape.

package gitutil

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/tesseracode/tesserapatch/internal/safety"
)

const shadowDir = ".tpatch/shadow"

// shadowTimestampLayout matches the format written by CreateShadow. It
// is intentionally filesystem-safe (no colons) and sorts
// lexicographically by creation time. Microsecond precision avoids
// collisions when reconcile is retried quickly.
const shadowTimestampLayout = "2006-01-02T15-04-05.000000Z"

// Shadow describes an existing shadow worktree on disk.
type Shadow struct {
	Slug      string    // feature slug
	Path      string    // absolute path to the shadow root
	CreatedAt time.Time // parsed from the directory name
}

// CreateShadow provisions a new shadow worktree for slug rooted at
// commit (typically the new upstream HEAD). Any prior shadow for the
// same slug is pruned first so callers never see stale state from a
// previous reconcile run.
//
// Returns the absolute shadow path on success.
func CreateShadow(repoRoot, slug, commit string) (string, error) {
	if slug == "" {
		return "", fmt.Errorf("shadow: slug is required")
	}
	if commit == "" {
		return "", fmt.Errorf("shadow: commit is required")
	}
	if err := PruneAllShadows(repoRoot, slug); err != nil {
		return "", fmt.Errorf("shadow: prune prior: %w", err)
	}

	absRoot, err := filepath.Abs(repoRoot)
	if err != nil {
		return "", fmt.Errorf("shadow: abs repo root: %w", err)
	}
	shadowRoot := filepath.Join(absRoot, shadowDir)
	if err := os.MkdirAll(shadowRoot, 0o755); err != nil {
		return "", fmt.Errorf("shadow: mkdir %s: %w", shadowRoot, err)
	}

	ts := time.Now().UTC().Format(shadowTimestampLayout)
	dirName := fmt.Sprintf("%s-%s", slug, ts)
	path := filepath.Join(shadowRoot, dirName)

	// Refuse to clobber an existing directory — the timestamp makes a
	// collision nearly impossible, but be defensive.
	if _, err := os.Stat(path); err == nil {
		return "", fmt.Errorf("shadow: path already exists: %s", path)
	}
	if err := safety.EnsureSafeRepoPath(repoRoot, path); err != nil {
		return "", fmt.Errorf("shadow: unsafe path: %w", err)
	}

	add := exec.Command("git", "worktree", "add", "--detach", "-q", path, commit)
	add.Dir = absRoot
	var stderr strings.Builder
	add.Stderr = &stderr
	if err := add.Run(); err != nil {
		// Clean up partial state on failure.
		_ = os.RemoveAll(path)
		return "", fmt.Errorf("git worktree add: %v: %s", err, strings.TrimSpace(stderr.String()))
	}
	return path, nil
}

// ResolveShadow returns the most recently created shadow for slug, or
// (nil, nil) if no shadow exists. Multiple shadows for the same slug
// should never exist under normal operation (CreateShadow reaps priors)
// but this tolerates leftovers from a crashed run.
func ResolveShadow(repoRoot, slug string) (*Shadow, error) {
	shadows, err := listShadows(repoRoot, slug)
	if err != nil {
		return nil, err
	}
	if len(shadows) == 0 {
		return nil, nil
	}
	// listShadows returns chronologically sorted; take the newest.
	s := shadows[len(shadows)-1]
	return &s, nil
}

// PruneShadow removes a specific shadow worktree for slug. Uses
// `git worktree remove --force` to detach git's bookkeeping, then
// deletes any residue. Returns nil if no shadow exists for the slug
// (prune is idempotent).
func PruneShadow(repoRoot, slug string) error {
	sh, err := ResolveShadow(repoRoot, slug)
	if err != nil {
		return err
	}
	if sh == nil {
		return nil
	}
	return pruneShadowPath(repoRoot, sh.Path)
}

// PruneAllShadows removes every shadow associated with slug. Used by
// CreateShadow to guarantee a clean slate and by `--reject` for
// thoroughness.
func PruneAllShadows(repoRoot, slug string) error {
	shadows, err := listShadows(repoRoot, slug)
	if err != nil {
		return err
	}
	for _, sh := range shadows {
		if err := pruneShadowPath(repoRoot, sh.Path); err != nil {
			return err
		}
	}
	return nil
}

// CopyShadowToReal copies a set of feature-relative files from the
// shadow worktree into the real working tree at repoRoot. Paths must
// be repo-relative. Each resolved destination is validated via
// `safety.EnsureSafeRepoPath` before any write. Parent directories are
// created as needed.
//
// This is an atomicity boundary: on any failure, callers should treat
// the working tree as potentially partially written. For a truly
// atomic accept, wrap in a higher-level "copy to temp, rename into
// place" step in the caller — v0.5.0 trades this for simplicity.
func CopyShadowToReal(repoRoot, slug string, files []string) error {
	sh, err := ResolveShadow(repoRoot, slug)
	if err != nil {
		return err
	}
	if sh == nil {
		return fmt.Errorf("shadow: no shadow exists for slug %q", slug)
	}

	absRoot, err := filepath.Abs(repoRoot)
	if err != nil {
		return fmt.Errorf("shadow: abs repo root: %w", err)
	}

	for _, rel := range files {
		rel = filepath.ToSlash(rel)
		if rel == "" || strings.HasPrefix(rel, "/") || strings.Contains(rel, "..") {
			return fmt.Errorf("shadow: unsafe relative path %q", rel)
		}
		src := filepath.Join(sh.Path, filepath.FromSlash(rel))
		dst := filepath.Join(absRoot, filepath.FromSlash(rel))

		// Refuse to write into .git/ or any path outside the repo.
		if err := safety.EnsureSafeRepoPath(absRoot, dst); err != nil {
			return fmt.Errorf("shadow: copy %s: %w", rel, err)
		}
		if insideGitDir(absRoot, dst) {
			return fmt.Errorf("shadow: refusing to write into .git/: %s", rel)
		}

		if err := copyFilePreservingMode(src, dst); err != nil {
			return fmt.Errorf("shadow: copy %s: %w", rel, err)
		}
	}
	return nil
}

// ShadowDiff returns a unified diff of the specified files as they
// currently differ between the shadow and the real working tree.
// Empty output means every file matches. Returns the raw concatenated
// diff output for human review (used by `tpatch reconcile --shadow-diff`).
func ShadowDiff(repoRoot, slug string, files []string) (string, error) {
	sh, err := ResolveShadow(repoRoot, slug)
	if err != nil {
		return "", err
	}
	if sh == nil {
		return "", fmt.Errorf("shadow: no shadow exists for slug %q", slug)
	}

	absRoot, err := filepath.Abs(repoRoot)
	if err != nil {
		return "", fmt.Errorf("shadow: abs repo root: %w", err)
	}

	var out strings.Builder
	for _, rel := range files {
		rel = filepath.ToSlash(rel)
		real := filepath.Join(absRoot, filepath.FromSlash(rel))
		shadow := filepath.Join(sh.Path, filepath.FromSlash(rel))

		// `git diff --no-index` gives us a proper unified diff with
		// labels and handles "missing on one side" gracefully.
		cmd := exec.Command("git", "diff", "--no-index", "--", real, shadow)
		cmd.Dir = absRoot
		stdout, _ := cmd.Output() // exit 1 just means "files differ"
		if len(stdout) > 0 {
			out.Write(stdout)
		}
	}
	return out.String(), nil
}

// listShadows enumerates existing shadow directories for slug, sorted
// chronologically oldest-first. Parse failures are skipped silently —
// unrecognised directory names are out-of-band garbage that shouldn't
// block reconcile.
func listShadows(repoRoot, slug string) ([]Shadow, error) {
	absRoot, err := filepath.Abs(repoRoot)
	if err != nil {
		return nil, fmt.Errorf("shadow: abs repo root: %w", err)
	}
	root := filepath.Join(absRoot, shadowDir)
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("shadow: read %s: %w", root, err)
	}
	prefix := slug + "-"
	var out []Shadow
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		stamp := strings.TrimPrefix(name, prefix)
		t, perr := time.Parse(shadowTimestampLayout, stamp)
		if perr != nil {
			continue
		}
		out = append(out, Shadow{
			Slug:      slug,
			Path:      filepath.Join(root, name),
			CreatedAt: t,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

// pruneShadowPath detaches the worktree via git, then removes any
// residue on disk. Git's own bookkeeping lives in .git/worktrees/
// and is cleaned up by `git worktree remove`.
func pruneShadowPath(repoRoot, path string) error {
	absRoot, err := filepath.Abs(repoRoot)
	if err != nil {
		return err
	}
	if err := safety.EnsureSafeRepoPath(absRoot, path); err != nil {
		return fmt.Errorf("shadow: prune %s: %w", path, err)
	}
	rm := exec.Command("git", "worktree", "remove", "--force", path)
	rm.Dir = absRoot
	// Ignore the exit code — if git refuses (e.g., the directory is no
	// longer registered), we still want the disk residue gone. The
	// subsequent RemoveAll is the authoritative cleanup.
	_ = rm.Run()
	if err := os.RemoveAll(path); err != nil {
		return fmt.Errorf("shadow: remove %s: %w", path, err)
	}
	// Best-effort: prune stale worktree registrations. Never fatal.
	prune := exec.Command("git", "worktree", "prune")
	prune.Dir = absRoot
	_ = prune.Run()
	return nil
}

// copyFilePreservingMode copies src to dst, creating parent dirs as
// needed and preserving the source file's mode. Atomic via write-then-
// rename on the same filesystem (shadow is inside the repo so this
// always holds).
func copyFilePreservingMode(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("stat %s: %w", src, err)
	}
	if info.IsDir() {
		return fmt.Errorf("copy: source is a directory: %s", src)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	tmp, err := os.CreateTemp(filepath.Dir(dst), ".tpatch-shadow-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() {
		// Best-effort cleanup if we bailed before the rename succeeded.
		_ = os.Remove(tmpPath)
	}()

	if _, err := io.Copy(tmp, in); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpPath, info.Mode()); err != nil {
		return err
	}
	return os.Rename(tmpPath, dst)
}

// insideGitDir reports whether path is inside repoRoot/.git.
func insideGitDir(repoRoot, path string) bool {
	gitDir := filepath.Join(repoRoot, ".git") + string(filepath.Separator)
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	return strings.HasPrefix(absPath, gitDir)
}
