#!/bin/bash
# promo-campaign — record iOS Simulator takes, their cue sheets, and lossless stills.
#
# Usage:
#   bash capture-ios.sh --campaign <abs path to docs/marketing/campaign.md> [--repo <abs repo root>]
#                       [--take <id>]... [--no-stills] [--keep-app] [--replace-installed] [--keep-clipboard]
#   bash capture-ios.sh --help
#
# For each capture.takes[] entry, on the one booted Simulator that matches
# capture.ios.device (a name or UDID; any single booted Simulator when absent):
#   simctl uninstall -> install -> seed (seed.backend) -> status bar clear + override
#   -> starting appearance -> launch -> recordVideo --codec=h264 -> steps -> stop
# Steps run in order inside one recording, and start only after recordVideo
# prints "Recording started":
#   maestro: "<flow.yaml>"   a consumer-authored flow; `label:` names its cues
#   hold_s: 1.5              keep recording, untouched
#   appearance: "dark"       flipped from the shell, so it lands inside the take
# Writes <outputs.dir>/takes/<take>.mov and <take>.cues.json beside it. A take's
# previous files are removed before it records, so a failed run never leaves an
# older take for the composer. A step that fails discards its take and fails the
# run. Unless --no-stills, a second pass that does not record replays each take's
# steps and saves every capture.stills[] entry with `simctl io screenshot` to
# <outputs.dir>/stills/<id>.png.
#
# Every take uninstalls the app, which deletes its data on that Simulator. So an
# app that is already installed there is refused unless --replace-installed.
#
# Host clipboard (SKILL.md Safety Rule 5): on macOS it is saved with pbpaste,
# cleared for the capture, and restored on every exit. The restore is text
# only; the run says so when the clipboard held anything else. --keep-clipboard
# leaves it alone. The Simulator's own pasteboard is emptied before each take.
# simctl has no switch for notifications (see references/capture-and-compose.md).
#
# Every Maestro call gets --device <UDID>, and DEVELOPER_DIR when xcode-select
# points at the Command Line Tools. Only simulators are addressed, so an attached
# physical device is never driven. On any exit a trap restores what the run
# changed: status bar cleared, the Simulator's previous appearance, the app
# uninstalled unless --keep-app, the host clipboard restored.
#
# Exit: 0 ok; 64 usage; 65 campaign data or an installed app without
# --replace-installed; 66 missing input; 69 missing prerequisite or not exactly
# one matching booted Simulator; 1 a step or recording failed.
set -euo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
. "$HERE/lib/common.sh"
. "$HERE/lib/contract.sh"

campaign=""; repo=""; keep_app=0; want_stills=1; replace_installed=0; manage_clipboard=1; only=" "
while [ $# -gt 0 ]; do
  case "$1" in
    --campaign|--repo|--take)
      [ $# -ge 2 ] || die 64 "$1 needs a value"
      case "$1" in --campaign) campaign="$2" ;; --repo) repo="$2" ;; --take) only="$only$2 " ;; esac
      shift 2 ;;
    --keep-app) keep_app=1; shift ;;
    --no-stills) want_stills=0; shift ;;
    --replace-installed) replace_installed=1; shift ;;
    --keep-clipboard) manage_clipboard=0; shift ;;
    -h|--help) usage_from_header "${BASH_SOURCE[0]}"; exit 0 ;;
    *) die 64 "unknown argument '$1' (see --help)" ;;
  esac
done
[ -n "$campaign" ] || die 64 "--campaign <absolute path to campaign.md> is required (see --help)"
need jq perl xcrun maestro ffprobe yaml
[ -f "$campaign" ] || die 66 "campaign file '$campaign' not found"
campaign="$(abs_path "$campaign")"
repo="$(resolve_repo "$campaign" "$repo")"

