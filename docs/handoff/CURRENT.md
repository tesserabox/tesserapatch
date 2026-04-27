# Current Handoff

## Active Task

- **Task ID**: M15-W3-DESIGN
- **Milestone**: M15 → Wave 3 (lifecycle / reconcile semantics tranche), **design-first**
- **Description**: Write **one combined PRD** covering `feat-verify-command` + `feat-feature-tested-state`, plus a companion **ADR-012** for the state-machine extension. NO CODE in this dispatch — design only. The next agent (separate dispatch) will implement against the approved design.
- **Status**: In Progress — implementer dispatched
- **Assigned**: 2026-04-27

## Why one PRD covers both

The reviewer's go-to-tag note explicitly required clarifying how `feat-verify-command` and `feat-feature-tested-state` relate before scoping either. They likely share machinery (`verify` is the most natural producer of the `tested` state) and they share contract surface (does `tested` satisfy hard dependencies? does `verify` transition state? do `verify` checks include reachability of `satisfied_by`?). Splitting into two PRDs forces those decisions twice and risks them drifting apart. Single PRD, two implementation slices later.

## Scope

### Must cover in `docs/prds/PRD-verify-and-tested-state.md`

1. **Goals + non-goals.** Clearly call out what `verify` is NOT (not a re-apply, not a reconcile, not a test runner — those exist as `apply`, `reconcile`, `test`).
2. **`tpatch verify <slug>` contract.**
   - Set of checks. Minimum starter set, in order:
     - spec.md present and non-empty
     - exploration.md targets exist in the working tree
     - apply-recipe.json (if present) parses and op targets resolve to real paths
     - apply-recipe.json operations re-apply cleanly to a clean shadow / fresh-branch workspace
     - artifacts/post-apply.patch still applies cleanly to the upstream baseline (if recorded)
     - dependency metadata passes `store.ValidateDependencies` (already exists)
     - `satisfied_by` SHAs are 40-hex AND reachable from HEAD (already enforced at edit time post-v0.6.1; verify re-checks for drift since edit)
     - any newly proposed checks (call them out explicitly)
   - Output: pass/fail per check, machine-readable JSON option (`--json`), actionable remediation per failure mode.
   - State transitions: does `verify` ever change `FeatureState`? Decision required.
   - Failure semantics: does `verify` exit non-zero on any failed check? On all? Configurable?
   - Harness integration: how does an agent harness know to run `verify` between phases?
   - Interaction with the existing `test` command: do they compose? Sequence? Mutual?
3. **`tested` lifecycle state.**
   - Where does `tested` sit in the state machine? Between `applied` and `active`? Or alongside? Truth table required.
   - Producer: only `verify`? Also `test`? Both? Manual via `amend`?
   - Persistence: same `status.json` schema, no new file.
   - **Critical contract questions** (each needs an explicit answer in the PRD):
     - Does `tested` satisfy hard dependencies? (i.e. does the apply-time gate accept a `tested` parent the same way it accepts `applied`?)
     - Does `tested` interact with reconcile labels? (`waiting-on-parent` etc.)
     - Does `upstream_merged` short-circuit `tested` (already-shipped parent never needs verify)?
     - Forward/backward transitions: can a feature regress from `tested` back to `applied`? On what trigger?
   - Backwards compatibility: a v0.6.1 repo that never sees `verify` keeps every status.json byte-identical; new state only appears once `verify` is run for the first time on that feature.
4. **CLI surface.**
   - `tpatch verify <slug> [--json] [--shadow] [--fresh-branch]` — exact flag set + defaults.
   - Optional: `tpatch verify --all` for batch.
   - `tpatch status` rendering of `tested` state (DAG and flat).
   - `tpatch amend <slug> --state tested` if manual flip is in scope (decision required).
5. **Skill / harness updates.**
   - Which of the 6 skill formats need updates and what changes.
   - Parity guard implications.
