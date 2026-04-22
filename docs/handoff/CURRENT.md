# Current Handoff

## Active Task

- **Task ID**: M12 / Tranche B2 / v0.5.0 — Provider-Assisted Conflict Resolver
- **Status**: In progress. **3 of 10 b2 todos done** (shadow + validation + resolver-core). Next: `b2-reconcile-wiring`.
- **PRD**: `docs/prds/PRD-provider-conflict-resolver.md`
- **Milestone**: `docs/milestones/M12-provider-conflict-resolver.md`
- **ADR**: `docs/adrs/ADR-010-provider-conflict-resolver.md`

### PRD-locked answers to ADR-010's 6 open questions

1. Sequential per-file calls (no parallelism in v0.5.0).
2. Single combined `patches/NNN-reconcile.patch`; per-file audit in `reconcile-session.json`.
3. `test_command` is the `--apply` gate; refused if unset.
4. One shadow per feature; reap prior on new resolve.
5. Full file with `max_file_bytes` cap (default 200 KB); oversize → `skipped-too-large`.
6. Golden scenarios harness ships with v0.5.0 (≥5 scenarios).

### B2 progress

| Todo | Status | Commit | File(s) |
|---|---|---|---|
| `b2-shadow-worktree` | ✅ done | `8bd8eb6` | `internal/gitutil/shadow.go` + test |
| `b2-validation-gate` | ✅ done | `bf28b58` | `internal/workflow/validation.go` + test; `gitutil.HasConflictMarkers` exported |
| `b2-resolver-core` | ✅ done | `25b7774` | `internal/workflow/resolver.go` + test |
| `b2-reconcile-wiring` | ⏭️ NEXT | — | modify `internal/workflow/reconcile.go` |
| `b2-state-machine` | blocked on wiring | — | `reconciling-shadow` state + `reconcile-session.json` schema |
| `b2-cli-flags` | blocked on wiring+state | — | `--resolve --apply --accept --reject --shadow-diff --max-conflicts --model` |
| `b2-derived-refresh` | blocked on state | — | `store.RefreshDerivedArtifacts` atomic on accept |
| `b2-golden-tests` | unblocked (parallel) | — | `tests/reconcile/golden/` ≥5 scenarios |
| `b2-skills-update` | blocked on cli+refresh | — | 6 skills + docs/agent-as-provider.md |
| `b2-release` | blocked on skills+golden | — | v0.5.0 tag |

SQL: `SELECT id, status FROM todos WHERE id LIKE 'b2-%' ORDER BY id;`

### What `b2-reconcile-wiring` needs to do

Inject phase 3.5 between the existing phase-3 (provider semantic check) and phase-4 (forward-apply) in `internal/workflow/reconcile.go`. Specifically:

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
