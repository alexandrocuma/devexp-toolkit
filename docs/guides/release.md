# Release Guide

> Kit doc · Last verified: 2026-09-16 against commit `653387863c7fdc8d4e2ee1075bd2dc801c1e751b`
>
> Consumed by `/release` (executes ship steps), grooming (affected targets), `/deliver` (release readiness) and `/monitor` (post-release checks). Why release guides exist and how the lifecycle reads them: [`release-targets.md`](release-targets.md). Build and test commands: [`../development/setup.md`](../development/setup.md).

## Targets

| Target | Kind | Path | Channel | Rollback |
|--------|------|------|---------|----------|
| `cli` | cli | `cli/`, plus the assets embedded at build time: `agents/`, `skills/`, `hooks/`, `mcps/`, `devexp.config.json`, `uninstall.sh`, the `.devexp-toolkit` marker; build config `.goreleaser.yaml`, `.github/workflows/release.yml` | GitHub Releases for `alexandrocuma/devexp-toolkit`, built by goreleaser on a `v*` tag push; installed with `scripts/remote-install.sh` | hotfix-forward with a new patch tag; mark the bad release pre-release so `latest` falls back — see Target: cli |
| `toolkit-clone` | other | `agents/`, `skills/`, `hooks/`, `mcps/`, `templates/`, `devexp.config.json`, `devexp.config.schema.json`, `install.sh`, `uninstall.sh`, `scripts/` (and `cli/` for clone users, who build it locally) | the `main` branch — contributors and teams run `git pull` + `./install.sh` from a clone | `git revert` the offending merge on `main` — see Target: toolkit-clone |

Kinds: `library` · `cli` · `web` · `service` · `ios` · `android` · `desktop` · `other`

Asset edits reach the two targets at different times. Clone users get them on the next `git pull` (a binary built from the clone reads assets live from that clone — `Resolve` in `cli/internal/repo/repo.go`). Binary users get them only in the next tagged release, because goreleaser stages and embeds them at build time (`.goreleaser.yaml:5-7`, `scripts/stage-assets.sh:12-23`; `mcps/.env` is never embedded — `scripts/stage-assets.sh:19`).

## Cut (shared by all targets)

- Version source of truth: **the git tag.** There is no version file. goreleaser injects the tag's version at build time with `-X devexp/cmd.version={{ .Version }}` (`.goreleaser.yaml:25`, `cli/cmd/root.go:9-15`); local builds report `dev`. Scheme: SemVer (`CHANGELOG.md:6`).
- Tag format: `v<version>`, annotated, message `Release v<version> — <summary>` (`git tag -n1 v0.7.0 v0.6.0`). Any pushed `v*` tag starts the `release` workflow (`.github/workflows/release.yml:3-6`).
- Release commit: `chore: release v<version>`, committed directly on `main`, changing only `CHANGELOG.md`. It moves the `## [Unreleased]` entries under `## [<version>] - YYYY-MM-DD` (Keep a Changelog — `CHANGELOG.md:5,8,60`). See `git show --stat d5f6943 f88f123 879a5c8 d15c02d` (v0.7.0, v0.6.0, v0.5.0, v0.4.0).
- GitHub Release object: **created by `/release` as a draft right after the tag push**, with the version's `CHANGELOG.md` section as notes: `gh release create v<version> --draft --title v<version> --notes-file <section> --verify-tag`. A draft isn't public and doesn't move Latest. With `release.use_existing_draft: true` (`.goreleaser.yaml:49`), goreleaser looks for a draft whose name is its `name_template` (default `{{.Tag}}`, which matches `--title v<version>`), keeps its notes (default `release.mode` keep-existing), uploads the assets while it is still a draft, and only then publishes it; it becomes Latest at that moment. Source, goreleaser v2.18.2 (the `~> v2` the action installs today): `findRelease` → `findDraftRelease` matches `r.GetDraft() && r.GetName() == name`, `createOrUpdateRelease` keeps `Draft` and `getReleaseNotes` returns the existing body, `doPublish` uploads and then calls `PublishRelease` with `Draft: false` (`internal/client/github.go`, `internal/client/release_notes.go`, `internal/pipe/release/release.go`). Before this, `/release` created a published release and goreleaser uploaded into it — how v0.7.1, v0.7.0 and v0.5.0 were released. v0.6.0 was the exception: goreleaser created it with its own commit-list notes, which leave out `docs:`, `test:` and `chore:` commits (`.goreleaser.yaml:36-42`).
- Nothing waits for CI before the tag. `main` has no branch protection or rulesets (GitHub API, checked 2026-09-16), and `release.yml` runs no tests; its only check is the `govulncheck` job before goreleaser (see Target: cli → Build). For v0.7.0, `ci` started on the release commit at 04:48:20Z and `release` started from the tag at 04:48:23Z (`gh run list`).

