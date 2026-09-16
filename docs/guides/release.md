# Release Guide

> Kit doc · Last verified: 2026-09-16 against commit `a86d2c3f6a41a6d033d31afd858ff723d5267dd7`
>
> Consumed by `/release` (executes ship steps), grooming (affected targets), `/deliver` (release readiness) and `/monitor` (post-release checks). Why release guides exist and how the lifecycle reads them: [`release-targets.md`](release-targets.md). Build and test commands: [`../development/setup.md`](../development/setup.md).

## Targets

| Target | Kind | Path | Channel | Rollback |
|--------|------|------|---------|----------|
| `cli` | cli | `cli/`, plus the assets embedded at build time: `agents/`, `skills/`, `hooks/`, `mcps/`, `devexp.config.json`, `uninstall.sh`; build config `.goreleaser.yaml`, `.github/workflows/release.yml` | GitHub Releases for `alexandrocuma/devexp-toolkit`, built by goreleaser on a `v*` tag push; installed with `scripts/remote-install.sh` | `[CONFIRM] no rollback strategy defined — see Target: cli` |
| `toolkit-clone` | other | `agents/`, `skills/`, `hooks/`, `mcps/`, `templates/`, `devexp.config.json`, `devexp.config.schema.json`, `install.sh`, `uninstall.sh`, `scripts/` (and `cli/` for clone users, who build it locally) | the `main` branch — contributors and teams run `git pull` + `./install.sh` from a clone | `[CONFIRM] no rollback strategy defined — see Target: toolkit-clone` |

Kinds: `library` · `cli` · `web` · `service` · `ios` · `android` · `desktop` · `other`

Asset edits reach the two targets at different times. Clone users get them on the next `git pull` (the CLI reads assets live from disk — `cli/internal/repo/repo.go:45-63`). Binary users get them only in the next tagged release, because goreleaser stages and embeds them at build time (`.goreleaser.yaml:5-7`, `scripts/stage-assets.sh:12-22`; `mcps/.env` is never embedded — `scripts/stage-assets.sh:19`).

## Cut (shared by all targets)

- Version source of truth: **the git tag.** There is no version file. goreleaser injects the tag's version at build time with `-X devexp/cmd.version={{ .Version }}` (`.goreleaser.yaml:23`, `cli/cmd/root.go:9-15`); local builds report `dev`. Scheme: SemVer (`CHANGELOG.md:6`).
- Tag format: `v<version>`, annotated, message `Release v<version> — <summary>` (`git tag -n1 v0.7.0 v0.6.0`). Any pushed `v*` tag starts the `release` workflow (`.github/workflows/release.yml:3-6`).
- Release commit: `chore: release v<version>`, committed directly on `main`, changing only `CHANGELOG.md`. It moves the `## [Unreleased]` entries under `## [<version>] - YYYY-MM-DD` (Keep a Changelog — `CHANGELOG.md:5,8,60`). See `git show --stat d5f6943 f88f123 879a5c8 d15c02d` (v0.7.0, v0.6.0, v0.5.0, v0.4.0).
- GitHub Release object: `[INCONSISTENT — created by the maintainer with the CHANGELOG entry as notes, with goreleaser then uploading the assets to it (v0.7.0, v0.5.0) vs created by goreleaser with its own commit-list notes (v0.6.0)]`. Both produced the full asset set. The latest release (v0.7.0) follows the first pattern, which is what `/release` Phase 6 does (`gh release create … --notes "<changelog entry>"`, `skills/release/SKILL.md:191`). goreleaser kept the existing notes. When goreleaser writes the notes itself, it leaves out `docs:`, `test:` and `chore:` commits (`.goreleaser.yaml:34-40`).
- Nothing waits for CI before the tag. `main` has no branch protection or rulesets (GitHub API, checked 2026-09-16), and `release.yml` runs no tests. For v0.7.0, `ci` started on the release commit at 04:48:20Z and `release` started from the tag at 04:48:23Z (`gh run list`).

## Target: cli

- **Kind / path:** cli — `cli/` and the embedded asset dirs listed in Targets (changes under these paths affect this target)
- **Versioning:** the tag only; no build number. `devexp --version` prints `devexp version <version>` without the leading `v` (`cli/cmd/root.go:20`, goreleaser `.Version`; verified with v0.7.0)
- **Prerequisites:** push access to `origin` (`github.com/alexandrocuma/devexp-toolkit`) for `main` and tags · `GITHUB_TOKEN`, supplied by GitHub Actions (`.github/workflows/release.yml:30`) with `contents: write` (`.github/workflows/release.yml:8-9`) · `gh auth status` for creating the release and for verification

### Build

The tag push from the cut starts the build. Watch it; don't run it again.

```bash
gh run list --workflow release.yml --limit 1   # watch the tag-triggered run until completed/success   # source: .github/workflows/release.yml:1-6
# CI runs: ./scripts/stage-assets.sh (goreleaser before-hook) → goreleaser release --clean   # source: .goreleaser.yaml:5-7, .github/workflows/release.yml:24-28
```

