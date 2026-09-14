#!/bin/bash
# promo-campaign — read and type-check the campaign's YAML front matter.
#
# Usage:
#   bash contract.sh validate <campaign.md> capture   # before capture: seed, capture.ios, takes, stills
#   bash contract.sh validate <campaign.md> [all]     # after the cue sheets exist: everything, capture.cut included
#   bash contract.sh validate <campaign.md> compose   # what compose-reel.sh reads
#   bash contract.sh json <campaign.md>               # print the front matter as JSON
# Sourced by capture-ios.sh (capture) and compose-reel.sh (compose).
#
# YAML -> JSON with the first parser that works: ruby (YAML.safe_load, ships
# with macOS) -> python3 with PyYAML -> yq v4. Before loading, every parser path
# applies the same parser-proof lint as SKILL.md's Phase 6 check, naming the key
# path and line:
# - a plain key that loads as a boolean or null;
# - a plain value that is not a decimal number, true, false or null. Psych reads
#   0:05.5 as 330.0 and PyYAML as 5.5, and `no` becomes false;
# - an anchor, an alias, or a duplicate key. Parsers resolve or silently keep
#   one of those, and do not all agree which.
#
# The jq pass type-checks every key the scripts read. For keys SKILL.md's check
# also reads, the type rules are its rules. Storyboard limits are SKILL.md's
# (Phase 6); this file owns seed, capture, outputs.dir and the render limits the
# composer depends on (caption windows inside the footage).

[ -n "${PROMO_LIB_DIR:-}" ] || . "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/common.sh"

PROMO_SEED_BACKENDS="rn-asyncstorage"

yaml_parser() {
  if command -v ruby >/dev/null 2>&1 && ruby -ryaml -rjson -e 'exit 0' >/dev/null 2>&1; then echo ruby
  elif command -v python3 >/dev/null 2>&1 && python3 -c 'import yaml, json' >/dev/null 2>&1; then echo python3
  elif command -v yq >/dev/null 2>&1 && yq --version 2>&1 | grep -q 'mikefarah.*version v4\.'; then echo yq
  else return 1
  fi
}

# contract_frontmatter <file>: the text between the first two `---` lines, which
# may only be preceded by blank lines (the same split as SKILL.md's check). The
# lines before it come out blank, so parser line numbers are the file's.
contract_frontmatter() {
  awk 's == 0 { if ($0 ~ /^---[ \t]*$/) { s = 1; print ""; next } if ($0 ~ /^[ \t]*$/) { print ""; next } exit 3 }
       s == 1 { if ($0 ~ /^---[ \t]*$/) { s = 2; exit 0 } print }
       END { if (s != 2) exit 3 }' "$1"
}

ruby_program() {
cat <<'RB'
require "yaml"; require "json"
text = File.read(ARGV[0], encoding: "UTF-8"); err = []
key_re = /\A(y|yes|n|no|on|off|true|false|null|~)\z/i
val_re = /\A(-?\d+(\.\d+)?|true|false|null)\z/
walk = lambda do |n, path|
  where = path.empty? ? "top level" : path
  if n.is_a?(Psych::Nodes::Alias)
    err << "#{where} (line #{n.start_line + 1}): YAML alias *#{n.anchor} is not allowed; write the value out"
    next
  end
  err << "#{where} (line #{n.start_line + 1}): YAML anchor &#{n.anchor} is not allowed; write the value out" if n.respond_to?(:anchor) && n.anchor
  case n
  when Psych::Nodes::Mapping
    seen = {}
    n.children.each_slice(2) do |k, v|
      if k.is_a?(Psych::Nodes::Scalar)
        err << "#{where} (line #{k.start_line + 1}): quote the key #{k.value.inspect}, it loads as a boolean or null" if k.plain && k.value =~ key_re
        err << "#{where} (line #{k.start_line + 1}): YAML anchor &#{k.anchor} is not allowed; write the value out" if k.anchor
        if seen.key?(k.value)
          err << "#{where} (line #{k.start_line + 1}): duplicate key #{k.value.inspect} (first at line #{seen[k.value]}); a parser would silently keep one"
        else
          seen[k.value] = k.start_line + 1
        end
        name = k.value
      else
        walk.(k, path); name = "?"
      end
      walk.(v, path.empty? ? name : "#{path}.#{name}")
    end
  when Psych::Nodes::Sequence then n.children.each_with_index { |c, i| walk.(c, "#{path}[#{i}]") }
  when Psych::Nodes::Scalar
    err << "#{where} (line #{n.start_line + 1}): #{n.value.inspect} must be \"quoted\", a decimal number, true, false or null" if n.plain && n.value !~ val_re
  else (n.children || []).each { |c| walk.(c, path) }
  end
end
begin
  doc = Psych.parse(text); walk.(doc, "") if doc
  unless err.empty?
    warn err.each_with_index.sort_by { |e, i| [e[/\(line (\d+)\)/, 1].to_i, i] }.map(&:first).uniq.join("\n")
    exit 65
  end
  fm = YAML.safe_load(text)
rescue Psych::Exception => e
  warn "front matter does not parse: #{e.message}"; exit 65
end
puts JSON.generate(fm)
RB
}

