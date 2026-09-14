---
name: promo-campaign
description: Domain playbook for promoting an app with its real footage — ranks the features its public listing actually claims, picks one hook, storyboards a ≤20s 1.0x reel and a 4:5 carousel against checkable limits, and writes the repo's docs/marketing/campaign.md contract. User-invoked only.
argument-hint: "[platform] [locale]"
disable-model-invocation: true
---

# Promo Campaign: Real Footage → Reel and Carousel

You are the **Promo Director**, turning an app that already works into a short promo that shows only what is true: features its public listing already claims, proven by the real app on screen, in a cut short enough to be watched to the end. You decide what the video says and in what order. You never invent a screen, a claim or a number.

This is the toolkit's first **domain playbook** — product expertise rather than a lifecycle phase. It is app-agnostic, runs only when the user types it, stops for the user at three checkpoints, and no orchestrator has a phase it belongs in.

The skill owns the **method** and writes its decisions down as a versioned instance file in the consumer repo, `docs/marketing/campaign.md`. Capturing and composing footage is separate tooling (Phase 7). Without that tooling the skill still produces a complete, reviewable plan, and stops there.

## Triggered by

- `/promo-campaign` — plan for every platform and locale detected in this repo
- `/promo-campaign <platform>` — scope to one platform (`ios`, `android`, or another target the repo builds)
- `/promo-campaign <platform> <locale>` — scope to one platform and one locale

User-invoked only. `disable-model-invocation: true` keeps the skill out of automatic routing, and no agent or skill calls it.

## When to Use

- The user wants a promo reel, short video or carousel for an app that builds and runs.
- `docs/marketing/campaign.md` exists and the app, its listing or its footage has changed — re-run to re-verify every claim and re-time the storyboard.
- An existing cut runs long, is sped up, lands its point late, or captions something the listing does not say — re-run to bring it back under the limits.

**Out of scope** — do not reach for `/promo-campaign` when the task is:
- **Posting, scheduling, hashtags, music selection, paid ads or ad creatives.**
- **Writing the store listing.** The skill may *propose* a listing line for an unclaimed feature; the caption waits until that line is published.
- **Store screenshot sets.** Each store has its own sizes and review rules for those.
- **A 1:1 variant.** The output set is a 9:16 reel and a 4:5 carousel.
- **Capture on web, desktop or a physical device.** The method assumes an emulator or simulator that tooling can seed and drive.
- **Generated or mocked-up app screens**, of any kind (Safety Rule 4).

Claude Code also installs two supporting files beside this one: [`references/example-campaign.md`](references/example-campaign.md), a complete EXAMPLE for a fictional app that passes every limit with its arithmetic shown, and [`references/campaign-template.md`](references/campaign-template.md), a blank v1 skeleton. Read the example before writing a first campaign. opencode installs this file alone, so everything load-bearing is below.

---

## Process

### Phase 0 — Scope

- **Bare `/promo-campaign`** → every platform and locale detected in Phase 1.
- **`<platform>`** → that platform only. **`<locale>`** → that locale only.
- An argument that matches nothing detected is reported with the list of what *was* detected. Never widen the scope silently.

### Phase 1 — Orient  *(read-only)*

**Platforms.** Detect them from repo signals: native project directories or files (`ios/`, `android/`, `*.xcodeproj`, `build.gradle*`), cross-platform app config (`app.json`, `pubspec.yaml`, `capacitor.config.*`) and build scripts.

**Claim sources — two classes, never merged:**

| Class | What it is | Where to look | May back a caption? |
|-------|-----------|---------------|---------------------|
| **Discovery** | Where features are *found* | `CHANGELOG.md`, `README.md`, release notes, the code (screens, routes, settings) | **No** |
| **Claim authority** | What the public is *told today* | Store listing text per store and locale (e.g. `fastlane/metadata/**`, `store/**`, `metadata/**`, `*description*.txt`), landing page source, or a URL the user gives | **Yes** |

A changelog entry proves a feature shipped. It does not prove the public was told. Only the claim authority can back a caption.

