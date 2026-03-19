# Agent Architecture Reference

This document defines the structural conventions that all devexp agents must follow. It is the authoritative reference for the Phase 0 pattern, MCP usage protocols, and chaining conventions.

---

## The Phase 0 Pattern

Every agent that operates on a codebase must begin with **Phase 0: Orient** before doing any domain work. This is non-negotiable — it prevents agents from re-deriving information that is already indexed, and ensures reviews and implementations are calibrated to the project's actual conventions.

### Standard Phase 0 template

```markdown
### Phase 0: Orient

1. Run `git rev-parse --show-toplevel 2>/dev/null || pwd` to get the project root
2. Derive the project name from the root directory name
3. Read `~/.claude/agent-memory/codebase-navigator/MEMORY.md` to see if an atlas exists
4. If yes, read `~/.claude/agent-memory/codebase-navigator/<project-name>.md` — use stack, layer map, canonical example, and conventions; skip re-deriving what's already there
5. Query OpenViking for existing project knowledge:
   `mcp__openviking__list_namespaces` — check if `<project-name>` namespace exists
   If yes: `mcp__openviking__query` — question: "<domain-specific question for this agent>" — namespace: "viking://<project-name>/"
   Use results (score > 0.5) to surface conventions, ADRs, and known issues relevant to the agent's task.
   If OpenViking is unavailable, continue — the atlas is sufficient.
6. Skip redundant discovery steps that the atlas or OpenViking already covers
```

### What to ask OpenViking in step 5

Tailor the query to the agent's role:

| Agent type | Recommended query |
|------------|-------------------|
| Implementation (dev-agent, scaffold) | `"<task description> conventions patterns known issues"` |
| Review (backend-senior-dev, frontend-senior-dev) | `"What are the error handling, naming, and architecture conventions?"` |
| Testing (test-gen) | `"What are the test conventions, fixture patterns, and known coverage gaps?"` |
| Security audit | `"What are known vulnerabilities, security decisions, and auth patterns?"` |
| Performance analysis | `"What are known bottlenecks, caching strategies, and data access patterns?"` |
| Documentation (gen-claude-md, docs-sync) | `"What are the architecture, conventions, ADRs, and implementation patterns?"` |
| Dependency audit | `"What are known dependency vulnerabilities, accepted CVEs, and upgrade constraints?"` |
| Migration | `"What are prior migration decisions, upgrade constraints, and library version history?"` |

### When to skip Phase 0

- `docs-sync` — operates on git diff, not codebase conventions; runs Phase 0 only to check if CLAUDE.md/README changed
- `changelog` — works from git history only; no codebase orientation needed
- `postmortem` — incident docs; no codebase orientation needed
- `project-manager` — GitHub issue management; no codebase orientation needed

---

## OpenViking Protocol

OpenViking is the framework's semantic knowledge store. All agents that write outputs (not just read) must also **write back** so the knowledge base stays current.

### Read protocol (all agents, Phase 0)

1. `list_namespaces` — check if the project namespace exists before querying (avoids errors on first run)
2. `query` with namespace `"viking://<project-name>/"` — use results with score > 0.5
3. Never use bare `search` for the Phase 0 pre-check — always `list_namespaces` → `query`

### Write protocol (documentation and orientation agents)

After producing an artifact that represents project knowledge, ingest it:

| Agent | What to ingest | Path |
|-------|---------------|------|
| `codebase-navigator` | Atlas file + `docs/` folder | `viking://<project-name>/atlas`, `viking://<project-name>/docs` |
| `gen-claude-md` skill | Generated `CLAUDE.md` | `viking://<project-name>/claude-md` |
| `docs-sync` | Updated `CLAUDE.md` and/or `README.md` | `viking://<project-name>/claude-md`, `viking://<project-name>/readme` |

**Never ingest the raw project root** — it would index all source files including generated code. Always ingest specific high-signal files: the atlas, `docs/`, `CLAUDE.md`, `README.md`.

### Availability

OpenViking is a locally-hosted HTTP service. It may not be running. Every agent must handle unavailability gracefully:

```
If OpenViking is unavailable, skip silently — [atlas / memory file / source files] are sufficient.
```

Never block execution or report an error when OpenViking is down.

