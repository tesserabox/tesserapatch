# Tessera Patch — Claude Code Skill

## What This Is

Tessera Patch is a framework for customizing open-source projects through natural-language-driven patching. This skill teaches you the methodology.

## The 7-Phase Lifecycle

```
analyse → define → explore → implement → test → record → reconcile
```

## CLI Commands

| Command | Purpose |
|---------|---------|
| `tpatch init` | Initialize `.tpatch/` workspace and install skill formats |
| `tpatch add <description>` | Create a tracked feature request |
| `tpatch status` | Show feature status dashboard |
| `tpatch analyze <slug>` | Run analysis phase on a feature |
| `tpatch define <slug>` | Generate acceptance criteria and plan |
| `tpatch explore <slug>` | Read codebase, find minimal changeset |
| `tpatch implement <slug>` | Generate deterministic apply recipe |
| `tpatch apply <slug>` | Execute apply recipe or record session |
| `tpatch record <slug>` | Capture patches (tracked + untracked files) |
| `tpatch reconcile [slug...]` | Reconcile features against upstream |
| `tpatch provider check` | Validate LLM provider endpoint |
| `tpatch config show\|set` | Manage configuration |
| `tpatch cycle <slug>` | Run the full lifecycle (analyze→define→explore→implement→apply→record) in sequence. Add `--interactive` to pause between phases. |
| `tpatch test <slug>` | Run the configured `test_command` and record the pass/fail outcome |
| `tpatch next <slug>` | Emit the next logical action for a feature. `--format harness-json` outputs structured JSON for consumption by coding-agent harnesses |

## .tpatch/ Structure

```
.tpatch/
├── config.yaml          # Provider settings (secret-by-reference)
├── FEATURES.md          # Master feature index
├── upstream.lock        # Upstream commit tracking
├── steering/            # Local + upstream patching guidance
└── features/<slug>/     # Per-feature artifacts
    ├── status.json      # Machine-readable state
    ├── request.md       # Natural-language request
    ├── analysis.md      # Compatibility and impact analysis
    ├── spec.md          # Acceptance criteria + plan
    ├── exploration.md   # Codebase exploration log
    ├── record.md        # Implementation summary
    ├── reconciliation/  # Per-version reconciliation logs
    └── artifacts/       # Machine-generated data
```

## Feature States

```
requested → analyzed → defined → implementing → applied → active
                                                     ↓
                                               reconciling → active / upstream_merged / blocked
```

## Workflow Steps

### 1. Initialize
```bash
tpatch init --path /path/to/fork
```

### 2. Add Feature
```bash
tpatch add "Fix model ID translation bug" --path /path/to/fork
```

### 3. Analyze
```bash
tpatch analyze <slug>
```
Produces `analysis.md` + `artifacts/analysis.json`. Works in heuristic mode without a provider.

### 4. Define
```bash
tpatch define <slug>
```
Produces `spec.md` with acceptance criteria and implementation plan.

### 5. Explore
```bash
tpatch explore <slug>
```
Reads codebase, identifies relevant files, produces `exploration.md`.

### 6. Implement
Use the analysis, spec, and exploration to make the code changes. Then record:
```bash
tpatch apply <slug> --mode started
# ... make changes ...
tpatch apply <slug> --mode done
tpatch record <slug>
```

### 7. Reconcile
When upstream updates:
```bash
tpatch reconcile --upstream-ref upstream/main
```

**4-Phase Decision Tree**:
1. Reverse-apply check → UPSTREAMED (fast, free)
2. Operation-level evaluation → partial detection
3. Provider-assisted semantic check → structural differences
4. Forward-apply attempt → REAPPLIED or BLOCKED

## Safety

- Path traversal protection on all file writes
- Secret-by-reference: config stores env var name, not the token
- Patches exclude `.tpatch/`, skill directories, and framework files
- Deterministic apply recipes can be reviewed before execution

## Editable Sections

<!-- Add project-specific instructions below -->

### Project-Specific Notes

*(Add notes about the upstream project's build system, test commands, and patching quirks here)*

### Custom Acceptance Criteria

*(Add standard acceptance criteria that should apply to all features in this fork)*
