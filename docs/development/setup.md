# Setup

> Kit doc · Last verified: 2026-09-16 against commit `a86d2c3f6a41a6d033d31afd858ff723d5267dd7`

For **contributors** working on the toolkit from a clone: prerequisites, build, the dev command list, and environment variables. Installing devexp as an end user (remote install, `./install.sh` flags, uninstall, where files land) is covered in [`../guides/install.md`](../guides/install.md). Tests in depth: [`testing.md`](testing.md). Code style: [`conventions.md`](conventions.md). How the pieces fit: [`../architecture/overview.md`](../architecture/overview.md).

## Prerequisites

| Tool | Version | Source |
|------|---------|--------|
| Go | Go 1.21+ on `PATH`. `cli/go.mod` sets `toolchain go1.26.8` as the minimum for local builds. With the default `GOTOOLCHAIN=auto`, an older Go downloads go1.26.8 on the first build (needs network; offline it fails with `toolchain not available`), and a newer Go builds with itself. With `GOTOOLCHAIN=local` you need Go ≥ 1.25.11 (the `go` line), and the build uses that version. CI and release install exactly go1.26.8 (`go-version-file: cli/go.mod`) | `cli/go.mod:3-5`, `.github/workflows/ci.yml` (jobs `test`, `govulncheck`), `.github/workflows/release.yml` (job `goreleaser`) |
| bash | any — every script is `#!/usr/bin/env bash` | `install.sh:1`, `scripts/stage-assets.sh:1` |
| rsync | not pinned — copies assets for embedding | `scripts/stage-assets.sh:16,19` |
| python3 | not pinned — Claude Code hooks and their tests pipe the JSON envelope through it | `hooks/claude-code/secret-guard.test.sh:11`, `hooks/claude-code/fail-closed.test.sh:4-5` |
| Node.js | 22 in CI — opencode hook modules are ESM (`{"type":"module"}`) | `.github/workflows/ci.yml` (job `hooks`), `hooks/opencode/package.json` |
| `claude`, `opencode` and/or `kimi` on `PATH` | `claude`/`opencode` any; Kimi Code CLI ≥ `0.31.0` (older is skipped, and the legacy kimi-cli is not Kimi Code). `devexp install` refuses to run without at least one | `cli/cmd/targets.go` (`selectTargets`), `cli/cmd/kimi_detect.go` (`kimiMinVersion`) |
| git | any — to clone | `README.md:83` |

No lint, formatter, scanner or release tool needs to be installed locally: CI has no lint job, `scripts/govulncheck.sh` installs the pinned govulncheck into a temporary directory, and goreleaser runs only in GitHub Actions (see [`../guides/release.md`](../guides/release.md)).

## First Run

```bash
git clone https://github.com/alexandrocuma/devexp-toolkit.git   # source: README.md:83
cd devexp-toolkit
cp mcps/.env.example mcps/.env        # optional — only MCPs with required_env need it   # source: README.md:133
./scripts/stage-assets.sh             # stage embeddable assets — required before any go build/test   # source: cli/internal/assets/assets.go:1-6
(cd cli && go build -o ../bin/devexp .)   # source: install.sh:12
./install.sh --dry-run                # preview; installs nothing   # source: install.sh:22, cli/cmd/install.go:115
```

