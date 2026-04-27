# Changelog

All notable changes to tpatch are recorded here.

## v0.6.1 — 2026-04-27 — M15 Stabilization (Wave 1 + Wave 2 + fix-pass)

User-facing stabilization release. Seven backlog items + a four-finding fix-pass against the merged surface. No schema, ADR, or default-behavior changes — `v0.6.0` repos round-trip byte-identical and behave the same except where explicitly noted.

### Added

- **`tpatch define <slug>` accepts `spec` as an alias** (`internal/cli/cobra.go`). Lifecycle commands enumerated in skill assets gain the alias too. Backwards compatible: `define` is unchanged.
- **`tpatch record <slug> --files <pathspec>...`** narrows the captured patch to the supplied pathspecs (`internal/cli/cobra.go`, `internal/gitutil/gitutil.go::CapturePatchScoped`). `--files` is mutually exclusive with `--from`; the empty-pathspec path is byte-identical to the historical full-tree capture. Companion `gitutil.CaptureDiffStatScoped` keeps `post-apply-diff.txt` and `record.md` scoped to the same pathspecs (no metadata leak).
- **`tpatch record <slug>` autogenerates `apply-recipe.json`** from the captured patch (`internal/workflow/recipe.go`). Default produces a fresh recipe alongside the patch; existing recipes are preserved unless the file/op-set has drifted, in which case a `recipe-stale.json` sidecar is emitted with a stderr warning. `--regenerate-recipe` opts into overwriting. `--no-recipe-autogen` disables generation entirely. The recipe-op JSON schema is unchanged: file deletions are not in the schema today, so they are skipped during autogen with a stderr note (a future ADR is required to add a `delete-file` op).
- **Validation: `satisfied_by` reachability + 40-hex contract.** `internal/store/validation.go` now refuses to persist a `satisfied_by` value that is not a 40-character hex SHA *and* not reachable from `HEAD`. Sentinel errors `ErrSatisfiedByMalformed` and `ErrSatisfiedBySHANotReachable` make the failure mode explicit. Closes the M14.1 deliberate limitation (any well-formed hex was accepted) and the M15-W2 review F1 contract drift (validation accepted reachable short SHAs that the apply-time gate would reject later).
- **Cross-platform user-shell selection.** New `internal/workflow.UserShell()` helper picks `sh -c` on Unix and `cmd /C` on Windows. Used by the `test_command` runner, the syntax-check gate, and `tpatch test`. Companion `shellQuoteFor(goos, p)` quotes `{file}` substitutions with the right convention for the chosen shell (single-quote on Unix, double-quote with `""` doubling on Windows). Unix behavior is byte-identical.
- **Skill assets ship valid YAML frontmatter.** `assets/skills/copilot/tessera-patch/SKILL.md` and `assets/skills/claude/tessera-patch/SKILL.md` now begin with a `---`-delimited frontmatter block (`name`, `description`). The Copilot CLI / Claude Code skill loaders that previously rejected the files with "missing or malformed YAML frontmatter" now accept them. Skill body is unchanged; parity guard (`assets/assets_test.go`) holds.

### Changed

- **`tpatch record` now detects post-record drift** between the captured patch and any pre-existing recipe (file-set comparison). Drift triggers a `recipe-stale.json` sidecar plus a stderr warning rather than overwriting silently. Same-files-different-content is intentionally below the floor (file-set drift only); deeper drift detection is deferred to a follow-up.
- **`gitutil.CapturePatchScoped` surfaces git errors when called with explicit pathspecs.** Previously any `git diff` failure was collapsed into an empty patch, then `recordCmd` reported the generic "captured 0 bytes" diagnostic. Empty pathspecs preserves the historical tolerant behavior the unscoped capture path has always relied on.

### Notes

- **Apply-time dependency gate is unchanged.** It still does the cheap 40-hex regex check; reachability lives in validation. The two layers now enforce the same value space (defense-in-depth) and validation refuses to persist anything reachability would later reject.
- **Hookable seam pattern.** Two new package-level vars (`store.isAncestor = gitutil.IsAncestor`, `workflow.userShellFor`) keep unit tests free of real git/shell calls. Convention for future external-command call sites.
- **Recipe-op JSON schema unchanged.** The schema does not include a `delete-file` op; autogen skips deletions and warns. Adding the op type requires an ADR and a parity-guard update. Flagged for a future wave.
- **Source-truth guard (ADR-011 D6) preserved.** No new readers of `artifacts/reconcile-session.json`. `status.Reconcile.Outcome` remains the authoritative machine-readable reconcile result; `artifacts/post-apply.patch` remains the reconcile authority for replay.
- **No new external Go dependencies.** stdlib + `cobra/pflag` only.
- **Backward compatibility.** A v0.6.0 repo with no `--files`, no `spec` alias, no autogen interaction, and a Unix shell behaves byte-identical. Repos that already declared a `satisfied_by` short SHA will fail validation on next status — fix by replacing with the full 40-hex SHA (this was a save-now/fail-later path before; now it fails at edit time, which is the intended contract).
- **Out of scope for v0.6.1** (deferred to Wave 3 / later): `tpatch verify <slug>`, `tested` lifecycle state, code-presence reconcile verdicts, fresh-branch reconcile mode, recipe-op schema extension for deletions.



