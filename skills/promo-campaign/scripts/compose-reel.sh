#!/bin/bash
# promo-campaign — compose the 9:16 reel from recorded takes, at 1.0x.
#
# Usage:
#   bash compose-reel.sh --campaign <abs path to docs/marketing/campaign.md> [--repo <abs repo root>]
#                        [--locale <locale>] [--launch-day]
#   bash compose-reel.sh --help
#
# Cuts capture.cut[] segments out of <outputs.dir>/takes/<take>.mov, using each
# take's <take>.cues.json (capture-ios.sh writes both), and writes
# <outputs.dir>/reel-<locale>.mp4: 1080x1920 H.264, 30 fps, one per locale.
# One review frame per caption, at its midpoint, goes to
# <outputs.dir>/checks/reel-<locale>-caption-<n>.png.
#
# Badges: public ones only. With --launch-day, launch_day badges are added and
# the output is reel-<locale>.launch-day.mp4 (review frames
# reel-<locale>.launch-day-caption-<n>.png), never the postable reel-<locale>.mp4.
# A launch-day render must not be posted before its badges' do_not_post_before,
# which the summary line names.
#
# Timeline: segments play back to back at 1.0x (a `fade` join overlaps its two
# segments by fade_s), then the end card crossfades in at the end of the
# footage, which is held on its last frame for the transition. So
#   render = (sum of segments - sum of fades) + end_card.duration_s
# and sum of segments - sum of fades must equal the sum of beats[].target_s.
# Captions must lie inside the footage: 0 <= start_s < end_s <= sum of beats
# (contract.sh refuses the campaign otherwise, exit 65).
#
# Self-checks, each failing the run (exit 70): the frame layer is srgba with a
# transparent hole; each caption fits its band; every out_s is within its take's
# logged length; the filtergraph re-times nothing; segments - fades = beats within
# one frame; the render's video-stream duration equals the plan within one frame
# and is <= max_duration_s; the render is 1080x1920.
#
# Environment: PROMO_CAMPAIGN_FONT / PROMO_CAMPAIGN_FONT_BOLD override
# capture.compose.font / font_bold (default: macOS Arial and Arial Bold);
# PROMO_RENDER_TIMEOUT_S bounds each render in whole seconds (default 600).
#
# Exit: 0 rendered and checked; 64 usage; 65 campaign data; 66 missing input;
# 69 missing prerequisite; 70 a self-check failed; 1 ffmpeg failed.
set -euo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
. "$HERE/lib/common.sh"
. "$HERE/lib/contract.sh"
. "$HERE/lib/geometry.sh"

CW=1080; CH=1920; TOP=250; BOTTOM=200; MARGIN=120; FPS=30
FRAME_S="$(calc "1 / $FPS")"
# Applied after every trim and after every join, so it sits immediately before
# every concat/xfade input. concat re-stamps its output at 1/1000000 while a
# trimmed segment stays at 1/30, and xfade refuses mismatched timebases:
# normalising once at the source is not enough.
JOIN_NORM=",settb=AVTB,fps=$FPS"

campaign=""; repo=""; only_locale=""; launch_day=0
while [ $# -gt 0 ]; do
  case "$1" in
    --campaign|--repo|--locale)
      [ $# -ge 2 ] || die 64 "$1 needs a value"
      case "$1" in --campaign) campaign="$2" ;; --repo) repo="$2" ;; --locale) only_locale="$2" ;; esac
      shift 2 ;;
    --launch-day) launch_day=1; shift ;;
    -h|--help) usage_from_header "${BASH_SOURCE[0]}"; exit 0 ;;
    *) die 64 "unknown argument '$1' (see --help)" ;;
  esac
done
[ -n "$campaign" ] || die 64 "--campaign <absolute path to campaign.md> is required (see --help)"
# perl's alarm takes whole seconds, and 0 (or a fraction, truncated to 0) would
# switch the watchdog off.
RENDER_TIMEOUT_S="${PROMO_RENDER_TIMEOUT_S:-600}"
case "$RENDER_TIMEOUT_S" in
  ""|*[!0-9]*|0*) die 64 "PROMO_RENDER_TIMEOUT_S must be a whole number of seconds >= 1, got '$RENDER_TIMEOUT_S'" ;;
