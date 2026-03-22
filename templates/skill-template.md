---
# REQUIRED: The skill name. Must match the directory name exactly.
# This becomes the slash command: /my-skill
# Use lowercase kebab-case. Prefix with a namespace if this is part of a suite (e.g., devexp-bugfix).
name: my-skill

# REQUIRED: One-line description of what this skill does.
# Shown in skill listings and used by the orchestrating Claude when deciding which skill to load.
# Be specific about the output and use case, not just the domain.
description: Does X for Y — producing Z
---

# [Skill Title]

<!-- One sentence: role statement. What persona does this skill give Claude? -->
You are the **[Role Name]**, specialized in [specific domain or task type].

## Triggered by

<!-- List which agents or skills invoke this skill. Helps the orchestrator understand the invocation graph. -->
<!-- Examples: "dev-agent — for focused bug investigation", "bugfix skill — after fix implementation" -->

- `[agent-or-skill-name]` — [reason they invoke this skill]
- `[agent-or-skill-name]` — [reason]

## When to Use

<!-- Be explicit about the trigger conditions. This helps the orchestrator decide when to load this skill. -->
<!--
Choose the pattern that fits:
  - "Invoked directly by the user when..."  (for user-facing skills)
  - "Spawned by [other skill] when..."       (for sub-skills in a pipeline)
  - "Use when [condition]..."                (for general-purpose skills)
-->

[Trigger condition description — when should this skill be loaded?]

Common triggers:
- [User says or asks...]
- [Another skill delegates when...]
- [Condition that makes this skill the right choice]

## Process

<!-- The ordered steps this skill follows. Be concrete — vague steps produce vague output. -->
<!-- Good: "1. Read the target file and identify all public functions" -->
<!-- Bad: "1. Understand the code" -->

### 1. [Phase Name: e.g., Discovery, Analysis, Implementation, Verification]
- [Concrete action]
- [Concrete action]
- [What to look for / what to read]

### 2. [Phase Name]
- [Concrete action]
- [Decision rule or heuristic]

### 3. [Phase Name]
- [Concrete action]
- [What to produce]

### 4. [Phase Name: Verification or Output]
- [How to validate the work]
- [What to check before finishing]

## Guidelines

<!-- Optional: behavioral rules specific to this skill. -->
<!-- Use for rules that aren't captured by the process steps — edge cases, safety rules, quality standards. -->

1. [Rule]
2. [Rule]
3. [Rule]

## Output

<!-- Required: what does this skill produce? -->
<!-- Be specific about structure, format, and level of detail. -->
<!-- If the output is a report, name the sections. If it's code, describe what to deliver. -->

Provide [output type] with:
- [Required element 1]
- [Required element 2]
- [Required element 3]
- [Required element 4]

---

<!--
TIPS FOR WRITING EFFECTIVE SKILLS:

1. Skills inject context, not just instructions. When your SKILL.md is loaded, its full
   content becomes part of Claude's context for that conversation. Write it as instructions
   to Claude, not documentation about the skill.

2. Process steps should be actionable. "Read the file at path X" is actionable.
   "Understand the codebase" is not. Make every step something Claude can execute.

3. Output specification is load-bearing. If you want a specific format (sections, tables,
   code blocks), describe it explicitly. Unspecified output format produces inconsistent results.

4. Keep scope narrow. A skill that does one thing well is more reliable than one that tries
   to handle every case. If you find yourself writing "if X then do Y else do Z", consider
   splitting into two skills.

5. "When to Use" helps orchestration. If this skill is designed to be spawned by another
   skill (like devexp-bugfix spawning devexp-root-cause), say so explicitly. This helps
   the orchestrator make correct routing decisions.

6. Test by reading it cold. After writing, read the skill as if you've never seen it.
   Ask: would Claude know what to do with just this file? If not, add specificity.

See docs/skill-authoring-guide.md for the full guide.
-->
