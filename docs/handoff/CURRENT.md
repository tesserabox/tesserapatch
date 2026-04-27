# Current Handoff

## Active Task

- **Task ID**: `feat-satisfied-by-reachability` (+ Wave 1/2 implementation brief)
- **Milestone**: M15 stream — v0.6.x stabilization and Path B follow-through
- **Status**: Planned — ready to hand to implementation agent
- **Assigned**: 2026-04-26

## Session Summary

**M15.1 shipped** — `created_by` auto-inference at implement time. Advisory only (recipe never mutated), hard-parents only, opt-out via `--no-created-by-infer`, flag-off byte-identity preserved. Reviewer APPROVED on commit `6407b6b`; closeout commit `ee6d6c8` archived the handoff; followup chore `e8542fa` gitignored the `.tpatch-backlog/` mirror dump.

This closes the v0.6.0 user-experience loop: instead of a hard `ErrPathCreatedByParent` at apply time, users now get a stderr suggestion at implement time pointing at the likely parent.

**Planning update (2026-04-26)** — backlog registrations from the case-study sweep were verified in the read-only SQL mirror. All 11 new IDs exist, plus an extra external registration: `chore-skill-frontmatter`.

**Not yet shipped as a release.** Version still `0.6.0`; no CHANGELOG entry yet. M15.1 is mid-cycle polish — supervisor decides whether to batch with the next stabilization item(s) into a `v0.6.1` cut, or tag now.

## Current State

- HEAD is `e8542fa` on `origin/main` (`.tpatch-backlog/` gitignore chore), with M15.1 review + archive at `6407b6b` + `ee6d6c8`.
- Build clean, full test suite green, parity guard holds.
- ROADMAP M14 ✅ (Tranche D); no M15 box flipped (M15 is treated as a backlog stream, not a tranche).
- Next implementation slice should start with `feat-satisfied-by-reachability`. It is additive, dependency-free, and explicitly closes the deliberate M14 limitation documented in the dependency gate.
- Wave 1 and Wave 2 items below are all `pending` in the SQL mirror and all have zero unmet dependencies.
- Review boundary: stop after Wave 2 before taking on lifecycle/reconcile semantics (`feat-verify-command`, `feat-feature-tested-state`, `feat-reconcile-code-presence-verdicts`, `feat-reconcile-fresh-branch-mode`).

## Files Changed

Planning transition only:
- `docs/handoff/CURRENT.md` (this file — convert idle handoff into implementation brief)

## Test Results

Last green validation gate: M15.1 reviewer pass (see top entry of `docs/supervisor/LOG.md`).

Planning-only update; no code changed in this transition.

## Next Steps — recommended sequencing

Implementation sequencing for the next agent:

1. **Wave 1 — bounded stabilization**
	- `feat-satisfied-by-reachability`
	- `chore-skill-frontmatter`
	- `feat-define-spec-alias`
   
	Why first: smallest surface area, lowest regression risk, and all three are user-visible quality improvements. This wave is a good candidate for an early checkpoint if unexpected DAG or skill-loader fallout appears.

2. **Wave 2 — Path B correctness and ergonomics**
	- `bug-test-command-shell-selection`
	- `feat-record-autogen-recipe`
	- `bug-recipe-stale-after-manual-flow`
	- `feat-record-scoped-files`
   
	Why second: these are the strongest recurring case-study issues after DAG work shipped. Together they improve the practical manual/agent-authored path without opening reconcile design questions yet.

3. **Review gate — stop here for review**
	- Expected point to request review: after Waves 1 and 2 are green and handoff/log/docs are updated.
	- Reason: the next tranche changes lifecycle semantics and reconcile truthfulness, which is a materially larger product/design surface.

4. **Wave 3 — after review**
	- `feat-verify-command`
	- `feat-feature-tested-state`

5. **Wave 4 — after review**
	- `feat-reconcile-code-presence-verdicts`
	- `feat-reconcile-fresh-branch-mode`
	- likely companion follow-ups: `feat-patch-compatibility`, `feat-reconcile-metadata-refresh`

6. **Defer for now**
	- `feat-feature-provider-overrides`
	- `feat-upstream-merged-override`
	- `feat-explore-prereq-enforcement`
	- `feat-agent-collision-detection`
   
	These remain valid but are either more invasive, more policy-heavy, or lower priority than the current stabilization / Path B tranche.

### Newly registered from 2026-04-26 case-study sweep

11 backlog items added based on field reports + parallel-recovery case study (full descriptions in SQL):

