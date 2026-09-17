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

## The Development Kit

An index is only useful if there is something to point to. Every repo gets a minimum set of docs — the **Development Kit** — holding everything needed to work on the project effectively:

| Kit doc | Answers |
|---------|---------|
| `docs/development/setup.md` | Install, run, build, configure; full command list; env vars |
| `docs/development/conventions.md` | Naming, module structure, error handling, logging, style, commits |
| `docs/development/testing.md` | Test types, locations, reference tests, fixtures, pre-commit checks |
| `docs/architecture/overview.md` | Layers, request flow, key directories, external deps, reference implementation |
| `docs/guides/workflows.md` | Exact steps with real paths: add a feature, fix a bug, change the data model, add config/dependency |
| `docs/guides/release.md` | Release targets — build, distribute, promote, roll back, verify ([release-targets.md](release-targets.md)) |

Templates and rules live in the `gen-docs` / `update-docs` agents. Every claim is cited from code; what can't be proven is a visible marker (`[NOT FOUND]`, `[verify]`, `[INCONSISTENT]`, `[CONFIRM]`, `N/A — reason`) and the doc is `draft` until they're closed.

**Order matters.** `/devxp` builds the kit **before** `CLAUDE.md`. Generating the index first leaves it nothing to link to, which is exactly how knowledge ends up inlined.

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
| What the project is (1–3 sentences + stack line) | Architecture walkthroughs, request traces → `architecture/overview.md` |
| Start Here — newcomer reading order | Convention patterns with code examples → `development/conventions.md` |
| Rules — always/never directives, one line, cited | Step-by-step playbooks → `guides/workflows.md` |
| Gotchas — silent failures, one or two lines, cited | Full command list, env var tables → `development/setup.md` |
| Commands — the ≤6 most used | Test patterns and fixtures → `development/testing.md` |
| Layer map — paths + one-line roles, ≤8 rows (optional) | Full component catalogs, API reference → `reference/`, `api/` |
| Where Things Are — "I need to… → docs/…" | ADR content → `architecture/adr/` |

**Hard limits** (`gen-indexer` and `update-indexer` check them): ≤150 lines · no code blocks · no section over ~15 lines · every `docs/` pointer resolves. The test for every line: is it a rule, a gotcha, a command, or a pointer? If not, it belongs in docs/. All of this applies outside `devexp:preserve` blocks — the one exception, below.

---

## The Exception: Preserve and Inherit Blocks

Some directives are owned **outside the repo** — a rulebook shared by a family of repos, an organisation-wide policy. They can't be verified against this repo's `docs/`, and moving them there would fork them from their owner. Without a marker, a refresh would read such a section as leaked knowledge and move or drop it, and a freshly generated `CLAUDE.md` would never get it. Two HTML-comment markers handle this. They are invisible when the Markdown is rendered.

### Preserve block — in the repo's `CLAUDE.md`

```markdown
<!-- devexp:preserve id="family-rulebook" -->
## Rule 1 — Follow the family rulebook
...any markdown...
<!-- /devexp:preserve -->
```

- **`update-indexer`** never edits, rewrites, reorders, moves to `docs/` or removes anything from the opening marker line to the closing one. The block isn't classified as leaked or drifted, and it doesn't count toward the hard limits (line budget, code blocks, section length, citations). It stays at its position: normalising the file to the index shape may add sections around it, never through it. Repo-relative paths inside it are checked, and any that don't resolve are **reported, not fixed**. URLs and paths outside the repo (`../…`, absolute, `~`) aren't verified.
- **`gen-indexer`**, when regenerating over an existing `CLAUDE.md`, carries every preserve block over verbatim, in the same relative position (for example right after top rules such as a "Rule 0"), one block per `id`.

### Inherit block — in a parent directory's `CLAUDE.md`

```markdown
<!-- devexp:inherit id="family-rulebook" remote="gitlab.com[:/]acme/apps/" -->
## Rule 1 — Follow the family rulebook
The rulebook is cloned at `{{repo_to_parent}}/rulebook/`. Read its README first.
<!-- /devexp:inherit -->
```

- **Where it's looked for:** the `CLAUDE.md` of every ancestor directory of the repo root, walking up from the parent and stopping at `$HOME` inclusive (at `/` for a repo outside `$HOME`). If several ancestors define the same `id`, the nearest wins.
- **`remote`** (optional) is an extended regular expression (`grep -E`), matched unanchored against `git remote get-url origin`. The block applies when it matches, or when `remote` is absent. A repo with no `origin` only gets blocks without `remote`.
- **`gen-indexer`** inserts each matching block into the generated `CLAUDE.md` as a **preserve** block with the same `id`, right after the title block. **`update-indexer`** does the same when the repo's `CLAUDE.md` has no preserve block with that `id`, and says so in its report. **An existing preserve block with the same `id` always wins.** It's never overwritten or merged, even when the parent's content has since changed, so a changed parent block reaches existing repos only if you delete their preserve block and refresh.
- **`{{repo_to_parent}}`** is the only token. On insertion it's replaced with the relative path from the repo root to the directory holding the parent `CLAUDE.md`: `..` for the parent, `../..` for the grandparent. The inserted preserve block holds that literal path from then on.

### Rules for both

- Each marker sits alone on its own line. `id` is required and unique within a file, and blocks don't nest. A malformed block (unclosed, nested, or without an `id`) is reported, and the indexer won't write `CLAUDE.md` until it's fixed. It never guesses where a block ends.
- Both indexers list preserve blocks in their plan and report: kept, added from an inherit block, skipped (remote didn't match, or the `id` already existed), and unresolved paths. `/devxp` measures leakage outside the blocks, and treats a matching inherit block that's missing from `CLAUDE.md` as a reason to refresh it.
- Use the blocks sparingly. They exist for content whose owner is outside the repo. Knowledge this repo owns still goes in `docs/`.

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

Run `/devxp` — it detects a leaky CLAUDE.md (over 150 lines, code blocks, restated docs) and `update-indexer` performs these steps, verifying moved content against current code. By hand:

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