T="$(make_tmp)"
U=""; BID=""; REC_PID=""; ORIG_APPEARANCE=""; TOUCHED=0; INSTALLED=0; CLIP_SAVED=0
cleanup() {
  local rc=$? keep_tmp=0
  set +e
  if [ -n "$REC_PID" ]; then kill -INT "$REC_PID" 2>/dev/null; wait "$REC_PID" 2>/dev/null; fi
  if [ -n "$U" ] && [ "$TOUCHED" = 1 ]; then
    xcrun simctl status_bar "$U" clear >/dev/null 2>&1
    case "$ORIG_APPEARANCE" in light|dark) xcrun simctl ui "$U" appearance "$ORIG_APPEARANCE" >/dev/null 2>&1 ;; esac
    if [ "$keep_app" = 0 ] && [ "$INSTALLED" = 1 ]; then
      xcrun simctl terminate "$U" "$BID" >/dev/null 2>&1
      xcrun simctl uninstall "$U" "$BID" >/dev/null 2>&1
    fi
    say "restored the Simulator: status bar cleared, appearance ${ORIG_APPEARANCE:-unchanged}, app $([ "$keep_app" = 1 ] && echo kept || echo uninstalled)"
  fi
  if [ "$CLIP_SAVED" = 1 ]; then
    if pbcopy < "$T/clipboard.txt" 2>/dev/null; then say "host clipboard: restored (text only)"
    else keep_tmp=1; say "host clipboard: could NOT be restored; its saved text is in $T/clipboard.txt"; fi
  fi
  [ "$keep_tmp" = 1 ] || rm -rf "$T"
  exit "$rc"
}
trap cleanup EXIT
trap 'exit 130' INT TERM

contract_load "$campaign" "$T"
contract_validate capture

OUT="$(repo_path "$repo" "$(cj '.outputs.dir')")"
APP="$(repo_path "$repo" "$(cj '.capture.ios.app')")"
BID_WANTED="$(cj '.capture.ios.bundle_id')"
DEVICE="$(cj '.capture.ios.device // ""')"
START_APPEARANCE="$(cj '.capture.ios.appearance // "light"')"
SB_TIME="$(cj '.capture.ios.status_bar.time // "9:41"')"
SB_BATTERY="$(cj '.capture.ios.status_bar.battery_level // 100')"
SEED_BACKEND="$(cj '.seed.backend // ""')"

[ -d "$APP" ] || die 66 "capture.ios.app '$APP' is not a directory: build a Release-iphonesimulator .app first"
if command -v plutil >/dev/null 2>&1; then
  plist_id="$(plutil -extract CFBundleIdentifier raw "$APP/Info.plist" 2>/dev/null || true)"
  [ -z "$plist_id" ] || [ "$plist_id" = "$BID_WANTED" ] \
    || die 65 "capture.ios.bundle_id \"$BID_WANTED\" does not match the app's CFBundleIdentifier \"$plist_id\""
fi
if [ -n "$SEED_BACKEND" ]; then
  [ -f "$HERE/seed-$SEED_BACKEND-ios.sh" ] || die 69 "seed backend \"$SEED_BACKEND\" has no iOS writer (seed-$SEED_BACKEND-ios.sh)"
  cj -c '.seed.entries' > "$T/seed-entries.json"
fi
while IFS= read -r flow; do
  [ -f "$(repo_path "$repo" "$flow")" ] || die 66 "Maestro flow '$flow' not found under $repo"
done < <(cj '.capture.takes[].steps[] | .maestro // empty')
for id in $only; do
  cj -e --arg t "$id" 'any(.capture.takes[]; .id == $t)' >/dev/null || die 64 "--take $id: no capture.takes entry has that id"
done

ensure_developer_dir
xcrun simctl help >/dev/null 2>&1 || die 69 "xcrun simctl is unavailable (DEVELOPER_DIR=${DEVELOPER_DIR:-unset}): install Xcode"

# Exactly one booted Simulator must match: never guess between two.
matches="$(xcrun simctl list -j devices booted | jq -r --arg d "$DEVICE" \
  '[.devices[][] | select(.state == "Booted") | select($d == "" or .udid == $d or .name == $d) | "\(.udid)\t\(.name)"] | .[]')"