Expected result: the dry run prints `DRY RUN MODE — no files will be written`, `Detected: Claude Code` (and/or `opencode`, `Kimi Code CLI`), the MCPs/agents/skills/hooks it would install, and ends with `All done.` If Kimi Code is the **only** CLI detected it instead exits non-zero with `nothing was installed` — deliberate, because nothing is installed for Kimi yet; see [Troubleshooting](#troubleshooting). Until `UI_INSPECTOR_DIR` is set it also prints `[REQUIRED] ui-inspector — missing required env vars` — a warning, not a failure (`cli/internal/ui/output.go:28`).

`./install.sh` on its own would have built `bin/devexp` for you (it runs staging + `go build` when the binary is missing — `install.sh:7-20`); the explicit steps above make each stage visible. To install for real, run `./install.sh` — see [`../guides/install.md`](../guides/install.md).

Because a binary run from the clone reads `agents/`, `skills/`, `hooks/` and `mcps/` **live from disk** — a dev build uses the checkout it was compiled from (`Resolve` in `cli/internal/repo/repo.go`), editing an asset needs only `./install.sh` again — no rebuild. Changing Go code under `cli/` does need a rebuild (see Troubleshooting).

## Commands

The full list — `CLAUDE.md` shows only the most-used few and links here.

| Task | Command | Source |
|------|---------|--------|
| Install deps | none separate. The first `go build`/`go test` downloads the Go modules, and also go1.26.8 if your Go is older and `GOTOOLCHAIN=auto` (both need network). Hooks use only Node/python builtins | `cli/go.mod`, `hooks/opencode/package.json` |
| Stage embedded assets | `./scripts/stage-assets.sh` | `scripts/stage-assets.sh` (wipes and recopies into `cli/internal/assets/`, excluding `mcps/.env`) |
| Build | `./scripts/stage-assets.sh && (cd cli && go build -o ../bin/devexp .)` | `install.sh:11-12` |
| Rebuild via the installer | `rm bin/devexp && ./install.sh` | `install.sh:7` (builds only when `bin/devexp` is missing) |
| Run locally (from source) | `cd cli && go run . install --dry-run` | `cli/main.go`; a dev build uses the checkout it was compiled from, wherever it runs — `sourceRoot` in `cli/internal/repo/repo.go` |
| Show version | `bin/devexp --version` → `devexp version dev` for local builds | `cli/cmd/root.go:15` |
| Preview an install | `./install.sh --dry-run` (or `-n`) | `cli/cmd/install.go:32` |
| Install — interactive wizard | `./install.sh` (no flags; needs a TTY) | `cli/cmd/install.go:115-151` |
| Install — non-interactive | `./install.sh` with any of `--dry-run`, `--reinstall-mcps`, `--mcps-only`, `--agents-only`, `--skills-only` (`--model` alone does **not** skip the wizard) | `cli/cmd/install.go:115-119` |
| Uninstall | `./uninstall.sh` / `./uninstall.sh --yes` | [`../guides/install.md`](../guides/install.md#uninstallsh) |
| Test — all (what CI runs) | the four suites in [`testing.md`](testing.md#test-types) | `.github/workflows/ci.yml` (also called by `release.yml` on a tag) |
| Test — Go | `./scripts/stage-assets.sh && (cd cli && go test ./... -race -cover)` | `.github/workflows/ci.yml` (job `test`) |
| Test — single Go test | `cd cli && go test ./internal/manifest -run 'TestStale'` | Go toolchain; verified at this commit |
| Test — single hook | `bash hooks/claude-code/secret-guard.test.sh` · `node hooks/opencode/secret-guard.test.js` | `Run:` header line in each test file |
| Test — hooks / installer script | `for f in hooks/claude-code/*.test.sh; do bash "$f" \|\| exit 1; done` · `for f in hooks/opencode/*.test.js; do node "$f" \|\| exit 1; done` · `for f in ./*.test.sh; do bash "$f" \|\| exit 1; done` | `.github/workflows/ci.yml` (job `hooks`) |
| Coverage | `cd cli && go test ./... -cover` | `.github/workflows/ci.yml` (job `test`) |
| Vulnerability scan (CI job `govulncheck` — on every PR and push to `main`, weekly, and before every release) | `./scripts/govulncheck.sh` — every release platform; exit 3 on a called vulnerability, any other failure is infrastructure. Policy and how to respond in [`testing.md`](testing.md#vulnerability-scan) | `scripts/govulncheck.sh`, `.github/workflows/ci.yml` (job `govulncheck`), `.github/workflows/release.yml` (job `ci`, which calls `ci.yml`) |
| Lint / format | Not enforced — no lint job in CI and no linter config in the repo. `(cd cli && go vet ./... && gofmt -l .)` is clean at this commit and was run by hand for #97 | `.github/workflows/ci.yml`, `CHANGELOG.md` (`## [0.7.0]`, the `cmd/install.go` split entry) |
| Shell syntax check (hooks) | `bash -n hooks/claude-code/<hook>.sh` | `docs/development/hook-authoring-guide.md` (Deployment Checklist) |
| Type check | N/A — Go is type-checked by `go build`/`go vet`; no type checker is configured for the shell or JS hooks | `cli/go.mod`, `hooks/opencode/package.json` |
| Release build | CI only — tag push → the whole `ci` workflow → goreleaser. See [`../guides/release.md`](../guides/release.md) | `.github/workflows/release.yml` |
| Migrate | N/A — no database; the CLI only reads/writes files and shells out to `claude` | `cli/go.mod` (cobra, viper, promptui only) |

## Environment Variables

| Variable | Required | Default | What it controls | Source |
|----------|----------|---------|------------------|--------|
| `DEVEXP_DIR` | No | unset | Forces the asset root `devexp install` reads from, instead of the checkout a dev build was compiled from or the bundled assets. It is the only way to point a release binary, or a dev binary built elsewhere, at a checkout: `devexp` never uses a directory it finds on disk. A relative value is resolved to an absolute path (hook commands in `settings.json` are built from it and are always absolute), and it must be a devexp-toolkit checkout — the `.devexp-toolkit` marker file (first line `devexp-toolkit`) plus `agents/`, `skills/`, `mcps/` — otherwise install stops with an error rather than falling back to another lookup. It is also always set to the resolved repo dir in the env used to expand `${VAR}` in MCP entries | `devexpDir` / `isRepoDir` in `cli/internal/repo/repo.go`, `cli/cmd/registry.go:57` |
| `HOME` | Yes | from shell | Root of every install destination (`~/.claude/…`, `~/.config/opencode/…`). Must be an absolute path: `devexp install`, `devexp uninstall` and `uninstall.sh` refuse to run, touching nothing, when it is unset, empty or relative | `cli/cmd/install_claude.go`, `cli/cmd/install_opencode.go`, `cli/cmd/paths.go` (`targetHome`), `cli/cmd/install.go:78`, `cli/cmd/uninstall.go:80` |
| `PATH` | Yes | from shell | Which of `claude` / `opencode` / `kimi` is found decides the install targets | `cli/cmd/targets.go` (`detectTargets`) |
| `KIMI_CODE_HOME` | No | `~/.kimi-code` | Root of the Kimi Code install destinations. Must be absolute, and may be neither `/` nor `$HOME`; anything else is refused. Nothing is written there yet (#112-#114) | `cli/cmd/paths.go` (`resolveKimiHome`) |
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

**Problem:** `no supported CLI detected (claude, opencode, kimi)` · **Cause:** none of them is on `PATH`, or the only one found is unusable — an older Kimi Code, or the legacy kimi-cli — in which case the error says which (`cli/cmd/targets.go`, `selectTargets`) · **Fix:** install one of them, or fix `PATH`.

**Problem:** `this run is not a complete install: Kimi Code CLI is not a fully supported install target yet (#110)` · **Cause:** Kimi Code was the only selected target. Its agents and skills *were* installed — the run names the directories — but MCP servers (#112) and hooks (#114) are not installed for it, and `./uninstall.sh` cannot remove it (#115), so the run refuses to call itself finished · **Fix:** nothing to fix — it is deliberate. Select another target too, or wait for those tickets.

**Problem:** the installer opens an interactive wizard · **Cause:** none of `--dry-run`, `--reinstall-mcps`, `--mcps-only`, `--agents-only`, `--skills-only`, `--target` was passed — `--model` alone doesn't count (`cli/cmd/install.go`, `flagsProvided`) · **Fix:** pass one of those flags for the non-interactive path. Without a terminal that path no longer prompts at all: it installs for every detected CLI unless `--target` says otherwise.

**Problem:** `[REQUIRED] ui-inspector — missing required env vars: UI_INSPECTOR_DIR` · **Cause:** the MCP's `required_env` isn't set (`mcps/registry.json:18`) · **Fix:** set it in `mcps/.env` and re-run `./install.sh --mcps-only`; add `--reinstall-mcps` if the MCP was already registered with an old value.

**Problem:** `devexp install` uses different assets than you expected — a release binary ignores your clone, a dev binary uses a different clone than the one you're in, or a warning says the checkout this binary was built from was skipped · **Cause:** the asset root is, in order: `$DEVEXP_DIR`; for dev builds only (`version` is `dev`, i.e. not built by goreleaser), the checkout the binary was compiled from, recorded at build time (`sourceRoot`), if it can be verified as yours (`ownedCheckout`, Unix only: the checkout root, `.devexp-toolkit`, `devexp.config.json`, `uninstall.sh`, and every directory and file under `agents/`, `skills/`, `mcps/` and `hooks/` are owned by you, are not symlinks, are not writable by others, and are writable by group only if that group is your *private group* — your primary group, named like your user, as with the umask `002` default of many Linux distributions; the directory holding the checkout is owned by you or root and passes the same write rule unless it is sticky; a symlinked checkout path must be your link in a directory passing the same check); otherwise the embedded assets, extracted to `<user cache dir>/devexp/assets` (tagged builds, reused by the same version) or `<user cache dir>/devexp/assets-dev` (dev builds, re-extracted every run). That path is a symlink to a complete extraction, swapped atomically, so an interrupted or concurrent run never leaves a partial tree; the tree it pointed at before is kept for an hour, so hooks started just before a swap keep working, then removed by a later run. The current directory and the binary's location are never searched. A `-trimpath` build records no checkout. `DEVEXP_DIR` and the compiled-from checkout must be devexp-toolkit checkouts: the `.devexp-toolkit` marker file at the root (a regular file whose first line is `devexp-toolkit`) plus `agents/`, `skills/` and `mcps/` (`isRepoDir`). A dev build warns when its checkout lacks the marker (a clone from before it existed), no longer exists (moved or deleted since the build), or can't be verified as yours (e.g. files writable by others, owned by another user, or writable by a group shared with other users — group write is accepted only for your private group, so a umask `002` clone on a distribution with user private groups is fine). With no absolute user cache dir it refuses instead of using a temp directory (`Resolve`, `sourceCheckout`, `embeddedDir` in `cli/internal/repo/repo.go`) · **Fix:** read the warning and the `Asset root: <dir> (<how it was chosen>)` line `devexp install` prints before installing anything (`announceAssetRoot` in `cli/cmd/install.go`). Set `DEVEXP_DIR` to the checkout you want, `git pull` a clone that lacks `.devexp-toolkit`, for one that can't be verified, fix what the warning names — typically `chmod -R o-w <checkout>` (and `g-w` if the group is shared), `chown` files you copied from another user, or the parent directory's mode — or rebuild a dev binary whose checkout moved: `rm bin/devexp && ./install.sh`.

**Problem:** `TestCommandExists` or the hook tests fail on a new machine · **Cause:** `TestCommandExists` expects `go` on `PATH` (`cli/cmd/install_test.go:115`); hook `.test.sh` files need `python3` · **Fix:** put `go` and `python3` on `PATH`. More in [`testing.md`](testing.md).
