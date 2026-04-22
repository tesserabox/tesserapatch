# Current Handoff

## Active Task

- **Task ID**: M13 / Tranche C1 / v0.5.1 — UX Polish & Quick Wins
- **Status**: 🔨 **In Progress — scoped, implementation prompt ready**
- **Milestone**: (inline — no separate milestone file for polish tranches)
- **Previous**: M12 / B2 / v0.5.0 shipped ✅ — archived below

### C1 scope (8 items, all low-risk)

| Todo ID | Type | Description |
|---------|------|-------------|
| `c1-apply-default-execute` | feat | `tpatch apply <slug>` without `--mode` runs prepare→execute→done in one shot; keep `--mode` for granular control |
| `c1-add-stdin` | feat | `tpatch add -` or pipe detection reads feature description from stdin |
| `c1-progress-indicator` | feat | Lightweight stderr spinner during LLM calls (zero-dep, stdlib only) |
| `c1-edit-flag` | feat | `tpatch edit <slug> [artifact]` opens feature artifacts in `$EDITOR` |
| `c1-feature-amend` | feat | `tpatch amend <slug> <new-description>` updates request.md, optionally resets state |
| `c1-feature-removal` | feat | `tpatch remove <slug> [--force]` deletes feature directory with confirmation |
| `c1-recipe-stale-guard` | bug | Warn when `apply-recipe.json` base commit doesn't match current HEAD |
| `c1-record-lenient` | bug | `tpatch record --lenient` skips reverse-apply check for whitespace-sensitive files |

### B2 progress

| Todo | Status | Commit | File(s) |
|---|---|---|---|
| `b2-shadow-worktree` | ✅ done | `8bd8eb6` | `internal/gitutil/shadow.go` + test |
| `b2-validation-gate` | ✅ done | `bf28b58` | `internal/workflow/validation.go` + test; `gitutil.HasConflictMarkers` exported |
| `b2-resolver-core` | ✅ done | `25b7774` | `internal/workflow/resolver.go` + test |
| `b2-reconcile-wiring` | ✅ done | `53b38ee` | `internal/workflow/reconcile.go` + `gitutil.FileAtCommit`/`MergeBase` + test |
| `b2-state-machine` | ✅ done | (this commit) | `StateReconcilingShadow` + `ReconcileSummary` shadow fields + `status` command surfaces shadow pointer + test |
| `b2-cli-flags` | ✅ done | `c022b19` | `reconcileCmd` + 7 flags + accept/reject/shadow-diff handlers + `validateReconcileFlags` + 2 tests |
| `b2-derived-refresh` | ✅ done | `1507b7a` | `FilesInPatch`/`ForwardApplyExcluding`/`DiffFromCommitForPaths` + `RefreshAfterAccept` + accept flow rewired + 4 tests |
| `b2-golden-tests` | ✅ done | (this commit) | `golden_reconcile_test.go` — 5 ADR-010 acceptance scenarios (clean-reapply / shadow-awaiting / validation-failed / too-many-conflicts / no-provider) |
| `b2-skills-update` | ✅ done | (this commit) | 6 skills + `docs/agent-as-provider.md` — Phase 3.5 section, `--resolve/--apply/--accept/--reject/--shadow-diff/--max-conflicts/--model` flags, `reconciling-shadow` state, `reconcile-session.json` schema, shadow worktree concept; parity guard green |
| `b2-release` | ✅ done | (this commit) | v0.5.0: version bump in `cobra.go`, CHANGELOG entry, git tag pushed |

SQL: `SELECT id, status FROM todos WHERE id LIKE 'b2-%' ORDER BY id;`

### What `b2-cli-flags` needs to do (NEXT)

Add flags to `reconcileCmd` in `internal/cli/cobra.go`:

- `--resolve` bool → `ReconcileOptions.Resolve`
- `--apply` bool → `ReconcileOptions.Apply` (requires `--resolve`)
- `--max-conflicts N` int → `ReconcileOptions.MaxConflicts`
- `--model NAME` string → `ReconcileOptions.Model`
- `--accept <slug>`, `--reject <slug>`, `--shadow-diff <slug>` — terminal operations; read `status.Reconcile.ShadowPath` (already populated by b2-state-machine). Mutually exclusive with `--resolve`.

Handler sketch:

