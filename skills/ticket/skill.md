---
name: ticket
description: Create a well-structured GitHub Issue for a bug, feature, or tech-debt item
---

# Ticket Creator

You are creating a **GitHub Issue** — a well-structured ticket that a developer can pick up and execute without needing to ask clarifying questions.

## Triggered by

- `project-manager` agent — for individual ticket creation
- `/ticket` — direct invocation

## When to Use

When the user needs to create a well-structured GitHub Issue for a bug, feature, or tech-debt item. Phrases: "create a ticket", "file a bug", "open a GitHub issue", "write a ticket for X".

## Process

### 1. Determine the ticket type

From the user's description, classify as:
- **bug** — something is broken or behaving incorrectly
- **feature** — new capability that doesn't exist yet
- **enhancement** — improvement to an existing feature
- **tech-debt** — internal quality work with no direct user-visible change
- **security** — security vulnerability or hardening work
- **documentation** — docs-only change

### 2. Check repository context

```bash
gh repo view --json name,url 2>/dev/null
gh label list 2>/dev/null
gh milestone list 2>/dev/null
```

Use only existing labels. If the needed label doesn't exist, note it and use the closest match.

### 3. Write the ticket body

**For a bug:**

```markdown
## Description

[Clear description of the broken behavior and its user impact.]

## Steps to Reproduce

1. [Step one]
2. [Step two]
3. [Step three — this is where the bug occurs]

## Expected Behavior

[What should happen]

## Actual Behavior

[What actually happens — include error messages verbatim if available]

## Environment

- Version/branch: [if known]
- OS/Browser: [if relevant]
- Additional context: [logs, screenshots, frequency]

## Acceptance Criteria

- [ ] The described behavior no longer occurs
- [ ] Regression test added to prevent recurrence
- [ ] Root cause documented in PR description
- [ ] No regressions in related functionality
```

Labels: `bug`
Priority label: `P1` (data loss/security/total outage), `P2` (significant impact), `P3` (minor/cosmetic)

**For a feature:**

```markdown
## User Story

As a [role/persona], I want [capability], so that [benefit/outcome].

## Description

[Context and motivation. Why does this feature matter? What problem does it solve?]

## Acceptance Criteria

- [ ] [Specific, testable criterion — what "done" looks like]
- [ ] [Another criterion]
- [ ] [Another criterion]
- [ ] Tests cover happy path, error cases, and edge cases
- [ ] Documentation updated (if user-facing)

## Out of Scope

- [What is explicitly NOT part of this ticket — prevents scope creep]
- [Another out-of-scope item]

## Design / Notes

[Links to design docs, mockups, or ADRs if available. Key implementation considerations.]
```

Labels: `feature`

**For tech-debt:**

```markdown
## Current State

[What exists today and why it's a problem. Be specific: file names, function names, the actual technical issue.]

## Desired State

[What it should look like after this work is done. What improves?]

## Risk of Not Doing

[What breaks, degrades, or becomes harder if this is left as-is. Quantify if possible: "every new service requires 30 minutes of manual wiring because..."]

## Acceptance Criteria

- [ ] [Measurable improvement — "service registration is automated" not "code is cleaner"]
- [ ] All existing tests still pass
- [ ] No behavior change from the user's perspective
- [ ] Updated docs/comments if public APIs changed

## Effort Estimate

[S (< 1 day) / M (1-3 days) / L (3-5 days) / XL (needs breakdown)]
```

Labels: `tech-debt`

**For security:**

```markdown
## Security Issue

**Severity**: Critical / High / Medium / Low
**Type**: [injection / auth bypass / data exposure / dependency vulnerability / misconfiguration]

## Description

[Description of the vulnerability. For public repos: describe the class of issue without disclosing the exact exploit path.]

## Impact

[What an attacker could do if this is exploited]

## Acceptance Criteria

- [ ] Vulnerability is remediated
- [ ] Security test added (or existing test updated) to prevent regression
- [ ] Related code paths audited for the same class of issue
- [ ] If a CVE exists, it is referenced in the PR

## References

[CVE number, security advisory, OWASP link if applicable]
```

Labels: `security`

### 4. Create the issue

```bash
gh issue create \
  --title "<title>" \
  --body "<body>" \
  --label "<label>" \
  [--milestone "<milestone>"] \
  [--assignee "<assignee>"]
```

### 5. Report

```
Issue created: #<number> — <title>
URL: <github-url>
Labels: <labels applied>
```

## Good Title Patterns

- Bug: `[Bug] <what is broken> when <condition>`
- Feature: `[Feature] <imperative description of new capability>`
- Tech-debt: `[Tech Debt] <what needs to change>`
- Security: `[Security] <class of issue> in <component>`

## What Makes a Bad Ticket

- Vague title: "Fix auth" → "Fix session token not invalidated on logout"
- No acceptance criteria — developer can't know when done
- No reproduction steps for bugs — impossible to verify the fix
- Missing out-of-scope section for features — scope creep happens
