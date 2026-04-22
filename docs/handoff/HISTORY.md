## 2026-04-22 — Tranche C1 / v0.5.1 shipped

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

## Archived 2026-04-20 — v0.4.2 Tranche A handoff (superseded by B1 --manual flag landing)

# Current Handoff

## Active Task

- **Task ID**: v0.4.2 released — Tranche A "Truthful Errors" complete
- **Milestone**: All 10 Tranche A items (A1–A10) landed + `docs/{record,feature-layout,reconcile}.md` shipped.
- **Status**: Ready to tag `v0.4.2`. No open Tranche A work.
- **Next**: Tranche B kickoff — headline is `feat-provider-conflict-resolver`. Full backlog in session SQL `todos` table (32 pending feature/improvement todos).

## Session Summary

One full v0.4.2 release cycle landed in this session:

- **A1 bug-implement-silent-fallback** — `Config.MaxTokensImplement` knob (default 16384, was hard-coded 8192). New `WarnWriter io.Writer = os.Stderr` in `internal/workflow/implement.go`; fallback emits a stderr warning naming retry count, error, raw-response path, and the config knob.
- **A2 bug-cycle-state-mismatch** — `RunImplement` writes `StateImplementing`. `assertCycleState` + `featureStateRank` check every phase transition in `internal/cli/phase2.go`.
- **A3 bug-record-validation-false-positive** — new `gitutil.ValidatePatchReverse`. Record now validates round-trip against the tree it captured from; forward validation stays for reconcile.
- **A4 bug-reconcile-phase4-false-positive** — new `gitutil.PreviewForwardApply` runs `--3way` in an isolated `git worktree` and classifies `Strict | 3WayClean | 3WayConflicts | Blocked`. Conflicts promote to `ReconcileBlocked`.
- **A5 bug-skill-invocation-clarity** — three canonical top-of-file blocks (Invocation / Phase Ordering / Preflight) in all 6 skill formats. Parity guard (`assets/assets_test.go`) enforces anchor phrases — wording can't drift.
- **A6 bug-provider-set-global** — `tpatch provider set` defaults to the global config; `--repo` for per-repo override. New `TestMain` in `internal/cli/phase2_test.go` redirects `XDG_CONFIG_HOME` so tests cannot clobber the developer's machine config.
- **A7 bug-extract-json-robustness** — one `ExtractJSONObject` helper replaces four extractors. Brace-balanced, string-aware, handles trailing prose / nested / arrays / escaped quotes / fences. 11-case table test.
- **A8 doc-record-timing** — `tpatch record` refuses clean-tree-no-`--from` with a "captured 0 bytes" diagnostic + up to 10 `git log` candidates. New helpers: `gitutil.RecentCommits`, `gitutil.IsWorkingTreeDirty`. Plus `docs/record.md` + skill one-liner.
- **A9 doc-patches-vs-artifacts** — `docs/feature-layout.md` with the "canonical vs audit trail" callout. `tpatch record` prints a cleanup hint past 6 patches. CLI subcommand (`tpatch patches`) + dedup deferred to v0.5.x (`feat-patches-subcommand`, `feat-record-dedup-patches`).
- **A10 doc-reconcile-workflow** — new `gitutil.PreflightReconcile` + `ReconcilePreflight` struct. `tpatch reconcile` refuses dirty trees / conflict markers / `*.orig|*.rej`. New flags: `--preflight`, `--allow-dirty`. Untracked-`.tpatch/` tip. `docs/reconcile.md` + skill one-liner.

### Version / release

- `internal/cli/cobra.go`: `const version = "0.4.2"`.
- `CHANGELOG.md`: new file, v0.4.2 section written.
- Commit + tag `v0.4.2` pending at time of handoff write.

## Files Changed (net vs v0.4.1)

New files:
- `CHANGELOG.md`
- `docs/record.md`
- `docs/feature-layout.md`
- `docs/reconcile.md`
- `internal/workflow/jsonextract.go` + `jsonextract_test.go`
- `internal/workflow/implement_test.go` (A1/A2)
- `internal/gitutil/gitutil_test.go` (A3/A4/A10)

Substantial edits:
- `internal/cli/cobra.go` — record empty-capture refusal, reconcile preflight + flags, `providerSetCmd` global default, version bump.
- `internal/cli/phase2.go` — `assertCycleState`, `featureStateRank`.
- `internal/cli/phase2_test.go` — `TestMain` XDG isolation, 3 new regression tests.
- `internal/gitutil/gitutil.go` — `ValidatePatchReverse`, `PreviewForwardApply`, `RecentCommits`, `IsWorkingTreeDirty`, `IsPathTracked`, `PreflightReconcile`.
- `internal/workflow/implement.go` — `WarnWriter`, state transition fix, MaxTokens knob, `ExtractJSONObject` migration.
- `internal/workflow/workflow.go`, `retry.go`, `reconcile.go` — migrated to `ExtractJSONObject`.
- `internal/store/{types,store,global}.go` — `MaxTokensImplement` knob.
- All 6 skill files (Claude / Copilot / Cursor / Windsurf / Generic / prompt) — 3 canonical blocks + 2 one-liners (record timing, reconcile clean tree).
- `assets/assets_test.go` — `requiredAnchors` list (10 anchors total).

## Test Results

```
$ gofmt -l .
(clean)

$ go build ./cmd/tpatch
(clean)

$ go test ./...
ok  	.../assets              0.469s
ok  	.../internal/cli        0.945s
ok  	.../internal/gitutil    1.486s
ok  	.../internal/provider   (cached)
ok  	.../internal/safety     (cached)
ok  	.../internal/store      (cached)
ok  	.../internal/workflow   2.124s
```

## Next Steps