python_program() {
cat <<'PY'
import json, re, sys, yaml
KEY = re.compile(r'(y|yes|n|no|on|off|true|false|null|~)\Z', re.I | re.A)
VAL = re.compile(r'(-?\d+(\.\d+)?|true|false|null)\Z', re.A)
text = open(sys.argv[1], encoding='utf-8').read(); err = []

class StrictLoader(yaml.SafeLoader):
    """SafeLoader that records every anchor and alias with its key path, and raises on a duplicate key."""
    def __init__(self, stream):
        yaml.SafeLoader.__init__(self, stream)
        self.paths = []
    def compose_node(self, parent, index):
        base = self.paths[-1] if self.paths else ''
        if isinstance(index, yaml.ScalarNode):
            path = base + '.' + index.value if base else index.value
        elif isinstance(index, int) and not isinstance(index, bool):
            path = '%s[%d]' % (base, index)
        else:
            path = base
        ev = self.peek_event()
        where, line = path or 'top level', ev.start_mark.line + 1
        if isinstance(ev, yaml.AliasEvent):
            err.append('%s (line %d): YAML alias *%s is not allowed; write the value out' % (where, line, ev.anchor))
        elif getattr(ev, 'anchor', None):
            err.append('%s (line %d): YAML anchor &%s is not allowed; write the value out' % (where, line, ev.anchor))
        self.paths.append(path)
        try:
            return yaml.SafeLoader.compose_node(self, parent, index)
        finally:
            self.paths.pop()
    def construct_mapping(self, node, deep=False):
        seen = set()
        for k, _ in node.value:
            if isinstance(k, yaml.ScalarNode):
                if k.value in seen:
                    raise yaml.constructor.ConstructorError(None, None, 'duplicate key %s' % json.dumps(k.value), k.start_mark)
                seen.add(k.value)
        return yaml.SafeLoader.construct_mapping(self, node, deep)

def walk(n, path, visited):
    if id(n) in visited:
        return
    visited.add(id(n))
    where = path or 'top level'
    if isinstance(n, yaml.MappingNode):
        seen = {}
        for k, v in n.value:
            if isinstance(k, yaml.ScalarNode):
                if k.style is None and KEY.match(k.value):
                    err.append('%s (line %d): quote the key %s, it loads as a boolean or null' % (where, k.start_mark.line + 1, json.dumps(k.value, ensure_ascii=False)))
                if k.value in seen:
                    err.append('%s (line %d): duplicate key %s (first at line %d); a parser would silently keep one' % (where, k.start_mark.line + 1, json.dumps(k.value, ensure_ascii=False), seen[k.value]))
                else:
                    seen[k.value] = k.start_mark.line + 1
                name = k.value
            else:
                walk(k, path, visited); name = '?'
            walk(v, path + '.' + name if path else name, visited)
    elif isinstance(n, yaml.SequenceNode):
        for i, c in enumerate(n.value):
            walk(c, '%s[%d]' % (path, i), visited)
    elif isinstance(n, yaml.ScalarNode):
        if n.style is None and not VAL.match(n.value):
            err.append('%s (line %d): %s must be "quoted", a decimal number, true, false or null' % (where, n.start_mark.line + 1, json.dumps(n.value, ensure_ascii=False)))
try:
    node = yaml.compose(text, Loader=StrictLoader)
    if node is not None:
        walk(node, '', set())
    if err:
        line = lambda e: int(re.search(r'\(line (\d+)\)', e).group(1))
        out = []
        for e in sorted(err, key=line):
            if e not in out:
                out.append(e)
        sys.stderr.write('\n'.join(out) + '\n'); sys.exit(65)
    fm = yaml.load(text, Loader=StrictLoader)
except yaml.YAMLError as e:
    sys.stderr.write('front matter does not parse: %s\n' % e); sys.exit(65)
json.dump(fm, sys.stdout, ensure_ascii=False)
PY
}