6. **Out of scope.**
   - Explicitly defer: code-presence reconcile verdicts, fresh-branch reconcile mode, anything that touches `artifacts/post-apply.patch` as authoritative source.
   - List the 4 remaining Wave 3 candidates and what RELATION they have to verify/tested but are NOT part of this PRD.
7. **Implementation slices for downstream waves.**
   - Slice A — verify command shell (no state transition). Lowest risk.
   - Slice B — tested state + state-machine plumbing.
   - Slice C — verify wired to produce tested state.
   - Slice D — `verify --all`, JSON output polish, harness docs.
   - Each slice is independently dispatchable; the PRD must be specific enough that an implementer can pick one slice and write code without further design.

### Must cover in `docs/adrs/ADR-012-feature-tested-state.md`

Standard ADR shape (see ADR-011 for reference, ~145 lines). Decisions to lock:

1. **Where `tested` sits in the FeatureState enum.** Linear vs branching state machine.
2. **`tested` satisfies hard dependencies: yes / no / configurable.** This is the single most consequential decision; argue it explicitly with both directions.
3. **Producers of `tested`.** verify-only vs verify+test+amend. Trade-offs.
4. **Backwards-compatibility contract.** Byte-identity for v0.6.1 repos that never run verify.
5. **Transitions.** Allowed forward edges (e.g. `applied → tested`, `tested → upstream_merged`), allowed backward edges (e.g. `tested → applied` on what trigger), and disallowed edges (e.g. `requested → tested` directly).
6. **Source-truth alignment.** `tested` must NOT be inferred from `artifacts/reconcile-session.json` or any non-`status.json` source (ADR-011 D6).

## Constraints (binding for the implementer)

- **No code changes** in this dispatch. Only `docs/prds/PRD-verify-and-tested-state.md` and `docs/adrs/ADR-012-feature-tested-state.md`.
- **Reuse existing primitives** where they exist: `store.ValidateDependencies`, `store.satisfiedBySHARe` (40-hex regex), `gitutil.IsAncestor` (reachability), `gitutil.CapturePatchScoped`, `internal/workflow.UserShell` / `shellQuoteFor`.
- **Source-truth guard (ADR-011 D6)**: any reconcile-related decision MUST read `status.Reconcile.Outcome`, NEVER `artifacts/reconcile-session.json`. Bake this into PRD §verify-checks where it touches reconcile state.
- **Recipe-op JSON schema is frozen.** No `delete-file` op (separate ADR before that ships). PRD must NOT assume schema extension.
- **`status.json` schema additions must be omitempty + round-trip stable.** A v0.6.1 repo with no verify history must round-trip byte-identical.
- **Harness contract:** any new CLI surface must support a `--json` machine-readable output mode for harness integration. Document exact JSON shape in the PRD.
- **No ADR-011 amendments without an explicit, justified change in this ADR-012.** ADR-011's `Reconcile.Outcome` source-truth guard, the apply-time dependency gate behaviour, and the 40-hex satisfied_by contract are all locked.

## Process

1. Implementer (dispatched as `m15-w3-design-implementer`) writes the PRD + ADR.
2. Implementer runs no Go tests (this is design-only); validates that PRD/ADR cross-references resolve and that no contract conflicts exist with ADR-011 / ADR-010 / ADR-006 / `docs/dependencies.md`.
3. Implementer updates this CURRENT.md with files written, decisions made, open questions surfaced for review.
4. Reviewer (dispatched as `m15-w3-design-reviewer`) critiques the design against the constraints above. Looks for: contract conflicts, missing decisions, unclear slicing, missing failure modes, missing JSON shape, missing source-truth guard, ergonomics gaps.
5. Supervisor decides: APPROVED / NEEDS REVISION / dispatch the first implementation slice.

## Files Changed

