# Capture and Compose — Reference

The `scripts/` directory beside SKILL.md turns a `promo-campaign/v1` instance file into footage. SKILL.md Phase 7 says when to run each script; this file holds the full `seed` and `capture` schema, the cue sheet, the render arithmetic, the traps behind each rule, and the mutation record that proves the self-checks.

Claude Code installs this file and `scripts/`. opencode installs SKILL.md alone, so capture and compose work in Claude Code only.

## Entry points

Skill files install with mode 0644, so every script runs as `bash <path>`. `${CLAUDE_SKILL_DIR}` is substituted into SKILL.md's text; it is not an environment variable, so pass every path explicitly and absolutely. Each script finds its siblings from `${BASH_SOURCE[0]}`, prints its usage with `--help`, and checks its prerequisites before doing anything else.

| Script | Does | Needs |
|--------|------|-------|
| `capture-ios.sh --campaign <file> [--repo <dir>] [--take <id>]... [--no-stills] [--keep-app] [--replace-installed] [--keep-clipboard]` | Records every take on one booted iOS Simulator, writes cue sheets, then takes lossless stills | jq, perl, xcrun (Xcode), maestro, ffprobe, a YAML parser |
| `compose-reel.sh --campaign <file> [--repo <dir>] [--locale <l>] [--launch-day]` | Cuts `capture.cut[]` into a 1080x1920 reel per locale and runs every self-check | ffmpeg (with libx264), ffprobe, magick (ImageMagick 7), jq, perl, a YAML parser |
| `seed-rn-asyncstorage-ios.sh --udid <u> --bundle-id <id> --entries <json>` | The `rn-asyncstorage` seed backend; `capture-ios.sh` calls it | jq, perl, xcrun |
| `lib/contract.sh validate <file> capture\|all\|compose` · `lib/contract.sh json <file>` | Lints, converts and type-checks the front matter. `capture` checks what capture needs (no `cut` yet); `all`, the default, adds `capture.cut` and every composer key, so run it once the cue sheets exist; `compose` is what `compose-reel.sh` runs | jq, a YAML parser |

`--repo` defaults to the git top level of the campaign file's directory. Paths in the contract are relative to it; an absolute path is used as is.

**Exit codes:** 0 ok · 64 usage · 65 campaign data · 66 missing input file · 69 missing prerequisite or device · 70 a self-check failed · 1 a step or tool failed.

**Outputs**, all under `outputs.dir`. Nothing is ever written into the skill's install directory, and scratch goes to `mktemp -d /tmp/.promo-campaign-XXXXXX`, which is removed on exit.

```
<outputs.dir>/
  takes/<take>.mov              recordVideo output (H.264, variable frame rate)
  takes/<take>.cues.json        the cue sheet
  takes/<take>.failed-step-N.log  only when a Maestro step failed and its take was discarded
  stills/<id>.png               lossless simctl screenshots
  stills/<take>.failed-step-N.log only when a Maestro step failed in the still pass
  reel-<locale>.mp4             the postable reel, written only after every self-check passed
  reel-<locale>.launch-day.mp4  with --launch-day only: adds launch_day badges; never post before their do_not_post_before
  checks/reel-<locale>[.launch-day]-caption-N.png   one frame per caption, at its midpoint, for review
  checks/reel-<locale>[.launch-day].rejected.mp4    a render that failed a post-render check
```

A take's previous `.mov`, `.cues.json` and failure logs are removed before it records, and a still's PNG before its pass, so a failed run never leaves an older file for the composer to pick up. A cue sheet is written to a temporary name and moved into place last.

**Fonts and colours:** `PROMO_CAMPAIGN_FONT` / `PROMO_CAMPAIGN_FONT_BOLD` in the environment override `capture.compose.font` / `font_bold`. Without either, the default is macOS Arial (`/System/Library/Fonts/Supplemental/Arial.ttf`, `Arial Bold.ttf`), and a missing font fails the run naming both ways to set it. `PROMO_RENDER_TIMEOUT_S` (whole seconds ≥ 1, default 600) bounds each render; anything else exits 64, because perl's `alarm` truncates a fraction and 0 switches the watchdog off.

## Parsing the front matter

