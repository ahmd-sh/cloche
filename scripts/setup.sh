#!/usr/bin/env bash
set -euo pipefail

root=$(cd "$(dirname "$0")/.." && pwd)
cd "$root"

go install ./cmd/cloche
mkdir -p ~/.local/bin
ln -sf ~/go/bin/cloche ~/.local/bin/cloche
echo "cloche is on PATH: ~/.local/bin/cloche"

"$root/scripts/launchagent.sh" install

mkdir -p ~/.claude/skills
ln -sfn "$root/skill" ~/.claude/skills/cloche
echo "agents know cloche: ~/.claude/skills/cloche"

if [ $# -gt 0 ]; then
  apk=$root/android/app/build/outputs/apk/release/app-release.apk
  [ -f "$apk" ] || (cd android && ./gradlew -q :app:assembleRelease)
  out=$(mktemp -d)
  cp "$apk" "$out/Cloche.apk"
  tailscale file cp "$out/Cloche.apk" "$1:"
  rm -rf "$out"
  echo "sent Cloche.apk to $1 over Taildrop"
fi

sleep 2
~/go/bin/cloche status
