# Conventions

> Kit doc · Last verified: 2026-09-16 against commit `a86d2c3f6a41a6d033d31afd858ff723d5267dd7`

How code is written in this repo. Every rule cites the files that prove it.

The repo holds two kinds of code, and both have rules:

- **Toolkit assets** — agents, skills, hooks, MCP registry, templates. How to write each one is already covered by the authoring guides: [agents](agent-authoring-guide.md) · [agent structure (Phase 0, chaining, memory)](agent-architecture-reference.md) · [skills](skill-authoring-guide.md) · [hooks](hook-authoring-guide.md) · [MCPs](mcp-guide.md). This doc covers only the rules that apply across many assets.
- **Go CLI** — `cli/` (module `devexp`), the installer.

For where things live, see the [architecture overview](../architecture/overview.md). For how tests are written, see [testing](testing.md). For commands, see [setup](setup.md).

## Naming

| Thing | Convention | Example |
|-------|-----------|---------|
| Agent files | kebab-case `<name>.md`. The filename matches the frontmatter `name:` (true for all 34 agents) | `agents/codebase-navigator.md` |
| Skill directories | kebab-case `skills/<name>/SKILL.md`. The directory matches the frontmatter `name:` (true for all 8 skills) | `skills/release/SKILL.md` |
| Hooks | One basename used in three places: `hooks/claude-code/<name>.sh`, `hooks/opencode/<name>.js`, and `"name"` in `hooks/registry.json` | `secret-guard` in `hooks/registry.json`, `hooks/claude-code/secret-guard.sh`, `hooks/opencode/secret-guard.js` |
| Hook JS exports | lowerCamel form of the hook name, as `export async function <name>(_ctx)` (`ctx` when the module reads it, as the on-save modules do for `ctx.directory`) | `dangerousCmdGuard` in `hooks/opencode/dangerous-cmd-guard.js`; `lintOnSave` in `hooks/opencode/lint-on-save.js` |
| Hook messages | Start with `[devexp <hook-name>]` | `hooks/claude-code/dangerous-cmd-guard.sh`, `hooks/claude-code/format-on-save.sh`, `hooks/opencode/dangerous-cmd-guard.js` |
| Go packages | One lowercase word, the same as the directory. One package per asset kind or concern | `cli/internal/manifest`, `cli/internal/hooks`, `cli/internal/mcp` |
| Go files | lowercase, `snake_case` for more than one word. An asset-kind package puts its logic in `installer.go`. `mcp` splits by target (`claude.go`, `opencode.go`, `types.go`) | `cli/cmd/install_claude.go`, `cli/internal/agents/installer.go`, `cli/internal/skills/installer.go` |
| Go types | Exported PascalCase for a package's API. Unexported lowerCamel inside `cmd` | `manifest.Manifest`, `repo.Source`, `hooks.Registry`; `installOpts`, `claudePaths` in `cli/cmd/` |
| Go functions | Per-target installers are `InstallClaude` / `InstallOpencode`. Readers are `Load` / `Load<Thing>`. In `cmd`, cobra handlers are `run<Name>`, per-target orchestration is `doInstall<Target>`, commands are `<name>Cmd` and flags are `flag<Name>` | `cli/internal/agents/installer.go`, `cli/internal/skills/installer.go`, `cli/internal/mcp/claude.go`; `hooks.LoadRegistry`, `mcp.LoadRegistry`, `manifest.Load`; `runInstall`, `installCmd`, `flagDryRun` in `cli/cmd/install.go` |
| Go tests | `_test.go` in the same package, `Test<Func>` or `Test<Func>_<Scenario>`, with table cases keyed by a sentence. See [testing](testing.md) | `cli/internal/manifest/manifest_test.go`, `TestFindRepoDir_DevexpDirEnv` in `cli/internal/repo/repo_test.go` |
| Script tests | `<hook>.test.sh` / `<hook>.test.js` next to the hook. Installer script tests are `*.test.sh` at the repo root | `hooks/claude-code/secret-guard.test.sh`, `hooks/opencode/secret-guard.test.js`, `uninstall.test.sh` |
| DB tables / collections | N/A — there is no database. The only persisted data is JSON files: registries, config, and the install manifest | — |

## Module Structure

**Go CLI** — commands are thin, and there is one internal package per asset kind:

- `cli/cmd/` holds one file per concern since #97 (`06c45a1`): `install.go` (entry point and option resolution), `install_claude.go` / `install_opencode.go` (per-target orchestration), `wizard.go`, `targets.go`, `registry.go`, `backup.go`, `paths.go`.
- `cli/internal/<kind>/` does the file, JSON and exec work for one kind (`agents`, `skills`, `hooks`, `mcp`) or one concern (`repo`, `assets`, `config`, `manifest`, `ui`).
- **Dependencies flow one way.** `cmd` imports `internal/*`. Internal packages import only `internal/ui`, plus `internal/repo` → `internal/assets` (from the imports in `cli/internal/*/*.go`). Don't add imports from one internal package to another.
- **Installers take explicit paths and return what they installed.** For example, `agents.InstallClaude(srcDir, targetDir, model string, disabled []string, dryRun bool) ([]string, error)` returns names that the caller diffs against the manifest. See `cli/internal/agents/installer.go` and `cli/internal/skills/installer.go`. `hooks.InstallClaude` takes `settingsPath` instead of reading `$HOME` (`cli/internal/hooks/installer.go`).
- **Copy by registry list, never by glob.** `hooks.InstallOpencode` copies exactly the files the selected registry entries name, plus the entry, `utils.js` and `package.json`, so `*.test.js` never ships (`cli/internal/hooks/opencode.go`).
- **Keep decisions apart from I/O.** A pure function makes the decision, and a separate function prints, prompts or execs. The pure function takes whatever varies, such as the clock or the answers, as parameters. See `cli/cmd/targets.go` (`selectTargets` is pure, `announceTargets` handles I/O, `detectTargets` wraps both), `cli/cmd/paths.go` (`claudeTargetPaths(home, now)`) and `cli/internal/ui/prompts.go` (the pure `buildMultiSelectDisplay` / `applyMultiSelectChoice` behind the TTY-bound `MultiSelect`).

```go
// It is deliberately pure: no PATH lookup, no printing, no prompt. Both the
// flag path (detectTargets) and the interactive path (runWizard) resolved
// targets with their own copy of this switch, so the rule lived in two places
// and could drift.
func selectTargets(hasClaude, hasOpencode bool, choice string) (claude, opencode bool, err error) {
```
— `cli/cmd/targets.go`

- **Test seams are passed-in functions, not interfaces.** The repo has no interfaces and no DI container. For example, `removeStale(home, dir string, old, installed []string, shape staleShape, removeFn func(root *os.Root, name string) error, dryRun bool) (kept []string)` in `cli/cmd/backup.go` receives `(*os.Root).Remove` or `(*os.Root).RemoveAll`, and `var userCacheDir = os.UserCacheDir` in `cli/internal/repo/repo.go` is a package variable that tests swap.
- **Section banners** such as `// ── Title ───…` split longer Go files (`cli/cmd/install.go`, `cli/cmd/wizard.go`, `cli/internal/repo/repo.go`, `cli/internal/ui/prompts.go`). Shell files use the same style with `#` (`hooks/claude-code/dangerous-cmd-guard.sh`, `hooks/claude-code/fail-closed.test.sh`, `uninstall.sh`).

**Assets** — each kind has its own layout, described in its authoring guide. The cross-cutting rules are:

- **Author in Claude Code format only.** opencode copies are generated at install time: agent frontmatter is rewritten by `transformForOpencode` (`cli/internal/agents/installer.go`), and skills are reduced to `SKILL.md` saved as `commands/<name>.md` (`cli/internal/skills/installer.go`, `InstallOpencode`). Anything a skill needs to work must therefore be in `SKILL.md`, because `references/` files reach Claude Code only (`docs/reference/skills.md`, "Adding a New Skill"). opencode-only agents go in `agents/opencode/` (`InstallOpencodeExclusive`).
- **Agents are run by reading the file, never spawned by name.** A custom agent name is not a valid `subagent_type`. Skills and agents read `~/.claude/agents/<name>.md` and follow it in the current context. See `skills/devxp/SKILL.md` ("read `~/.claude/agents/gen-docs.md` … and follow their instructions"), `agents/README.md` ("Critical"), and `CLAUDE.md`.
- **A hook is three touch points:** the `.sh` script, the `.js` module, and the `hooks/registry.json` entry, including the `opencode` mapping (`module`, `export`, and `fail_closed` for security guards). The opencode entry (`hooks/opencode/devexp-plugin.js`) composes from the installed selection, so it is never edited per hook. The step list is in [workflows](../guides/workflows.md#add-a-hook).
- **The source repo is the only place to edit.** `~/.claude/agents/`, `~/.claude/skills/` and `~/.config/opencode/` hold copies that the next install overwrites (`agents.InstallClaude` writes with `os.WriteFile`; `skills.CopyDir` copies the whole directory).
- **Documentation:** `CLAUDE.md` holds only an index. Everything else goes in `docs/`, and every docs folder has a `README.md` index with a Status column. See [docs-architecture](../guides/docs-architecture.md) and `docs/development/README.md`.

## Error Handling

**Go CLI:**

- **Wrap errors with context at command boundaries** using `fmt.Errorf("<what>: %w", err)`. See `cli/cmd/install.go` (`claude install:`, `opencode install:`), `cli/cmd/registry.go` (`load MCP registry:`) and `cli/internal/repo/repo.go` (`Resolve`). Errors come back up to cobra, and `Execute` prints them to stderr and exits 1 (`cli/cmd/root.go`). Nothing calls `ui.Error`.

```go
	if installClaude {
		if err := doInstallClaude(opts); err != nil {
			return fmt.Errorf("claude install: %w", err)
		}
	}
```
— `cli/cmd/install.go`

- **Internal installers return fatal I/O errors unwrapped** and give back the partial result. Examples are `return installed, err` in `cli/internal/agents/installer.go` and `cli/internal/skills/installer.go`.
- **Problems with a single item warn and continue; they never abort the install.** A failed opencode transform does `ui.Warn` then `continue` (`cli/internal/agents/installer.go`). A failed `claude mcp add` warns and returns nil, and a missing `required_env` prints a `[REQUIRED]` notice and returns nil (`cli/internal/mcp/claude.go`). A bad hooks registry only warns (`cli/cmd/install_claude.go`), and so do bad `mcps` entries in config (`cli/cmd/registry.go`).
- **Loading state never blocks an install.** A missing manifest counts as empty. An unreadable or malformed one also counts as empty, with a warning, and no agent or skill is treated as stale on that run (opencode plugin files are also recognised on disk) (`manifest.Load` in `cli/internal/manifest/manifest.go`, `loadOldManifest` in `cli/cmd/backup.go`). Malformed `settings.json` or opencode `config.json` is read with `json.Unmarshal(...) //nolint:errcheck` and treated as empty (`cli/internal/hooks/installer.go`, `cli/internal/mcp/opencode.go`). A missing `devexp.config.json` warns and uses the defaults (`cli/cmd/install.go`). Backup failures are ignored (`cli/cmd/backup.go`). The reason is recorded in the doc comments:

```go
// A file that exists but can't be read or parsed returns an empty Manifest
// together with the error, so the caller can warn and still install. The
// manifest is empty rather than whatever decoded before the error (a type
// mismatch leaves earlier fields filled in): an untrustworthy manifest must
// never mark files as stale, and an empty one marks none.
```
— `cli/internal/manifest/manifest.go`

**Hooks:**

- **To block, write the reason to stderr and `exit 2` (shell) or `throw` (JS). To allow, `exit 0` with no output.** See the header of `hooks/claude-code/secret-guard.sh` ("To block: print reason to stderr, exit 2"), `hooks/claude-code/dangerous-cmd-guard.sh`, and `throw new Error(...)` in `hooks/opencode/dangerous-cmd-guard.js` and `hooks/opencode/secret-in-write-guard.js`. The response types, including `ask`, are listed in the [hook guide](hook-authoring-guide.md#response-types).
- **Security guards fail closed; advisory hooks fail open but say so.** If a hook can't read its input, `secret-guard`, `secret-in-write-guard` and `dangerous-cmd-guard` print `internal error` and `exit 2`. `large-file-guard`, `format-on-save`, `lint-on-save` and `test-on-save` print `internal error` and `exit 0`. `hooks/claude-code/fail-closed.test.sh` enforces this, and it came from #92 (`2afc3af`). Never put `2>/dev/null || echo ""` on an extraction step, because an empty result silently allows the operation. The same three guards also scan under a wall-clock budget and block when it runs out, because a timed-out command hook does *not* block the tool call — see [reference/hooks → The Scan Budget](../reference/hooks.md#the-scan-budget).

```bash
command=$(echo "$input" | python3 -c \
    "import sys,json; d=json.load(sys.stdin); print(d.get('tool_input',{}).get('command',''))") || {
    echo "[devexp dangerous-cmd-guard] internal error -- the guard could not read its input, so it did not run. Blocking to be safe; the interpreter's error is above." >&2
    exit 2
}
```
— `hooks/claude-code/dangerous-cmd-guard.sh`

- **Post-edit hooks never block.** JS `file.edited` handlers are "Advisory only — never throws" (`hooks/opencode/lint-on-save.js`). A guard that fails in opencode stops the chain, because handlers run one after another and the first throw wins (`hooks/opencode/devexp-plugin.js`). opencode modules never spawn synchronously — opencode runs plugins in its server process — so they `await` `runCommand` / `which` / `runLinter` from `hooks/opencode/utils.js`.

## Logging & Observability

There is no logger and no structured logging. All user-facing output goes to stdout through `cli/internal/ui/output.go`: `Info` / `Success` / `Warn` add a colored `[devexp]` prefix, and `Added` / `Removed` / `Updated` / `Skipped` / `DryRun` / `Required` print item lines. `ui.Error` is the only helper that writes to stderr. Dry-run paths report through `ui.DryRun` instead of writing anything (`cli/internal/agents/installer.go`, `cli/cmd/backup.go`, `cli/internal/hooks/installer.go`).

Hooks report only on stderr, prefixed `[devexp <hook-name>]`, and print nothing when they allow an operation (`hooks/claude-code/fail-closed.test.sh` asserts that a clean envelope "exits 0 quietly").

## Configuration Access

- **Team config** (`devexp.config.json`, checked against `devexp.config.schema.json`) is read only by `config.Load`, which uses viper (`cli/internal/config/config.go`). viper is not used anywhere else. The parsed values go to installers inside `installOpts` (`cli/cmd/install.go`), never as globals.
- **Environment:** only `cmd` and `repo` read the process environment. That covers `HOME` (`cli/cmd/install_claude.go`, `cli/cmd/install_opencode.go`, passed into the pure path builders in `cli/cmd/paths.go`) and `DEVEXP_DIR` (`cli/internal/repo/repo.go`). Internal installers get paths and env maps as arguments and never call `os.Getenv`.
- **MCP secrets** are never written into the registry. The registry uses `${VAR}` placeholders plus `required_env` (`mcps/registry.json`). Values come from `mcps/.env`, which is gitignored (`.gitignore`) and has a template at `mcps/.env.example`. `buildEnv` merges the OS env, `DEVEXP_DIR` and dotenv, with dotenv winning (`cli/cmd/registry.go`), and `resolveStr` substitutes the values (`cli/internal/mcp/claude.go`). See the [MCP guide](mcp-guide.md#secrets-with-mcpsenv).
- **Hooks** read only the JSON envelope on stdin (`hooks/claude-code/secret-guard.sh`, `hooks/claude-code/dangerous-cmd-guard.sh`), or `(input, output)` in opencode (`hooks/opencode/dangerous-cmd-guard.js`).

## Style

Nothing enforces style: no linter or formatter config exists, and CI runs only tests and a govulncheck scan (`.github/workflows/ci.yml`). The Go code is gofmt-clean today (`gofmt -l cli` prints nothing for tracked files at this commit), and #97 records "go vet and gofmt clean" as a manual check (`06c45a1`). `//nolint:errcheck` marks errors that are ignored on purpose (`cli/cmd/backup.go`, `cli/internal/hooks/installer.go`, `cli/internal/mcp/opencode.go`).

Rules that no tool enforces:

- **Doc comments explain why,** including what went wrong before and why the current shape was chosen. See `cli/cmd/targets.go`, `cli/cmd/paths.go`, `cli/internal/repo/repo.go` (`userCacheDir`), `cli/internal/hooks/installer.go` (`isForeignDevexpHook`) and `cli/internal/skills/installer.go` (`stripFrontMatterName`). Issue numbers go in hook headers (`#87`, `#81` in `hooks/claude-code/secret-guard.sh` and `hooks/opencode/secret-guard.js`) and in commit bodies. Go source doesn't cite issue numbers.
- **Unexported by default.** Only a package's API is exported, and all of `cli/cmd` is unexported apart from `Execute` (`cli/cmd/root.go`).
- **Keep Go files small.** #97 split `cli/cmd/install.go` from 762 lines to 193 across cohesive siblings. No non-test Go file is over 270 lines; the largest is `cli/internal/hooks/installer.go` at 269.
- **Shell scripts start with `set -euo pipefail`** (`install.sh`, `scripts/stage-assets.sh`, every `hooks/claude-code/*.sh` hook). Test scripts use `set -uo pipefail` so a failing case is counted instead of aborting the run (`hooks/claude-code/fail-closed.test.sh`, `hooks/claude-code/dangerous-cmd-guard.test.sh`).
- **Shell hook headers** follow the same pattern: `# devexp hook: <name>`, then `# Event: <event> | Matcher: <matcher>`, then a short rationale (all 10 of `hooks/claude-code/*.sh`).
- **Hooks use only standard libraries.** Shell hooks import only python3 stdlib modules (`json`, `sys`, `os`, `re`, `shlex`, `shutil`, `subprocess`, `glob`). JS hooks import only `child_process`, `fs` and `path` plus local modules, and `hooks/opencode/package.json` is just `{ "type": "module" }`. The installer has no npm or pip step (`cli/cmd/install_claude.go`).
- **Fix the cause.** When a bug has shaped how assets are written, the workaround guidance is removed together with the fix (`f41c79f`: "a workaround outliving its bug is a trap for the next author").

## Commits & Branches

- **Conventional commits** with lowercase, outcome-focused subjects: `feat:`, `fix:`, `refactor:`, `test:`, `docs:`, `chore:`, sometimes with a scope (`test(cmd):` in `bdf026f`). Every commit since #84 (`1cce527`) follows this. The older `#NN: subject (#PR)` ticket-prefix style (for example `ace04dc`) is no longer used. See `git log --oneline`.
- **Squash-merge PRs** so the subject ends with `(#PR)`, as in `f06b67e`, `06c45a1` and `bdf026f`. Before #42, merge commits (`Merge pull request #41 …`) were used.
- **Bodies** explain why and what was observed, record how the change was verified (test counts, dry-run diffs), and end with `Closes #NN` and a `Co-Authored-By:` trailer (`2afc3af`, `f41c79f`, `06c45a1`).
- **Every change adds a `CHANGELOG.md` entry under `[Unreleased]`** in the same commit (`f06b67e`, `2afc3af`, `06c45a1`, `bdf026f`, `b7f81b9`). Release commits are `chore: release vX.Y.Z` (`d5f6943`, `f88f123`, `879a5c8`). See the [release guide](../guides/release.md).
- **The prefix decides release notes.** goreleaser leaves `docs:`, `test:` and `chore:` commits out of GitHub release notes (`.goreleaser.yaml`, `changelog.filters`), so user-visible changes need `feat:`, `fix:` or `refactor:`.
- **Branches** are `<type>/<topic>` (see `git branch -a`: `feat/release-targets`, `fix/hooks-fail-closed`, `refactor/split-install-go`, `test/cover-cmd`, `improve/release-phase`). Delivery uses one worktree per ticket, described in [worktree-per-ticket](../guides/worktree-per-ticket.md).

## Inconsistencies

- `[INCONSISTENT — output through ui helpers (cli/cmd/backup.go, cli/cmd/targets.go, cli/internal/agents/installer.go) vs inline fmt.Printf with raw ANSI codes (cli/cmd/install.go, cli/internal/hooks/installer.go, cli/internal/mcp/opencode.go)]`. New code should use `cli/internal/ui`, as the files written after #97 do.
- `[INCONSISTENT — documented exported API with package doc comments (cli/internal/manifest/manifest.go, cli/internal/repo/repo.go, cli/internal/assets/assets.go) vs no doc comments (cli/internal/config/config.go, cli/internal/mcp/types.go, cli/internal/mcp/claude.go, hooks.LoadRegistry and hooks.InstallClaude in cli/internal/hooks/installer.go)]`. New code should follow the documented style.
- `[INCONSISTENT — one test file per source file (cli/internal/manifest/manifest_test.go, cli/internal/config/dotenv_test.go, cli/internal/hooks/installer_test.go) vs one test file per package (cli/cmd/install_test.go covers targets.go, paths.go, registry.go, backup.go and wizard.go; cli/internal/mcp/mcp_test.go; cli/internal/ui/ui_test.go)]`. The repo doesn't show which one to prefer; see [testing](testing.md).
- `[INCONSISTENT — branch naming <type>/<ticket-id> in docs/guides/worktree-per-ticket.md ("Naming scheme") vs <type>/<topic-slug> on the remote (fix/hooks-fail-closed, refactor/split-install-go), with a few older <type>/<issue>-<slug> branches (docs/36-worktree-convention, test/23-cli-test-coverage)]`
- `[INCONSISTENT — docs/development/agent-architecture-reference.md "Agent File Checklist" requires color: and a ## Chaining section vs agents without color: (gen-docs, gen-indexer, update-docs, update-indexer) and agents without ## Chaining (dev-agent, gen-docs, gen-indexer, grooming-agent, update-docs, update-indexer)]`. Either update the checklist or the agents.
- `[INCONSISTENT — hooks with mirrored sh+js unit tests (secret-guard, secret-in-write-guard, dangerous-cmd-guard) vs hooks covered only by the shell fail-closed test (large-file-guard, lint-on-save, format-on-save, test-on-save) or by nothing (the three graphify-* hooks)]`. When you change a hook's decision logic, add mirrored tests; see [testing](testing.md).
