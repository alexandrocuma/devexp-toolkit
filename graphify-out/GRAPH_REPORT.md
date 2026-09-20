# Graph Report - devexp-toolkit  (2026-09-19)

## Corpus Check
- 242 files · ~420,051 words
- Verdict: corpus is large enough that graph structure adds value.
- Unclassified: 4 file(s) not represented in the graph (top: (none) 3, .example 1)

## Summary
- 2157 nodes · 5239 edges · 129 communities (99 shown, 30 thin omitted)
- Extraction: 80% EXTRACTED · 20% INFERRED · 0% AMBIGUOUS · INFERRED: 1050 edges (avg confidence: 0.81)
- Token cost: 1,053,310 input · 0 output

## Community Hubs (Navigation)
- CLI Install Options & Paths
- Hook Registry & Installer
- opencode Config Merge
- Kimi Hook Adoption
- Install Mode Selection
- Install Manifest & Config Types
- Kimi Install Command
- Asset Repo Resolution & Ownership
- Kimi JSON Config Writer
- Grooming & Onboarding Agents
- Dotenv Loading
- Agent Memory & Templates
- Hook Command JSON Marshalling
- Dangerous Command Guard (opencode)
- Kimi MCP Install Tests
- Kimi Agent Frontmatter
- Uninstall Script Tests
- Docs Architecture & Kit Docs
- opencode Hook Modules
- Improve, Monitor & Refine Skills
- On-Save Advisory Hooks
- Docs Sync & Generation
- MCP Registration & Secrets
- Deliver Skill & Release Gates
- Scan Budget Tests (JS)
- Data Flow & Dependency Agents
- Agent Authoring Conventions
- Secret-in-Write Guard
- CLAUDE.md Indexer Agents
- Cleanup, Hygiene & Release Cut
- Architecture & Backend Review Agents
- CI/CD & Frontend Agents
- MCP Registry Tests
- Repo Frontmatter Tests
- Postmortem & Project Management
- Config Schema Root
- Cleanup Safety & Skill Taxonomy
- opencode Plugin Tests
- Agent Installer
- Shell Command Parser
- On-Save Path Tests (JS)
- Remote Install Script
- graphify Pipeline & Hooks
- Dangerous Command Guard Tests
- Release Build & CI Rationale
- Go Test Conventions & Coverage Gaps
- Architecture Overview & Install Flows
- Hook Authoring Contract
- Kimi Hook Shape & Advisory Hooks
- Remote Install Tests
- Phase 0 Orientation Protocol
- Fail-Closed Guards & CI Suites
- Workflow Recipes & Catalogs
- Uninstall Script
- Agent Installer Tests
- Kimi Adapter & Guard Verdicts
- Interpreter Proof Tests
- Skill Authoring Guide
- Skill Installer
- Impact Analysis & Migration
- Agent Installer
- Kimi Hook Removal
- devexp.config.json
- Indexer Limits & Install Ownership
- Kimi Adapter Tests
- Release Build & Install Rationale
- Atlas Drift & Coupling Analysis
- Secret Guard Tests (sh)
- Kimi Hook Selection
- Kimi Hook Install
- Wizard UI Tests
- Skills Config Schema
- Hook Config Schema
- Worktree-per-Ticket Isolation
- Changelog & Versioning
- Embedded Assets
- On-Save Path Tests (sh)
- MCP Recipes & Catalog
- Dangerous Guard Tests (sh)
- Large File Guard Tests
- Scan Budget Tests (sh)
- Config Disable Lookups
- Claude MCP Registration
- MCP Args Schema
- MCPs Config Schema
- Agents Config Schema
- Install Script Tests
- Kimi Runner Tests
- graphify Read Guard
- Dangerous Command Guard (bash)
- Interpreter Isolation Tests
- Scan Budget Helper
- MCP Env Schema
- opencode Plugin Entry
- Fail-Closed Tests
- CLI Entry Point
- Hooks Config Schema Block
- MCP required_env Schema
- MCP Scope Schema
- Skills Config Schema
- Bug Fix Recipe & Hook Compatibility
- Kimi Adapter Script
- graphify Session Sentinel
- Secret Guard Script (bash)
- PTY Helper (darwin)
- PTY Helper (linux)
- PTY Helper (other)
- Umask Tests
- File Ownership (other)
- File Ownership (unix)
- Changelog Format Policy
- Format-on-Save Hook
- graphify Grep Nudge Hook
- graphify Read Guard Hook
- graphify Sentinel Hook
- Large File Guard Hook
- Lint-on-Save Hook
- Test-on-Save Hook
- MCP Guide
- Go CLI Change Recipes
- Install Script
- Hooks Package Manifest
- opencode Package Manifest
- User-Invoked Skill Archetype
- GraphML Export
- Neo4j Export
- SVG Export
- Wiki Export

## God Nodes (most connected - your core abstractions)
1. `contains()` - 189 edges
2. `writeFile()` - 139 edges
3. `T` - 78 edges
4. `symlink()` - 46 edges
5. `T` - 44 edges
6. `T` - 40 edges
7. `kimiHome()` - 37 edges
8. `kimiScratch()` - 34 edges
9. `install()` - 33 edges
10. `removeStale()` - 32 edges

## Surprising Connections (you probably didn't know these)
- `Orchestrator Workflow Presets` --semantically_similar_to--> `Chaining Convention`  [INFERRED] [semantically similar]
  agents/opencode/orchestrator.md → docs/development/agent-architecture-reference.md
