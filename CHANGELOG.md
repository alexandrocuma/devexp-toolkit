# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- **devexp agents are transformed for Kimi Code CLI (#113).** `$KIMI_CODE_HOME/agents/` gets all 34 agents with their tool names mapped onto Kimi's — `WebFetch` becomes `FetchURL`, the four `Task*` tools and `TodoWrite` collapse onto `TodoList`, and a name with no equivalent is dropped and **named in the output**, because Kimi drops an unknown tool silently and writes nothing to its log. `color`, `memory` and `model` are dropped, since Kimi ignores them. Each of the nine agents that can delegate gets an explicit `subagents` allowlist — every agent this run installed that cannot itself delegate, plus Kimi's `explore` and `plan` — so a delegation chain stays one level deep and a disabled agent is never offered. Body references to `~/.claude/agents/<name>.md` are repointed at the Kimi install; `~/.claude/agent-memory` is deliberately left alone, so both CLIs go on sharing one atlas. Front matter is rebuilt through a YAML parser rather than by splitting lines, so a description keeps its exact value whatever quoting it used.
  Every ported agent ends with Kimi's `${base_prompt}` marker. **A custom agent body in Kimi replaces the whole system prompt where Claude Code appends to it**, so without that line an agent renders with no tool guidance, no `AGENTS.md`, no working-directory listing and an empty skills catalog — and still gets every tool, so nothing looks wrong. An agent whose tools all lack a Kimi equivalent is refused rather than installed: Kimi would load it, list it and offer it as a delegation target with no tools and no error anywhere.