# yq_convert <front matter file>: the same lint with yq v4's node style, line,
# anchor and kind operators, then the conversion. yq resolves aliases and keeps
# duplicate keys in its JSON, so both are detected explicitly first. Keys are
# collected per mapping as parallel arrays: building one object per key with
# `keys[] | select(style == "")` lost every field for 4 of 115 keys on a fixture
# (yq v4.53). stdin is closed on every call so yq never reads a caller's pipe.
yq_convert() {
  local errs off
  # yq numbers lines from the first non-blank line; add the blank lines that
  # stand in for the opening `---`, so its lines match the file's.
  off="$(awk 'NF { exit } { n++ } END { print n + 0 }' "$1")"
  errs="$({ yq -o=json -I=0 '[.. | select(kind == "map") | {"mp": path, "keys": [keys[] | to_string], "styles": [keys[] | style], "lines": [keys[] | line], "anchors": [keys[] | anchor]}]' "$1" < /dev/null
            yq -o=json -I=0 '[.. | select(kind == "scalar" and style == "") | {"p": path, "l": line, "v": (. | to_string)}]' "$1" < /dev/null
            yq -o=json -I=0 '[.. | select(kind == "alias" or anchor != "") | {"p": path, "l": line, "an": anchor, "isalias": (kind == "alias"), "al": alias}]' "$1" < /dev/null
          } | jq -rs --argjson off "$off" '
    def fp: reduce .[] as $s (""; if ($s | type) == "number" then . + "[\($s)]" elif . == "" then $s else . + "." + $s end);
    def where: fp | if . == "" then "top level" else . end;
    [ (.[0][] | . as $m | range(0; $m.keys | length) as $i
        | ( (select($m.styles[$i] == "" and ($m.keys[$i] | test("^(y|yes|n|no|on|off|true|false|null|~)$"; "i")))
              | {l: $m.lines[$i], e: "\($m.mp | where) (line \($m.lines[$i] + $off)): quote the key \($m.keys[$i] | tojson), it loads as a boolean or null"}),
            (select($m.anchors[$i] != "")
              | {l: $m.lines[$i], e: "\($m.mp | where) (line \($m.lines[$i] + $off)): YAML anchor &\($m.anchors[$i]) is not allowed; write the value out"}),
            (($m.keys[:$i] | index($m.keys[$i])) as $first | select($first != null)
              | {l: $m.lines[$i], e: "\($m.mp | where) (line \($m.lines[$i] + $off)): duplicate key \($m.keys[$i] | tojson) (first at line \($m.lines[$first] + $off)); a parser would silently keep one"}) )),
      (.[1][] | select(.v | test("^(-?[0-9]+([.][0-9]+)?|true|false|null)$") | not)
        | {l: .l, e: "\(.p | where) (line \(.l + $off)): \(.v | tojson) must be \"quoted\", a decimal number, true, false or null"}),
      (.[2][] | if .isalias then {l: .l, e: "\(.p | where) (line \(.l + $off)): YAML alias *\(.al) is not allowed; write the value out"}
                else {l: .l, e: "\(.p | where) (line \(.l + $off)): YAML anchor &\(.an) is not allowed; write the value out"} end)
    ] | sort_by(.l) | map(.e) | reduce .[] as $e ([]; if index([$e]) then . else . + [$e] end) | .[]')" \
    || { say "front matter does not parse (yq)"; return 65; }
  if [ -n "$errs" ]; then printf '%s\n' "$errs" >&2; return 65; fi
  yq -o=json -I=0 '.' "$1" < /dev/null
}

