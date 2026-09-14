---
# EXAMPLE — Tallybird is a FICTIONAL board-game score keeper, invented for this
# reference. It has no store listing, no app id and no URL, and every path below
# is illustrative. Copy the shape, never the content.
schema: "promo-campaign/v1"
app:
  name: "Tallybird"
  icon: "assets/icon-1024.png"
  tagline:
    "en": "Score any game night."
    "es": "Puntúa cualquier noche de juegos."
platforms: ["ios"]
locales: ["en", "es"]
max_duration_s: 20.0
claim_sources:
  authority:
    - { kind: "store_listing", store: "app_store", locale: "en", path: "store/app-store/en/description.txt", confirmed_live: "2026-09-01" }
    - { kind: "store_listing", store: "app_store", locale: "es", path: "store/app-store/es/description.txt", confirmed_live: "2026-09-01" }
  discovery: ["CHANGELOG.md", "README.md", "src/"]
features:
  - id: "live-totals"
    title: "Running totals"
    rank: 1
    visual: true
    status: "claimed"
    claim:
      "en": "Tap a score and every total adds itself up."
      "es": "Toca una puntuación y cada total se suma solo."
  - id: "leader-ranking"
    title: "Leader on top"
    rank: 2
    visual: true
    status: "claimed"
    claim:
      "en": "The leader always rises to the top of the table."
      "es": "El líder siempre sube a lo alto de la tabla."
  - id: "round-history"
    title: "Round history"
    rank: 3
    visual: true
    status: "claimed"
    claim:
      "en": "Every round is saved to a history you can scroll back through."
      "es": "Cada ronda queda guardada en un historial que puedes repasar."
  - id: "rematch"
    title: "One-tap rematch"
    rank: 4
    visual: true
    status: "claimed"
    claim:
      "en": "Start a rematch with the same players in one tap."
      "es": "Empieza la revancha con los mismos jugadores con un solo toque."
  - id: "offline"
    title: "Works offline"
    rank: 5
    visual: false
    status: "claimed"
    claim:
      "en": "Works offline, at any table."
      "es": "Funciona sin conexión, en cualquier mesa."
  - id: "csv-export"
    title: "CSV export"
    rank: null
    visual: true
    status: "unclaimed"
    claim: null
    found_in: "CHANGELOG.md"
hook:
  feature: "live-totals"
  beat: "hook-tap"
  moment: "Tapping +12 on the third player counts their total up from 31 to 43, with no sum typed."
  lands_at_s: 2.4
beats:
  - { id: "setup-table", role: "setup", feature: null, shows: "Four players mid-game, round 5, totals visible", target_s: 1.2 }
  - { id: "hook-tap", role: "hook", feature: "live-totals", shows: "+12 on the third player; the total counts up to 43", target_s: 3.3 }
  - { id: "rerank", role: "proof", feature: "leader-ranking", shows: "Two more scores; rows re-rank and the new leader moves to the top", target_s: 3.0 }
  - { id: "end-round", role: "proof", feature: "round-history", shows: "End round; round 5 joins the history table", target_s: 3.5 }
  - { id: "rematch", role: "proof", feature: "rematch", shows: "Game over; one tap starts a rematch with the same four players", target_s: 2.5 }
captions:
  "en":
    - { start_s: 1.2, end_s: 4.5, text: "Totals add themselves.", emphasis: true, claim: "Tap a score and every total adds itself up." }
    - { start_s: 4.6, end_s: 7.5, text: "The leader rises to the top.", emphasis: false, claim: "The leader always rises to the top of the table." }
    - { start_s: 7.6, end_s: 11.0, text: "Every round is saved.", emphasis: false, claim: "Every round is saved to a history you can scroll back through." }
    - { start_s: 11.1, end_s: 13.4, text: "Rematch in one tap.", emphasis: false, claim: "Start a rematch with the same players in one tap." }
  "es":
    - { start_s: 1.2, end_s: 4.5, text: "Los totales se suman solos.", emphasis: true, claim: "Toca una puntuación y cada total se suma solo." }
    - { start_s: 4.6, end_s: 7.5, text: "El líder sube arriba.", emphasis: false, claim: "El líder siempre sube a lo alto de la tabla." }
    - { start_s: 7.6, end_s: 11.0, text: "Cada ronda queda guardada.", emphasis: false, claim: "Cada ronda queda guardada en un historial que puedes repasar." }
    - { start_s: 11.1, end_s: 13.4, text: "Revancha con un toque.", emphasis: false, claim: "Empieza la revancha con los mismos jugadores con un solo toque." }
