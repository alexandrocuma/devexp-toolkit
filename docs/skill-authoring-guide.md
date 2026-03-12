# Skill Authoring Guide

This guide covers everything you need to write effective skills for the devexp framework. Skills are different from agents in important ways — understand the distinction before writing your first one.

---

## What Is a Skill?

A skill is a Markdown file at `~/.claude/skills/<name>/skill.md`. When a user invokes `/skill-name` in Claude Code or opencode, the skill's content is injected into the conversation context. This shapes how Claude behaves for the duration of that task.

Key differences from agents:

| | Agents | Skills |
|-|--------|--------|
| Invocation | Spawned by Claude via `Agent` tool | Invoked by user via `/skill-name` slash command |
| Isolation | Runs in a sub-agent context | Injected into the current conversation |
| Tool access | Full tool access (as configured) | No independent tool access — uses the current conversation's tools |
| Persistence | Has its own memory and state | Stateless — context injection only |
| Best for | Autonomous multi-step tasks | Shaping Claude's approach to a specific type of task |

Think of skills as **behavioral overlays** — they add focused expertise and structure to Claude's current context. The difference between Claude reviewing code with and without a skill active is the difference between general advice and a structured, expert review with defined severity levels and a specific output format.

---

## File Format

```markdown
---
name: my-skill
description: One-line description of what this skill does
---

# Skill Title

Body content — instructions, process, output format...
```

The file lives at `skills/my-skill/skill.md`. The directory name must match the `name` field exactly.

---

## Frontmatter Reference

### `name` (required)

The skill name, which becomes the slash command: `/name`.

- Must match the containing directory name exactly
- Lowercase kebab-case: `bugfix`, `api-design`, `logic-review`
- No namespace prefix — use short, descriptive names directly

### `description` (required)

A single-line description shown in skill listings. This is also what Claude reads when deciding which skill to load for a task.

Write it as: **[What it does] for [what] — producing [output]**

Examples:
- "Root cause analysis and automated bug fixing with safety checks"
- "Designs API contracts, endpoints, schemas, and error handling"
- "Generates a daily standup update from recent git activity"

Avoid vague descriptions like "helps with bugs" or "for code review".

---

## Writing the Skill Body

### Start with a Role Statement

The first thing in the body should be a clear persona statement. This is the "voice" Claude adopts when the skill is active.

```markdown
You are the **Bug Fixer**, specialized in identifying, analyzing, and fixing software bugs.
```

The bold name is a convention across the devexp skills — it creates a clear identity that Claude can inhabit.

### Include "Triggered by"

After the role statement, include a "Triggered by" section listing which agents or other skills invoke this skill. This creates a readable dependency map in the codebase and helps future authors understand how skills are connected.

```markdown
## Triggered by

- `dev-agent` — to fix bugs encountered during autonomous implementation
- `feature` skill — to verify the bug was fixed before marking complete
```

If the skill is only user-invoked and no agent uses it, say so:

```markdown
## Triggered by

User-invoked only.
```

### Define When to Use

After "Triggered by", include a "When to Use" section. Even if the skill is primarily user-invoked, this section:
- Helps Claude route correctly when used as an orchestrated sub-skill
- Helps users understand when to reach for this skill
- Acts as a contract: if the situation matches these triggers, this skill applies

```markdown
## When to Use

When the Orchestrator needs to fix a bug, or when the user says:
- "Fix the bug in..."
- "There's an error in..."
- "This is crashing when..."
```

### Write a Concrete Process

This is the most important section. A good process is:
- **Numbered** — steps are in order, not a bag of advice
- **Named** — each phase has a name (Discovery, Analysis, Implementation)
- **Actionable** — each step is something Claude can execute, not vague guidance
- **Complete** — covers the full workflow from start to finish

**Weak process:**
```markdown
## Process
1. Read the code
2. Find the bug
3. Fix it
```

**Strong process:**
```markdown
## Process

### 1. Understanding
- Parse the bug report (expected vs actual behavior)
- Read the relevant source files and stack traces
- Identify the entry point of the failing behavior

### 2. Reproduction
- Create or find a test case that triggers the bug
- Confirm the bug exists and document exact reproduction steps
- Note: do not proceed to fix if you cannot reproduce

### 3. Root Cause Analysis
- Trace the execution path from entry point to failure
- Identify the exact line or logic responsible and why it's wrong
- Check for the same pattern elsewhere in the codebase

### 4. Fix Implementation
- Design the minimal fix (change as little as possible)
- Implement following the project's existing code style
- Add a regression test that would have caught this bug

### 5. Verification
- Run the regression test to confirm it passes
- Run the full test suite for the affected package
- Check for side effects on callers of the changed code
```

### Include Safety Rules or Guidelines

For skills that modify files or execute commands, add explicit safety rules:

```markdown
## Safety Rules

1. Always reproduce the bug before implementing a fix
2. Make the minimal possible change — don't refactor while fixing
3. Never modify code outside the scope of the reported bug
4. Add a regression test before marking the fix done
5. Run the full test suite, not just the new test
```

