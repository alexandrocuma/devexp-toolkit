---
name: release
description: Full release workflow — version bump, changelog, commit, tag, and platform release (GitHub, GitLab, or manual)
---

# Release Workflow

You are executing a **full release workflow**. This covers: determining the next version, updating version files, generating the changelog entry, committing the bump, creating the git tag, and optionally creating a platform release (GitHub, GitLab, or manual).

## Triggered by

- `changelog` agent — after changelog generation when tagging is needed
- `/release` — auto-determine version from commits
- `/release patch` — force a patch bump
- `/release minor` — force a minor bump
- `/release major` — force a major bump

## When to Use

When the user is ready to cut a new release: version bump, changelog, tag, and publish. Phrases: "release", "cut a release", "bump the version", "tag and release", "ship v2.0.0".

## Process

### 1. Pre-flight checks

```bash
# Ensure we're on the default branch (or the release branch)
git branch --show-current

# Ensure working tree is clean
git status --short

# Ensure we're up to date with remote
git fetch origin
git status -uno
```

If the working tree is not clean: **stop**. Report the uncommitted changes and ask the user to commit or stash them first.

If behind remote: **stop**. Ask the user to pull first.

### 2. Determine the current version

Check these locations in order:
```bash
cat package.json 2>/dev/null | grep '"version"'    # Node.js
cat go.mod 2>/dev/null | head -5                    # Go (module line)
cat pyproject.toml 2>/dev/null | grep "^version"   # Python
cat Cargo.toml 2>/dev/null | grep "^version"        # Rust
cat VERSION 2>/dev/null                             # Plain version file
git describe --tags --abbrev=0 2>/dev/null          # Last git tag
```

Report the current version found and its source.

### 3. Determine the next version

**If an explicit bump was provided** (`/release patch|minor|major`): apply it directly.

**If no bump provided**: analyze commits since last tag using conventional commit types:

```bash
LAST_TAG=$(git describe --tags --abbrev=0 2>/dev/null || echo "")
git log "${LAST_TAG}..HEAD" --format="%s" --no-merges
```

- Any `feat!:` or `BREAKING CHANGE:` → major
- Any `feat:` → minor
- Only `fix:`, `perf:`, `docs:`, `chore:` → patch
- No conventional commits → patch

Apply semver arithmetic to the current version.

**Show the user the plan and ask for confirmation before proceeding:**

```
Current version: v1.2.3
Commits since last release:
  feat: add Stripe webhooks
  fix: token refresh race condition
  docs: update auth guide

Recommended bump: MINOR (new feature)
Next version: v1.3.0

Proceed? (yes/no)
```

Wait for explicit confirmation before continuing.

### 4. Generate changelog entry

Run the changelog generation (same logic as `/changelog`):
- Parse commits since last tag
- Group by type
- Format as Keep a Changelog entry for the new version

Update `CHANGELOG.md` with the new versioned entry.

### 5. Update version files

Update the version string in every file that contains it:

**package.json:**
```bash
# Use npm version for node projects (handles package-lock.json too)
npm version <new-version> --no-git-tag-version
# or manually: sed -i 's/"version": ".*"/"version": "<new-version>"/' package.json
```

**go.mod**: Go uses tags, not a version field — no file update needed.

**pyproject.toml:**
```bash
sed -i 's/^version = ".*"/version = "<new-version>"/' pyproject.toml
```

**Cargo.toml:**
```bash
sed -i 's/^version = ".*"/version = "<new-version>"/' Cargo.toml
```

**VERSION file:**
```bash
echo "<new-version>" > VERSION
```

Report every file updated.

### 6. Commit the version bump

Stage and commit the version-related changes:
```bash
git add CHANGELOG.md package.json pyproject.toml Cargo.toml VERSION
# (only add files that were actually changed)

git commit -m "chore(release): v<new-version>

Release v<new-version>

$(cat <<'EOF'
Co-Authored-By: Claude <noreply@anthropic.com>
EOF
)"
```

### 7. Create the git tag

```bash
git tag -a "v<new-version>" -m "Release v<new-version>"
```

Annotated tags (not lightweight) — they carry metadata and show properly in `git describe`.

### 8. Push tag and commit

```bash
git push origin HEAD
git push origin "v<new-version>"
```

### 9. Create platform release (if requested or if CLI is available)

Extract the new changelog entry for the release notes:
```bash
NOTES=$(sed -n "/## \[<new-version>\]/,/## \[/p" CHANGELOG.md | head -n -1)
```

Detect the available platform CLI and create the release:

**GitHub (`gh` available):**
```bash
gh auth status 2>/dev/null
gh release create "v<new-version>" \
  --title "v<new-version>" \
  --notes "$NOTES" \
  --verify-tag
```

**GitLab (`glab` available):**
```bash
glab auth status 2>/dev/null
glab release create "v<new-version>" \
  --name "v<new-version>" \
  --notes "$NOTES"
```

**Neither available:** Provide the URL and release notes for the user to create the release manually in the platform's web UI.

### 10. Report

```
Release v<new-version> complete.

Files updated:
  - CHANGELOG.md
  - package.json (or equivalent)

Commits:
  - chore(release): v<new-version> (<sha>)

Tags:
  - v<new-version> (annotated)

Pushed:
  - origin/main
  - v<new-version> tag

Release: <platform-release-url>/releases/tag/v<new-version>
```

## Safety Rules

- Never proceed if the working tree is dirty
- Never tag without user confirmation of the version number
- Never force-push tags
- If any step fails, report the exact error and the state left behind — do not silently skip steps
- If `npm version` is used, it may auto-create a git commit — check with `git log -1` and don't double-commit
