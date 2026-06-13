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
4a. **If your task consults `## Known Technical Debt` or `## Gotchas`**: extract the date(s) and file path(s) from the entries you're relying on, then run `git log --since="<entry-date>" --oneline -- <path-from-entry> 2>/dev/null`. If this returns commits, treat that specific entry as unverified — read the current file instead of restating the claim. **If an entry has no `[YYYY-MM-DD]` date at all (pre-migration format), treat it as unverified by default regardless of commit history** — read the current file. If the atlas's top-level `Last updated` date is >30 days old, also note that a `codebase-navigator` refresh may be worthwhile.
5. Check for an existing knowledge graph:
   Check whether `graphify-out/graph.json` exists in the project root.
   If yes: run `graphify query "<domain-specific question for this agent>"` and use the results to surface conventions, ADRs, and known issues relevant to the agent's task.
   If no graph exists, continue — the atlas is sufficient; building one is the user's call (`/graphify`), not the agent's.
6. Skip redundant discovery steps that the atlas or graph already covers
```

### What to ask graphify in step 5

Tailor the query to the agent's role:

| Agent type | Recommended query |
|------------|-------------------|
| Implementation (dev-agent, scaffold) | `"<task description> conventions patterns known issues"` |
| Review (backend-senior-dev, frontend-senior-dev) | `"What are the error handling, naming, and architecture conventions?"` |
| Testing (test-gen) | `"What are the test conventions, fixture patterns, and known coverage gaps?"` |
| Security audit | `"What are known vulnerabilities, security decisions, and auth patterns?"` |
| Performance analysis | `"What are known bottlenecks, caching strategies, and data access patterns?"` |
| Documentation (gen-indexer, docs-sync) | `"What are the architecture, conventions, ADRs, and implementation patterns?"` |
| Dependency audit | `"What are known dependency vulnerabilities, accepted CVEs, and upgrade constraints?"` |
| Migration | `"What are prior migration decisions, upgrade constraints, and library version history?"` |

### When to skip Phase 0

- `docs-sync` — operates on git diff, not codebase conventions; runs Phase 0 only to check if CLAUDE.md/README changed
- `changelog` — works from git history only; no codebase orientation needed
- `postmortem` — incident docs; no codebase orientation needed
- `project-manager` — GitHub issue management; no codebase orientation needed

---

## graphify Protocol

graphify turns a codebase into a persistent, queryable knowledge graph (`graphify-out/graph.json`). It's optional per project — a project only has a graph if someone has run `/graphify` in it. Agents that write durable artifacts should keep an existing graph current, not build one on the agent's own initiative.

### Read protocol (all agents, Phase 0)

1. Check whether `graphify-out/graph.json` exists before querying — querying a project with no graph wastes a turn
2. If `mcp__graphify__query_graph` (or sibling tools `get_node`, `get_neighbors`, `shortest_path`) are available in the session, prefer them — they return structured results over stdio without spawning a subprocess per call. See "Optional: graphify as a project MCP" below for how a project gets these tools.
3. Otherwise, fall back to the CLI via `Bash`/`Skill`: `graphify query "<question>"` — BFS for broad context, `--dfs` to trace one specific path; `graphify path "<A>" "<B>"` for relationships between two named concepts, `graphify explain "<node>"` for a plain-language explanation of one
4. Prefer graphify (either form) over grep for conceptual questions ("why is X designed this way", "what are the conventions for Y") — it understands intent, not just keywords

### Optional: graphify as a project MCP

graphify can run as a stdio MCP server (`graphify <path> --mcp`) exposing `query_graph`, `get_node`, `get_neighbors`, and `shortest_path` as structured tools — faster and cheaper than shelling out to the CLI on every lookup, since the graph stays loaded in the server process.

This is **per-project and opt-in, not part of the curated `mcps/registry.json`** (that registry is installed globally via `./install.sh` and would try to spawn `graphify --mcp` in every project regardless of whether a graph exists — most don't have one). Instead, a team that has built a graph for a specific project can register it there with `scope: "project"`:

```json
{
  "name": "graphify",
  "description": "Structured queries over this project's knowledge graph",
  "command": "graphify",
  "args": [".", "--mcp"],
  "scope": "project"
}
```

Add this via `claude mcp add --scope project` (Claude Code) once `graphify-out/graph.json` exists. Agents should check for `mcp__graphify__*` tools first (step 2 above) and transparently fall back to the CLI — never assume the MCP form is present.

### Write protocol (documentation and orientation agents)

After producing an artifact that represents project knowledge, trigger an incremental rebuild so the graph reflects it:

| Agent | What changed | Action |
|-------|-------------|--------|
| `codebase-navigator` | Atlas file + `docs/` folder | `/graphify --update` |
| `gen-indexer` skill | Generated `CLAUDE.md` | `/graphify --update` |
| `docs-sync` | Updated `CLAUDE.md` and/or `README.md` | `/graphify --update` |

`--update` re-extracts only new/changed files — cheap to run after every write. Only run it if `graphify-out/graph.json` already exists; never bootstrap a graph (`/graphify` with no `--update`) on an agent's own initiative — that's an expensive, user-directed operation.

### Availability

graphify is a skill backed by a local Python package, and most projects won't have a graph built. Every agent must handle both cases gracefully:

```
If graphify-out/graph.json doesn't exist or graphify is unavailable, skip silently — [atlas / memory file / source files] are sufficient.
```

Never block execution, prompt the user to run `/graphify`, or report an error when there's no graph to query.

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

The `tools:` field in agent frontmatter lists Claude's **built-in tools only** — not MCP tools. MCP tools (context7) are globally available in the session and do not need to be declared. graphify is a skill, not an MCP — agents invoke it via `Bash` (`graphify query ...`) or the `Skill` tool, both of which must be declared if used.

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
- graphify is a complement to memory, not a replacement — memory is agent-private, the knowledge graph (when one exists) is shared and per-project
- **Dated atlas entries are advisory, not absolute**: `## Known Technical Debt` and `## Gotchas` entries carry a `[YYYY-MM-DD]` observation date. Before repeating one of these claims in your own output, do the Phase 0 step 4a freshness check — don't propagate a claim that may already be resolved.
- **Don't duplicate the atlas**: if Phase 0 already reads `~/.claude/agent-memory/codebase-navigator/<project>.md`, your memory should hold only what that atlas doesn't — your agent's own domain-specific findings (e.g., prior trace results, prior audit findings, prior groomed tickets), not architecture, layer maps, conventions, or file paths the atlas already covers.
- **Self-heal format drift**: if your memory files (atlas, `MEMORY.md`, or topic files) use section names or structures from a previous version of this convention, migrate them to the current structure as part of your normal write-back for that session — don't perpetuate an outdated format indefinitely. codebase-navigator's Memory Protocol Migration Pass is the concrete implementation of this for atlases.

