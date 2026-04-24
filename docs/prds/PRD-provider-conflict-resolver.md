# PRD — Provider-Assisted Conflict Resolver (v0.5.0 / M12 / Tranche B2)

**Status**: Accepted
**Date**: 2026-04-21
**ADR**: ADR-010
**Owner**: Core
**Milestone**: M12

## Summary

Implement the v0.5.0 headline: when `tpatch reconcile` would otherwise produce `3WayConflicts`, an opt-in phase 3.5 asks the provider to resolve each conflicted file in a shadow `git worktree`. Validation gates the output; accept/reject is atomic.

The architectural shape is locked by **ADR-010**. This PRD answers the 6 open questions and specifies the data model, flag semantics, and acceptance criteria.

## Goals

- Ship the core value proposition of tpatch: "you don't worry about how to apply the changes" for the 3-way conflict case.
- Atomicity: accept is a single transition; nothing touches the real working tree until accept.
- Honesty: no silent degradation — if the provider fails, verdict is `blocked-requires-human` (ADR-010 D9).
- Reviewability: the shadow survives until explicit accept/reject, and `--shadow-diff` lets a human inspect before committing.

## Non-goals (v0.5.0)

- Parallel per-file provider calls (follow-up: `feat-resolver-parallel`).
- Chunked-by-hunk resolution for files larger than the cap (follow-up: `feat-resolver-chunked-context`).
- Heuristic fallback when the provider is unavailable (follow-up: `feat-resolver-heuristic-fallback`; ADR-010 D9 forbids the default).
- Auto-refresh of `spec.md`/`analysis.md` on accept (ADR-010 D6 — warn-only; handled by `feat-reconcile-metadata-refresh` later).
- Multi-feature parallel reconcile.

## Answers to ADR-010's 6 open questions

### Q1 — Parallel per-file calls?

**No — sequential in v0.5.0.**

- Simpler control flow: one `for` loop, one validation cycle per file, predictable error surface.
- Provider-rate-limit friendly: most hosted providers cap concurrent chat completions. Sequential avoids 429 storms.
- Debuggable: if resolution fails on file N, we have a clean stack without interleaved log output.

`feat-resolver-parallel` is logged as a v0.5.x follow-up. It becomes attractive once streaming ships and token cost becomes the bottleneck over latency.

### Q2 — Patch attribution in `patches/NNN-reconcile.patch`?

**Single combined patch per reconcile run.**

- Matches the existing numbered-snapshot convention (`NNN-<label>.patch`).
- Per-file attribution lives in `resolution-session.json` — that's the auditable record.
- Avoids N extra files per reconcile for a cosmetic gain.

### Q3 — Does `--resolve --apply` need a manual CI signal before auto-accepting?

**No. The configured `test_command` is the gate.**

- If `test_command` is unset in `config.yaml`, `--apply` is refused. Explicit — no silent auto-accept without tests.
- If `test_command` is set and passes in the shadow, auto-accept proceeds.
- This keeps v0.5.0 self-contained. CI integration (`feat-ci-cd-integration`) can layer its own gates.

### Q4 — Shared vs. per-feature shadow worktree when multiple features conflict?

**One shadow per feature.** `.tpatch/shadow/<slug>-<ts>/`.

- Isolation > disk savings. Each feature's shadow is independently accept/rejectable.
- A subsequent reconcile for the same slug reaps any prior shadow for that slug before creating a new one.
- Cross-feature shadow sharing would couple accept semantics — a hard no for atomicity.

### Q5 — Full file vs. chunked context?

**Full file in v0.5.0, with a hard cap.**

- Default `max_file_bytes: 204800` (200 KB). Configurable in `config.yaml`.
- Files over the cap are marked `skipped-too-large` in the resolution report. They do NOT fail the whole reconcile — user can resolve them manually and re-run.
- Chunked resolution (`feat-resolver-chunked-context`) is logged. It's real work (needs hunk overlap, merge-driver semantics). Not worth blocking v0.5.0.

### Q6 — Golden conflict-scenarios test harness?

**Yes. Ships with v0.5.0.**

- `tests/reconcile/golden/<scenario>/` — each scenario has `base/`, `ours/`, `theirs/`, `spec.md`, `exploration.md`, `expected.json`.
- Test harness uses a stub `Provider` that returns canned responses per scenario.
- Initial scenario set (≥5) listed in `b2-golden-tests`. Grows as real-world cases surface.

## Data model additions

### `resolution-session.json` (in `.tpatch/features/<slug>/artifacts/`)

*(Renamed from `reconcile-session.json` in v0.5.3; the old filename is
now the high-level reconcile summary owned by `saveReconcileArtifacts`.)*

```json
{
  "session_id": "rec-2026-04-21T12-34-56Z-a1b2c3",
  "started_at": "2026-04-21T12:34:56Z",
  "ended_at": "2026-04-21T12:36:40Z",
  "verdict": "shadow-awaiting",
  "upstream_commit_before": "abc123",
  "upstream_commit_after": "def456",
  "provider": "openai-compatible",
  "model": "claude-sonnet-4",
  "conflicted_files": ["src/foo.ts", "src/bar.go"],
  "resolved_files": [
    { "path": "src/foo.ts", "validation": "passed", "bytes": 4821, "tokens_in": 3200, "tokens_out": 1100 }
  ],
  "failed_files": [
    { "path": "src/bar.go", "reason": "conflict-markers-present", "validation_detail": "line 47: <<<<<<<" }
  ],
  "skipped_files": [
    { "path": "src/huge.json", "reason": "too-large", "bytes": 987654 }
  ],
  "test_command_result": { "ran": true, "passed": true, "duration_ms": 12400 },
  "token_cost_total": { "in": 12800, "out": 4400 },
  "shadow_path": ".tpatch/shadow/my-feature-2026-04-21T12-34-56Z/"
}
```

