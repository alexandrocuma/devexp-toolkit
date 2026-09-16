---
name: release
description: Release phase — takes approved work from merge to shipped: worktree merge, changelog, version bump, tag, then ships each affected release target (deploy, app-store/beta channels, package publish) from the repo's release guide with a gate per target, and retires the delivery's artifacts. Runs inside /deliver or standalone to finish a deferred or externally-gated release.
argument-hint: "[ticket]"
---

# Release: Approved Work → Shipped

You are the **Release Manager**. You take work that is already written, reviewed and approved, and you put it in front of users: merge, changelog, version, tag — then ship it to every target it affects — then retire what the delivery left behind.

A release is two stages:

- **Cut** — the same for every repo: merge, changelog, version bump, tag. It produces one versioned, immutable point in history.
- **Ship** — different for every target. A service is deployed; a web app is published to its hosting; an iOS or Android app is built, uploaded to a beta channel, then promoted through store review and a staged rollout; a library is published to its registry. *How* each target ships is not known to this skill — it is read from the repo's **release guide**, `docs/guides/release.md`, which `/devxp` writes and keeps current.

Release is the one phase that touches shared systems and cannot be undone. The cut is gated once; every target is gated on its own; every production-facing step inside a target is confirmed on its own. Every failure path preserves state rather than cleaning up.

## Triggered by

- `/release` — release the current branch's work
- `/release <ticket>` — release a specific ticket, resolving its worktree and branch by id
- `/deliver` Phase 6 — delegates here once code review passes
- `changelog` agent — chains here when it detects a version bump is needed

## When to Use

- **Inside a delivery** — `/deliver` reaches the release gate and hands off.
- **Resuming a deferred release** — a previous `/deliver` run ended with *"I'll release manually."* This is the command that finishes it. Without it, the only options are re-running `/deliver` (which would redo implementation) or doing it by hand — which is how worktrees accumulate.
- **Resuming an externally-gated release** — a target was left **awaiting-external** (store review, staged rollout in progress). Re-running `/release <ticket>` picks up at that target's next promote stage.
- **Releasing work merged by hand** — the branch already landed and the bookkeeping (changelog, version, tag, shipping) is still outstanding.

**Out of scope** — do not reach for `/release` when:
- The work is not reviewed. Release is not a review gate; run `/deliver` Phase 5 first.
- You want to cut a changelog without releasing — that is the `changelog` agent on its own.
- You want to retire worktrees from *finished* work with no release pending — that is `/cleanup`.
- You want to define or fix *how* the repo ships — that is `/devxp` (it writes the release guide). This skill never invents ship steps.

---

## Process

### Phase 0 — Bind Scope

Resolve the ticket, the branch, the base and the targets before touching anything. Every later step is keyed to these.

```bash
ticket="<ticket-id-or-empty>"; type="feat"          # feat | fix | docs | chore — match the ticket
main="$(git worktree list --porcelain | head -1 | cut -d' ' -f2)"   # main checkout is always first
base="$(git symbolic-ref --short refs/remotes/origin/HEAD 2>/dev/null | sed 's@^origin/@@')"; base="${base:-main}"
```

- **With a ticket id** — the branch is `<type>/<ticket>` and the worktree is `../<repo>-worktrees/<ticket>`. Confirm both exist; if the worktree is gone but the branch remains, this is a resumed release and there is nothing to merge from a tree.
- **Without a ticket id** — release the current branch. Say which branch that is before going further.
- **Single-stream** — if the work was delivered in place with no worktree, the merge step is a no-op and everything else applies unchanged.

**Return to the main checkout first.** `git worktree remove` refuses to delete a tree you are standing in, and every step below operates on the base branch:

```bash
cd "$main"
```

**Load the release guide and bind the targets:**

```bash
cat docs/guides/release.md 2>/dev/null || echo "release guide: MISSING"
cat ~/.claude/agent-memory/grooming-agent/plans/"$ticket".md 2>/dev/null | sed -n '/Affected Release Targets/,/^## /p'
```

