package main

import (
	"bytes"
	"encoding/binary"
	"encoding/xml"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf16"
)

func wellFormed(t *testing.T, doc string) {
	t.Helper()
	d := xml.NewDecoder(strings.NewReader(doc))
	d.CharsetReader = func(_ string, r io.Reader) (io.Reader, error) { return r, nil }
	for {
		_, err := d.Token()
		if err == io.EOF {
			return
		}
		if err != nil {
			t.Fatalf("not XML: %v\n%s", err, doc)
		}
	}
}

func TestLaunchdPlist(t *testing.T) {
	p := launchdPlist("/Users/a&b/go/bin/cloche", "/Users/a&b/Library/Logs/cloche.log", map[string]string{"ANDROID_HOME": "/opt/<sdk>"})
	wellFormed(t, p)
	for _, want := range []string{
		"<string>/Users/a&amp;b/go/bin/cloche</string><string>serve</string>",
		"<key>ANDROID_HOME</key><string>/opt/&lt;sdk&gt;</string>",
		"<key>PATH</key>",
		"<key>KeepAlive</key><true/>",
	} {
		if !strings.Contains(p, want) {
			t.Errorf("no %q in\n%s", want, p)
		}
	}
	if lint, err := exec.LookPath("plutil"); err == nil {
		f := filepath.Join(t.TempDir(), "x.plist")
		os.WriteFile(f, []byte(p), 0o644)
		if out, err := exec.Command(lint, "-lint", f).CombinedOutput(); err != nil {
			t.Errorf("plutil: %s", out)
		}
	}
}

func TestSystemdUnit(t *testing.T) {
	u := systemdUnit(`/home/u/my "bin"/$x%y/cloche`, map[string]string{"CLOCHE_HOME": "/home/u/100%"})
	for _, want := range []string{
		`ExecStart="/home/u/my \"bin\"/$$x%%y/cloche" serve`,
		`Environment="CLOCHE_HOME=/home/u/100%%"`,
		"Restart=always",
		"WantedBy=default.target",
	} {
		if !strings.Contains(u, want) {
			t.Errorf("no %q in\n%s", want, u)
		}
	}
}

func TestTaskXML(t *testing.T) {
	x := taskXML(`C:\Users\a&b\cloche.exe`, `PC\a&b`)
	wellFormed(t, x)
	for _, want := range []string{
		`<Command>"C:\Users\a&amp;b\cloche.exe"</Command>`,
		`<UserId>PC\a&amp;b</UserId>`,
		"<ExecutionTimeLimit>PT0S</ExecutionTimeLimit>",
		"<DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries>",
		"<LogonType>InteractiveToken</LogonType>",
	} {
		if !strings.Contains(x, want) {
			t.Errorf("no %q in\n%s", want, x)
		}
	}
	b := utf16File("é<")
	if !bytes.Equal(b[:2], []byte{0xff, 0xfe}) {
		t.Fatal("no BOM")
	}
	u := make([]uint16, (len(b)-2)/2)
	for i := range u {
		u[i] = binary.LittleEndian.Uint16(b[2+2*i:])
	}
	if string(utf16.Decode(u)) != "é<" {
		t.Error("UTF-16 doesn't read back")
	}
}
