# Feature Layout — What lives under `.tpatch/features/<slug>/`

Every feature managed by tpatch has a directory at `.tpatch/features/<slug>/`. This doc is the authoritative map of what each file is, who writes it, and — most importantly — **which file is canonical vs. audit-trail** when you want to replay or reason about the feature.

The #1 confusion, observed in live stress testing, is that users see many numbered patches under `patches/` and wonder which one is current. Short answer: **none of them — use `artifacts/post-apply.patch`.** Long answer below.

## At a glance

```
.tpatch/features/<slug>/
├── request.md              ← user's original request, verbatim
├── analysis.md             ← LLM / heuristic analysis output (phase: analyze)
├── spec.md                 ← acceptance criteria, plan (phase: define)
├── exploration.md          ← file-level investigation (phase: explore)
├── record.md               ← human-readable summary of the last record run
├── status.json             ← machine state (state, last_command, timestamps, apply.*)
├── artifacts/
│   ├── apply-recipe.json   ← operation list (phase: implement)
│   ├── post-apply.patch    ★ CANONICAL feature diff, always-current
│   ├── incremental.patch   ← (optional) delta between two post-apply snapshots
│   ├── post-apply-diff.txt ← `git diff --stat` of the recorded patch
│   └── raw-*-response-*.txt← LLM raw responses for debugging
└── patches/
    ├── 001-started.patch   ← HISTORICAL full-diff snapshots, append-only
    ├── 002-record.patch    ← each file is a *full* diff at write-time,
    ├── 003-record.patch    ← NOT incremental. Highest number = latest.
    └── …
```

Legend: **★** = canonical; **←** = lifecycle / debug; anything under `patches/` = audit trail only.

## Canonical vs. audit trail

### `artifacts/post-apply.patch` — use this one

`tpatch record` writes this file on every invocation, overwriting the previous contents. It is *always* the current full diff of the feature against the baseline commit recorded in `status.json:apply.base_commit`.

**Replay path:**

```
git apply .tpatch/features/<slug>/artifacts/post-apply.patch
```

`tpatch reconcile` reads this file to decide what to re-apply onto a new upstream. Downstream tooling (CI/CD, scripts, agents) should treat this as the source of truth for "what does this feature do?".

### `artifacts/incremental.patch` — sometimes present

Set by the apply flow when a started/done pair produces a delta that differs from the full diff (see `DeriveIncrementalPatch` in `internal/gitutil/`). Reconcile uses it in preference to `post-apply.patch` when both exist and the delta is smaller. You can ignore it for day-to-day work — it's an optimisation detail.

### `patches/NNN-<label>.patch` — audit trail, not replay input

Every time `tpatch record` (or certain apply modes) runs, it appends a numbered snapshot here via `Store.NextPatchNumber` (scan the directory, take max+1). The labels you'll see in the wild:

| Label | Written by | Meaning |
|---|---|---|
| `record` | `tpatch record` | Full feature diff at record time |
| `started` | `tpatch apply --mode started` | Diff captured right before execute |
| `cycle` | `tpatch cycle` | Patch from a cycle run |
| `done` | `tpatch apply --mode done` | Diff captured after execute |

Each file is a **complete** diff of the feature vs baseline — not an incremental delta between `NNN` and `NNN-1`. They exist so you can audit history ("what did my feature look like three days ago?"), not so you can replay them in order. **Applying `patches/001-record.patch` replays a stale state** that is missing every amendment recorded after it.

Rule of thumb:

- **Replay or reconcile** → `artifacts/post-apply.patch`.
- **"What did the feature look like before amendment X?"** → `patches/<older-number>-*.patch`.
- **Pruning is safe** as long as you keep the latest numbered file and `artifacts/post-apply.patch`. A dedicated `tpatch patches <slug> --prune` subcommand is planned (see `feat-patches-subcommand`) — for now, `rm` the older files manually if the directory bothers you.

When `patches/` exceeds six files, `tpatch record` will print a one-line reminder so you don't have to memorise this doc.

## Lifecycle files

These are written once or twice per feature, by named phases:

| File | Written by | Purpose |
|---|---|---|
| `request.md` | `tpatch add` | The user's original prompt, stored verbatim for context. |
| `analysis.md` | `tpatch analyze` | LLM's (or heuristic's) classification + risk rating. |
| `spec.md` | `tpatch define` (alias: `tpatch spec`) | Acceptance criteria + phased plan. Drives implement. |
| `exploration.md` | `tpatch explore` | Target files + existing-code facts. Grounds implement. |
| `apply-recipe.json` | `tpatch implement` | Operation list (create/modify) the apply flow executes. |
| `record.md` | `tpatch record` | Human-readable summary of the last record run. |

## State & debug files

- `status.json` — authoritative machine state. Fields include `state`, `last_command`, `apply.has_patch`, `apply.base_commit`, and timestamps. Only `tpatch` writes this; editing it by hand is unsupported.
- `artifacts/raw-*-response-*.txt` — one file per LLM call. Inspect when an agent did something surprising; these are what `tpatch implement` hands back to `JSONObjectValidator`.
- `artifacts/post-apply-diff.txt` — `git diff --stat` output for quick eyeballing.

## Related

- [Recording Patches](./record.md) — when and how to run `tpatch record` (plus the anti-pattern refusal).
- `SPEC.md` — authoritative CLI surface and state machine.
- `AGENTS.md` — file ownership matrix for the implementation team.