# contract_load <campaign.md> <scratch dir>: sets CONTRACT_JSON and CONTRACT_PARSER.
contract_load() {
  [ -f "$1" ] || die 66 "campaign file '$1' not found"
  local fm="$2/frontmatter.yaml" rc=0
  CONTRACT_JSON="$2/contract.json"
  contract_frontmatter "$1" > "$fm" || die 65 "$1: no YAML front matter between two --- lines"
  CONTRACT_PARSER="$(yaml_parser)" || die 69 "no YAML parser: install ruby, python3 with PyYAML, or yq v4"
  case "$CONTRACT_PARSER" in
    ruby)    ruby -e "$(ruby_program)" "$fm" > "$CONTRACT_JSON" || rc=$? ;;
    python3) python3 -c "$(python_program)" "$fm" > "$CONTRACT_JSON" || rc=$? ;;
    yq)      yq_convert "$fm" > "$CONTRACT_JSON" || rc=$? ;;
  esac
  [ "$rc" = 0 ] || die 65 "$1: front matter rejected ($CONTRACT_PARSER); fix the lines above"
  jq -e 'type == "object"' "$CONTRACT_JSON" >/dev/null || die 65 "$1: front matter must be a mapping"
  jq -e '.schema == "promo-campaign/v1"' "$CONTRACT_JSON" >/dev/null \
    || die 65 "$1: schema $(jq -c '.schema' "$CONTRACT_JSON") is not \"promo-campaign/v1\": stop, never migrate silently"
}

# cj [jq options...] <jq filter>: read the loaded contract (raw output).
cj() { jq -r "$@" "$CONTRACT_JSON"; }