esac
VARIANT=""
if [ "$launch_day" = 1 ]; then VARIANT=".launch-day"; fi
need ffmpeg ffprobe magick jq perl yaml
encoders="$(ffmpeg -hide_banner -encoders 2>/dev/null || true)"
case "$encoders" in *libx264*) ;; *) die 69 "this ffmpeg has no libx264 encoder: brew reinstall ffmpeg" ;; esac
[ -f "$campaign" ] || die 66 "campaign file '$campaign' not found"
campaign="$(abs_path "$campaign")"
repo="$(resolve_repo "$campaign" "$repo")"
T="$(make_tmp)"
trap 'rm -rf "$T"' EXIT

contract_load "$campaign" "$T"
contract_validate compose
cj -e 'any(.outputs.formats[]; . == "reel_9x16")' >/dev/null || die 65 "outputs.formats has no \"reel_9x16\": nothing to compose"

OUT="$(repo_path "$repo" "$(cj '.outputs.dir')")"; TAKES="$OUT/takes"
FONT="$(resolve_font font "${PROMO_CAMPAIGN_FONT:-}" "$(cj '.capture.compose.font // ""')" /System/Library/Fonts/Supplemental/Arial.ttf)"
BOLD="$(resolve_font font_bold "${PROMO_CAMPAIGN_FONT_BOLD:-}" "$(cj '.capture.compose.font_bold // ""')" "/System/Library/Fonts/Supplemental/Arial Bold.ttf")"
BG="$(cj '.capture.compose.background // "#F3EDE4"')"
FG="$(cj '.capture.compose.text_color // "#1A1A1A"')"
ICON="$(repo_path "$repo" "$(cj '.app.icon')")"
[ -f "$ICON" ] || die 66 "app.icon '$ICON' not found"

# --- render arithmetic, before anything is rendered -------------------------
BEATS_S="$(cj '[.beats[].target_s] | add // 0')"
CARD_S="$(cj '.end_card.duration_s')"; TR_S="$(cj '.end_card.transition_s')"
LIMIT_S="$(cj '[.max_duration_s // 20, 20] | min')"
NSEG="$(cj '.capture.cut | length')"
SEG_SUM="$(cj '[.capture.cut[] | .out_s - .in_s] | add')"
FADE_SUM="$(cj '[.capture.cut[] | select(.join == "fade") | .fade_s] | add // 0')"
MAIN_S="$(calc "$SEG_SUM - $FADE_SUM")"
PLANNED_S="$(calc "$BEATS_S + $CARD_S")"
say "plan: segments $SEG_SUM - fades $FADE_SUM = $MAIN_S s against beats $BEATS_S s; + end card $CARD_S s = $PLANNED_S s (limit $LIMIT_S s)"
fabs_le "$MAIN_S" "$BEATS_S" "$FRAME_S" \
  || die 70 "self-check: segments ($SEG_SUM s) minus crossfades ($FADE_SUM s) = $MAIN_S s, but beats[].target_s sum to $BEATS_S s. At 1.0x the cut must fill the beats exactly: fix capture.cut or the beats"
fle "$PLANNED_S" "$LIMIT_S" 0.000001 || die 70 "self-check: planned length $PLANNED_S s is over the limit $LIMIT_S s"

# --- takes: logged length, padding, source size ------------------------------
TK_ID=(); TK_PAD=(); TK_LEN=(); TK_USED=(); SW=""; SH=""
while IFS= read -r t; do
  mov="$TAKES/$t.mov"; cues="$TAKES/$t.cues.json"
  [ -f "$mov" ] && [ -f "$cues" ] || die 66 "take \"$t\": $mov and $t.cues.json are both needed; run capture-ios.sh first"
  jq -e '.take_len_s | type == "number"' "$cues" >/dev/null || die 65 "$cues has no numeric take_len_s"
  len="$(jq -r '.take_len_s' "$cues")"
  dur="$(probe_video "$mov" duration)"
  case "$dur" in ""|N/A) dur="$(ffprobe -v error -show_entries format=duration -of default=nw=1:nk=1 "$mov")" ;; esac
  w="$(probe_video "$mov" width)"; h="$(probe_video "$mov" height)"
  if [ -z "$SW" ]; then SW="$w"; SH="$h"
  elif [ "$w" != "$SW" ] || [ "$h" != "$SH" ]; then die 65 "take \"$t\" is ${w}x${h} but an earlier take is ${SW}x${SH}: capture every take on one device"
  fi
  # recordVideo writes no frames while the screen is static, so a trailing hold
  # is missing from the file. Pad back to the logged length with the last frame.
  TK_ID+=("$t"); TK_LEN+=("$len"); TK_USED+=(0)
  TK_PAD+=("$(awk -v l="$len" -v d="$dur" 'BEGIN { p = l - d; printf "%.3f", (p > 0 ? p : 0) }')")
  say "take $t: file ${dur}s, logged ${len}s, padded by ${TK_PAD[${#TK_PAD[@]}-1]}s, ${w}x${h}"
