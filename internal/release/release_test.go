package release

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestVersions(t *testing.T) {
	for _, c := range []struct {
		a, b  string
		newer bool
	}{
		{"0.3.1", "0.3.0", true},
		{"0.10.0", "0.9.9", true},
		{"1.0.0", "0.999.999", true},
		{"0.3.0", "0.3.0", false},
		{"0.2.9", "0.3.0", false},
		{"0.3.1", "dev", false},
		{"0.3.1", "0.0.0-20261114055927-2791eb6733da+dirty", false},
		{"0.3.1-rc.1", "0.3.0", false},
		{"1000.0.0", "0.3.0", false},
	} {
		if got := Newer(c.a, c.b); got != c.newer {
			t.Errorf("Newer(%q, %q) = %v", c.a, c.b, got)
		}
	}
	if Code("0.3.0") != 3000 || Code("1.2.3") != 1002003 || Code("dev") != 0 {
		t.Errorf("Code: %d %d %d", Code("0.3.0"), Code("1.2.3"), Code("dev"))
	}
}

func serve(t *testing.T, files map[string]string) *httptest.Server {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, ok := files[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte(b))
	}))
	t.Cleanup(s.Close)
	return s
}

func TestFetch(t *testing.T) {
	sum := strings.Repeat("ab", 32)
	s := serve(t, map[string]string{
		"/ok":       `{"version":"0.3.1","files":{"cloche.apk":"` + sum + `"}}`,
		"/version":  `{"version":"0.3.1-rc1","files":{}}`,
		"/name":     `{"version":"0.3.1","files":{"../cloche":"` + sum + `"}}`,
		"/sum":      `{"version":"0.3.1","files":{"cloche.apk":"AB"}}`,
		"/notjson":  `<html>`,
		"/toolarge": `{"version":"0.3.1","x":"` + strings.Repeat("x", maxManifest) + `"}`,
	})
	m, err := Fetch(context.Background(), Client(), s.URL+"/ok")
	if err != nil || m.Version != "0.3.1" || m.Files["cloche.apk"] != sum {
		t.Fatalf("Fetch = %+v, %v", m, err)
	}
	for _, p := range []string{"/version", "/name", "/sum", "/notjson", "/toolarge", "/missing"} {
		if _, err := Fetch(context.Background(), Client(), s.URL+p); err == nil {
			t.Errorf("Fetch(%s) took it", p)
		}
	}
}

func TestDownload(t *testing.T) {
	body := "the binary"
	d := sha256.Sum256([]byte(body))
	sum := hex.EncodeToString(d[:])
	s := serve(t, map[string]string{"/f": body})
	var b bytes.Buffer
	if err := Download(context.Background(), Client(), s.URL+"/f", sum, 100, &b); err != nil || b.String() != body {
		t.Fatalf("Download = %q, %v", b.String(), err)
	}
	if err := Download(context.Background(), Client(), s.URL+"/f", strings.Repeat("0", 64), 100, &bytes.Buffer{}); err == nil {
		t.Error("took a file that doesn't match its checksum")
	}
	if err := Download(context.Background(), Client(), s.URL+"/f", sum, 4, &bytes.Buffer{}); err == nil {
		t.Error("took a file over the limit")
	}
	if err := Download(context.Background(), Client(), s.URL+"/f", "", 100, &bytes.Buffer{}); err == nil {
		t.Error("took a file with no checksum")
	}
}

func TestNoDowngradeRedirect(t *testing.T) {
	plain := serve(t, map[string]string{"/f": "x"})
	tls := httptest.NewTLSServer(http.RedirectHandler(plain.URL+"/f", http.StatusFound))
	defer tls.Close()
	c := Client()
	c.Transport = tls.Client().Transport
	if _, err := Fetch(context.Background(), c, tls.URL); err == nil || !strings.Contains(err.Error(), "HTTPS") {
		t.Errorf("followed a redirect to plain HTTP: %v", err)
	}
}
