# MCP Servers

MCP (Model Context Protocol) servers extend Claude with additional tool capabilities. devexp manages a registry of curated MCP servers and installs them alongside agents, skills, and hooks.

Install them by running `../install.sh` from the repo root.

---

## MCP Catalog

| MCP | Transport | Description |
|-----|-----------|-------------|
| **context7** | stdio | Up-to-date library documentation and code examples for any package — fetched at query time, not from training data. |
| **ui-inspector** | stdio | UI/UX inspection via headless Chromium — screenshot, interact (click/type/scroll), ARIA accessibility tree, computed CSS, axe-core a11y audit, page metrics. **Ships separately** — [mcp-ui-inspector](https://github.com/alexandrocuma/mcp-ui-inspector); set `UI_INSPECTOR_DIR` to its clone. |

Full registry format and details: [`docs/reference/mcps.md`](../docs/reference/mcps.md)

---

## Configuration

```bash
cp mcps/.env.example mcps/.env
# edit mcps/.env and fill in your keys
./install.sh
```

`mcps/.env` is gitignored — never commit real secrets. The installer only registers MCPs (a command or a URL); it never starts servers, so an HTTP/SSE MCP's server must already be running. See [`docs/development/mcp-guide.md`](../docs/development/mcp-guide.md).

---

## Adding a New MCP

1. Add an entry to `mcps/registry.json`
2. If it needs secrets, add key names to `required_env`, set `setup_instructions`, document keys in `mcps/.env.example`
3. Run `./install.sh --mcps-only` to register it (or `--dry-run` to preview)

Full guide: [`docs/reference/mcps.md`](../docs/reference/mcps.md)
