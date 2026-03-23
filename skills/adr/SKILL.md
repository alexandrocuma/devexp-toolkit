---
name: adr
description: Write an Architecture Decision Record documenting a technical decision
---

# Architecture Decision Record Writer

You are writing a formal **Architecture Decision Record (ADR)** — a document that captures the context, decision, consequences, and alternatives for a significant technical choice.

## Triggered by

- `tech-lead` agent — for documenting architectural decisions
- `/adr <decision title>` — direct invocation

## When to Use

When a significant technical decision needs to be documented for future reference. Phrases: "write an ADR", "document this architecture decision", "we decided to use X", "capture this decision".

## Process

### 1. Determine the ADR number and location

Find existing ADRs to determine the next number:
```bash
find . -path "*/adr/*.md" -o -path "*/decisions/*.md" 2>/dev/null | sort | tail -1
```

Output location priority:
1. `docs/adr/` (if it exists or if `docs/` exists)
2. `docs/decisions/` (if that's the pattern)
3. `adr/` (if that directory exists)
4. Create `docs/adr/` if none exist

Filename: `docs/adr/<NNNN>-<kebab-case-title>.md`

Auto-increment: find the highest NNNN, add 1. Start at `0001` if none exist.

### 2. Gather context

If the user hasn't provided all the context, gather it:
- Read the relevant code or configuration that motivates this decision
- Check git history for related changes: `git log --oneline --since="30 days ago"`
- Read any existing ADRs for context on prior decisions in the same domain

### 3. Write the ADR

Use this exact format:

```markdown
# <NNNN>. <Title in present tense imperative — "Use Redis for session storage">

**Date**: YYYY-MM-DD
**Status**: Proposed | Accepted | Deprecated | Superseded by [NNNN](./NNNN-title.md)
**Deciders**: [Who was involved in making this decision]

---

## Context

[What situation, constraint, or requirement makes this decision necessary? Be specific: include scale, team size, performance requirements, existing dependencies, or any other force that shapes the solution space. A reader 2 years from now must understand WHY a decision was needed.]

## Decision

[State the decision clearly in one sentence. Then explain the reasoning — the specific factors in the Context that made this the right choice. Don't just state what; explain why this option beats the alternatives for this specific situation.]

## Consequences

### Positive
- [Specific benefit that results from this decision]
- [Another benefit]

### Negative
- [Specific cost, limitation, or risk introduced]
- [Another cost]

### Neutral
- [Significant change that is neither clearly positive nor negative — migrations, learning curve, etc.]

## Alternatives Considered

### Alternative: <name>
**Description**: [What this option is and how it would work]
**Why not chosen**: [Specific reason this was rejected given the Context above]

### Alternative: <name>
**Description**: [What this option is]
**Why not chosen**: [Specific reason]

## Implementation Notes

[Optional: Links to follow-up work, migration guides, or related ADRs. Specific gotchas for implementing this decision correctly.]
```

### 4. Update the ADR index

After writing the ADR file, update `docs/architecture/adr/README.md` (create it if missing):

```markdown
# Architecture Decision Records

Decisions that shaped how this system is built. Read before implementing anything significant.

## Decisions

| ADR | Title | Status | Impact |
|-----|-------|--------|--------|
| [NNNN](NNNN-<title>.md) | <Title> | Accepted | One line: what this means for how you write code today |
```

Add a row for the new ADR. If superseding an older one, update that row's status to `Superseded by [NNNN](NNNN-title.md)`.

### 5. Save and report

Write the file. Then report:
- Full path of the ADR created
- ADR number and title
- Status (Proposed or Accepted)
- Suggest: "If this decision is already made, update Status to Accepted"

## Quality Checklist

Before finishing, verify:
- [ ] Context explains WHY a decision was needed, not just WHAT was decided
- [ ] Decision section gives specific reasoning, not just "it's better"
- [ ] At least 2 alternatives considered with specific rejection reasons
- [ ] Consequences include both positives and negatives
- [ ] Status is set appropriately (Proposed if still being discussed, Accepted if decided)
- [ ] Date is today's date

## Output Example

```
ADR written: docs/adr/0004-use-redis-for-session-storage.md

Status: Proposed
Review with your team, then update Status to "Accepted" when the decision is finalized.
```
