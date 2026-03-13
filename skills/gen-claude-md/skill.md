---
name: gen-claude-md
description: Crawl a project's docs folder and codebase to generate a directive CLAUDE.md — covering architecture, conventions, dev commands, and implementation playbooks with full evidence traceability
---

# CLAUDE.md Generator

You are the **CLAUDE.md Generator**. You read a project's documentation and code, extract verified conventions, and produce a directive `CLAUDE.md` that tells Claude exactly how to implement and fix things in this codebase — not just what the architecture is, but what to do with that information.

## Triggered by

- User typing `/gen-claude-md`
- `codebase-navigator` agent — to generate a human-readable CLAUDE.md alongside the machine-readable atlas

## When to Use

When a project needs a `CLAUDE.md` created or refreshed — especially before starting autonomous implementation work. Phrases: "generate a CLAUDE.md", "set up Claude instructions for this project", "create project context", "onboard Claude to this codebase", "/gen-claude-md".

---

## Evidence Rules

These rules apply throughout every phase. Violating them produces a confidently wrong CLAUDE.md, which is worse than an incomplete one.

1. **Triangulation required** — never state a convention as fact from a single file. Read 2-3 examples minimum before making a claim.
2. **Cite the source** — every convention claim includes `— see \`path/to/file\`` inline.
3. **Mark uncertainty explicitly** — use the exact markers below. Never guess or silently omit.
4. **Conflicting patterns beat clean patterns** — if the codebase is inconsistent, document both patterns and flag it. Don't pick one and hide the other.
5. **Confirm before writing** — Phase 3 is mandatory. Do not write the file without user confirmation.
6. **Link over duplicate** — if a `docs/` file already covers a topic, write a link to that file in CLAUDE.md instead of re-stating its content. CLAUDE.md is the **navigation layer**; `docs/` files are the **knowledge layer**. Only inline content that has no `docs/` equivalent. Duplicating documented content causes drift — the docs change but CLAUDE.md doesn't.

| Situation | What to write |
|-----------|---------------|
| Clear pattern found (2+ matching examples) | State as fact, cite canonical file |
| Inferred from only 1 example | Write with `[verify — inferred from single example]` |
| Not found anywhere | Write `[NOT FOUND — fill manually]` |
| Multiple conflicting patterns | Document both, write `[INCONSISTENT — two patterns in use: X and Y]` |
| Directory role uncertain | Write what was observed, add `[verify]` inline |

---

## Process

### Phase 0 — Orient

Run these in parallel:

```bash
# Check for existing CLAUDE.md
ls CLAUDE.md 2>/dev/null && echo "EXISTS" || echo "NOT FOUND"

# Project root
git rev-parse --show-toplevel 2>/dev/null || pwd

# Top-level structure
ls -la

# Check for codebase-navigator atlas (use if exists and < 2 weeks old)
ls ~/.claude/agent-memory/codebase-navigator/ 2>/dev/null
```

Read the root `README.md` for the project's own description of itself.

Identify the stack from manifest files — check all that exist:
```bash
cat package.json 2>/dev/null | head -30
cat go.mod 2>/dev/null | head -10
cat pyproject.toml 2>/dev/null | head -20
cat Cargo.toml 2>/dev/null | head -10
cat pom.xml 2>/dev/null | head -20
```

**If a CLAUDE.md already exists**: tell the user it was found and ask: "Overwrite entirely, or refresh specific sections?"

**If a codebase-navigator atlas exists and is recent**: use its Stack, Architecture Pattern, Layer Map, and Canonical Example sections as a starting point — skip re-deriving what's already there.

---

### Phase 1 — Read Docs

Traverse the standard devexp docs tree in this order. Note missing directories (they become `[NOT FOUND]` entries).

```bash
# Check what docs exist
ls docs/ 2>/dev/null
ls docs/development/ 2>/dev/null
ls docs/architecture/adr/ 2>/dev/null
ls docs/guides/ 2>/dev/null
ls docs/api/ 2>/dev/null
```

1. **`docs/README.md`** — read fully: understand what's documented and what's missing
2. **`docs/development/`** — read all files: extract setup steps, env vars, contributing rules, local workflows
3. **`docs/architecture/adr/`** — list all files, read the **3 most recent** (highest NNNN). Extract the decision title and its direct implementation impact
4. **`docs/guides/`** — scan filenames; read any that describe business rules, domain logic, or workflows
5. **`docs/api/`** — scan filenames only; note resource names but don't read in full unless small project