First user-facing release of the feature-dependency DAG. Features can now declare hard / soft parents; apply, reconcile, status, and remove all observe the graph. Default-on (toggle via `features_dependencies: false`). PRD: `docs/prds/PRD-feature-dependencies.md` · ADR: `docs/adrs/ADR-011-feature-dependencies.md` · User reference: `docs/dependencies.md`.

### Added

- **M14.1 — Schema + validator (`internal/store`)** — `status.json` gains `depends_on: [{slug, kind, satisfied_by?}]` (omitempty round-trip). `Config.FeaturesDependencies` (`features_dependencies: true|false` in `.tpatch/config.yaml`). Pure DAG primitives: `DetectCycles`, `TopologicalOrder`, `Children`. Validator (`ValidateDependencies` / `ValidateAllFeatures`) with five sentinel errors: self-dep, dangling, kind-conflict, cycles, satisfied_by-requires-upstream-merged. Atomic edit semantics (rejected change leaves store untouched).
- **M14.2 — Apply gate + ordering (`internal/workflow`)** — `tpatch apply --mode execute` checks each hard parent before any file mutation; lists unsatisfied parents with their states. Soft parents never gate. Recipe-level `created_by` field on every operation: declares "this op was authored by parent feature X", validated against the recipe's `depends_on`. Cycle-aware traversal everywhere; `RunImplement` and `RunApply` topo-sort dependents.
- **M14.3 — Reconcile labels + compound presentation (`internal/store`, `internal/workflow`)** — Three composable labels overlayed on `Reconcile.Outcome`: `waiting-on-parent`, `blocked-by-parent`, `stale-parent-applied`. `EffectiveOutcome()` reports the compound string `blocked-by-parent-and-needs-resolution` when the child's intrinsic outcome is `blocked-requires-human` AND `blocked-by-parent` is set (display-only — programmatic decisions still read `Outcome` and `Labels` separately). Source-truth guard: all DAG/label code reads `status.Reconcile.Outcome` via `store.LoadFeatureStatus`, NEVER `artifacts/reconcile-session.json` (ADR-010 D5). Adversarial test pins this.
- **M14.4 chunk A — `tpatch status --dag`** — ASCII renderer (hard `─►`, soft `┄►`) with full-graph and scoped-to-slug modes; `--json` flag emits a structured shape for harnesses. Cycle-safe (corrupted graph degrades to flat list with `⚠ cycle detected` warning rather than recursing).
- **M14.4 chunk B — Default flip** — `features_dependencies` now defaults to **true** when the key is absent. `Init()` template writes the explicit `true`. v0.5.3 byte-identity preserved by setting `features_dependencies: false`.
- **M14.4 chunk C — Dependency-management CLI** — New verb tree `tpatch feature deps [<slug> [add|remove] <parent>[:hard|:soft]] | --validate-all`. `tpatch amend <slug> --depends-on <parent>[:kind] / --remove-depends-on <parent>` (deps-only mode skips request.md rewrite). `tpatch remove <slug> --cascade` performs reverse-topo deletion with TTY confirm; `--cascade --force` skips the prompt for non-TTY use. **`--force` alone never bypasses the dep-integrity gate** (PRD §3.7, ADR-011 D7).
- **M14.4 chunk D — Status-time validation** — `tpatch status` (with or without `--dag`) re-runs `ValidateAllFeatures` and surfaces every cycle / dangling / kind-conflict inline.
- **M14.4 chunk E — 6-skill rollout** — All six shipped skill formats (Claude, Copilot, Copilot Prompt, Cursor, Windsurf, Generic workflow) updated atomically with the dependency surface. `created_by` description updated from "inert" (v0.5.x) to live apply-time gate (v0.6.0+) with the dry-run downgrade noted. Parity guard (`assets_test.go`) holds.
- **M14.4 chunk F — User reference** — `docs/dependencies.md` covers edge model, declaration paths, validation rules, apply-time gate, `created_by` op-level gate, reconcile labels, compound verdict, `status --dag` examples, `--cascade` contract, migration, and the explicit out-of-scope list.

### Changed

- `tpatch apply --mode execute` now **fails fast** when an op's `created_by` parent is missing from `depends_on` or when a hard parent is unsatisfied. `tpatch apply --dry-run` downgrades the missing-hard-parent miss to a **warning** so operators can inspect the planned changes; recipe-shape errors (parent absent from `depends_on`, unknown kind) remain hard in both modes (PRD §4.3).
- `Reconcile.Labels` is now persisted on `status.json` (omitempty — empty slices round-trip to `nil`).
- `tpatch status` topology and presentation are unchanged on dependency-free repos; the new label / DAG output only appears when relevant.

### Notes