## Target: cli

- **Kind / path:** cli — `cli/` and the embedded asset dirs listed in Targets (changes under these paths affect this target)
- **Versioning:** the tag only; no build number. `devexp --version` prints `devexp version <version>` without the leading `v` (`cli/cmd/root.go:20`, goreleaser `.Version`; verified with v0.7.0)
- **Prerequisites:** push access to `origin` (`github.com/alexandrocuma/devexp-toolkit`) for `main` and tags · `GITHUB_TOKEN`, supplied by GitHub Actions (`.github/workflows/release.yml:50`) with `contents: write` on the goreleaser job only (`.github/workflows/release.yml:32-33`; the workflow default is `contents: read`, lines 8-9) · `gh auth status` for creating the release and for verification

### Build

The tag push from the cut starts the build. Watch it; don't start it again (re-running failed jobs of the same run is fine, see below).

```bash
gh run list --workflow release.yml --limit 1   # watch the tag-triggered run until completed/success   # source: .github/workflows/release.yml:1-6
# CI runs: job govulncheck (read-only): ./scripts/govulncheck.sh → job goreleaser (needs: govulncheck): ./scripts/stage-assets.sh (before-hook) → goreleaser release --clean → publishes the draft   # source: .github/workflows/release.yml:14-50, .goreleaser.yaml:5-7,44-49
```

The `govulncheck` job runs `scripts/govulncheck.sh`, the same check as in `ci.yml`, with a read-only token and no persisted credentials. A new advisory can appear between the PR's CI run and the tag, which is why it runs again here. Until goreleaser publishes, the release is only the draft `/release` created: nothing is public and Latest doesn't move.

If the run fails, read which job and exit status:

- **`govulncheck` exited 1 (infrastructure, not a finding)** — a fetch or network error (vuln.go.dev, the module proxy), marked `infrastructure failure, not a finding` in the log. Re-run: `gh run rerun <run-id> --failed`. This re-runs `govulncheck` and then `goreleaser`, which still finds the draft.
- **`govulncheck` exited 3 (called vulnerability), or `goreleaser` failed and can't be fixed by re-running** — this tag won't ship:
  1. Delete the leftover draft: `gh release delete v<version> --yes`. The tag stays; a draft was never public, so this isn't the deletion Rollback forbids.
  2. Fix on a branch — for a finding, as described in [`../development/testing.md`](../development/testing.md#vulnerability-scan) — and merge it.
  3. Cut `v<next patch>`. Don't move or re-push the failed tag (tags are never moved — see Rollback).

Artifact: `devexp-toolkit_darwin_amd64.tar.gz`, `devexp-toolkit_darwin_arm64.tar.gz`, `devexp-toolkit_linux_amd64.tar.gz`, `devexp-toolkit_linux_arm64.tar.gz` (each holds a static `devexp` binary, `CGO_ENABLED=0`, built with `-trimpath` so it records no build paths, stripped with `-s -w`, compiled with the Go `toolchain` from `cli/go.mod` via `go-version-file` in `release.yml`; check a binary with `go version devexp`) plus `checksums.txt`, attached to the tag's GitHub Release (`.goreleaser.yaml:9-34,44-52`).

### Distribute

N/A — there is no pre-production channel. The draft `/release` creates is only a staging area for the upload, not a channel: `.goreleaser.yaml` sets no `release.draft`, prerelease or snapshot option, so goreleaser publishes the draft as soon as the assets are uploaded (`.goreleaser.yaml:44-52`).

### Promote

| Stage | How | Gate |
|-------|-----|------|
| tag push + draft → published GitHub Release, marked **Latest** | automatic once the `v*` tag is pushed (`.github/workflows/release.yml`): goreleaser uploads into the draft, then publishes it. New `remote-install.sh` installs pick it up at once, because the script resolves `releases/latest` (`scripts/remote-install.sh:51-55`), and the assets are already there when it turns Latest | manual — the tag push at the cut gate. One automated gate: the `govulncheck` job must pass before the goreleaser job runs (`needs: govulncheck`, `.github/workflows/release.yml:29-30`). No tests in `release.yml` and no branch protection |

### Rollback

Strategy: **hotfix-forward.** A pushed tag and its GitHub Release are never deleted or moved — users may already have installed them, and `remote-install.sh` pins by tag. A bad release is replaced by the next patch release through the normal cut. Until that ships:

1. Mark the bad release as a pre-release. GitHub's `releases/latest` excludes pre-releases, so new `remote-install.sh` installs fall back to the previous release (`scripts/remote-install.sh:53`).
2. Tell users who already installed it to pin the last good release with `DEVEXP_VERSION` (`scripts/remote-install.sh:50`).
3. Fix on a branch, merge, and cut `v<next patch>`; the new release becomes `latest` again.

```bash
gh release edit v<bad> --prerelease                      # latest falls back to the previous release
gh release list --limit 3                                # verify: v<previous> shows as Latest
# users on the bad version:
DEVEXP_VERSION=v<previous> bash <(curl -fsSL https://raw.githubusercontent.com/alexandrocuma/devexp-toolkit/main/scripts/remote-install.sh)
# after the fix: /release cuts v<next patch>, which becomes Latest; leave v<bad> marked pre-release
```

### Post-release verification

Each check was run against v0.7.0 on 2026-09-16.

| Signal | Where | Healthy when |
|--------|-------|--------------|
| Release workflow | `gh run list --workflow release.yml --limit 1` | `completed success` for the new tag (`.github/workflows/release.yml:1`) |
| Release assets | `gh release view v<version> --json assets,isDraft,isPrerelease` | 5 assets — the 4 `devexp-toolkit_<os>_<arch>.tar.gz` + `checksums.txt`; `isDraft` false once the workflow has finished (goreleaser published the draft — still `true` means the run failed, see Build), `isPrerelease` false (`.goreleaser.yaml:16-34,44-52`) |
| Latest resolution | `gh release list --limit 1` | the new tag shows as `Latest`, not `Draft` — what `remote-install.sh` installs by default (`scripts/remote-install.sh:53`) |
| Binary downloads and reports its version | `DEVEXP_VERSION=v<version> DEVEXP_INSTALL_DIR="$(mktemp -d)" DEVEXP_SKIP_RUN=1 bash scripts/remote-install.sh` | exits 0 and prints `devexp version <version>` (`scripts/remote-install.sh:61-83`); a temp install dir and `DEVEXP_SKIP_RUN` keep `~/.local/bin` and `~/.claude` untouched |

## Target: toolkit-clone

- **Kind / path:** other — `agents/`, `skills/`, `hooks/`, `mcps/`, `templates/`, `devexp.config.json`, `devexp.config.schema.json`, `install.sh`, `uninstall.sh`, `scripts/` (changes under these paths affect this target)
- **Versioning:** none. A clone tracks `main`; its locally built binary reports `dev` (`cli/cmd/root.go:15`). Changes waiting for release collect under `## [Unreleased]` in `CHANGELOG.md:8`
- **Prerequisites:** push access to `main` on `origin`. Consumers need the contributor toolchain in [`../development/setup.md`](../development/setup.md#prerequisites)

Every merge to `main` ships this target; the release cut only adds the changelog entry and tag. `scripts/remote-install.sh` also belongs here: the documented one-liner downloads it from `main` (`scripts/remote-install.sh:7`), so a change to it reaches binary installers when it lands on `main`, not when a tag is cut.

### Build

N/A — there is nothing to build centrally. In a clone the CLI built by `./install.sh` reads assets live from that clone (`Resolve` in `cli/internal/repo/repo.go`). Each user's `./install.sh` builds `bin/devexp` only if it is missing (`install.sh:7-20`), so after Go changes users must `rm bin/devexp` first.

### Distribute

N/A — no pre-production branch or channel. Work reaches `main` through pull requests, and `ci` runs on each one (`.github/workflows/ci.yml:3-4`).

### Promote

| Stage | How | Gate |
|-------|-----|------|
| push to `main` | `git push` of the release commit at the cut (feature work lands earlier through squash-merged PRs, `(#NN)` subjects in `git log --first-parent origin/main`) | `ci` on the PR and on push to `main` (`.github/workflows/ci.yml:3-6`). It isn't enforced: `main` has no branch protection or rulesets |

### Rollback

Strategy: **revert on `main`.** Undo the offending change with `git revert` (history is never rewritten on `main`), land it through a PR like any other change, and have clone users pull and rebuild. If the bad change also shipped in a CLI tag, roll that target back too (see Target: cli).

```bash
git revert <sha>              # a squash-merged PR (one commit on main)
git revert -m 1 <merge-sha>   # a merge commit
# open a PR with the revert; ci must pass before merging
# clone users after it lands:
git pull && rm -f bin/devexp && ./install.sh   # install.sh never rebuilds an existing binary (install.sh:7-20)
```

### Post-release verification

| Signal | Where | Healthy when |
|--------|-------|--------------|
| CI on `main` | `gh run list --workflow ci.yml --branch main --limit 1` | `completed success` on the release commit (`.github/workflows/ci.yml:3-6`) |
| Installer from a clone at the tag | `rm -f bin/devexp && ./install.sh --dry-run` | builds without error and ends with `All done.`; `[REQUIRED]` notices for unset MCP env vars are warnings, not failures (`install.sh:7-22`, `cli/internal/ui/output.go:28`; verified at this commit) |