carousel:
  "en":
    - { still: "hook-total", text: "Totals add themselves.", emphasis: true, claim: "Tap a score and every total adds itself up." }
    - { still: "reranked-table", text: "The leader rises to the top.", emphasis: false, claim: "The leader always rises to the top of the table." }
    - { still: "history-table", text: "Every round is saved.", emphasis: false, claim: "Every round is saved to a history you can scroll back through." }
    - { still: "rematch-prompt", text: "Rematch in one tap.", emphasis: false, claim: "Start a rematch with the same players in one tap." }
  "es":
    - { still: "hook-total", text: "Los totales se suman solos.", emphasis: true, claim: "Toca una puntuación y cada total se suma solo." }
    - { still: "reranked-table", text: "El líder sube arriba.", emphasis: false, claim: "El líder siempre sube a lo alto de la tabla." }
    - { still: "history-table", text: "Cada ronda queda guardada.", emphasis: false, claim: "Cada ronda queda guardada en un historial que puedes repasar." }
    - { still: "rematch-prompt", text: "Revancha con un toque.", emphasis: false, claim: "Empieza la revancha con los mismos jugadores con un solo toque." }
end_card:
  duration_s: 2.5
  transition_s: 0.5
  badges:
    - { store: "app_store", locale: "en", path: "marketing/badges/app-store-en.svg", public: true, launch_day: false }
    - { store: "app_store", locale: "es", path: "marketing/badges/app-store-es.svg", public: true, launch_day: false }
    - { store: "google_play", locale: "en", path: "marketing/badges/google-play-en.png", public: false, launch_day: true, do_not_post_before: "2026-10-15" }
    - { store: "google_play", locale: "es", path: "marketing/badges/google-play-es.png", public: false, launch_day: true, do_not_post_before: "2026-10-15" }
outputs:
  dir: "releases/promo"
  formats: ["reel_9x16", "carousel_4x5"]
seed:
  backend: "rn-asyncstorage"
  entries:
    - { key: "tallybird:game:v1", value: "{\"round\":5,\"players\":[{\"id\":\"p1\",\"label\":\"Ana\",\"total\":38},{\"id\":\"p2\",\"label\":\"Ben\",\"total\":35},{\"id\":\"p3\",\"label\":\"Chloe\",\"total\":31},{\"id\":\"p4\",\"label\":\"Dev\",\"total\":29}]}" }
    - { key: "tallybird:onboarding-seen:v1", value: "true" }
capture:
  ios: { app: "build/sim/Release-iphonesimulator/Tallybird.app", bundle_id: "com.example.tallybird", device: "iPhone 17 Pro", status_bar: { time: "9:41", battery_level: 100 }, appearance: "light" }
  takes:
    - id: "scores"
      steps:
        - maestro: "docs/marketing/flows/tap-score.yaml"
        - hold_s: 1.5
        - maestro: "docs/marketing/flows/rerank.yaml"
        - hold_s: 1.5
        - maestro: "docs/marketing/flows/end-round.yaml"
        - hold_s: 2.0
    - id: "rematch"
      steps:
        - maestro: "docs/marketing/flows/rematch.yaml"
        - hold_s: 1.5
  stills:
    - { id: "hook-total", take: "scores", after_step: 2 }
    - { id: "reranked-table", take: "scores", after_step: 4 }
    - { id: "history-table", take: "scores", after_step: 6 }
    - { id: "rematch-prompt", take: "rematch", after_step: 2 }
  cut:
    - { take: "scores", in_s: 5.6, out_s: 10.1, join: "cut" }
    - { take: "scores", in_s: 11.0, out_s: 14.0, join: "fade", fade_s: 0.4 }
    - { take: "scores", in_s: 16.2, out_s: 20.1, join: "cut" }
    - { take: "rematch", in_s: 5.4, out_s: 7.9 }
  compose: { background: "#F3EDE4", text_color: "#1A1A1A" }
