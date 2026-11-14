#!/bin/sh
set -eu

root=$(cd "$(dirname "$0")/.." && pwd)
out=$1
rm -rf "$out"
mkdir -p "$out/fonts"
cp -R "$root/site/." "$out/"
cp "$root/internal/host/web/fonts/plex_mono_regular.ttf" "$root/internal/host/web/fonts/plex_mono_semibold.ttf" "$out/fonts/"
cp "$root/android/app/FONT-LICENSE.txt" "$out/fonts/OFL.txt"
cp "$root/internal/host/web/icon-192.png" "$out/icon.png"
