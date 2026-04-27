# Current Handoff

## Active Task

- **Task ID**: M15.1 — `created_by` auto-inference at implement time (PRD §4.3.1)
- **Milestone**: M15 — v0.6.x stabilization & polish (post-Tranche-D)
- **Status**: Implementation complete — awaiting reviewer
- **Assigned**: 2026-04-26
- **Estimated size**: ~120–180 LOC + tests; one logical commit

## Why this is next

v0.6.0 just shipped `created_by` as a real apply-time gate (M14.2 schema + correctness pass + C5 fix-pass). First-time users will hit `ErrPathCreatedByParent` when their recipe omits the annotation. The PRD already specified an advisory inference heuristic at implement time (§4.3.1, line 381 of `docs/prds/PRD-feature-dependencies.md`); shipping it now closes the user-experience loop while users are field-testing v0.6.0.

This is **stabilization-tier polish** — small, additive, advisory-only. Not a milestone tranche.

## Files Changed

- `internal/workflow/created_by_inference.go` (new, ~210 LOC) — advisory matcher; `WithDisableCreatedByInference` ctx helper; `inferCreatedBy` scanner; `pristineHasSearch` working-tree probe.
- `internal/workflow/created_by_inference_test.go` (new, ~270 LOC) — all 8 tests from the dispatch contract.
- `internal/workflow/implement.go` — call `inferCreatedBy(ctx, s, slug, recipe)` between recipe parse and recipe write; failures degrade to a warning so persistence is never blocked.
- `internal/cli/cobra.go` — `--no-created-by-infer` flag on `implement` command, plumbed via `workflow.WithDisableCreatedByInference`.

The created-by **gate** (`internal/workflow/created_by_gate.go`) was NOT touched — apply-time concern, separate file, separate sentinel.

## Test Results

```
$ gofmt -l .
(no output)

$ go build ./cmd/tpatch && rm -f tpatch
BUILD OK

$ go test ./...
ok  	github.com/tesseracode/tesserapatch/assets	0.362s
?   	github.com/tesseracode/tesserapatch/cmd/tpatch	[no test files]
ok  	github.com/tesseracode/tesserapatch/internal/cli	4.007s
ok  	github.com/tesseracode/tesserapatch/internal/gitutil	(cached)
ok  	github.com/tesseracode/tesserapatch/internal/provider	(cached)
ok  	github.com/tesseracode/tesserapatch/internal/safety	(cached)
ok  	github.com/tesseracode/tesserapatch/internal/store	(cached)
ok  	github.com/tesseracode/tesserapatch/internal/workflow	8.567s

$ go test ./internal/workflow -run 'CreatedByInference|CreatedByGate' -count=1 -v
--- PASS: TestCreatedByGate_FlagOff_NoOp (0.01s)
--- PASS: TestCreatedByGate_HardParent_TargetMissing_ErrPathCreatedByParent (0.01s)
--- PASS: TestCreatedByGate_DryRun_HardParent_TargetMissing_DowngradesToWarning (0.01s)
--- PASS: TestCreatedByGate_Execute_HardParent_TargetMissing_ReturnsErr (0.01s)
--- PASS: TestCreatedByGate_HardParent_TargetExists_NoError (0.01s)
--- PASS: TestCreatedByGate_SoftParent_TargetMissing_FallsThroughWithWarning (0.01s)
--- PASS: TestCreatedByGate_ParentNotInDependsOn_RecipeRejected (0.01s)
--- PASS: TestCreatedByGate_ParentUpstreamMerged_TargetExists_NoError (0.01s)
--- PASS: TestCreatedByGate_AppliesToReplaceAndAppend (0.01s)
--- PASS: TestCreatedByInference_SuggestsHardParent (0.01s)
--- PASS: TestCreatedByInference_RespectsExistingAnnotation (0.01s)
--- PASS: TestCreatedByInference_AmbiguousMultipleParents (0.01s)
--- PASS: TestCreatedByInference_SkipsSoftParents (0.01s)
--- PASS: TestCreatedByInference_OptOut (0.01s)
--- PASS: TestCreatedByInference_FlagOff (0.01s)
--- PASS: TestCreatedByInference_PristineHasSearch_NoSuggestion (0.01s)
--- PASS: TestCreatedByInference_NoMatchSilent (0.01s)
PASS
ok  	github.com/tesseracode/tesserapatch/internal/workflow	0.535s

$ go test ./assets/... -count=1
ok  	github.com/tesseracode/tesserapatch/assets	0.334s
```

All 9 pre-existing CreatedByGate tests + all 8 new CreatedByInference tests green. Full suite green.

## Deviations from dispatch contract

None. The advisory output, opt-out flag, scope guards (replace-in-file only, hard parents only, non-transitive, no recipe mutation, advisory stderr only, flag-off byte identity), and 8-test layout all match the handoff verbatim.

One minor implementation note for the reviewer: the inference call inside `RunImplement` is wrapped in a `if ierr != nil { warn }` guard rather than a hard-return. Rationale: a transient error in the advisory layer must not block recipe persistence — the apply-time gate is the authoritative enforcement point and would catch any real downstream issue. The dispatch contract didn't pin this either way; this is the conservative choice.