---

# EXAMPLE — Tallybird Promo Campaign

> **EXAMPLE — fictional app.** Tallybird does not exist. This file shows a complete `promo-campaign/v1` instance file that passes every storyboard limit, with the arithmetic written out. Nothing in it describes a real app, listing or store.

The front matter above is what capture and compose tooling reads. This body is the rationale a reviewer reads. The plan is for iOS (`platforms`), shot on a simulator.

## Claim authority

| Store | Locale | Source | Confirmed live |
|-------|--------|--------|----------------|
| App Store | en | `store/app-store/en/description.txt` | 2026-09-01 — matches the signed-out listing |
| App Store | es | `store/app-store/es/description.txt` | 2026-09-01 — matches the signed-out listing |
| Google Play | en, es | *(not an authority)* | Listing exists in closed testing only, so the public reads nothing there yet |

The tagline "Score any game night." / "Puntúa cualquier noche de juegos." is the first line of each App Store listing.

## Features

| Rank | Feature | Visual | Claim (en, verbatim) |
|------|---------|--------|----------------------|
| 1 | Running totals | visual | "Tap a score and every total adds itself up." — the listing's subtitle |
| 2 | Leader on top | visual | "The leader always rises to the top of the table." |
| 3 | Round history | visual | "Every round is saved to a history you can scroll back through." |
| 4 | One-tap rematch | visual | "Start a rematch with the same players in one tap." |
| 5 | Works offline | not visual | "Works offline, at any table." |

**Unclaimed — never captioned:** CSV export, found in `CHANGELOG.md`, appears in neither listing. Decision: dropped from this campaign. Proposed listing line: "Export any game to CSV." Re-run once it is published.

**Claimed but not in the cut:** Works offline. It is not visual, and captioning it would need a fifth ≥ 2.0s caption over a beat that shows something else. Shorter wins.

## Hook

Running totals is the rank-1 visual claim. The moment that proves it: tap +12 on the third player and watch their total count up from 31 to 43. It is one continuous shot, and it proves the claim with the caption muted.

The proof frame (the total reading 43) is on screen at **2.4s**. The only setup is 1.2s of the table mid-game, which is possible because the game is seeded at round 5 rather than filmed from player entry. Filming player entry would put the hook near 9s.

## Storyboard

| Beat | Role | Window (s) | On screen | Caption (en) | Caption time |
|------|------|------------|-----------|--------------|--------------|
| setup-table | setup | 0.0–1.2 | Four players mid-game, round 5 | — | — |
| hook-tap | hook | 1.2–4.5 | +12 → total counts up to 43 | **Totals add themselves.** | 1.2–4.5 = 3.3s |
| rerank | proof | 4.5–7.5 | Two more scores; rows re-rank | The leader rises to the top. | 4.6–7.5 = 2.9s |
| end-round | proof | 7.5–11.0 | Round 5 joins the history table | Every round is saved. | 7.6–11.0 = 3.4s |
| rematch | proof | 11.0–13.5 | One tap starts a rematch | Rematch in one tap. | 11.1–13.4 = 2.3s |
| end card | — | 13.5–16.0 | 0.5s crossfade, then icon, name, tagline, badges | — | — |

The `es` captions use the same windows.

### Arithmetic