Artifact: `devexp-toolkit_darwin_amd64.tar.gz`, `devexp-toolkit_darwin_arm64.tar.gz`, `devexp-toolkit_linux_amd64.tar.gz`, `devexp-toolkit_linux_arm64.tar.gz` (each holds a static `devexp` binary, `CGO_ENABLED=0`, stripped with `-s -w`) plus `checksums.txt`, attached to the tag's GitHub Release (`.goreleaser.yaml:9-32,42-45`).

### Distribute

N/A — there is no pre-production channel. `.goreleaser.yaml` sets no draft, prerelease or snapshot option, and `release.yml` publishes straight to the tag's GitHub Release (v0.7.0: not a draft, not a prerelease).

### Promote

| Stage | How | Gate |
|-------|-----|------|
| tag push → published GitHub Release, marked **Latest** | automatic once the `v*` tag is pushed (`.github/workflows/release.yml`). New `remote-install.sh` installs pick it up at once, because the script resolves `releases/latest` (`scripts/remote-install.sh:40-44`) | manual — the tag push at the cut gate. No automated gate (no branch protection; no tests in `release.yml`) |

### Rollback

Strategy: `[CONFIRM] rollback strategy for a bad CLI release — the repo defines none. What exists (not documented as policy): users can pin an earlier tag with DEVEXP_VERSION=v<previous> (scripts/remote-install.sh:39); new installs get whichever release GitHub marks Latest (scripts/remote-install.sh:42); or hotfix-forward with a new patch tag`

```bash
[CONFIRM] no rollback commands defined
```

### Post-release verification

Each check was run against v0.7.0 on 2026-09-16.

| Signal | Where | Healthy when |
|--------|-------|--------------|
| Release workflow | `gh run list --workflow release.yml --limit 1` | `completed success` for the new tag (`.github/workflows/release.yml:1`) |
| Release assets | `gh release view v<version> --json assets,isDraft,isPrerelease` | 5 assets — the 4 `devexp-toolkit_<os>_<arch>.tar.gz` + `checksums.txt`; `isDraft` and `isPrerelease` false (`.goreleaser.yaml:16-32`) |
| Latest resolution | `gh release list --limit 1` | the new tag shows as `Latest` — what `remote-install.sh` installs by default (`scripts/remote-install.sh:42`) |
| Binary downloads and reports its version | `DEVEXP_VERSION=v<version> DEVEXP_INSTALL_DIR="$(mktemp -d)" DEVEXP_SKIP_RUN=1 bash scripts/remote-install.sh` | exits 0 and prints `devexp version <version>` (`scripts/remote-install.sh:50-72`); a temp install dir and `DEVEXP_SKIP_RUN` keep `~/.local/bin` and `~/.claude` untouched |

## Target: toolkit-clone

- **Kind / path:** other — `agents/`, `skills/`, `hooks/`, `mcps/`, `templates/`, `devexp.config.json`, `devexp.config.schema.json`, `install.sh`, `uninstall.sh`, `scripts/` (changes under these paths affect this target)
- **Versioning:** none. A clone tracks `main`; its locally built binary reports `dev` (`cli/cmd/root.go:15`). Changes waiting for release collect under `## [Unreleased]` in `CHANGELOG.md:8`
- **Prerequisites:** push access to `main` on `origin`. Consumers need the contributor toolchain in [`../development/setup.md`](../development/setup.md#prerequisites)

Every merge to `main` ships this target; the release cut only adds the changelog entry and tag. `scripts/remote-install.sh` also belongs here: the documented one-liner downloads it from `main` (`scripts/remote-install.sh:7`), so a change to it reaches binary installers when it lands on `main`, not when a tag is cut.

### Build

N/A — there is nothing to build centrally. In a clone the CLI reads assets live from disk (`cli/internal/repo/repo.go:45-63`). Each user's `./install.sh` builds `bin/devexp` only if it is missing (`install.sh:7-18`), so after Go changes users must `rm bin/devexp` first.

### Distribute

N/A — no pre-production branch or channel. Work reaches `main` through pull requests, and `ci` runs on each one (`.github/workflows/ci.yml:3-4`).

### Promote

| Stage | How | Gate |
|-------|-----|------|
| push to `main` | `git push` of the release commit at the cut (feature work lands earlier through squash-merged PRs, `(#NN)` subjects in `git log --first-parent origin/main`) | `ci` on the PR and on push to `main` (`.github/workflows/ci.yml:3-6`). It isn't enforced: `main` has no branch protection or rulesets |

### Rollback

Strategy: `[CONFIRM] rollback strategy for a bad change on main — the repo defines none (git revert on main followed by users re-running git pull && ./install.sh is the git-native option, not documented as policy)`

```bash
[CONFIRM] no rollback commands defined
```

### Post-release verification

| Signal | Where | Healthy when |
|--------|-------|--------------|
| CI on `main` | `gh run list --workflow ci.yml --branch main --limit 1` | `completed success` on the release commit (`.github/workflows/ci.yml:3-6`) |
| Installer from a clone at the tag | `rm -f bin/devexp && ./install.sh --dry-run` | builds without error and ends with `All done.`; `[REQUIRED]` notices for unset MCP env vars are warnings, not failures (`install.sh:7-20`, `cli/internal/ui/output.go:28`; verified at this commit) |
