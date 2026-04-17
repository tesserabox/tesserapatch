# CLAUDE.md

## What This Repo Is

This is the **unified** Tessera Patch implementation, consolidating the best ideas from three independent prototypes:

- `cc/` — TypeScript/Node.js (broadest lifecycle, richest skills, best prompt engineering)
- `experimental/` — TypeScript/Bun (zero deps, deterministic apply recipes, path safety, offline fallback)
- `gpt/` — Go (single binary, embedded assets, replay hardening, provider-assisted reconciliation)

The unified implementation is in Go, building on the GPT team's architecture with additions from CC and experimental.

## Read This First

| Need | File |
|------|------|
| Product contract | `SPEC.md` |
| Current milestone | `docs/ROADMAP.md` |
| Active task | `docs/handoff/CURRENT.md` |
| Previous handoffs | `docs/handoff/HISTORY.md` |
| Review log | `docs/supervisor/LOG.md` |
| Team workflow | `AGENTS.md` |
| Initial review | `../review/COMPARISON.md` |
| Reconciliation review | `../review/RECONCILIATION-REVIEW.md` |
| Forward plan | `../review/RECOMMENDATIONS.md` |
| Original requirements | `../FIRST-PROMPT.md` |
| Test scenario | `../PATCH-TEST-PROMPT.md` |

## Reference Implementations (Read-Only)

These folders contain the three original prototypes. Read them for reference but do NOT edit them.

| Folder | What to look at |
|--------|----------------|
| `../gpt/` | **Primary base.** `internal/cli/`, `internal/tpatch/`, `internal/workflow/`, `internal/gitutil/`, `internal/provider/`, `assets/`, `cmd/tpatch/` |
| `../cc/` | Best SKILL.md (`src/skill/SKILL.md`), prompt templates (`src/utils/prompts.ts`), full lifecycle commands (`src/cli/`), reconciler (`src/core/reconciler.ts`), steering files |
| `../experimental/` | Apply recipe system (`src/core/workflow.ts`), path safety (`ensureSafeRepoPath`), heuristic fallback, operation-level reconciliation, mock server (`scripts/mock-copilot-server.ts`) |

## Test Artifacts (Read-Only)

| Folder | What it contains |
|--------|-----------------|
| `../tests/gpt/` | GPT team's bug bash results, replay scripts, copilot-api work clones |
| `../tests/experimental/` | Experimental team's bug bash results, replay scripts |
| `../tests/cc/` | CC team's reconciliation results |

## Working Rules

1. Only create/edit files inside this `tpatch/` folder.
2. Minimal external Go dependencies — `cobra/pflag` for CLI, stdlib for everything else.
3. Use `go embed` for bundling skill/workflow assets into the binary.
4. Do not store secrets in tracked files. Use secret-by-reference pattern.
5. Prefer deterministic artifacts in `.tpatch/` over hidden session state.
6. Run `gofmt`, `go test ./...`, and `go build ./cmd/tpatch` after code changes.
7. **Update `docs/handoff/CURRENT.md` at every phase transition** — not only before ending a session. See `AGENTS.md` → "Context Preservation Rules" for the full cadence.
8. **Log to `docs/supervisor/LOG.md` on every review** and flip `docs/ROADMAP.md` status the moment a milestone ticks over.
9. Keep shipped skill assets aligned with CLI contract. The `assets_test.go` parity guard must pass.

## Repository Shape

```text
tpatch/
  cmd/tpatch/          CLI entrypoint
  internal/cli/        Cobra command definitions (cobra.go)
  internal/store/      .tpatch manifests, store, types, slug generation
  internal/provider/   Provider abstraction (OpenAI-compatible)
  internal/workflow/   Analyze, define, explore, implement, reconcile orchestration
  internal/gitutil/    Git operations (diff, patch capture, reverse-apply, 3-way merge)
  internal/safety/     Path validation, input sanitization
  assets/              Skills, prompts, workflows, templates (embedded via go:embed)
  docs/                Roadmap, handoff, milestones, supervisor log, ADRs
  tests/               Integration test fixtures and scripts
  SPEC.md              Technical specification
  CLAUDE.md            This file (agent orientation)
  AGENTS.md            Agent roles and workflow
```

## Key Design Decisions

These are locked in from the review process. Do not change without an ADR:

1. **Go with zero deps** — Single binary distribution, embedded assets
2. **4-phase reconciliation** — Reverse-apply → operation-level → provider-assisted → forward-apply
3. **Deterministic apply recipe** — `apply-recipe.json` with validated operations, review before execute
4. **Path traversal protection** — `ensureSafeRepoPath()` before file writes outside `.tpatch/`
5. **Secret-by-reference** — Config stores env var name, not the value
6. **6 skill formats** — Claude, Copilot, Copilot Prompt, Cursor, Windsurf, Generic
7. **Parity guard test** — `assets_test.go` ensures skills stay in sync with CLI
8. **Heuristic offline fallback** — CLI works without a provider
9. **Untracked file capture** — Patches include new files, not just modifications
10. **`upstream.lock` + `steering/`** — Explicit upstream tracking and patching guidance