- **Version**: bumped to `0.6.0` in `internal/cli/cobra.go`.
- **No new external Go dependencies**; stdlib + `cobra/pflag` only.
- **No tag is created in this commit** — supervisor performs the `v0.6.0` tag at closeout.
- **Backward compatibility**: a v0.5.3 repo that does not declare any `depends_on` continues to round-trip byte-identical and behaves exactly as before. Setting `features_dependencies: false` in `.tpatch/config.yaml` restores full v0.5.3 semantics for projects that pin tpatch behaviour by SHA.
- **Out of scope for v0.6.0** (deferred): provider-assisted parent-patch injection in the M12 resolver (ADR-011 D8); auto-inference of `created_by` from file paths (PRD §4.3.1); soft-only cascade modes.

## v0.6.0 — 2026-04-26 — Feature Dependencies (Tranche D)

First user-facing release of the feature-dependency DAG. Features can now declare hard / soft parents; apply, reconcile, status, and remove all observe the graph. Default-on (toggle via `features_dependencies: false`). PRD: `docs/prds/PRD-feature-dependencies.md` · ADR: `docs/adrs/ADR-011-feature-dependencies.md` · User reference: `docs/dependencies.md`.

### Added

- **M14.1 — Schema + validator (`internal/store`)** — `status.json` gains `depends_on: [{slug, kind, satisfied_by?}]` (omitempty round-trip). `Config.FeaturesDependencies` (`features_dependencies: true|false` in `.tpatch/config.yaml`). Pure DAG primitives: `DetectCycles`, `TopologicalOrder`, `Children`. Validator (`ValidateDependencies` / `ValidateAllFeatures`) with five sentinel errors: self-dep, dangling, kind-conflict, cycles, satisfied_by-requires-upstream-merged. Atomic edit semantics (rejected change leaves store untouched).
- **M14.2 — Apply gate + ordering (`internal/workflow`)** — `tpatch apply --mode execute` checks each hard parent before any file mutation; lists unsatisfied parents with their states. Soft parents never gate. Recipe-level `created_by` field on every operation: declares "this op was authored by parent feature X", validated against the recipe's `depends_on`. Cycle-aware traversal everywhere; `RunImplement` and `RunApply` topo-sort dependents.
- **M14.3 — Reconcile labels + compound presentation (`internal/store`, `internal/workflow`)** — Three composable labels overlayed on `Reconcile.Outcome`: `waiting-on-parent`, `blocked-by-parent`, `stale-parent-applied`. `EffectiveOutcome()` reports the compound string `blocked-by-parent-and-needs-resolution` when the child's intrinsic outcome is `blocked-requires-human` AND `blocked-by-parent` is set (display-only — programmatic decisions still read `Outcome` and `Labels` separately). Source-truth guard: all DAG/label code reads `status.Reconcile.Outcome` via `store.LoadFeatureStatus`, NEVER `artifacts/reconcile-session.json` (ADR-010 D5). Adversarial test pins this.
- **M14.4 chunk A — `tpatch status --dag`** — ASCII renderer (hard `─►`, soft `┄►`) with full-graph and scoped-to-slug modes; `--json` flag emits a structured shape for harnesses. Cycle-safe (corrupted graph degrades to flat list with `⚠ cycle detected` warning rather than recursing).
- **M14.4 chunk B — Default flip** — `features_dependencies` now defaults to **true** when the key is absent. `Init()` template writes the explicit `true`. v0.5.3 byte-identity preserved by setting `features_dependencies: false`.
- **M14.4 chunk C — Dependency-management CLI** — New verb tree `tpatch feature deps [<slug> [add|remove] <parent>[:hard|:soft]] | --validate-all`. `tpatch amend <slug> --depends-on <parent>[:kind] / --remove-depends-on <parent>` (deps-only mode skips request.md rewrite). `tpatch remove <slug> --cascade` performs reverse-topo deletion with TTY confirm; `--cascade --force` skips the prompt for non-TTY use. **`--force` alone never bypasses the dep-integrity gate** (PRD §3.7, ADR-011 D7).
- **M14.4 chunk D — Status-time validation** — `tpatch status` (with or without `--dag`) re-runs `ValidateAllFeatures` and surfaces every cycle / dangling / kind-conflict inline.
- **M14.4 chunk E — 6-skill rollout** — All six shipped skill formats (Claude, Copilot, Copilot Prompt, Cursor, Windsurf, Generic workflow) updated atomically with the dependency surface. `created_by` description updated from "inert" (v0.5.x) to live apply-time gate (v0.6.0+) with the dry-run downgrade noted. Parity guard (`assets_test.go`) holds.
- **M14.4 chunk F — User reference** — `docs/dependencies.md` covers edge model, declaration paths, validation rules, apply-time gate, `created_by` op-level gate, reconcile labels, compound verdict, `status --dag` examples, `--cascade` contract, migration, and the explicit out-of-scope list.

### Changed

