---
name: monitor
description: Operate-phase health review of a deployed system — detects stack surfaces (cloud/infra, dashboards, logging, alerting, tracing) and produces a change-independent, scored assessment that flags what's off. Stack-agnostic; names no vendor.
---

# Monitor: Deployed-System Health Review

You are running the **operate phase** of the development cycle. Where `/devxp`, `/refine`, `/deliver`, and `/improve` build and refine the *codebase*, `/monitor` assesses the *running system* — a change-independent, multi-dimensional read of how the deployed system is actually doing, aggregated into one overall score that surfaces "something is off."

This is a **point-in-time review**, not a runtime daemon: it inspects what exists right now (live signal where a connector is available, configuration-as-code otherwise) and reports. It never polls, never alerts on a loop, and never collects credentials.

## Triggered by

- `/monitor` — review every deployed-system surface detected in this repo/environment
- `/monitor <surface>` — scope the review to a single detected surface (e.g. the cloud surface, the dashboards surface, the logging surface)

Invoked directly by the user. `/monitor` is self-contained — it does not call other skills, and other skills do not call it. (`/improve`'s Observability Maturity dimension defers to this skill for the deep review — see #59 — but that is a documentation pointer, not a runtime dependency.)

## When to Use

When you want to know whether the *deployed* system is healthy — independent of any recent code change. Run it after a deploy, during an incident triage, on a maintenance cadence, or any time the system "feels off" and you want an evidence-backed read rather than a hunch. Because it does not diff commits, it is equally valid on a quiet repo and a busy one.

Do **not** use it to assess code quality, test coverage, or repo hygiene — that is `/improve`. Do not use it to instrument new code — that is `/deliver`'s Instrument phase.

---

## Process

> **Scaffold status.** This is the foundation skill (epic #54, ticket #55). Phase 0 (orientation), the trigger/scope handling, and the report shape are live. The three review phases below are **stubs** — they describe the contract each will fulfill and point to the ticket that implements them. Until those land, `/monitor` reports the detected surface inventory and an explicit "review not yet implemented" notice rather than a fabricated score.

### Phase 0 — Establish Scope

Parse the invocation:

- **Bare `/monitor`** → `scope = "all"`; review every detected surface.
- **`/monitor <surface>`** → `scope = "<surface>"`; the argument is a *surface hint*, resolved against the surface categories below. It is never a vendor name baked into this prompt — it names a category (or a connector discovered at runtime). If the hint matches no detected surface, say so and list what *was* detected, rather than failing silently.

Surface categories (the agnostic taxonomy every later phase works in):

| Category | What it covers |
|----------|----------------|
| `cloud` / `infra` | provisioned infrastructure, compute, managed services |
| `dashboards` | metrics dashboards / visualization defined as code or served live |
| `logging` | log aggregation, structured log pipelines, log queries |
| `alerting` | alert rules, alarms, notification policies |
| `tracing` / `metrics` | distributed tracing, instrumentation SDKs, metric exporters |

A surface is only in play if it is **detected** (Phase 1). Categories with no signal are reported `N/A`, never scored red.

### Phase 1 — Detect Surfaces  *(implemented in #56)*

> **Stub.** Detect which surface categories are present, by repo signals (IaC, dashboards-as-code, alert rules, instrumentation SDK imports) and by the presence of authenticated CLIs/MCPs — resolving vendor identity at runtime, never hardcoding it here. Honor `scope` from Phase 0. Emit a surface inventory before any review. Until #56 lands, report: "surface detection not yet implemented (#56)".

### Phase 2 — Review Each Surface  *(implemented in #57)*

> **Stub.** For each in-scope detected surface: query live signal when a connector exists (resource health, dashboard list, recent alert/log state), else review configuration-as-code — labeling which mode produced each finding. Never request or store credentials. Cross-reference coverage against the system's critical paths (API handlers, background jobs, external calls, DB writes), marking each covered / partial / blind with evidence. Until #57 lands, report: "per-surface review not yet implemented (#57)".

### Phase 3 — Score & Flag  *(implemented in #58)*

> **Stub.** Assign each surface 🟢/🟡/🔴/N/A, compute an overall composite score from the non-N/A surfaces, and list concrete anomalies / blind spots as actionable items with evidence — not just colors. The assessment reflects current deployed/config state and does not diff commits. Until #58 lands, report: "scoring not yet implemented (#58)".

### Phase 4 — Report

```
# System Health Review — <project> — <date>

Scope: <all surfaces / "<surface>">

| Surface              | Status      | Summary |
|----------------------|-------------|---------|
| Cloud / Infra        | 🟢/🟡/🔴/N/A | <one-line> |
| Dashboards           | 🟢/🟡/🔴/N/A | <one-line> |
| Logging              | 🟢/🟡/🔴/N/A | <one-line> |
| Alerting             | 🟢/🟡/🔴/N/A | <one-line> |
| Tracing / Metrics    | 🟢/🟡/🔴/N/A | <one-line> |

Overall: <composite score>   (<N> surfaces reviewed, <N> N/A)

Anomalies / blind spots:
  - <actionable item with evidence>
```

Persisting this report to `.devexp/system-health-review.md` and reconciling with `/improve`'s Observability dimension is handled in #59.

---

## Guidelines

- **Stack-agnostic by construction** — this prompt names no specific vendor, platform, or product. Surfaces are categories; concrete tools are resolved at runtime from repo signals and available connectors. A change that hardcodes a vendor name here is a regression.
- **Change-independent** — the review reflects the deployed system's current state, not a diff. It must be valid to run on a repo with zero recent commits.
- **Read-only and credential-free** — query live signal only through connectors the environment already has authenticated; never prompt for, collect, or store secrets. This is a review, not a deploy.
- **N/A is not failure** — a surface with no signal is genuinely absent or out of scope; report it `N/A`, never a false 🔴.
- **Self-contained** — `/monitor` invokes no other skill and is invoked by none.

## Output

A System Health Review (the Phase 4 table) with:
- A per-surface status for each detected, in-scope surface
- An overall composite score across non-N/A surfaces
- An evidence-backed list of anomalies / blind spots
- An explicit note of which phases are not yet implemented while this skill is in scaffold state
