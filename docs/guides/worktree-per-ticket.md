# Worktree-per-Ticket Convention

The single reference for how the toolkit isolates delivery work using git worktrees. Orchestrators (`/deliver`, `/improve`) point here rather than re-describing the mechanism.

## Why

When a ticket enters delivery, its implementation, tests, and review should not mutate the main working tree. Isolating each ticket in its own worktree keeps the primary checkout clean, lets unrelated work continue uninterrupted, and makes parallelism a free byproduct: two tickets in two worktrees never contend for the same files. The cost is one cheap `git worktree add` at the start and one `git worktree remove` at the end.

## Trigger

A worktree is created **when a ticket enters delivery** — at the start of a `/deliver` run, before any implementation step. No worktree is created for grooming, refinement, or read-only inspection; those operate against the current checkout.

## One worktree per deliverable ticket

The unit of isolation is **one deliverable ticket = one worktree**. A ticket is "deliverable" when it has a verified execution plan and can be implemented, tested, and released on its own.

For an **epic**, the epic itself does not get a worktree — each of its sub-tickets does. Delivering an epic means delivering its sub-tickets in dependency order, each in its own worktree. Because the worktrees are independent, sub-tickets with no dependency between them can be worked in parallel without extra setup — that parallelism falls out of the convention rather than being requested explicitly.

## Naming scheme

Both the worktree directory and its branch are derived from the ticket identifier so the mapping is obvious and collisions are impossible:

- **Branch:** `<type>/<ticket-id>` — the same branch convention delivery already uses (e.g. a feature branch for a feature ticket, a fix branch for a bug ticket).
- **Worktree directory:** a sibling of the main checkout, named for the ticket — e.g. `../<repo>-worktrees/<ticket-id>`. Placing worktrees **outside** the primary working tree keeps them from being scanned, indexed, or accidentally committed into the main checkout.

Deriving both names from the ticket id means a glance at `git worktree list` shows exactly which ticket each tree belongs to.

## Lifecycle

```
create  →  work  →  merge (at release gate)  →  remove
```

1. **Create** — at `/deliver` start, add a worktree on a fresh branch derived from the ticket id.
2. **Work** — implementation, instrumentation, test-gap filling, and code review all run *inside* the worktree. The main checkout is never touched during this phase.
3. **Merge** — deferred to the **release gate**. Only when the ticket is approved and ready to release is the branch merged back. Merges are never performed mid-delivery.
4. **Remove** — on a **successful** release, the worktree and its branch are removed automatically; the toolkit cleans up after itself so finished trees don't accumulate.

## Failure handling

If delivery fails at any step — failing tests, an unresolved review finding, an aborted release — the worktree is **kept, not removed**. The isolated tree preserves the exact state where work stopped so it can be inspected, resumed, or diagnosed. Cleanup of a failed worktree is a deliberate, separate action, never automatic.

## Merge discipline

- **Merges are serialized.** When multiple worktrees are ready to merge, they merge one at a time, never concurrently, so each merge sees a consistent base.
- **Conflicts surface to the user.** A merge conflict is always raised for the user to resolve. The toolkit **never auto-resolves** conflicts — silently picking a side risks discarding correct work.

## Single-stream / no-isolation note

Worktree isolation is the default for delivery, but it is not mandatory in every environment. When isolation is unavailable or unwanted — a single-stream setup, a shallow or non-worktree-capable checkout, or a one-off change the user explicitly wants applied to the current tree — delivery may proceed **in place** on the current branch. In that mode the lifecycle collapses to work → release on the existing checkout, the merge step is a no-op, and the same merge discipline (serialized, conflicts surfaced) still applies to whatever integration happens.