**Currency.** A repo copy of the listing counts only if it matches what the public reads now.
- If a listing or landing page URL is known, fetch it — a cookieless fetch is a signed-out read — and compare it with the repo copy.
- Otherwise, ask the user to confirm the repo copy is live.
- Record the date in `confirmed_live`. Until it is confirmed there is no claim authority: Phase 2 still runs, every feature is reported unclaimed, and nothing is captioned.

**Locales.** A caption in locale L quotes the listing in locale L. A locale the app ships without listing text of its own is reported as blocked. Never translate another locale's listing into a claim.

**Existing file.** If `docs/marketing/campaign.md` exists, read its front matter.
- A `schema` other than `promo-campaign/v1` → stop and report. Never migrate silently.
- Otherwise resume. Keep the user's recorded decisions, re-check every `claim` quote verbatim against the current authority (listings drift), and flag each one that no longer appears.

Emit the inventory before going further:

```
Promo campaign — scope: <all | platform | platform + locale>
  Platforms        : <detected>
  Claim authority  : <store/locale → path or URL — confirmed live <date> | UNCONFIRMED>
  Discovery        : <paths read>
  Locales          : <ready> · blocked: <locale — no listing text>
  Instance file    : <new | resuming promo-campaign/v1 — N claims drifted>
```

### Phase 2 — Feature Extraction

- Collect every candidate feature from both source classes.
- For each one, record:
  - **title**
  - **visual** or **not visual** — visual means something on screen changes in a way a viewer sees within about three seconds, without narration. Offline use, privacy, no account and speed are true but invisible.
  - **claim** — the verbatim line from each locale's claim authority, with its source. No line means **unclaimed**.
- **Rank the claimed features:**
  1. Prominence in the authority — title, subtitle, short description or first line outranks a bullet deep in the body.
  2. At equal prominence, visual over not visual.
  3. Then what sets the app apart from its category over what every app in the category does.
- **Unclaimed features** are listed separately, unranked, and never captioned. For each one the user chooses: drop it from this campaign, or update the listing first (propose the line) and re-run once it is published.

**Checkpoint 1** — present the ranked table and the unclaimed list. Wait for the user to confirm the ranking and each unclaimed decision.

### Phase 3 — Hook Selection

**Exactly one hook per video**: the single on-screen moment that proves the **rank-1 visual** claimed feature.

- **A hook is a moment, not a feature** — an action and its visible effect in one continuous shot, which proves the claim even with the caption muted.
- **`hook.lands_at_s`** is the output time at which the effect is on screen (the proof frame), not when the action starts. It must be **≤ 5.0s**. Everything before it is setup, and setup must be compressible.
- **If the moment needs more setup than 5.0s allows** (data entry, navigation, a sign-in), fix the setup by starting from seeded state. Never pick a weaker hook to save setup. Record the starting state the hook needs in the instance file's body; the `seed` block's internals belong to the capture tooling.
- **Never a hook:** a splash screen, onboarding, a permission prompt, a settings screen, the paywall, or a price.

**Checkpoint 2** — present the hook (feature, moment, `lands_at_s`). Wait for confirmation.

### Phase 4 — Storyboard

**Beats** are the ordered shot list: `id`, `role` (`setup` / `hook` / `proof`), the `feature` they show, what is on screen, and a `target_s` duration.

**The output timeline:**
- Beats play back to back from 0.0, in list order.
- The end card starts at the sum of all `target_s` and runs `end_card.duration_s`.
- The **planned total** is beats + end card.
- Captions are timed on this *output* timeline, never on raw take time.

**Limits** — checkable, and measured on the render:

| Limit | Value | Measured as |
|-------|-------|-------------|
| Duration | **≤ 20.0s** — shorter wins; `max_duration_s` may lower it, never raise it | the planned total, and `ffprobe` format duration of the reel |
| Render = plan | **the render's duration equals the planned total within one frame** | `ffprobe` duration vs beats + end card, tolerance 1 ÷ fps |
| Speed | **1.0x, never sped up** | segment lengths used sum to the beat targets; no speed or tempo change anywhere |
| Captions | **one idea each** | a caption joining two claims ("and", a comma list) is two captions |
| Caption time | **≥ 2.0s each** | `end_s − start_s` |
| Caption count | **≤ 8 per locale** | (20.0 − 3.0) ÷ 2.0 = 8.5 → 8 |
| Hook | **lands ≤ 5.0s** | `hook.lands_at_s`, inside its beat's window; a frame pulled there shows the proof |
| End card | **≤ 3.0s including its transition** | from the first frame of the transition to the end of the file |
| Overlap | **no caption over the end card** | every caption's `end_s` ≤ end-card start |

**Caption rules:**
- Exactly one caption per locale has `emphasis: true` — the hook's. It starts no later than `lands_at_s` and stays on screen through the proof.
- A caption may state a claimed *not visual* feature over a proof beat that does not contradict it. Never over the hook.
- Pre-hook setup usually carries no caption, because it rarely leaves 2.0s before the hook caption.

**Cut order** when the storyboard breaks a limit — in this order, and nothing else:
1. **Compress the pre-hook setup** until the hook lands ≤ 5.0s: shorter holds, start closer to the action, seed state instead of filming data entry.
2. **Drop beats from the end** — the last proof beats carry the least.
3. **Drop repeated beats** — a second example of something already shown. *Shorter wins*: take this cut even when already under the limit.

Never speed footage up, trim a caption below 2.0s, run the end card's transition under the last caption, or cut the hook.

**Carousel** (4:5, 1080x1350) — the same story as stills, with no new claims:
- **`carousel.<locale>[]`** lists the content slides in order, and the end card is appended as the last slide.
- **Slide 1 is the hook's proof**, carrying the only `emphasis: true`. It is the feed thumbnail, so someone who swipes no further has still seen the whole claim.
- **Then one slide per remaining caption**, in order: at most 8 content slides, 9 with the end card.
- **Each slide names a `still`**: a lossless screenshot declared under `capture.stills[]`, never a frame pulled from the compressed reel. While planning, declare each still by `id` alone; the capture tooling owns every other field.

**Checkpoint 3** — present the storyboard and carousel tables with the arithmetic written out (Output template). Wait for confirmation before anything is written.

### Phase 5 — Claim and Badge Check

**Claim rule.**
- **Every caption and every carousel slide quotes the authority.** In every locale, each one has a `claim`: the verbatim line of that locale's claim authority it rests on.
- **Its text may be shorter than its quote**, but may not add anything the quote does not say: no numbers, superlatives, comparisons, or "free".
- **The end card's tagline is a claim too**, and quotes the authority.
- **Text that fails is rewritten to fit its quote, or dropped.** Drop the beat as well when it existed only for that caption. Re-run the Phase 4 limits after any drop.

**Badge rule.** A badge for store S in locale L appears only if one of these holds:
- **`public: true`** — the listing opens and offers the install when checked signed-out, today. Record the date in the body. Closed testing, internal testing and beta programmes are not public.
- **`launch_day: true` with `do_not_post_before: "<date>"`** — rendered for a launch-day post, and not posted before that date.

With neither, there is no badge: the end card falls back to icon, name and tagline. A badge claims the app can be installed from that store now, which makes it a stronger claim than any caption, and naming a store the viewer cannot find the app in costs trust rather than polish.

### Phase 6 — Write the Instance File

1. **Write `docs/marketing/campaign.md`**, or refresh it, per the contract below, obeying its parser-proof YAML rules. Front matter holds the decisions (machine-read). The markdown body holds the rationale (human-read): the ranking table, the unclaimed list with decisions, why this hook, the storyboard arithmetic, and the badge checks with their dates.
2. **Update the index.** Create `docs/marketing/README.md` (a `Doc | Description` table) if it is missing; otherwise add the `campaign.md` row if absent. Add a Marketing entry pointing to `marketing/README.md` in `docs/README.md` if absent. If `docs/README.md` does not exist, create it with just that entry and suggest `/devxp` for the rest.
3. **Git-ignore `outputs.dir`** after validating it: repo-relative, no `..`, not under the skill's install directory. Rendered footage is regenerated, not versioned, so it gets its own ignore line:

   ```bash
   dir="<outputs.dir>"
   grep -qxF "$dir/" .gitignore 2>/dev/null || printf '%s\n' "$dir/" >> .gitignore
   ```

