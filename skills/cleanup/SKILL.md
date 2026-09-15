---
name: cleanup
description: On-demand cleanup of finished or abandoned delivery artifacts — merged or superseded git worktrees, orphaned branches, stale persisted plans, groom-session leftovers, and stray /tmp scratch. Use when worktrees accumulate after deliveries (especially deferred "I'll release manually" releases), or for a post-delivery tidy-up without a full /improve run.
---

# Cleanup: On-Demand Artifact Retirement

You are the **cleanup specialist**, retiring delivery artifacts that are provably finished: worktrees whose branch has merged (or whose delivery was abandoned), branches that no longer need to exist, and the plan/session/scratch files orphaned deliveries leave behind. You delete nothing you cannot prove is finished — everything else is kept and reported.

## Triggered by

- `/cleanup` — sweep all known artifact locations for the current repo
- `/cleanup <ticket-id>` — scope the sweep to one ticket's worktree, branch, plan, and scratch

## When to Use

- Finished, merged, or abandoned worktrees are accumulating — `git worktree list` is longer than the set of tickets actually in flight.
- A release was deferred at `/release`'s gate ("I'll release manually") and the branch has since merged — the tree it left behind is ready to retire. To *finish* the release instead, use `/release <ticket>`.
- A delivery failed and was superseded, leaving its worktree and persisted plan behind.
- Post-delivery tidy-up: the user wants the orphans gone now, without a full `/improve` run.

**Out of scope** — do not reach for `/cleanup` when:
- The task is **agent-memory pruning**. That stays in `/improve`'s hygiene sweep (C2): memory entries may only be removed with a drift/staleness justification from codebase-navigator's canonical A1 Drift Classification, and `/cleanup` does not perform that classification.
- The target is **anything in the main checkout** or on the **default branch**. Cleanup never touches them.
- The task is **release or changelog work**. Retiring artifacts is not releasing; a merged branch whose release is still pending is reported, not removed unless its worktree qualifies on its own.

**Relationship to `/improve`:** `/improve`'s hygiene sweep (C2) remains the repo-wide, periodic version of this work — it finds orphans across the whole repo on a schedule and includes drift-stale memory pruning. `/cleanup` is the on-demand counterpart: narrower (no memory pruning), faster, runnable any time. Do not duplicate C2's memory logic here.

## Process

### Phase 1 — Discovery  *(read-only — list, never delete)*

Enumerate every candidate location. Prefix-anchor all artifact globs (see the safety rules — a leading wildcard is never allowed):

```bash
# Worktrees and their branches
git worktree list --porcelain 2>/dev/null
git worktree prune --dry-run 2>/dev/null        # admin entries prune would drop (dirs already gone)

# Default branch (never a cleanup target)
base="$(git symbolic-ref --short refs/remotes/origin/HEAD 2>/dev/null | sed 's@^origin/@@')"
base="${base:-$(git config init.defaultBranch 2>/dev/null)}"
base="${base:-main}"

# Branch merge status vs the base
git branch --merged "$base" 2>/dev/null
git for-each-ref --sort=-committerdate refs/heads --format='%(refname:short) %(committerdate:relative)' 2>/dev/null | head -10

# Open PRs/MRs (an open PR makes its branch live)
gh pr list --state open --json number,title,headRefName 2>/dev/null
glab mr list --state opened 2>/dev/null

# Persisted plans, groom sessions, /tmp scratch (toolkit prefix patterns only)
ls ~/.claude/agent-memory/grooming-agent/plans/ 2>/dev/null
ls ~/.claude/agent-memory/grooming-agent/sessions/ 2>/dev/null
ls -1 /tmp/.deliver-* /tmp/.improve-* /tmp/.groom-* 2>/dev/null
```

The main checkout (the first entry in `git worktree list`) is never a candidate. The default branch is never a candidate.

### Phase 2 — Classification

Classify every discovered candidate before proposing any action:

| Candidate | Classification | Evidence to check |
|-----------|---------------|-------------------|
| Worktree | **live** | Uncommitted/untracked work (`git -C <path> status --porcelain`), an open PR/MR on its branch, an open ticket, or its branch is not merged to the base |
| Worktree | **finished** | Branch is merged to the base, **or** delivery abandoned: no open PR, no uncommitted work, ticket closed/cancelled |
| Branch (not default) | candidate | `git branch --merged <base>` lists it, or the delivery it belonged to is provably abandoned |
| Persisted plan | candidate | Its ticket is closed/delivered; keep plans for open tickets |
| Groom session | candidate | Its ticket is closed; keep sessions for open tickets |
| /tmp scratch | candidate | Matches a toolkit prefix (`.deliver-`, `.improve-`, `.groom-`) and the owning run is finished |