- `Agents Are Run By Reading The File, Never Spawned By Name` --conceptually_related_to--> `orchestrator Agent (opencode)`  [AMBIGUOUS]
  docs/development/conventions.md → agents/opencode/orchestrator.md
- `/refine — backlog refinement ceremony` --semantically_similar_to--> `Phase 0 — check shared context (atlas + graph)`  [INFERRED] [semantically similar]
  skills/refine/SKILL.md → templates/agent-template.md
- `Kimi PreToolUse Hook Adapter` --references--> `Ten Shipped Hooks (7 Default, 3 Opt-In graphify)`  [INFERRED]
  CHANGELOG.md → README.md
- `Phase 0 — Check Shared Context Before Discovery` --semantically_similar_to--> `CLAUDE.md as Indexer (No Knowledge, Pointers Only)`  [INFERRED] [semantically similar]
  agents/arch-review.md → CLAUDE.md

## Import Cycles
- None detected.

## Hyperedges (group relationships)
- **Specialist Audit → Synthesis Consolidation Flow** — agents_security_security, agents_arch_review_arch_review, agents_performance_performance, agents_tech_debt_tech_debt, agents_dep_audit_dep_audit, agents_root_cause_root_cause, agents_synthesis_synthesis [EXTRACTED 1.00]
- **Cleanup-safety guard family (id validation, scoped globs, preserve-on-failure)** — skills_improve_skill_safe_id_guard, skills_improve_skill_never_auto_delete, skills_improve_skill_toolkit_hygiene_sweep, skills_release_skill_ticket_id_safety_gate, skills_release_skill_retire_delivery_artifacts [EXTRACTED 1.00]
- **The Six Cleanup Safety Rules** — docs_guides_cleanup_safety_dry_run, docs_guides_cleanup_safety_confirm, docs_guides_cleanup_safety_scoped_glob, docs_guides_cleanup_safety_shared_state, docs_guides_cleanup_safety_memory_staleness, docs_guides_cleanup_safety_preserve_on_doubt [EXTRACTED 1.00]
- **devexp Development Lifecycle Cycle** — skills_devxp_skill_devxp, docs_reference_skills_refine, skills_deliver_skill_deliver, docs_reference_skills_release, docs_reference_skills_improve, docs_reference_skills_monitor, skills_cleanup_skill_cleanup, skills_graphify_skill_graphify [EXTRACTED 1.00]
- **devexp Lifecycle Orchestrator Loop** — docs_guides_quickstart_devxp, docs_guides_quickstart_refine, docs_guides_quickstart_deliver, docs_guides_quickstart_release, docs_guides_quickstart_improve, docs_guides_quickstart_monitor [EXTRACTED 1.00]
- **devexp development lifecycle phases (refine → release → improve → monitor)** — skills_refine_skill_refine, skills_release_skill_release, skills_improve_skill_improve, skills_monitor_skill_monitor [EXTRACTED 1.00]
- **CLAUDE.md-as-index documentation pipeline** — agents_gen_docs_gen_docs, agents_update_docs_update_docs, agents_gen_indexer_gen_indexer, agents_update_indexer_update_indexer, skills_devxp_skill_devxp, agents_update_indexer_index_shape [EXTRACTED 1.00]
- **Development Kit documentation pipeline (write kit → index it → keep both in sync)** — agents_gen_docs, agents_gen_indexer, agents_update_docs, agents_update_indexer, agents_docs_sync, skills_devxp_skill, agents_gen_docs_development_kit [EXTRACTED 1.00]
- **Fail-Closed Security Guard Chain Across CLIs** — changelog_kimi_adapter, changelog_fail_closed_guard, changelog_proof_of_scan, changelog_scan_budget, changelog_secret_in_write_guard, changelog_dangerous_cmd_guard [EXTRACTED 1.00]
- **Fail-Closed Guard System (budget + proof + three guards)** — docs_reference_hooks_secret_guard, docs_reference_hooks_secret_in_write_guard, docs_reference_hooks_dangerous_cmd_guard, docs_reference_hooks_scan_budget, docs_reference_hooks_proof_of_work, docs_reference_hooks_fail_closed [EXTRACTED 1.00]
- **graphify integration protocol (query before re-deriving; /graphify --update after writing, never bootstrap)** — skills_graphify_skill, agents_codebase_navigator, agents_data_flow, agents_dep_audit, agents_dev_agent, agents_docs_sync, agents_gen_indexer, agents_frontend_senior_dev, agents_feature_path_tracer [EXTRACTED 1.00]
- **Hook Safety Contract (fail-closed guard family)** — docs_development_hook_authoring_guide_fail_closed, docs_development_hook_authoring_guide_advisory, docs_development_hook_authoring_guide_scan_budget, docs_development_hook_authoring_guide_proof_of_work, docs_development_hook_authoring_guide_hard_block [EXTRACTED 1.00]
- **Phase 0 Shared Context / Codebase Atlas Protocol** — agents_codebase_navigator_codebase_navigator, agents_grooming_agent_grooming_agent, agents_impact_analysis_impact_analysis, agents_migration_migration, agents_onboarding_onboarding, agents_performance_performance, agents_postmortem_postmortem, agents_pr_feedback_pr_feedback, agents_pr_review_pr_review, agents_project_manager_project_manager, agents_root_cause_root_cause, agents_runbook_runbook, agents_scaffold_scaffold, agents_security_security, agents_synthesis_synthesis, agents_tech_debt_tech_debt, agents_tech_lead_tech_lead [EXTRACTED 1.00]
- **Phase 0 Shared Context Protocol (consult the codebase-navigator atlas before discovery)** — agents_codebase_navigator_atlas, agents_changelog, agents_ci_cd, agents_data_flow, agents_dep_audit, agents_dep_map, agents_dev_agent, agents_feature_path_tracer, agents_frontend_senior_dev [EXTRACTED 1.00]
- **Agents Orient Off One Shared Codebase Atlas Before Working** — agents_codebase_navigator, agents_arch_review_shared_context, agents_backend_senior_dev_shared_context, agents_backend_senior_dev_orientation_protocol, skills_graphify [EXTRACTED 1.00]
- **A Tag Publishes Only Through the Full CI Gate** — github_workflows_ci_test_job, github_workflows_ci_hooks_job, github_workflows_ci_govulncheck_job, github_workflows_release_goreleaser_job, _goreleaser_use_existing_draft [EXTRACTED 1.00]
- **Worktree Isolation Flow Across Skills and Guide** — docs_guides_worktree_per_ticket_convention, docs_guides_worktree_per_ticket_lifecycle, skills_deliver_skill_worktree_isolation, skills_deliver_skill_additional_directories_grant, skills_cleanup_skill_classification, docs_reference_skills_release [EXTRACTED 1.00]
- **The six Development Kit docs a repo must carry** — agents_update_docs_development_kit, docs_architecture_overview_architecture_overview, docs_development_conventions_conventions, docs_readme_documentation_index, agents_update_docs_release_guide_template [INFERRED 0.85]
- **graphify pipeline reference set (ingest, transcribe, extract, update, query, export)** — skills_graphify_references_add_watch_graphify_add, skills_graphify_references_transcribe_transcribe_all, skills_graphify_references_extraction_spec_extraction_subagent_prompt, skills_graphify_references_update_incremental_update, skills_graphify_references_query_traversal_modes, skills_graphify_references_exports_mcp_server, skills_graphify_references_github_and_merge_merge_graphs [INFERRED 0.85]
- **Phase 0 orientation protocol across agents** — docs_development_agent_architecture_reference_phase_0_pattern, docs_development_agent_architecture_reference_graphify_protocol, agents_codebase_navigator_codebase_navigator, agents_test_gen_test_gen, agents_test_runner_test_runner, agents_opencode_orchestrator_orchestrator [INFERRED 0.85]
- **Platform Detection + Adapter Table Pattern** — agents_grooming_agent_ticket_platform_adapter, agents_project_manager_platform_detection, agents_postmortem_tracker_detection, agents_pr_feedback_platform_detection, agents_pr_review_platform_detection [INFERRED 0.95]

