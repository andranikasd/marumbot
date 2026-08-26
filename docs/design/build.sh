#!/usr/bin/env bash
# Rebuild the system and architecture design PDF. Diagrams come from docs/diagrams/build.sh.
set -euo pipefail
cd "$(dirname "$0")"
python3 - <<'PY'
import re, os
s = open('marum-final.html', encoding='utf-8').read()
def inline(m):
    svg = open(os.path.join('..', 'diagrams', 'svg', m.group(1) + '.svg'), encoding='utf-8').read()
    svg = re.sub(r'<\?xml[^>]*\?>', '', svg)
    svg = re.sub(r'<!DOCTYPE[^>]*>', '', svg)
    return svg.strip()
out = re.sub(r'<!--SVG:([a-z0-9\-]+)-->', inline, s)
assert '<!--SVG:' not in out, 'unresolved diagram placeholder'
open('marum-final.render.html', 'w', encoding='utf-8').write(out)
PY
chromium --headless --disable-gpu --no-sandbox \
  --no-pdf-header-footer --generate-pdf-document-outline \
  --virtual-time-budget=25000 \
  --print-to-pdf="$PWD/Marum-MVP-System-and-Architecture-Design.pdf" \
  "file://$PWD/marum-final.render.html"
echo "wrote $PWD/Marum-MVP-System-and-Architecture-Design.pdf"