if [ "$(printf '%s' "$matches" | grep -c . || true)" != 1 ]; then
  say "booted Simulators: $(xcrun simctl list -j devices booted | jq -r '[.devices[][] | select(.state == "Booted") | "\(.name) (\(.udid))"] | join(", ")')"
  die 69 "need exactly one booted Simulator matching capture.ios.device \"${DEVICE:-any}\": boot one (xcrun simctl boot <name>) or name it by UDID"
fi
SIM_NAME="${matches#*	}"
ORIG_APPEARANCE="$(xcrun simctl ui "${matches%%	*}" appearance 2>/dev/null || true)"
U="${matches%%	*}"; BID="$BID_WANTED"
say "Simulator: $SIM_NAME ($U)"

# An installed app has data this run would delete: every take uninstalls first.
if xcrun simctl get_app_container "$U" "$BID" data >/dev/null 2>&1; then
  [ "$replace_installed" = 1 ] || die 65 "$BID is already installed on $SIM_NAME ($U). Capture uninstalls and reinstalls it for every take, which deletes that app's data on this Simulator. Re-run with --replace-installed to allow that, or uninstall it yourself first"
  say "--replace-installed: $BID's data on $SIM_NAME will be deleted"
fi

say "notifications: simctl has no switch for them; a fresh install has not been granted notification permission, and this run sends no simctl push"

# save_clipboard: SKILL.md Safety Rule 5 on the host side.
save_clipboard() {
  local info other=""
  if [ "$manage_clipboard" = 0 ]; then say "host clipboard: left as is (--keep-clipboard); clear it yourself (SKILL.md Safety Rule 5)"; return 0; fi
  if [ "$(uname -s)" != Darwin ] || ! command -v pbpaste >/dev/null 2>&1 || ! command -v pbcopy >/dev/null 2>&1; then
    say "host clipboard: pbpaste/pbcopy unavailable, so it is not managed; clear it yourself (SKILL.md Safety Rule 5)"; return 0
  fi
  pbpaste > "$T/clipboard.txt" 2>/dev/null || die 1 "could not read the host clipboard with pbpaste (use --keep-clipboard to skip)"
  if command -v osascript >/dev/null 2>&1; then
    info="$(osascript -e 'clipboard info' 2>/dev/null || true)"
    other="$(printf '%s' "$info" | perl -e '@f = split /, /, join("", <STDIN>); for ($i = 0; $i < @f; $i += 2) { print "$f[$i]\n" }' \
      | grep -vE '^(«class utf8»|«class ut16»|string|Unicode text)$' | tr '\n' ' ' || true)"
  fi
  CLIP_SAVED=1
  pbcopy < /dev/null || die 1 "could not clear the host clipboard (use --keep-clipboard to skip)"
  if [ -n "${other// /}" ]; then
    say "host clipboard: cleared for the capture. It also held non-text data (${other% }); restore is text-only, so that part will not come back"
  else
    say "host clipboard: saved ($(wc -c < "$T/clipboard.txt" | tr -d ' ') bytes of text), cleared for the capture, restored on exit"
  fi
}

# prepare_app: a fresh install with seeded state, a demo status bar and the
# starting appearance, launched. `simctl install` over an existing install keeps
# its data container, so uninstall always comes first.
prepare_app() {
  TOUCHED=1
  xcrun simctl terminate "$U" "$BID" >/dev/null 2>&1 || true
  xcrun simctl uninstall "$U" "$BID" >/dev/null 2>&1 || true
  xcrun simctl install "$U" "$APP"
  INSTALLED=1
  if [ -n "$SEED_BACKEND" ]; then
    bash "$HERE/seed-$SEED_BACKEND-ios.sh" --udid "$U" --bundle-id "$BID" --entries "$T/seed-entries.json"
  fi
  printf '' | xcrun simctl pbcopy "$U"
  xcrun simctl status_bar "$U" clear
  xcrun simctl status_bar "$U" override --time "$SB_TIME" --dataNetwork wifi --wifiMode active --wifiBars 3 \
    --cellularMode active --cellularBars 4 --operatorName '' --batteryState discharging --batteryLevel "$SB_BATTERY"
  xcrun simctl ui "$U" appearance "$START_APPEARANCE"
  xcrun simctl launch "$U" "$BID" >/dev/null
  sleep 1
}