- `--accept`: refuse if state != `reconciling-shadow`. Look up resolved_files from `reconcile-session.json`. Call `gitutil.CopyShadowToReal(shadow, root, files)`. Transition state to `applied` via `s.MarkFeatureState`. Add TODO note: "derived artifacts not yet refreshed — run `tpatch record` until b2-derived-refresh lands."
- `--reject`: `gitutil.PruneShadow(shadow)`. Roll state back to `applied`. Clear `status.Reconcile.ShadowPath`.
- `--shadow-diff`: walk resolved_files, shell out to `diff -u` per pair, stream to stdout.

Also: truthful validation errors for nonsensical combos (e.g. `--accept` + `--resolve`).

### What was in the old wiring guidance (preserved below for reference — all implemented)

1. **Trigger condition**: only when `PreviewForwardApply` returns `ForwardApply3WayConflicts` AND the caller set `ReconcileOpts.Resolve = true` (new field — add to the opts struct).
2. **Git plumbing** (new, needs a helper in gitutil or inline): for each conflicted file from the preview, fetch three versions:
   - `base` = file at the feature's base upstream commit (from `upstream.lock` or the patch's base)
   - `ours` = file after feature's patch is applied on `base` (either read from real working tree if currently on base+patch, OR synthesize: `git show <base>:<path>` + apply feature's post-apply.patch selectively).
   - `theirs` = `git show <upstreamCommit>:<path>`
   - Simplest v0.5.0 approach: use `git show <ref>:<path>` via `runGit` for base and theirs; for ours, read the file from the real working tree (reconcile runs after `tpatch apply` has put the feature on disk). Document the assumption.
3. **Call `RunConflictResolve`** with the gathered `ConflictInput`s and `upstreamCommit`. Pass through `ResolveOptions{AutoApply: opts.Apply, ModelOverride: opts.Model, MaxConflicts: opts.MaxConflicts, Validation: ValidationConfig{TestCommand: cfg.TestCommand, IdentifierCheck: true}}`.
4. **Map `ResolveResult` → `ReconcileResult`**: new `ReconcileOutcome` values mirror the resolver verdicts. Add `ShadowPath`, `ResolvedFiles`, `FailedFiles`, `SkippedFiles` to `ReconcileResult`.
5. **Preserve v0.4.4 `promoteIfMarkers`** on every Reapplied path that bypasses phase 3.5 (when `--resolve` is off). Already present; just make sure new branching doesn't orphan it.
6. **Skip phase 3.5 entirely** when forward-apply preview verdict is anything other than `3WayConflicts` — the resolver only exists to turn that verdict into something actionable.

### Key technical facts (for a fresh agent)

- **Module path**: `github.com/tesseracode/tesserapatch` (renamed from `tesserabox` on 2026-04-21).
- **Provider interface**: `provider.Provider{ Check, Generate }`. Resolver uses `Generate` only. `cfg.Configured()` is the "usable?" check.
- **Store API**: `s.ReadFeatureFile(slug, name)`, `s.WriteArtifact(slug, name, content)`, `s.LoadConfig()`, `s.Root` (repo root). Flat YAML config.
- **Shadow path**: `.tpatch/shadow/<slug>-<ts>/` where ts is `2006-01-02T15-04-05.000000Z`. Microsecond precision — required to avoid collisions on rapid recreate.
- **No heuristic fallback** (ADR-010 D9): when provider not configured, resolver returns `BlockedRequiresHuman` with per-file `provider-error` status. Never degrade silently.
- **Fence stripping**: use `stripResolverFences` (conservative whole-response regex), NOT `stripCodeFences` (JSON-lenient). Documented in resolver.go.
- **Validation**: `ValidateResolvedFile` runs markers + native-parse + identifier-preservation (opt-in). `RunTestCommandInShadow` is a SEPARATE call, run after all files resolve.
- **Session JSON**: written on EVERY path, including short-circuit verdicts (too-many-conflicts, no-provider). Auditability > optimization.
- **Parity guard**: `assets/assets_test.go` has `TestSkillRecipeSchemaMatchesCLI` with `DisallowUnknownFields`. Any skill edit that invents a field fails build. B2 skill update must extend the anchors + recipe schema carefully.

### Follow-ups registered (post-B2, later tranches)

- `feat-resolver-heuristic-fallback` — opt-in `--heuristic` for provider-unavailable cases. Depends on `b2-release`.
- `feat-feature-standalonify` — rebase a dependent feature into standalone. Depends on `feat-feature-dependencies`.
- `feat-parallel-feature-workflows` — `tpatch workon --parallel` fans out features into per-feature worktrees. Depends on `feat-feature-dependencies`.

### Bugs fixed in v0.5.0 alongside B2