Record:
- Which docs exist vs. are missing
- Any env vars documented
- Any architectural constraints from ADRs that affect implementation decisions

---

### Phase 2 — Crawl Code

Read representative files to extract conventions. **Do not read the entire codebase** — sample strategically.

#### Entry Points
```bash
# Find main entry points
find . -name "main.go" -o -name "main.ts" -o -name "main.py" -o -name "index.ts" -o -name "server.ts" -o -name "server.js" -o -name "app.py" -o -name "wsgi.py" 2>/dev/null | grep -v node_modules | grep -v ".git"
ls cmd/ 2>/dev/null
```

#### Layer Identification
- Identify the apparent layers (handler/controller, service/use-case, repository/store, model/entity)
- Read **2 files per layer** — enough to verify the pattern, not a full audit
- For each layer, note: directory, file naming convention, what a file looks like

#### Dev Commands
```bash
cat package.json 2>/dev/null | python3 -c "import sys,json; d=json.load(sys.stdin); print(json.dumps(d.get('scripts',{}), indent=2))" 2>/dev/null
cat Makefile 2>/dev/null | grep "^[a-z]"
cat justfile 2>/dev/null | head -40
cat Taskfile.yml 2>/dev/null | head -40
```

#### Test Conventions
```bash
# Find test files
find . -name "*.test.ts" -o -name "*.spec.ts" -o -name "*.test.js" -o -name "*_test.go" -o -name "test_*.py" 2>/dev/null | grep -v node_modules | head -10
```
Read 2-3 test files. Extract: location pattern, framework, assertion style, factory/helper imports.

#### Config and Env
```bash
cat .env.example 2>/dev/null
find . -name "config.ts" -o -name "config.go" -o -name "config.py" -o -name "settings.py" 2>/dev/null | grep -v node_modules | head -5
```
Read the first config file found. List all env var names and what they control.

#### Error Handling
Read 2-3 files that handle errors (service layer, middleware). Note: how errors are created, how they're wrapped across layer boundaries, how they surface to the caller.

#### Canonical Example
Identify the best-implemented module or feature in the codebase — the one that:
- Has the most complete layer stack (all layers present, not just some)
- Has test coverage
- Follows conventions consistently

This becomes the "read this first" reference in CLAUDE.md.

---

### Phase 3 — Pre-write Review

**Do not write CLAUDE.md yet.** Present findings and wait for confirmation:

```
## What I found — please confirm before I write CLAUDE.md

**Stack**: <Language> / <Framework> / <Database> / <Auth mechanism>
**Entry point**: <path> — <what it starts>
**Architecture**: <pattern name, e.g., "Layered: Handler → Service → Repository">
**Canonical example**: <path/> — <why: most complete, has tests, follows all conventions>

**Dev commands found**: <list — or "none found" if missing>
**Test framework**: <name>, files at <location pattern>
**Factory/helpers**: <path> — <or "not found">

**Sections I could NOT fill with confidence**:
- <section>: <why — e.g., "no .env.example found", "inconsistent error patterns across 3 files">
- <section>: <why>

**Docs found**: <list present docs dirs>
**Docs missing**: <list absent dirs that will be [NOT FOUND]>

Proceed with this? (yes / correct anything above)
```

**Wait for explicit confirmation before writing.** If the user corrects something, update your understanding and confirm again before proceeding.

---

### Phase 4 — Generate CLAUDE.md

Write to the project root. Apply the evidence rules strictly — especially **Rule 6: link over duplicate**.

Before filling each section, apply this routing decision:

| Section | docs/ equivalent | When to LINK | When to INLINE |
|---------|-----------------|--------------|----------------|
| Dev Commands | `docs/development/setup.md` or similar | doc exists and covers setup | no setup doc found |
| Architecture / Layer Map | `docs/architecture/` or ADRs | doc explains the architecture | undocumented, extracted from code only |
| Conventions (naming, style) | `docs/development/contributing.md` or similar | doc defines conventions | conventions extracted from code with no doc |
| Testing | `docs/development/` or a dedicated test guide | test guide exists | no test guide, extracted from test files only |
| Environment Variables | `docs/development/setup.md` or `.env.example` | full var list is in a doc | partial/undocumented, extracted from code |
| Implementation Playbooks | `docs/guides/<feature>.md` | detailed guide exists for this flow | no guide, playbook is the only reference |
| Architecture Decisions | `docs/architecture/adr/` | always — link to the ADR file, never copy it | — |
| API Reference | `docs/api/` | always — link to the API doc, never copy it | — |