Parsers are tried in order: `ruby` (`YAML.safe_load`, ships with macOS) → `python3` with PyYAML (macOS's python3 has none) → `yq` v4. With none present, the script exits 69 naming all three. Whichever parser runs, the same lint as SKILL.md's Phase 6 check runs first, and each error names the key path and the file line:
- A plain (unquoted) key that loads as a boolean or null (`no`, `yes`, `on`, `off`, `~`, …) fails. That is a bare `no:` locale.
- A plain value that is not a decimal number, `true`, `false` or `null` fails. That covers `locale: no` (read as false), `time: 9:41` (read as 34860 by Psych, 581 by PyYAML) and `in_s: 0:05.5` (read as 330.0 by Psych, 5.5 by PyYAML). An empty value and `~` fail too.
- Inside a `{ }` flow mapping, Psych rejects `9:41` as a syntax error (line and column, no path), while PyYAML and yq read a plain scalar and name the path. All three reject it.
- An anchor (`&name`), an alias (`*name`) or a duplicate key fails. Left alone, Ruby's `safe_load` refuses aliases while PyYAML and yq resolve them, and Psych and PyYAML silently keep the last duplicate while yq keeps both in its JSON. The python3 path uses a SafeLoader subclass that records anchors and aliases with their key path and raises on a duplicate; the yq path queries `anchor`, `kind == "alias"` and the per-mapping key list explicitly.

A jq pass then type-checks every key the scripts read, and reports every error at once. For keys SKILL.md's check also reads, the types and defaults are its rules. The storyboard limits stay in SKILL.md; the scripts check what rendering needs:
- each `locales[]` entry matches `^[a-z]{2,3}(-[A-Za-z0-9]{2,8})*$`, because it names output files;
- `outputs.dir` is a repo-relative subdirectory, with no `.` or `..` segment;
- every caption has `0 ≤ start_s < end_s ≤` Σ `beats[].target_s` (the end-card start, with half a frame of tolerance), so no caption rides into the end card or its transition.

## `seed`

| Key | Type | Rule |
|-----|------|------|
| `seed` | mapping | Optional. `{}` or absent means no seeding. |
| `seed.backend` | string | `"rn-asyncstorage"`. Anything else fails before any device is touched, naming the supported backends. |
| `seed.entries[]` | list, ≥ 1 | `{ key, value }`. `key` is a non-empty string, unique across entries. |
| `seed.entries[].value` | string | Already serialised, exactly as the app stores it: a JSON app's `true` is `"true"`, a JSON string is `"\"premium\""`, an object is its JSON text. |

**`rn-asyncstorage` on iOS** (React Native AsyncStorage 2.x):
- Writes `<data container>/Library/Application Support/<bundle id>/RCTAsyncLocalStorage_V1/manifest.json`. A fresh install has no `Library/Application Support`, so it is created.
- Runs after `simctl uninstall` + `simctl install` and before the first `simctl launch`. `simctl install` over an existing install keeps the old data container, so capture always uninstalls first.
- A value longer than 1024 UTF-16 code units (NSString `length`, the library's inline threshold) goes to a file in the same directory. The file is named the lowercase MD5 hex of the UTF-8 key, and the manifest holds `null` for that key.
- Storage that already exists is refused (exit 65): the app has launched, or was seeded already.

## `capture`

| Key | Type | Rule |
|-----|------|------|
| `capture.ios.app` | string | Path to a Simulator build (`Release-iphonesimulator/<App>.app`). Its `CFBundleIdentifier` must equal `bundle_id`. |
| `capture.ios.bundle_id` | string | Reverse-DNS characters only (letters, digits, `-`, `.`): it becomes a directory name when seeding. |
| `capture.ios.device` | string | Optional. A Simulator name or UDID. Exactly one booted Simulator must match; with no value, exactly one must be booted. |
| `capture.ios.status_bar` | mapping | Optional. `time` (string, default `"9:41"`), `battery_level` (whole number 0–100, default 100). |
| `capture.ios.appearance` | string | Optional. `"light"` (default) or `"dark"`: the appearance each take starts in. |
| `capture.takes[]` | list, ≥ 1 | One recording each. |
| `capture.takes[].id` | string | Letters, digits, `.`, `_`, `-`; unique. It names the output files. |
| `capture.takes[].steps[]` | list, ≥ 1 | In order, inside the recording. Each step has exactly one key. |
| `… steps[].maestro` | string | Path to a Maestro flow file the consumer wrote. `label:` on a command becomes its cue name. |
| `… steps[].hold_s` | number > 0 | Keep recording, untouched. |
| `… steps[].appearance` | string | `"light"` or `"dark"`, flipped from the shell so it lands inside the take. |
| `capture.stills[]` | list | `id` (unique; letters, digits, `.`, `_`, `-`). `take` and `after_step` (1-based, ≤ that take's step count) come together: when to screenshot. A still with only an `id` is reported and skipped. |
| `capture.cut[]` | list, ≥ 1 | Segments played in order at 1.0x. |
| `… cut[].take` | string | A `capture.takes[].id`; every take in the cut must have the same pixel size. |
| `… cut[].in_s`, `out_s` | numbers | Take seconds. 0 ≤ `in_s` < `out_s` ≤ the take's logged `take_len_s`. |
| `… cut[].join` | string | The transition into the **next** segment: `"cut"` (default) or `"fade"`. The last segment has none. |
| `… cut[].fade_s` | number > 0 | Only with `"fade"`; shorter than both segments it joins. |
| `capture.compose` | mapping | Optional. `font`, `font_bold` (file paths), `background`, `text_color` (`"#RRGGBB"`; defaults `#F3EDE4`, `#1A1A1A`). |

The worked example is the front matter of [`example-campaign.md`](example-campaign.md): two takes, four segments, one crossfade, and the arithmetic in its "Capture and cut" section.

## Each take

On the one resolved Simulator (every `simctl` call takes its UDID; every Maestro call gets `--device <UDID>`):

**Before the first take:**
- **An installed app is refused.** If `simctl get_app_container <U> <bundle id> data` succeeds, the app is already on that Simulator, and every take would delete its data. The run exits 65 without touching the Simulator, unless `--replace-installed` is given. After a run with `--keep-app`, the next run needs the flag too.
- **The host clipboard is saved and cleared** (macOS, `pbpaste`/`pbcopy`), and restored on every exit, Ctrl-C and SIGTERM included. The restore is text only. When `osascript -e 'clipboard info'` lists anything besides `utf8`/`ut16`/`string`/`Unicode text` (an image, say), the run says that part will not come back. `--keep-clipboard` leaves the clipboard alone and says so.
- **Notifications:** `simctl` has no notification switch; `simctl help` lists none, and `simctl privacy` covers no notification service. A fresh install has never been granted notification permission, and the scripts send no `simctl push`. Other apps on the Simulator can still post; turn on a Focus in the Simulator's Settings if they do (manual, unverified from a script).

**Each take:**

1. `simctl terminate` + `uninstall` + `install`, then the seed backend, then `simctl pbcopy <U>` with empty input to clear the Simulator's own pasteboard.
2. `simctl status_bar <U> clear`, then `override --time … --dataNetwork wifi --wifiMode active --wifiBars 3 --cellularMode active --cellularBars 4 --operatorName '' --batteryState discharging --batteryLevel …`.
3. `simctl ui <U> appearance <starting appearance>`, `simctl launch`, a 1s settle.
4. `simctl io <U> recordVideo --codec=h264 --force`; no step runs until its log prints `Recording started`.
5. The steps, in order. A step that exits non-zero stops the recording, deletes the take, keeps a failing Maestro step's log as `takes/<take>.failed-step-N.log`, and fails the run.
6. The stop time is logged, then the recorder gets SIGINT. A file that `ffprobe` cannot read (an unfinalised recording) discards the take with recordVideo's exit status and log.

After every take, a separate pass per take with stills reinstalls, reseeds and replays the steps without recording. After each step that a still names, it runs `simctl io <U> screenshot --type=png`; a failed or empty screenshot stops the run with simctl's message.

On any exit, a trap undoes what the run changed: status bar cleared and the Simulator's previous appearance restored (once a take has started), the app uninstalled if the run installed it and `--keep-app` is absent, and the host clipboard restored. Maestro's stdin is closed.

`DEVELOPER_DIR=/Applications/Xcode.app/Contents/Developer` is exported for the script's own process tree only when `xcode-select -p` points at the Command Line Tools and that Xcode exists. A `DEVELOPER_DIR` already set is respected.

## The cue sheet — `<take>.cues.json`

```json
{
  "schema": "promo-campaign/cues-v1",
  "take": "scores", "file": "scores.mov",
  "take_len_s": 18.559,
  "file_dur_s": 16.612,
  "cues":   [{ "step": 1, "name": "tap +12", "command": "tapOnElement", "t_s": 5.245, "dur_s": 0.61, "status": "COMPLETED", "depth": 0 }],
  "events": [{ "step": 2, "kind": "hold_s", "value": 1.5, "t_s": 5.86, "dur_s": 1.503 },
             { "step": 3, "kind": "appearance", "value": "dark", "t_s": 16.026, "dur_s": 0.09 }]
}
```

- **`take_len_s`** is the stop time minus the `Recording started` time: the length the take really ran.
- **`file_dur_s`** is the file's video-stream duration. It is usually shorter (trap 1).
- **`cues`** come from each Maestro step's `--test-output-dir` `commands.json`: `t_s` = command timestamp − recording start, named by `label:` or else by the command. Maestro's two internal setup commands are dropped. `status` is kept as Maestro wrote it (`COMPLETED`, `SKIPPED`, `WARNED`, …). Maestro 2.10.0 was measured to stamp a `runFlow` skipped by an unmet `when:` and a failed `optional: true` assert with a timestamp and duration too. A command without them still gets a cue, with `t_s` and `dur_s` null, sorted last.
- **`events`** are every step: flows, holds and appearance flips.
- **All times are take seconds.** A cue marks when a command started, which comes before the visible change, so choose `in_s` from the cue and confirm it on a frame.

## Render arithmetic

```
footage   = Σ(out_s − in_s) − Σ fade_s        must equal Σ beats[].target_s within one frame (1.0x)
render    = footage + end_card.duration_s      must equal the plan within one frame, and ≤ max_duration_s
```

- **Segments:** `trim` + `setpts=PTS-STARTPTS` each. A `cut` join is `concat`, and a `fade` join is `xfade` overlapping both segments by `fade_s`.
- **Normalisation:** `settb=AVTB,fps=30` follows every trim and every join, so it sits immediately before every `concat`/`xfade` input.
- **Takes:** each is padded with `tpad=stop_mode=clone` to its `take_len_s` first.
- **End card:** its transition starts at the end of the footage, which is the end-card start. The footage's last frame is held for `transition_s` **before** captions are drawn, and `lib/contract.sh` refuses any caption ending after the end-card start, so no caption rides into the transition. The card, transition included, lasts `duration_s`.
- **Launch day:** `--launch-day` adds `launch_day` badges and writes `reel-<locale>.launch-day.mp4`, never `reel-<locale>.mp4`, and its summary line names the `do_not_post_before` dates.
- **Captions:** each is drawn for `[start_s, end_s)` on the output timeline, to the frame.
- **Window:** the phone window is the largest one with the source's aspect ratio that fits between the 250px caption band and the 200px footer, within 120px side margins, with even dimensions. A 1206x2622 source gives 676x1470+202+250, and a 1080x2220 source 716x1470+182+250.

**Self-checks that fail the run (exit 70):**
- The frame layer is `srgba` **and** its hole centre has alpha 0.
- Every caption block fits its band: height ≤ window top − 40, width ≤ 940.
- Every `out_s` ≤ its take's `take_len_s`.
- The filtergraph contains no `setpts` other than `PTS-STARTPTS`, and no `atempo`, `asetrate`, `itsscale`, `minterpolate` or `setrate`.
- footage = beats within one frame.
- The render's video-stream duration = plan within one frame, and ≤ `max_duration_s`.
- The render is 1080x1920.
- A render that has not ended within `PROMO_RENDER_TIMEOUT_S` is killed.

## Writing Maestro flows for capture

- **`label:` every command** you will cut on; the label becomes the cue name.
- **Tap by visible text, never by coordinates.** When the text repeats, a suggestion list for example, use a relative selector (`below:`).
- **Dismiss the keyboard with `pressKey: Enter`.**
- **Leave `launchApp` out.** Capture has already launched the seeded app.
- **Keep appearance flips out of flows.** Put them in `steps[]`, so they land inside the take.
- **Author against the app's real states.** An accessibility label can change after the first tap, and an identical second tap then fails.

```yaml
appId: com.example.tallybird
---
- extendedWaitUntil:
    visible: "Round 5"
    timeout: 15000
    label: "table ready"
- tapOn:
    text: "+12"
    below: "Chloe"
    label: "tap +12"
```

## Trap catalogue

Each trap below was measured on macOS with an iOS 26.5 Simulator, Xcode, Maestro 2.10.0, ffmpeg 9.0.1, ImageMagick 7.1.2 and `/bin/bash` 3.2.57, unless marked unverified.

1. **`recordVideo` writes no frames while the screen is static.** A take stopped 18.559s after `Recording started` was a 16.612s file: its trailing hold was gone. A segment past the end of the file is shortened silently, and ffmpeg exits 0. Hence the logged stop time, `tpad`, and the duration-equality check (≤ max alone passes a short render). Leading static time does survive.
2. **Recording starts about 0.11s after the command.** Drive only after `Recording started`.
3. **Every `maestro test` pays about 5.2s of startup** on a warm Simulator before its first command. Recording first leaves a lead-in, which the cut trims. `--no-reinstall-driver` saved nothing. Maestro's in-flow `startRecording` blocks about 2s and owns start and stop, so a shell appearance flip cannot land in its take.
4. **A cue precedes what it causes:** a tap whose command started at 5.245s changed the screen at 5.957s. A shell appearance flip that returned at take 16.026s crossed the luminance midpoint at 16.263s.
5. **A missing element costs about 17s** before Maestro exits 1, with the recording still running. The take is discarded.
6. **`maestro list-devices` lists device templates**, not connected devices. Resolve Simulators with `simctl list devices booted`. Without `--device`, Maestro can attach to a USB-connected phone.
7. **`xcode-select -p` pointing at the Command Line Tools** leaves `simctl` and Maestro's iOS driver without Xcode. That is what `DEVELOPER_DIR` fixes.
8. **concat re-stamps at 1/1000000 while a trimmed segment stays at 1/30.** Normalising only at the source makes xfade refuse: `First input link main timebase (1/1000000) do not match the corresponding second input link xfade timebase (1/30)`.
9. **An overlay of a looped image without `shortest=1` never ends.** The render runs until killed.
10. **An overlay's alpha must be real.** A canvas never given `-alpha set` keeps an opaque hole that hides the footage, and the bezel stroke still makes its channels read `srgba`. A PNG written without an alpha channel still reads alpha 0 at any pixel. So the frame check asserts both. On ImageMagick 7.1, `%[channels]` prints `srgba 4.0`.
11. **ffmpeg reads the caller's stdin** for its interactive keys. A render launched from a script consumed the rest of that script. Every ffmpeg call passes `-nostdin`.
12. **A take frame is not a clean still.** H.264 tv range turns white 255 into 252 (SSIM 0.9957, PSNR 36.7 dB against the lossless screenshot). Stills come from `simctl io screenshot`.
13. **ffmpeg 9 rejects `-of csv=s=' '`.** Query ffprobe fields one at a time.
14. **yq v4 evaluates a piped stdin** alongside its file argument, and `keys[] | select(...)` objects lost every field for 4 of 115 keys. Stdin is closed and keys are collected per mapping.
15. **Host leaks.** The Simulator syncs the host clipboard into its own pasteboard: after a run restored the host clipboard at exit, `simctl pbpaste` returned that text. So capture saves, clears and restores the host clipboard and empties the Simulator's pasteboard before each take. `pbpaste` returns 0 bytes for an image, so only text can be restored. Still unresolved: the QuickType predictive bar over the keyboard can show on camera, and `simctl` offers no way to silence notifications.
16. **`run_step … || rc=$?` switches `set -e` off inside the function**, and so does `$(func)`. A failed lookup left a step's kind empty and skipped it with status 0, and a raw string passed to `--argjson` dropped cue sheet events silently. Every command there is checked explicitly.

## Measured timings

| What | Measured |
|------|----------|
| `simctl install` of a Release React Native build | 0.26s |
| `recordVideo` start after the command | ≈ 0.11s |
| `maestro test` start to first command (warm Simulator) | 5.24–5.52s |
| A two-command flow (wait + tap), start to exit | 8.23s |
| Maestro `startRecording` block | ≈ 2.0s |
| Missing element until Maestro exits 1 | ≈ 17s |
| `pressKey: Enter` | 547ms |
| Shell appearance flip to visible midpoint | 0.24–0.35s after the command |
| Static tail lost by `recordVideo` | 1.9–2.0s on takes ending in a 2.5s hold |
| A whole `capture-ios.sh` run: one 12.4s take plus its still pass | 27s |

An end-to-end run on a React Native app went like this:
- A 2273-character seeded value spilled to its MD5-named file, and the app's first paint showed it.
- The take logged 12.447s against a 10.487s file (56 frames, variable rate).
- A 2-segment cut with a 0.3s fade and a 2.5s end card rendered 7.400000s against a 7.400000s plan.
- The dark flip crossed the window's luminance midpoint at output 3.067s against a predicted 3.052s, within one frame, so the footage played at 1.0x.

## Mutation record

Every guard was mutated on a copy of `scripts/`, so the shipped files never changed. Each run used a synthetic `testsrc2` 1206x2622 take of 16.6s with a hand-written cue sheet logging 18.559s. The fixture cut is 3 segments: 5.2–7.7 cut, 7.6–10.2 fade 0.3, 15.8–18.3. It has two captions and a 3.0s end card with a 0.5s crossfade. Plan: 7.3 + 3.0 = 10.3s.

| # | Mutation | Result |
|---|----------|--------|
| G0 | none | **green**: 10.300000s = plan, 309 frames, 1080x1920, window 676x1470+202+250 |
| M1 | frame build without `-alpha set` and `PNG32:` | **red** (70): `frame.png hole alpha at 540,985 is 1, expected 0` |
| M1a | without `-alpha set` only | **red** (70): same message |
| M1b | without `PNG32:` only | green: ImageMagick 7.1.2 keeps the channel on its own; `PNG32:` stays as insurance |
| M2 | `shortest=1` removed from every overlay | **red** (70): killed by the 60s perl-alarm watchdog |
| M3 | `tpad` removed from each take | **red** (70): `duration 6.066016s is not the planned 10.300000s within one frame` |
| M4 | normalisation only at the source | **red** (1): xfade refuses the 1/1000000 vs 1/30 timebases |
| M5 | out_s 18.3 → 18.6 (data) | **red** (70): `segments (7.9 s) minus crossfades (0.3 s) = 7.6 s, but beats sum to 7.3 s` |
| M6 | `setpts=0.5*PTS-STARTPTS` in a trim | **red** (70): `the filtergraph re-times frames` |
| M7 | segment 16.2–18.7 against a take logged at 18.559 (data) | **red** (70): `out_s 18.7 is past take "main"'s logged length` |
| M8 | a caption wrapping to 234px in a 210px band (data) | **red** (70): `caption … is 934x234, its band is 940x210` |
| M9 | M3 with the duration check disabled | green at 6.066016s: the duration check is what turns M3 red |
| C1 | one segment, no join | **green**: 5.500000s = plan |
| C2 | 1080x2220 source | **green**: window 716x1470+182+250, 7.000000s = plan |
| G1 | none, after all mutations | **green**: 10.300000s |

Parser fixtures, each checked by SKILL.md's Ruby check and by `lib/contract.sh` under ruby, under python3 with ruby hidden from PATH, and under yq v4 with both hidden:
- `locale: no`, `time: 9:41` and `in_s: 0:05.5` (block style) fail in all four, naming the same line; the scripts' paths also name the key path.
- A bare `no:` locale key fails in all four.
- A fully quoted fixture passes in all four.
- `seed.backend: "sqlite"` passes SKILL.md's check, which does not read `seed`, and fails all three script parsers with `supported backends: rn-asyncstorage`.

## Limitations

- **iOS Simulator only.** Android emulator capture and the 4:5 carousel composer are #78. SKILL.md reports each as not installed.
- **One pixel size per reel:** every take in a cut must come from the same device.
- **Default fonts are macOS paths.** Elsewhere, set them in `capture.compose` or the environment.
- **No web, desktop or physical-device capture.**
