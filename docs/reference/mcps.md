# MCP Servers Reference

MCP servers extend Claude's capabilities with external tools (documentation lookup, databases, APIs). The devexp framework manages MCPs alongside agents and skills.

## Registry Format

MCP servers are declared in `mcps/registry.json`. Two transport types are supported: **stdio** (default) and **HTTP/SSE**.

**stdio MCP:**
```json
{
  "name": "context7",
  "description": "Up-to-date library documentation for any package",
  "command": "npx",
  "args": ["-y", "@upstash/context7-mcp"],
  "scope": "user",
  "env": {},
  "required_env": []
}
```

**HTTP/SSE MCP (locally-hosted):**
```json
{
  "name": "my-mcp",
  "description": "Short description",
  "transport": "http",
  "url": "http://localhost:1234/mcp",
  "docker_compose": "mcps/my-mcp/docker-compose.yml",
  "scope": "user",
  "env": {},
  "required_env": ["MY_MCP_API_KEY"],
  "setup_instructions": "Human-readable setup guidance shown when required_env keys are missing"
}
```

| Field | Description |
|-------|-------------|
| `name` | Unique MCP identifier |
| `description` | What this MCP provides |
| `transport` | `"http"` for streamable-HTTP; `"sse"` for legacy SSE-only; omit for stdio (default) |
| `url` | Server URL — required when `transport` is `"http"` or `"sse"` |
| `command` | Executable to run — stdio MCPs only |
| `args` | Arguments passed to the command — stdio MCPs only |
| `docker_compose` | Path to a Docker Compose file (relative to repo root); installer auto-starts these |
| `scope` | `"user"` (global) or `"project"` (Claude Code only) |
| `env` | Static environment variables to pass |
| `required_env` | Env vars that must be set — installer shows a loud `[REQUIRED]` warning if missing |
| `setup_instructions` | Human-readable text shown when `required_env` keys are absent |

---

## MCPs in This Repo

| Name | Transport | Description |
|------|-----------|-------------|
| context7 | stdio | Up-to-date library documentation and code examples for any package |
| ui-inspector | stdio | UI/UX inspection via headless Chromium — screenshot, interact (click/type/scroll), ARIA accessibility tree, computed CSS, axe-core a11y audit, page metrics. No external daemon required. |

---

## Project-Scoped MCPs (graphify)

Most entries above use `scope: "user"` — installed globally via `./install.sh`, active in every project. Some tools only make sense for one specific project and shouldn't be in that curated list.

graphify is the example: it can run as a stdio MCP (`graphify <path> --mcp`) exposing structured query tools (`query_graph`, `get_node`, `get_neighbors`, `shortest_path`) over a project's `graphify-out/graph.json`. Since most projects don't have a graph, registering it globally would spawn a server with nothing to serve almost everywhere. Instead, a team that has built a graph for a given project registers it there with `scope: "project"`:

```json
{
  "name": "graphify",
  "description": "Structured queries over this project's knowledge graph",
  "command": "graphify",
  "args": [".", "--mcp"],
  "scope": "project"
}
```

Add it via `claude mcp add --scope project` once `graphify-out/graph.json` exists. See [graphify Protocol](../development/agent-architecture-reference.md#graphify-protocol) for how agents detect and prefer these tools when present, falling back to the CLI form otherwise.

---

## API Keys and Secrets

MCPs that need API keys use `mcps/.env` (gitignored):

```bash
cp mcps/.env.example mcps/.env
# edit mcps/.env and fill in your keys
./install.sh
```

The installer loads `mcps/.env` and passes values as `--env KEY=VALUE` to `claude mcp add` (stored permanently in Claude Code's MCP config) or writes them into opencode's `config.json`. Any MCP whose `required_env` keys are missing is skipped with a loud red `[REQUIRED]` warning until keys are provided.

`mcps/.env.example` is committed and documents what keys are expected. Never commit `mcps/.env`.

---

## Docker-Backed MCPs

MCPs with a `docker_compose` field run as local Docker services. The installer starts them automatically:

```bash
docker compose -f <docker_compose_path> up -d
```

---

## Adding a New MCP

1. Add an entry to `mcps/registry.json`
2. If it needs secrets, add key names to `required_env`, set `setup_instructions`, document keys in `mcps/.env.example`
3. If it runs as a local Docker service, add a `docker_compose` field and create `mcps/<name>/docker-compose.yml`
4. Run `./install.sh --mcps-only` to register it (or `--dry-run` to preview)

---

## CLI Compatibility

| | Claude Code | opencode |
|---|---|---|
| Install method | `claude mcp add --scope <scope>` | Written to `~/.config/opencode/config.json` |
| Uninstall method | `claude mcp remove` | Entry removed from config.json |