- `bug-features-md-stale-state` — `FEATURES.md` not regenerated on state transitions from `apply --mode done` / `record` / etc. Fix: `SaveFeatureStatus` now calls `RefreshFeaturesIndex` unconditionally. Regression test: `TestSaveFeatureStatusRefreshesIndex`.

## Session Summary (2026-04-22 session — B2 derived-refresh + golden-tests)

**Commits this session** (continuing):
- `c022b19` — b2-cli-flags (prior)
- `3aab0c4` — docs checkpoint (prior)
- `1507b7a` — **b2-derived-refresh**: accept-flow correctness fix + atomic post-apply.patch regen + numbered reconcile patch + 4 tests
- (this commit) — **b2-golden-tests**: 5 ADR-010 PRD#6 acceptance scenarios

All pushed. `gofmt`, `go vet`, `go test ./...` clean.

### `b2-derived-refresh` fixed a real bug

The prior `--accept` only copied resolved (conflicted) files from the shadow.
Non-conflicted hunks from `post-apply.patch` were **never applied** to the real
tree, leaving the feature half-reconciled. New accept flow:

1. `ForwardApplyExcluding(patch, resolvedFiles)` — non-conflicted hunks land via 3-way
2. `CopyShadowToReal(resolvedFiles)` — resolver output overlays those paths
3. `RefreshAfterAccept` — regenerates post-apply.patch restricted to originally-touched files (via `git diff <upstreamCommit> -- <paths>` with `git add -N` first so new files appear); snapshots new patch as `patches/NNN-reconcile.patch`
4. `MarkFeatureState → applied`; prune shadow; clear status pointer

Explicitly deferred: `apply-recipe.json` regen (lossy from a raw diff);
documented in `refresh.go`. `tpatch record` remains the fallback.

### `b2-golden-tests` — 5 scenarios via `RunReconcile`

File: `internal/workflow/golden_reconcile_test.go`

| Scenario | Fixture | Expected outcome |
|---|---|---|
| clean-reapply | Non-conflicting feature vs unchanged upstream | `reapplied` / `upstreamed`, no shadow |
| shadow-awaiting | Conflict + provider returns clean merge | `shadow-awaiting`, 1 resolved, shadow populated |
| validation-failed | Conflict + provider returns content with `<<<<<<<` markers | `blocked-requires-human`, 1 failed |
| too-many-conflicts | 2 conflicted files, MaxConflicts=1 | `blocked-too-many-conflicts`, provider.calls==0 |
| no-provider | Conflict + nil provider + `--resolve` | `blocked-requires-human`, no shadow |

Pattern reuses `scriptedProvider` with `keyed` map for resolver calls + positional response for phase-3 semantic probe. Fixtures capture real `git diff --cached HEAD` output so `--3way` can locate the base blob.

## Session Summary (2026-04-22 session — B2 cli-flags)

**Commits this session** (continuing from b2-state-machine):
- `53b38ee` — `b2-reconcile-wiring` (prior)
- `1767c1d` — `b2-state-machine` (prior)
- `6229203` — docs checkpoint (prior)
- (this commit) — `b2-cli-flags`: 7 new `tpatch reconcile` flags + 3 terminal handlers (accept/reject/shadow-diff) + mutex validation + 2 tests

All pushed. All tests green. `gofmt`, `go vet` clean.

### What `b2-cli-flags` shipped

- `--resolve`, `--apply`, `--max-conflicts`, `--model` → wired into `ReconcileOptions` struct
- `--accept <slug>`: reads `reconcile-session.json`, copies resolved files via `gitutil.CopyShadowToReal`, transitions state to `applied`, prunes shadow, clears status pointer. TODO emitted pointing to `tpatch record` (derived-refresh deferred)
- `--reject <slug>`: prunes shadow, rolls state back to `applied` if parked in `reconciling-shadow`
- `--shadow-diff <slug>`: non-destructive; streams `gitutil.ShadowDiff` to stdout
- `validateReconcileFlags`: rejects terminal-op combos + `--apply` without `--resolve`
- Safety: terminal ops never call `openStoreFromCmd` before flag validation

## Session Summary (2026-04-22 session — B2 middle)

**Commits this session** (continuing from B2 kickoff):
- `ed8457b` — docs: checkpoint B2 progress in CURRENT.md
- `53b38ee` — `b2-reconcile-wiring` (reconcile.go + gitutil.FileAtCommit/MergeBase + 1 test)
- `1767c1d` — `b2-state-machine` (StateReconcilingShadow + ReconcileSummary fields + status surface + 1 test)

All pushed to origin/main. All tests green.

## Session Summary (2026-04-21 evening session — B2 kickoff)

