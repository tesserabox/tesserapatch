# AGENTS.md

## Agent Roles

### Supervisor Agent

Oversees the consolidation project. Picks tasks from the ROADMAP, spawns implementation and review cycles, makes final approval decisions, and maintains the tracking system.

**Owns**: `docs/ROADMAP.md`, `docs/milestones/*.md`, `docs/handoff/HISTORY.md`, `docs/supervisor/LOG.md`

### Implementation Sub-Agent

Executes a bounded task: either planning work (PRD, ADR) or code implementation. Reads the handoff, does the work, updates the handoff.

**Owns**: `cmd/`, `internal/`, `assets/`, `tests/`, `SPEC.md`, `CLAUDE.md`, `AGENTS.md`, `docs/handoff/CURRENT.md`

### Reviewer Sub-Agent

Reviews implementation sub-agent output against SPEC and ROADMAP. Runs the review checklist. Writes a verdict.

**Owns**: `docs/supervisor/LOG.md` (review entries only)

---

## The Cyclic Workflow

The project advances through repeating implementation → review cycles:

```
┌───────────────────────────────────────────────────────────────┐
│                        SUPERVISOR                             │
│                                                               │
│  1. Read ROADMAP → pick next task                             │
│  2. Write task scope to docs/handoff/CURRENT.md               │
│  3. IMPLEMENTATION PHASE                                      │
│     └─ Sub-agent reads handoff, implements, updates handoff   │
│  4. REVIEW PHASE                                              │
│     └─ Sub-agent reviews changes, writes verdict to LOG.md    │
│  5. DECISION                                                  │
│     ├─ APPROVED → archive handoff, mark task complete, → 1    │
│     ├─ APPROVED WITH NOTES → mark complete w/ follow-up, → 1 │
│     ├─ NEEDS REVISION → add feedback to handoff, → 3          │
│     └─ BLOCKED → log blocker, skip to next task, → 1          │
└───────────────────────────────────────────────────────────────┘
```

### Implementation Phase Details

The implementation sub-agent must:

1. Read `docs/handoff/CURRENT.md` — understand the task
2. Read relevant SPEC sections, reference files from other teams if needed
3. Do the work:
   - **If planning**: Write a PRD (`docs/prds/<name>.md`) or ADR (`docs/adrs/<name>.md`)
   - **If coding**: Write Go code, tests, update assets, update docs
4. Verify: `gofmt`, `go test ./...`, `go build ./cmd/tpatch`
5. Update `docs/handoff/CURRENT.md` with:
   - What was done
   - Files changed
   - Test results
   - Remaining issues
   - Context for reviewer

### Review Phase Details

The reviewer sub-agent must:

1. Read the updated `docs/handoff/CURRENT.md`
2. Read the changed files
3. Run the review checklist:
   - [ ] Code compiles: `go build ./cmd/tpatch`
   - [ ] Tests pass: `go test ./...`
   - [ ] Code formatted: `gofmt -l .`
   - [ ] `.tpatch/` artifacts are deterministic and documented
   - [ ] Secrets kept out of tracked files
   - [ ] CLI behavior matches `SPEC.md`
   - [ ] Handoff file is accurate and complete
   - [ ] If assets changed: parity guard passes
   - [ ] No regressions to previously passing functionality
4. Write verdict to `docs/supervisor/LOG.md`

### Supervisor Decision

After the review verdict, the supervisor:

- **APPROVED**: Archives handoff to HISTORY.md, checks off the task in milestones, updates ROADMAP status, picks next task
- **NEEDS REVISION**: Adds specific feedback to CURRENT.md, sends implementation sub-agent back to iterate
- **BLOCKED**: Logs the blocker, moves to the next unblocked task

---

## Handoff File Contract

`docs/handoff/CURRENT.md` must always contain:

```markdown
# Current Handoff

## Active Task
- **Task ID**: M0.1
- **Milestone**: M0 — Bootstrap
- **Description**: [what needs to be done]
- **Status**: [Not Started | In Progress | Review | Complete | Blocked]
- **Assigned**: [date]

## Session Summary
[What was done in this session]

## Current State
[What works, what's partial, known issues]

## Files Changed
[List of files created/modified]

## Test Results
[Build output, test results]

## Next Steps
[Numbered list of what comes next]

## Blockers
[Any blockers]

## Context for Next Agent
[Non-obvious things the next agent needs to know]
```

