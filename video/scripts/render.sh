#!/bin/sh
set -e
cd "$(dirname "$0")/.."
node scripts/sfx.mjs >/dev/null
mkdir -p out
for c in Portrait Landscape; do
  f=$(echo "$c" | tr 'A-Z' 'a-z')
  npx remotion render src/index.ts "$c" "out/$f-raw.mp4" --crf=16 --log=error
  flt=$(ffmpeg -hide_banner -i "out/$f-raw.mp4" -vn -af loudnorm=I=-16:TP=-1.5:LRA=11:print_format=json -f null - 2>&1 | sed -n '/^{/,/^}/p' |
    python3 -c "import json,sys;d=json.load(sys.stdin);print(f\"loudnorm=I=-16:TP=-1.5:LRA=11:measured_I={d['input_i']}:measured_TP={d['input_tp']}:measured_LRA={d['input_lra']}:measured_thresh={d['input_thresh']}:offset={d['target_offset']}:linear=true\")")
  ffmpeg -v error -y -i "out/$f-raw.mp4" -c:v copy -af "$flt" -ar 48000 -c:a aac -b:a 192k -movflags +faststart "out/cloche-$f.mp4"
  rm "out/$f-raw.mp4"
  echo "out/cloche-$f.mp4"
done