| State | Targets to ship |
|-------|-----------------|
| Guide + groom plan lists affected targets | exactly those targets |
| Guide, no plan section | map the branch's changed paths (`git diff --name-only "$base"...<branch>`) onto each target's path in the guide; show the mapping |
| **No guide** | offer: *"This repo has no release guide, so I can only cut (tag + platform release). Run `/devxp` to write one first?"* — on decline, proceed **cut-only**, exactly as a library release |

**Resumed release:** if a previous run recorded target states for this ticket (`~/.claude/agent-memory/release/<ticket>.md`), load them — `shipped` targets are skipped, `awaiting-external` targets resume at their next promote stage, and the cut is skipped if the tag already exists (`git rev-parse -q --verify "refs/tags/v<version>"`).

---

### Phase 1 — Preflight  *(read-only)*

Establish what is true before asking for a decision. Report each, never assume.

```bash
git merge-base --is-ancestor "${type}/${ticket}" "$base" 2>/dev/null && echo "MERGED" || echo "NOT MERGED"
git log "$(git describe --tags --abbrev=0 2>/dev/null)"..HEAD --oneline --no-merges 2>/dev/null | head -20
ls package.json go.mod pyproject.toml Cargo.toml version.go VERSION 2>/dev/null | head -3
gh auth status >/dev/null 2>&1 && echo "platform: github" || { glab auth status >/dev/null 2>&1 && echo "platform: gitlab" || echo "platform: none — tag only"; }
```

**Per target**, check the guide's **Prerequisites** read-only: the named CLI reports an authenticated session (its own status/whoami subcommand), the named CI secret or connector is present. Never trigger an auth flow and never ask for a credential. A target whose prerequisites are missing, or whose steps still contain a `[CONFIRM]` marker, is **blocked** — it is reported and not attempted; the rest of the release can still proceed.

Emit the inventory:

```
Release preflight — <ticket or branch>
  Branch         : <type>/<ticket> — <MERGED into base | NOT MERGED>
  Worktree       : <path | none (single-stream) | already removed>
  Commits since  : <last tag> — N commits (N feat, N fix, N breaking)
  Version file   : <path | none detected>
  Current version: <vX.Y.Z> → proposed <vX.Y.Z> (<patch|minor|major>)
  Platform       : <github | gitlab | none — tag only>
  Release guide  : <docs/guides/release.md, last verified YYYY-MM-DD | missing — cut only>

  Targets:
  | Target | Kind    | Build no.       | Channel → production              | Rollback             | Status   |
  |--------|---------|-----------------|-----------------------------------|----------------------|----------|
  | api    | service | —               | staging → production              | redeploy previous    | ready    |
  | ios    | ios     | 41 → 42         | beta → store review → phased      | halt phased / flag   | ready    |
  | android| android | 1041 → 1042     | internal → production staged 10%  | halt rollout / flag  | blocked — distribution CLI not authenticated |
```

**Version bump is derived, not guessed:** any breaking change → major; any `feat:` → minor; otherwise patch. If commits are not conventional, say so and propose a bump from the diff's shape rather than inventing one. Platform build numbers follow each target's **Versioning** rule in the guide.

---

### Phase 2 — The Cut Gate  *(requires explicit confirmation)*

```
Ready to release <version>?
  Cut:
    Step 1: Merge <type>/<ticket> into <base>
    Step 2: Changelog entry from commits since <last tag>
    Step 3: Bump version to <vX.Y.Z> (+ build numbers: <target: N → N+1, …>)
    Step 4: Tag and publish on <platform>
  Then ship, each target gated on its own:
    <target> → <channel → production>
    <target> → blocked (<reason>) — will not be attempted
  On success: retire the worktree, branch, plan and scratch
  On any failure: everything is preserved for inspection

Confirm release? (yes / no — I'll release manually)
```