| ID | Theme |
|---|---|
| `feat-satisfied-by-reachability` | M14.1 limitation (#1 above) |
| `feat-define-spec-alias` | Naming polish — `define` ↔ `spec` |
| `feat-feature-provider-overrides` | Per-feature provider/model pinning (cost control) |
| `feat-upstream-merged-override` | Auditable manual flip with reason + commit/PR/SHA |
| `feat-verify-command` | New `tpatch verify <slug>` — re-applicability check, distinct from `test` |
| `feat-reconcile-fresh-branch-mode` | Codify fresh-branch recovery pattern from case studies |
| `feat-reconcile-code-presence-verdicts` | Evidence-based reconcile (kill false-positive `upstreamed`) |
| `feat-feature-tested-state` | New lifecycle state between `applied` and `active` |
| `bug-test-command-shell-selection` | Stop hardcoding `sh` (Windows/WSL bug) |
| `feat-explore-prereq-enforcement` | Loud guardrail before `implement` |
| `feat-agent-collision-detection` | File-hash drift warnings (parallel-recovery scenarios) |

Already-covered themes (do **not** re-register): `feat-record-autogen-recipe`, `bug-recipe-stale-after-manual-flow`, `feat-record-scoped-files`, `feat-prompt-anti-hallucination`, `feat-agentic-tool-use`, `feat-skill-artifact-schemas`, `feat-reconcile-metadata-refresh`, `feat-patch-compatibility`, `feat-feature-autorebase`, `feat-reconcile-reapply-action`. Standalone "remove temperature" intentionally **not** registered — fold into `feat-feature-provider-overrides` if that ships.

Backlog stream view: 64 pending todos in SQL after this packet. See `docs/supervisor/LOG.md` for review history.

### Likely code owners for Wave 1 / Wave 2

- `feat-satisfied-by-reachability`
	- likely files: `internal/workflow/dependency_gate.go`, `internal/workflow/dependency_gate_test.go`, `internal/store/validation.go`, `internal/store/validation_test.go`, and possibly `internal/gitutil/gitutil.go` if a small `IsAncestor` helper is added around `git merge-base --is-ancestor`.
	- key design constraint: keep `satisfied_by` provenance semantics, but harden validation so obviously wrong SHAs stop being silently accepted.

- `chore-skill-frontmatter`
	- likely files: `assets/skills/claude/tessera-patch/SKILL.md`, `assets/skills/copilot/tessera-patch/SKILL.md`, `assets/assets_test.go`.
	- goal: add valid YAML frontmatter without perturbing command anchors enforced by the parity guard.

- `feat-define-spec-alias`
	- likely files: `internal/cli/cobra.go`, CLI tests under `internal/cli/cobra_test.go`, and possibly any skill/docs text that enumerates phase commands.
	- goal: alias only; do not fork phase semantics or artifact naming.

- `bug-test-command-shell-selection`
	- likely files: `internal/workflow/validation.go`, `internal/workflow/validation_test.go`, `internal/cli/phase2.go`, and maybe config docs if an explicit shell override is introduced.
	- current footgun: `exec.Command("sh", "-c", cfg.TestCommand)` is Unix-specific.

- `feat-record-autogen-recipe`, `bug-recipe-stale-after-manual-flow`, `feat-record-scoped-files`
	- likely files: `internal/cli/cobra.go` (`recordCmd`), `internal/workflow/refresh.go`, `internal/workflow/recipe.go`, related tests in `internal/cli/cobra_test.go`, `internal/workflow/refresh_test.go`, and potentially `internal/gitutil/` helpers if scoped capture needs file filtering.
	- important product boundary: keep `artifacts/post-apply.patch` as reconcile source of truth; recipe generation is there to make Path B replay/inspection better, not to demote the patch.

### Validation expectations for implementation agent

For each item or tightly-coupled pair, prefer the narrowest falsifying check first, then rerun before moving on:

1. focused package tests for the touched slice (`internal/workflow`, `internal/store`, `internal/cli`, or `assets` parity)
2. `gofmt -w` on touched Go files
3. `go test ./...`
4. `go build ./cmd/tpatch`

Review request should happen after Waves 1 and 2, not mid-tranche, unless Wave 1 exposes unexpected DAG/validation behavior.

## Blockers

None.

## Context for Next Agent

- **`tpatch` binary at the repo root is NOT gitignored.** Bare `tpatch` ignore would shadow `cmd/tpatch/`. Always `rm -f tpatch` after `go build ./cmd/tpatch` BEFORE staging.
- **Commit trailer mandatory**: `Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>`. Use `git -c commit.gpgsign=false`.
- **Source-truth guard (ADR-011 D6)**: any DAG/label/status code reads `status.Reconcile.Outcome` via `store.LoadFeatureStatus`, NEVER `artifacts/reconcile-session.json`.
- **`--force` is NOT a DAG-integrity bypass** (PRD §3.7, ADR-011 D7). Only `--cascade` opts into removing a feature with downstream dependents.
- **Skill parity guard** (`assets/assets_test.go`) enforces required CLI-command anchors and the recipe-op JSON schema. Treat as a real reviewer.
- **`git push` is slow** (60+ seconds typical).
- **M15.1 architectural choice**: inference errors degrade to warnings (don't block recipe persistence). Apply-time gate is the safety net. Mirror this pattern for future advisory features.
- **`internal/workflow/created_by_gate.go`** = apply-time gate (M14.2 + correctness pass + C5). **`internal/workflow/created_by_inference.go`** = implement-time advisor (M15.1). They are separate concerns; do not entangle.
- **Reachability work should stay cheap and additive.** Prefer a tiny git helper over spreading raw `git merge-base --is-ancestor` shelling across workflow/store packages.
- **Record-path work must preserve the existing source-of-truth rule.** `artifacts/post-apply.patch` remains authoritative for reconcile; recipe improvements should not invert that.
