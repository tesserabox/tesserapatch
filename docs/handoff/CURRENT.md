# Current Handoff

## Active Task

- **Task ID**: M15-W3-SLICE-D
- **Milestone**: M15 Wave 3 — Verify freshness rollout (final slice)
- **Description**: Finalize the verify-freshness work by adding `tpatch verify --all` topo-ordered aggregate reporting, rolling the §4.4 freshness bullet across all 6 skill formats, extending the `assets/assets_test.go` parity guard with the new anchors, cross-linking `docs/dependencies.md` to verify, and shipping CHANGELOG v0.6.2.
- **Status**: Not Started — ready for implementer dispatch
- **Source PRD**: `docs/prds/PRD-verify-freshness.md` §9 (Slice D row), §4.4 (skill bullet contract)

## Predecessor — Slice C

✅ Approved by external supervisor on `23af23e`. Stack on `origin/main`:

- `32f50c8` — Slice C original (V3–V9 + closure-replay)
- `5892ae0` — revision-1 (V8 runs against closure-replayed baseline when recipe absent + patch present)
- `23af23e` — revision-2 (V8 precondition is file presence, not non-empty content)
- `08ed4e5` — tracking (external verdicts logged + ROADMAP flip)

Full retrospective archived in `docs/handoff/HISTORY.md` under `2026-04-29 — M15-W3-SLICE-C`.

## Scope (per PRD §9 Slice D)

1. **`tpatch verify --all`** — new aggregate runner.
   - Topologically order all features (Kahn, hard-deps first, same primitive used by `tpatch land --all` if applicable; otherwise vendored/inlined locally).
   - Skip pre-apply features (state ∈ {drafted, analyzed, defined, explored, implemented}) per PRD Q2 and emit a one-line `skipped: pre-apply` row for each. Only states with a recorded patch (applied / reconciled / verified) execute V0–V9.
   - Per-feature output: existing single-feature verdict line + checks block, prefixed with the slug.
   - Aggregate footer: counts per verdict, plus an exit code that is non-zero if ANY feature failed (verdict ∈ {failed} or any check.severity=error fails). `--json` emits a list of per-feature reports + an aggregate summary object.
   - No new state transitions; this is a read-only aggregate over existing single-feature verify.

2. **Skill bullet rollout (all 6 surfaces)** — add the §4.4 freshness bullet to:
   - `assets/skills/claude/SKILL.md`
   - `assets/skills/copilot/SKILL.md`
   - `assets/skills/copilot-prompt/tpatch.prompt.md`
   - `assets/skills/cursor/.cursor/rules/tpatch.mdc`
   - `assets/skills/windsurf/.windsurfrules`
   - `assets/skills/generic/AGENT.md`
   The bullet must verbatim match the PRD §4.4 wording (or as close as the surface allows; copy-paste anchor strings into all six files).

3. **Parity guard extension** — `assets/assets_test.go` adds anchor checks for the §4.4 bullet across all 6 surfaces. Pattern matches existing slice rollouts (one anchor substring per file).

4. **`docs/dependencies.md` cross-link** — add a short paragraph pointing readers from the dependency model to verify, since hard-dep semantics now drive V7/V8 closure replay.

5. **`CHANGELOG.md` v0.6.2 entry** — name the verb (`tpatch verify`), call out the freshness overlay (V0–V9 numbered checks), call out the explicit out-of-scope list (no provider calls, no state transitions, no `--all` interaction with shadow). Link to PRD §9 for the slice-by-slice landing.

## Constraints

- DO NOT touch `internal/workflow/verify.go` V3–V9 logic (Slice C closed). Slice D is additive surface only.
- DO NOT touch the closure-replay primitive or shadow lifecycle.
- `verify --all` must reuse the existing single-feature `RunVerify` entrypoint per feature; no separate code path.
- Pre-apply skip must be deterministic and ordered first in topo (i.e., even features with no recorded patch participate in topo order; their skip row appears at their topo position).
- Parity guard must keep the existing anchor checks intact and only ADD new ones.
- Out-of-scope file folders: `docs/whitepapers/`, exploratory PRDs (`PRD-feature-slices-and-nested-changes.md`, `PRD-intent-version-control-evaluation.md`, `PRD-record-auto-base.md`, `PRD-record-collision-detection.md`, `PRD-tpatch-git-primitive-mapping.md`, `PRD-tpatch-land.md`).

