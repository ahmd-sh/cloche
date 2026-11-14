package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ahmd-sh/cloche/internal/host"
	"github.com/ahmd-sh/cloche/internal/release"
	"github.com/ahmd-sh/cloche/internal/tailnet"
)

func tarGz(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var b bytes.Buffer
	gz := gzip.NewWriter(&b)
	tw := tar.NewWriter(gz)
	for name, body := range files {
		tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(body)), Typeflag: tar.TypeReg})
		tw.Write([]byte(body))
	}
	tw.Close()
	gz.Close()
	return b.Bytes()
}

func TestExtract(t *testing.T) {
	files := map[string]string{"cloche": "unix", "LICENSE": "MIT", "README.md": "x"}
	if b, err := extract("cloche-linux-amd64.tar.gz", tarGz(t, files)); err != nil || string(b) != "unix" {
		t.Errorf("tar.gz: %q %v", b, err)
	}
	if _, err := extract("cloche-linux-amd64.tar.gz", tarGz(t, map[string]string{"bin/cloche": "x", "../cloche": "y"})); err == nil {
		t.Error("took a cloche from another folder")
	}
	if _, err := extract("cloche-linux-amd64.tar.gz", []byte("not gzip")); err == nil {
		t.Error("took a broken archive")
	}
	if got := archiveName("linux", "amd64"); got != "cloche-linux-amd64.tar.gz" {
		t.Error(got)
	}
	if got := archiveName("darwin", "arm64"); got != "cloche-darwin-arm64.tar.gz" {
		t.Error(got)
	}
}

func TestReplace(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "cloche")
	os.WriteFile(bin, []byte("old"), 0o755)

	if err := replace(bin, []byte("#!/bin/sh\necho cloche 0.0.9\n"), "0.1.0"); err == nil {
		t.Error("swapped in a binary that says it's another version")
	}
	if b, _ := os.ReadFile(bin); string(b) != "old" {
		t.Errorf("the old binary is now %q", b)
	}
	if err := replace(bin, []byte("#!/bin/sh\necho cloche 0.1.0\n"), "0.1.0"); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(bin); !strings.Contains(string(b), "0.1.0") {
		t.Errorf("the binary is %q", b)
	}
	if left, _ := filepath.Glob(filepath.Join(dir, ".cloche.new*")); len(left) > 0 {
		t.Errorf("left %v", left)
	}
}

func TestSendApp(t *testing.T) {
	apk := "PK\x03\x04 the new Cloche"
	d := sha256.Sum256([]byte(apk))
	sum := hex.EncodeToString(d[:])
	files := map[string]string{
		"/download/v0.3.1/latest.json": `{"version":"0.3.1","files":{"cloche.apk":"` + sum + `"}}`,
		"/download/v0.3.1/cloche.apk":  apk,
		"/download/v0.3.2/latest.json": `{"version":"0.3.2","files":{"cloche.apk":"` + strings.Repeat("0", 64) + `"}}`,
		"/download/v0.3.2/cloche.apk":  apk,
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, ok := files[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte(b))
	}))
	defer srv.Close()
	defer func(b string) { release.Base = b }(release.Base)
	release.Base = srv.URL
	t.Setenv("CLOCHE_HOME", t.TempDir())

	h, err := host.Open(host.Config{Dir: home(), Name: "mac", Addr: "100.64.0.1:41778", Peers: tailnet.Anyone{}})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	ctx := context.Background()

	if err := sendApp(ctx, release.Client(), h, "0.3.2"); err == nil {
		t.Error("sent an APK that doesn't match its checksum")
	}
	if err := sendApp(ctx, release.Client(), h, "0.3.1"); err != nil {
		t.Fatal(err)
	}
	var st host.Status
	rec := httptest.NewRecorder()
	h.LocalAPI().ServeHTTP(rec, httptest.NewRequest("GET", "/status", nil))
	if err := json.Unmarshal(rec.Body.Bytes(), &st); err != nil {
		t.Fatal(err)
	}
	if len(st.Deliveries) != 1 || st.Deliveries[0].Note != "Cloche 0.3.1" || st.Deliveries[0].File.Name != "cloche.apk" || st.Deliveries[0].File.SHA256 != sum {
		t.Errorf("deliveries: %+v", st.Deliveries)
	}
	if left, _ := filepath.Glob(filepath.Join(home(), "update-*")); len(left) > 0 {
		t.Errorf("left %v", left)
	}
}
