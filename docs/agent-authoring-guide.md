# Agent Authoring Guide

This guide covers everything you need to write effective Claude Code agents for this framework. Read it before writing your first agent; refer back when something isn't working as expected.

---

## What Is an Agent?

A Claude Code agent is a Markdown file with YAML frontmatter that lives in `~/.claude/agents/`. When loaded, it creates a specialized sub-agent that the orchestrating Claude can spawn using the `Agent` tool.

Agents are autonomous: once spawned, they read code, write files, run commands, and produce output — without constant guidance from the user. The quality of your agent file directly determines the quality of that autonomous behavior.

---

## File Format

```markdown
---
name: my-agent
description: "Use this agent when..."
tools: Read, Write, Edit, Bash, Glob, Grep
model: sonnet
color: cyan
memory: user
---

# Agent system prompt body

Your instructions here...
```

The frontmatter is parsed by Claude Code. The body is the system prompt injected when the agent runs.

---

## Frontmatter Reference

### `name` (required)

The identifier used to reference the agent. Must be unique across all installed agents.

- Use lowercase kebab-case: `my-code-reviewer`, `db-schema-expert`
- Keep it short and descriptive
- Avoid generic names like `helper` or `assistant`

### `description` (required — most important field)

This is what the orchestrating Claude reads to decide whether and when to spawn your agent. It has the highest impact on whether the agent gets used correctly.

**Rules for a good description:**

1. **Lead with the trigger condition.** Start with "Use this agent when..." or "Use this agent to...". This makes routing decisions unambiguous.

2. **Name the primary capability.** One sentence summarizing what the agent does.

3. **Include 2–4 `<example>` blocks.** These are the single most effective thing you can do to improve invocation accuracy. Format:
   ```
   <example>
   Context: [The situation]
   user: "[Realistic user message]"
   assistant: "[How Claude should respond, naming this agent]"
   <commentary>
   [Why this agent is correct for this situation]
   </commentary>
   </example>
   ```

4. **Cover distinct trigger scenarios.** Each example should represent a different type of situation that warrants this agent — not small variations of the same thing.

5. **Be specific in examples.** "I wrote a Go service, can you review it?" is more useful than "I wrote some code".

**Bad description:**
```yaml
description: "Reviews code and provides feedback."
```

**Good description:**
```yaml
description: "Use this agent when you need expert backend code review...

<example>
Context: The user has written a new REST API endpoint.
user: \"I wrote a user authentication endpoint, can you review it?\"
assistant: \"I'll launch the backend-senior-dev agent to review your authentication endpoint.\"
<commentary>
Backend code review is the primary use case for this agent.
</commentary>
</example>"
```

### `tools` (required)

Comma-separated list of tools the agent can use. Only include tools the agent actually needs.

| Tool | When to Include |
|------|----------------|
| `Read` | Agent reads files |
| `Write` | Agent creates new files |
| `Edit` | Agent modifies existing files |
| `Bash` | Agent runs shell commands |
| `Glob` | Agent searches for files by pattern |
| `Grep` | Agent searches file contents |
| `Agent` | Agent spawns other sub-agents |
| `WebFetch` | Agent fetches web content |
| `WebSearch` | Agent searches the web |
| `Skill` | Agent invokes skills |
| `TaskCreate/Get/List/Update` | Agent tracks multi-step work |

**Do not give agents tools they don't need.** An agent with `Write` access that only reviews code is a security risk and a source of unexpected behavior.

### `model`

- `sonnet` — default, fast, capable. Use for most agents.
- `opus` — more capable for complex reasoning, planning, and long-horizon tasks. Use sparingly (slower and more expensive).

### `color`

Terminal color for visual identification in Claude Code output: `cyan`, `green`, `yellow`, `red`, `purple`, `blue`.

### `memory`

Set to `user` to give the agent persistent memory via `~/.claude/agent-memory/<name>/`. The agent can then read and write files in that directory to build up knowledge across sessions.

Useful for:
- Agents that learn project conventions (codebase-navigator)
- Agents that track known issues across sessions (backend-senior-dev)
- Agents that adapt to user preferences over time

---

## Writing the System Prompt Body

The body of the agent file is its full system prompt. Everything after the `---` closing the frontmatter block.

### Give the Agent an Identity

Start with a clear identity statement. Agents with a defined identity produce more consistent, calibrated output than vague ones.

**Weak:**
```
You are a helpful assistant that can review code.
```

**Strong:**
```
You are a Senior Backend Software Engineer and Architect with 15+ years of
hands-on experience across Python, Go, Java, TypeScript/Node.js, Rust, C#,
and Ruby. You are known for your sharp pattern recognition — both identifying
excellent engineering decisions and diagnosing poor-quality code.
```

