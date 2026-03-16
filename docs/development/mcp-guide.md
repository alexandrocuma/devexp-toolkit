# MCP Guide

This guide explains how MCP (Model Context Protocol) servers work in the devexp framework, how to configure secrets, and how to add new MCPs to the registry.

---

## What Are MCPs?

MCP servers extend Claude with additional tool capabilities beyond what's built into the CLI. They run as local processes and expose new tools that Claude can call during a conversation.

Examples of what MCPs can provide:
- **Up-to-date documentation**: Fetch current library docs for any package, bypassing training data cutoff
- **Database access**: Query a live database from within Claude conversations
- **Web browsing**: Navigate and interact with web pages
- **Filesystem access**: Read files outside the project directory
- **External APIs**: Connect Claude to services like Linear, Jira, Notion, or Slack

devexp manages a curated registry of MCP servers in `mcps/registry.json`. The installer registers them with Claude Code or opencode automatically.

---

## The Registry Format

`mcps/registry.json` is a JSON array. Each entry describes one MCP server.

### Full field reference

```json
{
  "name": "my-mcp",
  "description": "What this MCP does — shown in logs and for documentation",
  "command": "npx",
  "args": ["-y", "my-mcp-package@latest"],
  "scope": "user",
  "env": {
    "OPTIONAL_VAR": ""
  },
  "required_env": ["REQUIRED_API_KEY"]
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Unique identifier. Used as the MCP server name in CLI config. |
| `description` | string | Yes | What the MCP does. Shown in installer output and documentation. |
| `transport` | string | No | `"http"` for streamable-HTTP transport (preferred); `"sse"` for legacy SSE-only servers. Omit for stdio (default). |
| `url` | string | Conditional | Server URL — required when `transport` is `"http"` or `"sse"`. |
| `command` | string | Conditional | The executable to run. Required for stdio MCPs. Usually `npx`, `uvx`, or `node`. |
| `args` | array | Conditional | Arguments passed to `command`. Required for stdio MCPs. |
| `docker_compose` | string | No | Path to a Docker Compose file (relative to repo root). The installer runs `docker compose up -d` automatically before registering the MCP. |
| `scope` | string | No | `"user"` (default) — installed at the user level, available in all projects. Use `"project"` to scope to a specific project. |
| `env` | object | No | Environment variables passed to the MCP server. Values can be empty strings — they'll be resolved from `mcps/.env` or the shell at install time. |
| `required_env` | array | No | Environment variable names that must be present for the MCP to install. If any are missing, the MCP is skipped with a warning. |
| `setup_instructions` | string | No | Human-readable guidance shown alongside the `[REQUIRED]` warning when `required_env` keys are missing. |

### Minimal entry (no API key required)

```json
{
  "name": "context7",
  "description": "Up-to-date library documentation and code examples for any package",
  "command": "npx",
  "args": ["-y", "@upstash/context7-mcp"],
  "scope": "user",
  "env": {},
  "required_env": []
}
```

### Entry with a required API key

```json
{
  "name": "my-api-mcp",
  "description": "Integrates with My API service",
  "command": "npx",
  "args": ["-y", "my-api-mcp-server"],
  "scope": "user",
  "env": {
    "MY_API_KEY": ""
  },
  "required_env": ["MY_API_KEY"]
}
```

When `MY_API_KEY` is in `required_env`, the installer:
- Checks `mcps/.env` and the current shell environment for the value
- Skips the MCP and prints a warning if the key is not found
- Passes the key to the CLI when registering if found

### HTTP/SSE entry (locally-hosted server)

Use `"transport": "http"` for MCP servers that run as a local HTTP service rather than a subprocess (streamable-HTTP, the current MCP protocol). Use `"transport": "sse"` only for legacy servers that speak the older SSE-only protocol.

```json
{
  "name": "my-mcp",
  "description": "What this MCP provides",
  "transport": "http",
  "url": "http://localhost:2033/mcp",
  "docker_compose": "mcps/my-mcp/docker-compose.yml",
  "scope": "user",
  "env": {},
  "required_env": ["MY_MCP_API_KEY"],
  "setup_instructions": "Set MY_MCP_API_KEY in mcps/.env and re-run ./install.sh"
}
```

When `docker_compose` is set, the installer runs `docker compose -f <path> up -d` before registering the MCP, so the service is available by the time Claude tries to connect.

---

## Secrets with mcps/.env

`mcps/.env` holds API keys and other secrets needed by MCP servers. It is gitignored — never commit real values.

### Setup

```bash
cp mcps/.env.example mcps/.env
```

Edit `mcps/.env` and fill in your values:

```bash
# mcps/.env
MY_API_KEY=your_actual_key_here
ANOTHER_SECRET=another_value
```

### How secrets are resolved

At install time, the installer merges values from two sources (in priority order):

1. `mcps/.env` — takes precedence
2. Current shell environment

This means you can also set secrets as shell environment variables before running `./install.sh`:

```bash
MY_API_KEY=sk-... ./install.sh
```

Secrets are stored in the CLI's configuration at install time:
- **Claude Code**: stored in the MCP server config via `claude mcp add --env KEY=VALUE`
- **opencode**: written to `~/.config/opencode/config.json` under the MCP's `env` field

### Precedence rules

| Source | Precedence |
|--------|-----------|
| `mcps/.env` | Highest — overrides shell |
| Shell environment | Lower — used if not in .env |
| Empty string in `registry.json` `env` field | Lowest — placeholder only |

---

## Claude Code vs opencode: MCP Differences

### Claude Code

MCPs are registered using the `claude mcp add` command:

```bash
claude mcp add --scope user my-mcp -- npx -y my-mcp-package
```

The installer calls this automatically for each entry in `registry.json`. Already-installed MCPs are skipped.

After installation, verify with:

```bash
claude mcp list
```

### opencode

MCPs are written directly to `~/.config/opencode/config.json` under the `mcp` key:

```json
{
  "mcp": {
    "my-mcp": {
      "type": "local",
      "command": ["npx", "-y", "my-mcp-package"],
      "env": {
        "MY_API_KEY": "your-value"
      }
    }
  }
}
```

The installer handles this automatically. Already-configured MCPs are skipped.

### Key differences

| | Claude Code | opencode |
|---|-------------|---------|
| Config location | CLI-managed (not directly editable) | `~/.config/opencode/config.json` |
| Install mechanism | `claude mcp add` command | JSON patch to config file |
| `scope` field | Supported (`user` or `project`) | Ignored — all MCPs are user-scoped |
| Verification | `claude mcp list` | Read `~/.config/opencode/config.json` |

---

## Adding a New MCP to the Framework

### Step 1: Find the MCP package

Most MCP servers are published as npm packages and run via `npx`. The package name is usually listed in the MCP's documentation.

### Step 2: Add an entry to registry.json

Open `mcps/registry.json` and add your entry:

```json
[
  {
    "name": "context7",
    ...
  },
  {
    "name": "my-new-mcp",
    "description": "Brief description of what this MCP provides",
    "command": "npx",
    "args": ["-y", "the-npm-package-name"],
    "scope": "user",
    "env": {},
    "required_env": []
  }
]
```

### Step 3: If the MCP needs an API key

1. Add the key name to `required_env`:
   ```json
   "required_env": ["MY_MCP_API_KEY"]
   ```

2. Add it to `env` as an empty string (placeholder):
   ```json
   "env": {
     "MY_MCP_API_KEY": ""
   }
   ```

3. Document it in `mcps/.env.example`:
   ```bash
   # Required for my-new-mcp — get a key at https://example.com/api
   # MY_MCP_API_KEY=your_key_here
   ```

### Step 3b: If the MCP is a locally-hosted HTTP/SSE server

Set `"transport": "http"` (or `"sse"` for legacy SSE-only servers) and `"url"` in the registry entry. Add a `"setup_instructions"` field to explain what the user needs to configure.

The server process can be started in two ways:

**Docker-backed** — add a `docker_compose` field pointing to a Compose file; the installer runs `docker compose up -d` automatically:

1. Create `mcps/<name>/docker-compose.yml` with the service definition.
2. Set `"docker_compose": "mcps/<name>/docker-compose.yml"` in the registry entry.

**Process-based (pip/native)** — manage the process yourself (e.g. a Python venv started by the installer or a separate daemon). In this case, omit `docker_compose` from the registry entry and handle startup in `install.sh`. Reference implementations:
- `_setup_openviking` — Python venv at `~/.openviking/venv`, idempotent with PID-based skip, supports `--reinstall-openviking`
- `_setup_jina_embeddings` — OS-aware setup (Mac → pip `infinity-emb`, Linux+Docker → HuggingFace TEI image, Linux → pip fallback), port-based skip, supports `--reinstall-jina`

### Step 4: Test the install

```bash
./install.sh --dry-run
```

Confirm the new MCP appears in the dry-run output with the correct command and arguments.

```bash
./install.sh
```

Verify the MCP is registered:
- Claude Code: `claude mcp list`
- opencode: check `~/.config/opencode/config.json`

### Step 5: Update the docs

Add the new MCP to the "MCP Servers" table in both `README.md` and `CLAUDE.md`.

---

## Updating or Removing an MCP

MCPs that are already installed are skipped by the installer. To force a re-install after changing an entry:

> **Note — process-managed MCPs (OpenViking + Jina):** The installer automatically skips setup if the server is already running. Stale PIDs and occupied ports are detected and handled.
>
> To force a clean reinstall:
> ```bash
> ./install.sh --reinstall-openviking   # wipe ~/.openviking/venv, kill server, regenerate ov.conf
> ./install.sh --reinstall-jina         # wipe ~/.openviking/jina-venv (or stop Docker container), restart Jina
> ./install.sh --reinstall-openviking --reinstall-jina   # both at once
> ```

**Claude Code:**
```bash
claude mcp remove my-mcp
./install.sh
```

**opencode:**

Remove the entry from `~/.config/opencode/config.json` manually, then run `./install.sh`.

To remove a deprecated MCP from the framework entirely, delete its entry from `mcps/registry.json` and document the removal in the changelog.
