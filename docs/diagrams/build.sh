#!/usr/bin/env bash
# Rebuild every diagram: Mermaid sources -> .drawio, then SVG (for the PDF) and
# editable PNG (for the repo). Requires the draw.io desktop CLI and xvfb-run.
set -euo pipefail
cd "$(dirname "$0")"
export HOME="${HOME:-/tmp}"
mkdir -p svg png

for f in src/*.mmd; do
  b="$(basename "$f" .mmd)"
  xvfb-run -a drawio -x -f xml -o "$b.drawio" "$f" --disable-gpu
done

for f in *.drawio; do
  b="$(basename "$f" .drawio)"
  xvfb-run -a drawio -x -f svg -b 8 -o "svg/$b.svg" "$f" --disable-gpu
  xvfb-run -a drawio -x -f png -e -s 2 -b 8 -o "png/$b.drawio.png" "$f" --disable-gpu
done

# draw.io embeds a raster fallback of every label in SVG exports; Chromium renders
# the foreignObject instead, so dropping them cuts the files ~10x with no visual change.
python3 - <<'PY'
import re, glob
for p in glob.glob('svg/*.svg'):
    s = open(p, encoding='utf-8').read()
    s = re.sub(r'<image[^>]*?xlink:href="data:image/[^"]*"[^>]*/>', '', s)
    open(p, 'w', encoding='utf-8').write(s.replace(' color-scheme: light dark;', ''))
PY
echo "diagrams rebuilt"