## Communities (129 total, 30 thin omitted)

### Community 0 - "CLI Install Options & Paths"
Cohesion: 0.05
Nodes (148): claudePaths, FileInfo, Root, installOpts, Manifest, T, Config, installOpts (+140 more)

### Community 1 - "Hook Registry & Installer"
Cohesion: 0.07
Nodes (98): hookCmd, T, Hook, Registry, T, T, hookEntry, Hook (+90 more)

### Community 2 - "opencode Config Merge"
Cohesion: 0.07
Nodes (91): T, Hook, RawMessage, Registry, Hook, Registry, T, jsonContainer (+83 more)

### Community 3 - "Kimi Hook Adoption"
Cohesion: 0.08
Nodes (85): T, KimiHook, KimiHook, T, Registry, T, kimiRewritten(), TestIsKimiOwnCommand() (+77 more)

### Community 4 - "Install Mode Selection"
Cohesion: 0.05
Nodes (58): Command, Config, MCP, T, T, MCP, detection, announceAssetRoot() (+50 more)

### Community 5 - "Install Manifest & Config Types"
Cohesion: 0.05
Nodes (63): Manifest, installOpts, installOpts, kimiPaths, installOpts, Config, MCP, Registry (+55 more)

### Community 6 - "Kimi Install Command"
Cohesion: 0.13
Nodes (60): installOpts, kimiPaths, T, kimiPaths, kimiPaths, T, assertHostileRootRefused(), kimiAssetRepo() (+52 more)

### Community 7 - "Asset Repo Resolution & Ownership"
Cohesion: 0.09
Nodes (50): T, FileInfo, FS, FileMode, T, BehindSymlink(), mkdir(), TestBehindSymlink() (+42 more)

### Community 8 - "Kimi JSON Config Writer"
Cohesion: 0.11
Nodes (44): MCP, RawMessage, MCP, T, jsonObject, jsonObject, boolField(), decodeObject() (+36 more)

### Community 9 - "Grooming & Onboarding Agents"
Cohesion: 0.08
Nodes (52): Arch Review Agent, Backend Senior Dev Agent, Codebase Navigator Agent, Dep Audit Agent, Dev Agent, Feature Path Tracer Agent, Frontend Senior Dev Agent, Atlas Freshness Gate (Drift Classification) (+44 more)

### Community 10 - "Dotenv Loading"
Cohesion: 0.12
Nodes (39): T, File, FileMode, FileMode, T, T, File, FS (+31 more)

### Community 11 - "Agent Memory & Templates"
Cohesion: 0.07
Nodes (39): Watcher debounce window, /graphify add — URL ingestion, needs_update flag for doc/paper/image changes, URL type auto-detection (video, tweet, arXiv, PDF, image, webpage), --watch background folder watcher, graphify.serve stdio MCP server, Token reduction benchmark (>5000 words gate), calls edge direction and single-language constraint (+31 more)