- `docs/prds/PRD-verify-and-tested-state.md` — **NEW.** Combined PRD for `feat-verify-command` + `feat-feature-tested-state`. ~700 lines. Enumerates the 10-check verify sequence (V0–V9) with severities and primitive citations, full state-transition truth table, the consequential D2 decision (`tested` satisfies hard deps — pragmatic equivalence to `applied`), parent-state demotion hook, `--json` schema with three failure-case examples, four implementation slices (A: command shell, B: state plumbing, C: wiring, D: polish), explicit out-of-scope cross-links to `feat-reconcile-code-presence-verdicts`, `feat-reconcile-fresh-branch-mode`, and the frozen recipe-op schema.
- `docs/adrs/ADR-012-feature-tested-state.md` — **NEW.** Locks D1–D7 with full alternatives-considered. Cross-references ADR-011 D3/D6/D9 and ADR-010 D2/D5; preserves the v0.6.1 source-truth guard verbatim.
- `docs/handoff/CURRENT.md` — this file; Files Changed / Decisions made / Open questions for reviewer / Test Results sections updated. Active Task / Status / Process / Constraints / Context for Next Agent blocks intact.

No Go file modified. No test modified. No ADR-011 edit. No reconcile-session.json read.

## Decisions made

The PRD records the *what* and the *why-now*; the ADR locks the architecture. Summary of locked choices (each is justified in ADR-012 with both directions argued where the dispatch demanded):

1. **D1 — `tested` is a linear, sibling state of `applied`** in `FeatureState`. Not branching, not a parallel flag. Lives between `applied` and `active`. Single-string `state` field is preserved.
2. **D2 — `tested` satisfies the hard-dep gate** (pragmatic equivalence to `applied`). `CheckDependencyGate` satisfaction set extends from `{applied, upstream_merged}` to `{applied, tested, upstream_merged}`. Both directions argued at length in PRD §3.4.4 + ADR-012 D2; B-strict (reject `tested` parents) and the alt-3 derived-label option are deliberately rejected. This is the consequential decision the dispatch flagged.
3. **D3 — `verify` is the unique producer of `tested`.** `tpatch test`, `amend --state tested`, and implicit-`verify`-after-`apply` are all rejected (alternatives recorded). Producer expansion is reserved for `feat-tested-state-test-producer` (out of scope).
4. **D4 — Backwards-compat: byte-identical round-trip for v0.6.1 repos that never run `verify`.** Enforced by a regression fixture (PRD §7 + ADR D4). Enum extension is additive; no field-shape change; no new file.
5. **D5 — Forward / backward transition table.** Verify-driven edges: `applied → tested`, `tested → tested` (idempotent), `tested → applied` on block-severity FAIL. Cross-cutting demotions: `apply --mode execute on tested → applied`; `amend (recipe-touching) on tested → applied`; `amend (intent-only)` preserves `tested`; `reconcile (reapplied/upstreamed) on tested → applied` (NOT `tested`). Parent-state hook: hard-parent regression demotes direct children one step (cascade depth = 1).
6. **D6 — Source-truth guard preserved verbatim from ADR-011 D6 / ADR-010 D5.** `tested` is persisted in `status.json` only; V9's reconcile-outcome check reads `status.Reconcile.Outcome` only, never `artifacts/reconcile-session.json` or `artifacts/resolution-session.json`. Adversarial test pins this.
7. **D7 — Verify is read-only on the working tree.** Apply-simulation (V7) runs in a `gitutil.CreateShadow` worktree pruned via `defer PruneShadow(...)`. Per-slug shadow lock: verify refused while reconcile is in-flight (state ∈ `{reconciling, reconciling-shadow}`).

Slicing decision (downstream Wave 3 dispatches) — adopted the dispatch's A/B/C/D boundaries:
- **Slice A** — Verify command shell + V0–V9 checks; `--no-promote --no-demote` only; no enum / state plumbing yet.
- **Slice B** — `tested` enum + state-transition plumbing (`CheckDependencyGate` extension, `apply`/`amend`/`reconcile` demotion paths, parent-state hook); no verify wiring.
- **Slice C** — Wire verify into state transitions; `amend --state tested` rejection; `status` rendering.
- **Slice D** — Skill bullets in 6 surfaces, parity guard, `verify --all`, CHANGELOG, polish.

