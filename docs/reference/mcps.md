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

**HTTP/SSE MCP:**
```json
{
  "name": "my-mcp",
  "description": "Short description",
  "transport": "http",
  "url": "http://localhost:1234/mcp",
  "headers": {
    "Authorization": "Bearer ${MY_MCP_API_KEY}"
  },
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
| `args` | Arguments passed to the command — stdio MCPs only. `${VAR}` placeholders are substituted at install time |
| `headers` | HTTP headers — `http`/`sse` MCPs only. `${VAR}` placeholders in values are substituted at install time |
| `scope` | `"user"` (global, default) or `"project"` (Claude Code only) |
| `env` | Environment variables passed to a stdio MCP server |
| `required_env` | Env vars that must be set — if any is missing the MCP is skipped with a loud `[REQUIRED]` warning |
| `setup_instructions` | Human-readable text printed after the `[REQUIRED]` warning (Claude Code install only) |

Fields are defined by the `MCP` struct in `cli/internal/mcp/types.go`; any other key is ignored. Per-field detail and precedence rules: [MCP Guide](../development/mcp-guide.md).

---

## MCPs in This Repo

| Name | Transport | Description |
|------|-----------|-------------|
| context7 | stdio | Up-to-date library documentation and code examples for any package |
| ui-inspector | stdio | UI/UX inspection via headless Chromium. **Not vendored** — lives at [mcp-ui-inspector](https://github.com/alexandrocuma/mcp-ui-inspector); clone it, run `./setup.sh`, and set `UI_INSPECTOR_DIR` (substituted into `args`). Until it is set the installer skips it with a `[REQUIRED]` warning (plus setup guidance, on Claude Code). |

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

The installer reads values from `mcps/.env` and the shell environment (`mcps/.env` wins — `cli/cmd/registry.go` `buildEnv`). For stdio MCPs it passes them as `-e KEY=VALUE` to `claude mcp add` (stored permanently in Claude Code's MCP config) or writes them into the entry's `env` in opencode's `config.json`; for HTTP/SSE MCPs they reach the server only through `${VAR}` placeholders in `headers`. Any MCP whose `required_env` keys are missing is skipped with a loud red `[REQUIRED]` warning until keys are provided.

`mcps/.env.example` is committed and documents what keys are expected. Never commit `mcps/.env`.

---

## Server Lifecycle

The installer only **registers** MCPs — a command for stdio, a URL for HTTP/SSE. It never starts a server: whatever serves an HTTP/SSE URL must already be running when the CLI connects. There is no `docker_compose` field; installer-managed Docker services were removed with `scripts/docker_services.py` in `61f6c9f`. See [MCP Guide](../development/mcp-guide.md).

---

## Adding a New MCP

1. Add an entry to `mcps/registry.json`
2. If it needs secrets, add key names to `required_env`, set `setup_instructions`, document keys in `mcps/.env.example`
3. If it is served over HTTP/SSE, set `transport` and `url` (plus `headers` for auth) — and run the server yourself; the installer won't start it
4. Run `./install.sh --mcps-only` to register it (or `--dry-run` to preview)

---

## CLI Compatibility

| | Claude Code | opencode |
|---|---|---|
| Install method | `claude mcp add --scope <scope>` | Written to `~/.config/opencode/config.json` |
| HTTP/SSE entry | `--transport http\|sse` + `-H "Key: Value"` per header | `"type": "remote"` with `url` (and `headers`) |
| Re-running install | Already-installed MCPs are skipped | Unchanged entries skipped; changed entries overwritten |
| Force refresh | `./install.sh --reinstall-mcps` (remove, then re-add) | same |
| Uninstall method | `claude mcp remove` | Entry removed from config.json |

Sources: `cli/internal/mcp/claude.go`, `cli/internal/mcp/opencode.go`, `cli/cmd/install.go`, `uninstall.sh`.