Wait for an explicit **yes**. Anything else is a decline. A yes here authorizes the **cut only** — it never authorizes shipping a target.

**On decline, never just stop** — leave the worktree in a state the user can reason about:

- **Branch already merged** — the remaining steps are bookkeeping on the base branch; the tree is finished. Remove it now, with the success path's own commands:
  ```bash
  git worktree remove "/abs/path/to/<repo>-worktrees/<ticket>"
  git branch -d "<type>/<ticket>"
  ```
  Then go to Phase 9 and report the release as deferred.
- **Branch not merged** — keep the worktree and say so plainly: the branch still needs its PR merged, and the tree stays live until then. Tell the user that **`/release <ticket>` finishes this later**, and that once the branch lands `/cleanup <ticket>` retires the tree if they release by hand instead.

Declining the gate must never silently orphan a tree.

---

### Phase 3 — Merge  *(skip if already merged, or single-stream)*

Integrate through the project's normal path — merge the open PR, or a direct merge where no PR workflow exists.

- **Merges are serialized.** If other worktrees are also ready, merge one at a time so each sees a consistent base.
- **A conflict stops the run and surfaces to the user. Never auto-resolve** — silently picking a side risks discarding correct work.

---

### Phase 4 — Changelog

Delegate to the `changelog` agent, which parses conventional commits, groups by type with Breaking Changes first, and writes Keep a Changelog format:

> "Generate the changelog entry for `<version>` from commits since `<last tag>`. Write it to CHANGELOG.md under the new version heading, moving any `[Unreleased]` contents into it."

If the agent is unavailable, do it inline: group commits by `feat` / `fix` / `perf`, omit `test`/`style`/`ci`, put breaking changes first, and prepend the entry to `CHANGELOG.md` (creating it if absent).

**Never generate an empty section.** If nothing changed in a category, leave it out.

---

### Phase 5 — Version Bump

Update the version file detected in Phase 1 to the version confirmed at the gate. If no version file exists, the tag is the version — say so rather than inventing a file.

Then apply each affected target's **Versioning** rule from the guide — typically a platform build number that must increase on every upload (an iOS build number, an Android version code). Bump only the targets being shipped, in the files the guide names, and include them in the release commit.

---

### Phase 6 — Tag and Publish

```bash
git add CHANGELOG.md <version-file> <target build-number files>
git commit -m "chore: release v<version>"
git tag -a "v<version>" -m "Release v<version>"
git push && git push --tags
```

Publish on the platform detected in Phase 1:

```bash
gh release create  "v<version>" --title "v<version>" --notes "<changelog entry>"   # GitHub
glab release create "v<version>" --name  "v<version>" --notes "<changelog entry>"  # GitLab
```

With no platform detected, the tag **is** the release. Report that rather than failing.

**Tag push is the point of no return for the cut.** Everything before it is local and revertible; everything after is public. If a step here fails, stop — do not proceed to Phase 7, and do not retry a push that may have partially succeeded without checking `git ls-remote --tags origin` first.

If the guide says a target's pipeline is **triggered by the tag** (CI builds and deploys on tag push), the push has already started that target's ship — Phase 7 then *watches* that pipeline instead of running its build/distribute commands.

---

### Phase 7 — Ship Targets  *(each target gated on its own; skipped when cut-only)*

Ship the targets bound in Phase 0, in the order the guide lists them (the guide or groom plan may require an order — e.g. a backend before the app that depends on it). Blocked targets are skipped and reported.

**The guide is the only source of commands.** Run exactly what the target's section says; never improvise, substitute or "fix" a ship command. A step marked `[CONFIRM]` is not executed. If a guide command fails because it is wrong, stop that target and send the user to `/devxp` to correct the guide.

For each target:

**7a. Target gate** — show the plan *including the rollback* before anything runs:

```
Ship <target> (<kind>) — v<version> (build <N>)
  Build:       <command from guide>
  Distribute:  <channel> — <command>
  Promote:     <stage → stage → production> — gates: <manual approval / external review / staged %>
  Verify:      <post-release signals>
  Rollback:    <strategy> — <command or steps>

Ship <target>? (yes / skip / stop)
```

`skip` records the target as **skipped** and moves on; `stop` ends shipping and preserves everything.

**7b. Build → Distribute** — run the guide's build command, confirm the artifact it names exists, then distribute to the pre-production channel (staging, beta testers, internal track). Report the result of each step before the next.

**7c. Promote** — walk the guide's promote stages in order. **Every stage that reaches production or real users is its own confirmation**, even inside a target the user already said yes to:

```
<target>: <stage> is live and verified. Promote to <next stage>? (yes / hold)
```

- **External gate** (store review, change-approval board, anything this session cannot complete): submit if the guide says how, then record the target as **awaiting-external** with the stage it is waiting on. This is a valid outcome, not a failure — tell the user `/release <ticket>` resumes from here once the gate clears.
- **Staged rollout** (a percentage or phased release): promote to the first stage the guide names, verify, and record **awaiting-external** at that percentage. Widening the rollout is a later `/release <ticket>` run, each step confirmed.
- **hold** records the target as **awaiting-external** at the current stage.

**7d. Verify** — after each stage that reaches real traffic, check the guide's **Post-release verification** signals read-only (health endpoint, error rate, crash-free sessions) against their healthy threshold. Report the values observed, not a bare pass.

**7e. On failure** — a build, distribute, promote or verify step fails, or a signal is outside its threshold:

1. Stop this target. Do not retry a distribute or promote step that may have partially applied without first checking the channel's current state.
2. Print the guide's **Rollback** steps for this target and ask: *"Run rollback for <target>? (yes / no — I'll handle it)"*. **Never roll back automatically**, and never run a rollback the guide does not define.
3. Record the target as **failed at <step>** (with rollback run / not run), and ask whether to continue with the remaining targets or stop.

**Persist target state after every change**, so a later run can resume:

```bash
mkdir -p ~/.claude/agent-memory/release
# ~/.claude/agent-memory/release/<ticket>.md — version, tag, and one line per target:
# <target> | <shipped | awaiting-external @ <stage> | blocked: <reason> | skipped | failed at <step>> | <timestamp>
```

---

### Phase 8 — Retire Delivery Artifacts  *(only when the release is complete)*

**Gate strictly on completion.** The release is complete when the cut succeeded **and** every bound target is `shipped` or explicitly `skipped`. If any step failed or was declined, or any target is `awaiting-external`, `blocked` or `failed`, skip this phase entirely and preserve everything — worktree, plan, scratch, release state — for the resumed run. Scope every action to this ticket; the repo-wide sweep belongs to `/improve` (C2) and `/cleanup`.

**Safety gate — bind and verify the id before any deletion.** Every `rm` below is keyed to `$ticket`. An empty id turns an id-scoped glob into a blanket wipe (`/tmp/*$ticket*` → `/tmp/*`), so **abort cleanup entirely if the id is empty or unsafe:**

```bash
case "$ticket" in
  ""|*[!A-Za-z0-9_-]*) echo "ticket id missing or unsafe — skipping all artifact cleanup"; return 2>/dev/null || exit 0 ;;
esac
```

With `$ticket` verified, retire each artifact:

1. **Worktree and branch** — removed as part of the merge; confirm they are gone:
   ```bash
   git worktree list          # this ticket's tree should no longer appear
   git branch -d "${type}/${ticket}" 2>/dev/null
   ```
2. **Persisted plan** — the plan described pre-merge intent; once delivered it only invites drift:
   ```bash
   rm -f ~/.claude/agent-memory/grooming-agent/plans/"$ticket".md
   ```
   The ticket-platform copy is a tracker action, not a filesystem delete — **ask first**, then remove it via the platform's CLI.