1. Single commit with all v0.4.2 changes + co-author trailer; tag `v0.4.2`; push.
2. Begin Tranche B. Top of the backlog: **`feat-provider-conflict-resolver`** — a dedicated LLM-assisted resolver that can process phase-4 3-way conflicts instead of bubbling them up as `blocked`. Natural fit with `feat-soft-recipe-mode` (guidance recipes reconcile more easily).
3. Secondary Tranche B candidates (from session SQL):
   - `feat-feature-amend` — amend an already-recorded feature from an in-tree edit.
   - `feat-noncontiguous-feature-commits` — per-feature commit ledger for features that span discontiguous commits.
   - `feat-init-skill-drift` — apt/dpkg-style skill reconciliation on re-init.
   - `feat-max-tokens-uncapped` — research OpenRouter / LiteLLM / OpenCode conventions before deciding.
4. Stretch (v0.6.0): `feat-ci-cd-integration`, `feat-autoresearch-iterate-until-green`, `feat-delivery-modes`.

## Blockers

None.

## Context for Next Agent

- Session SQL is the authoritative task tracker. 29 pending todos, 49 done at this point.
- All three new docs in `docs/` (`record.md`, `feature-layout.md`, `reconcile.md`) cross-link to each other and `SPEC.md`. When adding another lifecycle doc, follow the same Related section pattern.
- The parity guard (`assets/assets_test.go` `requiredAnchors`) is now the enforcement surface for "what must all skill files say verbatim". When adding a skill block, add an anchor here or it will silently drift.
- `TestMain` in `internal/cli/phase2_test.go` redirects `XDG_CONFIG_HOME`. Any new CLI test that writes provider / global config MUST run in the `internal/cli` package (not elsewhere) to inherit that isolation.
- Reconcile preflight is now a hard gate. When writing tests that exercise reconcile phases, stage a fully clean tree first OR pass `--allow-dirty`.
- The `WarnWriter` pattern (see implement.go) is the convention for non-fatal workflow warnings. Swappable in tests via `prev := WarnWriter; WarnWriter = &buf; defer func() { WarnWriter = prev }()`.

## Archived 2026-04-18 — M11 handoff (superseded by v0.4.2 Tranche A)

# Current Handoff

## Active Task
- **Task ID**: M11 — Native Copilot provider (ADR-005)
- **Milestone**: M11 delivered
- **Description**: First-party Go provider speaking directly to `api.githubcopilot.com`. Mirrors the copilot-api/litellm pattern: device-code OAuth → session-token exchange → editor headers.
- **Status**: Implemented; awaiting supervisor review.
- **Assigned**: 2026-04-18

## Session Summary

1. **Auth store** (`internal/provider/copilot_auth.go`) — schema
   `{version, oauth, session}`, atomic write at `$XDG_DATA_HOME/tpatch/copilot-auth.json`
   with 0600 perms, rejects symlinks + world/group-writable parent dirs, tightens
   file perms on load, `TPATCH_COPILOT_AUTH_FILE` env override for tests,
   `authStoreMu` serialises writes + refreshes.
