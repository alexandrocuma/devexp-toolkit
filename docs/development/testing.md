# Testing

> Kit doc · Last verified: 2026-09-16 against commit `eaebd52de9323825e6f3fdf286253a89ea89a7a5`

Where tests live, how they're written and run, and what must pass before a commit. Commands for everything else (build, install, env vars) are in [`setup.md`](setup.md); code style in [`conventions.md`](conventions.md).

## Test Types

CI (`.github/workflows/ci.yml`) runs on every pull request and every push to `main`, in two jobs: `test` (Go) and `hooks` (the three script suites). All four suites below were green at this commit.

| Type | Framework | Location | Run |
|------|-----------|----------|-----|
| Unit — Go CLI | Go stdlib `testing` only; no assertion or mock library in `cli/go.mod` | `cli/**/<file>_test.go`, next to the code, same package | `./scripts/stage-assets.sh && (cd cli && go test ./... -race -cover)` — `ci.yml:17-21` |
| Hook behaviour — Claude Code | plain bash script, `pass`/`fail` counters | `hooks/claude-code/<hook>.test.sh` | `for f in hooks/claude-code/*.test.sh; do bash "$f" \|\| exit 1; done` — `ci.yml:30-34` |
| Hook behaviour — opencode | plain `node` ESM script, no framework (Node 22 in CI) | `hooks/opencode/<hook>.test.js` | `for f in hooks/opencode/*.test.js; do node "$f" \|\| exit 1; done` — `ci.yml:35-39` |
| Installer script | plain bash script | repo root `*.test.sh` (`uninstall.test.sh`, `remote-install.test.sh`) | `for f in ./*.test.sh; do bash "$f" \|\| exit 1; done` — `ci.yml:40-45` |
| Integration | N/A — no separate suite. Go tests already do real file I/O inside `t.TempDir()`, and each hook `.test.sh` executes the real hook script against a real tool-call JSON envelope | — |
| E2E | N/A — no automated end-to-end install test. `runInstall`, `doInstallClaude`, `doInstallOpencode` and `runWizard` have 0% coverage. Manual check: `./install.sh --dry-run` | — |
| Agents / skills (Markdown) | N/A — no automated validation. Edit → `./install.sh` → try it in Claude Code or opencode | `docs/development/README.md` (Notes) |

At this commit: 10 Go packages with tests; `dangerous-cmd-guard.test.sh` 25 cases, `fail-closed.test.sh` 10, `secret-guard.test.sh` 37; `dangerous-cmd-guard.test.js` 25, `devexp-plugin.test.js` 130, `secret-guard.test.js` 38; `uninstall.test.sh` 89, `remote-install.test.sh` 24.

Run one test:

```bash
cd cli && go test ./internal/manifest -run 'TestStale/empty_new'   # one Go subtest (spaces in map keys become _)
bash hooks/claude-code/secret-guard.test.sh                        # one hook suite — the "Run:" line in its header
node hooks/opencode/secret-guard.test.js
```

## Writing a Test

- **New test file path:**
  - Go: `<same dir>/<file>_test.go` declaring the **same package**, so unexported functions are testable — e.g. `cli/internal/manifest/manifest_test.go` (`package manifest`), `cli/cmd/install_test.go` (`package cmd`).
  - Hooks: `hooks/claude-code/<hook>.test.sh` **and** a mirrored `hooks/opencode/<hook>.test.js` covering the same cases — e.g. `secret-guard.test.sh` / `secret-guard.test.js`, `dangerous-cmd-guard.test.sh` / `.test.js`. CI picks new files up by glob (`ci.yml:32,37,42`); nothing to register.
- **Reference test to copy:**
  - `cli/internal/manifest/manifest_test.go` — smallest complete example: map-keyed table tests, `t.TempDir()`, `reflect.DeepEqual`, the house error-message format.
  - `cli/internal/hooks/installer_test.go` — the richer one: `t.Helper()` fixture builders (`createScript`, `writeSettingsHooks`, `readHooks`) and comments explaining why each case matters.
  - `hooks/claude-code/secret-guard.test.sh` + `hooks/opencode/secret-guard.test.js` — the mirrored hook pair.