3. **Groom-session artifacts** — **prefix-anchor** the glob (`"$ticket"-*`, never `*"$ticket"*`) so it can never match the project's shared memory file:
   ```bash
   rm -f ~/.claude/agent-memory/grooming-agent/sessions/"$ticket"-* ~/.claude/agent-memory/grooming-agent/"$ticket".scratch 2>/dev/null
   ```
4. **Release state** — the per-target record is only needed to resume:
   ```bash
   rm -f ~/.claude/agent-memory/release/"$ticket".md
   ```
5. **`/tmp` scratch** — only this run's id-prefixed scratch, never a leading-wildcard glob:
   ```bash
   rm -f /tmp/.deliver-"$ticket"-* /tmp/.groom-"$ticket"-* /tmp/.release-"$ticket"-* 2>/dev/null
   ```
6. **Agent-memory entries** — prune entries tied **to this ticket only**, and only where the delivered work made them stale. Everything else is out of scope.

---

### Phase 9 — Report

```
## Released: <version> — <ticket or branch>

  Merge:        <merged into <base> / already merged / no-op (single-stream)>
  Changelog:    <N entries — N feat, N fix, N breaking / skipped>
  Version:      <vOLD → vNEW (patch|minor|major) / tag-only, no version file>
  Tag:          <v<version> pushed / not pushed / already existed (resumed)>
  Platform:     <published on github|gitlab / tag only — no platform detected>

  Targets:      <cut only — no release guide>
  | Target  | Build | Channel reached          | State                                | Verification          |
  |---------|-------|--------------------------|--------------------------------------|-----------------------|
  | api     | —     | production               | shipped                              | health 200, err 0.1%  |
  | ios     | 42    | store review             | awaiting-external @ review           | beta: crash-free 99.8%|
  | android | 1042  | —                        | blocked — CLI not authenticated      | —                     |

  Worktree:     <merged and removed / removed — branch already merged / kept — release deferred / kept — targets pending / kept — release failed at <step> / none>
  Retired:      <plan + groom session + release state + scratch / preserved — release not complete>

Next:
  /release <ticket>  — resume pending targets (awaiting-external / blocked once fixed)
  /devxp             — fix blocked targets' [CONFIRM] markers or wrong guide steps
  /monitor           — review the deployed system against each target's verification signals
  /improve           — health check now the new code is live
```

On a deferred, pending or failed release, the report says which step or target stopped it and what state was preserved — never a bare failure.

---

## Guidelines

- **The gates are the whole point.** Release is irreversible and affects shared systems. The cut gate authorizes the cut; each target is its own decision; each production-facing promote is its own decision. Never infer consent from an earlier "yes" — not from `/deliver`, not from the cut gate, not from a previous target.
- **The guide is the source of ship commands.** Never improvise, guess or repair a ship step. `[CONFIRM]` means don't run it. Wrong guide → `/devxp`, not a workaround.
- **Show the rollback before the risk.** Every target gate displays the rollback plan; no target ships without one being known.
- **Never roll back automatically.** Offer the guide's rollback; run it only on an explicit yes.
- **Waiting is a state, not a failure.** Store review and staged rollouts are `awaiting-external` — recorded, resumable, and they keep the delivery's artifacts alive.
- **Failure preserves, completion retires.** Every failure or pending path keeps the worktree, the plan, the release state and the scratch. Only a complete release cleans up.
- **Never auto-resolve a merge conflict.** Always surface it.
- **Derive the version, don't invent it.** The bump follows from commit types; if they don't support a bump, say so and ask.
- **A declined gate is a valid outcome, not an error.** Report it as deferred and say exactly how to finish it later.
- **Never re-push a tag that may already exist.** Check `git ls-remote --tags origin` before retrying anything after a partial push.
- **No credentials, ever.** Probe prerequisites read-only; a missing one blocks that target, it is never requested or stored.
