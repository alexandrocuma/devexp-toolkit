# Release Targets

How the toolkit ships work that isn't just a tag — deploys, app-store and beta channels, package publishes — and how the release guide connects every lifecycle command. `/devxp`, `/refine` (via `grooming-agent`), `/deliver`, `/release`, `/monitor`, `/cleanup` and `/improve` point here rather than re-describing the model.

## Why

A tag and a GitHub release is a complete release for a library. It is not a release for anything users run: a service needs deploying, a web app needs publishing to its hosting, an iOS app needs a build number, a beta upload, store review and a phased release, and an Android app needs a version code, an internal track and a staged rollout. Each kind ships differently, can be rolled back differently (a redeploy vs. halting a rollout vs. only a feature flag or a hotfix) and is verified by different signals.

Encoding those differences in the skills would make them vendor-specific and wrong for most repos. Instead each repo declares how it ships in one file, and the skills read it.

## The release guide — `docs/guides/release.md`

A per-repo, version-controlled contract, written from the **Release Guide** template in the `gen-docs` / `update-docs` agents. It lists every **release target** — a separately shipped artifact — and for each: kind, path, versioning rule, build, distribute, promote stages and gates, rollback, post-release verification signals, and prerequisites (by name, never values).

Two rules keep it safe to execute:

- **Evidence or `[CONFIRM]`.** Every command cites where it came from (CI workflow, script, lane config). Anything detected but unproven is written `[CONFIRM] …` — never a plausible guess.
- **`/release` never improvises.** It runs only what the guide says, and refuses a step still marked `[CONFIRM]`. A wrong guide is fixed with `/devxp`, not worked around at release time.

## Who reads it

```
/devxp     detect targets (generic file shapes) → write / refresh the guide
/refine    grooming-agent → "Affected Release Targets" + release impact in the plan
/deliver   Phase 4.5 release readiness per affected target
/release   cut once (tag + release object from the guide's Cut) → ship each affected target from the guide
/monitor   verify each target's declared post-release signals
/cleanup   a ticket with a pending target is live — never retired
/improve   same rule in the hygiene sweep
```

Detection is by kind of file, not brand: container/orchestration/IaC definitions and deploy jobs in CI (service/web); Xcode projects and an `ios/` dir (iOS); Android application modules (Android); cross-platform mobile manifests (both); desktop packaging config; package manifests with publish metadata and no deploy signals (library/CLI).

## Cut vs. ship

| Stage | Scope | Gate | Undo |
|-------|-------|------|------|
| **Cut** — merge, changelog, version + build numbers, tag, platform release | once per release | one confirmation | local until the tag push |
| **Ship** — build → distribute → promote → verify | once per affected target | one per target, plus one per production-facing promote stage | the target's rollback from the guide, offered never automatic |

A target whose CI pipeline is triggered by the tag is *watched*, not re-run.

## Target states

| State | Meaning | Artifacts |
|-------|---------|-----------|
| `shipped` | reached production and verified | eligible for retirement |
| `skipped` | user declined this target at its gate | eligible for retirement |
| `awaiting-external` | waiting on something this session can't complete — store review, a staged rollout step, a change board | **kept**; `/release <ticket>` resumes at the next stage |
| `blocked` | a `[CONFIRM]` step or a missing prerequisite (unauthenticated CLI, absent secret) | **kept**; fix with `/devxp`, then resume |
| `failed at <step>` | a step or verification signal failed; rollback offered | **kept** for inspection |

State persists per ticket at `~/.claude/agent-memory/release/<ticket>.md`. A delivery's artifacts — worktree, plan, groom session, release state, scratch — are retired only when every target is `shipped` or `skipped`. Until then the ticket is **live** to `/cleanup` and `/improve`, even if its branch is merged and its ticket closed.

## No guide

Everything degrades to today's behaviour: grooming marks targets `unverified`, `/deliver` reports readiness as unverified, and `/release` offers `/devxp` and otherwise cuts only (tag + platform release).
