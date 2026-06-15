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

> **Status.** The full review pipeline is live: scope handling (Phase 0), surface detection (Phase 1), per-surface review (Phase 2), and scoring (Phase 3) all produce real output. Remaining epic #54 work is non-blocking: persisting the report to `.devexp/system-health-review.md` and reconciling with `/improve`'s Observability dimension (#59), and documenting `/monitor` as a first-class orchestrator (#60).

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

### Phase 1 — Detect Surfaces

Determine which surface categories are actually present. A category is **detected** if it has at least one signal from either class below. Detection is the gate for everything downstream: a category with **zero** signals is reported `N/A` (Phase 4) and never reviewed or scored — it is genuinely absent, not unhealthy.

Two signal classes per category. Gather both; either one is sufficient to mark the category present.

**Signal class A — repo signals (configuration-as-code).** Search the working tree for the generic shapes below. These patterns describe *kinds* of files and content, not products — resolve any concrete vendor identity from what you actually find, never assume one.

| Category | Repo signals (generic shapes — match the kind, not a brand) |
|----------|-------------------------------------------------------------|
| `cloud` / `infra` | infrastructure-as-code declarations (HCL/Terraform-style `*.tf`/`*.hcl`, `*.bicep`, CloudFormation/ARM-style resource templates in `*.yaml`/`*.json`), container/orchestration manifests (`Dockerfile`, `docker-compose*`, Kubernetes manifests, Helm charts) |
| `dashboards` | dashboard-as-code definitions — JSON/YAML files whose schema describes panels/rows/visualizations, anything under a `dashboards/` directory |
| `logging` | structured-logging SDK imports in source, log-shipper/collector config files, saved log-query definitions |
| `alerting` | alert/alarm rule files — YAML/JSON whose schema describes rules/conditions/thresholds — and notification/routing policy configs |
| `tracing` / `metrics` | distributed-tracing or metrics instrumentation SDK imports in source, metric/exporter/collector configuration, metric definition files |

