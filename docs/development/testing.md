# Testing

> Kit doc · Last verified: 2026-09-16 against commit `eaebd52de9323825e6f3fdf286253a89ea89a7a5`

Where tests live, how they're written and run, and what must pass before a commit. Commands for everything else (build, install, env vars) are in [`setup.md`](setup.md); code style in [`conventions.md`](conventions.md).

## Test Types

CI (`.github/workflows/ci.yml`) runs in three jobs: `test` (Go), `hooks` (the three script suites) and `govulncheck` (the [vulnerability scan](#vulnerability-scan), not a test suite). It runs on every pull request, on every push to `main`, weekly on a schedule (`on.schedule`, Mondays 06:27 UTC), and on every `v*` tag — `release.yml` calls it through `on.workflow_call` and goreleaser `needs:` that call, so the jobs below are also the release gate. All four suites below were green at this commit.

| Type | Framework | Location | Run |
|------|-----------|----------|-----|
| Unit — Go CLI | Go stdlib `testing` only; no assertion or mock library in `cli/go.mod` | `cli/**/<file>_test.go`, next to the code, same package | `./scripts/stage-assets.sh && (cd cli && go test ./... -race -cover)` — `ci.yml`, job `test` |
| Hook behaviour — Claude Code | plain bash script, `pass`/`fail` counters | `hooks/claude-code/<hook>.test.sh` | `for f in hooks/claude-code/*.test.sh; do bash "$f" \|\| exit 1; done` — `ci.yml`, job `hooks`, step `claude-code hook tests` |
| Hook behaviour — opencode | plain `node` ESM script, no framework (Node 22 in CI) | `hooks/opencode/<hook>.test.js` | `for f in hooks/opencode/*.test.js; do node "$f" \|\| exit 1; done` — `ci.yml`, job `hooks`, step `opencode hook tests` |
| Installer script | plain bash script | repo root `*.test.sh` (`uninstall.test.sh`, `install.test.sh`, `remote-install.test.sh`) | `for f in ./*.test.sh; do bash "$f" \|\| exit 1; done` — `ci.yml`, job `hooks`, step `installer script tests` |
| Integration | N/A — no separate suite. Go tests already do real file I/O inside `t.TempDir()`, and each hook `.test.sh` executes the real hook script against a real tool-call JSON envelope | — |
| E2E | N/A — no automated end-to-end install test. `runInstall`, `doInstallClaude`, `doInstallOpencode` and `runWizard` have 0% coverage. Manual check: `./install.sh --dry-run` | — |
| Agents / skills (Markdown) | N/A — no automated validation. Edit → `./install.sh` → try it in Claude Code or opencode | `docs/development/README.md` (Notes) |

At this commit: 10 Go packages with tests; `dangerous-cmd-guard.test.sh` 380 cases, `fail-closed.test.sh` 10, `interpreter-isolation.test.sh` 11, `interpreter-proof.test.sh` 120, `large-file-guard.test.sh` 34, `on-save-path.test.sh` 73, `scan-budget.test.sh` 64, `secret-guard.test.sh` 37, `secret-in-write-guard.test.sh` 321; `dangerous-cmd-guard.test.js` 434 (incl. timing checks on crafted long lines), `devexp-plugin.test.js` 130, `on-save-path.test.js` 61, `scan-budget.test.js` 81 (incl. both twins on the same input), `secret-guard.test.js` 38, `secret-in-write-guard.test.js` 301; `uninstall.test.sh` 250 (241 when running as root, as in CI, which skips the read-only permission checks), `install.test.sh` 21, `remote-install.test.sh` 24.

Run one test:

```bash
cd cli && go test ./internal/manifest -run 'TestStale/empty_new'   # one Go subtest (spaces in map keys become _)
bash hooks/claude-code/secret-guard.test.sh                        # one hook suite — the "Run:" line in its header
node hooks/opencode/secret-guard.test.js
```

## Writing a Test

- **New test file path:**
  - Go: `<same dir>/<file>_test.go` declaring the **same package**, so unexported functions are testable — e.g. `cli/internal/manifest/manifest_test.go` (`package manifest`), `cli/cmd/install_test.go` (`package cmd`).
  - Hooks: `hooks/claude-code/<hook>.test.sh` **and** a mirrored `hooks/opencode/<hook>.test.js` covering the same cases — e.g. `secret-guard.test.sh` / `secret-guard.test.js`, `dangerous-cmd-guard.test.sh` / `.test.js`. CI picks new files up by glob (the three `for f in …` loops in `ci.yml`'s `hooks` job); nothing to register.
- **Reference test to copy:**
  - `cli/internal/manifest/manifest_test.go` — smallest complete example: map-keyed table tests, `t.TempDir()`, `reflect.DeepEqual`, the house error-message format.
  - `cli/internal/hooks/installer_test.go` — the richer one: `t.Helper()` fixture builders (`createScript`, `writeSettingsHooks`, `readHooks`) and comments explaining why each case matters.
  - `hooks/claude-code/secret-guard.test.sh` + `hooks/opencode/secret-guard.test.js` — the mirrored hook pair.
  - `hooks/claude-code/scan-budget.test.sh` + `hooks/opencode/scan-budget.test.js` — one behaviour across every guard and both twins: the shell side drives all three guards through `DEVEXP_SCAN_BUDGET_MS`, the JS side does the same and then runs the shell twin on the equivalent envelope and compares the two verdicts. Copy this shape when a change has to hold for every guard rather than for one.
  - `hooks/claude-code/interpreter-proof.test.sh` — the same shape for a property with no opencode twin: every `fail_closed` guard is driven under stand-in `python3` and `grep` programs on `PATH`, each of which reports success without doing the work, and each guard must block. It reads the registry for the list of guards, so a new `fail_closed` guard is covered the day it is registered. Copy this shape when the thing under test is what a hook *depends on* rather than the hook's own input.
  - `hooks/claude-code/secret-in-write-guard.test.sh` + `hooks/opencode/secret-in-write-guard.test.js` — the pair for a guard that inspects content: the JS side drives the real `tool.execute.before` handler with opencode-shaped args (`write`, `edit`, `apply_patch`) instead of a pure predicate, the shell side also covers `MultiEdit` and `NotebookEdit` envelopes and checks that the registry matcher routes every content-writing tool to the guard (so the counts differ), and secret-shaped fixtures are assembled at runtime from split pieces, so the test file itself contains nothing the installed `secret-in-write-guard` would block.
- **Go test shape** (each point seen in 2+ files):
  - Table tests are a **map keyed by a sentence**: `tests := map[string]struct{...}` then `for name, tt := range tests { t.Run(name, ...) }` — see `manifest_test.go`, `hooks/installer_test.go`, `repo/repo_test.go` (`TestIsRepoDir`), `cmd/install_test.go` (`TestCommandExists`). Scenario tests use inline `t.Run("sentence", ...)` — see `cmd/install_test.go` (`TestDetectTargets`), `repo/repo_test.go` (`TestExtractEmbedded`).
  - Failure messages: `Func() error = %v` and `Func() = %v, want %v` — see `manifest_test.go`, `cmd/install_test.go`.
  - No test calls `t.Parallel()` (none in `cli/`); `t.Setenv`/`t.Chdir` panic in parallel tests, so keep it that way where they're used.
- **Fixtures / factories:** built per test by `t.Helper()` functions that write files into a `t.TempDir()` — `createScript`/`writeSettingsHooks` (`internal/hooks/installer_test.go`), `writeAgentFiles` (`internal/agents/installer_test.go`), `writeSkillDir` (`internal/skills/installer_test.go`), `writeRegistry` (`cmd/install_test.go`). There is no shared test-helper package: copy the helper you need — `captureStdout` is copied into four test packages (`cmd`, `internal/agents`, `internal/mcp`, `internal/skills`) and `captureOutput` into two (`internal/ui`, `internal/hooks`).
- **Mocking / fakes:** no mock library. Isolate with real resources the test owns:
  - Files → `t.TempDir()`; never the real `$HOME` or user cache. Installers take explicit src/target/settings paths, and path builders take `home`/`now` as arguments (`claudeTargetPaths(home, now)` in `cli/cmd/paths.go`) so they can be asserted without touching the machine.
  - Env, `PATH` and cwd → `t.Setenv` / `t.Chdir`, which restore automatically — see `repo/repo_test.go`, `cmd/install_test.go`. A fake CLI is an executable stub on a `PATH` that contains nothing else: `fakeCLI(t, "claude")` (`cmd/install_test.go`).
  - Logic behind a TTY prompt can't be driven by a test — extract it into a pure function and test that: `selectTargets` (`cli/cmd/targets.go:20`, tested by `TestSelectTargets`), `buildMultiSelectDisplay` (tested in `internal/ui/ui_test.go`).
  - Single-instance seams, reuse when they fit `[verify — inferred from single example]`: static fixture files under `testdata/` (`cli/internal/agents/testdata/`); `testing/fstest.MapFS` standing in for an `fs.FS` (`repo/repo_test.go`); a package-level function variable swapped with a `t.Cleanup` restore (`var userCacheDir` in `cli/internal/repo/repo.go:412`, swapped by `withTempCache` in `repo_test.go`); `blockedDir` for forcing a `MkdirAll` failure (`cmd/install_test.go`).
- **External services in tests:** none are called. The `claude mcp add/list/remove` exec paths are untested (only `TestAddClaude_NonExec` covers the non-exec branch, `internal/mcp/mcp_test.go`); CLI presence is faked with `fakeCLI`. No test uses the network.
- **Hook tests** (`secret-guard.test.sh`, `secret-in-write-guard.test.sh`, `dangerous-cmd-guard.test.sh`, and their `.js` mirrors):
  - Shell: header `# Run: bash hooks/claude-code/<hook>.test.sh`, `set -uo pipefail`, a `run` helper that builds the real PreToolUse JSON envelope with `python3` and pipes it into the hook, `expect block|allow …` lines (exit 2 = block, exit 0 = allow), then `printf '%d passed, %d failed'` and a non-zero exit on any failure.
  - JS: header `mirrors hooks/claude-code/<hook>.test.sh`, import the module's exported pure predicate (`isSecretFile`/`secretInCommand` from `secret-guard.js`, `blockReason` from `dangerous-cmd-guard.js`, which applies `BLOCK_PATTERNS` to each line of the `maskInert` text the hook scans), or drive the exported hook's handler when the decision depends on the tool args (`secretInWriteGuard` from `secret-in-write-guard.js`), loop over block/allow arrays, print `N passed, M failed`, `process.exit(fail === 0 ? 0 : 1)`.
  - The opencode entry has its own suite, `hooks/opencode/devexp-plugin.test.js` (no shell mirror): it builds the installed layout (`plugins/devexp.js` + `plugins/devexp/hooks.json` and modules) in a `mkdtempSync` dir, writes probe modules that record calls on `globalThis`, and imports the entry with a `?n=` cache-busting query. It covers selection, failure isolation, fail-closed stubs (including the `fail_closed` spelling, security-guard names and malformed entries), the `hooks.json` key contract (parsed from the authoring guide's example), module-name refusal (`node:` builtins, encoded dot segments), `event` → `file.edited` dispatch (polled with `waitFor`, since handlers run after `event` returns), a slow fake local `eslint` proving the event loop stays free, `runCommand` timeouts, the on-save handlers' edit path, and registry consistency (`opencode.module`/`export` for every hook).
  - Failure mode: add every new Claude Code hook to `hooks/claude-code/fail-closed.test.sh` — `check <hook> 2 guard` for security guards (must fail **closed**), `check <hook> 0 advisory` for advisory hooks (may fail open, but must print `internal error`).
- **Installer script tests:** never run `uninstall.sh` against your real `HOME` — it deletes real files. `uninstall.test.sh` does three things:
  - extracts the embedded python heredocs with `awk` and runs them against fixture `settings.json` / `config.json` content in a `mktemp -d` dir;
  - runs `uninstall.sh` itself with `--yes`, stdin closed, `env -i` and a temp `HOME`, with stub `devexp` binaries that log their calls (`make_stub`, `make_old_stub`, `run_uninstall`), to cover detection, binary lookup, step order, the crash regressions and the HOME refusal (unset, empty and relative `HOME` from a temp cwd holding a dotfiles-style tree, which must stay byte-identical — `tree_sum`);
  - when `go` is on `PATH` and the assets are staged, builds the real binary and does an install → uninstall round trip (otherwise it prints `SKIP`).
  The rules for which plugin files may be removed are tested in Go: `TestUninstallOpencode` and `TestUninstallOpencode_MatchesInstall` (every fixture must leave the same tree as an install with every hook disabled) in `cli/internal/hooks/opencode_test.go`, and `cli/cmd/uninstall_test.go` for the command (manifest, legacy config, registry, asset cache).
- **`install.test.sh`** covers only `install.sh`'s HOME check: a copy of `install.sh` in a temp repo with no `bin/`, a stub `scripts/stage-assets.sh` and a stub `go` that log to an absolute file, so "nothing logged" means no build was attempted.
- **`remote-install.test.sh`** covers only `scripts/remote-install.sh`'s install-directory check, offline: every run uses `env -i`, a temp cwd and a stub `curl` first on `PATH` that logs and fails, so "curl was called" means the check passed and "not called" means it refused before downloading. Never run the script itself without a stub `curl` in a test.

Gotchas when running tests:

- `go test` won't compile `internal/assets` until `./scripts/stage-assets.sh` has run (staged dirs are gitignored). `TestEmbeddedFS` reads the staged copy, so re-stage after editing assets.
- `TestCommandExists` expects `go` on `PATH` (`cmd/install_test.go:115`); hook shell tests need `python3`.
- On macOS `t.TempDir()` lives under `/var/…` → `/private/var/…`; compare resolved paths with `filepath.EvalSymlinks`, as `repo/repo_test.go` does.

## Vulnerability Scan

`scripts/govulncheck.sh` is the one place the scan is defined: the pinned govulncheck version (`GOVULNCHECK_VERSION`), the platforms and the exit codes. It stages assets, installs govulncheck into a temporary `GOBIN` it removes on exit, then runs `govulncheck -show verbose ./...` in `cli/` once for every platform the release ships: each `goos` × `goarch` in `.goreleaser.yaml` with its `CGO_ENABLED=0`. Reachability differs per platform (darwin builds `fsnotify`'s kqueue backend and `crypto/x509/root_darwin.go`, linux the inotify and `root_linux.go` files), so a scan of the runner's linux/amd64 alone could miss a call that ships in the darwin binary.

There is one `govulncheck` job, in `ci.yml`. It sets up Go from `cli/go.mod` (so the go1.26.8 standard library the release is built with is scanned), runs the script with a read-only token and `persist-credentials: false`, and runs wherever `ci` runs: pull requests, pushes to `main`, the weekly schedule, and the tag, where `release.yml` calls `ci.yml` and goreleaser `needs:` the call — so a failed scan publishes nothing (see [`../guides/release.md`](../guides/release.md#build)).

The weekly run exists because an advisory published while `main` is quiet would otherwise wait for the next push, PR or tag. It runs the whole `ci` workflow, not a scan-only copy of the job, and a failure is a red `ci` run on `main` in the Actions tab. The cron is `27 6 * * 1` rather than anything on the hour, because GitHub delays and sometimes drops scheduled runs at high-load times, of which the start of every hour is one.

**Who gets the email matters, because it is the whole alert.** This run deliberately files no issue — that would need a write-scoped token on a workflow whose point is a read-only one, plus dedup so a standing advisory doesn't file one issue a week. GitHub sends scheduled-run notifications to the user who *created* the workflow; if someone else later modifies the `cron` line, they become the recipient instead; and if the workflow is ever disabled and re-enabled, the recipient becomes whoever re-enabled it, which overrides the `cron` rule. So editing the schedule — or re-enabling it after the auto-disable below — silently moves the alerts onto you, and the previous recipient is never told they stopped receiving them. It also depends on that account having Actions notifications switched on.

Scheduled workflows fire only from the default branch. **In a public repository — this one — GitHub disables them after exactly 60 days with no repository activity.** A disabled schedule produces no run, so no failure, so no alert: that is the one way this can go quiet. `workflow_dispatch` is the way back — re-enable the workflow, then start `ci` by hand to confirm the scan runs again, instead of waiting until Monday. It is also how to scan a freshly published advisory on demand.

Exit status:

- **0 — nothing called.** Findings in packages the CLI imports or modules it requires, where nothing reaches the affected symbols (`=== Package Results ===` / `=== Module Results ===`), are printed but don't fail. Fix these in the next dependency bump; they still ship in the binary.
- **3 — called vulnerability** on at least one platform: a vulnerable function is reachable from the CLI's code, listed under `=== Symbol Results ===` with an example trace. The last line names the platforms. The job fails; fix it (below).
- **Anything else (1) — infrastructure, not a finding**: staging or installing govulncheck failed, `.goreleaser.yaml` couldn't be read, or govulncheck couldn't finish — usually a fetch or network error such as `govulncheck: fetching vulnerabilities: Get "https://vuln.go.dev/…"` or a module proxy error, with no `=== Symbol Results ===`. The last line says `infrastructure failure, not a finding`. Nothing in the code needs fixing: re-run the failed job with `gh run rerun <run-id> --failed` (the run ID is in `gh run list --workflow ci.yml` or `release.yml`).

Run it locally exactly as CI does (nothing is installed on `PATH`):

```bash
./scripts/govulncheck.sh
```

When it exits 3:

1. Read the finding: the advisory ID, `Found in` / `Fixed in`, and the trace showing which of our calls reaches it, on which platform.
2. **Standard library** (`Found in: <pkg>@go1.x.y`): bump only the `toolchain` line in `cli/go.mod` to a supported patch at or above `Fixed in` (see [`../guides/workflows.md`](../guides/workflows.md), Go toolchain). **Module**: `cd cli && go get <module>@<fixed version> && go mod tidy`, commit `go.mod` and `go.sum` together.
3. Re-run `./scripts/govulncheck.sh` until it exits 0, run the Go tests, and land the bump as its own `fix:` commit naming the advisory, with a `CHANGELOG.md` entry.
4. No fixed version yet: don't disable the job. Record the advisory in an issue; if the called path can be avoided in our code, do that.

To bump govulncheck itself, change `GOVULNCHECK_VERSION` in `scripts/govulncheck.sh`. CI runs with `GOTOOLCHAIN=local`, so the new version's `go` directive must not be newer than the `toolchain` in `cli/go.mod` (v1.8.0 needs Go 1.26.0).

## Before Every Commit

Mirror CI — it runs all of these on the PR:

- [ ] `./scripts/stage-assets.sh && (cd cli && go test ./... -race -cover)`
- [ ] `for f in hooks/claude-code/*.test.sh; do bash "$f" || exit 1; done`
- [ ] `for f in hooks/opencode/*.test.js; do node "$f" || exit 1; done`
- [ ] `for f in ./*.test.sh; do bash "$f" || exit 1; done`
- [ ] Touched `cli/go.mod`/`go.sum`, the Go toolchain or `.goreleaser.yaml` platforms? `./scripts/govulncheck.sh` — CI fails on a called finding (see [vulnerability scan](#vulnerability-scan)).
- [ ] Lint: not enforced — no lint job in `ci.yml`, no linter config. `(cd cli && go vet ./... && gofmt -l .)` is clean at this commit and was run by hand for #97 (`CHANGELOG.md`, `## [0.7.0]`, the `cmd/install.go` split entry).
- [ ] Type check: N/A — covered by `go test`/`go vet` for Go; none configured for shell/JS.
- [ ] Changed an agent, skill or hook? `./install.sh` and exercise it in Claude Code/opencode (see [`setup.md`](setup.md#commands)).

## Coverage & Gaps

No threshold: CI prints per-package coverage (`go test ./... -race -cover`, `ci.yml`, job `test`) and fails only on test failures. For a per-function view:

```bash
cd cli && go test ./... -coverprofile=/tmp/cover.out && go tool cover -func=/tmp/cover.out
```

Per package at this commit: `config` 48.5% · `ui` 65.6% · `mcp` 69.7% · `assets` 83.3% · `cmd` 86.7% · `repo` 86.7% · `agents` 89.0% · `removeguard` 90.0% · `manifest` 91.3% · `skills` 91.8% · `hooks` 92.8% · `fsutil` 93.1%.

Untested areas worth knowing:

- **Install orchestration:** `runWizard` is untested (0%) — it needs a TTY. `runInstall` is reached only through `executeRoot` by the HOME-refusal tests and by `TestInstallCmd_KimiSelection` (`cmd/install_test.go`), which drives the flag path end to end for the Kimi target — a real install of its MCP servers, agents and skills, a second run that must change nothing, each `--*-only` flag installing exactly its own kind, and a hostile `$KIMI_CODE_HOME` that must be refused without forging output. `TestDoInstallKimi_MCPs` covers `doInstallKimi` directly (a scratch `KIMI_CODE_HOME`, `--mcps-only` saving the ownership manifest, dry run, an unmergeable `mcp.json` that costs only the MCP step, a lost manifest, `--reinstall-mcps`), and the merge itself is covered in `cli/internal/mcp/kimi_test.go`, whose schema table mirrors what the real Kimi CLI accepts and rejects. Target selection itself is tested away from `runInstall`: `selectTargets` and `targetsFromLabels` are pure, and `resolveTargets` swaps the `stdinIsTerminal` and `promptTargets` package vars, so the checklist arm needs no terminal. The `doInstall*` functions read `os.Getenv("HOME")` directly (`claudeTargetPaths` / `opencodeTargetPaths` calls in `cli/cmd/install_claude.go` and `cli/cmd/install_opencode.go`), so their tests set a temp `HOME` with `t.Setenv` and an empty `PATH` via `fakeCLI(t)`. The HOME refusal is the exception that runs `runInstall` end to end: `TestInstallCmd_RefusesBadHome` (flag paths, standalone, and the wizard path, which is refused before its first prompt) and `TestDoInstall_RefusesBadHome` set HOME unset/empty/relative (`badHomes`) with a dotfiles-style tree in a temp cwd (`writeDotfilesTree`) and assert it stays byte-identical (`treeState`) — its asset cache sits under `home/` too, so a wipe by `repo.Resolve` shows — and that no CLI was called: the repo has one MCP (`refusalRepo`) and `loggingCLI` stubs log every call to an absolute file outside the cwd; `TestUninstallCmd_RefusesBadHome` does the same for `devexp uninstall`. `TestDoInstall_PartialRunsPrintNoHooks` and `TestDoInstall_UnreadableManifest` cover both targets (partial runs; an unreadable manifest warns and removes no agents or skills). `TestDoInstallOpencode_Hooks`, `_TamperedManifest`, `_RefusalKeepsLegacyInstall`, `_LostManifest` and `_SymlinkedDevexpAllDisabled` (`cmd/install_test.go`) cover the opencode target further, covering the hook plugin install, disabling, `--agents-only`/`--skills-only`, dry-run and legacy cleanup. The plugin installer itself is covered by `cli/internal/hooks/opencode_test.go`, which includes a `node` load of the Go-written tree (skipped when `node` isn't on `PATH`), symlinked-root refusals, atomic writes (read-only directories via `readOnlyDir`, skipped as root) and legacy fixtures in `cli/internal/hooks/testdata/legacy-opencode/`.
- **Config (0%):** `config.Load` and `IsAgentDisabled`/`IsSkillDisabled`/`IsHookDisabled` (`cli/internal/config/config.go`); only `dotenv.go` has tests.
- **Registry and exec paths (0%):** `mcp.InstallClaude`, `isInstalledClaude`, `RemoveClaude`; all promptui prompts in `cli/internal/ui/prompts.go`.
- **Hooks without behaviour tests:** `large-file-guard` is covered by `fail-closed.test.sh` and `large-file-guard.test.sh` (shell side only); `format-on-save`, `lint-on-save` and `test-on-save` by `fail-closed.test.sh` plus `on-save-path.test.sh` and its mirror `hooks/opencode/on-save-path.test.js`, which pin each tool call's exact argv and working directory with stub tools on `PATH`, not what the tool does; the three `graphify-*` hooks have no tests at all; on the opencode side `secret-guard`, `secret-in-write-guard` and `dangerous-cmd-guard` have their own `.test.js` files, and `devexp-plugin.test.js` covers the entry plus the lint/format/test-on-save edit handlers only as far as they run with no linter, formatter or test runner found (no tool output is asserted).
- **`uninstall.sh`:** the Claude Code hook-removal block, the opencode MCP block and the opencode plugin wiring are tested; the agent, skill and Claude Code MCP removal paths are not.
