# Hooks

Hooks are safety and quality guards that run automatically on every tool call — no configuration needed. They work identically in Claude Code (shell scripts in `claude-code/`) and opencode (JS plugin modules in `opencode/`).

Install them by running `../install.sh` from the repo root.

---

## Hook Catalog

10 hooks covering secret protection, destructive command blocking, large-file confirmation, and lint/format/test-on-save — plus an optional `graphify-*` set for projects using the `graphify` skill.

Full catalog with triggers and behavior: [`docs/reference/hooks.md`](../docs/reference/hooks.md)

---

## Adding a New Hook

1. Create `hooks/claude-code/<hook-name>.sh` and `hooks/opencode/<hook-name>.js`
2. Register the JS module in `hooks/opencode/devexp-plugin.js`
3. Add an entry to `hooks/registry.json`
4. `chmod +x hooks/claude-code/<hook-name>.sh` and run `./install.sh`

Full guide: [`docs/development/hook-authoring-guide.md`](../docs/development/hook-authoring-guide.md)