## Next Steps

1. Reviewer dispatch.
2. On APPROVED: archive this handoff, decide on `v0.6.1` cut.

## Blockers

None.

## Context for Next Agent

- The inference scanner is intentionally cheap: it only loads the child status when at least one candidate op exists (fast-path skip), reads each parent's `post-apply.patch` once and caches the bytes for the whole walk, and short-circuits as soon as the pristine working tree contains the Search bytes.
- `ctxKeyDisableCreatedByInfer` is declared with explicit value `1` to sit alongside `ctxKeyDisableRetry = iota = 0` in `retry.go` — same `contextKey` private type. If we add a third workflow-context flag, switch them all to a `const ( ... iota )` block to avoid drift.
- `--no-created-by-infer` is `implement`-only by design (PRD §4.3.1 places inference there, the gate is a separate CLI surface on `apply`). Do not promote.
- Pending follow-ups (separate backlog, NOT in scope here):
  - `feat-satisfied-by-reachability` — git merge-base check on `satisfied_by` SHAs
  - v0.6.0 field-feedback issues if any surface
  - `--auto-apply-inferred` — if operators ask for it, the inference layer is now structured to support recipe mutation as a follow-up.

## Why this is next

v0.6.0 just shipped `created_by` as a real apply-time gate (M14.2 schema + correctness pass + C5 fix-pass). First-time users will hit `ErrPathCreatedByParent` when their recipe omits the annotation. The PRD already specified an advisory inference heuristic at implement time (§4.3.1, line 381 of `docs/prds/PRD-feature-dependencies.md`); shipping it now closes the user-experience loop while users are field-testing v0.6.0.

This is **stabilization-tier polish** — small, additive, advisory-only. Not a milestone tranche.

## Context

Tranche D / M14 fully shipped at `v0.6.0` (commit `d2aa4d8`, tag pushed 2026-04-26). The DAG, dep-management CLI, status --dag, 6-skill rollout, and `docs/dependencies.md` are all live. `features_dependencies: true` is the default. See `docs/handoff/HISTORY.md` (top entry: M14.4) and `docs/supervisor/LOG.md` for full background.

## Spec — PRD §4.3.1

> `tpatch implement` is updated to suggest `created_by` population when the provider / heuristic output includes `replace-in-file` ops whose `Search` text cannot be found in the pristine checkout but *is* present in a parent feature's `post-apply.patch`. Low-cost heuristic; explicit opt-out via `--no-created-by-infer`. Inference only suggests hard parents (soft parents wouldn't gate anyway).

Risk-mitigation note from §8 (line 686):

> `created_by` inference is advisory: always printed for operator review at `implement` time; `--no-created-by-infer` disables it. Manual edits survive re-runs.

## Scope

### Algorithm

For each operation in the implement-phase recipe output where:
1. `op.Kind == "replace-in-file"`
2. `op.CreatedBy == ""` (already-set values are user authority — never overwrite)
3. `op.Search` is non-empty

Run this matcher:

a. Read the file at `op.Path` from the working tree (pristine checkout — implement runs before apply). If the file exists and `bytes.Contains(fileContent, []byte(op.Search))`, the Search text is present → no inference needed.

b. If the file does NOT exist OR Search not found in it: scan all **hard** parents declared in the child's `status.json.depends_on[].kind == "hard"`. For each hard parent, read `<parent>/artifacts/post-apply.patch` and check `bytes.Contains(parentPatch, []byte(op.Search))`.

c. If exactly ONE hard parent's patch contains the Search text → suggest `created_by: <parent-slug>`.

d. If MULTIPLE hard parents match → emit an ambiguity W ("multiple parents contain this text: %v; please set created_by manually") and do not auto-suggest.

e. If NO hard parents match → silent (no suggestion; the eventual apply-time gate will surface the real error).

### Output channel — ADVISORY ONLY

The inference results are **printed to stderr** for operator review. They are NOT silently written into the recipe. The user reviews the suggestions and either:
- accepts them by editing the recipe manually (current workflow), OR
- in a follow-up iteration we can offer `--auto-apply-inferred` (NOT in this scope).

Each suggestion line:
```
ℹ created_by inference: op #N (replace-in-file %path) → suggest created_by: %parent
   reason: Search text is missing in pristine checkout but present in %parent/artifacts/post-apply.patch
```

A summary line at end (`ℹ %d created_by suggestion(s) — review and edit apply-recipe.json before tpatch apply`) only when there is at least one suggestion.

### Opt-out

`--no-created-by-infer` flag on `implement` command. When set, the inference loop short-circuits and no scan runs.

Default: inference ON.

### Where to plug in

`internal/workflow/implement.go` runs after the provider returns the recipe. Plug the matcher between recipe parse and recipe write. The workflow already has access to:
- `store.Store` — for reading parent patches via `s.ReadFeatureFile(parentSlug, "artifacts/post-apply.patch")`
- `slug` — to load the child's `status.json` and read `depends_on`
- the parsed recipe — to walk `Operations`