---

## context7 Protocol

context7 provides up-to-date library and framework documentation. Use it when writing code against external APIs or flagging library usage in reviews.

### When to use

- Before writing code that calls a library API (may have changed, deprecated, or have known pitfalls)
- Before flagging a library usage as wrong in a review (verify it isn't a new API you don't know)
- When generating boilerplate that uses framework APIs (ORMs, HTTP clients, auth libs)

### Standard usage

```
1. mcp__context7__resolve-library-id — find the library's context7 ID
2. mcp__context7__query-docs — query the specific API, pattern, or migration topic
```

Fall back to WebFetch on the official docs URL only if context7 doesn't have the library indexed.

### Agents that must use context7

| Agent | When |
|-------|------|
| `dev-agent` | Before using any library API in implementation |
| `scaffold` | Before generating code with library/framework APIs |
| `backend-senior-dev` | Before flagging library usage as incorrect in a review |
| `frontend-senior-dev` | Before flagging framework APIs as deprecated or wrong |
| `test-gen` | Before writing tests against a specific test framework's API |
| `migration` | When fetching migration guides and breaking changes |
| `dep-audit` | When enriching Critical/High CVE findings with upgrade guides |
| `security` | When verifying security-sensitive library configuration |
| `ci-cd` | When writing or updating pipeline configuration |
| `tech-lead` | When producing ADRs involving library or framework choices |

---

## Chaining Convention

Every agent must have a `## Chaining` section at the end of its body. This section tells orchestrators and users what to do next based on the agent's output.

### Format

```markdown
## Chaining

After completing [task]:
- **[Condition]** → suggest invoking `<agent-name>` to [reason]
- **[Condition]** → suggest invoking `/<skill-name>` skill to [reason]
- **[Condition]** → note that [no further action needed / schedule X]
```

### Guidelines

- Always include at least one "no further action needed" branch (e.g., "no issues found")
- Map findings to the agents best suited to act on them — reviews lead to fixers, audits lead to migration agents
- Don't chain to the same agent type (e.g., a reviewer suggesting another reviewer)
- Keep conditions concrete — "if N critical findings" not "if things look bad"

---

## Tool Declaration in Frontmatter

The `tools:` field in agent frontmatter lists Claude's **built-in tools only** — not MCP tools. MCP tools (OpenViking, context7) are globally available in the session and do not need to be declared.

**Built-in tools reference:**

| Tool | Use for |
|------|---------|
| `Read` | Reading files |
| `Write` | Creating new files |
| `Edit` | Modifying existing files |
| `Bash` | Shell commands |
| `Glob` | File pattern matching |
| `Grep` | Content search |
| `Agent` | Spawning sub-agents |
| `WebFetch` | Fetching URLs |
| `WebSearch` | Web search |
| `Skill` | Invoking skills |
| `TaskCreate/Get/List/Update` | Task tracking (dev-agent only) |

Declare only the tools the agent actually uses. Do not add `Agent` unless the agent's body contains an explicit `Agent` tool call.

---

## Memory Convention

Agents with `memory: user` in frontmatter have a persistent memory directory at `~/.claude/agent-memory/<agent-name>/`. Follow these conventions:

- `MEMORY.md` is the index — kept concise (< 200 lines), links to topic files
- Topic files hold detail: `patterns.md`, `projects.md`, `gotchas.md`
- Save stable, cross-session facts — not session-specific context
- Always update when the user explicitly asks you to remember or forget something
- OpenViking is a complement to memory, not a replacement — memory is agent-private, OpenViking is shared across all agents

---

## Agent File Checklist

Before submitting a new agent file, verify:

- [ ] Frontmatter has `name`, `description` (with `<example>` blocks), `tools`, `color`
- [ ] Body starts with Phase 0 (unless explicitly exempted above)
- [ ] Phase 0 includes `list_namespaces` → `query` OpenViking pattern
- [ ] context7 usage documented where the agent writes code or reviews library usage
- [ ] `## Chaining` section present at end of body
- [ ] OpenViking write-back included if the agent produces knowledge artifacts
- [ ] All tools in `tools:` frontmatter are actually used in the body
- [ ] Installed via `./install.sh` after changes