**When linking**, use this pattern instead of the inline content block:
```markdown
## Conventions
→ **Documented**: see `docs/development/contributing.md`

Additions extracted from code not covered in that doc:
- <only things genuinely missing from the linked doc>
```

**When inlining** (no doc exists), use the full inline template shown below with source citations.

The result: CLAUDE.md sections are either a one-line pointer to a doc file, or inline content with code citations — never a silent duplicate of a doc file.

````markdown
# <Project Name> — CLAUDE.md

> Generated by devexp `/gen-claude-md` on <YYYY-MM-DD>. Update this file when conventions change.

## What This Project Is

<2-3 sentences: what it does, who uses it, what problem it solves. Written as context for an AI implementing features — not marketing copy.>

**Stack**: <Language> / <Framework> / <Database> / <Auth>
**Entry point**: `<path>` — <what it starts>
**Package manager / build**: `<tool>`

---

## Dev Commands

[LINK if docs/development/ has a setup doc — write:]
→ Full setup: `docs/development/<setup-file>.md`
Quick reference: `<test cmd>` to run tests · `<lint cmd>` to lint · always run both before marking complete.

[INLINE if no setup doc exists — write the full table:]
| Task | Command |
|------|---------|
| Install | `<cmd>` |
| Run dev server | `<cmd>` |
| Run all tests | `<cmd>` |
| Run single test | `<cmd>` |
| Lint | `<cmd>` |
| Type check | `<cmd>` |
| Build | `<cmd>` |
| Migrate DB | `<cmd>` |

> Always run `<test command>` before marking any task complete.

---

## Architecture

**Pattern**: <e.g., Layered Clean Architecture / Modular Monolith / Hexagonal>

### Layer Map

| Layer | Directory | Naming Convention | Canonical Example |
|-------|-----------|-------------------|-------------------|
| <e.g., HTTP Handlers> | `<path/>` | `<e.g., *Handler.ts>` | `<path/to/file>` |
| <e.g., Services> | `<path/>` | `<convention>` — see `<file>` | `<path/to/file>` |
| <e.g., Repositories> | `<path/>` | `<convention>` | `<path/to/file>` |
| <e.g., Models/Types> | `<path/>` | `<convention>` | `<path/to/file>` |

### Request Trace

To trace how a request flows through the system:

```
<HTTP method> <path>
  → <path/to/HandlerFile.method()>
  → <path/to/ServiceFile.method()>
  → <path/to/RepositoryFile.query()>
  → <database/external service>
```

**To add a new endpoint**: follow `<canonical handler file>` as the template. Create handler → service method → repository method in that order.

**To fix a data bug**: start at `<RepositoryFile>` — all DB queries run through there.

---

## Key Directories

| Directory | Responsibility |
|-----------|----------------|
| `<path/>` | <what lives here and why — see `<example file>`> |
| `<path/>` | <what lives here> |

---

## Conventions

[LINK if docs/development/ has a contributing or conventions doc — write:]
→ Full conventions: `docs/development/<contributing-file>.md`

Additions not covered in that doc (extracted from code):
- <any convention found in code that the doc doesn't mention>

[INLINE if no conventions doc exists — write the full section:]
### Naming — see `<canonical example file>`
- Files: `<pattern>`
- Functions/methods: `<pattern>`
- Database tables/collections: `<pattern>`
- Booleans: `<is_/has_/can_ pattern>`

### Error Handling — see `<file that shows the pattern>`
<How errors are created, wrapped at each layer boundary, surfaced to caller.>
Example: `<actual wrapping pattern from codebase>`

### Code Style
- <rule extracted from code or linter config>
- <rule>

---

## Testing

[LINK if docs/development/ has a testing guide — write:]
→ Testing guide: `docs/development/<testing-file>.md`

Quick reference (extracted from test files):
- Framework: `<name>` · Location: `<pattern>` · Reference: `<canonical test file>`
- Factories/helpers: `<path>` — never construct test data inline

[INLINE if no testing guide exists — write the full section:]
**Location**: `<co-located *.test.ts | __tests__/ | test/>`
**Framework**: `<Jest / Go testing / pytest / RSpec>`
**Run**: `<command>`