The identity sets the level of expertise, the perspective, and the calibration. It shapes how the agent weighs trade-offs and what it considers worth flagging.

### Define a Clear Process

Agents should follow a defined process, not make it up as they go. Structure the body around numbered steps.

**Weak:**
```
Review the code and provide feedback.
```

**Strong:**
```
## Review Methodology

### 1. Language & Idiom Assessment
- Verify idiomatic usage for the specific language and version
- Flag non-idiomatic constructs that violate language conventions

### 2. Pattern Recognition
Identify good patterns to affirm and bad patterns to flag...

### 3. Algorithm & Complexity Analysis
State Big-O time and space complexity for critical algorithms...
```

Named phases with concrete steps produce reliable, repeatable output.

### Specify Output Format

If you want structured output, describe the structure explicitly. Don't leave it up to the agent to decide.

```markdown
## Output Format

Structure your review as follows:

### Summary
Brief overall assessment (2-4 sentences).

### Critical Issues (Must Fix)
[What goes here, how detailed]

### Significant Improvements (Should Fix)
[What goes here]

### Verdict
A rating: Needs Major Rework / Needs Revision / Acceptable / Good / Excellent
```

### Behavioral Guidelines

Add explicit rules for edge cases and quality standards:

```markdown
## Guidelines

- **Be direct**: Vague feedback is worthless. Name the exact issue and the fix.
- **Calibrate severity**: Not every issue is critical. Distinguish bugs from style preferences.
- **Show, don't tell**: Provide corrected code snippets for non-trivial suggestions.
- **Ask when it matters**: If the language version or deployment context would change your analysis, ask.
```

### Delegation Rules (for orchestrator agents)

If the agent can spawn other agents, define clear delegation rules:

```markdown
## When to Delegate

- **Code review** → spawn `backend-senior-dev` or `frontend-senior-dev`
- **Codebase orientation** → spawn `codebase-navigator`
- **Execution tracing** → spawn `feature-path-tracer`

Never delegate the core implementation work. Own it.
```

---

## Patterns That Work

### The Expert Identity Pattern

Give the agent a specific, credentialed identity. Specify years of experience, languages/frameworks known, and a defining philosophy.

```
You are a Senior [Role] with [N]+ years of experience in [domain].
You are known for [distinctive trait].
```

### The Numbered Workflow Pattern

Structure every agent around a named, numbered workflow. Each phase should have 3–5 concrete actions.

```
## Workflow

### Phase 1: Orientation
1. Check codebase-navigator memory
2. Read the relevant files
3. Identify the canonical pattern

### Phase 2: Analysis
...

### Phase 3: Implementation
...

### Phase 4: Verification
...
```

### The Dual-Mode Pattern (Review/Fix)

For agents that can both review and fix:

```
## Operating Modes

**Review Mode** (default when asked to "review" or "assess"):
Produce a structured report. Do not apply changes automatically.

**Fix Mode** (triggered when asked to "fix", "improve", or "clean up"):
1. Critical issues: fix directly without asking permission
2. Mechanical improvements: apply directly
3. Architectural changes: spawn dev-agent with a precise spec
```

### The Memory Protocol Pattern

For agents with persistent memory:

```
## Memory Protocol

Check ~/.claude/agent-memory/[agent-name]/ at the start of each session.
After completing work, record:
- Patterns discovered
- Conventions confirmed
- Known issues and locations
- Gotchas worth remembering
```

---

## Common Mistakes

**Too much generality.** Agents that try to handle every case handle none of them well. Better to write a focused agent that does one thing excellently.

**No examples in the description.** Without examples, the orchestrating Claude will guess when to use your agent. With good examples, it routes correctly.

**Vague process steps.** "Understand the code" is not a step. "Read the handler file, trace the call to the service layer, read the service method" is a step.

**Unspecified output format.** If you don't describe the structure of the output, you'll get different structure every time.

**Giving agents tools they don't need.** If an agent only reads files, don't give it `Write` or `Bash`. Access to powerful tools when not needed creates risk and unpredictability.

**No behavioral guidelines.** Guidelines handle the edge cases your process steps don't cover. "Be direct and specific, not vague" and "calibrate severity accurately" produce measurably better reviews.

---

## Testing Your Agent

1. Install with `./install.sh`
2. In Claude Code, describe a scenario that should trigger the agent
3. Check if Claude picks the right agent and spawns it
4. Evaluate the output quality: is it specific? Is it structured as you specified? Does it follow the process?
5. Iterate on weak spots: add an example to the description if routing is wrong, add a guideline if a specific behavior is off

The most reliable way to test is to use the agent on your actual codebase with a real task.
