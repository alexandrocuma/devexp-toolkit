---
name: postmortem
description: Generate a structured blameless postmortem document for an incident
---

# Postmortem Generator

You are generating a **blameless postmortem** — a structured document that captures what happened during an incident, why it happened, and what will be done to prevent recurrence. The tone is analytical, not accusatory.

## Triggered by

- `postmortem` agent — for full incident postmortem workflows
- `/postmortem` — direct generation from the current conversation context

## Core Principle: Blameless

Every sentence must be blameless. Systems fail. Processes have gaps. Code has bugs. Engineers are human. Never write "X forgot to" or "X failed to" — write "the system lacked a check for" or "the deployment process did not include a step to."

## Process

### 1. Gather incident information

Collect from the user's description and from git/code context:

**From user (required):**
- What failed
- When it started and when it was resolved
- How it was detected (alert / user report / monitoring)
- What the resolution was

**Derive from git if not provided:**
```bash
git log --since="48 hours ago" --oneline  # recent changes
git log --oneline --all | head -20         # recent activity
```

**Estimate if not provided:**
- Severity: SEV1 (total outage) / SEV2 (partial/degraded) / SEV3 (minor impact)
- Users affected: "unknown" if not provided

### 2. Determine output location

```bash
ls docs/postmortems/ 2>/dev/null || ls postmortems/ 2>/dev/null
```

Filename: `POSTMORTEM-<YYYY-MM-DD>-<kebab-slug>.md`

Location priority: `docs/postmortems/` → `postmortems/` → project root

### 3. Write the postmortem

```markdown
# Postmortem: <Title>

**Date**: YYYY-MM-DD
**Severity**: SEV1 / SEV2 / SEV3
**Duration**: Xh Ym (HH:MM to HH:MM UTC)
**Status**: Resolved

---

## Executive Summary

[1-2 paragraphs. Cover: what happened, how it was detected, what the impact was, what fixed it. Written so a non-technical stakeholder can understand the incident in under 2 minutes. Blameless throughout.]

---

## Timeline

All times UTC.

| Time | Event |
|------|-------|
| HH:MM | First signal (alert / user report) |
| HH:MM | Incident declared |
| HH:MM | Investigation began |
| HH:MM | Root cause identified |
| HH:MM | Mitigation applied |
| HH:MM | Service restored |
| HH:MM | Incident closed |

---

## Impact

- **Users affected**: [number, percentage, or "unknown"]
- **Duration**: [HH:MM — first impact to full resolution]
- **Severity**: [SEV level + brief justification]
- **Business impact**: [revenue loss, SLA breach, data loss, reputational impact]
- **Features affected**: [list of degraded or unavailable features]

---

## Root Cause

[Technically precise. Name the specific condition, code path, configuration value, or interaction that caused the failure. Reference file names and line numbers if known. A reader not involved in the incident must be able to understand the exact failure mechanism from this section alone.]

---

## Contributing Factors

Conditions that allowed the root cause to have impact:

- **[Factor name]**: [How this condition enabled the root cause to cause harm. Example: "No automated rollback: the deploy pipeline lacked a post-deploy health check, so the bad deploy remained in production for 47 minutes instead of rolling back automatically within 2 minutes."]
- **[Factor name]**: [Explanation]

---

## Resolution

What was done to restore service:

1. [First action taken — include what was tried even if it didn't work]
2. [Second action]
3. [Final resolution step — what actually fixed it]

---

## Action Items

| Action | Owner | Due Date | Priority |
|--------|-------|----------|----------|
| [Specific, measurable improvement] | [Team or person] | YYYY-MM-DD | P1/P2/P3 |
| Add alert for [specific condition] with [threshold] | On-call team | YYYY-MM-DD | P1 |
| Add [specific test] to prevent regression | [team] | YYYY-MM-DD | P2 |

Action item categories:
- **Detection**: improve alerting or monitoring so this is caught sooner
- **Prevention**: remove the condition that enabled the failure
- **Mitigation**: reduce blast radius if this class of failure recurs
- **Process**: improve how deploys, oncall, or incidents are handled

---

## Lessons Learned

### What went well
- [Something that worked during the incident response]

### What could have gone better
- [Something that slowed detection or resolution]

### Where we got lucky
- [Acknowledge near-misses — these often reveal the most important prevention work]

---

## Prevention

Specific monitoring, tests, or process changes that would have caught this before it became an incident:

- [Specific check or alert that would have prevented or shortened this]
- [Another prevention measure]
```

### 4. Report

```
Postmortem written: <file path>

Severity: SEV<N>
Duration: Xh Ym
Action items: N

Next steps:
- Review with the team for timeline accuracy
- Assign owners to action items
- Run /ticket or project-manager to create action item GitHub Issues
- Share with stakeholders
```

## Quality Checklist

Before finishing:
- [ ] No blame language anywhere in the document
- [ ] Timeline is chronological and in a consistent timezone
- [ ] Root cause is technically specific (not "there was a bug")
- [ ] Every action item has an owner, due date, and priority
- [ ] "Where we got lucky" section is filled in honestly
- [ ] Executive summary is understandable to a non-technical reader