**Commits this session** (post-v0.4.4):
- `a6bd734` — docs: scope M12 / Tranche B2 (PRD + milestone + ROADMAP + CURRENT)
- `8bd8eb6` — `b2-shadow-worktree` (gitutil/shadow.go + 7 tests)
- `bf28b58` — `b2-validation-gate` (workflow/validation.go + 10 tests; gitutil.HasConflictMarkers exported)
- `25b7774` — `b2-resolver-core` (workflow/resolver.go + 6 tests)

All green: `gofmt -l .` clean, `go vet ./...` clean, `go test ./...` pass.

---

## Prior session summary (v0.4.4 + org rename)

Two HIGH bugs from the t3code v0.4.3 live stress test fixed and shipped.

1. **Skill recipe schema mismatch** — v0.4.3 skills documented `op`/`contents`/`occurrences`/`delete-file`; CLI reads `type`/`content`/no-occurrences/no-delete-file. Corrected all 6 skills + `docs/agent-as-provider.md`. Added `TestSkillRecipeSchemaMatchesCLI` — extracts every ```json block, unmarshals into `workflow.RecipeOperation` with `DisallowUnknownFields`, and validates op types. Prevents future drift.

2. **Reconcile reapplied-with-conflict-markers** — the degraded `PreviewForwardApply` fallback used to return `3WayClean` when `git worktree add` failed, undoing v0.4.2 A4. Now returns `Blocked`. Added `ScanConflictMarkers` defensive pass on the live tree after every Reapplied verdict; markers promote to Blocked. New test `TestReconcilePromotesOnLiveMarkers`.

Both bugs were direct B2 prerequisites (agents need a correct recipe schema; B2's resolver hooks on `3WayConflicts` which phase 4 was silently skipping).

## Files Changed

- `assets/skills/claude/tessera-patch/SKILL.md` — recipe schema block rewritten (`type`/`content`, append-file documented, delete-file/occurrences disclaimer).
- `assets/skills/copilot/tessera-patch/SKILL.md`, `assets/prompts/copilot/tessera-patch-apply.prompt.md`, `assets/skills/cursor/tessera-patch.mdc`, `assets/skills/windsurf/windsurfrules`, `assets/workflows/tessera-patch-generic.md` — recipe JSON block + semantics rewritten to match CLI.
- `docs/agent-as-provider.md` — recipe schema rewritten.
- `assets/assets_test.go` — new `TestSkillRecipeSchemaMatchesCLI`.
- `internal/gitutil/gitutil.go` — `PreviewForwardApply` degraded path returns Blocked; `ScanConflictMarkers` exported.
- `internal/workflow/reconcile.go` — `promoteIfMarkers` defensive pass on Reapplied paths.
- `internal/workflow/reconcile_test.go` — `TestReconcilePromotesOnLiveMarkers` regression.
- `internal/cli/cobra.go` — version → 0.4.4.
- `CHANGELOG.md` — v0.4.4 section.

## Test Results

- `gofmt -l .` — clean.
- `go build ./...` — ok.
- `go test ./...` — all packages pass. Two new tests green (`TestSkillRecipeSchemaMatchesCLI`, `TestReconcilePromotesOnLiveMarkers`).

## Next Steps — pick Tranche B2 scope

1. **Option A — `feat-provider-conflict-resolver`** (ADR-010, v0.5.0 headline): phase 3.5 in reconcile, shadow worktree, per-file provider call. The core value prop. Now unblocked by v0.4.4.
2. **Option B — Recipe modernisation**: `feat-recipe-schema-expansion` (add `delete-file`, `rename-file`, op aliases) + `feat-record-autogen-recipe` (derive recipe from diff on record). Makes Path B fully self-contained.
3. **Option C — `feat-feature-dependencies` DAG**: first-class depends_on plumbing; unlocks stacked features and ordered reconcile.

## Blockers

None.

## Context for Next Agent

- The new `TestSkillRecipeSchemaMatchesCLI` is strict (`DisallowUnknownFields`). Any future skill edit that invents a field will fail the build at the assets test. If the CLI adds a field (e.g. `occurrences`), update both `workflow.RecipeOperation` and the skills in the same commit.
- `ScanConflictMarkers` is now public (`gitutil.ScanConflictMarkers`). Reuse it anywhere a "did this really succeed?" check is needed (e.g. after `apply --mode execute`).
- The degraded path in `PreviewForwardApply` now refuses to guess. If users start seeing "worktree preview unavailable — refusing to guess", they have a real environment issue (bare repo, disk full, permissions) that was previously being masked.
