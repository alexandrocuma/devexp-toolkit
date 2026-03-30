---
name: scope
description: Break a large feature or epic into atomic, implementable tickets with dependencies — works with GitHub Issues, GitLab Issues, Linear, and Jira
---

# Epic Scoper

You are decomposing a **large feature or epic** into a set of atomic, implementable tickets. Each ticket should be completable in 1-3 days by one engineer, have clear acceptance criteria, and have explicit dependencies on other tickets.

## Triggered by

- `project-manager` agent — for epic decomposition
- `/scope <epic description>` — direct invocation

## When to Use

When a large feature or epic needs to be broken into atomic, sprint-ready tickets with explicit dependencies. Phrases: "scope this epic", "break this feature into tickets", "decompose this into tasks", "create tickets for X".

## Process

### 1. Understand the epic

Read any relevant context:
- User's description of the feature
- Existing code in the area (from codebase-navigator atlas or direct exploration)
- Related open issues (use the detected platform's list command, e.g. `gh issue list --label feature --limit 20` for GitHub)
- Any design docs, PRDs, or ADRs linked or mentioned

Ask for clarification only if the scope is fundamentally ambiguous (e.g., "add auth" without knowing whether OAuth, magic links, or passwords are intended).

### 2. Decompose into atomic tickets

Identify 3-10 tickets following these rules:

**Each ticket must:**
- Be completable by one engineer in 1-3 days (S or M size)
- Have a clear, testable outcome
- Touch a coherent unit of work (one layer, one module, one integration)
- Not require another in-progress ticket to be merged first (unless it's a declared dependency)

**Decomposition strategies:**
- **Vertical slices**: each ticket delivers a thin end-to-end slice of functionality (preferred)
- **Horizontal layers**: data layer → business logic → API → UI (use when the layers are truly independent)
- **Phase-based**: MVP → enhancements → polish

**Common ticket types in a feature:**
1. Data model / schema / migration
2. Core service/business logic
3. API endpoint(s)
4. Frontend UI / component
5. Integration with external service
6. Tests and test infrastructure
7. Documentation / configuration

### 3. Map dependencies

For each ticket, identify:
- Which other tickets must be **completed** before this one can start (hard dependency)
- Which other tickets are **helpful to have** but not blocking (soft dependency)

Build the dependency graph. Identify the **critical path** — the sequence of blocking dependencies that determines the minimum time to ship the feature.

### 4. Assign estimates

Size each ticket:
- **S**: < 1 day — a small, well-understood change
- **M**: 1-3 days — moderate complexity, some discovery expected
- **L**: 3-5 days — significant complexity; consider breaking down further
- **XL**: > 5 days — must be broken down, do not create an XL ticket

### 5. Present the plan before creating issues

Show the breakdown and dependency graph before creating any tickets:

```
Epic: <title>

Tickets (N total, estimated X-Y days total):

#1 [M] Set up database schema for <entity>
   Dependencies: none

#2 [M] Implement <entity> service layer
   Dependencies: #1

#3 [S] Add REST API endpoints for <entity>
   Dependencies: #2

#4 [M] Build <ComponentName> UI component
   Dependencies: #3 (or: can start after #2 with mock API)

#5 [S] Write integration tests
   Dependencies: #3

Critical path: #1 → #2 → #3 → #5 (6-9 days)
Parallelizable: #4 can start after #2

Proceed with creating these issues? (yes/no)
```

Wait for confirmation before creating issues.

### 6. Create the tickets

First, detect the available issue tracker:

| Priority | Signal | Platform |
|----------|--------|----------|
| 1 | `mcp__linear__*` tools present | Linear |
| 2 | `mcp__jira__*` or `mcp__atlassian__*` tools present | Jira |
| 3 | `gh auth status` succeeds | GitHub Issues |
| 4 | `glab auth status` succeeds | GitLab Issues |
| 5 | None | Output formatted markdown for user to file manually |

For each ticket, use the detected platform's create operation with this body structure:

```markdown
## Epic
Part of: <epic title>

## Description
<description>

## Acceptance Criteria
- [ ] <criterion>
- [ ] <criterion>
- [ ] Tests written and passing

## Dependencies
- Depends on: #<N> (if applicable)
- Blocks: #<M>, #<P> (if applicable)

## Size estimate
<S / M / L>
```

**GitHub:** `gh issue create --title "[Epic: <name>] <ticket title>" --body "<body>" --label "feature"`

**GitLab:** `glab issue create --title "[Epic: <name>] <ticket title>" --description "<body>" --label "feature"`

**Linear:** `mcp__linear__create_issue(title, description, labelIds)`

**Jira:** `mcp__jira__create_issue(summary, description, issueType)`

**None detected:** Output each ticket as formatted markdown.

After all tickets are created, update each body to add the actual ticket numbers for dependency links (now that you know them from the create output).

### 7. Report

```
Epic scoped: <title>

N tickets created:
  #101 [M] Set up database schema — <url>
  #102 [M] Implement service layer — <url>
  #103 [S] Add REST API endpoints — <url>
  #104 [M] Build UI component — <url>
  #105 [S] Write integration tests — <url>

Critical path: #101 → #102 → #103 → #105
Parallelizable: #104 can start after #102

Estimated total: X-Y days (sequential) / A-B days (with parallelism)
```

## Rules

- Tickets must be atomic — a developer should be able to pick one up without coordinating with anyone except for the dependencies declared in the ticket
- Never create an XL ticket — if something is too large, decompose it further
- Always make the dependency graph explicit — hidden dependencies cause sprint failures
- Include an out-of-scope note if the epic has known edges that are being deferred
- The critical path is the most important output after the tickets themselves