Run a broad search per in-scope category (adapt globs to the repo's languages), e.g.:

```bash
# Example shape — cloud/infra repo signals (extend per category, exclude vendored dirs)
find . -maxdepth 4 \( -name "*.tf" -o -name "*.hcl" -o -name "*.bicep" -o -name "Dockerfile" -o -name "docker-compose*" \) \
  2>/dev/null | grep -vE "node_modules|\.git|vendor" | head
grep -rlnE "kind:\s*(Deployment|Service|Ingress)" . 2>/dev/null | grep -vE "node_modules|\.git" | head   # k8s manifests
```

**Signal class B — connectors (live, already authenticated).** Probe the environment for tooling the user has *already* authenticated — never trigger an auth flow, never request credentials. Resolve the provider identity from whatever responds.

- **CLIs on `PATH`:** enumerate available command-line tools that expose an account/login *status* subcommand, and probe each read-only (e.g. a `… account show` / `… auth status` / `whoami`-style call). A tool that reports an authenticated session is a live connector for its category. Do not hardcode a list of tool names here — discover what is installed and infer the category from the tool's own output.
- **Connected MCP servers:** inspect the available MCP tool namespaces for ones that map to a cloud or observability provider (a namespace exposing resource/dashboard/log/metric operations). A connected server is a live connector for its category.

A live connector marks its category present **even if no repo signal exists** (and vice-versa) — the two classes are complementary: config-as-code without a connector is reviewable statically; a connector without config-as-code means the surface is managed outside this repo.

**Honor `scope` from Phase 0.** If `scope != "all"`, restrict detection to the named category (or to the connector matching the hint). If the hint resolves to no detected surface, say so and list what *was* detected — do not silently widen to all.

**Emit the surface inventory** before any review, so the reader sees what's in scope before findings:

```
Detected surfaces (scope: <all / "<surface>">):
  Cloud / Infra       : present  — <repo: N IaC files | connector: <resolved provider> authed | both>
  Dashboards          : present  — <repo: N dashboard defs | connector: …>
  Logging             : N/A      — no signals
  Alerting            : present  — <…>
  Tracing / Metrics   : N/A      — no signals
```

With the inventory emitted, proceed to review each in-scope detected surface (Phase 2).

### Phase 2 — Review Each Surface

For each **in-scope detected** surface from Phase 1, produce findings. Each surface is reviewed in one of two modes, chosen automatically by what Phase 1 found — never by asking the user.

**Mode selection (per surface):**

- **Live mode** — a connector (authenticated CLI or connected MCP, per Phase 1 class B) exists for this surface. Query it **read-only** for current state. Never trigger an auth flow, never request, prompt for, or store a credential; if a probe would require new auth, treat the connector as absent and fall back. What to read, by category:

  | Category | Read-only live signal to gather |
  |----------|----------------------------------|
  | `cloud` / `infra` | resource/service health and status, recent deploy/provision state, obvious misconfigurations surfaced by the tool |
  | `dashboards` | the list of defined dashboards and whether key panels resolve to data (vs. empty/broken) |
  | `logging` | recent log-pipeline health, error-rate spikes in the recent window, dropped/ingest-failure signal |
  | `alerting` | current alert/alarm state (firing / silenced / ok), rules with no recipients |
  | `tracing` / `metrics` | whether traces/metrics are arriving, exporters healthy, obvious gaps in expected series |

- **Config-as-code mode (fallback)** — no connector for this surface. Review the repo signals Phase 1 found (the configuration-as-code itself): are the definitions present, internally consistent, and pointing somewhere real? This is a valid review, not a degraded one — **label it explicitly** as config-derived so the reader knows it reflects intent, not live state. Surface-managed-outside-repo (connector present, no config-as-code) is the mirror case: note that the surface exists but isn't version-controlled here.

**Label every finding's provenance.** Tag each finding `[live]` or `[config]` so live state and declared intent are never conflated. A surface may produce both (e.g. a dashboard defined in-repo `[config]` that a connector shows is rendering empty `[live]`).

**Coverage cross-reference (observability vs. the system's critical paths).** Independently of the surfaces, walk the codebase's critical paths and check whether each is actually observable. The critical-path categories and the natural measurement at each (paraphrased inline so this skill stays self-contained — do not reference other skills):

| Critical path | Natural thing to measure there |
|---------------|--------------------------------|
| API / request handlers | latency, error rate, throughput |
| Background jobs / workers | success/failure rate, processing lag, queue depth |
| External calls (APIs, third parties) | error rate, timeout/retry rate, latency |
| Data writes (DB, persistent stores) | write error rate, contention/lock waits |

Find these paths in the code (handler signatures, job/worker registrations, outbound clients, write calls), then for each one mark coverage with evidence:

- **covered** — a dashboard/alert/metric/log line demonstrably observes this path (cite the file/line or the live dashboard/alert)
- **partial** — some signal exists but a key measurement is missing (e.g. logged but no error-rate alert)
- **blind** — no observability attaches to this path (the actionable gaps)

Output the per-surface findings and the coverage table, then proceed to score them (Phase 3).

### Phase 3 — Score & Flag

Turn the Phase 2 findings into a per-surface status, one composite score, and a ranked list of what's actually wrong.

**This assessment is change-independent.** It scores the deployed/configured system *as it stands right now* — it never diffs commits, never references recent changes, and is equally valid on a repo with no activity in months. Do not let git history influence the score in either direction.

**Per-surface status** — from that surface's Phase 2 findings:

| Status | Meaning |
|--------|---------|
| 🟢 | Live signal nominal (or, in config mode, definitions complete and internally consistent); no critical-path gap attributable to this surface |
| 🟡 | Partial — some signal exists but a key piece is missing or unverifiable (e.g. dashboards defined but rendering empty, alerts without recipients, config present but no connector to confirm it's live) |
| 🔴 | Failing — live signal shows a real problem (firing critical alerts, broken pipeline, unhealthy resources) **or** a critical path is blind with no compensating signal |
| N/A | Not detected in Phase 1 — excluded from the composite, never scored as a failure |

**Composite score** — map each non-N/A surface to points (🟢 = 100, 🟡 = 50, 🔴 = 0), take the **equal-weighted mean** across non-N/A surfaces, and band the result:

```
overall = mean(points for each non-N/A surface)
  ≥ 80  → 🟢 healthy
  40–79 → 🟡 needs attention
  < 40  → 🔴 unhealthy
```

Equal weighting is the documented default — every detected surface counts the same — and is intentionally simple so the number is explainable. It is overrideable later (a future per-surface weight map) without changing this contract. State the formula and the non-N/A surface count alongside the number so the score is never a black box. If *every* surface is N/A, there is nothing to score: report "no surfaces detected" rather than a misleading 0 or 100.

**Anomalies / blind spots** — the score says *how much*; this list says *what to do*. Compile every 🟡/🔴 surface finding and every `partial`/`blind` critical path from the Phase 2 coverage table into concrete, actionable items — never just colors. Each item carries:

- **what's off** — one line, specific ("checkout DB writes have no error-rate signal", not "logging could be better")
- **where** — the surface and/or critical path, with **evidence**: a `file:line`, a live alert/dashboard name, or the explicit absence
- **provenance** — `[live]` or `[config]`, carried through from Phase 2
- **suggested action** — the smallest next step that would clear it

Rank the list 🔴 before 🟡, and within each, live-confirmed failures before config-only gaps. This ranked list is the heart of the report — a 🟡 score with three clear actions is more useful than a bare color.

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
- An evidence-backed, ranked list of anomalies / blind spots with suggested actions