- `tpatch apply --mode execute` now **fails fast** when an op's `created_by` parent is missing from `depends_on` or when a hard parent is unsatisfied. `tpatch apply --dry-run` downgrades the missing-hard-parent miss to a **warning** so operators can inspect the planned changes; recipe-shape errors (parent absent from `depends_on`, unknown kind) remain hard in both modes (PRD §4.3).
- `Reconcile.Labels` is now persisted on `status.json` (omitempty — empty slices round-trip to `nil`).
- `tpatch status` topology and presentation are unchanged on dependency-free repos; the new label / DAG output only appears when relevant.

### Notes

- **Version**: bumped to `0.6.0` in `internal/cli/cobra.go`.
- **No new external Go dependencies**; stdlib + `cobra/pflag` only.
- **No tag is created in this commit** — supervisor performs the `v0.6.0` tag at closeout.
- **Backward compatibility**: a v0.5.3 repo that does not declare any `depends_on` continues to round-trip byte-identical and behaves exactly as before. Setting `features_dependencies: false` in `.tpatch/config.yaml` restores full v0.5.3 semantics for projects that pin tpatch behaviour by SHA.
- **Out of scope for v0.6.0** (deferred): provider-assisted parent-patch injection in the M12 resolver (ADR-011 D8); auto-inference of `created_by` from file paths (PRD §4.3.1); soft-only cascade modes.


## v0.5.3 — Shadow Accept Accounting Fixes (Tranche C3)

Three follow-up findings from an external reviewer on the v0.5.2 shadow-accept flow. One silent correctness regression (manual `reconcile --accept` broken after shadow-awaiting), one missing end-to-end regression test, one status-metadata inconsistency that would mis-feed M14.3 DAG label composition.

### Fixed

- **c3-separate-resolution-artifact** (silent correctness bug) — Two writers had collided on `artifacts/reconcile-session.json`: the resolver wrote `ResolveResult` with per-file `outcomes[]`, then `saveReconcileArtifacts` overwrote the same path with the high-level `ReconcileResult` (no outcomes). `loadResolvedFiles` — called by manual `reconcile --accept <slug>` and `--shadow-diff` — then failed with "no resolved files recorded". Fix (Option A): split the artifacts. Resolver now writes `artifacts/resolution-session.json` (per-file outcomes, resolver-owned); `reconcile-session.json` remains the reconcile-owned high-level summary (external contract unchanged). `loadResolvedFiles` + `shadow-diff` + the `tryPhase35` notes string all read the new path. Drift audit synchronized 5 skill/prompt/workflow assets + `docs/agent-as-provider.md` + `docs/prds/PRD-provider-conflict-resolver.md`.
- **c3-accept-stamps-reconcile-outcome** (internal consistency / M14.3 blocker) — `workflow.AcceptShadow` marked `State=applied` and cleared the shadow pointer but left `Reconcile.Outcome=shadow-awaiting` stale in `status.json`. M14.3 label composition (ADR-011 D6) reads `Reconcile.Outcome` as the child's intrinsic verdict before overlaying DAG labels — stale outcome would yield wrong labels. Fix: `clearShadowPointerAndStamp` signature extended to `(s, slug, sessionID, phase)`; now sets `Reconcile.Outcome = ReconcileReapplied` and refreshes `Reconcile.AttemptedAt`. Auto-apply path already wrote the same value at the outer `updateFeatureState` — double-write is harmless (idempotent). Manual `--accept` now leaves a truthful terminal outcome.

### Added

- **c3-manual-accept-regression-test** — `TestGoldenReconcile_ManualAcceptFlow` in `internal/workflow/golden_reconcile_test.go`. Drives `RunReconcile(Resolve:true)` to a `shadow-awaiting` verdict, parses `resolution-session.json` inline (mirrors the CLI `loadResolvedFiles` path), calls `workflow.AcceptShadow`, then asserts: merged content on disk, `State=applied`, `Reconcile.Outcome=reapplied`, `ShadowPath` cleared, shadow directory pruned. Counterpart to `TestGoldenReconcile_ResolveApplyTruthful` (v0.5.2) — together they cover both shadow-accept paths end-to-end. Would have caught both artifact-collision and stale-outcome bugs in v0.5.2.

### Notes

- Version bumped to `0.5.3` in `internal/cli/cobra.go`.
- `gofmt -l .` clean · `go build ./cmd/tpatch` ok · `go test ./...` all green · assets parity guard passes.
- All 3 findings shipped as single-purpose commits on `main` (`4636878`, `3ac7465`, `8a4af4b`).
- Backward compatibility: an old `reconcile-session.json` written by v0.5.2's resolver with the pre-split schema is ignored on v0.5.3; re-running `reconcile --resolve` creates the correct `resolution-session.json`. Shadow worktrees are ephemeral — no migration needed.
- Code-review sub-agent verdict: **APPROVED**. Both manual and auto-apply paths converge on `Reconcile.Outcome=reapplied` with no divergence.

## v0.5.2 — Correctness Fix Pass (Tranche C2)

Six confirmed findings from the v0.4.3..v0.5.1 delta review. One silent correctness bug on the v0.5.0 headline feature (`reconcile --resolve --apply`), one index-dirt bug, one stale-guard gap, one contract-drift bug, one feature addition, one doc drift. 8 regression tests added. No new Go dependencies.