## Open questions for reviewer

The PRD surfaces these instead of guessing — supervisor / reviewer to adjudicate:

1. **Q1 (PRD §6 / §10) — V9 (`reconcile_outcome_consistent`) severity: warn (default) vs block?** PRD defaults warn so a feature with `Reconcile.Outcome = shadow-awaiting` can still be promoted to `tested` (the shadow is awaiting accept/reject, but the feature on disk is structurally intact). Counter-argument: stronger contract — `tested` implies "no pending reconcile work." If reviewer pushes block, escalate to ADR-012 amendment before Slice C lands. Default in this PRD: **warn**.
2. **Q3 (PRD §6) — `verify --all` should skip pre-apply states?** PRD defaults to skip-with-line: features in `requested`/`analyzed`/`defined`/`implementing` are reported as "skipped: pre-apply state" and verify-all exit code reflects only post-apply slugs. Slice D detail; not blocking Slice A.
3. **PRD §4.3 example 3 UX subtlety** — the Failure case 3 JSON shows `passed: false` on warn-only checks coexisting with `verdict: passed` and `state_after: tested`. The remediation text intentionally describes the situation, not a hypothetical block, but reviewers may judge the `passed: false` field name confusing on warn-only checks. Alternative: rename to `ok` or split into `passed_block` / `passed_warn`. Default: keep `passed` per check, with severity carrying the gating semantics.
4. **D2 wording check** — ADR-012 D2 states "`tested` satisfies, but only because it implies `applied`-level guarantees; it gains no extra power vs. `applied` for gating purposes." Reviewer should confirm this framing reads correctly: the gate accepts both states identically, and the operator-facing semantics are "verify makes a feature stronger, but for dep-gating purposes the strengthening is invisible." If the framing reads as contradictory, propose alt wording.
5. **Slice B parent-state hook performance** — the hook fires in the same `LoadFeatureStatus` post-processing loop that today recomputes `Reconcile.Labels` (M14.3). Reviewer to confirm this is the right insertion point and that the additional check (parent left `{applied, tested, upstream_merged}` AND child currently `tested`) is cheap enough to keep the hot path. PRD §3.4.5 documents this as "no new hot path"; if reviewer disputes, escalate.

## Test Results

N/A — design-only dispatch. No Go code, no test changes. PRD/ADR cross-references hand-validated (see PRD §10 cross-cutting impact matrix and ADR-012 References section); both documents resolve internally and against ADR-010, ADR-011, `docs/dependencies.md`, and CHANGELOG v0.6.1.

## Next Steps

After PRD + ADR approval: dispatch Slice A (`verify` command shell) as the first M15-W3 code wave. The implementation handoff will reference the PRD/ADR sections that bound the work.

## Blockers

None.

## Context for Next Agent

- **`tpatch` binary at the repo root is NOT gitignored.** Always `rm -f tpatch` after any inadvertent `go build`. (Design dispatch should not build, but mentioning for completeness.)
- **Commit trailer mandatory**: `Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>`. Use `git -c commit.gpgsign=false`.
- **Source-truth guard (ADR-011 D6)**: any DAG/label/status code reads `status.Reconcile.Outcome` via `store.LoadFeatureStatus`, NEVER `artifacts/reconcile-session.json`. Bake into PRD verify-checks.
- **Recipe vs patch authority**: `artifacts/post-apply.patch` is the reconcile source of truth. Recipes serve replay/inspection. PRD must respect this.
- **Hookable-var pattern**: `var isAncestor = gitutil.IsAncestor` (Wave 1), `var userShellFor` (Wave 2). Convention for unit-test isolation; design should anticipate continued use.
- **`satisfied_by` contract (post-v0.6.1)**: 40-hex AND reachable, enforced at edit time. `verify` re-checks reachability for drift since edit.
- **Self-reviews are status signals only.** Per the v0.6.1 fix-pass lesson, the reviewer agent's verdict on this PRD is one input — supervisor will request an external read before any implementation slice ships.