### Community 12 - "Hook Command JSON Marshalling"
Cohesion: 0.12
Nodes (18): RawMessage, hookCmd, RawMessage, hookCmd, hookEntry, isJSONObject(), layout, member (+10 more)

### Community 13 - "Dangerous Command Guard (opencode)"
Cohesion: 0.07
Nodes (29): blank(), BLOCK_PATTERNS, bodyReader(), CLOSERS, commandEnd(), commandEndQuoted(), DEFINERS, downstreamOk() (+21 more)

### Community 14 - "Kimi MCP Install Tests"
Cohesion: 0.21
Nodes (33): MCP, T, install(), kimiFile(), readJSON(), serversOf(), TestEntryFingerprint_NumbersAsWritten(), TestInstallKimi_DoesNotAdoptAnIdenticalUserEntry() (+25 more)

### Community 15 - "Kimi Agent Frontmatter"
Cohesion: 0.13
Nodes (30): bodyUsesKimiPromptVar(), repoAgentFiles(), splitRepoFrontmatter(), TestKimiPromptVarsCoversBasePrompt(), TestRepoAgentsFrontmatterStrictYAML(), InstallKimi(), kimiSubagents(), kimiTools() (+22 more)

### Community 16 - "Uninstall Script Tests"
Cohesion: 0.10
Nodes (20): cc_install(), check(), expect(), expect_file(), fill_paths(), kimi_install(), kimi_run(), kimi_un() (+12 more)

### Community 17 - "Docs Architecture & Kit Docs"
Cohesion: 0.10
Nodes (28): Setup (Kit Doc), install.sh Never Rebuilds an Existing Binary, Output Template Pattern, Severity Ladder Pattern, Testing (Kit Doc), Docs Architecture Pattern, The Development Kit, Standard docs/ Folder Tree (+20 more)

### Community 18 - "opencode Hook Modules"
Cohesion: 0.13
Nodes (17): BLOCK_PATTERNS, dangerousCmdGuard(), DevExpPlugin(), FORMAT_EXTS, formatOnSave(), largeFileGuard(), lintOnSave(), SECRET_PATTERNS (+9 more)

### Community 19 - "Improve, Monitor & Refine Skills"
Cohesion: 0.11
Nodes (25): .devexp/health-baseline.json trend baseline, Health scorecard (8 dimensions, RYG thresholds), /improve — continuous improvement cycle, Evidence-grounded blameless retrospective, Tech debt triage (tech-debt agent delegation), Equal-weighted composite health score, Config-as-code fallback review mode, Critical-path observability coverage cross-reference (+17 more)

### Community 20 - "On-Save Advisory Hooks"
Cohesion: 0.15
Nodes (12): FORMAT_EXTS, runFormatter(), runTests(), SOURCE_EXTS, TEST_MARKERS, countLines(), editedPath(), findRoot() (+4 more)

### Community 21 - "Docs Sync & Generation"
Cohesion: 0.13
Nodes (23): docs/ Folder-Index Fast Path, Autonomous Decision Rules, Docs Sync Agent, Change Map (changed file → affected doc surfaces), Doc Surface Map, Scope Creep Is a Bug, Documentation Generator Agent, Development Kit (+15 more)

### Community 22 - "MCP Registration & Secrets"
Cohesion: 0.10
Nodes (22): claude mcp add Registration, mcps/.env Secrets File, MCP Entry Field Reference, opencode config.json mcp Key, MCP Secret Precedence, mcps/registry.json, --reinstall-mcps Config Refresh, required_env Gating (+14 more)

### Community 23 - "Deliver Skill & Release Gates"
Cohesion: 0.14
Nodes (22): Add a Skill Recipe, The Eight Slash Commands, /improve Orchestrator, Three Install Facts That Shape a Skill, /monitor Orchestrator, /refine Orchestrator, /release Orchestrator, Release Guide Ties the Cycle Together (+14 more)

### Community 24 - "Scan Budget Tests (JS)"
Cohesion: 0.13
Nodes (15): CASES, CC, HERE, INSIDE, jsSeconds(), seconds(), shellSeconds(), SLOW_GUARD (+7 more)

### Community 25 - "Data Flow & Dependency Agents"
Cohesion: 0.15
Nodes (21): codebase-navigator Agent, Codebase Atlas, Data Flow Mapper Agent, Data Flow Map, Silent Drop / Data Loss Risk, Dependency Auditor Agent, Previously Reviewed / Accepted Vulnerabilities, Security vs Staleness Separation (+13 more)

### Community 26 - "Agent Authoring Conventions"
Cohesion: 0.20
Nodes (21): Parallel-By-Default Execution Model, orchestrator Agent (opencode), Orchestrator Workflow Presets, Trivially Passing Tests Are Worse Than No Tests, Go Table-Driven Tests Keyed By Case Name, Test Gen Agent, Coverage Analysis of Uncovered Critical Paths, Flaky Test Detection (3-Run Comparison) (+13 more)

### Community 27 - "Secret-in-Write Guard"
Cohesion: 0.12
Nodes (17): patchAddedText(), SECRET_PATTERNS, secretInWriteGuard(), ALLOW, argsFor(), BLOCK, body(), CODE (+9 more)

### Community 28 - "CLAUDE.md Indexer Agents"
Cohesion: 0.18
Nodes (20): gen-docs Agent, gen-indexer Agent, Development Kit (six kit docs), Release Guide Template (per-repo release contract), Doc Routing Rules, Standard Documentation Tree, update-docs Agent, CLAUDE.md Is The Index, docs/ Is The Knowledge Store (+12 more)

