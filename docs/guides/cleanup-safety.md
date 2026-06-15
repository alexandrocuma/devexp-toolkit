# Cleanup Safety

The single reference for how the toolkit deletes artifacts safely. Both cleanup paths point here: `/deliver`'s per-ticket final phase (C1) and `/improve`'s repo-wide hygiene sweep (C2). Any step that removes files, branches, worktrees, or memory entries follows these rules — they exist because a wrong glob or an unset variable can destroy user data.

## The rules

1. **Dry-run first — list before you delete.** Enumerate every candidate and show it to the user before removing anything. A sweep that deletes before reporting is never correct.

2. **Confirm or log every destructive action.** Removal requires explicit user confirmation, or — when running in an already-confirmed/flagged mode — a clear line-by-line log of exactly what was removed. Silent deletion is forbidden.

3. **Scoped patterns only — guard the identifier, and prefix-anchor the glob.** Every `rm`/remove must be anchored to a verified identifier (ticket id, worktree path, plan filename). Two requirements:
   - **Validate the id first** — assert it is **non-empty and matches `[A-Za-z0-9_-]`**, aborting the whole step otherwise (a path-traversal or glob character would escape the intended directory).
   - **Put a literal prefix before any wildcard** — write `…/.deliver-"$id"-*` or `…/"$id"-*`, never `…/*"$id"*`. A leading `*` right after the directory boundary collapses to a blanket wipe the instant `$id` is empty (`/tmp/*"$id"*` → `/tmp/*`), and it reads identically to a deliberate `/tmp/*` to any safety check.

   ```bash
   case "$id" in
     ""|*[!A-Za-z0-9_-]*) echo "id missing or unsafe — skipping deletion"; return 2>/dev/null || exit 0 ;;
   esac
   rm -f /tmp/.deliver-"$id"-* 2>/dev/null   # safe: id validated AND the glob is prefix-anchored
   ```

   The `dangerous-cmd-guard` hook enforces this at execution time — it blocks an unanchored wildcard delete in a sensitive dir (`/tmp/*`, `~/.claude/.../*`) even if a skill's inline guard is wrong, so a prefix-anchored glob is also the only form that will actually run.

4. **Never touch shared or unscoped state.** Project-shared agent memory (`<PROJECT-NAME>.md`), the main checkout, the default branch, and anything not provably an orphan or a this-ticket artifact are off-limits. Scope globs so they cannot match shared files even on an empty match set.

5. **Memory needs a staleness reason.** Never delete or rewrite an agent-memory entry without a drift/staleness justification from the canonical A1 Drift Classification (codebase-navigator's Memory Protocol). Re-date re-verified entries; remove only resolved/contradicted ones.

6. **Preserve on failure or doubt.** If a delivery failed, or it is unclear whether an artifact is still live (a worktree with uncommitted work, a plan for an open ticket), **keep it and report it** rather than removing it. Cleanup is reversible only by not having run it.

## Who follows this

- **`/deliver` Phase 7 (C1)** — retires *this ticket's* artifacts on successful completion, keyed to the verified ticket id.
- **`/improve` hygiene sweep (C2)** — finds and retires *repo-wide* orphans (abandoned worktrees, stale plans, old groom sessions, stray `/tmp` scratch, drift-stale memory), dry-run then confirmed.

Both carry their own concrete guards inline (each skill runs standalone); this doc is the shared rationale they must not diverge from.
