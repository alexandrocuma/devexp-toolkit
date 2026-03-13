# Team Distribution

This guide explains how to customise and distribute the devexp framework for your organisation — selecting which agents, skills, hooks, and MCP servers your team installs, without forking the entire repo.

---

## How it works

The installer reads `devexp.config.json` from the repo root before installing anything. You can use it to:

- **Disable** agents, skills, or hooks your team doesn't use
- **Set a default model** for all agents (without requiring every engineer to pass `--model`)
- **Add org-internal MCP servers** that are installed alongside the base devexp MCPs

The config file is committed to the repo — it travels with your fork and applies automatically every time someone runs `./install.sh`.

---

## Setup for your org

1. Fork or clone the devexp repo into your org's GitHub account
2. Edit `devexp.config.json` in the repo root
3. Add any org-specific MCP secrets to `mcps/.env` (this file is gitignored — never commit it)
4. Point your engineers at your fork: `git clone https://github.com/your-org/devexp && ./install.sh`

---

## `devexp.config.json` reference

```json
{
  "$schema": "./devexp.config.schema.json",

  "model": "sonnet",

  "agents": {
    "disabled": ["scaffold", "orchestrator"]
  },

  "skills": {
    "disabled": ["gen-claude-md", "release"]
  },

  "hooks": {
    "disabled": ["lint-on-save"]
  },

  "mcps": [
    {
      "name": "our-internal-docs",
      "description": "Internal documentation search",
      "command": "npx",
      "args": ["-y", "@our-org/docs-mcp"],
      "scope": "user",
      "env": {},
      "required_env": ["ORG_DOCS_TOKEN"]
    }
  ]
}
```

### Fields

| Field | Type | Description |
|-------|------|-------------|
| `model` | `string \| null` | Default model for all agents. `null` = inherit CLI default. Overridden by `--model` flag. |
| `agents.disabled` | `string[]` | Agent names (without `.md`) to skip. |
| `skills.disabled` | `string[]` | Skill names (directory name under `skills/`) to skip. |
| `hooks.disabled` | `string[]` | Hook names from `hooks/registry.json` to skip. |
| `mcps` | `object[]` | Additional MCP servers. Same schema as entries in `mcps/registry.json`. |

### Precedence

`--model` CLI flag > `devexp.config.json "model"` > CLI's own default model.

---

## Adding org-internal MCPs

MCPs that need API keys use `mcps/.env` (gitignored):

```bash
cp mcps/.env.example mcps/.env
# add your keys to mcps/.env
./install.sh
```

Declare the MCP in `devexp.config.json` with the key name in `required_env`:

```json
{
  "mcps": [
    {
      "name": "our-internal-mcp",
      "command": "npx",
      "args": ["-y", "@our-org/internal-mcp"],
      "scope": "user",
      "required_env": ["ORG_MCP_TOKEN"]
    }
  ]
}
```

The installer loads `mcps/.env`, resolves `ORG_MCP_TOKEN`, and passes it to the MCP at install time. If the key is missing from both `.env` and the shell environment, the MCP is skipped with a clear warning — the rest of the install continues.

---

## Keeping your fork up to date

Add the upstream devexp repo as a remote:

```bash
git remote add upstream https://github.com/original-org/devexp
git fetch upstream
git merge upstream/main
```

Your `devexp.config.json` customisations will survive the merge as long as you don't modify the same lines the upstream changes. Conflicts are rare since the config is a separate file from the agents and skills.

---

## Schema validation

`devexp.config.schema.json` is a JSON Schema (draft-07) that editors like VS Code use to validate your config file automatically. If you see red underlines in `devexp.config.json`, the schema will tell you what's wrong.