### Community 29 - "Cleanup, Hygiene & Release Cut"
Cohesion: 0.14
Nodes (20): permissions.additionalDirectories grant in main checkout settings.local.json, Convention divergence audit, Never auto-delete stance, safe_id() identifier validation guard, Stale work + dead code scan, Toolkit hygiene sweep (repo-wide orphan retirement), Worktree isolation for parallel cleanup streams, awaiting-external — waiting is a state, not a failure (+12 more)

### Community 30 - "Architecture & Backend Review Agents"
Cohesion: 0.14
Nodes (19): arch-review Agent, Anti-Pattern Detection Catalog, Honest Architecture Health Score, Architectural Pattern Identification, arch-review Persistent Agent Memory, backend-senior-dev Agent, Review Mode vs Fix Mode, Verify Library APIs via context7 Before Flagging (+11 more)

### Community 31 - "CI/CD & Frontend Agents"
Cohesion: 0.14
Nodes (19): CI/CD Engineer Agent, Pinned Action Versions Rule, Pipeline Failure Diagnosis, Production Deploy Approval Gate, Required Secrets Documentation, dev-agent, Bias Toward Action, Canonical Example Matching (+11 more)

### Community 32 - "MCP Registry Tests"
Cohesion: 0.19
Nodes (15): T, RawMessage, MCP, captureStdout(), TestAddClaude_NonExec(), TestInstallOpencode(), TestInstallOpencode_ConfigEdgeCases(), TestInstallOpencode_RefusesUnmergeableConfig() (+7 more)

### Community 33 - "Repo Frontmatter Tests"
Cohesion: 0.25
Nodes (16): T, T, grantAssetFiles(), grantLineIsNested(), repoSkillFiles(), splitRepoFrontmatter(), TestRepoAssetsGrantUsesNestedPermissionsKey(), TestRepoSkillsFrontmatterStrictYAML() (+8 more)

### Community 34 - "Postmortem & Project Management"
Cohesion: 0.15
Nodes (17): CI/CD Agent, Ticket Platform Adapter, Do Not Fabricate History, Postmortem Action Items, Contributing Factors, Postmortem Agent, Issue Tracker Detection (postmortem), Where We Got Lucky (+9 more)

### Community 35 - "Config Schema Root"
Cohesion: 0.12
Nodes (16): additionalProperties, description, $schema, title, type, Hand Tools an Absolute Edited Path, devexp-plugin.js opencode Entry Point, devexp/hooks.json Selection Contract (+8 more)

### Community 36 - "Cleanup Safety & Skill Taxonomy"
Cohesion: 0.15
Nodes (17): Skill Naming and Merge-Over-Split, Skill vs Agent Distinction, Cleanup Safety, C1 — /release Phase 8 Per-Ticket Retirement, C2 — /improve Repo-Wide Hygiene Sweep, /cleanup On-Demand Command, Confirm or Log Every Destructive Action, Dry-Run First (+9 more)

### Community 37 - "opencode Plugin Tests"
Cohesion: 0.12
Nodes (8): BENIGN, HERE, PROBES, READ_ENV, REPO, sleep(), trees, waitFor()

### Community 38 - "Agent Installer"
Cohesion: 0.15
Nodes (8): MCP, ConfigRefusedError, ocEntry, InstallOpencode(), loadOpencodeConfig(), Added(), AddedLine(), Updated()

### Community 40 - "On-Save Path Tests (JS)"
Cohesion: 0.13
Nodes (14): BASE, check(), GO, HERE, JS, JST, modules, OUT (+6 more)

### Community 41 - "Remote Install Script"
Cohesion: 0.16
Nodes (14): Go Toolchain Pinning (go1.26.8), Contributor Prerequisites, govulncheck Exit Code Policy, Vulnerability Scan (scripts/govulncheck.sh), Weekly Scheduled Scan and Its Failure Modes, CI Gate on the Tagged Commit, cli Release Target, Cut Procedure (tag is the version) (+6 more)

### Community 42 - "graphify Pipeline & Hooks"
Cohesion: 0.17
Nodes (15): graphify-grep-nudge Hook, graphify-read-guard Tapering Gate, graphify-session-sentinel Hook, Project-Scoped graphify MCP, The Development Kit (docs/ knowledge store), Part A — Structural AST Extraction, Chunk File on Disk as the Subagent Success Signal, Step 5 — Label Communities (+7 more)

### Community 43 - "Dangerous Command Guard Tests"
Cohesion: 0.17
Nodes (12): blockReason(), dangerousCmdGuard(), maskInert(), ALLOW, BLOCK, blocked(), crafted(), ENTRY (+4 more)

### Community 44 - "Release Build & CI Rationale"
Cohesion: 0.16
Nodes (12): Four CI Suites Must Pass Before Done, govulncheck Vulnerability Scan Job, Hook Test Suites Job, Read-Only Workflow Token, Jobs Defined Once, Reused via workflow_call, Weekly Unattended Suite Run, CI Workflow, Tag Publishes Only From a Passing Commit (+4 more)

### Community 45 - "Go Test Conventions & Coverage Gaps"
Cohesion: 0.14
Nodes (13): Gotcha — go build Fails Without Staged Assets, Hook Deployment Checklist, hooks/registry.json Entry, Absolute HOME Requirement, Wizard vs Non-Interactive Install Mode, Known Coverage Gaps, Go Unit Tests (stdlib testing only), No Mock Library — Isolate With Real Resources (+5 more)