When a handoff entry is superseded, append it to `docs/handoff/HISTORY.md` with a timestamp header.

---

## Supervisor Log Format

Each entry in `docs/supervisor/LOG.md`:

```markdown
## Review — [Task ID] — [Date]

**Reviewer**: [agent identifier]
**Task**: [description]

### Checklist
- [ ] Compiles
- [ ] Tests pass
- [ ] Formatted
- [ ] Artifacts deterministic
- [ ] Secrets safe
- [ ] Matches SPEC
- [ ] Handoff accurate

### Verdict: [APPROVED | APPROVED WITH NOTES | NEEDS REVISION | BLOCKED]

### Notes
[Specific observations, feedback, or follow-up items]

### Action Taken
[What the supervisor did: archived handoff, assigned next task, sent back for revision]
```

---

## File Ownership

| File/Folder | Owner |
|-------------|-------|
| `SPEC.md` | Implementation, reviewed by Reviewer |
| `CLAUDE.md` | Implementation |
| `AGENTS.md` | Supervisor |
| `docs/ROADMAP.md` | Supervisor |
| `docs/milestones/*.md` | Supervisor |
| `docs/handoff/CURRENT.md` | Implementation (writes), Supervisor (reads, archives) |
| `docs/handoff/HISTORY.md` | Supervisor |
| `docs/supervisor/LOG.md` | Reviewer + Supervisor |
| `docs/adrs/*.md` | Implementation (writes), Reviewer (reviews) |
| `docs/prds/*.md` | Implementation (writes), Reviewer (reviews) |
| `cmd/`, `internal/`, `assets/` | Implementation |
| `tests/` | Implementation |

## Naming Conventions

- Milestones: `M0`, `M1`, `M2`, ...
- Tasks: `M0.1`, `M0.2`, `M1.1`, ...
- ADRs: `ADR-001-<topic>.md`, `ADR-002-<topic>.md`, ...
- PRDs: `PRD-<feature-name>.md`
- Feature slugs in target repos: kebab-case, e.g., `fix-model-id-translation`

## Context Preservation Rules

**These rules are enforced, not aspirational. A task is not complete until the tracking documents reflect its state.**

1. **Update `docs/handoff/CURRENT.md` at every phase transition** — not just at session end. Phases include: starting a new task, finishing a unit of work, switching from implementation to review, discovering a blocker.
2. **Before context gets low**: Write everything you know to the handoff — assume the next agent has zero context beyond what's in the files.
3. **After each task completion**: Archive the old CURRENT.md entry to `docs/handoff/HISTORY.md` immediately. Do not batch.
4. **After each review**: Append the verdict to `docs/supervisor/LOG.md` immediately. Do not batch.
5. **After every milestone transition**: Update the milestone box in `docs/ROADMAP.md` (✅ / ⬜ / 🚧). If you touch a milestone, you touch the roadmap.
6. **ADR on every architecture decision**: When you evaluate alternatives and pick one, write an ADR in `docs/adrs/`. "Decision by agent consensus" is not a valid answer — future agents need the reasoning.
7. **Plan.md is session-local, tracking docs are repo-global**: Use `plan.md` (in `~/.copilot/session-state/...`) for scratch planning. The moment a decision is made, propagate it into `CURRENT.md`, `ROADMAP.md`, or an ADR — do not leave load-bearing context in session state.
8. **Never leave tracking stale**: If you did work, update tracking *before* stopping. If you stopped because of a blocker, update tracking with the blocker *before* handing off.

### Cadence cheatsheet

| Trigger | Update |
|---------|--------|
| Started a new task | `CURRENT.md` Active Task block |
| Finished analyze/define/explore/implement | `CURRENT.md` Session Summary + Files Changed |
| Ran tests (pass or fail) | `CURRENT.md` Test Results |
| Hit a blocker | `CURRENT.md` Blockers + `LOG.md` if it affects review |
| Made an architecture choice | New `ADR-00N-<topic>.md` |
| Reviewer wrote a verdict | `LOG.md` entry + handoff transition |
| Milestone done | `ROADMAP.md` status flip + milestone file closed |
| Task done | `CURRENT.md` → `HISTORY.md` archive + new `CURRENT.md` for next task |
