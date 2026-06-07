# Docs Architecture Pattern

## The Problem with Fat CLAUDE.md Files

CLAUDE.md is loaded into AI context at the start of every conversation. Every line costs token budget. A CLAUDE.md that inlines full component catalogs, convention examples with code blocks, and step-by-step playbooks wastes context on things that belong in docs/ — and creates drift, because the inlined content gets stale while the docs/ equivalent stays current.

The pattern: **CLAUDE.md is an indexer. docs/ is the knowledge store.**

---

## The Pattern

```
CLAUDE.md                    ← directives + gotchas + command table + navigation pointers (≤150 lines)
  → docs/README.md           ← top-level index of all documentation
    → docs/development/README.md   ← index of dev/authoring docs
    → docs/guides/README.md        ← index of how-to guides
    → docs/reference/README.md     ← index of component/API reference
    → docs/architecture/README.md  ← index of ADRs and architecture docs
      → actual .md files           ← real content lives here
```

The AI navigates the index chain to find what it needs mid-task. The pre-tool-use hooks and CLAUDE.md directives ensure it knows where to look.

---

## Standard docs/ Folder Tree

Every repo this framework works in uses this structure:

```
docs/
├── README.md               # Top-level navigation index — always kept up to date
├── api/                    # REST/GraphQL endpoints, SDK methods
│   └── README.md
├── guides/                 # How-to guides, tutorials, business logic, setup walkthroughs
│   └── README.md
├── reference/              # Component catalogs, CLI reference, configuration schemas
│   └── README.md
├── architecture/           # Architecture overviews, ADRs
│   ├── README.md
│   └── adr/
│       └── README.md       # ADR index with status (Accepted/Superseded/Proposed)
├── development/            # Dev environment, authoring guides, conventions, testing
│   └── README.md
└── postmortems/            # Incident postmortems (no index required)
```

**Routing rules:**

| What you're documenting | Where it goes |
|------------------------|---------------|
| REST/GraphQL endpoints, SDK methods | `docs/api/<resource>.md` |
| Business rules, domain logic | `docs/guides/<feature>-logic.md` |
| How-to guides, tutorials | `docs/guides/<topic>.md` |
| Component catalogs, CLI reference | `docs/reference/<topic>.md` |
| Dev environment, setup, authoring | `docs/development/<topic>.md` |
| Architecture decisions | `docs/architecture/adr/NNNN-<title>.md` |
| Incident postmortems | `docs/postmortems/YYYY-MM-DD-<title>.md` |

Note: `docs/api/` vs `docs/reference/` — use `api/` for REST/GraphQL endpoints; use `reference/` for component catalogs, tool reference, or configuration schemas that aren't HTTP endpoints.

---

## The README.md Index System

Every folder that contains documentation files **must have a `README.md` index**. This is the navigation layer that agents rely on to orient without reading every file blindly.

Standard subfolder README format:

```markdown
# <Folder Name>

Brief description of what this folder contains.

## Files

| File | Description | Status |
|------|-------------|--------|
| [<filename>.md](<filename>.md) | One-line description | ready |

## Status Values

- **ready** — current, accurate, safe to rely on
- **draft** — work in progress, may be incomplete
- **reference** — historical or background, not actionable today
- **blocked** — waiting on something
```

The status column lets agents skip irrelevant files without opening them.

---

## What Belongs in CLAUDE.md

| Keep in CLAUDE.md | Move to docs/ |
|---|---|
| Behavioral directives (must-do, must-not-do) | Full component catalogs |
| Must-know gotchas (silent bugs if forgotten) | Convention patterns with code examples |
| Quick command table | Step-by-step playbooks |
| Layer map / structure (file paths only, 1 line each) | Full API reference |
| Navigation pointers to docs/ | Environment variables tables |

**Target size: ≤150 lines.** If CLAUDE.md is growing past this, content is leaking in that belongs in docs/.

---

## Link Over Duplicate

If a `docs/` file already covers a topic, write a link to it in CLAUDE.md — never re-state its content. Duplicating documented content causes drift: the docs change but CLAUDE.md doesn't.

```markdown
# In CLAUDE.md — correct
For conventions (error handling, OTel spans, RBAC), see [`docs/development/conventions.md`](docs/development/conventions.md)

# In CLAUDE.md — wrong
## Error Handling
Handlers never write error JSON. They call `_ = c.Error(err); return`...
[full code example]
```

---

## How to Migrate a Fat CLAUDE.md

1. **Categorize** — read each section and assign it to a routing target (reference/, guides/, development/, etc.)
2. **Create target files** — write the content into the appropriate `docs/` file using the standard template for that type
3. **Update subfolder README** — add the new file to its folder's README.md index
4. **Update top-level index** — add or update the entry in `docs/README.md`
5. **Replace with pointer** — in CLAUDE.md, replace the full section with a one-line pointer: `For X, see [docs/Y](docs/Y)`
6. **Verify** — CLAUDE.md should be ≤150 lines; every docs/ folder should have a README.md

---

## Why This Works

- **Token efficiency** — CLAUDE.md loads in every session; docs/ files load only when needed
- **No drift** — one source of truth per topic; no dual-maintenance
- **Navigable by index** — agents read folder READMEs to find relevant files without scanning everything
- **Human-friendly** — docs/ has full context, examples, and status tracking; CLAUDE.md is a fast orientation layer
