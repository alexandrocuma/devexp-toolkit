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
| `args` | array | Conditional | Arguments passed to `command`. Required for stdio MCPs. `${VAR}` placeholders are substituted from `mcps/.env` / the shell at install time. |
| `scope` | string | No | `"user"` (default) — installed at the user level, available in all projects. Use `"project"` to scope to a specific project. |
| `env` | object | No | Environment variables passed to a stdio MCP server. Values can be empty strings — they'll be resolved from `mcps/.env` or the shell at install time. |
| `headers` | object | No | HTTP headers for `http`/`sse` MCPs (e.g. an auth token). `${VAR}` placeholders in values are substituted at install time. Passed as `-H "Key: Value"` to `claude mcp add`, or as `headers` in the opencode config. |
| `required_env` | array | No | Environment variable names that must be present (non-empty) for the MCP to install. If any are missing, the MCP is skipped with a `[REQUIRED]` warning. |
| `setup_instructions` | string | No | Human-readable guidance printed after the `[REQUIRED]` warning when `required_env` keys are missing (Claude Code install only — the opencode installer prints the warning without it). |

Fields are defined by the `MCP` struct in `cli/internal/mcp/types.go`; the installer ignores any other key.

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
  "headers": {
    "Authorization": "Bearer ${MY_MCP_API_KEY}"
  },
  "scope": "user",
  "env": {},
  "required_env": ["MY_MCP_API_KEY"],
  "setup_instructions": "Set MY_MCP_API_KEY in mcps/.env and re-run ./install.sh"
}
```

The installer only registers the URL — it does not start the server. Whatever serves that URL must be running (started by you, or by the MCP itself) by the time Claude connects. There is no `docker_compose` field: installer-managed Docker services went away with the Python install scripts (`scripts/docker_services.py`, removed in `61f6c9f`).

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

HTTP/SSE MCPs are written as `"type": "remote"` with `url` (and `headers`, if any). The installer handles this automatically: an entry identical to the existing one is skipped; an entry that differs from what's in the config is overwritten in place (`cli/internal/mcp/opencode.go`).

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

Set `"transport": "http"` (or `"sse"` for legacy SSE-only servers) and `"url"` in the registry entry. Add a `"setup_instructions"` field to explain what the user needs to configure. The installer does not start servers, so `setup_instructions` should say how to get the server running.

**Self-contained process** — the MCP launches and manages its own backend process on demand. Reference implementation: [`mcp-ui-inspector`](https://github.com/alexandrocuma/mcp-ui-inspector) — a Node MCP that spins up a headless Chromium browser when first called and shuts it down on SIGTERM. No installer-managed venvs, Docker, or separate daemon. It ships as its own repo rather than vendored here: clone it, run `./setup.sh` once, and point `UI_INSPECTOR_DIR` at it. A distribution repo should distribute, not host a build.

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

Add a row for the new MCP to the MCP catalog tables in `mcps/README.md` and `docs/reference/mcps.md`, and update the MCP list in the `README.md` repo tree. `CLAUDE.md` has no MCP table — it only indexes these docs. The full checklist is in [workflows — Add an MCP server](../guides/workflows.md#add-an-mcp-server).

---

## Updating or Removing an MCP

For Claude Code, MCPs that are already installed are skipped by the installer (`cli/internal/mcp/claude.go`). For opencode, a changed entry is rewritten on the next `./install.sh`. To force a re-install after changing an entry:

> **Note — forcing an MCP config refresh:** MCPs already registered with Claude Code or opencode are skipped by default. To remove and re-add every registry MCP (forces a fresh config write without touching unrelated entries):
> ```bash
> ./install.sh --reinstall-mcps
> ```

**Claude Code:**
```bash
claude mcp remove my-mcp
./install.sh
```

**opencode:**

Remove the entry from `~/.config/opencode/config.json` manually, then run `./install.sh`.

To remove a deprecated MCP from the framework entirely, delete its entry from `mcps/registry.json` and document the removal in the changelog.