- **Shipped skills are valid for a strict YAML parser, and their shell snippets survive Kimi's parameter expansion (#113).** Three `SKILL.md` descriptions held an unquoted `": "`, which Claude Code accepts and a strict YAML parser rejects — Kimi Code CLI would have skipped `/deliver`, `/improve` and `/release` entirely. Three more held bash snippets using `$1` or awk's `$0`, which Kimi rewrites to the empty string when it expands a skill's parameters, silently breaking a delete-guard validator and the release-notes extractor. The descriptions are now quoted and the snippets use the exact substitutes `${1}` and `$(0)`. **Behaviour is unchanged for Claude Code and opencode users** — the text and the shell semantics are identical. Two repo-level tests keep both classes at zero.
- **Any combination of install targets, and a `--target` flag (#111).** `devexp install --target claude,kimi` installs for exactly those CLIs; the flag is repeatable and comma-separated. When more than one CLI is detected the interactive prompt is now a checklist rather than a three-way "Claude Code / opencode / Both" choice, so any combination of the three can be picked. Deselecting everything is refused instead of being read as "install for all", and asking for a CLI that is not installed is refused instead of quietly falling back to another.
- **Kimi Code CLI is a selectable target that installs nothing yet (#111).** It can be picked in the wizard or with `--target kimi`, and doing so writes nothing: agents, skills, MCPs and hooks arrive in #112-#114. So that it cannot be mistaken for a successful install, the run names the directory it left untouched, a multi-target run repeats it in the summary, and a run whose every target installs nothing exits non-zero instead of printing `All done.`
- **Kimi Code CLI detection (#111).** devexp recognises Kimi Code CLI `0.31.0` or newer on `PATH` from the bare semver `kimi --version` prints. An older one is skipped with the minimum named, and the legacy Python kimi-cli — which ships a binary of the same name, with overlapping version numbers, and is told apart by its `kimi, version <x>` output — is skipped as unsupported rather than mistaken for Kimi Code. A binary that answers with anything else is skipped rather than guessed at, and the probe is the first exec in the installer with a timeout, so a `kimi` that blocks cannot hang an install.
- **Kimi Code CLI target paths (#111).** `kimiTargetPaths` resolves the agents and skills directories, `mcp.json`, `config.toml`, the devexp manifest and the backup directory under `$KIMI_CODE_HOME`, defaulting to `~/.kimi-code`. Nothing installs there yet (#112-#114); the paths exist so a `$KIMI_CODE_HOME` devexp must not write to — relative, the filesystem root, the home directory itself, or anything containing the home directory — is refused now rather than discovered later. An unrelated absolute path such as `/opt/kimi` is deliberately allowed: removals stay guarded there because the guard root is the Kimi root's parent, not `$HOME`.

### Fixed

- **A non-interactive install with more than one CLI reached a prompt nobody could answer (#111).** With both `claude` and `opencode` on `PATH` and no terminal — CI, a closed stdin, or a pipe — the installer showed the "Platform" prompt anyway. With stdin at EOF it died there (`Error: ^D`, exit 1, nothing installed); with a pipe it silently consumed whatever was piped in as the answer. Both now install for **every detected CLI** without prompting.

  **If a script of yours pipes an answer into the installer** (`printf '\n' | ./install.sh`, `yes | …`), it used to install for the one target that answer selected and will now install for all of them — including first-time writes under `~/.config/opencode/`. Pass `--target claude` (or whichever you meant) to pin the old outcome explicitly. This is the one change in this release that alters what an existing two-target install does.

  "No terminal" is decided by the same terminal check promptui prompts through, so stdin redirected from `/dev/null` counts as non-interactive even though it is a character device.

- **A failed install no longer dumps the usage block or says everything twice (#111).** `devexp install` errors are printed once, by the same code that sets the exit status, and the flag list is shown only when a flag was actually wrong. It matters because selecting only a target that installs nothing yet is a deliberate non-zero exit, and the notice explaining it was ending up three screens above the usage dump.
- **A `kimi` that fails is reported as a failure (#111).** A binary that printed a usable version and then exited non-zero was reported as unidentifiable output, with no hint that the command had failed at all; a timed-out probe surfaced under the same heading. The notice now names the failure and quotes whatever the probe managed to print.
- **Foreign text reaching the terminal is quoted and capped (#111).** The Kimi root comes from `$KIMI_CODE_HOME` and the probe notice repeats whatever `kimi` wrote, so both are printed quoted, and the probe output is truncated — an embedded newline could otherwise forge a line of devexp output and an escape sequence could reach the terminal. Same rule the stale-file removal already follows.

### Changed

- **The multi-CLI announcement is comma-separated (#111).** `Detected: Claude Code and opencode` is now `Detected: Claude Code, opencode`, because the "and" form does not extend to three targets. Nothing in the repo greps it, but a script of yours might.

## [0.9.4] - 2026-09-18

### Fixed

- **A security guard could allow a tool call when its scanning step never ran
  (#168).** The Claude Code guards do their parsing and matching in a program
  run through an interpreter found on `PATH`, and the shell could read only
  that program's exit status — so anything on `PATH` that reported success
  without doing the work (a wrapper, a shim, a broken virtualenv) read as "the
  scan found nothing", and the call went through unscanned. The scan budget
  (#162) widened it: the watchdog runs through the same interpreter, so such a
  stand-in answered before the guard had even read the envelope. Every
  `fail_closed` guard now allows only against positive proof that the code
  which decides actually ran — the scanning step reports a per-invocation token
  back as the first line of the output the guard already captures, and the
  budget watchdog reports one of its own on a descriptor opened for it alone.
  A missing token blocks, whatever the exit status. A token is refused unless it
  is this invocation's own, so one captured from an earlier run does not carry
  over, and the descriptor the watchdog reports on reaches the watchdog alone —
  anything the guarded run left holding it would keep the guard waiting past its
  budget, which is the #162 fail-open by another road.
  `grep`, whose status is the only thing `dangerous-cmd-guard`'s pattern checks
  read, now has to answer three questions with known answers — a hit, a miss and
  a case-insensitive hit — **in the mode and the dialect those checks use**,
  through the one function that hands a pattern to grep: same `-q`, same `-e`,
  same here-string, patterns carrying the same constructs the real ones do
  (`\s`, `\b`, `\S`, a POSIX class, a bracket range, a literal brace,
  alternation, `+`, `*`, `?`, both anchors), and a two-line subject whose answer
  is on the second line. A grep honest in some other mode and blind under `-q`
  would otherwise pass and report every blocked command as clean; so would one
  that reads `\s` as a literal, which is what a strict-POSIX or busybox grep
  does — an accident on somebody's PATH, not an attack. The suite fails if a
  pattern ever uses a construct no probe exercises, or if a second `grep`
  invocation appears anywhere in the guards. v0.9.1 made a grep *error* block;
  "no match" was still believed.
  The budget's own properties are unchanged (prologue floor, argv sentinel,
  depth counter, ceiling, the 0-or-2 rule), a caller with no stdout still gets a
  decision instead of a wedged guard, and the added cost is +1 ms per guarded
  call for the two secret guards and +6 ms for `dangerous-cmd-guard` (40 paired
  runs against v0.9.3; the three probes are 4.9 ms of `grep` against 2.0 ms,
  timed on their own), no extra interpreter start. The distribution has a tail
  the same machine shows in v0.9.3 — an occasional run costing ~50 ms more —
  and two more short-lived processes make hitting it likelier, 2 runs in 40
  becoming 14. `hooks/claude-code/interpreter-proof.test.sh` covers all of it, and
  takes its list of guards from the registry so a new `fail_closed` guard is
  covered the day it is registered. The opencode twins need nothing: their
  guards are in-process JavaScript with no child process and no interpreter
  resolved from `PATH`.
- **The opencode scan budget's documented cost is now measured rather than
  asserted (#168).** `docs/reference/hooks.md` gave it as "none measurable"
  while the paragraph under it quoted 9% for a *different* comparison, so the
  two read as a contradiction. Timing the handlers end to end cannot settle it —
  paired runs against the pre-#162 code swing between −3% and +12% on a 250 ms
  scan, which is GC and JIT variance — so the section now counts the clock reads
  instead, which is deterministic: 145 on an 800k-token command, 2,025 on a
  200k-line command, 13 on `ls -la`, at about 22 ns each. Under 0.05 ms on any
  input the guards accept — a floor rather than the whole cost, since what is
  counted is the clock reads and not the per-unit branch, the sampling counter
  or the deadline object each handler builds. The 9% figure is kept as what it always was, the cost
  of checking at *every* token rather than sampling, with the arithmetic that
  makes it plausible (800k reads ≈ 18 ms) and the measurement beside it.

## [0.9.3] - 2026-09-17

### Added

- **The vulnerability scan runs weekly, and can be started by hand (#155).**
  `ci.yml` gained a `schedule:` trigger (Mondays 06:27 UTC, off the hour where
  GitHub drops scheduled runs less often), so an advisory published while
  `main` is quiet no longer waits for the next push, pull request or tag. The
  schedule runs the whole `ci` workflow rather than a second copy of the
  `govulncheck` job, and a failure is a red `ci` run on `main`. It deliberately
  opens no issue: that would mean a write-scoped token on a workflow whose
  point is a read-only one, plus dedup so a standing advisory doesn't file one
  issue a week. `workflow_dispatch:` came with it, because a public repo's
  schedules are disabled after 60 days without activity and a schedule that
  never fires raises no alarm — the manual trigger is how you confirm the scan
  works again, or scan a new advisory without waiting for Monday.

### Changed

- **A tag can no longer publish from a commit whose tests fail (#155).**
  `ci.yml` gained a `workflow_call` trigger and `release.yml` now calls it,
  instead of carrying its own copy of the `govulncheck` job. A release run is
  `ci / test`, `ci / hooks`, `ci / govulncheck` and then `goreleaser`, which
  `needs:` the call — so every CI job must pass on the tagged commit before
  anything is published. `goreleaser` is still the only job with
  `contents: write`, the scan keeps its read-only token and
  `persist-credentials: false`, and the job definitions now exist in one place.

### Fixed

- **Docs no longer cite line numbers that had drifted (#155).** `setup.md`,
  `testing.md` and `release.md` each pointed at a changelog line that had since
  moved to an unrelated entry. Changelog references are now version headings,
  and the workflow and Go-test references that had drifted (or that this
  change would have moved) now name the job, step or symbol instead of a line.
- **A slow security guard no longer lets a tool call through unscanned (#162).**
  Claude Code does not block a tool call when a *command* hook times out — the
  call continues through the normal permission flow — and its default timeout
  for one is 600 seconds, so a guard that was slow on some input failed **open**
  after a long wait. No hook field expresses "block on timeout", so each
  `fail_closed` guard now enforces its own wall-clock scan budget and blocks
  when it is exceeded, in both twins.

  The budget is `DEVEXP_SCAN_BUDGET_MS`, 15 seconds by default and capped at 44,
  shared by `secret-guard`, `secret-in-write-guard` and `dangerous-cmd-guard`.
  It is sized from the worst cases the timing tests measure — around a second
  for a multi-megabyte input, under 0.1 s for ordinary input — with room for a
  much slower machine, so ordinary work never meets it. A larger value is
  clamped rather than honoured, with a notice: above the registered hook timeout
  Claude Code would cancel the guard first, so the one knob on offer could
  otherwise put the bug back. Only a plain ASCII non-negative integer counts, on
  both sides, so the twins read the same variable the same way.

  In Claude Code a shared `hooks/claude-code/scan-budget.sh` runs the guard
  under a watchdog that covers the whole hook, every interpreter and `grep`
  included, and exits 2 at the deadline; a run whose status is neither allow nor
  block, and a watchdog that cannot start at all, block as well. So does every
  way the prologue itself can fail, before any budget exists — a missing,
  unreadable or unparsable helper — which needs a trap rather than an `if`,
  because `set -e` abandons the script at the failing `.` and then reports 0,
  and 0 reads as *allow*. The marker that tells the budgeted run apart travels
  in argv, not the environment, so an ambient variable cannot switch the budget
  off — and a depth counter that *is* read from the environment, because all it
  can do is block, stops the guard if that marker ever stops being recognised
  rather than letting it fork without bound. It costs one extra `python3` and
  one extra `bash` per guarded tool call, roughly +60 to +85 ms on current
  hardware — about double a guard's run.

  In opencode, where a plugin hook has no timeout and runs in the server
  process, the deadline is checked as the scan runs — including inside
  `dangerous-cmd-guard`'s masking pass, its largest single piece of work — and
  the check refuses the call.

  Each of these guards is also registered with an explicit hook `timeout` of 45
  seconds — three times the default budget, and just above the cap — so the
  guard's own block always lands first. The installer writes that field and
  brings an existing registration to it, so a machine installed before this
  stops running on the 600-second default at its next install.
  `docs/reference/hooks.md` has the full behaviour.
- **`/release` creates the platform release the way the repo's release guide
  says (#154).** Phase 6 now reads the guide's **Cut** section first. When it
  gives the release-creation command, `/release` runs exactly that — in this
  repo a draft that goreleaser publishes after the build succeeds, so a failed
  or blocked build no longer leaves an empty release marked Latest. When the
  Cut section says nothing about creating a release, the generic commands are
  used, and they now verify the tag reached the remote (`--verify-tag`, or a
  `git ls-remote` check before `glab`, which has no such flag and would
  otherwise tag the default branch itself) and pass the version's changelog
  section as a notes file instead of an inline `--notes` string the shell can
  mangle. The generic path never adds `--draft` on its own: in a repo with no
  publisher step the draft would stay unpublished forever. A Cut section that
  mentions release creation without a runnable command stops the cut and sends
  the user to `/devxp`, rather than guessing — classified in the read-only
  preflight, so the stop happens before the tag is pushed and the gate shows
  the command it is about to run. The cut now also refuses to create anything
  unless the tag is actually on the remote (`git ls-remote … | grep -q .`,
  because `ls-remote` exits 0 either way) and unless the changelog section it
  extracted is non-empty, and every refusal in the phase now fires before the
  push. The tag name and message come from the guide's Cut format too, so the
  release commands carry a tag like `<target>@<version>` in a monorepo instead
  of a hardcoded `v<version>`. An unpublished release is reported as not yet
  shipped: the target that publishes it is bound watch-only, so its pipeline
  is watched and verified, and the delivery's worktree, plan and scratch are
  not retired until the release object is public. The failure path — check it
  is still unpublished, delete it, fix, cut the next patch, never move the
  tag — is spelled out for both platform CLIs. The `gen-docs` / `update-docs`
  Release Guide template has a matching `Release object:` line.

## [0.9.2] - 2026-09-17

### Added

- **`secret-in-write-guard` detects OpenAI's older service keys (#152).**
  Keys issued with the `sk-service-…` prefix can still be live, and public
  scanners (Trivy, TruffleHog) still detect them. Both twins now block them.
  The shape and length come from those scanners and are cited in the pattern
  comment. Look-alike names that start with `sk-service-` are still allowed.
- **CI scans the CLI for known vulnerabilities (#141).** A new `govulncheck`
  job in `ci.yml` runs on every pull request and push to `main`, and
  `release.yml` runs the same job before goreleaser, which now `needs:` it, so
  a failed scan publishes nothing. Both run `scripts/govulncheck.sh`, which
  holds the pinned govulncheck version (v1.8.0) and scans `cli/` with the Go
  toolchain from `cli/go.mod` once for every platform the release ships (each
  GOOS and GOARCH in `.goreleaser.yaml`, `CGO_ENABLED=0`). It exits 3 when the
  CLI calls vulnerable code, in the standard library or a module, on any
  platform. Findings the CLI doesn't call are printed but don't fail it, and
  any other failure, such as vuln.go.dev being unreachable, is reported as an
  infrastructure failure to re-run. Run it locally with
  `./scripts/govulncheck.sh`; `docs/development/testing.md` describes the
  policy and how to fix a finding.

### Changed

- **A GitHub Release is no longer public before its binaries are uploaded
  (#141).** `/release` now creates the release as a draft
  (`gh release create v<version> --draft …`), and `.goreleaser.yaml` sets
  `release.use_existing_draft: true`, so goreleaser uploads the assets into
  that draft, keeps its notes and then publishes it. Before, the release was
  Latest from the moment `/release` created it, and `remote-install.sh`
  failed for new installs until the upload finished, or for good if the
  workflow failed. If a release run fails now, delete the draft, fix, and cut
  the next patch; see `docs/guides/release.md`.
- **The release workflow's write token is limited to the goreleaser job.**
  The workflow default is now `contents: read`, and the scan job checks out
  without persisted credentials.

### Fixed

- **`dangerous-cmd-guard` could miss a delete of a protected directory
  (#151, security).** Both the Claude Code hook and the opencode module
  decided where a target ends from literal characters only.
  - A shell expansion right after the target didn't end it, although the
    expansion could leave the directory itself: it may be empty, split the
    word, or be a glob or brace expansion. Now any expansion right after a
    target ends it, as a quote or a redirect already did. So do the
    remaining redirection operators.
  - Some spellings of the home directory weren't recognised. Every rule that
    recognised `$HOME` or `~` now also recognises the other bash and zsh
    spellings, and a trailing `/` after any of them.
  - A path with a literal component after the protected directory is still
    allowed. To delete under a protected directory by variable, put a literal
    component first.
  - `rm` now has to be a word of its own, and the wildcard-delete rule needs
    the target in the same simple command as that `rm`. A `--rm` option, a
    longer word ending in `rm`, or a protected path after a real `;`, `&&`,
    `||` or `&` in a later command no longer blocks (for example a container
    run that removes itself and mounts `/tmp`). `rm` as an argument of another
    command, `rm` followed at once by an expansion or redirect, and a `;` or
    `&` that is quoted or inside a substitution still count. Upgrade to pick
    this up; rules:
    `docs/reference/hooks.md#what-dangerous-cmd-guard-matches`.
- **`dangerous-cmd-guard` could take a very long time on some crafted
  commands (#146).** Checks that could run slowly:
  - Both implementations re-checked the rest of a pipeline for every stage
    while deciding what text is inert, so a very long pipeline took time that
    grew with the square of its length. A 1 MB command took about 40 seconds
    in the Claude Code hook. That check now runs once per pipeline.
  - The opencode module's patterns backtracked on some crafted long lines:
    a few KB took seconds and longer lines minutes, delaying every guarded
    command. Each rule now decides one line in time linear in its length,
    a few hundred ms at most for a 1 MB line. Decisions are unchanged.
  - Nesting of subshells and substitutions deeper than 100 levels is now
    scanned whole in both implementations. Before, the Claude Code hook fell
    back at Python's recursion limit and the opencode module much deeper, so
    the two could decide differently.
- **`secret-in-write-guard` blocked long snake_case names that contain a
  GitHub token prefix (#143).** A name like `test_blocks_right_token_…`
  contains `ght_`, and the GitHub pattern accepted `_` in the token body, so a
  long enough name was refused. Classic GitHub tokens (`ghp_`, `gho_`, `ghu_`,
  `ghs_`, `ghr_`) now match only by their documented alphanumeric shape. GitHub
  App installation tokens (`ghs_`, including Actions' `GITHUB_TOKEN`) have been
  moving to a longer stateless format since 2026-04-27, and that format contains
  `_`. It gets its own match, so those tokens are still blocked; a rare name
  in which a prefix is followed by a segment that starts like that token's
  JWT is blocked too. Both twins change together.
- **`secret-in-write-guard` blocked placeholders shaped like keys (#143).**
  Documentation and templates use values such as a Slack prefix followed by
  `your-token`, AWS's published example access key ID, or a body of repeated
  `x`, and the guard refused them. A value that is entirely a placeholder is
  now allowed. A real key next to a placeholder, or joined to one, still
  blocks. Mirrored tests cover both.
- **`secret-in-write-guard` blocked a private-key header quoted on its own
  (#143).** Any `-----BEGIN … PRIVATE KEY` line was refused, including one
  named in documentation or matched in code. A header now blocks only when key
  material follows it closely, before any `END` or `BEGIN` line. Text further
  on in the same write doesn't count, so an identifier, fingerprint, path or
  hash later in the file no longer blocks, but key-like text on the header's
  line or just after it still does. Real PEM blocks still block, including
  escaped, concatenated, encrypted and PGP-armored ones and ones with a dash
  rule under the header. Other backslashes near a quoted header, as in Windows
  paths, escaped code strings or LaTeX, don't count as key material. A body
  wrapped far narrower than the usual PEM line width, written in pieces across
  several edits, or starting far from its header, isn't recognized as key
  material; `docs/reference/hooks.md` lists this with the guard's other
  limits. The test fixtures now use a realistic PEM body.
- **`secret-in-write-guard` decides in linear time (#143).** Each pattern is
  retried at every position in the text, and some of the new patterns could
  rescan the rest of a long write from each retry. A write that repeated a
  prefix or a header could keep the guard busy for minutes, and a Claude Code
  hook that runs past its timeout lets the write through. Every repetition
  that can reach past the next start is now bounded, and timing tests in both
  suites pin it. Decisions are unchanged except for rare edge shapes that go
  beyond the new bounds.
- **Saving `settings.json`, the manifests and the opencode plugin files could
  leave a truncated file, and a plain atomic rename would have replaced a
  symlinked file with a regular one (#124).** `devexp install` wrote
  `~/.claude/settings.json` and both `.devexp-manifest.json` files by
  truncating and rewriting them in place. All of them, and the opencode plugin
  files and legacy `config.json` edit, now go through one writer
  (`cli/internal/fsutil`):
  - the new bytes go to a temp file next to the file being replaced, are
    fsynced, get that file's permission bits (not a fixed 0644) and are
    renamed over it, so an interrupted save leaves the old file or the new
    one. A new file still gets 0644 minus the umask (0600 under `umask 077`),
    as before. Replacing breaks a hard link and drops xattrs, ACLs and
    setuid/setgid/sticky bits;
  - a symlinked file (dotfiles) is followed to the file it finally points at,
    which is replaced; the link, and every link in a chain, stays;
  - a dangling link, a link loop, a directory, a file you can't write, a
    directory where no temp file can be created, or a rename the file system
    refuses (a bind-mounted file) is refused with an error naming the file,
    which is left untouched. Earlier releases created the file a dangling
    link pointed at.
- **`devexp install` replaced an opencode `config.json` it couldn't parse
  (#124).** The MCP merge ignored decode errors, so a `config.json` with a
  syntax error or comments was overwritten with a file holding only the MCP
  servers, and one holding `null` crashed the install. A `config.json` that
  isn't strict JSON, or whose top level or `mcp` value isn't an object, is now
  left untouched: the MCP step is skipped with a warning naming the file and
  the servers to add by hand, and agents, skills and hooks still install (with
  `--mcps-only` it is an error). opencode reads `config.json` as JSONC, so a
  commented one is valid there. Numbers are
  written back as they were instead of through a float (`12345678901234567890`
  no longer becomes `12345678901234567000`). The save is atomic and keeps a
  symlinked `config.json`'s link.
- **`devexp install` overwrote the file behind a symlinked agent, command or
  skill (#124).** An installed agent file (`~/.claude/agents/<name>.md`,
  opencode `agents/`), opencode command, skill directory, or file or directory
  inside a skill that was a symlink had its target overwritten, which could be
  a customised copy in a dotfiles repo or the toolkit's own source file.
  **Behaviour change:** such an entry is now left untouched, with a warning
  naming it, in a dry run too. Its name stays in the manifest. Replace the link
  with a regular file to get the release's copy. Agent, command and skill
  files are also written atomically now.
- **Hook registrations from another install root were never pruned once their
  script left the registry, and Claude Code ignored `claude_code.enabled`
  (#150).**
  - `devexp install` now removes a registration in a form devexp writes (a
    plain or single-quoted absolute path) of a script that no longer exists
    directly under `<root>/hooks/claude-code/`, when `<root>` is another devexp
    install root. Such a root is recognised by its own `hooks/registry.json`
    (a non-empty JSON array of hooks, each with a `name`). A directory without
    one, including a root that has been deleted, is treated as the user's and
    left alone, as are commands with arguments, double quotes, wrappers,
    other directories or `args`.
  - Claude Code hook registration uses `EnabledFor(claude_code)`, as opencode
    already did: a hook's `claude_code.enabled` overrides the top-level
    `enabled` either way.
- **`uninstall.sh`: crashes on deeply nested JSON, writes that weren't atomic,
  and a reformatted `config.json` (#124, #150).**
  - Both python steps skip a `settings.json` or `config.json` nested more
    than 500 levels deep, with a message and the file untouched, whatever
    python runs them. Python's `json` raised `RecursionError` (not a
    `ValueError`) near 1,000 levels on 3.9-3.11 but not on 3.13+, and the
    uninstall used to stop there under `set -e`. `devexp install` (Go) reads
    up to 10,000 levels, which python can't reliably match, so a file nested
    501-10,000 deep is edited by install but skipped, untouched, by uninstall. Any other failure of either step now prints
    a warning and the uninstall carries on.
  - The `settings.json` step saves atomically with the same rules as
    `devexp install` (a symlinked `settings.json` keeps its link, the file it
    points at is replaced; dangling links, unwritable files and directories
    are refused with a warning). It used to truncate and rewrite the file in
    place.
  - The opencode MCP step cuts only the removed servers' members out of
    `config.json`, so key order, indentation, CRLF line endings, escapes,
    numbers and the other servers stay byte for byte. It used to rewrite the
    whole file as 2-space JSON. A symlinked `config.json` is still left
    untouched with a warning.
  - The `settings.json` step also removes the orphaned registrations from any
    devexp install root that `devexp install` now prunes (#150).
- **Re-installing removed stale agents and skills through a symlinked target
  directory (#128).** When `~/.claude/agents`, `~/.claude/skills`,
  `~/.config/opencode/agents` or `~/.config/opencode/commands` was a symlink
  (for example into a dotfiles repo or a source checkout), `devexp install`
  removed an agent or skill it no longer installs from the tree the link points
  at, and a skill directory with everything in it.
  - The same happened when the directory was behind a symlink: under a linked
    `~/.claude`, `~/.config` or `~/.config/opencode`. That now counts too, for
    opencode `plugins/` as well. A symlink above `HOME`, such as macOS `/var`,
    doesn't.
  - devexp still installs through such a link, but no longer removes anything
    through it, the rule opencode `plugins/` already follows (#108). The output
    lists what was left: `"<dir>" is a symlink — devexp never removes files
    through it; remove these by hand: …`, or `is behind a symlink (it resolves
    to …)`.
  - Those entries stay in the manifest, once each, so a run after the link is
    replaced with a real directory removes them. So does an entry that couldn't
    be checked or removed (for example, permission denied): earlier releases
    dropped it from the manifest and never tried again.
  - An entry is now removed only under the exact name devexp recorded, as the
    directory lists it. On a case-insensitive filesystem such as default APFS,
    a recorded `retired-agent.md` used to remove a user's own
    `Retired-Agent.md`; a spelling that differs in case or Unicode
    normalization is now left alone, with a warning, and dropped from the
    manifest.
  - Removals go through a handle on the directory that was checked
    (`os.Root`), so swapping the directory for a symlink mid-run can't redirect
    them.
  - Real directories are cleaned up as before.
  - `uninstall.sh` follows the same rules. It removed agents and skills through
    an `agents/` or `skills/` directory that was a symlink or behind one, and
    removed an entry that was itself a symlink. It now leaves both in place and
    lists them before the confirmation, checks each entry again after the
    confirmation, no longer says skills are kept for another CLI when they are
    kept because of a symlink, and no longer counts a failed `rm` in
    `Removed N item(s)`.

## [0.9.1] - 2026-09-16

### Fixed

- **Rewriting `settings.json` dropped fields from users' hooks (#137).** devexp
  read hooks into types holding only `matcher`, `type` and `command`, so when
  `devexp install` rewrote `~/.claude/settings.json` every other field of a
  user's hook was lost: `timeout`, `async`, `shell`, `if`, `statusMessage`,
  `args` (turning an exec-form hook into a shell command), an `http`,
  `mcp_tool` or `prompt` hook's own fields, and anything a later Claude Code
  adds. The whole file was also re-sorted and re-indented, with `&`, `<` and `>`
  written as `\u` escapes.
  - Install now edits only devexp's own handlers. Every other handler and entry
    keeps all its fields, in order; a user's entry without `matcher` doesn't
    gain one; a re-quoted devexp command changes that value only.
  - Only the top-level `hooks` value is rewritten, in the indentation and line
    ending (LF or CRLF) of the file around it. Every byte outside it stays as
    it was. Inside it, fields, their order and values are kept, but the value
    is re-indented as a whole and escaped keys are re-encoded. New files and devexp's own entries are
    written as before. A number no float64 holds (such as `1e400`) is kept as
    written and doesn't stop the install.
  - A handler with `args` (spawned without a shell) or of a type other than
    `command` is the user's whatever path it names: never re-quoted, pruned,
    removed or counted as devexp's registration.
  - An entry or event that was already empty stays; one is removed only when
    removing devexp's handlers emptied it.
  - A `settings.json` that isn't valid JSON, or whose `hooks`, events, entries
    or handlers have an unexpected shape, is left untouched and the install
    stops with an error naming it. Earlier releases replaced it with a file
    holding only the hooks.
  - `uninstall.sh` applies the same rules: it keeps pre-existing empty events,
    no longer escapes non-ASCII text, splices the new `hooks` value into the
    original text in the file's own line ending (CRLF files stay CRLF), and
    skips entries and handlers that aren't objects instead of failing with a
    traceback. A `settings.json` holding `NaN`, `Infinity` or a number a float
    can't hold (such as `1e400`, which python would write back as `Infinity`)
    is left untouched with a message.
- **Existing installs never picked up a registry matcher change.** `devexp
  install` skipped a hook whose command was already registered, whatever
  matcher its entry had, so a hook whose registry matcher grew (as when
  `secret-guard` went from `Read` to `Read|Bash`) kept firing only for the old
  tools.
  - An enabled hook registered under another matcher is brought to the
    registry's. An entry holding only that hook takes the new matcher in place,
    keeping its other fields (a `matcher` key it lacked goes before `hooks`).
    From an entry shared with other commands the handler moves into an entry
    of its own, leaving the other commands and their matcher untouched.
  - **Behaviour change:** install now enforces the registry's matcher on
    devexp's own handlers, so a matcher you set by hand on one, or an entry
    with no `matcher` (matching every tool), is reset on the next install. To
    use a matcher of your own, disable the hook (`hooks.disabled` in
    `devexp.config.json`), remove devexp's entry for it, and register your own
    command for it, such as a wrapper script or the script with `"args": []`.
  - A hook already under the registry's matcher, or disabled, is left as it
    is, and a second install writes nothing. `uninstall.sh` is unchanged.
- **Stale hook pruning removed users' commands under the repo dir (#138).**
  `devexp install` treated any registered command whose string pointed under
  the devexp repo/cache dir and wasn't an existing file as a stale devexp
  hook, so a user's own hook such as `<dir>/hooks/x.sh --flag` was removed.
  - Only devexp's own form is pruned now: the plain or single-quoted path of a
    script directly in `<dir>/hooks/claude-code/` (or the bare path an earlier
    install from that dir wrote), when the script no longer exists.
  - A command with arguments, in another directory, double-quoted, chained or
    using a variable is the user's and is never pruned, whether or not what it
    names exists.
- **`dangerous-cmd-guard` no longer blocks a mention of a destructive command
  (#100).** It matched its patterns anywhere in the command line, so an `echo`
  printing a recovery hint, a commit message, or an issue body that named one
  was refused. That included the report of this bug. Before matching, both the
  Claude Code hook and the opencode module now blank out text that can't run:
  `echo`/`printf` arguments, `git commit|tag` messages, `gh issue|pr|release`
  bodies and titles, heredoc bodies fed to a text filter, and comments. They
  do this only when the output can't reach a shell (no redirect into a file,
  no pipe into anything but a text filter). Everything else is still scanned,
  including `bash -c`, `sh -c`, `eval`, `ssh`, `psql -c` strings, `$(…)`,
  heredocs fed to a shell, and commands after `&&`, `;`, `|`, `sudo` or `env`.
  If the guard can't classify part of a command, it scans the whole command as
  before. That covers unbalanced quotes, `case`, functions, aliases, pipes or
  redirects on compound commands, a shell or interpreter anywhere in the
  command, and a command word that is a path or a variable. Rules:
  `docs/reference/hooks.md#what-dangerous-cmd-guard-matches`.
- **`dangerous-cmd-guard` could miss a destructive command (security).** Some
  large commands were read as "no match", and some ways of writing a command
  (inside a quoted string passed to another shell or evaluator, or split
  across lines) were not matched. Both the Claude Code hook and the opencode
  module now match these forms, and a matching error blocks instead of
  allowing. Upgrade to pick this up; rules:
  `docs/reference/hooks.md#what-dangerous-cmd-guard-matches`.
- **The opencode `dangerous-cmd-guard` blocked some multi-line commands that
  the Claude Code hook allowed.** Its patterns could match across line
  breaks, while the Claude Code hook matches one line at a time. The opencode
  module now matches one line at a time too. Continued lines are still joined
  first, so a continued command is still checked as one line.
- **`secret-in-write-guard` let secrets through without saying anything (#101).**
  The guard had no behavior test, and the one added here found ways a
  secret reached disk while the guard exited 0:
  - Claude Code: some kinds of secret, and large writes, were never checked.
    The way the shell guard ran its pattern match read some failures as
    "no match", so the write went through. Matching now happens inside the
    guard's Python step, which already fails closed. Patterns, labels and
    messages are unchanged.
  - opencode: edits were never scanned. The guard read `new_string`, but
    opencode's `edit` tool passes `newString`, so only `write` was checked.
  - New mirrored tests: `hooks/claude-code/secret-in-write-guard.test.sh` and
    `hooks/opencode/secret-in-write-guard.test.js`. They check that every
    pattern is blocked in Write/Edit content, in code, on a later line, in a
    `.env.example` and in a 260 KB write, and that the block message names
    the kind of secret without repeating it. They check that prose mentioning
    tokens, variables named `token`, `.example`/`.sample`/`.template`/`.dist`
    templates with placeholder values, public keys and certificates, and an
    Edit that removes a key are allowed silently.
- **`secret-in-write-guard` detects current OpenAI and GitHub token formats
  (#101).** It named both vendors but matched only their older formats. It now
  also blocks OpenAI project, service-account and admin keys (`sk-proj-…`,
  `sk-svcacct-…`, `sk-admin-…`), whose bodies contain `_` and `-`. A key is
  found wherever it sits, including right after an escape sequence, a
  percent-encoded character or a joined name, and only a body as long as a real
  key's matches, so kebab-case names like `desk-admin-…` don't. Legacy user keys
  issued as `sk-None-…`, which can still be live, are blocked too. It also
  blocks GitHub user-to-server (`ghu_`), refresh (`ghr_`) and fine-grained
  (`github_pat_`) tokens, the last by their exact shape, so long snake_case
  names that start with `github_pat_` don't match. Both twins change together,
  and nothing that blocked before is allowed now.
- **`secret-in-write-guard` scans every tool that writes file content (#101).**
  - opencode: `apply_patch` wasn't scanned at all. opencode offers GPT models
    `apply_patch` instead of `write` and `edit`, so with those models nothing
    was checked. The guard now scans the lines a patch adds (`+` lines, which
    covers new-file bodies and added lines in update hunks). Removed lines,
    unchanged context and `***`/`@@` headers aren't scanned, so a patch that
    deletes a key is allowed.
  - Claude Code: the matcher `Write|Edit` matches tool names exactly, so the
    guard never ran for `NotebookEdit` (`new_source`) or for `MultiEdit`
    (`edits[].new_string`, in older releases). The registry matcher is now
    `Write|Edit|MultiEdit|NotebookEdit` and the guard scans those fields. Text
    being replaced (`old_string`) is still not scanned. Re-run `devexp install`
    to apply the new matcher.
  - Both: AWS temporary access key IDs (`ASIA…`) are blocked too, including
    right after an escape sequence or a percent-encoded character. The match
    is bounded by characters that can't be part of an ID, so words like
    `EURASIA…` and `ASIAPACIFICDATACENTER01` don't match.
  - Known limitation, now documented in both guards and
    `docs/reference/hooks.md`: only the new text of a write is scanned, not
    the file text around it, so a secret completed across existing file text
    and an edit isn't seen.

### Security

- **Pinned the Go toolchain to a patched release (#104).** Building `devexp`
  with Go 1.26.0–1.26.3 left a standard-library advisory (GO-2026-5037) in code
  the CLI calls. `cli/go.mod` now pins `toolchain go1.26.8`. CI and the release
  workflow read the Go version from that file, and local builds with an older
  Go switch up to it. If `./install.sh` can't build the CLI, it now also
  explains that the first build may need network to download that toolchain,
  and what to do offline. Released binaries up to
  v0.9.0 were built with Go 1.25.14, which already includes the fix, but Go 1.25
  no longer receives security updates.
- **Updated `golang.org/x/text` to v0.39.0 (#104)** to pick up the fix for
  advisory GO-2026-5970. The CLI doesn't call the affected code, but the old
  version was compiled into the release binaries.

## [0.9.0] - 2026-09-16

### Added

- **`devexp:preserve` and `devexp:inherit` blocks in CLAUDE.md.** A section
  pointing at rules owned outside the repo (for example a rulebook shared by a
  family of repos) can't be verified against in-repo docs, so `update-indexer`
  could classify it as leaked knowledge and move or drop it, and `gen-indexer`
  never wrote it for a new repo.
  - `<!-- devexp:preserve id="…" -->` … `<!-- /devexp:preserve -->` marks content
    `update-indexer` never edits, reorders, moves or removes, and that doesn't
    count as leakage or against the 150-line budget. Repo-relative paths inside
    it are checked and reported, never fixed. `gen-indexer` carries every
    preserve block over verbatim, in the same relative position, when
    regenerating.
  - `<!-- devexp:inherit id="…" remote="<regex>" -->` in any ancestor
    directory's `CLAUDE.md` (up to `$HOME`) is inserted as a preserve block with
    the same `id` into every repo whose `origin` matches `remote`: by
    `gen-indexer` on generation, and by `update-indexer` when the repo lacks
    it. An existing preserve block with that `id` wins. `{{repo_to_parent}}` is
    replaced with the relative path to the parent directory.
  - Both indexers report kept, added and skipped blocks and unresolved paths.
    `/devxp` measures leakage outside the blocks and refreshes a CLAUDE.md
    that's missing a matching inherit block. `docs-sync` never edits inside a
    preserve block. Documented in `docs/guides/docs-architecture.md`.

### Fixed

- **Hook commands run from paths with spaces or shell syntax (#135).** Claude
  Code runs a hook command through `sh -c`, and devexp registered the absolute
  script path unquoted. From a repo or asset-cache dir with a space, quote, `$`,
  `;`, `&` or other shell syntax in its path, the shell split or expanded the
  command, so the hook didn't run (a non-blocking error, meaning the guard
  failed open) or a different program ran.
  - Such a path is now registered as one POSIX single-quoted word (`'…'`, each
    `'` written as `'\''`). Every other path is registered as before, byte for
    byte.
  - On the next `devexp install`, a registration of one of the same repo/cache
    dir's scripts is rewritten in place to the form devexp writes (quoted only
    when the path needs it), disabled hooks included. This covers the bare
    path an earlier install wrote, the path in double quotes (the natural hand
    fix, recognised only when the path has no `$`, backquote, `\` or `"`), and
    a single-quoted path that needs no quoting, which becomes plain.
  - When the event already holds the registered command, for example after an
    older devexp re-added the bare path, that other spelling is removed rather
    than rewritten into a duplicate. An entry left with no commands goes too,
    so the script never runs twice.
  - Install-time pruning (stale, foreign-root, relative) and `uninstall.sh`
    recognise exactly the two forms devexp writes: a plain path, or one
    single-quoted absolute path that re-quotes to itself.
    `uninstall.sh` also removes its own repo's bare and double-quoted
    spellings. Any other double quotes, arguments, chained or concatenated
    words and other quoting are still the user's and are never touched.
  - Not migrated: an unquoted entry from a different root, which can't be told
    apart from a user command with arguments. An install or uninstall from its
    own root fixes it.
- **`devexp install` refuses an unset, empty or relative `HOME` (#126).** Every
  install target is built from `HOME`, so with it unset, empty or relative the
  installer would write into, back up from and remove stale files under the
  current directory — a dotfiles checkout, say. A standalone binary would also
  wipe and re-extract its assets there, since the user cache dir follows `HOME`.
  - `devexp install` now stops with a non-zero exit and
    `HOME is "…", not an absolute path — refusing to install anything` before
    anything else: no asset extraction, no wizard, no MCP registration, no
    writes or backups. This covers every flag path, the wizard (including its
    Remove action) and both targets.
  - The check #109 added for `devexp uninstall` is now one shared helper
    (`targetHome` in `cli/cmd/paths.go`), and `claudeTargetPaths` /
    `opencodeTargetPaths` go through it, so they can no longer return a relative
    target path. `devexp uninstall` behaves and reports as before.
  - `uninstall.sh` refuses the same HOME values (exit 1,
    `refusing to remove anything`) before it looks at anything. Before, an
    empty HOME pointed it at `/.claude/…` and a relative one at the current
    directory.
  - `scripts/remote-install.sh` refuses, before downloading, when
    `DEVEXP_INSTALL_DIR` is unset and HOME is unset, empty or relative (the
    default `~/.local/bin` would have been `/.local/bin` or a path under the
    current directory), and when `DEVEXP_INSTALL_DIR` is relative. An absolute
    `DEVEXP_INSTALL_DIR` still works without HOME.
  - `install.sh` refuses the same HOME values before it builds `bin/devexp`.
    In a fresh clone the build ran first and put Go's caches under the clone.
- **A standalone binary no longer extracts its assets to a temp directory
  (#126).** With no usable user cache dir (the lookup failed, or it was relative,
  e.g. a relative `XDG_CACHE_HOME` on Linux), `devexp` fell back to
  `os.TempDir()`. That was either a relative `$TMPDIR` under the current
  directory, wiped and re-extracted, or the shared `/tmp/devexp/assets`, which
  is not private to the user but was reused whenever its version marker
  matched. It now refuses with a message asking for an absolute `HOME` (or
  `XDG_CACHE_HOME`).
- **Hook commands are always absolute (#126).** A relative `DEVEXP_DIR` produced
  relative hook command paths in `~/.claude/settings.json`, which resolve
  against whatever directory Claude Code runs in rather than the devexp repo.
  - `DEVEXP_DIR` is now resolved to an absolute path and must be a devexp repo
    (`agents/`, `skills/`, `mcps/`); otherwise `devexp install` stops with an
    error instead of falling back to another lookup. Every resolved asset dir
    is checked to be absolute.
  - `hooks.InstallClaude` refuses a non-absolute repo dir.
  - Relative devexp hook entries left by an earlier install are removed on the
    next install and replaced by the absolute registration; relative hooks that
    aren't devexp's are left alone. `uninstall.sh` already removed them; it now
    has tests for it.
  - A hook command counts as devexp's only if it is a plain path (no
    whitespace, variables, `~`, quotes or other shell syntax) ending in
    `hooks/claude-code/<registry script>`, with `hooks/claude-code/` at a path
    segment boundary. This applies to install-time pruning and to
    `uninstall.sh`. Before, commands such as `$CLAUDE_PROJECT_DIR/hooks/claude-code/…`
    or `…/my-hooks/claude-code/…` could be removed.
- **Stale agent and skill removal no longer trusts manifest names (#117).**
  `devexp install` joined each stale entry of the previous manifest onto the
  agents/skills directory unchecked, so a corrupted or hand-edited
  `.devexp-manifest.json` could delete files outside it. Claude Code skills are
  removed recursively: an entry of `""`, `.` or `..` removed the whole
  `~/.claude/skills` or `~/.claude` directory.
  - An entry is removed only when it is a bare name devexp installs: no `/` or
    `\`, no `..`, no control characters, not empty or `.`, and ending in `.md` for agent files (opencode
    commands are recorded as `<name>` for a `<name>.md` file). Any other entry is
    kept and a warning names it exactly as the manifest records it. This covers
    both Claude Code and opencode, in real runs and `--dry-run`.
  - A valid entry is removed only when it is still what devexp installs: a
    regular file for agents and commands, a real directory for Claude Code
    skills. A symlink is never removed (as for opencode plugin files since
    #108); it is kept with a warning. So is an entry that can't be checked
    (for example, permission denied).
  - A stale entry is never removed when it names something this run installed:
    the same name apart from case (a case-only rename between releases), or
    the same file on disk. On a case-insensitive filesystem (the macOS default)
    `DEV-AGENT.md` in the old manifest used to delete the `dev-agent.md` just
    installed. It is kept with a warning. A variant that differs only in
    Unicode normalization is caught once the install is on disk, so
    `--dry-run` may still preview removing it.
  - Names and paths in these warnings, previews and removal lines are printed
    quoted, so a manifest entry can't write control sequences to the terminal.
    An entry that is already gone is no longer reported as removed.
  - Stale removal of valid entries is otherwise unchanged. The manifest format
    is unchanged.
- **On-save hooks: every tool now gets the edited file, as a path, and
  test-on-save works with Jest 30 (#121).**
  - A relative path that starts with `-` was passed on as given, in both
    Claude Code and opencode. `ruff`, `black`, `flake8`, `prettier`, `eslint`,
    `gofmt` and `rubocop` read it as options, and failed or did the wrong thing.
  - The tools run from the project root, not from the directory a relative
    path is relative to. With a nested project, or a hook cwd that isn't the
    session directory, they were pointed at a file that doesn't exist.
  - The hooks now resolve a relative path first: against the input's `cwd` in
    Claude Code, or `ctx.directory` in opencode (the directory opencode's own
    edit tool resolves against), else the process cwd. `.` and `..` segments
    are normalised the same way in both CLIs, so both give a tool the same
    argv. Every tool gets the absolute path, which is never read as an option
    or as a ruff `@argfile`.
    Absolute paths, which both CLIs send, still reach the tools byte for byte.
  - test-on-save passed jest `--testPathPattern <path>`. On Jest 29 a path
    starting with `-` ran the wrong tests. Jest 30, which renamed the option to
    `--testPathPatterns`, rejected it on every run. The pattern was also a
    regex, so a test file named `a+b.test.js` ran `ab.test.js`, one named
    `c(1).test.js` ran nothing and still reported PASS, and `mod.test.js`
    also ran `sub/mod.test.js` and `mod.test.jsx`. The hook now runs
    `jest --passWithNoTests --no-coverage --runTestsByPath -- <path>`, with
    the path relative to the project root. On Jest 29.7 and 30.5 that runs
    exactly the one test file, including names with regex characters or a
    leading `-`.
  - vitest keeps its absolute path with no `--`, because vitest drops file
    filters after `--`.
  - `hooks/claude-code/on-save-path.test.sh` and the new
    `hooks/opencode/on-save-path.test.js` pin every tool call's argv and
    working directory. The cases cover absolute paths, leading-dash relative
    paths, `./` and `..` segments, nested projects, the input `cwd` /
    `ctx.directory`, and jest test files with regex characters in their names.
- **Onboarding agent: its example test command works on Jest 30.** It showed
  `npm test -- --testPathPattern=orders`, which Jest 30 rejects, and now
  shows `npm test -- src/orders`.
- **Hook authoring guide: opencode modules spawn through `runCommand`.** The
  "tool input as data" section told opencode authors to use
  `execFileSync`/`spawnSync`, which the same guide forbids further down. It now
  points to the async `runCommand` helper, which takes an argument array and
  uses no shell.

### Security

- **`devexp install` no longer uses a directory it finds on disk (#134).** Repo
  detection accepted non-toolkit directories, and the choice depended on where
  the binary was run from; `devexp install` only named the directory it used
  when it fell back to the bundled assets.
  - Release builds use only `DEVEXP_DIR` or the assets bundled in the binary.
  - Dev builds (`./install.sh`'s `bin/devexp`, `go run`, `go test`) use only
    `DEVEXP_DIR`, the checkout they were compiled from, or their bundled assets.
    A copied or linked dev binary still uses its own checkout. A `-trimpath`
    build has no recorded checkout and uses its bundled assets.
  - A dev build uses the checkout it was compiled from only while that checkout
    can be verified as the user's (Unix). The checkout, its root files and
    every entry under `agents/`, `skills/`, `mcps/` and `hooks/` must be owned
    by the user, must not be symlinks and must not be writable by others; group
    write is accepted only for the user's private group (the primary group
    named like the user, as with a umask `002` default). The directory holding
    the checkout must be owned by the user or root and follow the same write
    rule unless it is sticky. Otherwise install warns that the checkout can't
    be verified as yours, names what failed, and uses the bundled assets; fix
    that, or set `DEVEXP_DIR`, to install from it.
  - Neither looks next to the binary or in the current directory or its
    parents any more.
  - A devexp-toolkit checkout is recognised by the committed `.devexp-toolkit`
    marker file (first line `devexp-toolkit`) plus `agents/`, `skills/` and
    `mcps/`. The marker tells a checkout apart from other directories; which
    directory may be used is decided by the rules above. `DEVEXP_DIR` must pass
    the same check. The marker is embedded, so the extracted bundled assets
    qualify too. Forks must keep the file.
  - `devexp install` prints `Asset root: <dir> (<how it was chosen>)` before it
    extracts or installs anything. When a dev build skips its own checkout,
    because the checkout lacks the marker, no longer exists or can't be verified
    as the user's, it warns and says how to fix it.
  - Dev builds re-extract their bundled assets on every run instead of reusing
    a cached copy, since every dev build has the same version, and they extract
    to their own `devexp/assets-dev` directory, so a dev run never touches the
    `devexp/assets` directory a release install registered hooks from.
  - Extraction is atomic: assets are extracted into a new directory, which
    replaces the previous one in a single rename. An interrupted or failed
    extraction leaves the previous assets in place, and concurrent runs always
    leave a complete copy. The previous copy is kept for an hour so hooks
    started just before the switch keep working, then removed by a later run.
  - Release binaries are built with `-trimpath`.
  - **Clone users:** run `rm bin/devexp && ./install.sh`. `install.sh` never
    rebuilds an existing `bin/devexp`, and a binary built before this change
    keeps the old lookup until it is rebuilt.

## [0.8.0] - 2026-09-16

### Added

- **Release targets and the per-repo release guide.** `/release` used to end at
  tag + GitHub/GitLab release — a full release for a library, but not for a
  service (deploy), a web app (hosting), or an iOS/Android app (beta channel →
  store review → staged rollout). Each repo now declares how it ships in
  `docs/guides/release.md`, and the lifecycle reads it end to end:
  - `/devxp` detects release targets by generic file shapes and writes or
    refreshes the guide through `gen-docs`/`update-docs` (new **Release Guide**
    template). Unproven fields are `[CONFIRM]` markers, never guesses.
  - `grooming-agent` records **Affected Release Targets** and release impact in
    the plan; `/refine` shows them and raises the estimate for externally-gated
    or multi-target releases.
  - `/deliver` gains **Phase 4.5 — Release Readiness** per affected target.
  - `/release` splits into **cut** (merge, changelog, version + build numbers,
    tag) and **ship** (per target, from the guide): a gate per target showing
    the rollback plan first, a separate confirmation for every production-facing
    promote stage, verification against the target's declared signals, rollback
    offered but never automatic. New target states `awaiting-external`,
    `blocked`, `skipped`, persisted to `~/.claude/agent-memory/release/<ticket>.md`
    so `/release <ticket>` resumes store-gated and staged releases. No guide →
    cut-only, as before.
  - `/monitor` reviews each target's declared post-release signals.
  - `/cleanup` and `/improve` treat a ticket with a pending target as live.
  - New maintainer guide: `docs/guides/release-targets.md`.

- **Development Kit + CLAUDE.md as a strict index.** `/devxp` wrote CLAUDE.md
  *before* docs/, and `gen-indexer`'s template inlined dev commands, conventions,
  testing, env vars and playbooks whenever a doc was missing — so new repos got
  the fat CLAUDE.md the docs-architecture guide warns against.
  - `gen-docs`/`update-docs` define a required **Development Kit** —
    `development/setup.md`, `development/conventions.md`, `development/testing.md`,
    `architecture/overview.md`, `guides/workflows.md`, `guides/release.md` — with
    templates, evidence rules, visible gap markers and `ready`/`draft` status.
  - `/devxp` now builds atlas → kit → CLAUDE.md, judges each kit doc
    individually, and lists every doc with its status and open markers in the
    plan and report.
  - `gen-indexer` rewritten to produce only an index: what the project is,
    Start Here, Rules, Gotchas, ≤6 commands, Where Things Are. Hard limits —
    ≤150 lines, no code blocks, every pointer verified. Missing docs become
    `[NOT FOUND]` pointers, never inlined content.
  - `update-indexer` detects leaked knowledge and moves it into the owning kit
    doc (re-verified against code) before replacing it with a pointer.

### Fixed

- **opencode: `devexp install` now installs the hook plugin (#106, #107, #108).**
  The Go CLI never deployed the opencode hooks, so opencode users got no guards
  while the installer reported success.
  - It installs `~/.config/opencode/plugins/devexp.js` (the single entry opencode
    loads) plus `devexp/` with only the selected modules, `utils.js`,
    `package.json` and `hooks.json`. Files are copied by the registry list, so
    `*.test.js` never ships, and `plugins/package.json` is never written.
  - It honours `hooks.disabled` and the wizard selection, the same way the Claude
    Code path does. A hook disabled or removed since the last install is deleted
    on re-install, tracked through the new `plugins` key in
    `~/.config/opencode/.devexp-manifest.json`. With every hook disabled it
    installs no plugin and says why. A lost or unreadable manifest doesn't stop
    that: devexp also recognises its own plugin files on disk.
  - It turns on lint/format/test-on-save for opencode. The `graphify-*` hooks are
    on for opencode (turn them off with `hooks.disabled`).
  - It cleans up the pre-v0.1.0 flat install, but only after the new plugin is in
    place, so a refused install keeps the old one. A file is removed only when its name
    is in the legacy set and its content carries the devexp header; a same-named
    user file is kept with a warning. The legacy `config.json` `plugin` entry is
    removed only on an exact match, and the rest of `config.json` keeps its bytes;
    a symlinked or read-only `config.json` is left untouched with a warning.
  - Nothing is written or removed until every check has passed:
    - It refuses a `plugins/devexp/` that is a symlink (it may point at a
      source checkout), and a `plugins/` link that is dangling or doesn't
      point at a directory.
    - A `plugins/` symlinked to a directory (for example from dotfiles) is
      written through but never removed through. Stale plugin files, the
      every-hook-disabled uninstall and legacy flat files are left in place.
      The output lists them to remove by hand, and they stay recorded in the
      manifest so a run after the link is replaced can clean them up.
    - It never overwrites a `devexp.js` that is neither a devexp entry nor
      recorded in the manifest. A recorded, damaged one is repaired.
  - It never deletes anything outside `devexp.js` and a real `devexp/`
    directory, and never deletes through a symlink. Files are replaced atomically (temp file + rename). Removing
    the plugin is all-or-nothing: if `devexp.js` has to stay (a symlink, it
    can't be deleted, or it isn't recognised as devexp's — neither recorded nor
    carrying the devexp header, for example after an editor added
    `// @ts-check`), `devexp/` stays too and the output says why.
  - `--dry-run` lists every file and writes nothing.
  - **Clone users:** run `rm bin/devexp && ./install.sh`. `install.sh` never
    rebuilds an existing binary.

- **opencode: `uninstall.sh` no longer aborts and now removes the hook plugin
  (#109).** A top-level `local` crashed every opencode uninstall after the agents
  were gone, and a malformed `config.json` crashed its MCP step; the plugin was
  left behind. Its plugin cleanup also knew only 6 legacy flat file names.
  - Plugin removal is delegated to a hidden `devexp uninstall --target opencode`
    that applies exactly the install's rules, from the same code: only
    devexp-owned files (recorded, or recognised on disk when the manifest is
    lost), never through a symlinked `plugins/`, a refusal for a symlinked
    `devexp/` or a dangling `plugins/` link, all-or-nothing on `devexp.js` +
    `devexp/` (so an edited, unrecognised `devexp.js` never loses its
    `hooks.json` and blocks every opencode tool call), and the byte-preserving
    legacy `config.json` edit. A Go test pins it to the every-hook-disabled
    install on every fixture. It refuses to run when `HOME` is unset, empty or
    relative.
  - It runs before the MCP step rewrites `config.json`. The opencode manifest's
    `plugins` key is then cut down to what had to stay, only when the manifest
    is a regular file that loaded cleanly, never through a symlink (a dangling
    link no longer creates its target) and never on `--dry-run`.
  - An install with only the plugin (no agents) is now detected.
  - The opencode MCP step, now reachable, never fails the uninstall: a
    malformed or oddly shaped `config.json` is skipped with a message, a
    symlinked one is left untouched (as the plugin step does), and one that
    can't be written (read-only file or directory) is left as it was with a
    warning, so the later steps still run. Its save replaces the file
    atomically, keeping its mode.
  - `uninstall.sh` finds the binary through `DEVEXP_BIN` (set by the wizard's
    Remove action), then `bin/devexp`, then `PATH`. Without one that has the
    command, it warns, leaves the plugin in place, prints the rebuild hint
    (`rm bin/devexp && ./install.sh`) and still exits 0.

- **An unreadable install manifest crashed `devexp install`.** When
  `.devexp-manifest.json` couldn't be read (for example a directory at that
  path), `manifest.Load` returned no manifest and both install targets panicked.
  It now returns an empty manifest with the error. The install warns, removes
  no stale agents or skills on that run, and rewrites the manifest at the end. A
  malformed manifest gets the same warning; before, it was silently treated as
  empty. Partly decoded data is discarded, so it can never mark files stale.
  `--agents-only`/`--skills-only` runs no longer print the opencode `Hooks :`
  line.

- **opencode plugin: data-driven entry, explicit registry mapping, parity fixes
  (#107).** Groundwork for installing the plugin (#108); no user-visible change
  until then.
  - `hooks/opencode/devexp-plugin.js` no longer statically imports all 10 modules
    through one `Promise.all`, where a single broken module rejected the whole
    plugin and left opencode running with no guard. It now exports exactly one
    function and composes only the modules listed in the installed selection
    `devexp/hooks.json`. A module that fails to import or initialise is skipped
    and logged; if it is a fail-closed guard every tool call is blocked with an
    internal-error message instead. Fail-closed can't hinge on one key: an entry
    blocks on `failClosed: true`, the registry spelling `fail_closed: true`, or a
    security-guard name (`secret-guard`, `secret-in-write-guard`,
    `dangerous-cmd-guard`, hard-coded in the entry). A missing, malformed or
    empty `hooks.json`, or an entry that isn't an object with a `name`, blocks
    too. `module` must be a bare `.js` file name (allowlist) that resolves inside
    `devexp/` — `node:` builtins, `%2e%2e` and `\`-separated paths are refused.
  - lint/format/test-on-save never ran in opencode: `file.edited` is not a plugin
    hook key, so file events only reach plugins through `event`. The entry now
    adapts `event` → `file.edited` and hands each module `{ file }`. Because
    opencode runs plugins in its server process, the handlers are queued so
    `event` returns at once, and the three modules (plus `utils.js` `which` /
    `runLinter`) now spawn asynchronously through a new `runCommand` helper with
    the same tools, cwd, output and 10s/15s/20s timeouts — a slow linter no
    longer freezes every session. `format-on-save` stays on for opencode; its
    rewrite lands after the edit tool computed its diff (documented).
  - opencode `test-on-save` threw `ReferenceError: path is not defined` on every
    source-file edit (`isTestFile` called `path.basename` without importing
    `path`), so it could never run a test even once file events arrived. It now
    uses the `basename` it already imports from `utils.js`.
  - The opencode `secret-guard` message now matches Claude Code's:
    `Blocked access to "…"` (was `Blocked read of` / `Blocked bash access to`).
  - `hooks/registry.json` maps each hook explicitly per install target: every
    `opencode` block has `module`, `export`, `fail_closed` (security guards) and
    `enabled` (the `graphify-*` hooks stay on for opencode). The Go registry type
    is target-generic — `hooks.Hook.Targets` is a `map[string]hooks.TargetSpec`
    filled from every sibling block, with `EnabledFor(target)` — so a new target
    is a new registry block, not a new Go type. The Claude Code install is
    unchanged (identical `settings.json` before and after).
  - "Add a hook" docs no longer tell authors to edit `devexp-plugin.js`; the
    `opencode` registry mapping is the touch point (`CLAUDE.md`, `conventions.md`,
    `workflows.md`, `hooks/README.md`, `reference/hooks.md`,
    `hook-authoring-guide.md`, `architecture/overview.md`, `docs-sync` agent).
    New `hooks/opencode/devexp-plugin.test.js`, recorded in `testing.md`.

- **Docs and agent sources drifted from the code** — found by the first `/devxp`
  run with the development kit, fixed by `update-docs` passes verified against code:
  - opencode hooks were documented as installed; the Go CLI never deploys
    `devexp-plugin.js` (known gap, `docs/architecture/overview.md`). Corrected in
    `install.md`, `README.md`, `hooks/README.md`, `reference/hooks.md`,
    `hook-authoring-guide.md`.
  - `install.md`/`README.md`: `git pull && ./install.sh` doesn't rebuild the CLI
    (`rm bin/devexp` first); `--model` doesn't skip the wizard and only rewrites
    existing `model:` lines; opencode skills go to `commands/`; backups are
    Claude Code only.
  - `docker_compose` MCP field and auto-start removed from `mcp-guide.md`,
    `reference/mcps.md`, `mcps/README.md` (dropped in `61f6c9f`); `headers`,
    re-install and secrets behaviour documented as implemented.
  - `agent-authoring-guide.md`: real `modelMap`, opencode tool mapping and
    `agents/opencode/` handling; `skill-authoring-guide.md`, `templates/README.md`,
    `adr/README.md`, `coverage.md`, `agent-architecture-reference.md`,
    `guides/README.md`, `quickstart.md` corrected.
  - 15 agent sources chained to skills removed in `13f3cf8` (`/refactor`,
    `/bugfix`, `/quality`, `/logic-review`, `/api-design`, `/db-design`, `/scope`,
    `/dead-code`, `/dep-map`, `/groom`, …); now point to the agents or
    orchestrators that absorbed them. `grooming-agent` Phase 7 writes the plan
    itself instead of invoking the missing `/groom` skill.

### Changed

- **Release guide: rollback defined.** `docs/guides/release.md` now defines rollback
  for both targets, so `/release` no longer blocks on `[CONFIRM]`: `cli` is
  hotfix-forward (mark the bad release pre-release so `latest` falls back, users
  pin `DEVEXP_VERSION`, cut the next patch); `toolkit-clone` reverts on `main`
  through a PR. The GitHub Release is created by `/release` with the CHANGELOG
  section as notes, goreleaser uploads the assets.
- `/release` phases renumbered: new Phase 7 (Ship Targets); retirement is now
  Phase 8 and the report Phase 9. Retirement requires every target shipped or
  skipped, not just a successful tag.

## [0.7.1] - 2026-09-16

### Security

- **Harden hook input handling** (#119). A Claude Code hook script could let
  tool-supplied values be interpreted as code under crafted input. Such values
  are now passed to the hook's interpreter strictly as data.
- **Harden hook interpreter isolation** (#119). Every Python-based Claude Code
  hook now runs its interpreter in isolated mode, so hook interpreters ignore
  modules in the project directory (and `PYTHON*` variables and user
  site-packages, which no hook needs; tools a hook launches still get the full
  environment).
- **Harden path handling** (#119). The on-save hooks and the large-file guard
  now act on the exact path they are given.
- Decisions for legitimate inputs are unchanged. All Claude Code and opencode
  hooks were audited; the opencode modules are not affected. Regression tests
  cover hostile input shapes, interpreter isolation for every hook, and exact
  path handling. The hook authoring guide now documents these rules and the
  fail-closed/fail-open contract for internal errors. Re-run `./install.sh` (or
  update the CLI) to deploy the fix.

## [0.7.0] - 2026-09-15

### Added


- **`cmd` coverage: 27.1% → 35.4%** (sub-ticket B of #69), covering every statement
  reachable without testing the wizard TTY flow or the real exec paths.
  - `selectTargets` is now fully covered, including the **both-CLI branch** — the
    one #70 flagged as unverifiable, because a dry-run on a single-CLI machine
    can never reach it. All four availability combinations plus the choice
    mapping are table-driven.
  - `claudeTargetPaths`/`opencodeTargetPaths` assert exact destinations; passing
    `now` as a parameter is what makes the backup directory's name checkable
    rather than clock-dependent.
  - `loadFullRegistry` covers a valid registry, a missing one, extra MCPs merged
    from config, and malformed extra JSON — which warns without failing the load.
  - `detectTargets` drives its arms through a `PATH` containing only fake
    executables, so no real CLI can leak into the result.
  - The three `backup.go` partials are finished: both `MkdirAll` failure returns,
    the unreadable-match `continue`, the `ReadDir` failure, the non-directory
    skip, and `removeStale`'s non-`IsNotExist` error path — which must warn and
    keep going rather than abort the remaining removals.
  - **Assertion strength proven by mutation, not inferred from coverage.**
    Dropping `"Both"` handling fails exactly the `picks_Both` case; dropping the
    "no CLI detected" error fails both the direct case and `detectTargets`'
    delegation; making both `announceTargets` arms announce the same text fails
    exactly `opencode_only_announces_opencode`. No other case moves.
  - `TestAnnounceTargets` captures stdout to assert each arm's *distinct*
    announcement. Its first draft asserted the same empty choice three times, so
    it would have passed unchanged had both arms printed the same thing — or
    nothing. A weak assertion inside this delivery's own diff, fixed here rather
    than filed as a follow-up.

- **`internal/repo` coverage: 28.3% → 83.0%** (sub-ticket C of #69). `extractFS`,
  `extractEmbedded` and `Resolve` were all at 0% despite being genuinely
  unit-testable — the package's asset-resolution path had no tests at all.
  - `extractFS` is exercised against an `fstest.MapFS` fixture: recursive copy,
    parent directories created, contents preserved, and `.sh` files marked
    executable while everything else stays `0644`.
  - `extractEmbedded` covers a fresh extraction with its version marker, reuse of
    a prior extraction at the same version, and a re-extraction when the version
    changes — the upgrade path that discards a stale copy.
  - `Resolve` covers both dispatch arms: a live repo found via `DEVEXP_DIR`, and
    the embedded-extraction fallback when no repo exists up-tree.
  - **`os.UserCacheDir` is now indirected through a package-level `userCacheDir`
    variable** so extraction can be redirected into `t.TempDir()`. Without that
    seam, testing `extractEmbedded` would write to the developer's real cache and
    make results depend on what was already there. This is the one production
    change in an otherwise test-only ticket.
  - Assertion strength was checked by mutation rather than inferred from the
    coverage number: breaking `extractFS`'s executable-bit logic turns the
    relevant subtest red and leaves the others green.

- **`/release` — the release phase, as its own command.** Release is a named phase of the development cycle, but it existed only as `/deliver` Phase 6, which made it unreachable on its own. The practical consequence: declining the release gate left **no way to resume** — the only options were re-running `/deliver` (redoing implementation) or releasing by hand, which is the main reason worktrees accumulated and why `/cleanup` had to exist. `/release <ticket>` now closes that loop.
  - **Process:** read-only preflight → gate → merge → changelog → version bump → tag and publish → retire artifacts → report.
  - **The gate is its own decision.** Consent is never inherited from `/deliver`'s Phase 1 "proceed" — release is irreversible and touches shared systems.
  - **Version bump is derived, not guessed:** breaking → major, `feat:` → minor, else patch. Non-conventional history is reported rather than invented around.
  - **Failure preserves, success retires.** Only a completed release removes the worktree, plan, groom session and scratch. Every failure path keeps them.
  - **Tag push is the point of no return** — after a partial push, `git ls-remote --tags origin` is checked before any retry.
  - Fixes a **dangling reference**: `agents/changelog.md` already chained to "invoke `/release` skill", which did not exist.
- **`/deliver` Phase 6 now delegates to `/release`** and reports one of three outcomes (released / deferred / failed). Former Phase 7 (retire artifacts) moved into `/release`, where it belongs — it was always gated on release success. Phase 8 became Phase 7. If `/release` is not installed, delivery reports it and stops rather than improvising a half-release.
- Documentation surface: **seven commands → eight** — six lifecycle orchestrators and two utilities.
- **The `postmortem` agent is reachable again.** It was a complete agent that no command ever invoked — `/improve` Phase 5 ran its own retro instead, and `/monitor` had no incident path at all. Both now hand off to it: `/monitor`'s report suggests it when an anomaly traces to an incident, and `/improve`'s retro suggests it before the timeline fades.
- **The planification phase is named.** The cycle includes it, but no document did. `/refine`'s verified execution plan *is* that phase; the skills reference now says so — and says why it has no command of its own: a plan with no ticket to attach to is just a document.

### Changed


- **`cmd/install.go` split from 762 to 193 lines** (sub-ticket A of #69). One file
  held the entry point, the interactive wizard, both install paths, registry and
  target resolution, and backup/stale handling — which is why its install-flow
  functions sat at 0% coverage: the testable logic was fused into I/O- and
  TTY-bound monoliths.
  - Split along cohesive seams: `install_claude.go`, `install_opencode.go`,
    `wizard.go`, `targets.go`, `registry.go`, `backup.go`, `paths.go`.
  - **Behaviour held identical by diff, not by assertion.** A
    `devexp install --dry-run` baseline was captured from merged `main` before any
    edit and verified deterministic across repeat runs; the output after every
    step is byte-identical to it.
  - **Pure logic extracted so it can be tested** (#72's enabler):
    `selectTargets` maps CLI availability onto the install flags with no PATH
    lookup, printing or prompting, and `announceTargets` holds the I/O half —
    the flag path and the wizard previously carried separate copies of the same
    switch. `claudeTargetPaths`/`opencodeTargetPaths` replace destinations
    assembled inline from `$HOME`, taking `now` as a parameter so the backup
    directory's name is assertable.
  - Exported `cmd` API unchanged (`go doc` diff against `main`); `go test -race`
    green across 10 packages; `go vet` and `gofmt` clean.
  - **Gate limitation, recorded honestly:** with `opencode` absent from PATH, the
    dry-run exercises only the single-CLI arm and never enters
    `doInstallOpencode`. The Claude path extraction *is* exercised; the both-CLI
    branch and the opencode path rest on review until #72's unit tests land.


- **`/deliver` and `pr-review` no longer defer in-scope defects.** Both told the delivery cycle to ship a known defect with a ticket attached: `deliver` Phase 5 said *"Fix only what's clearly wrong; note the rest as follow-up debt"*, and `pr-review` offered *"Approve with minor comments — merge is fine, but address comments in follow-up."* Neither applied a scope test, and no fold-vs-file policy existed anywhere in the toolkit.
  - **The rule, stated once:** a defect **inside the change being delivered** is folded into that delivery before the release gate; only work **outside its scope** becomes a ticket. Pre-existing debt elsewhere remains `/improve` Phase 4's job — the gap was that nothing routed findings between the two paths.
  - **Severity does not decide it, scope does.** A "minor" finding inside the diff is still this delivery's to fix; `pr-review`'s "Approve with minor comments" is now valid only when every comment falls outside the PR's own diff.
  - Filing a follow-up for something in scope ships a known gap, inflates the backlog with work that should have been finished, and makes "done" mean "done except the parts we wrote down."

- **`ui-inspector` now ships as its own repo** — [mcp-ui-inspector](https://github.com/alexandrocuma/mcp-ui-inspector). It was vendored here as a Node project with its own `setup.sh` and a committed `dist/`, which quietly turned a distribution repo into a monorepo. The registry locates it via `UI_INSPECTOR_DIR`, documented in the MCP env template, reusing the installer's existing `[REQUIRED]` warning and `setup_instructions` path — **no Go changes were needed**, because the CLI never special-cased it (it generically injects `DEVEXP_DIR` and renders `setup_instructions`). The extracted repo gitignores `dist/` instead of committing it; the vendored copy had gone stale, with `src/tools/interact.ts` having no built counterpart.

### Removed


- **`start-services.sh`.** It had become a documented no-op, printing "No background services required" — `ui-inspector` was the only service it ever managed, and that launches its own Chromium on demand and shuts it down on SIGTERM. Removed with its references in `README.md`, `CLAUDE.md`, `docs/README.md`, `docs/guides/README.md` and `docs/guides/install.md`.
- **`/promo-campaign` and the domain-playbook category.** Marketing is not a phase of the development lifecycle. This toolkit covers idea → refinement → grooming → planification → delivery → release → cleanup → improvements/postmortem; promoting an app that already ships sits outside that loop. The skill was also the repo's largest single component (~2,700 lines), the only one requiring ffmpeg, ImageMagick, Maestro and Xcode, and iOS-Simulator-only — so it did not work on the majority of its own author's projects. It moves to **`marketing-toolkit`**, a sibling project, where it gains Android capture, an audio path and a platform-plural contract.
  - **The category goes with it.** It had exactly one member, and its four-part entry test (app-agnostic, user-invoked, interactive, not absorbable by an orchestrator) admitted a capability that the lifecycle test rejects.
  - **Documentation surface: eight commands → seven** — five lifecycle orchestrators and two utilities (`/graphify`, `/cleanup`) — across `README.md`, `skills/README.md`, `docs/reference/skills.md`, `docs/coverage.md`, `docs/README.md` and `CLAUDE.md`.
  - **Corrected a stale count** found while rewriting those lines: the skills reference claimed "~40 specialist capabilities" where every other document says ~30.
  - **Installed copies retire themselves** — `manifest.Stale()` removes vanished skills on the next `install.sh`.

### Fixed


- **The opencode installer no longer strips `name:` lines from a skill's body.**
  `InstallOpencode` is documented to remove "the `name:` frontmatter line", which
  opencode derives from the filename — but it dropped **every** line of SKILL.md
  whose trimmed text began with `name:`. A YAML example, a config snippet or a
  table row was silently deleted from the opencode copy, so the two CLIs received
  different instructions from the same source file.
  - Only the **top-level** `name:` key inside the leading front matter block is
    removed now. An indented `name:` is left alone even within the front matter,
    because it is a key nested under another value rather than the skill's own
    name.
  - Content without a properly delimited front matter block is returned
    unchanged. An unterminated `---` is malformed, and guessing where the
    metadata ends would risk deleting body lines — the failure mode being fixed.
  - The hazard had been shaping how skills were written: `docs/reference/skills.md`
    told authors to keep `name:` lines out of SKILL.md bodies, and the (since
    removed) `promo-campaign` skill carried an inline comment forcing a contract
    key onto one line to survive the transform. That guidance is retired.
  - `docs/guides/install.md` still says `name:` is stripped for opencode, which
    remains accurate — the front-matter key is.

- **A clone install can now clean up hook registrations left by a release-binary
  install.** The two installs use different roots: a release binary extracts its
  assets to the user cache and registers commands there, while a clone registers
  commands inside the clone. Both sets survived, so every hook ran twice — once
  current, once frozen at whatever the other root last held. On the machine that
  surfaced this, `settings.json` carried **14 registrations for 7 hooks**, the
  second set three months stale, and `./install.sh` reported
  `[skip] already registered` for all of them while changing nothing.
  - **Why it was unreachable:** `isStaleDevexpHook` prunes only commands *under*
    `repoDir` whose script is missing. A foreign-root command yields a
    `..`-prefixed relative path, so the function returned `false` immediately and
    the entry could never be touched. The intent — don't delete hooks you don't
    own — was right; the premise was wrong, because devexp *did* write these,
    just from a different root.
  - **The fix:** identify a devexp hook by what the registry says it is — a
    `claude_code.script` basename plus the registry's own `hooks/claude-code/`
    directory — rather than by path prefix. A registration matching that outside
    the current root is a duplicate by definition, since the registry admits one
    script per hook, and is removed. Disabled hooks are included: a foreign copy
    of a disabled hook still runs.
  - **`uninstall.sh` had the mirror-image bug**, matching `if repo_dir in cmd`,
    so uninstalling from a clone stripped the *working* registrations and left
    the stale ones behind — worse than doing nothing. It now uses the same
    predicate. A second defect there is fixed in passing: it judged each entry by
    `entry['hooks'][0]` alone, so any additional command in the same entry was
    handled by accident rather than on its merits.
  - Covered by `TestIsForeignDevexpHook`, `TestPruneForeignDevexpHooks` and a new
    `uninstall.test.sh`, including a user hook that merely shares a basename, an
    entry holding both a devexp and a user command in either order, and a sibling
    root sharing a path prefix without being nested — the case a `strings.HasPrefix`
    check would have got wrong. `uninstall.test.sh` runs in CI via a new step, so
    the uninstaller has coverage for the first time.

- **Hooks no longer fail open silently.** Every shell hook extracted its decision
  input by piping the tool envelope through `python3`, wrapped as
  `2>/dev/null || echo ""`. Any interpreter failure — a syntax error, a missing
  `python3`, a malformed envelope — collapsed to an empty string, which is
  indistinguishable from *"ran fine, nothing to block."* The hook then exited 0
  and allowed the operation, saying nothing. **Seven of ten shell hooks shared
  the shape**, including all three security guards.
  - **Guards now fail closed.** `secret-guard`, `secret-in-write-guard` and
    `dangerous-cmd-guard` exit 2 with a named internal error. An empty extraction
    makes every pattern check trivially pass, so allowing there would have
    permitted precisely what each guard exists to stop — reading dotenv and
    private-key files, writing API keys, and `rm -rf` / `git push --force`.
  - **Advisory hooks fail open, but loudly.** `large-file-guard`,
    `format-on-save`, `lint-on-save` and `test-on-save` still exit 0 — blocking
    every `Write` because a formatter's parser broke would wedge the user — but
    they now say the hook did not run.
  - `2>/dev/null` was dropped throughout, so the interpreter's own error reaches
    the terminal instead of being discarded.
  - `hooks/claude-code/fail-closed.test.sh` asserts the exit code *and* that a
    message was printed, for all seven, plus that a clean envelope still exits 0
    quietly. The opencode hooks are unaffected: they run in-process and their
    guards already propagate exceptions.

- **`secret-guard` no longer blocks committed templates or bare mentions.** Two
  distinct false positives in the same guard, both of which obstructed without
  protecting anything:
  - **Templates.** A `.env.` catch-all classified every dotenv-prefixed file as a
    secret, including the committed templates (`*.example`, `*.sample`,
    `*.template`, `*.dist`) that exist precisely to document which keys a project
    needs. The guard blocked the one file a user is meant to read in order to
    configure the rest, and `docs/reference/mcps.md` tells them to read it.
    Templates are now allowed even when their stem is a real secret name.
  - **Mentions.** Every shell token was treated as a path, so program text and
    prose that merely contained such a string were refused: `jq -r '.key'` was
    blocked as a key file, and a heredoc naming a dotenv file blocked the whole
    command. A token now counts as a path only if it plausibly is one — heredoc
    bodies are stripped, tokens carrying program syntax or spaces are skipped,
    and an extension match requires a stem, so `server.key` blocks while `.key`
    does not. Exact names stay blocked unconditionally.
  - Both implementations fixed together (shell and opencode JS), each with a test
    twin asserting the red cases still block and the green cases now pass.

- **Hook tests now run in CI.** `dangerous-cmd-guard` shipped with tests that
  nothing executed; the workflow ran only `go test`. A `hooks` job now runs every
  `*.test.sh` and `*.test.js` beside the hooks, so a guard regression fails the
  build instead of surfacing as a mysterious block months later.

## [0.6.0] - 2026-09-14

Epic #77 — promo-campaign: the toolkit's first domain playbook, with its iOS Simulator capture and segment reel composer (#76). Android capture and the carousel composer (#78) follow.

### Added

- **`/promo-campaign` — app promo playbook (1st domain playbook).** A user-invoked command, with `disable-model-invocation: true` and an `argument-hint` of `[platform] [locale]`, that plans an app's promo reel and carousel from real footage. It is the first skill in a new domain-playbook category, whose entry test is: app-agnostic, user-invoked, interactive, and not absorbable by an orchestrator. (#75)
  - **Process:** feature extraction → exactly one hook → storyboard → claim and badge check → instance file → capture/compose handoff. The user confirms at three checkpoints.
  - **Claim rule:** discovery sources (changelog, README, code) find features, but only the claim authority (the current store listing or landing page) may back a caption or slide. Unclaimed features are reported, never captioned.
  - **Badge rule:** a store badge appears only for a listing publicly installable when checked signed-out, or when marked `launch_day: true` with a do-not-post-before date.
  - **Storyboard limits, measured on the render:**
    - ≤ 20.0s (shorter wins), and the render equals the plan within one frame
    - 1.0x, never sped up
    - captions: one idea each, ≥ 2.0s each, ≤ 8 per locale
    - hook lands ≤ 5.0s
    - end card ≤ 3.0s including its transition, with no caption over it
  - **Cut order:** compress pre-hook setup → drop beats from the end → drop repeated beats.
  - **Outputs:** a 9:16 reel (1080x1920) and a 4:5 carousel (1080x1350).
  - **Instance-file contract, inline in SKILL.md** (opencode installs SKILL.md only): `docs/marketing/campaign.md` in the consumer repo, with YAML front matter `schema: promo-campaign/v1`.
    - Keys: `app`, `platforms` (optional), `locales`, `max_duration_s`, `claim_sources`, `features`, `hook`, `beats`, `captions.<locale>`, `carousel.<locale>`, `end_card`, `outputs`, and reserved `seed`/`capture` blocks whose internals #76 defines.
    - Parser-proof rules: quoted strings and locale keys, decimal-second times, `true`/`false` only.
    - A dependency-free Ruby check in SKILL.md lints those rules before loading, type-checks, then enforces every limit, the single hook beat, verbatim claims and the badge rule, reporting every error by key.
    - The skill also maintains the consumer's `docs/marketing/README.md` index and `docs/README.md` entry, and git-ignores `outputs.dir`.
  - **Degrades without #76:** when the skill's `scripts/` directory is absent, it stops after writing the instance file and says so. `references/` ships a fictional, limit-passing EXAMPLE campaign and a blank template.
- **promo-campaign iOS capture and segment reel composer.** Bash scripts bundled in `skills/promo-campaign/scripts/`. They target `/bin/bash` 3.2, run as `bash <path>` because skill files install 0644, and work in Claude Code only (opencode installs SKILL.md alone). (#76)
  - **Contract:** defines the `seed` and `capture` blocks: `seed.backend` + `entries[]`; `capture.ios`; `takes[].steps[]` (Maestro flows interleaved with `hold_s` and `appearance`); `stills[]`; `cut[]` segments; `compose` styling. SKILL.md has a compact key table; `references/capture-and-compose.md` has the full schema, cue sheets, traps and the mutation record.
  - **`lib/contract.sh`:** converts front matter with ruby, then python3 + PyYAML, then yq v4. Each path applies SKILL.md's parser-proof lint first, naming the key path and line. A jq pass then type-checks every key the scripts read.
  - **`capture-ios.sh`:** one booted Simulator, pinned by UDID for every simctl and Maestro call. Each take: fresh install → `rn-asyncstorage` seed → demo status bar → starting appearance → `recordVideo` (h264), with the steps driven only after `Recording started`. Writes a cue sheet from Maestro's `commands.json`, a lossless still pass with `simctl io screenshot`, and a restore-on-exit trap.
  - **`seed-rn-asyncstorage-ios.sh`:** writes AsyncStorage's `manifest.json` before the first launch, spilling any value longer than 1024 UTF-16 units to a file named the MD5 of its key.
  - **`compose-reel.sh`:** a 1.0x reel from `capture.cut[]`: `trim` + `setpts=PTS-STARTPTS`, `concat` or `xfade`. Each take is padded with `tpad` to its logged length, and `settb=AVTB,fps=30` is applied before every join. Adds the phone window derived from the source aspect, PNG32 frame and caption layers with `shortest=1`, and a `-respect-parentheses` end card. Output: 1080x1920 H.264.
  - **Self-checks that fail the run:** frame alpha, caption band, out-points within the logged take, no re-timing, segments − fades = beats, render = plan within one frame and ≤ `max_duration_s`, size.
  - **Proven by mutation:** guards for frame alpha, `shortest=1`, take `tpad`, per-join normalisation, Σ segments ≠ Σ beats, PTS scaling, out-of-take out-points and caption band all go red. Single-segment and 1080x2220-source controls stay green.
  - **SKILL.md Phase 7** names the entry points, passes absolute paths, and reports each missing component separately: Android capture and the carousel composer are #78. Step 1 runs `contract.sh validate … capture`, and `all` runs once the cue sheets exist.
  - **Review fixes (#82):**
    - A caption ending after the end-card start, starting below 0, or empty is refused.
    - `--launch-day` writes `reel-<locale>.launch-day.mp4` and never replaces the postable reel.
    - Capture:
      - refuses an already-installed app unless `--replace-installed`
      - saves, clears and restores the host clipboard (text only; `--keep-clipboard` skips it) and empties the Simulator pasteboard
      - removes a take's previous files before recording, so a failed recapture cannot leave a stale take
      - keeps a cue when Maestro gives no timestamp
      - discards an unfinalised recording with a reason
    - The parser lint, in all three script paths and SKILL.md's check, refuses anchors, aliases and duplicate keys.
    - Stricter formats for locales, `outputs.dir`, `bundle_id` and `PROMO_RENDER_TIMEOUT_S`.

### Changed

- **Documentation surface reframed from seven commands to eight** across the skills catalog, READMEs, CLAUDE.md and coverage map: five lifecycle orchestrators, two utilities (`/graphify`, `/cleanup`) and one domain playbook (`/promo-campaign`). The skills reference gains a Domain Playbooks section with the entry test, plus notes on supporting files (Claude Code only, installed 0644) and on `disable-model-invocation`. (#75)

Epic #73 — on-demand cleanup, worktree access grants, and a closed manual-release gap.

### Added

- **`/cleanup` — on-demand artifact retirement (2nd utility).** A standalone command that retires finished or abandoned delivery artifacts: merged/superseded git worktrees, orphaned branches (never the default branch), stale persisted plans, groom-session leftovers, and stray `/tmp` scratch. Discovery → live/finished classification → always-on dry-run report → explicit confirmation (or `--cleanup` pre-confirmed mode with a line-by-line removal log) → scoped removal. Follows the cleanup-safety rules inline; explicitly out of scope: agent-memory pruning (stays in `/improve` C2, which owns codebase-navigator's drift classification), the main checkout, and release/changelog work. `/cleanup <ticket>` scopes the sweep to one ticket.
- **Worktree access grants at creation.** `/deliver` Phase 1.5 (and `/improve`'s parallel cleanup streams) now grant the runtime access to each new worktree immediately after `git worktree add` — the tree lives at `../<repo>-worktrees/<ticket>`, outside the project root, so without a grant every write prompts. Claude Code: the worktrees parent directory (absolute path) is merged into the project's `.claude/settings.json` `additionalDirectories` (idempotent, existing content preserved). opencode: no path-scoped grant exists in its tool-scoped permission model, so the user is told to approve the first write with "always allow" for the worktrees directory. The worktree-per-ticket lifecycle is now create → grant → work → merge → remove.

### Fixed

- **`/deliver` no longer orphans worktrees on a declined release gate.** When the user chooses "I'll release manually," Phase 6 now checks whether the ticket branch is already merged to the base: merged → the worktree and branch are removed immediately (same commands as the success path); not merged → the tree is kept and the user is told `/cleanup <ticket>` (or plain `/cleanup`) will retire it once the branch lands. The final report's worktree line reflects the deferred-release state. (#73)

### Changed

- Documentation surface reframed from six commands to seven (five lifecycle orchestrators — `/devxp`, `/refine`, `/deliver`, `/improve`, `/monitor` — plus the `/graphify` and `/cleanup` utilities) across the skills catalog, READMEs, CLAUDE.md, and coverage map. (#73)

## [0.5.0] - 2026-06-15

Epic #54 — `/monitor`, a new operate-phase orchestrator for reviewing the health of deployed systems.

### Added

- **`/monitor` — deployed-system health review (5th orchestrator).** A new operate-phase command that assesses the *running* system rather than the codebase. It detects stack surfaces by category (cloud/infra, dashboards, logging, alerting, tracing/metrics) from repo signals and already-authenticated connectors, reviews each surface live (read-only connector query) or via a config-as-code fallback, and produces a change-independent, equal-weighted composite health score with a ranked, actionable anomaly list. Vendor-agnostic (names no platform in its prompt) and credential-safe (never triggers auth or stores secrets). `/monitor <surface>` scopes the review to a single detected surface. (#55, #56, #57, #58)
- **Persisted health review + `/improve` reconciliation.** `/monitor` persists its scored report to `.devexp/system-health-review.md`; `/improve`'s Observability Maturity dimension now defers to that artifact when present — one home for "is the system healthy?" The existing `observability` baseline key is reused for trends with no schema migration. (#59)

### Changed

- Documentation surface reframed from five commands to six (five lifecycle orchestrators — `/devxp`, `/refine`, `/deliver`, `/improve`, `/monitor` — plus the `/graphify` utility) across the skills catalog, READMEs, CLAUDE.md, and quickstart. (#60)

## [0.4.0] - 2026-06-15

Epic #32 — fresh memory, worktree-per-ticket delivery, and completion cleanup.

### Added

- **Worktree-per-ticket delivery.** `/deliver` isolates each ticket in its own git worktree (create → work → merge-at-release-gate → remove), with a single-stream fallback; each epic sub-ticket gets its own worktree so independent work runs in parallel. `/improve` isolates parallel cleanup streams the same way. New `docs/guides/worktree-per-ticket.md` convention. (#36, #37, #38)
- **Memory freshness.** `codebase-navigator`'s coarse 30-day rebuild window is replaced by a canonical Drift Classification (CURRENT/SMALL/BIG keyed on *what changed*, not *how long ago*); `dev-agent` and `grooming-agent` run a cheap freshness gate before trusting the atlas; persisted plans are stamped with the commit they were validated against, and `/deliver` re-grooms on big drift. (#33, #34, #35)
- **Completion cleanup.** `/deliver` gains a final phase that retires the ticket's artifacts (worktree, persisted plan, groom session, `/tmp` scratch) on successful completion; `/improve` gains a repo-wide hygiene sweep for orphaned artifacts. Both follow the new `docs/guides/cleanup-safety.md` deletion-safety rules (dry-run, validated id guards, prefix-anchored globs). (#39, #40)
- Allow/deny test suites for the `dangerous-cmd-guard` hook (claude-code and opencode). (#50)

### Changed

- `dangerous-cmd-guard` now blocks unanchored wildcard deletes in sensitive directories (`/tmp/*`, `~/.claude/.../*`, and quoted variants) as an execution-time backstop for the cleanup phases. (#50)
- Toolkit-internal `docs/` references in the orchestrator skills are labeled as maintainer-only, since `docs/` is not installed into a user's environment. (#51)

### Fixed

- `dangerous-cmd-guard` no longer false-positives on a force flag that appears elsewhere in a command (e.g. a short flag inside a commit message); the force-push rule now matches only an argument of the same push command. (#50)

## [0.3.0] - 2026-06-13

### Added

- `devexp install` now performs manifest-based stale-file cleanup: each run records the agent/skill files it installs in `~/.claude/.devexp-manifest.json` (and `~/.config/opencode/.devexp-manifest.json` for opencode), and removes any from a prior run that are no longer shipped in the current version.
- Skill directories are now backed up to the timestamped backup folder before being overwritten, matching the existing behavior for agents.
- Stale hook pruning: hook entries whose backing script no longer exists on disk (because the hook was removed from `hooks/registry.json`) are automatically dropped from `settings.json`; user-authored hooks are left untouched.
- "Updating" section added to `docs/guides/install.md` documenting the update path, what's overwritten vs. preserved, stale-file/hook cleanup, and the one-time pre-manifest baseline caveat.

### Changed

- Disabling an agent or skill in `devexp.config.json` now removes it from disk on the next install (previously it only skipped updates, leaving the old copy in place).

### Fixed

- CI: bumped `actions/checkout` to v6 and `actions/setup-go` to v6 (with `cache-dependency-path: cli/go.sum`) in both workflows, and `goreleaser/goreleaser-action` to v7 — resolves the Node 20 deprecation warning and go.sum cache-restore failure seen on the v0.2.0 release run.

## [0.2.0] - 2026-06-13

### Added

- Canonical **Persistent Agent Memory** section rolled out to 25 agents (8 Category B, 9 analysis, 8 workflow) — every memory-enabled agent now follows a consistent format for storing and recalling project-specific context across sessions.
- "Persistent Agent Memory" checklist item added to the Agent File Checklist so new agents adopt the canonical pattern by default.
- Agent-memory duplication mapping convention: documents `~/.claude/agent-memory/graphify-out/` and wires a manual cross-agent duplication check into `/improve` Phase 3.
- Table-driven test coverage for the `cli/` Go module (previously 0%), covering `cli/cmd/install.go`, `cli/internal/config`, and `cli/internal/repo`.

### Changed

- Go table-driven tests now use `map[string]struct{}` keyed by test name across the `cli/` test suite.
- Removed 9 Python install scripts superseded by the Go CLI (`devexp install`); `install.sh` now defers entirely to `bin/devexp`.

### Fixed

- Atlas freshness checks: `/devxp` and codebase-navigator now validate dated Gotchas/Technical Debt entries against git history before trusting them, and feature-path-tracer's memory follows the index+topic-file convention instead of duplicating the atlas. codebase-navigator self-heals old-format atlases on the next run.
- CI: stage embedded assets before running `go test` so embed-dependent tests pass.
- Bumped Go toolchain to 1.25.11 and Cobra/Viper to v1.10.2/v1.21.0 to resolve stdlib and dependency CVEs.

## [0.1.0] - 2026-06-11

Initial release.

### Added

- `devexp` Go CLI (`cli/`) — installs agents, skills, hooks, and MCPs for Claude Code and opencode. No-clone installation via `curl | bash`, which downloads a release binary and runs `devexp install`.
- 34 specialist agents covering the full SDLC — implementation (`dev-agent`), code review (`backend-senior-dev`, `frontend-senior-dev`, `pr-review`), analysis (`root-cause`, `arch-review`, `impact-analysis`, `data-flow`), quality (`security`, `performance`, `test-gen`, `test-runner`, `dep-audit`), release (`changelog`, `ci-cd`, `postmortem`), and more — plus an opencode-exclusive swarm orchestrator.
- 5 user-facing slash commands covering the full development lifecycle: `/devxp` (orient), `/refine` (groom tickets), `/deliver` (implement, test, review, release), `/improve` (health check, cleanup, retro), and `/graphify` (build a persistent knowledge graph).
- 10 safety/quality hooks — secret protection, destructive command blocking, large-file confirmation, lint/format/test-on-save, plus an optional `graphify-*` set — implemented for both Claude Code (shell scripts) and opencode (JS plugin).
- Curated MCP server registry (`context7`, `ui-inspector`) with automatic install and Docker-backed service support.
- Team distribution via `devexp.config.json` — disable agents/hooks, set a default model, and register org-internal MCPs.
- GoReleaser-based release pipeline producing darwin/linux (amd64/arm64) binaries.