### Community 46 - "Architecture Overview & Install Flows"
Cohesion: 0.22
Nodes (14): all-tools-agent Fixture, basic-agent Fixture, no-frontmatter Fixture, ADR Index (empty), devexp Architecture Overview, Asset Root Resolution (clone vs embedded), Claude Code Install Flow, Kimi Code CLI Install Flow (+6 more)

### Community 47 - "Hook Authoring Contract"
Cohesion: 0.14
Nodes (14): Hook Authoring Guide, Advisory Hook (Fail Open but Loud), Byte-Exact Path Extraction, Claude Code Hook Shell Script, Queued file.edited Dispatch, Guards Always Hard-Block, devexp Hook, python3 -I Interpreter Isolation (+6 more)

### Community 48 - "Kimi Hook Shape & Advisory Hooks"
Cohesion: 0.20
Nodes (14): Merge Discipline (serialized, conflicts surfaced), dangerous-cmd-guard Hook, Fail-Closed Guard Principle, Kimi Code CLI Adapter, large-file-guard Hook, maskInert / scan_text — Blanking Text That Cannot Run, Advisory On-Save Hooks (lint/format/test), Proof of Work (interpreter and grep probes) (+6 more)

### Community 49 - "Remote Install Tests"
Cohesion: 0.21
Nodes (5): check(), ko(), ok(), run_remote(), remote-install.test.sh script

### Community 50 - "Phase 0 Orientation Protocol"
Cohesion: 0.23
Nodes (12): Phase 0 — Check Shared Context Before Discovery, Phase 0 — Calibrate Review Against the Atlas, Development Kit + CLAUDE.md as Strict Index, devexp:preserve and devexp:inherit Blocks, CLAUDE.md as Indexer (No Knowledge, Pointers Only), Rule — Never Edit Deployed Copies, Rule — Re-run install.sh After Editing Assets, devexp Toolkit (+4 more)

### Community 51 - "Fail-Closed Guards & CI Suites"
Cohesion: 0.23
Nodes (12): Fail-Closed Guard Contract, Proof of Work (DEVEXP_SCAN_PROOF), Scan Budget, Four CI Test Suites, opencode Hook .test.js Suite, Claude Code Hook .test.sh Suite, Installer Script Test Suite, Kimi Hook Test Suite (+4 more)

### Community 52 - "Workflow Recipes & Catalogs"
Cohesion: 0.21
Nodes (12): Add a Hook Recipe, Add an Agent Recipe, Change the Data Model (JSON/Frontmatter Schemas), Ground Rules for Every Change, Agent Catalog, Agent Frontmatter Fields, Custom Agents Are Read, Never subagent_type, opencode-Exclusive Agents (+4 more)

### Community 53 - "Uninstall Script"
Cohesion: 0.33
Nodes (10): behind_link(), die(), error(), info(), removable(), remove_entry(), select_target(), success() (+2 more)

### Community 54 - "Agent Installer Tests"
Cohesion: 0.47
Nodes (10): captureStdout(), readTestdata(), TestInstall_AgentFilesNeverWrittenInPlace(), TestInstall_SymlinkedAgentEntry(), TestInstallClaude(), TestInstallOpencode(), TestInstallOpencodeExclusive(), TestTransformForOpencode() (+2 more)

### Community 55 - "Kimi Adapter & Guard Verdicts"
Cohesion: 0.25
Nodes (11): dangerous-cmd-guard, devexp Install Manifest (Ownership Record), Fail-Closed Guard Verdicts, Kimi PreToolUse Hook Adapter, Kimi Honours Less — Installer Announces the Gaps, Marked config.toml Hooks Block, Proof-of-Scan Rule, Security Guard Scan Budget (+3 more)

### Community 56 - "Interpreter Proof Tests"
Cohesion: 0.29
Nodes (5): blind_to(), blocks(), check(), stub(), interpreter-proof.test.sh script

### Community 57 - "Skill Authoring Guide"
Cohesion: 0.18
Nodes (11): Skill Authoring Guide, SKILL.md Frontmatter (name, description), Numbered Phase Pattern, Orchestrator Skill Archetype, Role Statement Convention, Skill (Behavioral Overlay), Orchestrated Sub-Skill Archetype, Triggered by Section (+3 more)

### Community 58 - "Skill Installer"
Cohesion: 0.36
Nodes (9): CopyDir(), InstallClaude(), InstallOpencode(), isDisabled(), stripFrontMatterName(), warnSymlinked(), InstallKimi(), splitFrontmatter() (+1 more)

### Community 59 - "Impact Analysis & Migration"
Cohesion: 0.24
Nodes (10): Dep Map Agent, Every-Caller Audit (Phase 6b), Stop Transitive Tracing at API Boundaries, Blast Radius Report, Dynamic and String-Based Reference Scan, Impact Analysis Agent, Dependency Risk Scoring, Migration Audit Report (+2 more)

### Community 60 - "Agent Installer"
Cohesion: 0.44
Nodes (9): InstallClaude(), InstallOpencode(), InstallOpencodeExclusive(), isDisabled(), keepSymlinkedEntry(), resolveModel(), transformForOpencode(), removeLegacy() (+1 more)

### Community 61 - "Kimi Hook Removal"
Cohesion: 0.38
Nodes (9): Root, foldMatchKimi(), isKimiHookPath(), openKimiRemovalDir(), pruneEmptyKimiDirs(), quoteAllKimi(), removeKimiFiles(), removeOneKimiFile() (+1 more)