```
beats      1.2 + 3.3 + 3.0 + 3.5 + 2.5 = 13.5s   → end card starts at 13.5s
end card   2.5s, including its 0.5s crossfade     ≤ 3.0   PASS
total      13.5 + 2.5 = 16.0s planned             ≤ 20.0  PASS
render     must measure 16.0s ± 1 frame (±0.033s at 30 fps) with ffprobe
speed      every beat cut from takes at 1.0x              PASS
hook       lands at 2.4s, inside hook-tap [1.2, 4.5]  ≤ 5.0   PASS
captions   4 per locale                           ≤ 8     PASS
           3.3, 2.9, 3.4, 2.3 — shortest 2.3s     ≥ 2.0   PASS
           one emphasised caption (the hook's)            PASS
overlap    last caption ends 13.4s ≤ end-card start 13.5s  PASS
carousel   4 content slides + end card = 5        ≤ 8 + 1 PASS
```

No cut was needed. Beats and captions are one idea each, and no beat repeats another.

## Carousel

| Slide | Still | Text (en) |
|-------|-------|-----------|
| 1 | `hook-total` — the total reading 43 | **Totals add themselves.** |
| 2 | `reranked-table` — the new leader on top | The leader rises to the top. |
| 3 | `history-table` — rounds 1–5 | Every round is saved. |
| 4 | `rematch-prompt` — the rematch button | Rematch in one tap. |
| 5 | End card — icon, name, tagline, badges | — |

## Claim map

| Caption / slide (en) | Caption / slide (es) | Feature | Source |
|----------------------|----------------------|---------|--------|
| Totals add themselves. | Los totales se suman solos. | live-totals | App Store listing, en / es |
| The leader rises to the top. | El líder sube arriba. | leader-ranking | App Store listing, en / es |
| Every round is saved. | Cada ronda queda guardada. | round-history | App Store listing, en / es |
| Rematch in one tap. | Revancha con un toque. | rematch | App Store listing, en / es |

Each caption says less than its quote, never more.

## Badges

| Store | Locale | Public (checked signed-out) | launch_day · do_not_post_before | Shown |
|-------|--------|-----------------------------|---------------------------------|-------|
| App Store | en, es | yes — 2026-09-01 | — | yes |
| Google Play | en, es | no — closed testing | yes · 2026-10-15 | only in the launch-day render; **do not post before 2026-10-15** |

## Starting state for capture

A game in round 5 with four invented players ("Ana", "Ben", "Chloe", "Dev"). The third player's total is 31, and rounds 1–4 are already in the history. `seed` writes it, with the onboarding flag, into React Native AsyncStorage before the first launch, so neither take films player entry or onboarding.

## Capture and cut

Two takes. Each is a fresh install with the seeded game, recorded while its steps run:

| Take | Steps | Stills |
|------|-------|--------|
| `scores` | tap-score flow · hold 1.5 · rerank flow · hold 1.5 · end-round flow · hold 2.0 | `hook-total` after step 2, `reranked-table` after 4, `history-table` after 6 |
| `rematch` | rematch flow · hold 1.5 | `rematch-prompt` after step 2 |

The in and out points below are take seconds, read from each take's cue sheet:

| Segment | Take | In–out (s) | Length | Join into the next | Covers beats |
|---------|------|------------|--------|--------------------|--------------|
| 1 | scores | 5.6–10.1 | 4.5 | cut | setup-table + hook-tap, one continuous shot |
| 2 | scores | 11.0–14.0 | 3.0 | fade 0.4 | rerank |
| 3 | scores | 16.2–20.1 | 3.9 | cut | end-round (3.5 after the fade's 0.4) |
| 4 | rematch | 5.4–7.9 | 2.5 | — (last) | rematch |

```
segments   4.5 + 3.0 + 3.9 + 2.5 = 13.9s − 0.4s fade = 13.5s  = beats 13.5s   PASS (1.0x)
render     13.5 + end card 2.5 = 16.0s                          = plan 16.0s
hook       segment 1 starts at take 5.6s; the total reads 43 at take 8.0s → output 2.4s
```