When the evidence is mixed or incomplete, classify as **live** — preserve on doubt.

### Phase 3 — Dry-Run Report  *(always — before anything is deleted)*

Present every candidate with its classification and intended action:

```
Cleanup — dry run

Worktrees:
  <abs path>   <branch>   finished — merged to <base>     → remove tree + git branch -d
  <abs path>   <branch>   live — uncommitted changes      → KEEP
Branches (no worktree):
  <branch>     finished — merged to <base>                → git branch -d
Artifacts:
  plans/<id>.md        ticket closed                     → remove
  sessions/<id>-*      ticket closed                     → remove
  /tmp/.deliver-<id>-* owning run finished              → remove

Nothing will be removed until you confirm.
Remove these N items? (yes / choose / skip)
```

If `/cleanup <ticket-id>` was invoked, scope the report to that ticket; report the rest of the sweep only as a one-line count.

### Phase 4 — Confirmation

Wait for explicit confirmation (yes / choose / skip). When invoked as `/cleanup --cleanup` (pre-confirmed mode), skip the prompt but **log every removal line-by-line** as it happens — silent deletion is forbidden either way.

### Phase 5 — Act

Work one candidate at a time, in this order, validating ids before they reach any delete:

```bash
# Validate an identifier before using it in any delete pattern (see safety rules).
safe_id() { case "$1" in ""|*[!A-Za-z0-9_-]*) return 1 ;; *) return 0 ;; esac; }

# Finished worktrees — git refuses a dirty tree on its own; never add --force to a live tree
git worktree remove "<verified-abs-path>"
git branch -d "<type>/<ticket-id>"          # -d, never -D — refuse if not merged

# Stale admin entries for directories already gone
git worktree prune

# Scoped artifact removal — prefix-anchored globs, validated ids only
id="<finished-ticket-id>"; safe_id "$id" && rm -f ~/.claude/agent-memory/grooming-agent/plans/"$id".md
id="<finished-ticket-id>"; safe_id "$id" && rm -f ~/.claude/agent-memory/grooming-agent/sessions/"$id"-* 2>/dev/null
id="<finished-ticket-id>"; safe_id "$id" && rm -f /tmp/.deliver-"$id"-* /tmp/.improve-"$id"-* /tmp/.groom-"$id"-* 2>/dev/null
```

A `git worktree remove` or `git branch -d` that refuses (dirty tree, unmerged branch) is a **signal, not an obstacle** — reclassify the candidate as live, report it, and move on. Never escalate to `--force` or `-D`.

## Safety Rules  *(load-bearing — do not soften)*

The canonical rationale lives in `docs/guides/cleanup-safety.md` (a maintainer reference in the devexp-toolkit repo, **not** installed alongside this skill — do not look for it in the current repo). These rules are inlined and authoritative:

1. **Dry-run first.** List every candidate with its classification and intended action before removing anything. A sweep that deletes before reporting is never correct.
2. **Confirm or log every destructive action.** Removal requires explicit confirmation, or — in `--cleanup` pre-confirmed mode — a clear line-by-line log of exactly what was removed.
3. **Validate every id, and prefix-anchor every glob.** Assert each identifier is **non-empty and matches `[A-Za-z0-9_-]`**, aborting that step otherwise. Write `…/.deliver-"$id"-*` or `…/"$id"-*`, never `…/*"$id"*` — a leading wildcard collapses to a blanket wipe the instant the id is empty. The `dangerous-cmd-guard` hook enforces this at execution time, so the prefix-anchored form is also the only form that will actually run.
4. **Never touch shared or unscoped state.** The main checkout, the default branch, and shared agent memory (`<PROJECT-NAME>.md`) are off-limits. Scope globs so they cannot match shared files even on an empty match set.
5. **Preserve on failure or doubt.** A worktree you cannot prove is finished (uncommitted work, open PR, ambiguous ticket state) is **kept and reported, never removed**. Cleanup is reversible only by not having run it.
6. **No memory pruning.** Agent-memory entries are out of scope here regardless of age — that classification belongs to `/improve` C2.

## Output

```
## Cleanup complete

  Worktrees removed:   N   [<branch> — merged to <base>]
  Worktrees kept:      N   [<branch> — <reason: uncommitted work / open PR / not merged>]
  Branches removed:    N   [<name> — merged]
  Plans removed:       N   [<id>]
  Sessions removed:    N   [<id>]
  /tmp scratch removed: N  [<id>]
  Admin entries pruned: N

Kept (reported, not removed):
  <candidate — reason it is still live>

Next:
  /improve   — repo-wide hygiene sweep (includes drift-stale memory pruning, which /cleanup does not do)
```