### Specify Output Format

Always describe what the skill should produce. Specific output formats are more useful and more consistent.

**Weak:**
```markdown
## Output
Summarize the results.
```

**Strong:**
```markdown
## Output

Provide a clear fix summary covering:
- **Bug**: What was the bug? (one sentence)
- **Location**: Where was it? (file and line)
- **Root cause**: The exact cause, not just the symptom
- **Fix applied**: Description of the change
- **Verification**: Tests run and their results
```

If the output is a structured report, name the sections. If it's a file to create, describe the file.

---

## Skill Archetypes

### User-Invoked Skill

The user types `/my-skill` and describes their task. The skill shapes how Claude approaches it.

Best for: tasks where a user wants a specific type of analysis or output format.

```markdown
## When to Use
Invoked by the user directly when they need [specific type of help].

## Process
[Steps that work starting from whatever the user provides]
```

### Orchestrated Sub-Skill

Invoked by an agent or another skill. The parent passes context; this skill executes a specific phase.

Best for: specialized phases in a multi-step pipeline — architecture review after analysis, fix verification after implementation.

```markdown
## Triggered by
- `dev-agent` — to [specific purpose]

## When to Use
Invoked by [parent] when [condition].
```

### Orchestrator Skill

Routes to other skills or agents based on the user's request.

Best for: entry points that delegate to a family of sub-skills.

```markdown
## Your Purpose
You are the entry point for [domain] operations. You do NOT perform work directly —
you coordinate specialized capabilities.

### Available Capabilities
| User Request | Delegate To |
|-------------|-------------|
| "analyze..." | [skill or agent name] |
| "fix..." | [skill or agent name] |
```

---

## Patterns That Work

### The Numbered Phase Pattern

Divide the process into named phases. Each phase gets a header, and each step within a phase is a bullet point. This is the most reliable structure for consistent, repeatable behavior across different invocations.

### The Verification Checklist Pattern

For quality-focused skills (bug fix verifier, regression tester), use an explicit checklist:

```markdown
## Verification Checklist

- [ ] Original bug is fixed
- [ ] Edge cases pass
- [ ] No unintended side effects
- [ ] All tests pass
- [ ] Performance is acceptable
```

Checklists create accountability. Claude works through each item rather than guessing when it's done.

### The Severity Ladder Pattern

For review and audit skills, define severity levels explicitly:

```markdown
## Issue Severity

- **Critical**: Correctness bugs, security vulnerabilities, data loss risks — must fix
- **High**: Significant performance issues, error handling gaps — should fix
- **Medium**: Code quality issues, maintainability concerns — worth addressing
- **Low**: Style preferences, minor naming issues — optional
```

Unspecified severity means everything gets treated the same. Explicit severity makes output actionable.

### The Output Template Pattern

For skills that produce structured reports, provide a template:

```markdown
## Output Template

Provide your analysis in this format:

### Summary
[2-3 sentence overview]

### Findings by Severity

**Critical**
- [Issue]: [Explanation] — [Fix recommendation]

**High**
- [Issue]: [Explanation] — [Fix recommendation]

### Recommendations
[Prioritized list of next steps]
```

---

## Common Mistakes

**No process definition.** A skill that says "review the code for security issues" without defining a process will produce inconsistent output. Define the steps.

**Vague output specification.** If you don't say what to produce, you'll get different things each time. Name the sections, describe what level of detail is expected.

**Trying to do everything.** A skill named "code quality" that covers style, performance, security, and architecture will do all of them poorly. Split into focused skills.

**Missing "Triggered by".** Without this, there's no record of which agents use the skill. Future authors can't tell whether it's safe to rename or change the interface.

**Missing "When to Use".** Without this, the orchestrator can't route correctly, and users don't know when to reach for the skill.

**Process steps that aren't steps.** "Understand the codebase" is not a step. "Read the three most-imported files to understand the architecture" is a step.

**Confusing skills with agents.** Skills are context injection — they shape behavior but don't run autonomously. If your skill needs to read files, run tests, and make changes independently, it should be an agent.

---

## Naming Conventions

Skill names in the devexp suite use short, descriptive kebab-case names without a namespace prefix:

```
<domain>           (no namespace prefix)
```

Examples:
- `bugfix` — root cause analysis and bug fixing
- `logic-review` — code logic review for bugs and edge cases
- `api-design` — API contract design
- `db-design` — database schema design
- `test-gen` — test generation

Prefer merging closely related concerns into a single skill with sub-sections rather than creating many fine-grained skills. For example, `docs` covers API docs, code comments, usage examples, and README — not four separate skills.

---

## Testing Your Skill

1. Install with `./install.sh`
2. Start a new conversation in Claude Code or opencode
3. Type `/skill-name`
4. Describe a task or paste code
5. Evaluate: does Claude follow the process you defined? Does the output match the format you specified?
6. Iterate: add specificity where the output diverges from what you wanted

The fastest feedback loop is: write skill → install → test with a real example → refine → repeat.