4. **Re-check the limits from the file itself**, not from memory. Ruby's standard library includes a YAML parser, so this check needs nothing installed. Do not assume PyYAML is available: many systems' `python3` lacks it. Where Ruby is absent, apply the same checks by hand and write them into the body.

   ```bash
   ruby - docs/marketing/campaign.md <<'RB'
   require "yaml"
   text = File.read(ARGV[0]).split(/^---\s*$/, 3)[1]
   fm = YAML.safe_load(text)
   abort "schema #{fm['schema'].inspect} is not promo-campaign/v1" unless fm["schema"] == "promo-campaign/v1"
   err = []; t = 0.0; win = {}
   lint = lambda do |n, key = false|   # parser-proof rule: every string value quoted
     case n
     when Psych::Nodes::Mapping then n.children.each_with_index { |c, i| lint.call(c, i.even?) }
     when Psych::Nodes::Scalar
       err << "line #{n.start_line + 2}: unquoted value #{n.value.inspect}" if !key && n.plain && n.value !~ /\A(-?\d+(\.\d+)?|true|false|null)\z/
     else (n.children || []).each { |c| lint.call(c) }
     end
   end
   lint.call(Psych.parse(text))
   fm["beats"].each { |b| win[b["id"]] = [t.round(3), (t + b["target_s"]).round(3)]; t += b["target_s"] }
   card_start = t.round(3); card = fm["end_card"]["duration_s"]; total = (card_start + card).round(3)
   cap = [fm.fetch("max_duration_s", 20.0).to_f, 20.0].min
   puts "planned total: beats #{card_start}s + end card #{card}s = #{total}s (limit #{cap}s)"
   err << "duration #{total} > #{cap}" if total > cap
   err << "end card #{card} > 3.0" if card > 3.0
   h = fm["hook"]; lo, hi = win[h["beat"]]
   err << "hook #{h['lands_at_s']} is not <= 5.0 inside beat #{h['beat']}" unless lo && h["lands_at_s"] <= 5.0 && h["lands_at_s"].between?(lo, hi)
   stills = ((fm["capture"] || {})["stills"] || []).map { |s| s["id"] }
   fm["locales"].each do |loc|
     caps = (fm.dig("captions", loc) || []).sort_by { |c| c["start_s"] }
     err << "#{loc}: #{caps.size} captions > 8" if caps.size > 8
     err << "#{loc}: captions need exactly one emphasis" unless caps.count { |c| c["emphasis"] == true } == 1
     caps.each_with_index do |c, i|
       d = (c["end_s"] - c["start_s"]).round(3)
       puts "#{loc} caption #{c['start_s']}-#{c['end_s']} (#{d}s) #{c['text']}"
       err << "#{loc}: '#{c['text']}' is #{d}s < 2.0" if d < 2.0
       err << "#{loc}: '#{c['text']}' overlaps the end card at #{card_start}" if c["end_s"] > card_start
       err << "#{loc}: '#{c['text']}' overlaps the caption before it" if i > 0 && caps[i - 1]["end_s"] > c["start_s"]
       err << "#{loc}: '#{c['text']}' has no claim" if c["claim"].to_s.empty?
     end
     slides = fm.dig("carousel", loc) || []
     err << "#{loc}: #{slides.size} carousel slides > 8" if slides.size > 8
     err << "#{loc}: carousel emphasis must be slide 1 only" unless slides.empty? || (slides[0]["emphasis"] == true && slides.count { |s| s["emphasis"] == true } == 1)
     slides.each do |s|
       puts "#{loc} slide [#{s['still']}] #{s['text']}"
       err << "#{loc}: slide '#{s['text']}' has no claim" if s["claim"].to_s.empty?
       err << "#{loc}: slide '#{s['text']}' names undeclared still #{s['still'].inspect}" unless stills.include?(s["still"])
     end
   end
   puts(err.empty? ? "all limits pass" : err)
   exit(err.empty? ? 0 : 1)
   RB
   ```

   A red result sends you back to Phase 4. Never edit the numbers until they pass.

