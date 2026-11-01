#!/bin/sh
# Renders the pictures of the website and the manual into site/assets.
#
# Run from the repository root:  sh scripts/render-screenshots.sh
#
# The pictures come from the real app: TestScreenshots in internal/ui plays a
# simulated user through it and writes each screen as HTML, and Playwright
# (https://playwright.dev, the `playwright` command) takes the picture.
set -eu

out="site/assets"
tmp="$(mktemp -d "${TMPDIR:-/tmp}/tuipe-shots.XXXXXX")"
trap 'rm -rf "$tmp"' EXIT

TUIPE_SHOTS="$tmp" go test ./internal/ui -run '^TestScreenshots$' -count=1

mkdir -p "$out"
for page in "$tmp"/*.html; do
  name="$(basename "$page" .html)"
  # 80 x 24 cells of 9 x 18 px and the margin of the page; the device gives 2x.
  playwright screenshot --device "Desktop Chrome HiDPI" --viewport-size "744, 452" \
    "file://$page" "$out/$name.png"
done