### Community 62 - "devexp.config.json"
Cohesion: 0.20
Nodes (9): agents, disabled, hooks, disabled, $schema, mcps, model, skills (+1 more)

### Community 63 - "Indexer Limits & Install Ownership"
Cohesion: 0.22
Nodes (10): Indexer Hard Limits, devexp:inherit Block, devexp:preserve Block, {{repo_to_parent}} Token, Atomic Save (WriteFileAtomic), Disabling Now Removes, .devexp-manifest.json Ownership Manifest, settings.json Ownership Rules (+2 more)

### Community 64 - "Kimi Adapter Tests"
Cohesion: 0.38
Nodes (6): check(), expect(), expect_env(), note(), run(), adapter.test.sh script

### Community 65 - "Release Build & Install Rationale"
Cohesion: 0.25
Nodes (9): devexp Cross-Platform Build Matrix, goreleaser Release Config, Draft Release Published Only With Assets, Broken MCP Registry Surfaced, Not Swallowed, chooseInstallMode — Degrade to Announced Default, nil Means Never Asked, Empty Means Answered None, Release Targets and Per-Repo Release Guide, curl | bash Remote Install (+1 more)

### Community 66 - "Atlas Drift & Coupling Analysis"
Cohesion: 0.22
Nodes (9): Drift Classification (CURRENT/SMALL/BIG), Incremental Atlas Update Procedure, Structural Sections (Layer Map, Canonical Example, Convention Anchors), Orphaned Write Detection, Circular Dependency Detection, High-Coupling Hotspot, Internal Import Graph, Layer Violation Detection (+1 more)

### Community 67 - "Secret Guard Tests (sh)"
Cohesion: 0.33
Nodes (4): allow(), block(), run(), secret-in-write-guard.test.sh script

### Community 68 - "Kimi Hook Selection"
Cohesion: 0.33
Nodes (7): Registry, T, KimiNames(), SelectKimi(), TestSelectKimi(), TestSelectKimi_RepoRegistry(), KimiHook

### Community 69 - "Kimi Hook Install"
Cohesion: 0.33
Nodes (8): KimiHook, Registry, copyKimiFile(), InstallKimi(), kimiInstallFiles(), staleNew(), UninstallKimi(), Skipped()

### Community 70 - "Wizard UI Tests"
Cohesion: 0.42
Nodes (8): File, T, captureOutput(), drain(), equalStrings(), TestBuildMultiSelectDisplay(), TestMultiSelectLogic(), TestOutputFormatters()

### Community 71 - "Skills Config Schema"
Cohesion: 0.22
Nodes (9): additionalProperties, properties, type, default, description, examples, type, agents (+1 more)

### Community 72 - "Hook Config Schema"
Cohesion: 0.22
Nodes (9): description, type, type, properties, description, type, command, description (+1 more)

### Community 73 - "Worktree-per-Ticket Isolation"
Cohesion: 0.31
Nodes (9): Worktree-per-Ticket Convention, Failure Preserves the Worktree, additionalDirectories Worktree Grant, Worktree Lifecycle (create, grant, work, merge, remove), Worktree Naming Scheme, Single-Stream / No-Isolation Fallback, Cleanup Candidate Classification (live vs finished), settings.local.json permissions.additionalDirectories Grant (+1 more)

### Community 74 - "Changelog & Versioning"
Cohesion: 0.25
Nodes (8): Changelog Agent, Conventional Commit Parsing, Keep a Changelog Format, Release Platform Notes Format, Semantic Version Bump Logic, PII Tracking Through the Flow, Release Guide Template, release Skill

### Community 75 - "Embedded Assets"
Cohesion: 0.29
Nodes (6): ExtractFile(), TestEmbeddedFS(), TestExtractFile(), FileMode, FS, T

### Community 76 - "On-Save Path Tests (sh)"
Cohesion: 0.36
Nodes (6): expect(), PATH, setup(), STUB_LOG, want(), on-save-path.test.sh script

### Community 77 - "MCP Recipes & Catalog"
Cohesion: 0.36
Nodes (8): Add an MCP Server Recipe, Add Configuration, context7 MCP, MCP Registry Format (stdio and HTTP/SSE), MCP API Keys via mcps/.env, MCP Server Lifecycle — Register, Never Start, ui-inspector MCP, MCPs README Catalog

### Community 78 - "Dangerous Guard Tests (sh)"
Cohesion: 0.38
Nodes (3): expect(), expect_big(), dangerous-cmd-guard.test.sh script

### Community 79 - "Large File Guard Tests"
Cohesion: 0.43
Nodes (5): baseline(), check(), LFG_SENTINEL, sibling(), large-file-guard.test.sh script

### Community 80 - "Scan Budget Tests (sh)"
Cohesion: 0.43
Nodes (3): check(), prologue_case(), scan-budget.test.sh script

### Community 81 - "Config Disable Lookups"
Cohesion: 0.48
Nodes (4): RawMessage, Config, Load(), sliceContains()

### Community 82 - "Claude MCP Registration"
Cohesion: 0.62
Nodes (6): MCP, AddClaude(), InstallClaude(), isInstalledClaude(), RemoveClaude(), resolveStr()

### Community 83 - "MCP Args Schema"
Cohesion: 0.29
Nodes (7): default, items, type, items, type, args, items

### Community 84 - "MCPs Config Schema"
Cohesion: 0.29
Nodes (7): additionalProperties, required, default, description, items, type, mcps

### Community 85 - "Agents Config Schema"
Cohesion: 0.29
Nodes (7): description, examples, type, properties, model, $schema, type