# run_step <take index> <step index> <maestro output dir> <events file or "">
# Called as `run_step … || rc=$?`, where set -e does not apply inside, so every
# command whose failure matters is checked explicitly.
run_step() {
  local ti="$1" si="$2" kind raw json t0 t1 rc=0 what="step $(($2 + 1)) of take index $1"
  kind="$(cj --argjson t "$ti" --argjson s "$si" '.capture.takes[$t].steps[$s] | keys[0]')" || die 1 "internal error: cannot read $what"
  case "$kind" in maestro|hold_s|appearance) ;; *) die 1 "internal error: $what has kind '$kind'" ;; esac
  raw="$(cj --argjson t "$ti" --argjson s "$si" '.capture.takes[$t].steps[$s][]')" || die 1 "internal error: cannot read the value of $what"
  # JSON, not raw: cj's -r would strip a string's quotes and break --argjson.
  json="$(jq -c --argjson t "$ti" --argjson s "$si" '.capture.takes[$t].steps[$s][]' "$CONTRACT_JSON")" || die 1 "internal error: cannot read the value of $what"
  t0="$(now)" || die 1 "internal error: the perl clock failed"
  case "$kind" in
    maestro)    maestro --device "$U" test --no-ansi --test-output-dir "$3" "$(repo_path "$repo" "$raw")" < /dev/null > "$3.log" 2>&1 || rc=$? ;;
    hold_s)     sleep "$raw" || rc=$? ;;
    appearance) xcrun simctl ui "$U" appearance "$raw" || rc=$? ;;
  esac
  t1="$(now)" || die 1 "internal error: the perl clock failed"
  if [ -n "$4" ]; then
    jq -nc --arg kind "$kind" --argjson value "$json" --argjson step "$((si + 1))" --argjson t0 "$t0" --argjson t1 "$t1" \
      --argjson rc "$rc" '{step: $step, kind: $kind, value: $value, start: $t0, end: $t1, exit: $rc}' >> "$4" \
      || die 1 "internal error: could not record $what in the cue sheet"
  fi
  return "$rc"
}

# stop_recording: SIGINT the recorder and keep its exit status in REC_RC.
stop_recording() {
  REC_RC=0
  kill -INT "$REC_PID" 2>/dev/null || true
  wait "$REC_PID" 2>/dev/null || REC_RC=$?
  REC_PID=""
}