## Tests required

- **Aggregate ordering**: 3-feature DAG with one hard-dep chain (A → B → C) and one independent feature D — assert `verify --all` runs A, B, C, D in topo order; insertion order in `.tpatch/features/` must NOT determine output order.
- **Pre-apply skip**: feature in state `defined` shows up in topo position with `skipped: pre-apply` row; does NOT execute V0; does NOT cause a non-zero exit on its own.
- **Aggregate exit code**: at least one failed feature → non-zero exit; all passing → zero exit.
- **Aggregate JSON shape**: `--json` emits `{ features: [...], summary: {passed, failed, skipped, error} }` (or equivalent already established for single-feature; extend rather than break).
- **Malformed-but-present artifact case** (carryover lesson from Slice C external review): include at least one `verify --all` test where one feature has a malformed-but-present artifact (e.g., zero-byte `post-apply.patch` or invalid `apply-recipe.json`) and assert the aggregate correctly reports that feature as failed without poisoning other features in the run.
- **Parity guard**: `go test ./assets/...` green with new anchors; intentionally remove a bullet locally to confirm the guard fails.

## Validation gate (must pass before review dispatch)

1. `gofmt -l .` — empty.
2. `go build ./cmd/tpatch` — success.
3. `go vet ./...` — clean.
4. `go test ./...` — all pass; new tests counted.
5. `tpatch verify --all` smoke run on a fixture repo with at least 3 features.
6. Skill files visually inspected — §4.4 bullet present in all 6.

## Reviewer prompt notes (for Slice D reviewer dispatch)

- **Carry forward the artifact-presence gate lesson from Slice C**: any new precondition probe added in Slice D (e.g., `verify --all` skipping pre-apply, or aggregate JSON shape gating) must be exercised with a malformed-but-present artifact case. Reviewer must explicitly run a 2-cell matrix (well-formed vs malformed-but-present) on every new gate.
- Reviewer should diff the 6 skill files against each other to confirm the bullet is consistent (not just present).
- Reviewer should `gofmt -l .` and confirm no unintended file format changes.

## Out of scope (DO NOT touch)

- V3–V9 logic in `internal/workflow/verify.go` (Slice C is closed; only `verify --all` orchestration above this layer).
- Shadow lifecycle.
- Closure-replay primitive.
- Provider integration (verify is local-only per PRD §3).
- `docs/whitepapers/` and the exploratory PRDs listed in Constraints.
- `tpatch` binary at repo root (untracked artifact).

## Files Changed

(None yet — implementer dispatch pending.)

## Test Results

(None yet.)

## Session Summary

Slice C external supervisor verdict came in APPROVED on `23af23e`. Tracking commit `08ed4e5` logged all three external verdicts (Slice C original NEEDS REVISION, rev-1 NEEDS REVISION, rev-2 APPROVED) into `docs/supervisor/LOG.md` and flipped Slice C from ⬜ to ✅ in `docs/ROADMAP.md`. Pushed the full 4-commit Slice C stack + tracking commit to `origin/main`. Archived Slice C to `docs/handoff/HISTORY.md`. This handoff stages Slice D for the next implementer dispatch.

## Next Steps

1. Hold for user direction before dispatching the Slice D implementer (per the established cadence).
2. On dispatch: implementer reads this handoff + PRD §9 Slice D + §4.4 + the Slice C archive in HISTORY.md.
3. After implementer ships, dispatch the sub-agent reviewer; on APPROVED, hand off to user for external supervisor pass.
4. After Slice D APPROVED externally, tag and ship `v0.6.2` (final M15 release).

## Blockers

None.

## Context for Next Agent

- Sub-agent reviewer prior-misses pattern is now 4 cycles deep: Slice A reviewer-1, Slice B reviewer-1, bug-fix reviewer (APPROVED WITH NOTES), Slice C reviewer-1. Slice C rev-1 + rev-2 reviewers broke the streak by following strict matrix instructions. Slice D reviewer prompts should keep the matrix-coverage discipline.
- The `tpatch` binary at repo root is an untracked development artifact — do NOT add it.
- `4945093` was already pushed before Slice C work; the Slice C stack itself is `32f50c8` → `5892ae0` → `23af23e` → `08ed4e5`.