done < <(cj '[.capture.cut[].take] | unique | .[]')
NT="${#TK_ID[@]}"

take_index() { local k=0; while [ "$k" -lt "$NT" ]; do [ "${TK_ID[$k]}" = "$1" ] && { echo "$k"; return; }; k=$((k + 1)); done; return 1; }

set -- $(window "$SW" "$SH" "$CW" "$CH" "$TOP" "$BOTTOM" "$MARGIN")
WIN_W="$1"; WIN_H="$2"; WIN_X="$3"; WIN_Y="$4"
BAND_H=$((WIN_Y - 40)); CAP_W=$((CW - 140))
say "window: ${WIN_W}x${WIN_H}+${WIN_X}+${WIN_Y} for a ${SW}x${SH} source"

build_frame "$T/frame.png" "$CW" "$CH" "$WIN_W" "$WIN_H" "$WIN_X" "$WIN_Y" "$BG" "#111111"
assert_frame "$T/frame.png" "$((WIN_X + WIN_W / 2))" "$((WIN_Y + WIN_H / 2))"

# --- the segment graph, shared by every locale -------------------------------
SEG_G=""; k=0
while [ "$k" -lt "$NT" ]; do
  uses="$(cj --arg t "${TK_ID[$k]}" '[.capture.cut[] | select(.take == $t)] | length')"
  outs=""; c=0; while [ "$c" -lt "$uses" ]; do outs="$outs[t${k}c$c]"; c=$((c + 1)); done
  SEG_G="$SEG_G[$k:v]tpad=stop_mode=clone:stop_duration=${TK_PAD[$k]},settb=AVTB,fps=$FPS,format=yuv420p,setsar=1,scale=$WIN_W:$WIN_H:flags=lanczos,split=$uses$outs;"
  k=$((k + 1))
done
j=0; ACC=""; ACC_S=0
while [ "$j" -lt "$NSEG" ]; do
  t="$(cj --argjson j "$j" '.capture.cut[$j].take')"
  in_s="$(cj --argjson j "$j" '.capture.cut[$j].in_s')"; out_s="$(cj --argjson j "$j" '.capture.cut[$j].out_s')"
  k="$(take_index "$t")"; c="${TK_USED[$k]}"; TK_USED[$k]=$((c + 1))
  fle "$out_s" "${TK_LEN[$k]}" 0.0005 \
    || die 70 "self-check: capture.cut[$j] out_s $out_s is past take \"$t\"'s logged length ${TK_LEN[$k]}s"
  len="$(calc "$out_s - $in_s")"
  SEG_G="$SEG_G[t${k}c$c]trim=start=$in_s:end=$out_s,setpts=PTS-STARTPTS$JOIN_NORM[s$j];"
  if [ "$j" = 0 ]; then
    ACC="s0"; ACC_S="$len"
  else
    join="$(cj --argjson j "$((j - 1))" '.capture.cut[$j].join // "cut"')"
    if [ "$join" = fade ]; then
      f="$(cj --argjson j "$((j - 1))" '.capture.cut[$j].fade_s')"
      SEG_G="$SEG_G[$ACC][s$j]xfade=transition=fade:duration=$f:offset=$(calc "$ACC_S - $f")$JOIN_NORM[j$j];"
      ACC_S="$(calc "$ACC_S + $len - $f")"
    else
      SEG_G="$SEG_G[$ACC][s$j]concat=n=2:v=1:a=0$JOIN_NORM[j$j];"
      ACC_S="$(calc "$ACC_S + $len")"
    fi
    ACC="j$j"
  fi
  j=$((j + 1))
done