### Fixed

- **c2-resolve-apply-truthful** (silent correctness bug) — `reconcile --resolve --apply` could set `ReconcileReapplied` without ever copying the shadow worktree into the real tree. Root cause: `ResolveVerdictAutoAccepted` was mapped directly to `Reapplied` by the caller, while the actual copy logic lived only in the manual `--accept` CLI path. Fix: new shared helper `workflow.AcceptShadow` owns the full accept sequence (forward-apply non-conflicting hunks → copy shadow → real via `ensureSafeRepoPath` → refresh artifacts → mark state → prune shadow). Both the manual `--accept` path and the auto-apply path route through it. On mid-flight failure the shadow is preserved and outcome maps to `ReconcileBlockedRequiresHuman` with instructions (per ADR-010 D4). Regression guards: `TestAcceptShadowCopiesResolvedContentToRealTree`, `TestAcceptShadowErrorsWithoutShadow`, `TestGoldenReconcile_ResolveApplyTruthful`.
- **c2-refresh-index-clean** — `DiffFromCommitForPaths` used `git add -N` (intent-to-add) to surface untracked files in diffs but never cleaned up, leaving intent-to-add entries in the user's real git index after reconcile/refresh. Fix: run the diff against a throwaway index via `GIT_INDEX_FILE` (temp file, deferred unlink, seeded from the real index). Regression guard: `TestRefreshAfterAcceptLeavesIndexClean` (byte-compares `git status --porcelain` before/after + checks `git ls-files --stage` for intent-to-add marker).
- **c2-recipe-hash-provenance** — Recipe stale guard only detected HEAD drift, not content drift. Modifying `apply-recipe.json` bytes without a new commit went unnoticed. Fix: provenance sidecar now records `recipe_sha256` at generation; `apply --mode execute` warns if either HEAD or hash differs from stored. Backward compatible with legacy sidecars (missing hash field) — emits "predates recipe-hash guard" note, does not error. Regression guards: `content-drift-warning` and `legacy-sidecar-skips-hash-check` subtests of `TestApplyExecuteRecipeStaleGuard`.
- **c2-remove-piped-stdin** — `printf 'y\n' \| tpatch remove <slug>` refused with "non-TTY" even though the v0.5.1 contract said piped stdin auto-confirms. Fix: TTY check inverted — non-TTY now auto-yes (matches shipped contract); interactive TTY still prompts `[y/N]`; `--force` always skips. Regression guard: `TestRemovePipedStdinSkipsConfirmation` (uses `os.Pipe()`, not a fake reader).

### Added

- **c2-amend-append-flag** — New `tpatch amend --append <slug>` flag for append semantics; replace stays the default (per supervisor decision). `--append` and `--reset` are mutually exclusive (rejected with clear error). Tests: `TestAmendAppendConcatenates`, `TestAmendAppendAndResetRejected`. Structured section-aware append left for a future enhancement.

### Docs

- **c2-max-conflicts-drift** — 8 doc/skill sites claimed `--max-conflicts` default was 3; runtime (`DefaultMaxConflicts = 10`) was correct. Fixed all 8 (CHANGELOG, `docs/agent-as-provider.md`, and 6 shipped skill/prompt/workflow formats). Parity guard passes.

### Notes

- Version bumped to `0.5.2` in `internal/cli/cobra.go`.
- `gofmt -l .` clean · `go build ./cmd/tpatch` ok · `go test ./...` all green · assets parity guard passes.
- All 6 findings shipped as single-purpose commits on `main` (`36e058d..73cd648`).
- Code-review sub-agent verdict: **APPROVED**. No drift remains between manual and auto accept paths; `ReconcileReapplied` now unreachable without `AcceptShadow` success for shadow-based paths.

## v0.5.1 — UX Polish & Quick Wins (Tranche C1 / M13)

Low-risk, high-daily-use-impact improvements. 8 items; no new Go dependencies; all prior tests remain green.

### New

- **c1-recipe-stale-guard** — `tpatch implement` now writes `.tpatch/features/<slug>/artifacts/recipe-provenance.json` (sidecar, not a field on `apply-recipe.json` — avoids updating all 6 skill formats). `tpatch apply --mode execute` compares the current recipe hash + HEAD against the sidecar and prints a stderr warning if either drifted since implementation.
- **c1-apply-default-execute** — `tpatch apply` default mode flipped from `prepare` to `auto` (chains `prepare → execute → done`). **Breaking UX**: pass `--mode prepare` explicitly to retain v0.5.0 behavior. `applyCmd` refactored into `runApplyPrepare / Started / Execute / Done / Auto` helpers.
- **c1-add-stdin** — `tpatch add` now accepts the feature description from stdin when piped, e.g. `echo "Fix model ID translation" | tpatch add`. Empty stdin is rejected; positional args still work.
- **c1-progress-indicator** — Braille spinner (150ms cadence) shown during every LLM call. Wired at the single `GenerateWithRetry` choke point so it covers `analyze / define / explore / implement` uniformly. TTY-only by default; can be forced on for tests.
- **c1-edit-flag** — New `tpatch edit <slug> [--artifact <name>]` opens feature artifacts (`spec.md`, `exploration.md`, `apply-recipe.json`, etc.) in `$EDITOR`. Default artifact is state-aware.
- **c1-feature-amend** — New `tpatch amend <slug> [<additional notes...>|<stdin>] [--reset]` appends or replaces the feature description. Refuses missing features.
- **c1-feature-removal** — New `tpatch remove <slug> [--force]` deletes `.tpatch/features/<slug>/` and refreshes `FEATURES.md`. Interactive `[y/N]` prompt on TTY; `--force` or piped stdin skips it.
- **c1-record-lenient** — New `--lenient` flag on `tpatch record` skips reverse-apply round-trip validation (for whitespace-sensitive files where the check would false-positive). The default failure message now points users at `--lenient`. See commit for investigation notes — synthetic repros of the reported markdown false-positive all passed cleanly, so we ship the documented escape hatch rather than a speculative root-cause fix.