2. **Device-code flow** (`internal/provider/copilot_login.go`) — `RequestDeviceCode`,
   `PollAccessToken` (honours `authorization_pending`, permanent `slow_down` bump,
   `expired_token`, `access_denied`, local deadline + ctx cancel, always sends
   `Accept: application/json`), `ExchangeSessionToken` (+ `…Locked` variant used
   by the provider's retry-on-401 path). Client ID `Iv1.b507a08c87ecfe98`
   matches copilot-api.
3. **Editor headers** (`internal/provider/copilot_headers.go`) — version
   constants tracking copilot-api 0.26.7, `x-request-id` uuid, `TODO(adr-005)`
   to refresh when upstream bumps.
4. **Provider impl** (`internal/provider/copilot_native.go`) — `CopilotNative`
   satisfies `Provider`. `Check` never initiates device flow (returns
   `errCopilotUnauthorized` if no auth file). `Generate` proactively refreshes
   the session 60s before expiry, retries once on 401 with a forced refresh,
   then fails. Routes via `auth.Session.Endpoints["api"]` verbatim (D5).
5. **Registry** — `provider.NewFromConfig` dispatches
   `CopilotNativeType = "copilot-native"`. `Config.Configured()` relaxed for
   copilot-native so `Model` alone is enough (`BaseURL` comes from the auth
   file). New `Config.Initiator` field plumbed through `store.ProviderConfig`,
   the YAML parser, `SaveConfig`, and `renderGlobalYAML`.
6. **Opt-in gate** — `store.AcknowledgeCopilotNativeOptIn`,
   `store.CopilotNativeOptedIn`, plus `CopilotNativeOptIn` + `…At` fields
   written to **global config only** (same class as `CopilotAUPAckAt`) so they
   don't leak via repo clones. Enforced in `providerSetCmd`, `config set`
   (`provider.type=copilot-native`), and implicitly in auto-detect (which never
   lists copilot-native as a candidate).
7. **CLI** (`internal/cli/copilot_native.go`) — `provider copilot-login`
   (enterprise prompt, device flow, AUP notice), `provider copilot-logout`
   (deletes auth file). Re-uses AUP language from M10.
8. **Config set** — `config set provider.copilot_native_optin true` routes
   to `SaveGlobalConfig` (rubber-duck #3); `config set provider.initiator`
   validates `""|user|agent`.
9. **Preset** — `--preset copilot-native` in `providerPresets` (empty
   BaseURL, default model `claude-sonnet-4`, empty AuthEnv).
10. **Version bump** — `0.4.0-dev`.
11. **Docs** — new `docs/faq.md` (macOS `~/Library/Application Support`
    caveat + `XDG_CONFIG_HOME` override + auth-file locations); harness
    doc `docs/harnesses/copilot.md` gains "Native path (experimental,
    opt-in)" section; ROADMAP M11 marked ✅.

## Files Created
- `internal/provider/copilot_auth.go`
- `internal/provider/copilot_login.go`
- `internal/provider/copilot_headers.go`
- `internal/provider/copilot_native.go`
- `internal/cli/copilot_native.go`
- `docs/faq.md`

## Files Modified
- `internal/provider/provider.go` — `Config.Initiator`, relaxed `Configured()`
- `internal/provider/anthropic.go` — `NewFromConfig` dispatches copilot-native
- `internal/store/types.go` — `CopilotNativeOptIn` + `…At`, `ProviderConfig.Initiator`, relaxed `ProviderConfig.Configured()`
- `internal/store/store.go` — YAML parse/emit for new fields
- `internal/store/global.go` — global opt-in render + merge + helpers
- `internal/cli/cobra.go` — preset, type flag, opt-in gate, config-set routing, version bump
- `internal/cli/copilot.go` — pipes `Initiator` into `provider.Config`
- `docs/harnesses/copilot.md` — native path section
- `docs/ROADMAP.md` — M11 marked ✅

## Test Results

```
$ go test ./... -count=1
ok  github.com/tesseracode/tesserapatch/assets
ok  github.com/tesseracode/tesserapatch/internal/cli
ok  github.com/tesseracode/tesserapatch/internal/provider
ok  github.com/tesseracode/tesserapatch/internal/safety
ok  github.com/tesseracode/tesserapatch/internal/store
ok  github.com/tesseracode/tesserapatch/internal/workflow
$ go build ./cmd/tpatch
# binary reports 0.4.0-dev
```

## Next Steps
1. Supervisor review per `AGENTS.md` cadence → approve → tag `v0.4.0`
   so the CI release job publishes notes.
2. Live smoke test against a real GitHub account with Copilot entitlement:
   - `tpatch config set provider.copilot_native_optin true`
   - `tpatch provider copilot-login`
   - `tpatch provider set --preset copilot-native`
   - `tpatch provider check`
   - full `tpatch cycle` of a toy feature.
3. Follow-up: add provider-level unit tests with an httptest fake for
   the device flow + session exchange + 401 retry (scaffolded but not
   included in this cut to keep the diff surgical).

## Blockers
None. Editor-header policy is a known unknown per ADR-005 OQ1; we ship
with editor headers until GitHub publishes an official compatibility
endpoint.

## Context for Next Agent
- `CopilotAuthFilePath()` returns `(string, error)` — don't call it as a
  single-value expression.
- `ExchangeSessionToken(ctx, opts, auth)` **mutates `auth` in place** and
  returns only `error`. That's intentional: the provider's retry-on-401
  path needs to refresh the in-memory struct without re-reading the file
  before writing.
- `CopilotSessionBlock.Endpoints["api"]` is the routing root. Treat it as
  opaque — don't parse or reconstruct it.
- `authStoreMu` guards **both** the file and `exchangeSessionTokenLocked`;
  always call `ExchangeSessionToken` (the public wrapper) unless you
  already hold the mutex.
- macOS + `os.UserConfigDir()` resolves to `~/Library/Application Support/tpatch/`.
  Documented in `docs/faq.md`; users who want XDG layout set
  `XDG_CONFIG_HOME`.

---

# Handoff History

*Completed handoff entries are archived here in reverse chronological order.*

---

## 2026-04-17 — Distribution Setup (module rename + CI workflow) (v0.3.0)

**Task**: Enable 'go install' + add free CI workflow
**Agent**: Distribution agent
**Verdict**: APPROVED — committed as dc42718 + 305781d, tagged v0.3.0

## Session Summary

Two operational follow-ups:

1. **Module path fixed to match repo** — `go.mod` said `github.com/tesseracode/tpatch` while the GitHub repo is `tesseracode/tesserapatch`. That mismatch blocks `go install`. Renamed the module and all imports to `github.com/tesseracode/tesserapatch` (user-selected option). The binary is still called `tpatch` because Go names installed binaries after the final path segment (`cmd/tpatch`).
2. **CI workflow added** — `.github/workflows/ci.yml` runs on push and PR to `main`. It sets up Go via `go-version-file: go.mod` (so CI tracks local dev), checks formatting with `gofmt`, runs `go vet`, builds, tests, and runs an install smoke test. Matrix on `ubuntu-latest` + `macos-latest`. Concurrency group cancels superseded runs to save minutes. Free for public repos.
3. **README install block updated** — now points to the correct module path.

## Files Changed
- `go.mod` — `module github.com/tesseracode/tesserapatch`.
- All `.go` files under `cmd/`, `internal/`, `assets/` — import paths rewritten.
- `.github/workflows/ci.yml` — new CI workflow.
- `README.md` — install instructions updated.

## Test Results
- `gofmt -l .` — clean
- `go test ./... -count=1` — **ALL PASS** across 7 packages
- `go build -o tpatch ./cmd/tpatch` — OK
- `./tpatch --version` → `tpatch 0.3.0-dev`

## Post-Merge Checklist (for the repo owner)
1. Make the repo public (required for `go install` without auth and for free unlimited Actions minutes).
2. Push to `main`; CI should pass on both ubuntu + macOS.
3. Tag a release: `git tag v0.3.0 && git push origin v0.3.0`. `go install ...@latest` will then resolve to that tag.
4. Verify from a clean machine: `go install github.com/tesseracode/tesserapatch/cmd/tpatch@latest`.

## Provider Preset Clarification
`tpatch provider set --preset copilot` targets `http://localhost:4141` with `auth_env: GITHUB_TOKEN`. That is the **copilot-api proxy** endpoint, not the Copilot CLI auth itself. To use the same Copilot subscription as `copilot-cli`:

- Install and run `copilot-api` locally (it does the GitHub OAuth and exposes an OpenAI-compatible endpoint on 4141).
- Then `tpatch provider set --preset copilot` just works.

There is no direct-to-GitHub-Copilot path today because GitHub has not published a public OpenAI-compatible Copilot endpoint. If that changes, we add another preset.

## Blockers
None.

## Next Steps
1. Push + make repo public + tag v0.3.0.
2. Confirm CI green on first main push.
3. Optional: add a `release.yml` workflow with goreleaser for prebuilt binaries (not required for `go install`).


---

## 2026-04-17 — Phase 2 Refinement: SDK Evaluation + Harness Guides + Tracking Cadence (v0.3.0-dev)

**Task**: Evaluate mainstream Go SDKs and agent CLIs; adopt simplest integration; tighten tracking cadence
**Agent**: Phase 2 refinement agent
**Verdict**: SUPERSEDED by 2026-04-17 distribution setup entry (see LOG.md)

## Session Summary

Iterated on the Phase 2 M7–M9 output after the user asked us to survey reference implementations and not waste resources on unneeded SDKs.

1. **SDK evaluation (ADR-003)** — Surveyed `OpenRouterTeam/go-sdk` (Speakeasy-generated, README marks non-production), `openai/openai-go`, `anthropics/anthropic-sdk-go`. Decided to keep stdlib providers because: (a) our surface is `Check` + `Generate` only, (b) OpenRouter is drop-in OpenAI-compatible, (c) SDKs would add ~20 transitive deps for zero new capability. Positioned `openai/codex` and `github/copilot-cli` as *harnesses* (callers of tpatch), not providers.
2. **Presets for API parity** — Added `tpatch provider set --preset copilot|openai|openrouter|anthropic|ollama` backed by a single `providerPresets` map. Refactored `autoDetectProvider` to reuse the same map so there is one source of truth. Preset composes with explicit flag overrides (e.g. `--preset anthropic --model claude-opus-4`). Invalid presets fail loudly.
3. **Harness integration guides** — Wrote `docs/harnesses/codex.md` and `docs/harnesses/copilot.md` explaining the `tpatch next --format harness-json` contract, example sessions, recommended allow-lists, and anti-patterns (do not let the harness re-implement workflow phases).
4. **Tracking cadence** — Rewrote "Context Preservation Rules" in `AGENTS.md` with an enforced cadence cheatsheet (trigger → update). Updated `CLAUDE.md` Working Rules to reference the cadence. Key directive: "A task is not complete until tracking reflects its state."

## Files Created
- `docs/adrs/ADR-003-sdk-evaluation.md` — SDK evaluation decision, matrix, rationale.
- `docs/harnesses/codex.md` — Codex CLI integration guide.
- `docs/harnesses/copilot.md` — GitHub Copilot CLI integration guide.

## Files Changed
- `internal/cli/cobra.go` — `providerPresets` map; `--preset` flag on `provider set`; auto-detect refactored to reuse presets.
- `internal/cli/phase2_test.go` — New `TestProviderSetPreset` covering openrouter/anthropic/unknown.
- `AGENTS.md` — Stronger "Context Preservation Rules" with cadence cheatsheet.
- `CLAUDE.md` — Working Rules point to cadence; explicit per-phase tracking requirement.

## Test Results
- `go test ./...` — **ALL PASS** (7 packages)
- `gofmt -l .` — **CLEAN**
- `go build -o tpatch ./cmd/tpatch` — **OK** (v0.3.0-dev)
- Manual verification:
  ```
  tpatch provider set --preset openrouter
  → type: openai-compatible, url: https://openrouter.ai/api, auth_env: OPENROUTER_API_KEY
  ```

## Key Decisions Locked In
- **No third-party provider SDKs.** Stdlib stays the provider layer.
- **`providerPresets` is the single source of truth.** Adding a new vendor = one map entry.
- **Harnesses (codex, copilot) call tpatch via CLI + JSON.** No SDK embed on either side.
- **Tracking updates are enforced per phase, not per session.**

## Blockers
None.

## Next Steps
1. Live smoke test with `codex exec` and `copilot` once an environment with both installed is available — confirm the handshake matches the guide.
2. Consider M10 (`tpatch mcp serve`) to expose the same state machine via MCP for Copilot CLI. Tracked as a follow-up only; not in the current ADR scope.
3. Supervisor review + roadmap update for this refinement pass.

## Context for Next Agent
- The preset map lives in `internal/cli/cobra.go` just below `providerSetCmd()`. Keep `--preset` and `autoDetectProvider` using the same map.
- Harness guides assume a repo-level `AGENTS.md` for codex and a `.github/copilot/cli/skills/tessera-patch/SKILL.md` for copilot-cli. Both are created by copying from the `.tpatch/steering/` outputs of `tpatch init`.
- ADR-003 explicitly lists the triggers that would cause us to reconsider adopting SDKs (streaming, non-standard schemas, official harness client libraries).
- Prior Phase 2 handoff (M7/M8/M9 initial) has been archived to `docs/handoff/HISTORY.md` under a 2026-04-17 entry.


---

## 2026-04-17 — M7 + M8 + M9 Phase 2 Implementation (v0.3.0-dev)

**Task**: Ship Phase 2 milestones (provider integration, LLM validation+retry, interactive/harness commands)
**Agent**: Phase 2 implementation agent
**Verdict**: APPROVED WITH NOTES (subsumed by 2026-04-17 refinement — see CURRENT.md)

## Session Summary

Implemented M7–M9 end-to-end:

1. **M7** — Added `AnthropicProvider` (`internal/provider/anthropic.go`) speaking the Messages API. Introduced `provider.NewFromConfig()` factory selecting by `cfg.Type`. Extended auto-detection to probe Ollama (localhost:11434), `ANTHROPIC_API_KEY`, and `OPENROUTER_API_KEY`. Added `provider set --type` flag and `provider.type` validation. Wrote `docs/adrs/ADR-002-provider-strategy.md` documenting the decision and live-probe evidence for copilot-api; Ollama/OpenRouter confirmed compatible via existing OpenAI-compat provider (no code changes required).
2. **M8** — Added `GenerateWithRetry` in `internal/workflow/retry.go` with pluggable validators. `JSONObjectValidator` strips fences and round-trips the payload; `NonEmptyValidator` guards define/explore. Each attempt logs to `artifacts/raw-<phase>-response-N.txt`. Retries reissue the prompt with a corrective suffix describing the validator error. `max_retries` added to `config.yaml` (default 2), `--no-retry` flag added to analyze/define/explore/implement, context-keyed via `workflow.WithDisableRetry` to avoid signature churn.
3. **M9** — Shipped three new commands: `cycle` (batch and `--interactive` with `--editor` and `--skip-execute` options), `test` (runs `config.test_command`, records outcome in `apply-session.json` + `artifacts/test-output.txt`), `next` (emits next action as plain text or `--format harness-json`). Registered in root, version bumped to `0.3.0-dev`. All 6 skill formats updated to include `cycle`/`test`/`next`. Parity guard extended.

## Files Created
- `internal/provider/anthropic.go` — Anthropic Messages provider + `NewFromConfig` factory
- `internal/provider/anthropic_test.go` — Anthropic + factory tests
- `internal/workflow/retry.go` — `GenerateWithRetry`, validators, context flag
- `internal/workflow/retry_test.go` — retry-path tests
- `internal/cli/phase2.go` — `cycle`, `test`, `next` commands
- `internal/cli/phase2_test.go` — integration tests for the new commands
- `docs/adrs/ADR-002-provider-strategy.md` — provider strategy decision

## Files Changed
- `internal/cli/cobra.go` — factory wiring, `--type` flag, `--no-retry` on 4 workflow commands, auto-detect extensions, config `max_retries`/`test_command` keys, version bump
- `internal/store/types.go` — `Config` gains `MaxRetries` and `TestCommand`
- `internal/store/store.go` — default config.yaml template + `SaveConfig` + `parseYAMLConfig` cover the new fields
- `internal/workflow/workflow.go` — analyze/define/explore call `GenerateWithRetry`
- `internal/workflow/implement.go` — implement calls `GenerateWithRetry`
- `assets/skills/*` + `assets/workflows/*` + `assets/prompts/*` — all 6 formats list the three new commands
- `assets/assets_test.go` — parity guard requires `cycle`, `test`, `next`
- `docs/ROADMAP.md` — M7/M8/M9 marked complete

## Test Results
- `go test ./...` — **ALL PASS** across 7 packages
- `gofmt -l .` — **CLEAN**
- `go build -o tpatch ./cmd/tpatch` — **OK** (v0.3.0-dev)
- Smoke test: `init` → `add` → `next --format harness-json` → `cycle --skip-execute` → `config set test_command echo hi` → `test` — all succeed end-to-end

## Noteworthy Details
- `Provider` interface unchanged (still `Check` + `Generate`). Adding providers is purely additive.
- Retry is disabled when no provider is configured (existing heuristic fallback untouched).
- `tpatch next` is state-aware: for `defined` features it further distinguishes "needs explore", "needs implement", or "needs apply" by probing the feature directory.
- `--no-retry` plumbing uses `context.WithValue` to avoid changing every workflow signature.
- Auto-detection order: copilot-api → Ollama → Anthropic (via env) → OpenAI (via env) → OpenRouter (via env).

## Blockers
None.

## Next Steps
1. Run live bug bash against copilot-api with retry enabled (ideally against a degraded-model path to exercise the corrective prompt).
2. Consider streaming/tool-use support as an optional capability interface when a future milestone needs it.
3. Consider harness integration guides (M9.10, M9.11) — deferred; the skill files and `tpatch next --format harness-json` already provide the contract.


---

## 2026-04-16 — M6 Live Provider Bug Bash (v0.2.0-dev, Session 4)

**Task**: Run bug bash with live copilot-api provider, add patch validation and merge strategy config  
**Agent**: Supervisor agent  
**Status**: Complete — Full pass with live LLM

**What was done**:
- Added `ValidatePatch()` to gitutil — automated patch validation on `record`
- Added `merge_strategy` config option (`3way` default, `rebase` alt) to types, store, and CLI
- Added `extractUpstreamContext()` to reconcile — reads affected files for Phase 3 prompt
- Ran complete bug bash with live copilot-api (claude-sonnet-4, 44 models)
- Live LLM analysis produced detailed, accurate results with correct file paths
- Feature A: `upstream_merged` via Phase 3 (LLM analyzed upstream model-mapping.ts)
- Feature B: `reapplied` via Phase 4 (LLM said still_needed, patch applied cleanly)

**Key finding**: Upstream context is critical for Phase 3. Without actual file contents, the LLM returns "unclear".

---

## 2026-04-16 — M6 Bug Bash + Bug Fixes (v0.2.0-dev)

**Task**: Run reconciliation bug bash, fix discovered bugs, re-test  
**Agent**: Supervisor agent (3 sessions)  
**Status**: Complete — Full pass

**What was done**:
- Session 2: Ran initial bug bash against `tesseracode/copilot-api` at commit `0ea08feb`
  - Feature A (model translation fix): Correctly detected as `upstream_merged` via Phase 3
  - Feature B (models CLI subcommand): Blocked — 3 bugs found in patch capture and CLI
  - Found BUG-1 (flag ordering), BUG-2 (corrupt patches), BUG-3 (stale recording)
- Session 3: Fixed all 3 bugs + bonus improvement
  - Migrated CLI from stdlib `flag` to `cobra` (fixes interspersed flags)
  - Rewrote `CapturePatch()` with `git add --intent-to-add` (fixes new file handling)
  - Added trailing newline to all patch output (fixes corrupt patch at EOF)
  - Added `--from` flag to `record` (captures committed diffs)
  - Added 3-way merge fallback to forward-apply (handles lockfile mismatches)
- Re-ran bug bash: Feature A → `upstream_merged`, Feature B → `reapplied`. Full pass.

**Key decisions**:
- Added cobra dependency (breaks zero-dep constraint, user-approved)
- Patches now always end with `\n`
- Forward-apply tries strict then 3-way merge fallback

---

## 2026-04-16 — M0–M5 Implementation (v0.1.0-dev)

**Task**: Build unified tpatch CLI from M0 through M5  
**Agent**: Supervisor agent (1 session)  
**Status**: Complete — All milestones approved

**What was done**:
- Built entire CLI in Go: 12 commands, ~2600 LOC source, ~850 LOC tests
- M0: Go module, CLI skeleton, Makefile
- M1: .tpatch/ data model, store layer, init/add/status/config, slug generation, path safety
- M2: OpenAI-compatible provider, analyze/define/explore with heuristic fallback
- M3: implement, apply (prepare/started/done), record, patch capture
- M4: 4-phase reconciliation engine with 4 test scenarios
- M5: 6 skill formats embedded via go:embed, parity guard test

---

## 2026-04-16 — Project Bootstrap (Governance)

**Task**: Bootstrap tpatch/ consolidation project with governance files  
**Agent**: Board review agent  
**Status**: Complete

**What was done**:
- Created SPEC.md consolidating technical decisions from all three teams
- Created CLAUDE.md for agent orientation with read-this-first table
- Created AGENTS.md defining the cyclic supervisor workflow (implementation → review → decision)
- Created ROADMAP.md with M0-M6 milestones + future M7-M11
- Created 7 milestone files with detailed task lists, acceptance criteria, and reference pointers
- Created handoff and supervisor log templates
- Created consolidation prompt for the supervisor agent

**Key decisions**:
- Go with zero dependencies (stdlib only)
- 4-phase reconciliation (reverse-apply → operation-level → provider-assisted → forward-apply)
- 6 skill formats (Claude, Copilot, Copilot Prompt, Cursor, Windsurf, Generic)
- Deterministic apply recipe with path traversal protection
- Secret-by-reference pattern for provider credentials
# Current Handoff

## Active Task
- **Task ID**: ADR-004 (M10 proxy UX) + ADR-005 (M11 native provider)
- **Milestone**: Planning locked-in for M10 and M11
- **Description**: User chose interactively through open questions; decisions captured as two ADRs. PRD updated to match the session-token-exchange direction (copilot-api/litellm pattern) instead of opencode's simpler path.
- **Status**: ADRs written, awaiting supervisor review
- **Assigned**: 2026-04-17

## Session Summary

1. **Committed Phase 2 work** as commit `dc42718` ("Phase 2 (v0.3.0): providers, validation, interactive/harness, distribution"). Includes all M7/M8/M9, refinement, and distribution changes.
2. **Released v0.3.0** — bumped version constant from `0.3.0-dev` to `0.3.0`, committed as `305781d`, tagged `v0.3.0` with a full release note. Tag is local; repo owner still needs to `git push origin main --tags`.
3. **Researched Copilot auth options**:
   - Pulled `tesseracode/copilot-api` README — explicitly "reverse-engineered proxy… not supported by GitHub… may trigger abuse-detection systems."
   - Pulled `github/copilot-cli` README and repo root listing — **not open source** (only README, install.sh, changelog, LICENSE published; the CLI is a closed-source binary on Homebrew/npm/WinGet). Official auth paths: `/login` OAuth or `GH_TOKEN`/`GITHUB_TOKEN` with "Copilot Requests" PAT permission.
   - Conclusion: **GitHub does not publish a public OpenAI-compatible Copilot endpoint.** Every third-party integration (copilot-api, Claude Code via proxy, tpatch) is on reverse-engineered surface.
4. **Wrote PRD** (`docs/prds/PRD-native-copilot-auth.md`) with 5 options evaluated and a two-phase recommendation: M10 managed-proxy UX (`copilot-start` / `copilot-stop` / `copilot-status`), then M11 opt-in native PAT provider calling `api.githubcopilot.com` directly. Shelling out to `copilot` CLI explicitly rejected (burns premium requests, re-runs its own agent loop).

## Files Created
- `docs/prds/PRD-native-copilot-auth.md`

## Files Changed
- `internal/cli/cobra.go` — version `0.3.0-dev` → `0.3.0` (committed)

## Git State
- `dc42718` — Phase 2 feature commit
- `305781d` — "Release v0.3.0" (version bump)
- `v0.3.0` — tag on 305781d
- **Not yet pushed.** Repo owner needs `git push origin main && git push origin v0.3.0`.

## Test Results
- `gofmt -l .` clean
- `go test ./...` — all 7 packages pass
- `tpatch --version` → `tpatch 0.3.0`

## Key Decisions (captured in ADR-004 and ADR-005)

**M10 — copilot-api UX (ADR-004)**
- No process supervision; we warn when unreachable, point at install instructions.
- Upstream `ericc-ch/copilot-api` is the recommended proxy; internal TODO to revisit the tesseracode fork if its fixes become blocking.
- New global config at `~/.config/tpatch/config.yaml`; per-repo `.tpatch/config.yaml` overrides.
- Reachability probe on first call (`GET /v1/models`, 2s timeout); warn-but-continue on `init`, hard-fail on workflow commands.
- First-run AUP warning stored in global config; no log piping; Windows deferred.

**M11 — native Copilot provider (ADR-005)**
- **Changed direction**: port ericc-ch/copilot-api's internal flow (session-token exchange via `copilot_internal/v2/token` + VS Code Copilot Chat client ID `Iv1.b507a08c87ecfe98`) rather than opencode's simpler Bearer-the-OAuth-token path. copilot-api and litellm both use this flow → proven, field-exposed surface that matches what Copilot's own editor plugins do.
- Token storage: `$XDG_DATA_HOME/tpatch/copilot-auth.json`, chmod 0600. OS keychain deferred.
- OAuth token treated as long-lived; 401 triggers one retry then "run copilot-login again".
- Device-flow prompts for GitHub.com vs Enterprise; Enterprise domain captured at login.
- `GET /models` every session, no persistent cache.
- Editor headers overridable via `provider.headers_override`; `x-initiator` opt-in, unset by default.
- `type: copilot-native` distinct from `type: openai-compatible` + copilot proxy.
- Opt-in gate with AUP acknowledgement in global config.

## Blockers
- None for the PRD itself.
- M11 (native provider) is soft-blocked on the "can we ship the editor header set?" legal question noted in the PRD.

## Next Steps
1. **Repo owner**: decide whether to create a GitHub Release for v0.3.0 (or add `softprops/action-gh-release@v2` to CI for automation on future tags).
2. **Before M11 implementation begins**: answer the two open questions in the PRD and ADR-005 (legal/ToS on editor headers; GitHub roadmap for an official endpoint).
3. **Next agent session — M10 implementation** per ADR-004: add global-config loader, reachability probe in provider-set/init flow, first-run AUP warning helper.
4. **After M10 lands — M11 implementation** per ADR-005, gated on the open questions.

## Context for Next Agent
- PRD lives at `docs/prds/PRD-native-copilot-auth.md`. It includes the full options matrix and the rejection rationale for each alternative.
- The `Provider` interface is stable and Phase 1 does not need to touch it at all — the managed proxy still routes through the existing `OpenAICompatible` code path. Phase 2 adds a sibling struct.
- `docs/harnesses/copilot.md` already documents the current manual setup; update it when M10 lands.
- GitHub has explicitly warned users in copilot-api's README about abuse-detection. Our UX for M10/M11 must surface that warning prominently.



---


---

# Archived — 2026-04-17T08:26:19Z

# Current Handoff

## Active Task
- **Task ID**: M10 — Managed Copilot proxy UX (ADR-004)
- **Milestone**: M10 delivered
- **Description**: Honest UX for the reverse-engineered `copilot-api` proxy — global config, reachability probe, first-run AUP warning, install pointers, CI release automation.
- **Status**: Implemented; awaiting supervisor review.
- **Assigned**: 2026-04-17

## Session Summary

1. **CI release automation** — added a `release` job to `.github/workflows/ci.yml` that triggers on `v*` tag pushes, creates a GitHub Release via `softprops/action-gh-release@v2`, auto-generates release notes, and marks tags containing `-` as prereleases. Uses the default `GITHUB_TOKEN` with `contents: write`. Cost: free.
2. **Global config** — new `internal/store/global.go` adds `GlobalConfigPath()`, `LoadGlobalConfig`, `SaveGlobalConfig`, `(s *Store).LoadMergedConfig`, `AcknowledgeCopilotAUP`, `CopilotAUPAcknowledged`, `mergeConfig`, `renderGlobalYAML`. Honors `XDG_CONFIG_HOME`, falls back to `os.UserConfigDir()` (macOS caveat documented in the harness doc). Chmod 0600 on write.
3. **Config precedence** — repo `.tpatch/config.yaml` overrides the global config field-by-field; zero values do **not** clear globals (must set the field explicitly). AUP ack is global-only.
4. **Types** — `Config.CopilotAUPAckAt string` added to `internal/store/types.go`.
5. **Reachability probe** — new `internal/provider/probe.go` with `Reachable(ctx, cfg)` (2s timeout), `IsLocalEndpoint(cfg)`, `IsCopilotProxyEndpoint(cfg)` helpers. Probes via existing `Check()`.
6. **CLI wiring** — new `internal/cli/copilot.go` with `copilotInstallHint`, `copilotAUPWarning`, `maybeShowAUPWarning`, `ensureProviderReachable`, `warnIfUnreachable`, `providerConfigFromStore`. Wired into `init` (warn-continue + AUP) and `providerSetCmd` + `autoDetectProvider` (AUP on first Copilot selection).
7. **Workflow hard-fail** — `loadAndProbeProvider(ctx, s)` replaces `loadProviderFromStore` in analyze/define/explore/implement/cycle. Probes once per process (cached per base URL). Local-endpoint-only; opt-out via `TPATCH_NO_PROBE=1`. Non-local endpoints skip the probe to avoid penalising custom remote configs.
8. **Execute now surfaces errors** — `Execute()` prints `error: %v` to stderr before returning exit code 1 so probe failures are visible. Preserves existing `SilenceErrors: true` cobra behaviour for graceful formatting.
9. **Harness doc refresh** — `docs/harnesses/copilot.md` now documents the install path, OS-dependent global config path (macOS caveat), warn-vs-fail behaviour, and links to ADR-004/005.
10. **Tests** — 6 new tests in `internal/store/global_test.go` (roundtrip, missing file, ack idempotency, precedence, merge-no-clear, save creates dir) and 5 in `internal/provider/probe_test.go` (httptest OK, TEST-NET-1 timeout, not-configured, URL matcher, cancelled ctx). All 7 packages pass.

## Files Created
- `.github/workflows/ci.yml` — amended (release job)
- `internal/store/global.go`
- `internal/store/global_test.go`
- `internal/provider/probe.go`
- `internal/provider/probe_test.go`
- `internal/cli/copilot.go`

## Files Changed
- `internal/cli/cobra.go` — `loadAndProbeProvider`, `Execute` prints errors, AUP wiring in `init` / `providerSetCmd` / `autoDetectProvider`, `sync` import.
- `internal/store/types.go` — `CopilotAUPAckAt` field.
- `docs/harnesses/copilot.md` — M10 section.

## Test Results
- `gofmt -w .` clean
- `go vet ./...` clean
- `go test ./... -count=1` — 7/7 packages pass
- `go build ./cmd/tpatch` OK
- Smoke: `init` + `provider set --preset copilot` prints AUP warning exactly once; second run is quiet; `analyze` against a dead localhost port hard-fails with an install hint; against a live copilot-api proxy falls through to the workflow.

## Key Behaviours

- **Warn vs fail**: `init` and `provider set` are warn-continue (a user may be bootstrapping before starting the proxy). Workflow commands that actually call the LLM (`analyze|define|explore|implement|cycle`) hard-fail when the local endpoint is unreachable.
- **Probe scope**: only runs for local endpoints (`localhost`, `127.0.0.1`, `[::1]`). Remote endpoints are trusted.
- **AUP once**: the AUP warning fires only when the new config actually points at the copilot-api proxy (`openai-compatible` + port 4141) and the user has not acknowledged before.
- **TODO**: `copilotInstallHint` carries an inline `TODO(adr-004)` comment to revisit the tesseracode fork recommendation if its divergent fixes become blocking.

## Blockers
- None for M10.
- M11 still soft-blocked on the two open questions in ADR-005 (editor-headers legal/ToS, official endpoint roadmap). User direction: proceed with editor headers, monitor; so these are effectively closed as "accept risk".

## Next Steps
1. Supervisor review of M10 implementation.
2. Commit as `feat(m10): managed copilot-api proxy UX (ADR-004)` and push.
3. Consider tagging `v0.3.1` once review lands — CI will produce the GitHub Release automatically.
4. Start M11 implementation per ADR-005 (native Copilot provider with session-token exchange) once M10 is merged.

## Context for Next Agent
- Global config on macOS defaults to `~/Library/Application Support/tpatch/config.yaml` unless `XDG_CONFIG_HOME` is set. Every test that touches global state sets `XDG_CONFIG_HOME` to a tempdir; follow this pattern.
- `TPATCH_NO_PROBE=1` disables the workflow hard-fail probe (useful for offline demos or CI steps that only read store state). Add it to future tests that should not hit the network.
- The probe cache is a process-level `map[string]error` guarded by a mutex — fine for the CLI's one-shot lifecycle but intentionally not time-bound, so long-running processes would need to invalidate it. Not a concern today.
- `Execute()` now prints errors. Tests that exercise `rootCmd.Execute()` directly still use the cobra `SetErr` buffer; only the top-level wrapper prints to stderr.
- The AUP warning text lives in `internal/cli/copilot.go::copilotAUPWarning`. Tweak there, not in harness docs.
# Current Handoff

## Active Task
- **Task ID**: v0.4.2 / A1 — `bug-implement-silent-fallback`
- **Milestone**: Tranche A "Truthful Errors" (post-stress-test, plan.md)
- **Description**: Surface the implement-phase fallback to the user, raise
  the LLM token budget so legitimate recipes are not truncated, and let
  the user override the budget via config.
- **Status**: A1 complete; A2 (`bug-cycle-state-mismatch`) is now active.
- **Assigned**: 2026-04-18

## Session Summary

A1 landed in this session:

1. **Config knob** — `Config.MaxTokensImplement` (`internal/store/types.go`),
   default `DefaultMaxTokensImplement = 16384`. Repo override via
   `max_tokens_implement:` in `.tpatch/config.yaml`; global override via
   the same key in `~/.config/tpatch/config.yaml`. `parseYAMLConfig` reads
   it; `SaveConfig` and `renderGlobalYAML` emit it; `mergeConfigs` lets
   the repo value win when set.
2. **Implement fallback no longer silent** — `internal/workflow/implement.go`
   gained a package-level `WarnWriter io.Writer = os.Stderr`. When
   `GenerateWithRetry` exhausts its retry budget the fallback writes a
   warning to `WarnWriter` naming the retry count, the underlying error,
   the path to `raw-implement-response-*.txt`, and the config knob to
   bump on retry.
3. **MaxTokens bump** — implement phase now requests
   `cfg.MaxTokensImplement` (defaulting to 16384) instead of the
   hard-coded 8192. Other phases unchanged for now (analyze/define/explore
   stay at 4096; revisit if real failures surface).
4. **Tests** — `internal/workflow/implement_test.go`:
   - `TestRunImplement_FallbackEmitsWarning` drives `RunImplement` with
     a fake provider that returns un-parseable JSON, captures
     `WarnWriter`, asserts the warning text, and confirms the heuristic
     recipe is the one written to disk.
   - `TestConfig_DefaultMaxTokensImplement` confirms a freshly-`Init`-ed
     repo loads the 16384 default.

## Current State

- Repo at clean working tree on top of v0.4.1 (no commits yet for v0.4.2;
  Tranche A will be tagged together once A1–A10 land).
- `gofmt -l .` clean, `go build ./cmd/tpatch` ok, `go test ./...` green.
- Plan lives at
  `~/.copilot/session-state/f2c5d9eb-cef9-41dc-aab7-ad825ffca018/plan.md`.

## Files Changed (A1)

- `internal/store/types.go` — added `MaxTokensImplement` field +
  `DefaultMaxTokensImplement` const.
- `internal/store/store.go` — parser entry, repo template, `SaveConfig`
  renderer.
- `internal/store/global.go` — merge precedence + `renderGlobalYAML`.
- `internal/workflow/implement.go` — `WarnWriter`, dynamic `MaxTokens`,
  surfaced fallback warning.
- `internal/workflow/implement_test.go` — new test file.

## Test Results

```
ok  github.com/tesseracode/tesserapatch/assets
ok  github.com/tesseracode/tesserapatch/internal/cli
ok  github.com/tesseracode/tesserapatch/internal/provider
ok  github.com/tesseracode/tesserapatch/internal/safety
ok  github.com/tesseracode/tesserapatch/internal/store
ok  github.com/tesseracode/tesserapatch/internal/workflow
```

## Next Steps

Continue Tranche A in order. The full ordered list is in plan.md; the
next 4 tasks are:

1. **A2 `bug-cycle-state-mismatch`** — audit `cycle` state transitions,
   ensure `state` advances even on heuristic fallback, add per-phase
   post-condition assertions, add a `cycle --skip-execute` test that
   reaches `implemented`. Currently `in_progress` in SQL.
2. **A3 `bug-record-validation-false-positive`** — switch record-time
   validation to `git apply --reverse --check` (add
   `gitutil.ValidatePatchReverse`).
3. **A4 `bug-reconcile-phase4-false-positive`** — three-state verdict
   (`reapplied-strict` / `reapplied-with-3way` / `blocked`); detect
   conflict markers via temp worktree apply.
4. **A5 `bug-skill-invocation-clarity`** — Invocation + Phase-ordering +
   Preflight blocks across all 6 skill formats; parity guard updated.

Then A6–A10, version bump to 0.4.2, CHANGELOG, tag.

## Blockers

None.

## Context for Next Agent

- Use `WarnWriter` (not `fmt.Fprintln(os.Stderr, ...)` directly) for any
  new non-fatal phase warnings; tests rely on being able to swap it.
- The implement phase is the only phase that needs the larger token
  budget right now. If you change another phase's budget, mirror the
  pattern (config knob + `Default*` const + global+repo merge).
- The Tranche-A version bump happens **once** at the end of A10. Do NOT
  bump `cobra.go:version` or write a CHANGELOG entry as you go — group
  them in a single v0.4.2 commit.
- The session SQL is the source of truth for task progress
  (`SELECT id, status FROM todos WHERE status='pending' ORDER BY id`).
- Co-author trailer required on every commit:
  `Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>`.