### `resolution-report.md` (in the shadow root, pointed to by `status`)

Human-readable summary of the session. Auto-generated. Survives until accept/reject.

### New state: `reconciling-shadow`

- Entered on successful phase 3.5 run with ≥1 resolved file.
- `tpatch status <slug>` surfaces it with the shadow path and report pointer.
- Exit: `--accept` → `reconciled`; `--reject` → previous stable state.
- `reconciling-shadow` is a terminal transient state: no other tpatch command advances from it. The agent/user must choose.

### New reconcile verdicts

| Verdict | When | Phase label |
|---|---|---|
| `ReconcileShadowAwaiting` | Phase 3.5 ran, ≥1 file resolved, validation passed, `--apply` not set | `phase-3.5-shadow` |
| `ReconcileBlockedTooManyConflicts` | Phase 3.5 not run because conflicts > `max_conflicts` | `phase-3.5-cap` |
| `ReconcileBlockedRequiresHuman` | Phase 3.5 ran, validation failed on ≥1 file | `phase-3.5-validation` |

Existing verdicts (`Reapplied`, `3WayConflicts`, `Obsolete`, `Blocked`) are unchanged.

## Flag semantics (ADR-010 D8)

```
tpatch reconcile <slug>                             # today's behaviour — stops at 3WayConflicts
tpatch reconcile <slug> --resolve                   # run phase 3.5; stops at ShadowAwaiting
tpatch reconcile <slug> --resolve --apply           # resolve + auto-accept IFF validation + test_command pass
tpatch reconcile --accept <slug>                    # commit shadow → real tree (triggers D5 refresh)
tpatch reconcile --reject <slug>                    # discard shadow
tpatch reconcile --shadow-diff <slug>               # read-only review
tpatch reconcile <slug> --resolve --max-conflicts N # override default 10
tpatch reconcile <slug> --resolve --model <name>    # override provider config for this call
```

Refused combinations (error at parse time):

- `--accept` with `--reject`, `--resolve`, `--apply`, `--shadow-diff`.
- `--reject` with `--shadow-diff`, `--resolve`, `--apply`.
- `--apply` without `--resolve`.
- `--apply` when `test_command` is unset in config.
- `--max-conflicts` or `--model` without `--resolve`.

## Config additions (`config.yaml`)

```yaml
resolver:
  max_conflicts: 10
  max_file_bytes: 204800
  syntax_check_cmd: ""       # optional — "{file}" placeholder; run per-file for non-Go extensions
  identifier_check: true     # regex-based exported identifier preservation (ADR-010 D4)
```

All keys optional; defaults above. Flat scalar map — respects the existing zero-dep YAML parser pattern.

## Acceptance criteria

- [ ] `go build ./...`, `go test ./...`, `gofmt -l .` all clean.
- [ ] Reconcile without `--resolve` still stops at `3WayConflicts` (v0.4.4 parity preserved).
- [ ] With `--resolve`: shadow worktree created, per-file provider calls made, `resolution-report.md` + `resolution-session.json` emitted.
- [ ] Validation gate: Go syntax, conflict markers, identifier preservation, optional `test_command`.
- [ ] `--accept` triggers atomic `RefreshDerivedArtifacts`; INTENT artifacts untouched.
- [ ] `--reject` prunes shadow, restores previous state.
- [ ] `tpatch status` surfaces `reconciling-shadow` with shadow path.
- [ ] Golden scenario harness ≥5 scenarios passing.
- [ ] All 6 skills document phase 3.5; parity test green.
- [ ] CHANGELOG v0.5.0 section; version bumped; tagged and pushed.

## Risks & mitigations

| Risk | Mitigation |
|---|---|
| Large files blow context | `max_file_bytes` cap; skipped-too-large verdict |
| Provider hallucinates conflict markers | Validation gate rejects; `blocked-requires-human` |
| Shadow worktree pollution | Reap-prior-shadow-on-new-resolve; explicit `--reject` prune |
| Partial resolution confuses user | Single atomic accept; no partial-accept in v0.5.0 |
| Spec drift after accept | D6 warning; logged for `feat-reconcile-metadata-refresh` |
| Provider rate limits | Sequential calls; `--model` override for lower-tier |

## Out-of-scope for v0.5.0 (logged as follow-ups)

- `feat-resolver-parallel` — concurrent per-file calls.
- `feat-resolver-chunked-context` — hunk-based resolution for large files.
- `feat-resolver-heuristic-fallback` — opt-in `--heuristic` path for provider-unavailable cases.
- `feat-reconcile-metadata-refresh` — auto-propose spec/analysis updates on accept.

## Implementation task list

See `docs/milestones/M12-provider-conflict-resolver.md`.
