#!/bin/bash
# promo-campaign — shared helpers, sourced by every script in scripts/.
#
# Rules every script here follows, and why:
# - /bin/bash 3.2 (macOS's bash). No `timeout` (macOS lacks it: with_timeout uses
#   a perl alarm), no EPOCHREALTIME (now() uses perl Time::HiRes), no associative
#   arrays, no mapfile. Counters are `i=$((i+1))`: `((i++))` returns 1 when i is 0,
#   and set -e then exits.
# - Invoked as `bash <path>`. `devexp install` copies skill files with mode 0644,
#   so `./script.sh` fails. Siblings are found from ${BASH_SOURCE[0]}, never from
#   $CLAUDE_SKILL_DIR, which is a SKILL.md text substitution and not an env var.
# - Writes only to the campaign's outputs.dir and to make_tmp directories. Never to
#   the skill's install directory: a reinstall overwrites it, and removing the
#   skill deletes it recursively.

PROMO_LIB_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROMO_SCRIPTS_DIR="$(cd "$PROMO_LIB_DIR/.." && pwd)"

# Exit codes: 64 usage, 65 bad campaign data, 66 missing input file,
# 69 missing prerequisite or device, 70 a self-check failed, 1 anything else.
say() { printf '%s\n' "$*" >&2; }
die() {
  local code="$1"; shift
  printf 'promo-campaign: %s\n' "$*" >&2
  exit "$code"
}

# usage_from_header <script path>: print the script's leading comment block.
usage_from_header() {
  awk 'NR == 1 { next } /^#/ { sub(/^# ?/, ""); print; next } { exit }' "$1"
}

# now: wall-clock seconds with millisecond precision.
now() { perl -MTime::HiRes=time -e 'printf "%.3f\n", time'; }

# with_timeout <seconds> <cmd> [args...]: run cmd, SIGALRM after <seconds>.
# The alarm survives exec, so the command itself is killed; status 142 = timed out.
with_timeout() {
  local s="$1"; shift
  perl -e 'alarm shift @ARGV; exec @ARGV or die "exec $ARGV[0]: $!\n"' "$s" "$@"
}

# calc <awk expression>: floating-point arithmetic, printed with 6 decimals.
calc() { awk "BEGIN { printf \"%.6f\n\", ($1) }"; }
# fle <a> <b> [epsilon]: true when a <= b + epsilon.
fle() { awk -v a="$1" -v b="$2" -v e="${3:-0}" 'BEGIN { exit !(a <= b + e) }'; }
# fabs_le <a> <b> <tolerance>: true when |a - b| <= tolerance.
fabs_le() { awk -v a="$1" -v b="$2" -v t="$3" 'BEGIN { d = a - b; if (d < 0) d = -d; exit !(d <= t + 0.000001) }'; }

hint_for() {
  case "$1" in
    ffmpeg|ffprobe) echo "brew install ffmpeg   (Debian/Ubuntu: apt-get install ffmpeg)" ;;
    magick)         echo "brew install imagemagick   (ImageMagick 7 provides the 'magick' command)" ;;
    jq)             echo "brew install jq   (Debian/Ubuntu: apt-get install jq)" ;;
    perl)           echo "perl ships with macOS; Debian/Ubuntu: apt-get install perl" ;;
    maestro)        echo "curl -fsSL https://get.maestro.mobile.dev | bash   (then put ~/.maestro/bin on PATH)" ;;
    xcrun)          echo "install Xcode from the App Store; simctl ships only with Xcode" ;;
    *)              echo "install '$1' and put it on PATH" ;;
  esac
}

# need <tool>... : check every prerequisite up front and report all missing ones
# at once, each with an install hint. The pseudo-tool `yaml` checks for a YAML
# parser (yaml_parser, lib/contract.sh).
need() {
  local t missing=""
  for t in "$@"; do
    if [ "$t" = yaml ]; then
      if ! yaml_parser >/dev/null 2>&1; then
        say "  missing: a YAML parser. Tried, in order: ruby (YAML.safe_load; ships with macOS), python3 with PyYAML (pip3 install pyyaml), yq v4 (brew install yq)"
        missing="$missing yaml"
      fi
    elif ! command -v "$t" >/dev/null 2>&1; then
      say "  missing: $t — $(hint_for "$t")"
      missing="$missing $t"
    fi
  done
  [ -z "$missing" ] || die 69 "prerequisites missing:$missing (install hints above)"
}

# ensure_developer_dir: simctl and Maestro's iOS driver need full Xcode. When
# xcode-select points at the Command Line Tools and Xcode is installed, export
# DEVELOPER_DIR for this process and its children only. A DEVELOPER_DIR already
# in the environment is respected.
ensure_developer_dir() {
  [ -n "${DEVELOPER_DIR:-}" ] && return 0
  local p
  p="$(xcode-select -p 2>/dev/null || true)"
  case "$p" in
    ""|*CommandLineTools*)
      if [ -d /Applications/Xcode.app/Contents/Developer ]; then
        export DEVELOPER_DIR=/Applications/Xcode.app/Contents/Developer
      else
        die 69 "xcode-select points at '${p:-nothing}' and /Applications/Xcode.app is absent: install Xcode, or export DEVELOPER_DIR"
      fi ;;
  esac
}

# make_tmp: a fresh, prefix-anchored scratch directory (SKILL.md Safety Rule 8).
make_tmp() { mktemp -d /tmp/.promo-campaign-XXXXXX; }

# abs_path <path>: absolute path of an existing file or directory.
abs_path() {
  if [ -d "$1" ]; then (cd "$1" && pwd -P)
  else printf '%s/%s\n' "$(cd "$(dirname "$1")" && pwd -P)" "$(basename "$1")"; fi
}

# repo_path <repo root> <path from the campaign>: absolute paths pass through,
# anything else is relative to the consumer repo root.
repo_path() {
  case "$2" in
    /*) printf '%s\n' "$2" ;;
    *)  printf '%s/%s\n' "$1" "$2" ;;
  esac
}

# resolve_repo <campaign file> <--repo value or empty>: the consumer repo root.
resolve_repo() {
  if [ -n "$2" ]; then
    [ -d "$2" ] || die 66 "--repo '$2' is not a directory"
    abs_path "$2"
    return
  fi
  git -C "$(dirname "$1")" rev-parse --show-toplevel 2>/dev/null \
    || die 64 "cannot find the repo root from '$1': pass --repo <absolute path>"
}

# resolve_font <what> <env value> <contract value> <default>: environment beats
# the campaign, which beats the macOS default. The result must exist.
resolve_font() {
  local f="${2:-${3:-$4}}"
  [ -f "$f" ] || die 66 "$1 font '$f' not found: set capture.compose.$1 in the campaign, or PROMO_CAMPAIGN_$(printf '%s' "$1" | tr '[:lower:]' '[:upper:]'), to a .ttf/.otf path"
  printf '%s\n' "$f"
}

# probe_video <file> <field>: one field of the first video stream (ffprobe 9
# rejects multi-field csv with a space separator, so fields are queried singly).
probe_video() {
  ffprobe -v error -select_streams v:0 -show_entries "stream=$2" -of default=nw=1:nk=1 "$1" | head -1
}