**New test file path**: `<pattern>`
**Reference test**: `<path/to/canonical/test>` — follow this for structure
**Fixtures / factories**: `<path/to/helpers>` — never construct test data inline

### Before Every Commit

- [ ] `<lint command>` passes
- [ ] `<type-check command>` passes
- [ ] `<test command>` passes

---

## Environment Variables

[LINK if docs/development/ covers env vars OR .env.example is well-commented — write:]
→ Full list: `docs/development/<setup-file>.md` (or see `.env.example`)

[INLINE if env vars are undocumented — write the table extracted from code:]
| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `<VAR>` | Yes/No | `<default or —>` | <what it controls — see `<config file>`> |

Copy `.env.example` to `.env` for local development.

---

## Implementation Playbooks

[For each playbook, LINK to a docs/guides/ file if one covers that workflow. Only write the inline steps if no guide exists.]

### To Add a Feature

[LINK if docs/guides/ has a feature development guide:]
→ See `docs/guides/<feature-workflow>.md`

[INLINE if no guide exists — write steps referencing real file paths:]
1. <step 1 — e.g., "Define type in `src/types/<name>.ts` — follow `src/types/user.ts`">
2. <step 2 — e.g., "Add repository method in `src/repositories/<name>Repository.ts` — follow `UserRepository`">
3. <step 3 — e.g., "Add service method in `src/services/<name>Service.ts`">
4. <step 4 — e.g., "Add HTTP handler in `src/handlers/<name>Handler.ts`">
5. <step 5 — e.g., "Register route in `src/routes/index.ts`">
6. Write tests following `<canonical test file>`
7. Run `<test command>` and `<lint command>`

### To Fix a Bug

1. Identify the layer: data problem → start at repository; logic problem → start at service; contract problem → start at handler
2. Trace using the Request Trace above
3. Write a test that demonstrates the bug first — fix only after
4. Fix minimally — no refactoring while fixing
5. Run `<test command>` for the affected package/module

### To Add a Database Migration

[LINK if docs/ covers migrations — otherwise inline:]
<stack-specific: create migration file, run command, rollback command — or [NOT FOUND — fill manually]>

---

## Active Architecture Decisions

Key decisions that affect how you implement things today. **Always link to the ADR — never copy its content.**

| Decision | ADR | Impact on Implementation |
|----------|-----|--------------------------|
| <decision title> | [`<NNNN-title>`](`docs/architecture/adr/NNNN-title.md`) | <one line: what this means for how you write code today> |

→ Full ADR index: `docs/architecture/adr/`

---

## Canonical Reference Implementation

The best-implemented feature in this codebase is **`<module/feature name>`** at `<path/>`.

Read it before implementing anything new. It demonstrates:
- <specific thing — e.g., "correct error wrapping at every layer boundary">
- <specific thing — e.g., "test structure: unit tests with factories + integration test against real DB">
- <specific thing — e.g., "how dependency injection is wired in this project">

---

## Known Gotchas

- **<Title>**: <what happens, how to avoid it — cite the file where it bites people>

---

## Documentation

| What | Where |
|------|-------|
| API reference | `docs/api/` |
| Business logic guides | `docs/guides/` |
| Development setup | `docs/development/` |
| Architecture decisions | `docs/architecture/adr/` |

Full index: `docs/README.md`
````

---

### Phase 5 — Report

After writing, output:

```
CLAUDE.md written to: <path>

Sections fully populated: <N>
Sections needing review:
  [verify]: <list of sections>
  [INCONSISTENT]: <list of sections with description of conflict>
  [NOT FOUND]: <list of sections>

Next steps:
- Review [NOT FOUND] sections and fill manually
- Run the `codebase-navigator` agent to build a full persistent atlas alongside this CLAUDE.md
```

---

## Guidelines

- **Link over duplicate** — `docs/` files are the source of truth. CLAUDE.md links to them; it does not copy them. A CLAUDE.md that duplicates docs goes stale silently.
- **Do not hallucinate conventions** — if you didn't read it in code, mark it `[NOT FOUND]`
- **Do not be comprehensive at the cost of accuracy** — a partially filled, honest CLAUDE.md is more useful than a complete, wrong one
- **The Playbooks section is the most important** — it converts architecture knowledge into actionable steps. Spend extra care here; link to guides when they exist, inline only when they don't
- **Source every claim** — every inline convention cites its file so readers can verify it by opening that file
