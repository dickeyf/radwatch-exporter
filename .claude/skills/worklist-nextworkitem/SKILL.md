---
name: worklist-nextworkitem
description: Suggest what to work on next. Reads docs/plan/07-worklist.md and the roadmap, works out which IMPL, AD and PD items are unblocked, and lists the best candidates for making progress. Use when the user asks "what's next", "what should I work on", or wants to pick a task.
---

# Pick the next work item

## 1. Load state
- Read `docs/plan/07-worklist.md` and `docs/plan/02-roadmap.md`; skim the others for context on the candidates.
- Quickly check the repo (`git log --oneline -10`, `git status`, file layout) to spot items whose status is stale, e.g. code exists but the item says `TODO`. Report those; don't fix them here (that's `/worklist-review`).

## 2. Work out what's actionable
For every item not `DONE`/`DROPPED`:
- **Unblocked:** all "Depends on" IDs are `DONE`, or a dependency is a decision that can reasonably be stubbed (say so).
- **Blocking value:** how many other items wait on it (directly or transitively). Decisions (`AD-*`, `PD-*`) that block current-phase IMPL items rank high.
- **Phase:** prefer the earliest unfinished phase; the roadmap exit criteria define "finished".
- `DOING` items come first: finishing beats starting.

## 3. Present the options
Give a short ranked list (3–6 items), mixing types where relevant:

| # | ID | Type | Why now | Unblocks | Size (S/M/L) |

Then:
- Name your recommendation and why, in one or two sentences.
- For a decision item, list the options from the worklist and what you'd need from the user.
- Mention the current phase and what remains for its exit criterion.

Ask the user which one to take (AskUserQuestion works well). Once they pick, mark it `DOING` in the worklist and start: for an AD/PD item, discuss it and record the outcome (ADR in `03-architecture.md` or a note in the worklist) before marking it `DONE`; for an IMPL item, implement it and update docs in the same change.
