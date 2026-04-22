# M12 — Provider-Assisted Conflict Resolver (v0.5.0)

**Goal**: Ship phase 3.5 of reconcile — per-file provider resolution in a shadow worktree with validation gate and atomic accept/reject. The v0.5.0 headline.

**PRD**: `docs/prds/PRD-provider-conflict-resolver.md`
**ADR**: `docs/adrs/ADR-010-provider-conflict-resolver.md`
**Tranche**: B2

## Tasks

| ID | Title | Depends on |
|---|---|---|
| b2-shadow-worktree | `internal/gitutil/shadow.go` — Create / Prune / Copy / ShadowDiff | — |
| b2-validation-gate | `internal/workflow/validation.go` — syntax, markers, identifiers, test_command | — |
| b2-resolver-core | `internal/workflow/resolver.go` — per-file sequential resolution | b2-shadow-worktree, b2-validation-gate |
| b2-reconcile-wiring | Wire phase 3.5 into `reconcile.go`; new verdicts | b2-resolver-core |
| b2-state-machine | `reconciling-shadow` state + `reconcile-session.json` | b2-reconcile-wiring |
| b2-cli-flags | `--resolve --apply --accept --reject --shadow-diff --max-conflicts --model` | b2-reconcile-wiring, b2-state-machine |
| b2-derived-refresh | `store.RefreshDerivedArtifacts` — atomic on accept | b2-shadow-worktree, b2-state-machine |
| b2-skills-update | 6 skills + `docs/agent-as-provider.md` document phase 3.5 | b2-cli-flags, b2-derived-refresh |
| b2-golden-tests | `tests/reconcile/golden/` scenario harness (≥5) | b2-resolver-core |
| b2-release | v0.5.0 version bump, CHANGELOG, tag, push | b2-skills-update, b2-golden-tests |

## Acceptance criteria

Mirrors `PRD-provider-conflict-resolver.md § Acceptance criteria`.

## Follow-ups (out of scope; logged as separate todos)

- `feat-resolver-parallel`
- `feat-resolver-chunked-context`
- `feat-resolver-heuristic-fallback`
- `feat-reconcile-metadata-refresh`
- `feat-feature-standalonify` (depends on `feat-feature-dependencies`)