validate_program() {
cat <<'JQ'
def show: if . == null then "nothing" else tojson end;
def num($p): if type == "number" then empty else "\($p) must be a number in decimal seconds, got \(show)" end;
def lst($p): if type == "array" then empty else "\($p) must be a list, got \(show)" end;
def mp($p): if type == "object" then empty else "\($p) must be a mapping, got \(show)" end;
def str($p): if type == "string" then empty else "\($p) must be a \"quoted\" string, got \(show)" end;
def ostr($p): if . == null then empty else str($p) end;
def aslst: if type == "array" then . else [] end;
def asmp: if type == "object" then . else {} end;
def opt($k; $d): if type == "object" and has($k) then .[$k] else $d end;
def maplist($p): lst($p), (aslst | to_entries[] | .key as $i | .value | mp("\($p)[\($i)]"));
def items: aslst | to_entries[] | select(.value | type == "object");
def slug($p): if type == "string" and test("^[A-Za-z0-9][A-Za-z0-9._-]*$") then empty
  else "\($p) must be a \"quoted\" id of letters, digits, '.', '_' or '-', got \(show)" end;
def enum($p; $vals): if IN($vals[]) then empty else "\($p) must be one of \($vals | map(tojson) | join(", ")), got \(show)" end;
def colour($p): if . == null or (type == "string" and test("^#[0-9A-Fa-f]{6}$")) then empty else "\($p) must be a \"#RRGGBB\" colour, got \(show)" end;
def dups($p): group_by(.) | map(select(length > 1) | .[0])[] | "\($p) \(tojson) is used more than once";
def seglen: if (.in_s | type) == "number" and (.out_s | type) == "number" then .out_s - .in_s else null end;
def jn: if .join == null then "cut" else .join end;
def r3: . * 1000 | round / 1000;

. as $fm
| ($fm | opt("capture"; {}) | asmp) as $cap
| ($cap.takes | aslst | map(objects | select(.id | type == "string")
    | {key: .id, value: (.steps | if type == "array" then length else 0 end)}) | from_entries) as $tmap
| ([$fm.beats | aslst[] | objects | .target_s] | if length > 0 and all(.[]; type == "number") then add else null end) as $beats
| [
  # outputs.dir and capture: both modes
  ($fm.outputs | asmp | .dir | if type != "string" then "outputs.dir must be a \"quoted\" repo-relative path, got \(show)"
     elif . == "" or test("^[/~]") or test("(^|/)[.]{1,2}(/|$)")
       then "outputs.dir \(tojson) must be a repo-relative subdirectory, without '.' or '..' segments" else empty end),
  ($fm | opt("capture"; {}) | mp("capture")),
  ($cap.takes | if type != "array" or length == 0 then "capture.takes must be a non-empty list of takes, got \(show)" else
     (to_entries[] | .key as $i | .value | "capture.takes[\($i)]" as $p |
       if type != "object" then mp($p) else
         (.id | slug("\($p).id")),
         (.steps | if type != "array" or length == 0 then "\($p).steps must be a non-empty list, got \(show)" else
            (to_entries[] | .key as $j | .value | "\($p).steps[\($j)]" as $sp |
              if type != "object" then mp($sp)
              elif (keys | length) != 1 or (keys[0] | IN("maestro", "hold_s", "appearance") | not)
                then "\($sp) must have exactly one of maestro, hold_s, appearance; got keys \(keys | tojson)"
              elif has("maestro") then .maestro | str("\($sp).maestro")
              elif has("hold_s") then .hold_s | if type == "number" and . > 0 then empty else "\($sp).hold_s must be a number of seconds > 0, got \(show)" end
              else .appearance | enum("\($sp).appearance"; ["light", "dark"]) end)
          end)
       end),
     ([.[] | objects | .id | strings] | dups("capture.takes id"))
   end),

  if $mode == "capture" or $mode == "all" then
    (if $fm | has("seed") then $fm.seed | mp("seed") else empty end),
    ($fm | opt("seed"; {}) | objects | select(length > 0) |
      (.backend | if type == "string" and IN($backends | split(" ")[]) then empty
         else "seed.backend \(show) is not supported; supported backends: \($backends | split(" ") | join(", "))" end),
      (.entries | if type != "array" or length == 0 then "seed.entries must be a non-empty list of { key, value }, got \(show)" else
         (to_entries[] | .key as $i | .value | "seed.entries[\($i)]" as $p |
           if type != "object" then mp($p) else
             (.key | if type == "string" and length > 0 then empty else "\($p).key must be a non-empty \"quoted\" string, got \(show)" end),
             (.value | if type == "string" then empty else "\($p).value must be a \"quoted\" string holding the value exactly as the app stores it (already serialised), got \(show)" end)
           end),
         ([.[] | objects | .key | strings] | dups("seed.entries key"))
       end)),
    ($cap.ios | if type != "object" then "capture.ios must be a mapping (app, bundle_id, device, status_bar, appearance), got \(show)" else
       (.app | str("capture.ios.app")),
       (.bundle_id | if type == "string" and test("^[A-Za-z0-9-]+([.][A-Za-z0-9-]+)*$") then empty
          else "capture.ios.bundle_id must be a \"quoted\" reverse-DNS id (letters, digits, '-' and '.'), got \(show)" end),
       (.device | ostr("capture.ios.device")),
       (if .appearance == null then empty else .appearance | enum("capture.ios.appearance"; ["light", "dark"]) end),
       (.status_bar | if . == null then empty elif type != "object" then mp("capture.ios.status_bar") else
          (.time | ostr("capture.ios.status_bar.time")),
          (.battery_level | if . == null or (type == "number" and . >= 0 and . <= 100 and floor == .) then empty
             else "capture.ios.status_bar.battery_level must be a whole number from 0 to 100, got \(show)" end)
        end)
     end),
    ($cap | opt("stills"; []) | maplist("capture.stills")),
    ($cap | opt("stills"; []) | items | .key as $i | .value | "capture.stills[\($i)]" as $p |
      (.id | slug("\($p).id")),
      (if has("take") or has("after_step") then
         (.take as $t | if ($t | type) == "string" and ($tmap | has($t)) then
              (.after_step | if type == "number" and floor == . and . >= 1 and . <= $tmap[$t] then empty
                 else "\($p).after_step must be a whole step number from 1 to \($tmap[$t]) (the steps of take \($t | tojson)), got \(show)" end)
            else "\($p).take must name a capture.takes id, got \($t | show)" end)
       else empty end)),
    ($cap | opt("stills"; []) | [aslst[] | objects | .id | strings] | dups("capture.stills id"))
  else empty end,

  if $mode == "compose" or $mode == "all" then
    # Keys SKILL.md's Phase 6 check also reads: the same types, the same defaults.
    ($fm.beats | maplist("beats")),
    ($fm.beats | items | .key as $i | .value.target_s | num("beats[\($i)].target_s")),
    ($fm.end_card | mp("end_card")),
    ($fm.end_card | asmp | (.duration_s | num("end_card.duration_s")), (.transition_s | num("end_card.transition_s"))),
    ($fm.end_card | asmp | opt("badges"; []) | maplist("end_card.badges")),
    ($fm.end_card | asmp | opt("badges"; []) | items | .key as $i | .value |
      (.path | str("end_card.badges[\($i)].path")), (.locale | str("end_card.badges[\($i)].locale"))),
    (if $fm | has("max_duration_s") then $fm.max_duration_s | num("max_duration_s") else empty end),
    ($fm.locales | lst("locales")),
    ($fm.locales | aslst | to_entries[] | .key as $i | .value |
      if type != "string" then str("locales[\($i)]")
      elif test("^[a-z]{2,3}(-[A-Za-z0-9]{2,8})*$") | not then "locales[\($i)] \(tojson) must be a locale like \"en\", \"es\" or \"pt-BR\" (it names output files)"
      else empty end),
    ($fm.outputs | mp("outputs")), ($fm.outputs | asmp | .formats | lst("outputs.formats")),
    ($fm.app | mp("app")),
    ($fm.app | asmp | (.tagline | mp("app.tagline")), (.name | str("app.name")), (.icon | str("app.icon"))),
    ($fm.captions | mp("captions")),
    ($fm.locales | aslst[] | strings | . as $l |
      ($fm.app | asmp | .tagline | asmp | .[$l] | ostr("app.tagline.\($l)")),
      ($fm.captions | asmp | opt($l; []) | maplist("captions.\($l)")),
      ($fm.captions | asmp | opt($l; []) | items | .key as $i | .value | "captions.\($l)[\($i)]" as $p |
        (.start_s | num("\($p).start_s")), (.end_s | num("\($p).end_s")), (.text | str("\($p).text")),
        # The composer draws each caption over [start_s, end_s) of the footage; the
        # end card and its transition start at the sum of the beats.
        (.start_s as $s | .end_s as $e |
          (if ($s | type) == "number" and $s < 0 then "\($p).start_s \($s) must be >= 0" else empty end),
          (if ($s | type) == "number" and ($e | type) == "number" and $s >= $e then "\($p): start_s \($s) must be before end_s \($e)" else empty end),
          (if $beats != null and ($e | type) == "number" and $e > $beats + (1 / 60)
           then "\($p).end_s \($e) is past the end-card start at \($beats | r3)s (the sum of beats[].target_s): no caption may run into the end card or its transition" else empty end)))),
    ($cap | opt("stills"; []) | maplist("capture.stills")),
    # capture.cut and capture.compose: this tooling's own keys.
    ($cap.cut | if type != "array" or length == 0 then "capture.cut must be a non-empty list of segments, got \(show) (fill it from the cue sheets after capture; before capture, validate with mode capture)" else
       . as $c | length as $n |
       (to_entries[] | .key as $i | .value | "capture.cut[\($i)]" as $p |
         if type != "object" then mp($p) else
           (.take as $tk | if ($tk | type) == "string" and ($tmap | has($tk)) then empty else "\($p).take must name a capture.takes id, got \($tk | show)" end),
           (.in_s | if type == "number" and . >= 0 then empty else "\($p).in_s must be a number of seconds >= 0, got \(show)" end),
           (.out_s | num("\($p).out_s")),
           (if seglen != null and seglen <= 0 then "\($p): out_s \(.out_s) must be greater than in_s \(.in_s)" else empty end),
           (if $i == $n - 1 then
              (if .join != null or .fade_s != null then "\($p) is the last segment: remove join and fade_s (the end card has its own transition)" else empty end)
            else
              (jn | enum("\($p).join"; ["cut", "fade"])),
              (if jn == "fade" then
                 (.fade_s | if type == "number" and . > 0 then empty else "\($p).fade_s must be a number of seconds > 0 when join is \"fade\", got \(show)" end),
                 (seglen as $a | ($c[$i + 1] | seglen) as $b | .fade_s as $f |
                   if ($f | type) == "number" and $a != null and $b != null and ($f >= $a or $f >= $b)
                   then "\($p).fade_s \($f) must be shorter than both segments it joins (\($a) and \($b) seconds)" else empty end)
               elif .fade_s != null then "\($p).fade_s is set but join is not \"fade\""
               else empty end)
            end)
         end)
     end),
    ($cap | opt("compose"; {}) | if type != "object" then mp("capture.compose") else
       (.font | ostr("capture.compose.font")), (.font_bold | ostr("capture.compose.font_bold")),
       (.background | colour("capture.compose.background")), (.text_color | colour("capture.compose.text_color"))
     end)
  else empty end
]
| reduce .[] as $e ([]; if any(.[]; . == $e) then . else . + [$e] end) | .[]
JQ
}