- **Go test shape** (each point seen in 2+ files):
  - Table tests are a **map keyed by a sentence**: `tests := map[string]struct{...}` then `for name, tt := range tests { t.Run(name, ...) }` — see `manifest_test.go`, `hooks/installer_test.go`, `repo/repo_test.go` (`TestIsRepoDir`), `cmd/install_test.go` (`TestCommandExists`). Scenario tests use inline `t.Run("sentence", ...)` — see `cmd/install_test.go` (`TestDetectTargets`), `repo/repo_test.go` (`TestExtractEmbedded`).
  - Failure messages: `Func() error = %v` and `Func() = %v, want %v` — see `manifest_test.go`, `cmd/install_test.go`.
  - No test calls `t.Parallel()` (none in `cli/`); `t.Setenv`/`t.Chdir` panic in parallel tests, so keep it that way where they're used.
- **Fixtures / factories:** built per test by `t.Helper()` functions that write files into a `t.TempDir()` — `createScript`/`writeSettingsHooks` (`internal/hooks/installer_test.go`), `writeAgentFiles` (`internal/agents/installer_test.go`), `writeSkillDir` (`internal/skills/installer_test.go`), `writeRegistry` (`cmd/install_test.go:774`). There is no shared test-helper package: copy the helper you need (`captureStdout` exists in both `cmd/install_test.go:615` and `internal/mcp/mcp_test.go:170`; `captureOutput` in `internal/ui/ui_test.go:131`).
- **Mocking / fakes:** no mock library. Isolate with real resources the test owns:
  - Files → `t.TempDir()`; never the real `$HOME` or user cache. Installers take explicit src/target/settings paths, and path builders take `home`/`now` as arguments (`claudeTargetPaths(home, now)` in `cli/cmd/paths.go`) so they can be asserted without touching the machine.
  - Env, `PATH` and cwd → `t.Setenv` / `t.Chdir`, which restore automatically — see `repo/repo_test.go:12,73`, `cmd/install_test.go:95`. A fake CLI is an executable stub on a `PATH` that contains nothing else: `fakeCLI(t, "claude")` (`cmd/install_test.go:700-710`).
  - Logic behind a TTY prompt can't be driven by a test — extract it into a pure function and test that: `selectTargets` (`cli/cmd/targets.go:20`, tested by `TestSelectTargets`), `buildMultiSelectDisplay` (tested in `internal/ui/ui_test.go`).
  - Single-instance seams, reuse when they fit `[verify — inferred from single example]`: static fixture files under `testdata/` (`cli/internal/agents/testdata/`); `testing/fstest.MapFS` standing in for an `fs.FS` (`repo/repo_test.go:98`); a package-level function variable swapped with a `t.Cleanup` restore (`var userCacheDir` in `cli/internal/repo/repo.go:83`, swapped by `withTempCache` in `repo_test.go`); `blockedDir` for forcing a `MkdirAll` failure (`cmd/install_test.go:839`).