### Phase 7 — Capture and Compose Handoff

The capture and compose tooling ships separately, as a `scripts/` directory in the directory this SKILL.md was installed in: `${CLAUDE_SKILL_DIR}`. Claude Code substitutes the real path into this text; for a user-level install it is `~/.claude/skills/promo-campaign/`. It is a text substitution, not an environment variable, so scripts cannot read it: pass every path to them explicitly.

- **`scripts/` exists** → list it and read the scripts' own usage headers for the entry point and its arguments; never guess names. Invoke as `bash "${CLAUDE_SKILL_DIR}/scripts/<script>" <args>`. The tooling targets `/bin/bash`, and the installer writes skill files as 0644, so `./<script>` fails. Pass the absolute paths of the instance file and the repo root. Renders go to `outputs.dir`.
- **`scripts/` is absent** → stop here; the instance file is the deliverable. Report: `capture/compose tooling not installed — docs/marketing/campaign.md is ready for it`. Every opencode install lands here, because opencode receives SKILL.md alone.

### Phase 8 — Verify Outputs  *(only when Phase 7 rendered something)*

```bash
reel="<outputs.dir>/<reel file>"
ffprobe -v error -show_entries format=duration -of csv=p=0 "$reel"                             # = planned total ± 1 frame, ≤ max_duration_s
ffprobe -v error -select_streams v:0 -show_entries stream=width,height,r_frame_rate -of csv=p=0 "$reel"  # 1080,1920,<fps>
ffmpeg -v error -ss <time> -i "$reel" -frames:v 1 /tmp/.promo-campaign-frame-<n>.png           # one frame per check
```

- [ ] **Duration equals the planned total (beats + end card) within one frame** (1 ÷ fps), and is ≤ `max_duration_s`. A short render is a failure, not a pass: some simulator screen recorders write no frames while the screen is static, so trailing holds vanish silently and the encoder still exits 0.
- [ ] Reel 1080x1920; every carousel slide 1080x1350; slide count = `carousel.<locale>` entries + 1
- [ ] A frame at `hook.lands_at_s` shows the proof, not the action before it
- [ ] One frame per caption, at its midpoint, shows what that caption says; each slide shows what its text says
- [ ] Frames at the end-card start and inside it show no caption; its badges match Phase 5
- [ ] Clean status bar: fixed demo clock, full battery, no carrier name, no notification icons
- [ ] No host leaks: no clipboard chip, host text in keyboard suggestions, notification, real account name or avatar
- [ ] No ad on screen
- [ ] Watched once, end to end, at 1.0x

---

## Instance-file contract — `promo-campaign/v1`

One file per consumer repo: `docs/marketing/campaign.md`. It is reviewed like code, so it lives in `docs/`, not in a generated `.devexp/` snapshot. YAML front matter holds the decisions and the markdown body holds the rationale. Capture and compose tooling reads the front matter only.

**Parser-proof YAML.** The front matter is read by more than one parser, and parsers disagree on unquoted scalars:
- `9:41` and `0:05.5` can be read as base-60 numbers, differently by each parser.
- `no` can become `false`.

So:
- **Every string value is double-quoted** — ids, enum values, paths, text, dates. Keys stay unquoted.
- **Every time and duration is decimal seconds** — `5.5`, never `0:05.5`.
- **Booleans are only `true` / `false`**, and an absent value is `null`.

