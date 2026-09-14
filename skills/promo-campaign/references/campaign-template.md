---
# promo-campaign/v1 — blank instance file.
# Copy to docs/marketing/campaign.md in the app repo, then fill it in through
# /promo-campaign rather than by hand. The rules for every key are in SKILL.md
# under "Instance-file contract"; references/example-campaign.md is a filled-in
# EXAMPLE that passes every limit.
#
# Parser-proof YAML: quote every string value and every locale key, write every
# time as decimal seconds (5.5, never 0:05.5), and use only true / false / null.
schema: "promo-campaign/v1"

app:
  name: "<app name>"
  icon: "<repo path to the app icon>"
  tagline:
    "<locale>": "<verbatim line from that locale's claim authority>"

# Optional: the platforms this plan was made for.
platforms: ["<platform>"]

locales: ["<locale>"]

# Hard ceiling 20.0. It may be lowered, never raised.
max_duration_s: 20.0

claim_sources:
  # The ONLY sources a caption or slide may quote: the current store listing or landing page.
  authority:
    - kind: "store_listing"                   # "store_listing" | "landing_page"
      store: "<app_store | google_play | other store id>"
      locale: "<locale>"
      path: "<repo path to the listing text>"  # and/or url
      confirmed_live: "<YYYY-MM-DD>"          # null until confirmed
  # Where features are found. Never backs a caption.
  discovery: ["CHANGELOG.md", "README.md"]

features:
  - id: "<slug>"
    title: "<short feature title>"
    rank: 1                                   # positive whole number; null when unclaimed
    visual: true                              # visible on screen within ~3s, no narration
    status: "claimed"                         # "claimed" | "unclaimed"
    claim:
      "<locale>": "<verbatim quote>"          # null when unclaimed
    found_in: "<discovery path, for unclaimed features>"

hook:
  feature: "<id of the highest-ranked claimed feature with visual: true>"
  beat: "<id of the beat with role hook>"
  moment: "<the action and its visible effect, in one shot>"
  lands_at_s: 0.0                             # proof frame on the output timeline; <= 5.0

# Storyboard intent. Beats play back to back from 0.0, and the end card starts
# at their sum. Take-level segments (in/out points, framing) belong to `capture`.
beats:
  - { id: "<slug>", role: "setup", feature: null, shows: "<what is on screen>", target_s: 0.0 }
  - { id: "<slug>", role: "hook", feature: "<feature id>", shows: "<what is on screen>", target_s: 0.0 }
  - { id: "<slug>", role: "proof", feature: "<feature id>", shows: "<what is on screen>", target_s: 0.0 }

# Output-timeline times. One idea each, >= 2.0s each, <= 8 per locale, exactly
# one emphasis: true (the hook's), none ending after the end-card start.
captions:
  "<locale>":
    - { start_s: 0.0, end_s: 0.0, text: "<caption>", emphasis: true, claim: "<verbatim quote>" }

# Content slides in order; the end card is appended after them. <= 8, slide 1 is
# the hook's with the only emphasis: true. `still` names an id under capture.stills.
carousel:
  "<locale>":
    - { still: "<still id>", text: "<slide text>", emphasis: true, claim: "<verbatim quote>" }

end_card:
  duration_s: 0.0                             # <= 3.0, transition included
  transition_s: 0.0
  badges:
    # Only when public (the user confirmed the install signed-out, today) or launch_day with a date.
    - { store: "<store id>", locale: "<locale>", path: "<repo path to official badge artwork>", public: false, launch_day: false, do_not_post_before: "<YYYY-MM-DD, required when launch_day>" }

outputs:
  dir: "<repo-relative dir without .., git-ignored>"
  formats: ["reel_9x16", "carousel_4x5"]      # 1080x1920 · 1080x1350

# Filled in at Phase 7, with the user. While planning, declare still ids only.
# Key rules: SKILL.md's contract table. Full schema: references/capture-and-compose.md.
seed:                                         # written before the app's first launch
  backend: "rn-asyncstorage"                  # the only backend so far
  entries:
    - { key: "<storage key>", value: "<the value exactly as the app stores it, already serialised>" }
capture:
  ios: { app: "<repo path to a simulator .app>", bundle_id: "<bundle id>", device: "<Simulator name or UDID>", status_bar: { time: "9:41", battery_level: 100 }, appearance: "light" }
  takes:
    - id: "<take id>"
      steps:                                  # in order, inside one recording
        - maestro: "<repo path to a Maestro flow>"
        - hold_s: 1.0
        - appearance: "dark"
  stills:
    - { id: "<still id>", take: "<take id>", after_step: 1 }
  cut:                                        # take seconds, played in order at 1.0x
    - { take: "<take id>", in_s: 0.0, out_s: 0.0, join: "fade", fade_s: 0.3 }   # join: into the next segment
    - { take: "<take id>", in_s: 0.0, out_s: 0.0 }                               # the last segment has no join
  compose: { background: "#F3EDE4", text_color: "#1A1A1A" }   # optional; font and font_bold are file paths
---

# <App> Promo Campaign

## Claim authority
<!-- store · locale · source · confirmed live on, matched against the signed-out listing -->

## Features
<!-- rank · feature · visual/not visual · verbatim claim · source. Then the unclaimed list, each with its decision. -->

## Hook
<!-- why this moment proves the highest-ranked visual claim, and how setup stays under 5.0s -->

## Storyboard
<!-- beat table, then the arithmetic block: beats sum, end card, planned total, hook, captions, overlap, carousel -->

## Carousel
<!-- slide · still · text -->

## Claim map

## Badges
<!-- store · locale · public (checked signed-out on) · launch_day · do_not_post_before · shown -->

## Starting state for capture