`feat-feature-dependencies` flag check: skip inference entirely when `cfg.FeaturesDependencies == false` (byte-identical pre-v0.6 behavior).

### Tests (in `internal/workflow/created_by_inference_test.go`)

1. `TestCreatedByInference_SuggestsHardParent` — child has hard parent P1; P1 patch contains the Search text; pristine doesn't. Assert: stderr contains the suggestion line; recipe is NOT mutated.
2. `TestCreatedByInference_RespectsExistingAnnotation` — op already has `created_by: P1`; assert no suggestion emitted (don't second-guess the user).
3. `TestCreatedByInference_AmbiguousMultipleParents` — two hard parents both contain the text; assert ambiguity W is emitted and no specific suggestion.
4. `TestCreatedByInference_SkipsSoftParents` — soft parent's patch contains the text; hard parent's doesn't; assert no suggestion (matches PRD: "Inference only suggests hard parents").
5. `TestCreatedByInference_OptOut` — `--no-created-by-infer` → matcher never runs even when a suggestion would have been made.
6. `TestCreatedByInference_FlagOff` — `features_dependencies: false` → inference inert (byte-identical to v0.5.x).
7. `TestCreatedByInference_PristineHasSearch_NoSuggestion` — Search text IS in the pristine file; assert no scan, no suggestion.
8. `TestCreatedByInference_NoMatchSilent` — no parent contains the text; assert no output (silent fail-through; the apply-time gate will catch the real issue).

## Authoritative docs

1. `docs/prds/PRD-feature-dependencies.md` §4.3.1 line 381 (the algorithm), §8 line 686 (advisory mitigation)
2. `docs/adrs/ADR-011-feature-dependencies.md` (D4 hard-only gating)
3. `internal/workflow/created_by_gate.go` + `created_by_gate_test.go` — the EXISTING apply-time gate (correctness pass + C5 fix-pass). Inference is a separate file; do not entangle.

## Strict scope guards (DO NOT do)

- Do NOT auto-mutate the recipe — advisory output only. Operator authority preserved per PRD §8 risk note.
- Do NOT extend to `write-file` ops in this round (the `Search`-presence heuristic doesn't apply). Keep replace-in-file only.
- Do NOT scan transitively (parent-of-parent). Just the direct hard parents declared in `status.json.depends_on`.
- Do NOT introduce a new flag at any other layer than `implement` — no new config keys.
- Do NOT promote `--no-created-by-infer` into `apply` — apply has its own gate, separate concern.
- Do NOT add new external Go dependencies (cobra/pflag + stdlib only).

## Validation gate

```
gofmt -l .
go build ./cmd/tpatch && rm -f tpatch
go test ./...
go test ./internal/workflow -run 'CreatedByInference|CreatedByGate' -count=1 -v
go test ./assets/... -count=1
```

All M14 series tests stay green.

## Operational reminders

1. **`tpatch` binary at the repo root is NOT gitignored.** A bare `tpatch` ignore would shadow `cmd/tpatch/`. After every `go build ./cmd/tpatch` run `rm -f tpatch` BEFORE staging anything. Recurring slip across the entire M14 series.
2. **Every commit MUST carry the trailer**:
   ```
   Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
   ```
   Use `git -c commit.gpgsign=false commit ...`.
3. **`git push` takes 60+ seconds** on this machine.
4. Make ONE logical commit (the feature is small and self-contained).
5. Do NOT bump version. This is a `v0.6.x` polish item; supervisor decides on `v0.6.1` cut.

## Deliverable — when complete, update this file

Replace the Active Task block with `Status: Implementation complete — awaiting reviewer`, list files changed, paste the validation-gate output, and push to `origin/main`.

## Session Summary

This session closed out M14.4 (v0.6.0 cutover, tagged 2026-04-26) and fixed the LOG.md ordering (`68d02c7`). Now picking up the natural next polish item — `created_by` auto-inference — so v0.6.0 users get a hint instead of a hard error when they forget the annotation.

## Files Changed

This handoff transition only:
- `docs/handoff/CURRENT.md` (this file — M15.1 dispatch contract)

## Test Results

Last green validation gate ran during M14.4 closeout (see `docs/handoff/HISTORY.md` top entry).

## Next Steps

1. Dispatch `m15-1-implementer` (general-purpose, background) against this handoff.
2. After implementer: `m15-1-reviewer` (code-review, background).
3. On APPROVED: archive this handoff, update ROADMAP if M15 entry exists, decide whether to cut `v0.6.1` immediately or batch with the next stabilization item.

## Blockers

None.

## Context for Next Agent

- The `created_by` apply-time gate is fully wired and tested. This task is the OTHER half — implement-time inference. Do not change the gate; just add the inference module.
- The flag-off (`features_dependencies: false`) byte-identity invariant is critical. Our regression coverage has historically pinned this; do not regress.
- Read `internal/workflow/created_by_gate.go` for the existing gate's shape (file naming, error variable conventions). Mirror that style for `created_by_inference.go`.
- Pending follow-ups (separate backlog, NOT in scope here):
  - `feat-satisfied-by-reachability` — git merge-base check on `satisfied_by` SHAs
  - v0.6.0 field-feedback issues if any surface
