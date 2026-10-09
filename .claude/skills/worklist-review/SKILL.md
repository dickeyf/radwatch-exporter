---
name: worklist-review
description: Audit everything in docs/ against itself and against the code. Checks that worklist statuses, decisions, ADRs, config keys, metric names, ports, paths and layouts are consistent across documents and match the repo. Reports findings and fixes them with the user's agreement. Use when the user asks to review, sync or check the docs.
---

# Review docs/ for consistency and accuracy

## 1. Gather
- Read every file under `docs/` in full.
- Survey the repo: `go.mod` (module, Go version), `cmd/`, `internal/`, `Makefile`, `.golangci.yml`, `.github/workflows/`, `Dockerfile`, `.dockerignore`, `deploy/helm/`, tests, and `git log --oneline -20`.
- In the code, grep for config keys (`GMC_*`), metric names (`gmc_*`), ports, the ingest path and endpoint paths.

## 2. Check
**Docs vs code**
- Each `DONE` item is really done in the repo; each `TODO` item doesn't already exist in code; `DOING` items show work in progress.
- Config keys, defaults, metric names/types/labels, ports, paths, file layout, Go version and image base match between docs and code.
- Makefile targets, CI steps and the Dockerfile match what `02-roadmap.md`, `04-container.md` and `05-helm-chart.md` describe.

**Docs vs docs**
- Every ID referenced (`IMPL-`, `AD-`, `PD-`, `Q-`, `ADR-`) exists; "Depends on" columns point to real IDs; no duplicate IDs.
- Decided items: an `AD-*` marked `DONE` has its ADR or recorded outcome; an ADR marked "Accepted" has its `AD-*` closed; "Proposed" ADRs match `TODO` AD items.
- Answered `Q-*` items are reflected in the affected decisions.
- Phase numbering and the milestone table agree between `02-roadmap.md` and the worklist.
- Relative links resolve; the plan `README.md` index lists every document.
- Stale wording ("will", "planned", "sketch") where the thing now exists.

## 3. Report and fix
Present findings grouped as **Wrong** (contradicts code or another doc), **Stale** (outdated status or wording), **Missing** (undocumented code or decision), **Nits**, each with file:line and the proposed fix.
Ask before applying. Statuses and factual mismatches can be fixed directly once the user agrees. Where the code and the docs disagree on intent, ask which one is right; never silently rewrite a decision. Don't commit unless asked.
