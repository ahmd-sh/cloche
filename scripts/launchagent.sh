#!/usr/bin/env bash
set -euo pipefail

label=sh.ahmd.cloche
plist=$HOME/Library/LaunchAgents/$label.plist
logs=$HOME/Library/Logs/cloche.log
bin=${CLOCHE_BIN:-$HOME/go/bin/cloche}
domain=gui/$(id -u)

case "${1:-}" in
install)
  [ -x "$bin" ] || { echo "no cloche at $bin: run go install ./cmd/cloche (or set CLOCHE_BIN)" >&2; exit 1; }
  mkdir -p "$(dirname "$plist")"
  cat >"$plist" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>$label</string>
  <key>ProgramArguments</key>
  <array><string>$bin</string><string>serve</string></array>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key><true/>
  <key>ThrottleInterval</key><integer>10</integer>
  <key>StandardOutPath</key><string>$logs</string>
  <key>StandardErrorPath</key><string>$logs</string>
  <key>EnvironmentVariables</key>
  <dict>
    <key>PATH</key><string>/usr/local/bin:/opt/homebrew/bin:/usr/bin:/bin:/usr/sbin:/sbin</string>
  </dict>
</dict>
</plist>
EOF
  launchctl bootout "$domain/$label" 2>/dev/null || true
  launchctl bootstrap "$domain" "$plist"
  echo "cloche serve runs at login now; it logs to $logs"
  ;;
restart)
  launchctl kickstart -k "$domain/$label"
  ;;
uninstall)
  launchctl bootout "$domain/$label" 2>/dev/null || true
  rm -f "$plist"
  echo "cloche serve no longer runs at login"
  ;;
*)
  echo "usage: $0 install|restart|uninstall" >&2
  exit 2
  ;;
esac
