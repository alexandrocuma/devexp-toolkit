#!/bin/bash
# promo-campaign — canvas geometry and the still layers the composer overlays.
# Sourced by compose-reel.sh after lib/common.sh. ImageMagick 7 (`magick`).

# window <src w> <src h> <canvas w> <canvas h> <top band> <bottom band> <margin>
# Prints "W H X Y": the largest window with the source's aspect ratio that fits
# between the caption band and the footer, inside the side margins, rounded to
# even dimensions (H.264 needs them). Deriving it from the source, rather than
# hard-coding one window, is what keeps a 1206x2622 take from being stretched.
window() {
  awk -v sw="$1" -v sh="$2" -v cw="$3" -v ch="$4" -v top="$5" -v bot="$6" -v m="$7" 'BEGIN {
    bw = cw - 2 * m; bh = ch - top - bot; h = bh; w = h * sw / sh
    if (w > bw) { w = bw; h = w * sh / sw }
    w = 2 * int(w / 2 + 0.5); h = 2 * int(h / 2 + 0.5)
    printf "%d %d %d %d\n", w, h, int((cw - w) / 2), top + int((bh - h) / 2) }'
}

# build_frame <out.png> <canvas w> <canvas h> <win w> <win h> <win x> <win y> <bg colour> <bezel colour>
# The layer drawn over the footage: background with a transparent rounded hole
# where the window is, then a bezel stroked around the hole. Without `-alpha set`
# the canvas has no alpha channel, so the hole stays opaque and the frame hides
# the footage; `PNG32:` keeps the encoder from dropping the channel.
# assert_frame proves the result, whatever the build did.
build_frame() {
  local out="$1" cw="$2" ch="$3" w="$4" h="$5" x="$6" y="$7" bg="$8" bezel="$9"
  local b=12 r
  r=$(( w * 9 / 100 ))
  magick -size "${cw}x${ch}" "xc:$bg" -alpha set \
    \( -size "${cw}x${ch}" xc:none -fill white -draw "roundrectangle $x,$y $((x + w - 1)),$((y + h - 1)) $r,$r" \) \
    -compose DstOut -composite -compose over \
    -fill none -stroke "$bezel" -strokewidth "$b" -draw "roundrectangle $((x - b / 2)),$((y - b / 2)) $((x + w + b / 2 - 1)),$((y + h + b / 2 - 1)) $((r + b / 2)),$((r + b / 2))" "PNG32:$out"
}

# assert_srgba <png>: a layer without an alpha channel covers the footage.
assert_srgba() {
  local ch
  ch="$(magick identify -format '%[channels]' "$1")"   # "srgba 4.0" on ImageMagick 7.1
  [ "${ch%% *}" = srgba ] || die 70 "self-check: $(basename "$1") channels=$ch, expected srgba (the overlay would hide the footage)"
}

# assert_frame <png> <hole centre x> <hole centre y>: the channel layout AND a
# fully transparent hole. Neither alone is enough: a file written without alpha
# still reads alpha 0 at any pixel, and a canvas that never had alpha set keeps
# srgba from the bezel stroke while its hole stays opaque.
assert_frame() {
  local a
  assert_srgba "$1"
  a="$(magick "$1" -format "%[fx:p{$2,$3}.a]" info:)"
  [ "$a" = 0 ] || die 70 "self-check: $(basename "$1") hole alpha at $2,$3 is $a, expected 0 (the frame would hide the footage)"
}

# im_text <text>: escape a string for ImageMagick's caption:/label: readers,
# which expand %-escapes and read a file when the text starts with @.
im_text() {
  local t="${1//%/%%}"
  case "$t" in @*) t="\\$t" ;; esac
  printf '%s' "$t"
}