### Breaking UX

- `tpatch apply` without `--mode` now runs the full prepare→execute→done chain. Users or agents that relied on the previous `prepare`-only default must pass `--mode prepare` explicitly.

### Notes

- Version bumped to `0.5.1` in `internal/cli/cobra.go`.
- No changes to skill assets; parity guard green.
- All 9 tranche items landed as 9 single-purpose commits on `main`.

## v0.5.0 — Provider-Assisted Conflict Resolution (Tranche B2 / M12)

Headline ship of ADR-010: `tpatch reconcile --resolve` now routes 3-way conflicts through the configured provider, one file at a time, inside a **shadow worktree** (`.tpatch/shadow/<slug>-<ts>/`) so the real working tree is untouched until you `--accept`.

### New

- **Phase 3.5 in `reconcile`** — after `PreviewForwardApply` returns `ForwardApply3WayConflicts` and `--resolve` is set, the resolver hands each conflicted file to the provider with `spec.md` + `exploration.md` + base/ours/theirs + the `<<<<<<<`-marked conflict as context. Proposed resolutions land in a shadow git worktree and go through a validation gate: rejected if any `<<<<<<<` / `>>>>>>>` markers remain, or if resolver output fails the JSON schema.
- **New flags on `reconcile`**: `--resolve`, `--apply` (auto-accept when every file is `resolved`; requires `--resolve`), `--max-conflicts N` (abort before provider call if count > N, default 10), `--model <name>` (per-run override), `--accept <slug>` / `--reject <slug>` / `--shadow-diff <slug>` (terminal operations on a pending shadow session — mutually exclusive; slug is the flag value, not a positional arg). `validateReconcileFlags` rejects nonsensical combos before `openStoreFromCmd`.
- **Three new verdicts**: `shadow-awaiting` (all files resolved; feature state `reconciling-shadow`), `blocked-requires-human` (validation failed or no provider configured — ADR-010 D9: no heuristic fallback), `blocked-too-many-conflicts` (count > `--max-conflicts`; provider never called).
- **New feature state `reconciling-shadow`** — surfaced by `tpatch status` with the shadow path so agents acting as provider (Path B) can `ls` the shadow, edit files directly, and `tpatch reconcile --accept <slug>`.
- **`reconcile-session.json`** — `.tpatch/features/<slug>/reconciliation/reconcile-session.json` records per-file status, validation reasons, model used, shadow path, and overall verdict. Source of truth for Path B shadow editing.
- **`--accept` is surgical, not a blind copy** — applies non-conflicting hunks of `post-apply.patch` via `git apply --3way --exclude=<resolved>`, overlays resolved files from the shadow, regenerates `artifacts/post-apply.patch` scoped to the feature's touched files (intent-to-add ensures new files appear), and snapshots the resolution delta as `patches/NNN-reconcile.patch`. `apply-recipe.json` is deliberately NOT auto-regenerated (lossy from a raw diff) — re-run `tpatch implement` or `tpatch record` if the recipe matters to you. Without this fix the accept flow would leave the tree half-reconciled (non-conflicted hunks never applied).
- **Skill updates (all 6 formats)** — Claude, Copilot, Copilot Prompt, Cursor, Windsurf, Generic workflow, plus `docs/agent-as-provider.md`. New "Reconcile Phase 3.5" section in every format documents the flags, verdicts, `reconcile-session.json` schema, shadow worktree concept, and the authoritative accept-flow algorithm. Claude SKILL extends the feature-state diagram with the `reconciling-shadow` branch. Parity guard green.
- **Golden scenarios** — `internal/workflow/golden_reconcile_test.go` exercises `RunReconcile` end-to-end across the five ADR-010 acceptance scenarios: clean-reapply, shadow-awaiting, validation-failed, too-many-conflicts, and no-provider. Fixtures capture real `git diff --cached HEAD` output so `git apply --3way` can find its base blobs. A single `go test -run GoldenReconcile` now serves as the ADR-010 acceptance suite.

### Design notes

