# Setup

> Kit doc · Last verified: 2026-09-16 against commit `a86d2c3f6a41a6d033d31afd858ff723d5267dd7`

For **contributors** working on the toolkit from a clone: prerequisites, build, the dev command list, and environment variables. Installing devexp as an end user (remote install, `./install.sh` flags, uninstall, where files land) is covered in [`../guides/install.md`](../guides/install.md). Tests in depth: [`testing.md`](testing.md). Code style: [`conventions.md`](conventions.md). How the pieces fit: [`../architecture/overview.md`](../architecture/overview.md).

## Prerequisites

| Tool | Version | Source |
|------|---------|--------|
| Go | 1.25 (`go 1.25.11` in the module; CI pins `"1.25"`) | `cli/go.mod:3`, `.github/workflows/ci.yml:15` |
| bash | any — every script is `#!/usr/bin/env bash` | `install.sh:1`, `scripts/stage-assets.sh:1` |
| rsync | not pinned — copies assets for embedding | `scripts/stage-assets.sh:16,19` |
| python3 | not pinned — Claude Code hooks and their tests pipe the JSON envelope through it | `hooks/claude-code/secret-guard.test.sh:11`, `hooks/claude-code/fail-closed.test.sh:4-5` |
| Node.js | 22 in CI — opencode hook modules are ESM (`{"type":"module"}`) | `.github/workflows/ci.yml:29`, `hooks/opencode/package.json` |
| `claude` and/or `opencode` on `PATH` | any — `devexp install` refuses to run without one | `cli/cmd/targets.go:31` |
| git | any — to clone | `README.md:83` |

No lint, formatter or release tool needs to be installed locally: CI has no lint job, and goreleaser runs only in GitHub Actions (see [`../guides/release.md`](../guides/release.md)).

## First Run

```bash
git clone https://github.com/alexandrocuma/devexp-toolkit.git   # source: README.md:83
cd devexp-toolkit
cp mcps/.env.example mcps/.env        # optional — only MCPs with required_env need it   # source: README.md:133
./scripts/stage-assets.sh             # stage embeddable assets — required before any go build/test   # source: cli/internal/assets/assets.go:1-6
(cd cli && go build -o ../bin/devexp .)   # source: install.sh:12
./install.sh --dry-run                # preview; installs nothing   # source: install.sh:20, cli/cmd/install.go:115
```

Expected result: the dry run prints `DRY RUN MODE — no files will be written`, `Detected: Claude Code` (and/or opencode), the MCPs/agents/skills/hooks it would install, and ends with `All done.` Until `UI_INSPECTOR_DIR` is set it also prints `[REQUIRED] ui-inspector — missing required env vars` — a warning, not a failure (`cli/internal/ui/output.go:28`).

`./install.sh` on its own would have built `bin/devexp` for you (it runs staging + `go build` when the binary is missing — `install.sh:7-18`); the explicit steps above make each stage visible. To install for real, run `./install.sh` — see [`../guides/install.md`](../guides/install.md).

Because a binary run from the clone reads `agents/`, `skills/`, `hooks/` and `mcps/` **live from disk** (`findRepoDir` in `cli/internal/repo/repo.go`), editing an asset needs only `./install.sh` again — no rebuild. Changing Go code under `cli/` does need a rebuild (see Troubleshooting).

## Commands

The full list — `CLAUDE.md` shows only the most-used few and links here.