- **External services in tests:** none are called. The `claude mcp add/list/remove` exec paths are untested (only `TestAddClaude_NonExec` covers the non-exec branch, `internal/mcp/mcp_test.go:201`); CLI presence is faked with `fakeCLI`. No test uses the network.
- **Hook tests** (`secret-guard.test.sh`, `dangerous-cmd-guard.test.sh`, and their `.js` mirrors):
  - Shell: header `# Run: bash hooks/claude-code/<hook>.test.sh`, `set -uo pipefail`, a `run` helper that builds the real PreToolUse JSON envelope with `python3` and pipes it into the hook, `expect block|allow …` lines (exit 2 = block, exit 0 = allow), then `printf '%d passed, %d failed'` and a non-zero exit on any failure.
  - JS: header `mirrors hooks/claude-code/<hook>.test.sh`, import the module's exported pure predicate (`isSecretFile`/`secretInCommand` from `secret-guard.js`, `BLOCK_PATTERNS` from `dangerous-cmd-guard.js`), loop over block/allow arrays, print `N passed, M failed`, `process.exit(fail === 0 ? 0 : 1)`.
  - The opencode entry has its own suite, `hooks/opencode/devexp-plugin.test.js` (no shell mirror): it builds the installed layout (`plugins/devexp.js` + `plugins/devexp/hooks.json` and modules) in a `mkdtempSync` dir, writes probe modules that record calls on `globalThis`, and imports the entry with a `?n=` cache-busting query. It covers selection, failure isolation, fail-closed stubs (including the `fail_closed` spelling, security-guard names and malformed entries), the `hooks.json` key contract (parsed from the authoring guide's example), module-name refusal (`node:` builtins, encoded dot segments), `event` → `file.edited` dispatch (polled with `waitFor`, since handlers run after `event` returns), a slow fake local `eslint` proving the event loop stays free, `runCommand` timeouts, the on-save handlers' edit path, and registry consistency (`opencode.module`/`export` for every hook).
  - Failure mode: add every new Claude Code hook to `hooks/claude-code/fail-closed.test.sh` — `check <hook> 2 guard` for security guards (must fail **closed**), `check <hook> 0 advisory` for advisory hooks (may fail open, but must print `internal error`).
- **Installer script tests:** never run `uninstall.sh` against your real `HOME` — it deletes real files. `uninstall.test.sh` does three things:
  - extracts the embedded python heredocs with `awk` and runs them against fixture `settings.json` / `config.json` content in a `mktemp -d` dir;
  - runs `uninstall.sh` itself with `--yes`, stdin closed, `env -i` and a temp `HOME`, with stub `devexp` binaries that log their calls (`make_stub`, `make_old_stub`, `run_uninstall`), to cover detection, binary lookup, step order, the crash regressions and the HOME refusal (unset, empty and relative `HOME` from a temp cwd holding a dotfiles-style tree, which must stay byte-identical — `tree_sum`);
  - when `go` is on `PATH` and the assets are staged, builds the real binary and does an install → uninstall round trip (otherwise it prints `SKIP`).
  The rules for which plugin files may be removed are tested in Go: `TestUninstallOpencode` and `TestUninstallOpencode_MatchesInstall` (every fixture must leave the same tree as an install with every hook disabled) in `cli/internal/hooks/opencode_test.go`, and `cli/cmd/uninstall_test.go` for the command (manifest, legacy config, registry, asset cache).
- **`remote-install.test.sh`** covers only `scripts/remote-install.sh`'s install-directory check, offline: every run uses `env -i`, a temp cwd and a stub `curl` first on `PATH` that logs and fails, so "curl was called" means the check passed and "not called" means it refused before downloading. Never run the script itself without a stub `curl` in a test.

Gotchas when running tests:

- `go test` won't compile `internal/assets` until `./scripts/stage-assets.sh` has run (staged dirs are gitignored). `TestEmbeddedFS` reads the staged copy, so re-stage after editing assets.
- `TestCommandExists` expects `go` on `PATH` (`cmd/install_test.go:115`); hook shell tests need `python3`.
- On macOS `t.TempDir()` lives under `/var/…` → `/private/var/…`; compare resolved paths with `filepath.EvalSymlinks`, as `repo/repo_test.go:81-87` does.

## Before Every Commit

Mirror CI — it runs all of these on the PR:

- [ ] `./scripts/stage-assets.sh && (cd cli && go test ./... -race -cover)`
- [ ] `for f in hooks/claude-code/*.test.sh; do bash "$f" || exit 1; done`
- [ ] `for f in hooks/opencode/*.test.js; do node "$f" || exit 1; done`
- [ ] `for f in ./*.test.sh; do bash "$f" || exit 1; done`
- [ ] Lint: not enforced — no lint job in `ci.yml`, no linter config. `(cd cli && go vet ./... && gofmt -l .)` is clean at this commit and was run by hand for #97 (`CHANGELOG.md:147`).
- [ ] Type check: N/A — covered by `go test`/`go vet` for Go; none configured for shell/JS.
- [ ] Changed an agent, skill or hook? `./install.sh` and exercise it in Claude Code/opencode (see [`setup.md`](setup.md#commands)).

## Coverage & Gaps

No threshold: CI prints per-package coverage (`go test ./... -race -cover`, `ci.yml:21`) and fails only on test failures. For a per-function view:

```bash
cd cli && go test ./... -coverprofile=/tmp/cover.out && go tool cover -func=/tmp/cover.out
```

Per package at this commit: `config` 48.5% · `ui` 61.5% · `mcp` 64.7% · `cmd` 65.0% · `repo` 83.0% · `assets` 83.3% · `agents` 85.9% · `skills` 86.0% · `hooks` 90.9% · `manifest` 91.3%.

Untested areas worth knowing:

- **Install orchestration:** `runInstall` and `runWizard` are untested (0%); `runWizard` needs a TTY. The `doInstall*` functions read `os.Getenv("HOME")` directly (`cli/cmd/install_claude.go:20`, `install_opencode.go:19`), so their tests set a temp `HOME` with `t.Setenv` and an empty `PATH` via `fakeCLI(t)`. The HOME refusal is the exception that runs `runInstall` end to end: `TestInstallCmd_RefusesBadHome` (flag paths, standalone, and the wizard path, which is refused before its first prompt) and `TestDoInstall_RefusesBadHome` set HOME unset/empty/relative (`badHomes`) with a dotfiles-style tree in a temp cwd (`writeDotfilesTree`) and assert it stays byte-identical (`treeState`); `TestUninstallCmd_RefusesBadHome` does the same for `devexp uninstall`. `TestDoInstall_PartialRunsPrintNoHooks` and `TestDoInstall_UnreadableManifest` cover both targets (partial runs; an unreadable manifest warns and removes no agents or skills). `TestDoInstallOpencode_Hooks`, `_TamperedManifest`, `_RefusalKeepsLegacyInstall`, `_LostManifest` and `_SymlinkedDevexpAllDisabled` (`cmd/install_test.go`) cover the opencode target further, covering the hook plugin install, disabling, `--agents-only`/`--skills-only`, dry-run and legacy cleanup. The plugin installer itself is covered by `cli/internal/hooks/opencode_test.go`, which includes a `node` load of the Go-written tree (skipped when `node` isn't on `PATH`), symlinked-root refusals, atomic writes (read-only directories via `readOnlyDir`, skipped as root) and legacy fixtures in `cli/internal/hooks/testdata/legacy-opencode/`.
- **Config (0%):** `config.Load` and `IsAgentDisabled`/`IsSkillDisabled`/`IsHookDisabled` (`cli/internal/config/config.go`); only `dotenv.go` has tests.
- **Registry and exec paths (0%):** `mcp.InstallClaude`, `isInstalledClaude`, `RemoveClaude`; all promptui prompts in `cli/internal/ui/prompts.go`.
- **Hooks without behaviour tests:** `secret-in-write-guard`, `large-file-guard`, `format-on-save`, `lint-on-save`, `test-on-save` are covered only by `fail-closed.test.sh` (shell side); the three `graphify-*` hooks have no tests at all; on the opencode side `secret-guard` and `dangerous-cmd-guard` have their own `.test.js` files, and `devexp-plugin.test.js` covers the entry plus the lint/format/test-on-save edit handlers only as far as they run with no linter, formatter or test runner found (no tool output is asserted).
- **`uninstall.sh`:** the Claude Code hook-removal block, the opencode MCP block and the opencode plugin wiring are tested; the agent, skill and Claude Code MCP removal paths are not.