mkdir -p "$OUT/checks"
# Locales are format-checked by contract.sh, so they are safe to split and to use in file names.
LOCALES="$(cj '.locales[]')"
if [ -n "$only_locale" ]; then
  printf '%s\n' "$LOCALES" | grep -Fqx -- "$only_locale" || die 64 "--locale '$only_locale' is not one of locales: $(printf '%s ' $LOCALES)"
  LOCALES="$only_locale"
fi

for L in $LOCALES; do
  NAME="reel-$L$VARIANT"
  # Never leave a previous run's reel or review frames looking current.
  reel="$OUT/$NAME.mp4"; rm -f "$reel" "$OUT/checks/$NAME-caption-"*.png "$OUT/checks/$NAME.rejected.mp4"
  # -nostdin: ffmpeg otherwise reads the caller's stdin for its interactive keys.
  argv=(-nostdin -hide_banner -v error -y)
  k=0; while [ "$k" -lt "$NT" ]; do argv+=(-i "$TAKES/${TK_ID[$k]}.mov"); k=$((k + 1)); done
  argv+=(-f lavfi -i "color=c=0x${BG#\#}:s=${CW}x${CH}:r=$FPS" -loop 1 -framerate "$FPS" -i "$T/frame.png")
  # The end-card transition starts at the end of the footage. Its last frame is
  # held for transition_s here, before any caption is drawn; captions end by the
  # end-card start (contract.sh), so none rides into the transition.
  if awk -v t="$TR_S" 'BEGIN { exit !(t > 0) }'; then
    G="$SEG_G[$ACC]tpad=stop_mode=clone:stop_duration=$TR_S$JOIN_NORM[main];"
  else
    G="$SEG_G[$ACC]null[main];"
  fi
  G="$G[$NT:v][main]overlay=$WIN_X:$WIN_Y:shortest=1[o0];[o0][$((NT + 1)):v]overlay=0:0:shortest=1[o1];"
  last="o1"
  ncap="$(cj --arg l "$L" '.captions[$l] // [] | length')"
  n=0
  while [ "$n" -lt "$ncap" ]; do
    text="$(cj --arg l "$L" --argjson n "$n" '.captions[$l][$n].text')"
    s="$(cj --arg l "$L" --argjson n "$n" '.captions[$l][$n].start_s')"; e="$(cj --arg l "$L" --argjson n "$n" '.captions[$l][$n].end_s')"
    pt=54; cj -e --arg l "$L" --argjson n "$n" '.captions[$l][$n].emphasis == true' >/dev/null && pt=62
    render_caption "$T/cap-$L-$n.png" "$text" "$BOLD" "$pt" "$FG" "$CW" "$CH" "$CAP_W" "$BAND_H"
    argv+=(-loop 1 -framerate "$FPS" -i "$T/cap-$L-$n.png")
    # [start_s, end_s) to the frame: half-frame offsets keep a frame stamped
    # 7.29999 from counting as before 7.3.
    G="$G[$last][$((NT + 2 + n)):v]overlay=0:0:shortest=1:enable='gte(t,$(calc "$s - $FRAME_S / 2"))*lt(t,$(calc "$e - $FRAME_S / 2"))'[o$((n + 2))];"
    last="o$((n + 2))"
    n=$((n + 1))
  done

  BADGES=()
  while IFS= read -r b; do
    [ -f "$(repo_path "$repo" "$b")" ] || die 66 "end_card badge '$b' not found under $repo"
    BADGES+=("$(repo_path "$repo" "$b")")
  done < <(cj --arg l "$L" --argjson ld "$launch_day" '.end_card.badges // [] | .[] | select(.locale == $l)
             | select(.public == true or ($ld == 1 and .launch_day == true and ((.do_not_post_before // "") | tostring | length) > 0)) | .path')
  POST_AFTER=""
  if [ "$launch_day" = 1 ]; then
    POST_AFTER="$(cj --arg l "$L" '[.end_card.badges // [] | .[] | select(.locale == $l and .public != true and .launch_day == true) | .do_not_post_before | tostring] | unique | join(", ")')"
  fi
  build_end_card "$T/card-$L.png" "$CW" "$CH" "$BG" "$FG" "$FONT" "$BOLD" "$ICON" "$(cj '.app.name')" \
    "$(cj --arg l "$L" '.app.tagline[$l] // ""')" ${BADGES[@]+"${BADGES[@]}"}
  argv+=(-loop 1 -framerate "$FPS" -t "$CARD_S" -i "$T/card-$L.png")
  CARD_IN=$((NT + 2 + ncap))
  if awk -v t="$TR_S" 'BEGIN { exit !(t > 0) }'; then
    # offset = end of footage = end-card start, so the card, transition included,
    # lasts duration_s.
    G="$G[$last]format=yuv420p,setsar=1$JOIN_NORM[m];[$CARD_IN:v]settb=AVTB,fps=$FPS,format=yuv420p,setsar=1[e];[m][e]xfade=transition=fade:duration=$TR_S:offset=$ACC_S[out]"
  else
    G="$G[$last]settb=AVTB,fps=$FPS,format=yuv420p,setsar=1[m];[$CARD_IN:v]settb=AVTB,fps=$FPS,format=yuv420p,setsar=1[e];[m][e]concat=n=2:v=1:a=0[out]"
  fi
  argv+=(-filter_complex "$G" -map "[out]" -an -c:v libx264 -pix_fmt yuv420p -r "$FPS" -crf 19 -preset veryfast -movflags +faststart "$T/$NAME.mp4")

  # 1.0x: the only re-timing allowed is setpts=PTS-STARTPTS after a trim.
  bad="$(printf '%s\n' "$G" | grep -oE 'setpts=[^,;[]*' | grep -vx 'setpts=PTS-STARTPTS' || true)"
  case "$G ${argv[*]}" in *atempo*|*asetrate*|*itsscale*|*minterpolate*|*setrate*) bad="$bad speed filter" ;; esac
  [ -z "$bad" ] || die 70 "self-check: the filtergraph re-times frames ($bad); only setpts=PTS-STARTPTS is allowed at 1.0x"

  rc=0
  with_timeout "$RENDER_TIMEOUT_S" ffmpeg "${argv[@]}" 2> "$T/ffmpeg-$L.log" || rc=$?
  if [ "$rc" = 142 ]; then
    die 70 "self-check: $NAME did not finish within ${RENDER_TIMEOUT_S}s and was killed. An overlay without shortest=1 never ends"
  elif [ "$rc" != 0 ]; then
    tail -20 "$T/ffmpeg-$L.log" >&2; die 1 "ffmpeg failed rendering $NAME (exit $rc)"
  fi

  dur="$(probe_video "$T/$NAME.mp4" duration)"
  w="$(probe_video "$T/$NAME.mp4" width)"; h="$(probe_video "$T/$NAME.mp4" height)"
  fail=""
  [ "$w" = "$CW" ] && [ "$h" = "$CH" ] || fail="size ${w}x${h} is not ${CW}x${CH}"
  fabs_le "$dur" "$PLANNED_S" "$FRAME_S" || fail="${fail:+$fail; }duration ${dur}s is not the planned ${PLANNED_S}s within one frame (${FRAME_S}s): a short render usually means a take lost its static tail"
  fle "$dur" "$LIMIT_S" 0.000001 || fail="${fail:+$fail; }duration ${dur}s is over the limit ${LIMIT_S}s"
  if [ -n "$fail" ]; then
    cp "$T/$NAME.mp4" "$OUT/checks/$NAME.rejected.mp4"
    die 70 "self-check: $NAME: $fail (kept for review as $OUT/checks/$NAME.rejected.mp4)"
  fi
  mv "$T/$NAME.mp4" "$reel"

  n=0
  while [ "$n" -lt "$ncap" ]; do
    mid="$(cj --arg l "$L" --argjson n "$n" '(.captions[$l][$n].start_s + .captions[$l][$n].end_s) / 2')"
    ffmpeg -nostdin -hide_banner -v error -y -ss "$mid" -i "$reel" -frames:v 1 "$OUT/checks/$NAME-caption-$((n + 1)).png"
    n=$((n + 1))
  done
  NOTE=""
  if [ "$launch_day" = 1 ]; then NOTE="; LAUNCH-DAY render: do not post before ${POST_AFTER:-the do_not_post_before of its badges}"; fi
  printf '%s: %s — %ss (plan %ss = beats %ss + end card %ss, limit %ss), %sx%s, window %sx%s+%s+%s, %s caption frames in %s%s\n' \
    "$NAME" "$reel" "$dur" "$PLANNED_S" "$BEATS_S" "$CARD_S" "$LIMIT_S" "$w" "$h" "$WIN_W" "$WIN_H" "$WIN_X" "$WIN_Y" "$ncap" "$OUT/checks" "$NOTE"
done
