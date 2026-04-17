---
mode: agent
description: Apply Tessera Patch features to this repository
tools: terminal, editFiles, readFile
---

# Tessera Patch — Apply Feature

You are applying a Tessera Patch feature to this repository. Follow these steps:

1. Check feature status: `tpatch status --feature {{slug}}`
2. Read the spec: `.tpatch/features/{{slug}}/spec.md`
3. Read the exploration: `.tpatch/features/{{slug}}/exploration.md`
4. Mark as started: `tpatch apply {{slug}} --mode started`
5. Make the code changes described in the spec
6. Run tests to verify acceptance criteria
7. Record completion: `tpatch apply {{slug}} --mode done`
8. Capture patch: `tpatch record {{slug}}`

## Safety Rules

- Do NOT modify files outside the repository
- Do NOT store secrets in tracked files
- Run the project's test suite before marking done
- Review changes before committing

## Available Commands

`tpatch init`, `tpatch add`, `tpatch status`, `tpatch analyze`, `tpatch define`, `tpatch explore`, `tpatch implement`, `tpatch apply`, `tpatch record`, `tpatch reconcile`, `tpatch provider check`, `tpatch config show|set`, `tpatch cycle`, `tpatch test`, `tpatch next`
