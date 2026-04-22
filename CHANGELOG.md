# Changelog

All notable changes to tpatch are recorded here.

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
- **New flags on `reconcile`**: `--resolve`, `--apply` (auto-accept when every file is `resolved`; requires `--resolve`), `--max-conflicts N` (abort before provider call if count > N, default 3), `--model <name>` (per-run override), `--accept <slug>` / `--reject <slug>` / `--shadow-diff <slug>` (terminal operations on a pending shadow session — mutually exclusive; slug is the flag value, not a positional arg). `validateReconcileFlags` rejects nonsensical combos before `openStoreFromCmd`.
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
