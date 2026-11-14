package main

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const serviceLabel = "sh.ahmd.cloche"

type service struct {
	how   string
	notes []string
}

func serviceEnv() map[string]string {
	env := map[string]string{}
	for _, k := range []string{"CLOCHE_HOME", "ANDROID_HOME", "ANDROID_SDK_ROOT", noUpdateCheck} {
		if v := os.Getenv(k); v != "" {
			env[k] = v
		}
	}
	return env
}

func xmlText(s string) string {
	var b bytes.Buffer
	xml.EscapeText(&b, []byte(s))
	return b.String()
}

func launchdPlist(bin, logs string, env map[string]string) string {
	env = maps.Clone(env)
	env["PATH"] = "/usr/local/bin:/opt/homebrew/bin:/usr/bin:/bin:/usr/sbin:/sbin"
	var e strings.Builder
	for _, k := range slices.Sorted(maps.Keys(env)) {
		fmt.Fprintf(&e, "    <key>%s</key><string>%s</string>\n", xmlText(k), xmlText(env[k]))
	}
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>%s</string>
  <key>ProgramArguments</key>
  <array><string>%s</string><string>serve</string></array>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key><true/>
  <key>ThrottleInterval</key><integer>10</integer>
  <key>StandardOutPath</key><string>%s</string>
  <key>StandardErrorPath</key><string>%s</string>
  <key>EnvironmentVariables</key>
  <dict>
%s  </dict>
</dict>
</plist>
`, serviceLabel, xmlText(bin), xmlText(logs), xmlText(logs), e.String())
}

func systemdQuote(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "%", "%%")
	return `"` + s + `"`
}

func systemdUnit(bin string, env map[string]string) string {
	var b strings.Builder
	b.WriteString("[Unit]\nDescription=cloche: builds to your phone over Tailscale\n\n[Service]\n")
	fmt.Fprintf(&b, "ExecStart=%s serve\n", strings.ReplaceAll(systemdQuote(bin), "$", "$$"))
	for _, k := range slices.Sorted(maps.Keys(env)) {
		fmt.Fprintf(&b, "Environment=%s\n", systemdQuote(k+"="+env[k]))
	}
	b.WriteString("Restart=always\nRestartSec=10\n\n[Install]\nWantedBy=default.target\n")
	return b.String()
}

func tilde(p string) string {
	h, err := os.UserHomeDir()
	if err != nil {
		return p
	}
	if rel, err := filepath.Rel(h, p); err == nil && !strings.HasPrefix(rel, "..") && rel != "." {
		return filepath.Join("~", rel)
	}
	return p
}
