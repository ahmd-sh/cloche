package apk

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/ahmd-sh/cloche/internal/wire"
)

func IsZip(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	head := make([]byte, 4)
	if _, err := io.ReadFull(f, head); err != nil {
		return false
	}
	return bytes.Equal(head, []byte("PK\x03\x04"))
}

func Inspect(ctx context.Context, path string) *wire.APK {
	tools, err := buildTools()
	if err != nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	out, err := exec.CommandContext(ctx, tool(tools, "aapt2"), "dump", "badging", path).Output()
	if err != nil {
		return nil
	}
	a := parseBadging(out)
	if a == nil {
		return nil
	}
	if out, err := exec.CommandContext(ctx, tool(tools, "apksigner"), "verify", "--print-certs", path).Output(); err == nil {
		a.Signer = parseSigner(out)
	}
	return a
}

var (
	packageLine = regexp.MustCompile(`^package: name='([^']*)' versionCode='([^']*)' versionName='([^']*)'`)
	labelLine   = regexp.MustCompile(`^application-label:'([^']*)'`)
	signerLine  = regexp.MustCompile(`^Signer #1 certificate SHA-256 digest: ([0-9a-f]+)`)
)

func parseBadging(out []byte) *wire.APK {
	var a *wire.APK
	for line := range strings.Lines(string(out)) {
		line = strings.TrimRight(line, "\n")
		if m := packageLine.FindStringSubmatch(line); m != nil {
			code, _ := strconv.ParseInt(m[2], 10, 64)
			a = &wire.APK{Package: m[1], VersionCode: code, VersionName: m[3]}
		} else if m := labelLine.FindStringSubmatch(line); m != nil && a != nil {
			a.Label = m[1]
		}
	}
	return a
}

func parseSigner(out []byte) string {
	for line := range strings.Lines(string(out)) {
		if m := signerLine.FindStringSubmatch(strings.TrimSpace(line)); m != nil {
			return m[1]
		}
	}
	return ""
}

func buildTools() (string, error) {
	sdk := sdkDir()
	entries, err := os.ReadDir(filepath.Join(sdk, "build-tools"))
	if err != nil {
		return "", err
	}
	var versions []string
	for _, e := range entries {
		if _, err := os.Stat(tool(filepath.Join(sdk, "build-tools", e.Name()), "aapt2")); err == nil {
			versions = append(versions, e.Name())
		}
	}
	if len(versions) == 0 {
		return "", errors.New("no build-tools with aapt2")
	}
	slices.SortFunc(versions, compareVersions)
	return filepath.Join(sdk, "build-tools", versions[len(versions)-1]), nil
}

func sdkDir() string {
	for _, env := range []string{"ANDROID_HOME", "ANDROID_SDK_ROOT"} {
		if d := os.Getenv(env); d != "" {
			return d
		}
	}
	home, _ := os.UserHomeDir()
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Android", "sdk")
	case "windows":
		if d := os.Getenv("LOCALAPPDATA"); d != "" {
			return filepath.Join(d, "Android", "Sdk")
		}
		return filepath.Join(home, "AppData", "Local", "Android", "Sdk")
	}
	return filepath.Join(home, "Android", "Sdk")
}

func tool(dir, name string) string {
	if runtime.GOOS == "windows" {
		if name == "apksigner" {
			name += ".bat"
		} else {
			name += ".exe"
		}
	}
	return filepath.Join(dir, name)
}

func compareVersions(a, b string) int {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < max(len(as), len(bs)); i++ {
		var x, y int
		if i < len(as) {
			x, _ = strconv.Atoi(as[i])
		}
		if i < len(bs) {
			y, _ = strconv.Atoi(bs[i])
		}
		if x != y {
			return x - y
		}
	}
	return 0
}