# render_caption <out.png> <text> <font> <pointsize> <colour> <canvas w> <canvas h> <max w> <band h>
# A full-canvas transparent layer with the caption centred in the top band.
# Fails the run when the wrapped block does not fit the band.
render_caption() {
  local out="$1" text="$2" font="$3" pt="$4" col="$5" cw="$6" ch="$7" maxw="$8" band="$9"
  local blk="${out%.png}.block.png" bw bh
  magick -background none -fill "$col" -font "$font" -pointsize "$pt" -size "${maxw}x" \
    -gravity center "caption:$(im_text "$text")" -trim +repage "PNG32:$blk"
  bw="$(magick identify -format '%w' "$blk")"; bh="$(magick identify -format '%h' "$blk")"
  [ "$bh" -le "$band" ] && [ "$bw" -le "$maxw" ] \
    || die 70 "self-check: caption \"$text\" is ${bw}x${bh}, its band is ${maxw}x${band}: shorten it"
  magick -size "${cw}x${ch}" xc:none "$blk" -gravity north -geometry "+0+$(( (band - bh) / 2 ))" \
    -compose over -composite "PNG32:$out"
  assert_srgba "$out"
  say "caption \"$text\": ${bw}x${bh} in a ${maxw}x${band} band"
}

# build_end_card <out.png> <canvas w> <canvas h> <bg> <text colour> <font> <bold font> <icon> <name> <tagline or ""> [badge...]
# Icon, app name, tagline and badges, stacked and centred. Badges are the
# consumer's official artwork, only scaled to one height: never recoloured,
# cropped or given added text (SKILL.md Safety Rule 7).
build_end_card() {
  local out="$1" cw="$2" ch="$3" bg="$4" col="$5" font="$6" bold="$7" icon="$8" name="$9"
  shift 9
  local tag="$1"; shift
  local tmp; tmp="$(dirname "$out")"
  local maxw=$(( cw - 240 )) y hi hn ht hb=0 total gap=48

  magick -background none "$icon" -resize 320x320 "PNG32:$tmp/card-icon.png"
  magick -background none -fill "$col" -font "$bold" -pointsize 84 "label:$(im_text "$name")" -trim +repage "PNG32:$tmp/card-name.png"
  if [ -n "$tag" ]; then
    magick -background none -fill "$col" -font "$font" -pointsize 54 -size "${maxw}x" -gravity center \
      "caption:$(im_text "$tag")" -trim +repage "PNG32:$tmp/card-tag.png"
  else
    magick -size 1x1 xc:none "PNG32:$tmp/card-tag.png"
  fi
  if [ "$#" -gt 0 ]; then
    build_badge_row "$tmp/card-badges.png" "$maxw" "$@"
    hb="$(magick identify -format '%h' "$tmp/card-badges.png")"
  fi
  hi="$(magick identify -format '%h' "$tmp/card-icon.png")"
  hn="$(magick identify -format '%h' "$tmp/card-name.png")"
  ht="$(magick identify -format '%h' "$tmp/card-tag.png")"
  total=$(( hi + gap + hn + 24 + ht ))
  [ "$hb" -gt 0 ] && total=$(( total + 72 + hb ))
  y=$(( (ch - total) / 2 ))

  set -- -respect-parentheses -size "${cw}x${ch}" "xc:$bg" -compose over \
    \( "$tmp/card-icon.png" \) -gravity north -geometry "+0+$y" -composite \
    \( "$tmp/card-name.png" \) -gravity north -geometry "+0+$(( y + hi + gap ))" -composite \
    \( "$tmp/card-tag.png" \) -gravity north -geometry "+0+$(( y + hi + gap + hn + 24 ))" -composite
  if [ "$hb" -gt 0 ]; then
    set -- "$@" \( "$tmp/card-badges.png" \) -gravity north -geometry "+0+$(( y + total - hb ))" -composite
  fi
  magick "$@" -alpha off "PNG24:$out"
}

# build_badge_row <out.png> <max w> <badge...>: badges side by side at one
# height, built as an argument array so paths with spaces survive.
# -respect-parentheses keeps each badge's -resize inside its own group.
build_badge_row() {
  local out="$1" maxw="$2"; shift 2
  local b
  local -a argv
  argv=(-respect-parentheses -background none)
  for b in "$@"; do argv=("${argv[@]}" "(" "$b" -resize x110 ")"); done
  magick "${argv[@]}" +smush 48 -resize "${maxw}x>" "PNG32:$out"
}
