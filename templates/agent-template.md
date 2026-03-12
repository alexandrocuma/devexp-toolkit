---
# ── Source format (Claude Code) ───────────────────────────────────────────────
# Agent files are authored in Claude Code format. install.sh auto-transforms
# them into opencode format when installing for opencode. Do not maintain
# separate opencode files — edit here and re-run install.sh.

# REQUIRED: The unique identifier for this agent. Used by the Agent tool to reference it.
# Use lowercase kebab-case. Example: my-code-reviewer
name: my-agent

# REQUIRED: Describes when to use this agent and what it does.
# This is what the orchestrating Claude reads to decide whether to launch this agent.
# Rules for a good description:
#   - Lead with the trigger condition ("Use this agent when...")
#   - Name the primary capability clearly
#   - Include 2-4 <example> blocks showing realistic invocations
#   - Examples dramatically improve invocation accuracy — don't skip them
description: "Use this agent when [trigger condition]. It [primary capability].

<example>
Context: [Describe the situation that would lead someone to use this agent]
user: \"[Realistic user message]\"
assistant: \"[How Claude would respond, naming this agent]\"
<commentary>
[Why this is the right agent for this situation]
</commentary>
</example>

<example>
Context: [A second scenario — pick a different trigger]
user: \"[Another realistic message]\"
assistant: \"[Claude's response]\"
<commentary>
[Explanation of the match]
</commentary>
</example>"

# REQUIRED: Comma-separated list of tools this agent may use.
# Only include tools the agent actually needs — fewer is better for focus.
# Claude Code tools: Read, Write, Edit, Bash, Glob, Grep, Agent, WebFetch, WebSearch,
#                    Skill, TaskCreate, TaskGet, TaskList, TaskUpdate
# Opencode mapping: install.sh maps Read→read, Write→write, Edit→edit, Bash→bash,
#                   Glob→glob, Grep→grep, WebFetch→webfetch, WebSearch→websearch.
#                   Agent/Skill/Task* have no opencode equivalent and are dropped.
tools: Read, Write, Edit, Bash, Glob, Grep

# OPTIONAL: Model alias. install.sh maps these to full IDs for opencode:
#   sonnet → anthropic/claude-sonnet-4-20250514
#   opus   → anthropic/claude-opus-4-20250514
#   haiku  → anthropic/claude-haiku-4-20250514
model: sonnet

# OPTIONAL: Terminal color — Claude Code only, stripped for opencode.
# Options: cyan, green, yellow, red, purple, blue
color: cyan

# OPTIONAL: Persistent memory — Claude Code only, stripped for opencode.
# When set, the agent can read/write to ~/.claude/agent-memory/<name>/
memory: user
---

# [Agent Display Name]

<!-- One sentence: what this agent is and its core specialty. -->
You are a [role description] specializing in [domain].

## Core Role

<!-- 3-5 sentences: what the agent does, what makes it distinct, what perspective it brings. -->
[Describe the agent's identity, expertise, and the value it provides. Be specific about the
skill level, the domain, and the philosophy. This section sets the agent's character.]

## Workflow

<!-- The step-by-step process this agent follows. Number the steps. -->
<!-- Good agents are opinionated about their process — vague processes produce vague output. -->

### Step 1: [Orientation / Understanding]
- [What the agent reads or checks first]
- [What context it needs before acting]

### Step 2: [Analysis / Investigation]
- [How the agent approaches the problem]
- [What it looks for, what it evaluates]

### Step 3: [Execution]
- [What the agent produces or does]
- [How it makes decisions]

### Step 4: [Verification / Output]
- [How the agent validates its work]
- [What it checks before finishing]

## Guidelines

<!-- Behavioral rules that shape how the agent approaches its work. -->
<!-- These are the "always do" and "never do" rules that define the agent's character. -->

- **[Guideline name]**: [Specific behavioral rule]
- **[Guideline name]**: [Specific behavioral rule]
- **[Guideline name]**: [Specific behavioral rule]

## Output Format

<!-- What the agent produces. Be specific about structure, sections, and level of detail. -->
<!-- If the output is a review, specify sections. If it's code, specify what to deliver. -->

Structure your output as follows:

### [Section 1 Name]
[What goes here]

### [Section 2 Name]
[What goes here]

### [Section 3 Name]
[What goes here]

---

<!--
TIPS FOR WRITING EFFECTIVE AGENTS:

1. Specificity beats generality. "Review for race conditions in async code" is better
   than "review for bugs". The more specific the instruction, the more consistent the output.

2. Process matters. Agents that follow a defined process produce more reliable output than
   agents with vague instructions. Number your steps.

3. Examples in the description are critical. They are the primary signal Claude uses to
   decide when to invoke an agent. Invest time in writing realistic examples.

4. Match tool access to actual need. An agent that only reviews code doesn't need Write.
   An agent that navigates a codebase doesn't need WebSearch.

5. Define output structure. If the agent should produce a review with specific sections,
   name those sections. Structured output is easier to act on.

6. Give the agent an identity. "You are a senior engineer with 10 years of experience in
   distributed systems" produces better output than "You are a helpful assistant". Identity
   shapes the agent's perspective and calibration.

See docs/agent-authoring-guide.md for the full guide.
-->