# record_take <take index> <take id>
record_take() {
  local ti="$1" id="$2" mov="$T/$2.mov" log="$T/$2.rec.log" ev="$T/$2.events.jsonl"
  local n i=0 waited=0 started stopped rc=0 dur nulls
  mkdir -p "$OUT/takes"
  # Never leave an earlier run's take behind a failed one: the composer would use it.
  rm -f "$OUT/takes/$id.mov" "$OUT/takes/$id.cues.json" "$OUT/takes/.$id.cues.json.tmp" "$OUT/takes/$id".failed-step-*.log
  : > "$ev"
  prepare_app
  xcrun simctl io "$U" recordVideo --codec=h264 --force "$mov" < /dev/null > "$log" 2>&1 &
  REC_PID=$!
  until grep -q "Recording started" "$log" 2>/dev/null; do
    kill -0 "$REC_PID" 2>/dev/null || { cat "$log" >&2; REC_PID=""; die 1 "take \"$id\": recordVideo exited before 'Recording started'"; }
    [ "$waited" -lt 300 ] || die 1 "take \"$id\": recordVideo did not print 'Recording started' within 15s"
    sleep 0.05; waited=$((waited + 1))
  done
  started="$(now)"
  n="$(cj --argjson t "$ti" '.capture.takes[$t].steps | length')"
  while [ "$i" -lt "$n" ]; do
    rc=0
    run_step "$ti" "$i" "$T/$id.maestro.$((i + 1))" "$ev" || rc=$?
    if [ "$rc" != 0 ]; then
      stop_recording; rm -f "$mov"
      if [ -f "$T/$id.maestro.$((i + 1)).log" ]; then
        cp "$T/$id.maestro.$((i + 1)).log" "$OUT/takes/$id.failed-step-$((i + 1)).log"
        die 1 "take \"$id\": step $((i + 1)) exited $rc, so the take is discarded (Maestro's log: $OUT/takes/$id.failed-step-$((i + 1)).log)"
      fi
      die 1 "take \"$id\": step $((i + 1)) exited $rc, so the take is discarded"
    fi
    i=$((i + 1))
  done
  # Logged before the stop signal: recordVideo writes no frame while the screen
  # is static, so the file can end seconds early. take_len_s is the length the
  # composer pads each take back to.
  stopped="$(now)"
  stop_recording
  [ -s "$mov" ] || die 1 "take \"$id\": recordVideo wrote no file (exit $REC_RC; log: $(tr '\n' ' ' < "$log"))"
  dur="$(probe_video "$mov" duration 2>/dev/null || true)"
  case "$dur" in ""|N/A) dur="$(ffprobe -v error -show_entries format=duration -of default=nw=1:nk=1 "$mov" 2>/dev/null || true)" ;; esac
  printf '%s' "$dur" | grep -Eq '^[0-9]+([.][0-9]+)?$' \
    || die 1 "take \"$id\": the recording is not a readable video, so the take is discarded (recordVideo exit $REC_RC; log: $(tr '\n' ' ' < "$log"))"

  find "$T" -path "$T/$id.maestro.*" -name commands.json | while IFS= read -r f; do
    step="${f#"$T/$id.maestro."}"; step="${step%%/*}"
    jq -c --argjson step "$step" '.[] | . + {step: $step}' "$f"
  done > "$T/$id.commands.jsonl" || die 1 "take \"$id\": could not read Maestro's commands.json"
  # A command Maestro did not run can lack timestamp or duration; its cue keeps
  # its status with t_s null rather than breaking the sheet. (Maestro 2.10.0 was
  # measured to stamp SKIPPED and WARNED commands too.)
  if ! jq -n --arg take "$id" --argjson started "$started" --argjson stopped "$stopped" --argjson dur "$dur" \
    --slurpfile cmds "$T/$id.commands.jsonl" --slurpfile events "$ev" '
    def r3: . * 1000 | round / 1000;
    { schema: "promo-campaign/cues-v1", take: $take, file: "\($take).mov",
      take_len_s: ($stopped - $started | r3), file_dur_s: ($dur | r3),
      cues: [$cmds[] | select((.command | type) == "object") | (.command | keys[0]) as $k
             | select($k != "defineVariablesCommand" and $k != "applyConfigurationCommand")
             | {step, name: (.command[$k].label? // $k), command: $k,
                t_s: (if (.metadata.timestamp | type) == "number" then (.metadata.timestamp / 1000 - $started | r3) else null end),
                dur_s: (if (.metadata.duration | type) == "number" then (.metadata.duration / 1000 | r3) else null end),
                status: (.metadata.status // null), depth: (.metadata.depth // 0)}]
             | sort_by(if .t_s == null then infinite else .t_s end),
      events: [$events[] | {step, kind, value, t_s: (.start - $started | r3), dur_s: (.end - .start | r3)}] }' \
    > "$OUT/takes/.$id.cues.json.tmp"; then
    rm -f "$OUT/takes/.$id.cues.json.tmp"
    die 1 "take \"$id\": could not build its cue sheet (jq error above), so the take is discarded"
  fi
  mv "$mov" "$OUT/takes/$id.mov"
  mv "$OUT/takes/.$id.cues.json.tmp" "$OUT/takes/$id.cues.json"
  nulls="$(jq '[.cues[] | select(.t_s == null)] | length' "$OUT/takes/$id.cues.json")"
  printf 'take %s: %s (take_len_s %s, file_dur_s %s, %s cues%s, %s events) + %s\n' "$id" "$OUT/takes/$id.mov" \
    "$(jq '.take_len_s' "$OUT/takes/$id.cues.json")" "$(jq '.file_dur_s' "$OUT/takes/$id.cues.json")" \
    "$(jq '.cues | length' "$OUT/takes/$id.cues.json")" "$([ "$nulls" = 0 ] || echo ", $nulls without a timestamp")" \
    "$(jq '.events | length' "$OUT/takes/$id.cues.json")" "$id.cues.json"
}

# stills_for_take <take index> <take id>: replay the steps without recording,
# screenshotting after each step a still names. Lossless PNG from simctl, never
# a frame pulled from the compressed take.
stills_for_take() {
  local ti="$1" id="$2" last i=0 rc sid
  last="$(cj --arg t "$id" '[.capture.stills // [] | .[] | select(.take == $t) | .after_step] | max // 0')"
  [ "$last" -gt 0 ] || return 0
  mkdir -p "$OUT/stills"
  while IFS= read -r sid; do rm -f "$OUT/stills/$sid.png"; done < <(cj --arg t "$id" '.capture.stills[] | select(.take == $t) | .id')
  rm -f "$OUT/stills/$id".failed-step-*.log
  prepare_app
  while [ "$i" -lt "$last" ]; do
    rc=0
    run_step "$ti" "$i" "$T/$id.stills.$((i + 1))" "" || rc=$?
    if [ "$rc" != 0 ]; then
      if [ -f "$T/$id.stills.$((i + 1)).log" ]; then
        cp "$T/$id.stills.$((i + 1)).log" "$OUT/stills/$id.failed-step-$((i + 1)).log"
        die 1 "stills for take \"$id\": step $((i + 1)) exited $rc (Maestro's log: $OUT/stills/$id.failed-step-$((i + 1)).log)"
      fi
      die 1 "stills for take \"$id\": step $((i + 1)) exited $rc"
    fi
    i=$((i + 1))
    while IFS= read -r sid; do
      if ! xcrun simctl io "$U" screenshot --type=png "$OUT/stills/$sid.png" > "$T/screenshot.log" 2>&1; then
        die 1 "still \"$sid\": simctl io screenshot failed: $(tr '\n' ' ' < "$T/screenshot.log")"
      fi
      [ -s "$OUT/stills/$sid.png" ] || die 1 "still \"$sid\": simctl io screenshot wrote no file: $(tr '\n' ' ' < "$T/screenshot.log")"
      printf 'still %s: %s (take %s, after step %s)\n' "$sid" "$OUT/stills/$sid.png" "$id" "$i"
    done < <(cj --arg t "$id" --argjson s "$i" '.capture.stills[] | select(.take == $t and .after_step == $s) | .id')
  done
}

save_clipboard

ntakes="$(cj '.capture.takes | length')"
ti=0
while [ "$ti" -lt "$ntakes" ]; do
  id="$(cj --argjson t "$ti" '.capture.takes[$t].id')"
  case "$only" in " ") record_take "$ti" "$id" ;; *" $id "*) record_take "$ti" "$id" ;; esac
  ti=$((ti + 1))
done

if [ "$want_stills" = 1 ]; then
  while IFS= read -r sid; do
    say "still $sid: no take/after_step yet, so it is not captured"
  done < <(cj '.capture.stills // [] | .[] | select(.take == null) | .id')
  ti=0
  while [ "$ti" -lt "$ntakes" ]; do
    id="$(cj --argjson t "$ti" '.capture.takes[$t].id')"
    case "$only" in " ") stills_for_take "$ti" "$id" ;; *" $id "*) stills_for_take "$ti" "$id" ;; esac
    ti=$((ti + 1))
  done
fi