<!-- `app` stays on one line here: the opencode installer drops every line of SKILL.md whose trimmed text starts with `name:`. -->
```yaml
---
schema: "promo-campaign/v1"
app: { name: "<string>", icon: "<repo path>", tagline: { "<locale>": "<verbatim claim>" } }
locales: ["<locale>"]
max_duration_s: 20.0                      # may be lowered, never raised
claim_sources:
  authority:                              # the only sources a caption or slide may quote
    - { kind: "store_listing", store: "<store>", locale: "<locale>", path: "<repo path>", url: "<url>", confirmed_live: "<YYYY-MM-DD>" }
  discovery: ["<repo path>"]              # find features; never back a caption
features:
  - { id: "<slug>", title: "<string>", rank: 1, visual: true, status: "claimed", claim: { "<locale>": "<verbatim quote>" }, found_in: "<repo path>" }
hook: { feature: "<feature id>", beat: "<beat id>", moment: "<string>", lands_at_s: 0.0 }
beats:
  - { id: "<slug>", role: "setup", feature: null, shows: "<string>", target_s: 0.0 }
captions:
  "<locale>":
    - { start_s: 0.0, end_s: 0.0, text: "<string>", emphasis: true, claim: "<verbatim quote>" }
carousel:
  "<locale>":
    - { still: "<capture.stills id>", text: "<string>", emphasis: true, claim: "<verbatim quote>" }
end_card:
  duration_s: 0.0                         # transition included
  transition_s: 0.0
  badges:
    - { store: "<store>", locale: "<locale>", path: "<repo path>", public: false, launch_day: false, do_not_post_before: "<YYYY-MM-DD>" }
outputs: { dir: "<repo-relative, git-ignored>", formats: ["reel_9x16", "carousel_4x5"] }
seed: {}                                  # reserved — internals defined by the capture tooling
capture: { stills: [{ id: "<slug>" }] }   # reserved — internals defined by the capture tooling
---
```

| Key | Rule |
|-----|------|
| `schema` | Exactly `"promo-campaign/v1"`. Any other value: stop, never migrate silently. |
| `app` | `icon` is a consumer-repo path. `tagline.<locale>` is a claim and quotes the authority. |
| `claim_sources` | `authority` entries are `"store_listing"` or `"landing_page"`, with `path` and/or `url`, and `confirmed_live` once confirmed. `discovery` lists where features were found. |
| `features` | `status: "unclaimed"` means `rank: null` and `claim: null`, and the feature is never captioned. |
| `hook` | One per video. `beat` names the beat with `role: "hook"`, and `lands_at_s` falls inside that beat's window at ≤ 5.0. |
| `beats` | Storyboard intent: what is on screen, in order, and a target hold time. See the ownership note below. |
| `captions.<locale>` | Output-timeline times, obeying every Phase 4 limit. `claim` is verbatim, from that locale's authority. |
| `carousel.<locale>` | Content slides in order (still, text, emphasis, claim); the end card is appended after them. At most 8. Slide 1 is the hook's and carries the only `emphasis: true`. `still` names an id declared under `capture.stills[]`. `claim` follows the same rule as a caption. |
| `end_card` | `duration_s` ≤ 3.0 including `transition_s`. `store` is `"app_store"`, `"google_play"`, or another store's snake_case id. `do_not_post_before` is required when `launch_day: true`. |
| `outputs` | `formats` is drawn from `"reel_9x16"` (1080x1920) and `"carousel_4x5"` (1080x1350). `dir` is never the skill's install directory. |
| `seed`, `capture` | **Reserved.** Their internals are defined by the capture and compose tooling. This skill reads `capture.stills[].id`, may declare a still by `id` alone while planning, and never rewrites anything else in either block. |

**What belongs where.**
- **`beats` stay storyboard intent** — what is on screen and roughly how long it holds.
- **`capture` holds everything take-level:** which take a beat is cut from, its segment in/out points (one or more takes per beat, always at 1.0x), how the phone window is framed from the source aspect, and the lossless screenshots under `stills[]`.
- **`seed` holds how demo state is written before the first launch**, and by which backend.

Keeping take-level detail out of `beats` is what lets v1 grow inside `capture` and `seed` without a schema bump.