- **No heuristic fallback (ADR-010 D9)** — if `--resolve` is set and no provider is configured, the verdict is `blocked-requires-human` immediately. The CLI never silently degrades to a rule-based merger.
- **Back-compat** — `ReconcileOptions` zero-value preserves v0.4.x behavior (no phase 3.5, no shadow). All new `ReconcileSummary` fields are `omitempty`. Nothing changes for callers that don't opt in via `--resolve`.
- **`promoteIfMarkers` v0.4.4 guard preserved** on strict and 3-way-clean paths; phase 3.5 only runs in the 3-way-conflicts branch, so the defensive conflict-marker scan is never bypassed.

### Also in v0.5.0

- **bug-features-md-stale-state** — `FEATURES.md` was only regenerated inside `AddFeature`, so every subsequent state transition (`analyze`, `define`, `explore`, `implement`, `apply`, `record`, `reconcile`) updated `status.json` but left the human-readable index stuck on the original state. Most visible on Path B flows (`add → apply --mode started → --mode done → record`): the table kept showing `requested` even after the feature was fully `applied`. Fix: `SaveFeatureStatus` now calls `RefreshFeaturesIndex` after every write, so the index is always a projection of the live feature statuses. Errors refreshing the index are swallowed (status.json remains the source of truth; next write retries). New `TestSaveFeatureStatusRefreshesIndex` locks this in.

### Follow-ups registered

- `feat-resolver-heuristic-fallback` — opt-in `--heuristic` (basic `git merge -Xours/theirs` + `git rebase` attempts) for future consideration; blocked until the provider path has a track record.
- `feat-parallel-feature-workflows` — fan out multiple features across shadow worktrees for parallel agent workers.
- `feat-feature-standalonify` — rebase a feature that used to depend on another into a standalone feature.

## v0.4.4 — Honest Recipes (pre-B2 ground truth)

Two HIGH-severity bugs surfaced by the v0.4.3 live stress test on tesseracode/t3code (~20h, 9 features, 1 upstream sync). Tight patch release: no new features, fix the ground truth so Tranche B2 (provider-assisted conflict resolution) can land on a reconcile that doesn't lie.

### Fixes

