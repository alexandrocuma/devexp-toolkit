#!/bin/bash
# promo-campaign — seed backend `rn-asyncstorage` on an iOS Simulator.
#
# Usage:
#   bash seed-rn-asyncstorage-ios.sh --udid <booted Simulator UDID> --bundle-id <id> --entries <entries.json>
#   bash seed-rn-asyncstorage-ios.sh --help
#
# <entries.json> is the campaign's seed.entries as JSON, [{"key": "...", "value": "..."}],
# each value exactly as the app stores it (already serialised). capture-ios.sh
# calls this after `simctl uninstall` + `simctl install` and before the first
# `simctl launch`: state written after first launch is state the app has already
# read, and first launch is when SDKs initialise.
#
# Writes React Native AsyncStorage's iOS layout:
#   <data container>/Library/Application Support/<bundle-id>/RCTAsyncLocalStorage_V1/manifest.json
# A value longer than 1024 UTF-16 code units (NSString length, AsyncStorage's
# inline threshold) is written to a file in the same directory, named the
# lowercase MD5 hex of the UTF-8 key, and the manifest holds null for that key.
# A fresh install has no Library/Application Support, so it is created.
# Refuses to write over storage that already exists.
#
# Exit: 0 seeded; 64 usage; 65 bad entries or storage already present;
# 66 app not installed; 69 missing prerequisite.
set -euo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
. "$HERE/lib/common.sh"

udid=""; bid=""; entries=""
while [ $# -gt 0 ]; do
  case "$1" in
    --udid|--bundle-id|--entries)
      [ $# -ge 2 ] || die 64 "$1 needs a value"
      case "$1" in --udid) udid="$2" ;; --bundle-id) bid="$2" ;; --entries) entries="$2" ;; esac
      shift 2 ;;
    -h|--help) usage_from_header "${BASH_SOURCE[0]}"; exit 0 ;;
    *) die 64 "unknown argument '$1' (see --help)" ;;
  esac
done
[ -n "$udid" ] && [ -n "$bid" ] && [ -n "$entries" ] || die 64 "--udid, --bundle-id and --entries are all required (see --help)"
need jq perl xcrun
ensure_developer_dir

[ -f "$entries" ] || die 66 "entries file '$entries' not found"
jq -e 'type == "array" and all(.[]; type == "object" and (.key | type) == "string" and (.key | length) > 0 and (.value | type) == "string")' \
  "$entries" >/dev/null || die 65 "'$entries' must be a JSON list of {\"key\": string, \"value\": string}"

data="$(xcrun simctl get_app_container "$udid" "$bid" data 2>/dev/null)" \
  || die 66 "$bid is not installed on $udid: simctl install it before seeding"
store="$data/Library/Application Support/$bid/RCTAsyncLocalStorage_V1"
[ ! -e "$store" ] || die 65 "AsyncStorage already exists for $bid on $udid: the app has run or was seeded. Uninstall and install it again, then seed before the first launch"
mkdir -p "$store"

U16='def u16: [explode[] | if . > 65535 then 2 else 1 end] | add // 0;'
n="$(jq 'length' "$entries")"; i=0; spilled=0
while [ "$i" -lt "$n" ]; do
  if [ "$(jq "$U16"' .[$i].value | u16' --argjson i "$i" "$entries")" -gt 1024 ]; then
    hash="$(jq -j --argjson i "$i" '.[$i].key' "$entries" | perl -MDigest::MD5 -e 'local $/; print Digest::MD5::md5_hex(<STDIN>)')"
    jq -j --argjson i "$i" '.[$i].value' "$entries" > "$store/$hash"
    spilled=$((spilled + 1))
  fi
  i=$((i + 1))
done
jq "$U16"' reduce .[] as $e ({}; .[$e.key] = (if ($e.value | u16) > 1024 then null else $e.value end))' \
  "$entries" > "$store/manifest.json"
say "seeded $n AsyncStorage entries for $bid ($spilled spilled to files)"