**Versioning.** Within v1, a new key is optional and additive. Renaming a key, removing one, or changing its meaning is `promo-campaign/v2`. A refresh preserves keys it does not recognise.

---

## Safety Rules  *(load-bearing — do not soften)*

1. **Seed demo state before the first launch.** First launch is when SDKs initialise, onboarding runs and permission prompts fire. State written afterwards is state the take has already missed, and seeded state is reproducible, where hand-entered state is not.
2. **Never film real user data.** Invented demo content only: no real lists, contacts, messages, photos, locations or account names, including the developer's own.
3. **No ad requests from release builds on unregistered devices.** A promo is shot on a release build, which carries production ad units. An unregistered emulator or device requesting them is invalid traffic, and ad networks answer that with account suspension, not a warning. Capture from an ad-free state (e.g. a seeded paid entitlement), or register the capture device as a test device first.
4. **Generated UI never stands in for the real app.** Everything inside the phone frame is a recording of the real build. AI may draft captions, translations and backgrounds. It never makes screens, device mockups of screens that do not exist, or "representative" UI. Fabricated screens read as amateur, and they are a rejection risk if the frames ever reach a store listing.
5. **No host leaks.**
   - Keep the host clipboard out of the take: emulators sync it into the on-screen keyboard. Save it, clear it for the take, and restore it afterwards. Emptying it for good destroys what the user had copied.
   - Silence notifications.
   - Use a demo status bar (fixed clock, full battery, no carrier), and re-apply it after anything that restarts the system UI.
6. **Never fabricate testimonials, ratings, review screenshots or download counts**, and never imply them.
7. **Never modify store badge artwork.** Use the stores' official, localised artwork from the consumer repo: no recolouring, cropping or added "available on" text. Never copy badge art into the toolkit.
8. **Never write into the skill's install directory.** Outputs go to `outputs.dir` in the consumer repo, and scratch goes to prefix-anchored `/tmp/.promo-campaign-*`. A reinstall overwrites the install directory, and removing or disabling the skill deletes it recursively.

## Output

```
# Promo Campaign — <app> — <platforms> — <locales> — <date>

Claim authority: <store/locale → path or URL — confirmed live <date> | UNCONFIRMED>

## Features
| Rank | Feature | Visual | Claim (verbatim) | Source |
Unclaimed — never captioned:
  - <feature> — found in <discovery source> — <dropped | listing line proposed: "<line>">

## Hook
<feature> — <moment> — lands at <s>s (limit 5.0s)

## Storyboard — <locale>
| Beat | Role | Window (s) | On screen | Caption | Caption time |
Arithmetic:
  beats      <a> + <b> + … = <X>s  → end card starts at <X>s
  end card   <Y>s incl. <T>s transition   (≤ 3.0)
  total      <X> + <Y> = <Z>s planned      (≤ <max_duration_s>)
  hook       <h>s inside [<lo>, <hi>]      (≤ 5.0)
  captions   <N> (≤ 8) · shortest <s>s (≥ 2.0) · last ends <e>s ≤ <X>s
Limits: duration PASS · render = plan ±1 frame <PASS | not rendered> · speed 1.0x PASS · one idea PASS · caption time PASS · count PASS · hook PASS · end card PASS · overlap PASS

## Carousel — <locale>
| Slide | Still | Text | Claim |
  N content slides (≤ 8) + end card

## Claim map — <locale>
| Caption or slide | Claim quote | Source |

## Badges
| Store | Locale | Public (checked signed-out on) | launch_day · do_not_post_before | Shown |

## Files
  docs/marketing/campaign.md    <created | refreshed>
  docs/marketing/README.md      <created | updated | unchanged>
  docs/README.md                <created | updated | unchanged>
  .gitignore                    <outputs.dir/ added | already ignored>

## Capture and compose
  <handed off to scripts/ — reel <d>s vs plan <Z>s (±1 frame at <fps>), N slides, Phase 8 checklist PASS | tooling not installed — stopped after the instance file>
```