# contract_validate <capture|compose|all>: every error at once, then exit 65.
contract_validate() {
  local errs
  errs="$(jq -r --arg mode "$1" --arg backends "$PROMO_SEED_BACKENDS" "$(validate_program)" "$CONTRACT_JSON")" \
    || die 1 "internal error: the contract type check itself failed (jq output above)"
  if [ -n "$errs" ]; then
    printf '%s\n' "$errs" | sed 's/^/  - /' >&2
    die 65 "campaign failed validation for $1 ($(printf '%s\n' "$errs" | wc -l | tr -d ' ') errors above)"
  fi
}

if [ "${BASH_SOURCE[0]}" = "$0" ]; then
  set -euo pipefail
  case "${1:-}" in
    validate|json)
      [ $# -ge 2 ] || die 64 "usage: bash contract.sh $1 <campaign.md>$([ "$1" = validate ] && echo ' [capture|compose|all]')"
      need jq yaml
      CT="$(make_tmp)"; trap 'rm -rf "$CT"' EXIT
      contract_load "$2" "$CT"
      if [ "$1" = json ]; then jq . "$CONTRACT_JSON"; exit 0; fi
      case "${3:-all}" in capture|compose|all) ;; *) die 64 "mode must be capture, compose or all" ;; esac
      contract_validate "${3:-all}"
      echo "contract ok (${3:-all}, parsed with $CONTRACT_PARSER): $2" ;;
    -h|--help) usage_from_header "${BASH_SOURCE[0]}"; exit 0 ;;
    *) usage_from_header "${BASH_SOURCE[0]}" >&2; exit 64 ;;
  esac
fi
