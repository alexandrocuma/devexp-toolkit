# Hooks

Hooks run automatically on matching tool calls — no configuration needed. Each hook has a Claude Code implementation (shell script in `claude-code/`) and an opencode implementation (JS module in `opencode/`, composed by `opencode/devexp-plugin.js`).

Install them by running `./install.sh` from the repo root. The installer registers the **Claude Code** scripts in `~/.claude/settings.json` and installs the **opencode** plugin into `~/.config/opencode/plugins/` (the entry `devexp.js` plus `devexp/` with the selected modules and `hooks.json`).

---

## Hook Catalog

10 hooks in `registry.json`. 7 are enabled by default: 4 safety guards (secret reads, secrets in writes, destructive commands, large-file overwrites) and 3 advisory lint/format/test-on-save hooks that never block. The other 3 are an opt-in `graphify-*` set (`"enabled": false`) for projects using the `graphify` skill.

Full catalog with triggers and behavior: [`docs/reference/hooks.md`](../docs/reference/hooks.md)

---

## Kimi Code CLI

Kimi runs the same `claude-code/` guard scripts, through `kimi/adapter.sh`. A Kimi `[[hooks]]` entry's whole `command` is:

```
bash '<hooks-dir>/kimi/adapter.sh' '<hooks-dir>/claude-code/<hook-name>.sh'
```

(both paths absolute and single-quoted — Kimi spawns the command with `shell: true` and passes it no arguments of its own). The adapter translates Kimi's camelCase envelope into the snake_case one the guards read, and turns everything Kimi would otherwise read as an allow — a translation failure, a missing guard, an unknown guard exit, an `ask` verdict — into exit 2. Which hooks are selected for Kimi, and why the rest are not, is the `kimi` block in `registry.json`.

`<hooks-dir>` is `$KIMI_CODE_HOME/hooks`, **not this directory**: the install copies `kimi/adapter.sh`, the selected guards and `claude-code/scan-budget.sh` there and registers the copies. Kimi reads a command it cannot run as an allow, so a checkout that moves or is deleted must not be able to disarm the guards — and `scan-budget.sh` has to travel with them because each guard sources it from its own directory.

---

## Adding a New Hook

1. Create `hooks/claude-code/<hook-name>.sh` and `hooks/opencode/<hook-name>.js`
2. Add an entry to `hooks/registry.json`, including the opencode mapping (`module`, `export`, `fail_closed` for guards)
3. Add mirrored `<hook-name>.test.sh` / `<hook-name>.test.js` tests (and a `check` line in `claude-code/fail-closed.test.sh`), then update the hook catalog and counts
4. `chmod +x hooks/claude-code/<hook-name>.sh` and run `./install.sh`

Step-by-step recipe: [`docs/guides/workflows.md#add-a-hook`](../docs/guides/workflows.md#add-a-hook) · Full guide: [`docs/development/hook-authoring-guide.md`](../docs/development/hook-authoring-guide.md)