- **bug-skill-recipe-schema-mismatch** — the v0.4.3 skills documented `apply-recipe.json` with wrong field names (`op` instead of `type`, `contents` instead of `content`), an invented `occurrences` field, and an unimplemented `delete-file` op. Every Path B user hit `ERROR: unknown operation type ""` on the first `apply --mode execute`. Corrected in all 6 formats (Claude, Copilot skill/prompt, Cursor, Windsurf, Generic) and in `docs/agent-as-provider.md`. Documented the supported `append-file` op (previously omitted). Added `TestSkillRecipeSchemaMatchesCLI` — a new parity-guard pass that extracts every ```json block from every skill, unmarshals the `operations` array into the authoritative `workflow.RecipeOperation` struct with `DisallowUnknownFields`, and verifies the op type against the CLI's switch. Prevents future drift: any field the skill documents that the CLI rejects (or vice versa) fails the build.
- **bug-reconcile-reapplied-with-conflict-markers** — the reconcile phase-4 preview had a degraded fallback: if `git worktree add` failed (bare repo, permissions, full disk), it silently dropped to `git apply --3way --check` and returned verdict `3WayClean` — the exact behaviour v0.4.2's A4 was supposed to eliminate. Fixed: the degraded path now returns `Blocked` with a clear "cannot verify 3-way merge cleanliness — refusing to guess" stderr. Added a belt-and-braces defensive pass: `ScanConflictMarkers` walks the live working tree after every Reapplied verdict and promotes to `Blocked` if any `<<<<<<< / >>>>>>>` markers are found, naming the offending files. New regression test `TestReconcilePromotesOnLiveMarkers` plants markers in an unrelated file and asserts promotion.

### Context

Both bugs were blockers for Tranche B2 (provider-assisted conflict resolution, ADR-010):
- B2 hinges on agents writing correct `apply-recipe.json` — Bug 1 made every agent-authored recipe fail.
- B2's entry point is the `3WayConflicts` verdict — Bug 2 meant reconcile could silently return `Reapplied` instead, never triggering the resolver.

No behavioural changes beyond the fixes. `--manual`, Path A/B, the v0.4.3 skills' structural additions all carry forward unchanged.

## v0.4.3 — Stand-In Agent, Part 1 (Tranche B1)

First slice of Tranche B. Surfaces the "agent-as-provider" pattern that emerged from v0.4.2 stress testing as a first-class workflow, and lets the agent advance feature state without calling the configured provider.

### New

- **`--manual` / `--skip-llm` flag on `analyze`, `define`, `explore`, `implement`** — when the agent has authored the phase's artifact by hand (Path B), pass `--manual` to advance state without invoking the provider. The flag validates that the expected artifact exists at the canonical path (and, for `implement`, is valid JSON) and refuses otherwise, pointing at the exact file. `--skip-llm` is an alias.
- **Skill rewrite (all 6 formats)** — Claude, Copilot skill, Copilot prompt, Cursor, Windsurf, Generic now teach Path A (CLI) and Path B (agent-authored) as equal peers. New sections in every format: "You Are the Provider" (when and why to take over), `apply-recipe.json` operation schema (literal search semantics, `EnsureSafeRepoPath`), "Patch vs recipe — mental model", and the 3WayConflicts playbook (`git checkout stash@{0}^3 -- .tpatch/`, never pop the stash). Parity guard extended from 10 → 16 anchor phrases.
- **`docs/agent-as-provider.md`** — long-form companion covering Path B end-to-end with worked recipe examples and a sample 3WayConflicts resolution.

### Design

- **ADR-010 `provider-conflict-resolver`** — locks the shape of the headline v0.5.0 feature (B8): phase 3.5 in reconcile, shadow worktree, per-file provider call with spec + exploration as intent, validation gate, report + `--apply`/`--accept`/`--reject` flags.
- **PRD `agent-as-provider-skills`** — full scope for this tranche (Path A/B contract, artifact map, flag spec, skill requirements, deferred items).

## v0.4.2 — Truthful Errors (Tranche A)

Ten fixes + three new docs surfaced by the v0.4.1 live stress test. Theme: when something goes wrong, say so loudly instead of silently advancing state.

### Fixes

- **A1 bug-implement-silent-fallback** — the implement phase no longer swallows LLM failures. Fallback to heuristic mode now emits a stderr warning naming the retry count, the underlying error, the raw-response artefact path, and the `max_tokens_implement` knob to try next. New `max_tokens_implement` config (default 16384, up from 8192 hard-coded).
- **A2 bug-cycle-state-mismatch** — `RunImplement` writes `state=implementing` instead of `state=defined`. Each `tpatch cycle` phase now asserts the state advanced post-Run* via `featureStateRank`.
- **A3 bug-record-validation-false-positive** — record now validates via `git apply --reverse --check` (proves round-trip against the tree the patch was captured from). The old forward `--check` produced guaranteed false positives because the patch is, by definition, already applied.
- **A4 bug-reconcile-phase4-false-positive** — phase 4 now runs `--3way` inside an isolated `git worktree` and classifies via a 4-state verdict: `Strict | 3WayClean | 3WayConflicts | Blocked`. Conflict markers promote to `ReconcileBlocked` instead of silently succeeding.
- **A5 bug-skill-invocation-clarity** — all 6 skill formats (Claude, Copilot skill/prompt, Cursor, Windsurf, Generic) carry three canonical top-of-file blocks: Invocation (no npx), Phase Ordering (state machine), Before You Run Anything (preflight). Parity guard enforces anchor phrases so the wording cannot drift.
- **A6 bug-provider-set-global** — `tpatch provider set` defaults to the **global** config (`$XDG_CONFIG_HOME/tpatch/config.yaml`); `--repo` overrides per-repo. Matches the user-level nature of provider config and stops failing outside a `.tpatch/` tree.
- **A7 bug-extract-json-robustness** — one `ExtractJSONObject` helper replaces four ad-hoc extractors. Brace-balanced, string-aware, handles trailing prose, nested objects, arrays, escaped quotes, bare fences. Subsumes `stripJSONFences`.
- **A8 doc-record-timing** — `tpatch record` on a clean tree without `--from` now refuses with exit 1, a "captured 0 bytes" diagnostic, and up to 10 candidate base commits from `git log`. Dirty-but-empty-diff case gets a distinct hint.

### New documentation

- **A8 docs/record.md** — two supported orderings (working tree / `--from`), the anti-pattern, decision table, refusal example.
- **A9 docs/feature-layout.md** — file-by-file reference with the big "canonical vs audit trail" callout: `artifacts/post-apply.patch` is always the replay target; `patches/NNN-*.patch` is append-only audit history with full-diff snapshots, not incremental deltas. `tpatch record` now prints a cleanup hint when `patches/` exceeds six files.
- **A10 docs/reconcile.md** — two supported workflow patterns, the anti-pattern, troubleshooting block, full preflight contract.

### A10 reconcile preflight

- `tpatch reconcile` refuses dirty trees, lingering conflict markers, and `*.orig`/`*.rej` leftovers. Error message names every violating file and prescribes the remediation (abort merge, reset, stash, or `--allow-dirty` override).
- `tpatch reconcile --preflight` — CI-friendly gate: runs the checks and exits, no reconcile phases.
- `tpatch reconcile --allow-dirty` — escape hatch with a warning banner; verdicts may be wrong.
- On successful reconcile, tips you off if `.tpatch/` is untracked in git.

### Deferred to v0.5.x / v0.6.0 (logged in session tracker)

Ideas captured during Tranche A for future milestones: `feat-init-skill-drift`, `feat-soft-recipe-mode`, `feat-noncontiguous-feature-commits`, `feat-max-tokens-uncapped`, `feat-record-auto-base`, `feat-patches-subcommand`, `feat-record-dedup-patches`, `feat-ci-cd-integration`, `feat-autoresearch-iterate-until-green`, `feat-delivery-modes`.

---

## v0.4.1 and earlier

See commit history — changelog adopted at v0.4.2.