### Cross-Agent Duplication Mapping

`~/.claude/agent-memory/graphify-out/` holds an optional knowledge graph built over the **entire agent-memory corpus** (across all agents and projects) — distinct from a per-project `graphify-out/`. Its purpose is to surface cross-agent duplication: cases where multiple agents' memory files reference the same project artifact (e.g. two agents both pointing at `codebase-navigator/<project>.md`'s atlas), which is a signal that context could be consolidated into the shared atlas instead of repeated per-agent.

**When to run:** manually, via `/improve` — not hooked to memory writes. At current write volumes (a handful of agents writing memory across a couple of projects), the signal-to-cost ratio favors periodic manual runs over an automated hook.

**How to run:**
```bash
/graphify ~/.claude/agent-memory
```

**Backend:** prefer the Gemini backend (set `GEMINI_API_KEY` or `GOOGLE_API_KEY`) — a 23-file/~24K-word run completed in ~57s at ~50K tokens via Gemini, versus ~288s at ~74K tokens via the Claude-subagent fallback for a smaller 7-file sample. Both produce the same signal; Gemini is materially cheaper and faster for this corpus size.

**Reading the output:** the `## Surprising Connections` section of `graphify-out/GRAPH_REPORT.md` is the primary signal — cross-agent references there indicate memory that could be consolidated into a shared atlas rather than duplicated per-agent.

---

## Agent File Checklist

Before submitting a new agent file, verify:

- [ ] Frontmatter has `name`, `description` (with `<example>` blocks), `tools`, `color`
- [ ] Body starts with Phase 0 (unless explicitly exempted above)
- [ ] Phase 0 includes the `graphify-out/graph.json` existence-check → `graphify query` pattern
- [ ] context7 usage documented where the agent writes code or reviews library usage
- [ ] `## Chaining` section present at end of body
- [ ] `/graphify --update` write-back included if the agent produces knowledge artifacts
- [ ] If Phase 0 reads `## Known Technical Debt`/`## Gotchas` from the atlas, step 4a's freshness check is applied before repeating any claim
- [ ] All tools in `tools:` frontmatter are actually used in the body
- [ ] Installed via `./install.sh` after changes
- [ ] If frontmatter has `memory: user`, the body has a `## Persistent Agent Memory` section consistent with [Memory Convention](#memory-convention) — including the "don't duplicate the atlas" and "self-heal format drift" guardrails — and vice versa (no `## Persistent Agent Memory` section without `memory: user` in frontmatter)
