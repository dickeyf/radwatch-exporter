---
name: worklist-planwork
description: Plan new work into the project plan. Reviews docs/plan/ (especially 07-worklist.md), takes the user's work request, agrees with them on new IMPL/AD/PD items, and updates every affected .md file so the plan stays consistent. Use when the user wants to add, change or scope new work.
argument-hint: "<description of the work to plan>"
---

# Plan new work into docs/plan/

The user's request: $ARGUMENTS (if empty, ask what they want to plan).

## 1. Load context
Read all of `docs/plan/*.md`. Note:
- Existing IDs and the highest number in each series (`IMPL-NN`, `AD-NN`, `PD-NN`, `Q-NN`, `ADR-NNN`, phases).
- Items already covering part of the request. Don't duplicate them; extend or reference them.
- Current statuses (`TODO` / `DOING` / `DONE` / `DROPPED`).
Glance at the repo layout (`go.mod`, `cmd/`, `internal/`, `deploy/`) only to ground the proposal in what exists.

## 2. Shape the work with the user
Before editing anything, propose:
- New items, each with type (IMPL/AD/PD/Q), phase, one-line description, and dependencies.
- Decisions the request raises that need a new `AD-*`/`PD-*`, or open device questions (`Q-*`).
- Which docs change and why (roadmap phase bullets, architecture data model or ADR, container, helm, security).
Ask about genuinely open points (use AskUserQuestion for 1–4 discrete choices). Iterate until the user agrees. Don't invent decisions the user hasn't made: record them as `TODO` decision items instead.

## 3. Apply the edits
- `07-worklist.md`: add rows in the right table, next free IDs, ordered by phase; status `TODO`. Never renumber existing IDs.
- `02-roadmap.md`: add bullets to the matching phase (or a new phase, plus the milestone table if relevant).
- `03-architecture.md`: update the component view and data model; add an ADR only for a decision the user actually took.
- `04`/`05`/`06`: update when the work touches the image, the chart/config keys, or security/open questions.
- `README.md` (plan index): update if a document is added or renamed.
Keep the existing style: tables, short sentences, cross-links like `[07-worklist.md](07-worklist.md)`.

## 4. Check consistency
- Every new ID referenced anywhere exists in the worklist, and every "Depends on" names a real ID.
- Config keys, ports, metric names and paths match across docs.
- Summarize to the user: items added (IDs), docs touched, decisions left open. Don't commit unless asked.