| Task | Command | Source |
|------|---------|--------|
| Install deps | none separate — Go modules download on first `go build`/`go test`; hooks use only Node/python builtins | `cli/go.mod`, `hooks/opencode/package.json` |
| Stage embedded assets | `./scripts/stage-assets.sh` | `scripts/stage-assets.sh` (wipes and recopies into `cli/internal/assets/`, excluding `mcps/.env`) |
| Build | `./scripts/stage-assets.sh && (cd cli && go build -o ../bin/devexp .)` | `install.sh:11-12` |
| Rebuild via the installer | `rm bin/devexp && ./install.sh` | `install.sh:7` (builds only when `bin/devexp` is missing) |
| Run locally (from source) | `cd cli && go run . install --dry-run` | `cli/main.go`; a dev build finds the checkout by walking up from cwd — `findRepoDir` in `cli/internal/repo/repo.go` |
| Show version | `bin/devexp --version` → `devexp version dev` for local builds | `cli/cmd/root.go:15` |
| Preview an install | `./install.sh --dry-run` (or `-n`) | `cli/cmd/install.go:32` |
| Install — interactive wizard | `./install.sh` (no flags; needs a TTY) | `cli/cmd/install.go:115-151` |
| Install — non-interactive | `./install.sh` with any of `--dry-run`, `--reinstall-mcps`, `--mcps-only`, `--agents-only`, `--skills-only` (`--model` alone does **not** skip the wizard) | `cli/cmd/install.go:115-119` |
| Uninstall | `./uninstall.sh` / `./uninstall.sh --yes` | [`../guides/install.md`](../guides/install.md#uninstallsh) |
| Test — all (what CI runs) | the four suites in [`testing.md`](testing.md#test-types) | `.github/workflows/ci.yml` |
| Test — Go | `./scripts/stage-assets.sh && (cd cli && go test ./... -race -cover)` | `.github/workflows/ci.yml:17-21` |
| Test — single Go test | `cd cli && go test ./internal/manifest -run 'TestStale'` | Go toolchain; verified at this commit |
| Test — single hook | `bash hooks/claude-code/secret-guard.test.sh` · `node hooks/opencode/secret-guard.test.js` | `Run:` header line in each test file |
| Test — hooks / installer script | `for f in hooks/claude-code/*.test.sh; do bash "$f" \|\| exit 1; done` · `for f in hooks/opencode/*.test.js; do node "$f" \|\| exit 1; done` · `for f in ./*.test.sh; do bash "$f" \|\| exit 1; done` | `.github/workflows/ci.yml:30-45` |
| Coverage | `cd cli && go test ./... -cover` | `.github/workflows/ci.yml:21` |
| Lint / format | Not enforced — no lint job in CI and no linter config in the repo. `(cd cli && go vet ./... && gofmt -l .)` is clean at this commit and was run by hand for #97 | `.github/workflows/ci.yml`, `CHANGELOG.md:147` |
| Shell syntax check (hooks) | `bash -n hooks/claude-code/<hook>.sh` | `docs/development/hook-authoring-guide.md` (Deployment Checklist) |
| Type check | N/A — Go is type-checked by `go build`/`go vet`; no type checker is configured for the shell or JS hooks | `cli/go.mod`, `hooks/opencode/package.json` |
| Release build | CI only — tag push → goreleaser. See [`../guides/release.md`](../guides/release.md) | `.github/workflows/release.yml` |
| Migrate | N/A — no database; the CLI only reads/writes files and shells out to `claude` | `cli/go.mod` (cobra, viper, promptui only) |

## Environment Variables

| Variable | Required | Default | What it controls | Source |
|----------|----------|---------|------------------|--------|
| `DEVEXP_DIR` | No | unset | Forces the asset root `devexp install` reads from, skipping the next-to-binary and walk-up-from-cwd lookup. It is the only way to point a tagged release binary at a checkout: release builds never adopt one they find on disk. A relative value is resolved to an absolute path (hook commands in `settings.json` are built from it and are always absolute), and it must be a devexp-toolkit checkout — the `.devexp-toolkit` marker file (first line `devexp-toolkit`) plus `agents/`, `skills/`, `mcps/` — otherwise install stops with an error rather than falling back to another lookup. It is also always set to the resolved repo dir in the env used to expand `${VAR}` in MCP entries | `findRepoDir` / `isRepoDir` in `cli/internal/repo/repo.go`, `cli/cmd/registry.go:57` |
| `HOME` | Yes | from shell | Root of every install destination (`~/.claude/…`, `~/.config/opencode/…`). Must be an absolute path: `devexp install`, `devexp uninstall` and `uninstall.sh` refuse to run, touching nothing, when it is unset, empty or relative | `cli/cmd/install_claude.go:20`, `cli/cmd/install_opencode.go:19`, `cli/cmd/paths.go` (`targetHome`), `cli/cmd/install.go:78`, `cli/cmd/uninstall.go:80` |
| `PATH` | Yes | from shell | Which of `claude` / `opencode` is found decides the install targets | `cli/cmd/targets.go:57-72` |
| `UI_INSPECTOR_DIR` | Only for the `ui-inspector` MCP | empty | Absolute path of a `mcp-ui-inspector` clone, expanded into that MCP's args. Unset → the MCP is skipped with a `[REQUIRED]` notice | `mcps/.env.example`, `mcps/registry.json:15,18` |
| any var named in an MCP's `required_env` | Per MCP | — | Value substituted for `${VAR}` in MCP args/headers — applies to registry MCPs and to org MCPs added under `mcps` in `devexp.config.json` | `cli/internal/mcp/claude.go:12-19,52`, `cli/internal/config/config.go:33-35` |
| `DEVEXP_VERSION` | No | latest release | `scripts/remote-install.sh` only — tag to download | `scripts/remote-install.sh:10,50` |
| `DEVEXP_INSTALL_DIR` | No | `$HOME/.local/bin` | `scripts/remote-install.sh` only — where the binary is placed. Must be absolute; when it is unset, `HOME` must be absolute — otherwise the script refuses before downloading | `scripts/remote-install.sh:11,20-30` |
| `DEVEXP_SKIP_RUN` | No | unset | `scripts/remote-install.sh` only — any value skips running `devexp install` after download | `scripts/remote-install.sh:12,85` |

Where values come from, in precedence order (`cli/cmd/registry.go:51-61`): the process environment, then `DEVEXP_DIR` = resolved repo dir, then `mcps/.env` — **`mcps/.env` wins**. `mcps/.env` is gitignored (`.gitignore:1`) and never embedded in the binary (`scripts/stage-assets.sh:19`); `mcps/.env.example` is the committed template.

Configuration file: `devexp.config.json` at the repo root (model default, disabled agents/skills/hooks, extra MCPs). A missing file is a warning, not an error (`cli/cmd/install.go:95-98`). Schema and team usage: [`../guides/team-distribution.md`](../guides/team-distribution.md).

## Troubleshooting

**Problem:** `go build`/`go test`/`go vet` fails with `internal/assets/assets.go:16:12: pattern all:agents: no matching files found` · **Cause:** the `//go:embed` dirs under `cli/internal/assets/` are gitignored staging output (`.gitignore:6-12`), absent in a fresh clone or worktree · **Fix:** `./scripts/stage-assets.sh`, then retry.

**Problem:** Go changes don't show up after `./install.sh` · **Cause:** `install.sh` builds `bin/devexp` only when it doesn't exist (`install.sh:7`) — it never rebuilds · **Fix:** `rm bin/devexp && ./install.sh`, or `./scripts/stage-assets.sh && (cd cli && go build -o ../bin/devexp .)`. Asset-only edits need no rebuild.

**Problem:** `HOME is "", not an absolute path — refusing to install anything; set HOME and re-run` (or `refusing to remove anything` from `devexp uninstall` / `uninstall.sh`, or `refusing to install` from `remote-install.sh`) · **Cause:** `HOME` is unset, empty or relative — e.g. under `env -i` or a CI step that clears the environment — and every target path is built from it, so it would resolve under the current directory (`targetHome` in `cli/cmd/paths.go`) · **Fix:** run with an absolute `HOME`, e.g. `HOME=/Users/you ./install.sh`. Nothing was written, registered or backed up.

**Problem:** `no supported CLI detected (claude or opencode)` · **Cause:** neither CLI is on `PATH` (`cli/cmd/targets.go:31`) · **Fix:** install Claude Code or opencode, or fix `PATH`.

**Problem:** the installer opens an interactive wizard (or fails without a TTY) · **Cause:** none of `--dry-run`, `--reinstall-mcps`, `--mcps-only`, `--agents-only`, `--skills-only` was passed — `--model` alone doesn't count (`cli/cmd/install.go:115-119`) · **Fix:** pass one of those flags for the non-interactive path.

**Problem:** `[REQUIRED] ui-inspector — missing required env vars: UI_INSPECTOR_DIR` · **Cause:** the MCP's `required_env` isn't set (`mcps/registry.json:18`) · **Fix:** set it in `mcps/.env` and re-run `./install.sh --mcps-only`; add `--reinstall-mcps` if the MCP was already registered with an old value.

**Problem:** `devexp install` uses different assets than you expected — a release binary ignores your clone, or a dev build picks up another checkout · **Cause:** a directory counts as an asset root only if it is a devexp-toolkit checkout: the `.devexp-toolkit` marker file at its root (a regular file whose first line is `devexp-toolkit`) plus `agents/`, `skills/` and `mcps/` (`isRepoDir`). Lookup order is `$DEVEXP_DIR` → (dev builds only) the directory above the executable → (dev builds only) walk up from cwd; a tagged release build (`version` set by goreleaser, anything but `dev`) skips both implicit lookups. With no match it extracts the embedded assets to `<user cache dir>/devexp/assets`, and with no absolute user cache dir it refuses instead of using a temp directory (`Resolve`, `findRepoDir`, `embeddedDir` in `cli/internal/repo/repo.go`) · **Fix:** read the `Asset root: <dir> (<how it was chosen>)` line `devexp install` prints before installing anything (`announceAssetRoot` in `cli/cmd/install.go`); set `DEVEXP_DIR` to the checkout you want. A checkout without `.devexp-toolkit` (e.g. a fork that dropped it) is not recognised — restore the file.

**Problem:** `TestCommandExists` or the hook tests fail on a new machine · **Cause:** `TestCommandExists` expects `go` on `PATH` (`cli/cmd/install_test.go:115`); hook `.test.sh` files need `python3` · **Fix:** put `go` and `python3` on `PATH`. More in [`testing.md`](testing.md).