### Community 86 - "Install Script Tests"
Cohesion: 0.48
Nodes (5): check(), ko(), ok(), run_install(), install.test.sh script

### Community 87 - "Kimi Runner Tests"
Cohesion: 0.52
Nodes (5): check_allowed(), check_blocked(), kimi_envelope(), run_hook(), runner.test.sh script

### Community 88 - "graphify Read Guard"
Cohesion: 0.33
Nodes (4): defaultState(), loadState(), REQUIRED, SOURCE_EXTS

### Community 89 - "Dangerous Command Guard (bash)"
Cohesion: 0.60
Nodes (4): devexp_grep(), devexp_grep_probe(), matches(), dangerous-cmd-guard.sh script

### Community 90 - "Interpreter Isolation Tests"
Cohesion: 0.50
Nodes (3): check(), ISO_SENTINEL, interpreter-isolation.test.sh script

### Community 91 - "Scan Budget Helper"
Cohesion: 0.50
Nodes (3): devexp_scan_budget(), devexp_scan_budget_run(), scan-budget.sh script

### Community 92 - "MCP Env Schema"
Cohesion: 0.40
Nodes (5): type, additionalProperties, default, type, env

### Community 93 - "opencode Plugin Entry"
Cohesion: 0.60
Nodes (4): blockAll(), DevExpPlugin(), misconfigured(), SECURITY_GUARDS

### Community 94 - "Fail-Closed Tests"
Cohesion: 0.83
Nodes (3): check(), quiet(), fail-closed.test.sh script

### Community 96 - "Hooks Config Schema Block"
Cohesion: 0.50
Nodes (4): additionalProperties, properties, type, hooks

### Community 97 - "MCP required_env Schema"
Cohesion: 0.50
Nodes (4): required_env, default, description, type

### Community 98 - "MCP Scope Schema"
Cohesion: 0.50
Nodes (4): scope, default, enum, type

### Community 99 - "Skills Config Schema"
Cohesion: 0.50
Nodes (4): skills, additionalProperties, properties, type

### Community 100 - "Bug Fix Recipe & Hook Compatibility"
Cohesion: 0.50
Nodes (4): Fix a Bug Recipe, Hook CLI Compatibility Matrix, How Hooks Work Across Three CLIs, Scope Test — In Scope Is Fixed, Out of Scope Is Filed

## Ambiguous Edges - Review These
- `Development Kit + CLAUDE.md as Strict Index` → `Phase 0 — Check Shared Context Before Discovery`  [AMBIGUOUS]
  CHANGELOG.md · relation: conceptually_related_to
- `PII Tracking Through the Flow` → `Release Guide Template`  [AMBIGUOUS]
  agents/data-flow.md · relation: conceptually_related_to
- `Previously Reviewed / Accepted Vulnerabilities` → `Dependency Mapper Agent`  [AMBIGUOUS]
  agents/dep-audit.md · relation: conceptually_related_to
- `Agent File Checklist` → `update-indexer Agent`  [AMBIGUOUS]
  docs/development/conventions.md · relation: conceptually_related_to
- `Agents Are Run By Reading The File, Never Spawned By Name` → `orchestrator Agent (opencode)`  [AMBIGUOUS]
  docs/development/conventions.md · relation: conceptually_related_to
- `devexp/hooks.json Selection Contract` → `devexp.config.schema.json`  [AMBIGUOUS]
  docs/development/hook-authoring-guide.md · relation: semantically_similar_to
- `Add a Hook Recipe` → `The Scan Budget`  [AMBIGUOUS]
  docs/guides/workflows.md · relation: references

## Knowledge Gaps
- **281 isolated node(s):** `Manifest`, `Manifest`, `staleShape`, `Command`, `MCP` (+276 more)
  These have ≤1 connection - possible missing edges or undocumented components. (Counts symbols only; 464 node(s) total have ≤1 connection when file, concept and rationale nodes are included.)
- **30 thin communities (<3 nodes) omitted from report** — run `graphify query` to explore isolated nodes.

## Suggested Questions
_Questions this graph is uniquely positioned to answer:_

- **What is the exact relationship between `Development Kit + CLAUDE.md as Strict Index` and `Phase 0 — Check Shared Context Before Discovery`?**
  _Edge tagged AMBIGUOUS (relation: conceptually_related_to) - confidence is low._
- **What is the exact relationship between `PII Tracking Through the Flow` and `Release Guide Template`?**
  _Edge tagged AMBIGUOUS (relation: conceptually_related_to) - confidence is low._
- **What is the exact relationship between `Previously Reviewed / Accepted Vulnerabilities` and `Dependency Mapper Agent`?**
  _Edge tagged AMBIGUOUS (relation: conceptually_related_to) - confidence is low._
- **What is the exact relationship between `Agent File Checklist` and `update-indexer Agent`?**
  _Edge tagged AMBIGUOUS (relation: conceptually_related_to) - confidence is low._
- **What is the exact relationship between `Agents Are Run By Reading The File, Never Spawned By Name` and `orchestrator Agent (opencode)`?**
  _Edge tagged AMBIGUOUS (relation: conceptually_related_to) - confidence is low._
- **What is the exact relationship between `devexp/hooks.json Selection Contract` and `devexp.config.schema.json`?**
  _Edge tagged AMBIGUOUS (relation: semantically_similar_to) - confidence is low._
- **What is the exact relationship between `Add a Hook Recipe` and `The Scan Budget`?**
  _Edge tagged AMBIGUOUS (relation: references) - confidence is low._