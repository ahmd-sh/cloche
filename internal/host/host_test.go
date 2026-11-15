package host

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ahmd-sh/cloche/internal/tailnet"
	"github.com/ahmd-sh/cloche/internal/wire"
)

var update = flag.Bool("update", false, "rewrite testdata/golden.json")

const (
	phoneIP    = "100.64.0.2"
	otherIP    = "100.64.0.3"
	strangerIP = "100.64.0.4"
)

type peers map[string]tailnet.Peer

func (p peers) Whois(_ context.Context, ip netip.Addr) (tailnet.Peer, error) {
	if peer, ok := p[ip.String()]; ok {
		return peer, nil
	}
	return tailnet.Peer{}, errors.New("no such peer")
}

func newHost(t *testing.T) *Host {
	t.Helper()
	h, err := Open(Config{
		Dir: t.TempDir(), Name: "mac", Addr: "100.64.0.1:41778", Owner: "me@github", DNSName: "mac.tail1234.ts.net.",
		Peers: peers{
			phoneIP:    {Node: "nPhone", Name: "phone", Login: "me@github"},
			otherIP:    {Node: "nOther", Name: "laptop", Login: "me@github"},
			strangerIP: {Node: "nStranger", Name: "theirs", Login: "them@github"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(h.Close)
	return h
}

func call(t *testing.T, h http.Handler, from, method, target, secret string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, target, r)
	req.Host = "100.64.0.1:41778"
	req.RemoteAddr = from + ":40000"
	if secret != "" {
		req.Header.Set("Authorization", "Bearer "+secret)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func pairPhone(t *testing.T, h *Host) wire.PairResponse {
	t.Helper()
	p, _ := h.StartPairing(time.Minute, "")
	w := call(t, h.PhoneAPI(), phoneIP, "POST", "/v1/pair", "", wire.PairRequest{Code: strings.ToLower(p.Code), Device: "Pixel"})
	if w.Code != 200 {
		t.Fatalf("pair: %d %s", w.Code, w.Body)
	}
	var res wire.PairResponse
	json.Unmarshal(w.Body.Bytes(), &res)
	return res
}

func fakeAPK(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "app-debug.apk")
	os.WriteFile(p, []byte("PK\x03\x04"+content), 0o644)
	return p
}

func TestPairing(t *testing.T) {
	h := newHost(t)
	api := h.PhoneAPI()

	if w := call(t, api, phoneIP, "POST", "/v1/pair", "", wire.PairRequest{Code: "0000-0000-0000"}); w.Code != 403 {
		t.Errorf("pairing while none is open: %d", w.Code)
	}

	p, _ := h.StartPairing(time.Minute, "")
	if w := call(t, api, strangerIP, "POST", "/v1/pair", "", wire.PairRequest{Code: p.Code}); w.Code != 403 {
		t.Errorf("someone else's device paired: %d", w.Code)
	}
	for i := range 5 {
		w := call(t, api, phoneIP, "POST", "/v1/pair", "", wire.PairRequest{Code: "WRNG-CODE-0000"})
		if w.Code != 403 {
			t.Fatalf("wrong code %d: %d", i, w.Code)
		}
	}
	if w := call(t, api, phoneIP, "POST", "/v1/pair", "", wire.PairRequest{Code: p.Code}); w.Code != 403 {
		t.Errorf("the right code still works after 5 wrong ones: %d", w.Code)
	}

	res := pairPhone(t, h)
	if !bytes.Equal(res.Key, h.spki) || res.Secret == "" || res.Host != "mac" || res.Addr != "100.64.0.1:41778" {
		t.Errorf("pair response: %+v", res)
	}
	if len(h.state.Phones) != 1 || h.state.Phones[0].Node != "nPhone" || h.state.Phones[0].Secret == res.Secret || h.state.Phones[0].Name != "phone" {
		t.Errorf("kept %+v", h.state.Phones)
	}
	if w := call(t, api, phoneIP, "POST", "/v1/pair", "", wire.PairRequest{Code: p.Code}); w.Code != 403 {
		t.Errorf("a code worked twice: %d", w.Code)
	}

	h2, err := Open(Config{Dir: h.dir, Peers: h.peers})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(h2.spki, h.spki) || len(h2.state.Phones) != 1 {
		t.Error("reopened without the key or the phone")
	}
}

func TestOnlyThePairedPhone(t *testing.T) {
	h := newHost(t)
	api := h.PhoneAPI()
	res := pairPhone(t, h)

	for _, c := range []struct {
		name, from, secret string
		want               int
	}{
		{"no secret", phoneIP, "", 401},
		{"wrong secret", phoneIP, "nope", 401},
		{"another of my devices with the secret", otherIP, res.Secret, 401},
		{"not on the tailnet", "192.168.1.5", res.Secret, 503},
		{"the phone", phoneIP, res.Secret, 200},
	} {
		if w := call(t, api, c.from, "GET", "/v1/inbox?after=0&wait=0", c.secret, nil); w.Code != c.want {
			t.Errorf("%s: %d, want %d", c.name, w.Code, c.want)
		}
	}

	got := make(chan int)
	go func() { got <- call(t, api, phoneIP, "GET", "/v1/inbox?after=0&wait=10", res.Secret, nil).Code }()
	time.Sleep(50 * time.Millisecond)
	if _, err := h.Unpair(""); err != nil {
		t.Fatal(err)
	}
	select {
	case code := <-got:
		if code != 401 {
			t.Errorf("poll ended by unpairing: %d", code)
		}
	case <-time.After(3 * time.Second):
		t.Error("unpairing didn't end the poll")
	}
	if w := call(t, api, phoneIP, "GET", "/v1/inbox?after=0&wait=0", res.Secret, nil); w.Code != 401 {
		t.Errorf("unpaired phone polled: %d", w.Code)
	}
}

func TestDelivery(t *testing.T) {
	h := newHost(t)
	api := h.PhoneAPI()
	res := pairPhone(t, h)
	pub := parseKey(t, res.Key)

	got := make(chan *httptest.ResponseRecorder)
	go func() { got <- call(t, api, phoneIP, "GET", "/v1/inbox?after=0&wait=10", res.Secret, nil) }()
	time.Sleep(50 * time.Millisecond)
	content := strings.Repeat("dex", 1000)
	s, err := h.Push(context.Background(), PushRequest{Path: fakeAPK(t, content), Note: "first\nline", Source: &wire.Source{Repo: "app", Commit: "abc123"}})
	if err != nil {
		t.Fatal(err)
	}
	var w *httptest.ResponseRecorder
	select {
	case w = <-got:
	case <-time.After(3 * time.Second):
		t.Fatal("the poll didn't wake for the push")
	}
	var in wire.Inbox
	json.Unmarshal(w.Body.Bytes(), &in)
	if in.Head != 1 || len(in.Deliveries) != 1 {
		t.Fatalf("inbox: %s", w.Body)
	}

	d := in.Deliveries[0]
	digest := sha256.Sum256(d.Manifest)
	if !ecdsa.VerifyASN1(pub, digest[:], d.Sig) {
		t.Error("the manifest's signature doesn't verify")
	}
	var m wire.Manifest
	json.Unmarshal(d.Manifest, &m)
	sum := sha256.Sum256([]byte("PK\x03\x04" + content))
	if m.ID != s.ID || m.Seq != 1 || m.File.SHA256 != hex.EncodeToString(sum[:]) || m.File.Size != int64(4+len(content)) || m.Note != "first line" || m.Source.Commit != "abc123" {
		t.Errorf("manifest: %+v", m)
	}

	start := time.Now()
	w = call(t, api, phoneIP, "GET", "/v1/inbox?after=1&wait=1", res.Secret, nil)
	if json.Unmarshal(w.Body.Bytes(), &in); len(in.Deliveries) != 0 || time.Since(start) < 900*time.Millisecond {
		t.Errorf("poll at head: %s after %s", w.Body, time.Since(start))
	}
	start = time.Now()
	w = call(t, api, phoneIP, "GET", "/v1/inbox?after=9&wait=5", res.Secret, nil)
	if json.Unmarshal(w.Body.Bytes(), &in); in.Head != 1 || time.Since(start) > time.Second {
		t.Errorf("poll past head: %s after %s", w.Body, time.Since(start))
	}

	req := httptest.NewRequest("GET", "/v1/apk/"+m.ID, nil)
	req.Host = "100.64.0.1:41778"
	req.RemoteAddr = phoneIP + ":40000"
	req.Header.Set("Authorization", "Bearer "+res.Secret)
	req.Header.Set("Range", "bytes=4-")
	rw := httptest.NewRecorder()
	api.ServeHTTP(rw, req)
	if rw.Code != 206 || rw.Body.String() != content {
		t.Errorf("range: %d, %d bytes", rw.Code, rw.Body.Len())
	}

	local := func(q string) Summary {
		var s Summary
		json.Unmarshal(call(t, h.LocalAPI(), "", "GET", "/deliveries/"+m.ID+q, "", nil).Body.Bytes(), &s)
		return s
	}
	rev := local("").Rev
	done := make(chan Summary)
	go func() { done <- local(fmt.Sprintf("?until=change&since=%d&wait=10", rev)) }()
	time.Sleep(50 * time.Millisecond)
	if w := call(t, api, phoneIP, "POST", "/v1/result/"+m.ID, res.Secret, wire.Result{Status: wire.Ready}); w.Code != 204 {
		t.Fatalf("result: %d", w.Code)
	}
	select {
	case s := <-done:
		if len(s.Phones) != 1 || s.Phones[0].Result == nil || s.Phones[0].Result.Status != wire.Ready || s.Rev == rev {
			t.Errorf("summary after ready: %+v", s)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("the CLI's wait didn't end")
	}
	call(t, api, phoneIP, "POST", "/v1/result/"+m.ID, res.Secret, wire.Result{Status: wire.Installed, Package: "sh.ahmd.app"})
	s = local("")
	if p := s.Phones[0]; p.Phone != "phone" || p.Result.Status != wire.Installed || p.Taken.IsZero() || p.Fetched.IsZero() {
		t.Errorf("summary: %+v", s)
	}
}

func TestNewPhoneGetsTheLastHour(t *testing.T) {
	h := newHost(t)
	now := time.Now()
	h.now = func() time.Time { return now.Add(-2 * time.Hour) }
	h.Push(context.Background(), PushRequest{Path: fakeAPK(t, "old")})
	h.now = func() time.Time { return now.Add(-10 * time.Minute) }
	h.Push(context.Background(), PushRequest{Path: fakeAPK(t, "recent")})
	h.now = time.Now
	if res := pairPhone(t, h); res.Head != 1 {
		t.Errorf("head %d, want 1: after the old push, before the recent one", res.Head)
	}
}

func TestPruning(t *testing.T) {
	h := newHost(t)
	var first Summary
	for i := range Keep + 3 {
		s, err := h.Push(context.Background(), PushRequest{Path: fakeAPK(t, "x")})
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			first = s
		}
	}
	if len(h.records) != Keep {
		t.Errorf("kept %d", len(h.records))
	}
	if _, err := os.Stat(h.path("deliveries", first.ID+".apk")); !os.IsNotExist(err) {
		t.Error("the oldest file is still there")
	}
}

func TestRefusesWhatIsntAnAPK(t *testing.T) {
	h := newHost(t)
	p := filepath.Join(t.TempDir(), "notes.txt")
	os.WriteFile(p, []byte("hello"), 0o644)
	if _, err := h.Push(context.Background(), PushRequest{Path: p}); err == nil {
		t.Error("pushed a text file")
	}
	if _, err := h.Push(context.Background(), PushRequest{Path: t.TempDir()}); err == nil {
		t.Error("pushed a directory")
	}
}

func TestCode(t *testing.T) {
	c := newCode()
	if len(c) != 14 || strings.Count(c, "-") != 2 {
		t.Errorf("code %q", c)
	}
	if normalCode("ab0l-o1i2") != normalCode("AB01-0112") {
		t.Error("codes aren't read as typed")
	}
}

func TestGolden(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "golden.json")
	if *update {
		h := newHost(t)
		h.now = func() time.Time { return time.Date(2026, 11, 4, 18, 0, 0, 0, time.UTC) }
		s, err := h.Push(context.Background(), PushRequest{Path: fakeAPK(t, "golden"), Note: "Sessions: rename inline", Source: &wire.Source{Dir: "/Users/me/dev/app", Repo: "app", Branch: "main", Commit: "020c4dd", Dirty: true}})
		if err != nil {
			t.Fatal(err)
		}
		g := golden{Key: h.spki, Fingerprint: h.Fingerprint(), Inbox: wire.Inbox{Head: 1, Deliveries: []wire.Delivery{h.find(s.ID).Delivery}}}
		b, _ := json.MarshalIndent(g, "", "  ")
		if err := os.WriteFile(path, append(b, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var g golden
	if err := json.Unmarshal(b, &g); err != nil {
		t.Fatal(err)
	}
	d := g.Inbox.Deliveries[0]
	digest := sha256.Sum256(d.Manifest)
	if !ecdsa.VerifyASN1(parseKey(t, g.Key), digest[:], d.Sig) {
		t.Error("golden signature doesn't verify")
	}
	if Fingerprint(g.Key) != g.Fingerprint {
		t.Error("golden fingerprint")
	}
}

type golden struct {
	Key         []byte     `json:"key"`
	Fingerprint string     `json:"fingerprint"`
	Inbox       wire.Inbox `json:"inbox"`
}

func parseKey(t *testing.T, der []byte) *ecdsa.PublicKey {
	t.Helper()
	k, err := x509.ParsePKIXPublicKey(der)
	if err != nil {
		t.Fatal(err)
	}
	return k.(*ecdsa.PublicKey)
}

func TestClean(t *testing.T) {
	for in, want := range map[string]string{
		"iPhone17,1":             "iPhone17,1",
		"  two\nlines\tand tab ": "two lines and tab",
		"red\x1b[31m":            "red[31m",
		"csi\u009b31m":           "csi31m",
		"del\x7f":                "del",
		"evil‮gpj.exe":           "evilgpj.exe",
		"日本語":                    "日本語",
	} {
		if got := clean(in, 64); got != want {
			t.Errorf("clean(%q) = %q, want %q", in, got, want)
		}
	}
	if got := clean("abcdef", 3); got != "abc" {
		t.Errorf("clean caps at n runes: got %q", got)
	}
}

func TestHostNames(t *testing.T) {
	h := newHost(t)
	for host, want := range map[string]bool{
		"100.64.0.1:41778":           true,
		"100.64.0.1":                 true,
		"mac:41778":                  true,
		"MAC.tail1234.ts.net:41778":  true,
		"mac.tail1234.ts.net.:41778": true,
		"evil.example:41778":         false,
		"127.0.0.1:41778":            false,
		"localhost:41778":            false,
		"100.64.0.2:41778":           false,
		"":                           false,
	} {
		if got := h.ourName(host); got != want {
			t.Errorf("ourName(%q) = %v, want %v", host, got, want)
		}
	}

	req := httptest.NewRequest("POST", "/v1/pair", strings.NewReader(`{}`))
	req.Host = "evil.example:41778"
	req.RemoteAddr = phoneIP + ":40000"
	w := httptest.NewRecorder()
	h.PhoneAPI().ServeHTTP(w, req)
	if w.Code != http.StatusMisdirectedRequest {
		t.Errorf("rebound name: %d", w.Code)
	}
}

func TestNoOwner(t *testing.T) {
	h, err := Open(Config{
		Dir: t.TempDir(), Name: "mac", Addr: "100.64.0.1:41778", Owner: "",
		Peers: peers{phoneIP: {Node: "nPhone", Name: "phone", Login: ""}},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(h.Close)
	if _, err := h.StartPairing(time.Minute, ""); err == nil {
		t.Error("pairing opened with no owner")
	}
	if h.ownedBy(tailnet.Peer{Login: ""}) {
		t.Error("an empty login counts as the owner's")
	}

	h2 := newHost(t)
	if h2.ownedBy(tailnet.Peer{Login: "me@github", Tagged: true}) {
		t.Error("a tagged node counts as mine")
	}
	if !h2.ownedBy(tailnet.Peer{Login: "me@github"}) {
		t.Error("my own node doesn't count as mine")
	}
}

func TestPhoneReportsItsApp(t *testing.T) {
	h := newHost(t)
	res := pairPhone(t, h)
	poll := func(ua string) {
		t.Helper()
		req := httptest.NewRequest("GET", "/v1/inbox?after=0&wait=0", nil)
		req.Host = "100.64.0.1:41778"
		req.RemoteAddr = phoneIP + ":40000"
		req.Header.Set("Authorization", "Bearer "+res.Secret)
		req.Header.Set("User-Agent", ua)
		w := httptest.NewRecorder()
		h.PhoneAPI().ServeHTTP(w, req)
		if w.Code != 200 {
			t.Fatalf("inbox: %d %s", w.Code, w.Body)
		}
	}
	poll("Cloche/0.3.0 (3000)")
	for _, ua := range []string{"Dalvik/2.1.0 (Linux; U; Android 16)", "Cloche/0.3.0 (abc)", "Cloche/0.3.0 (3000) extra", "Cloche/<b> (3000)"} {
		poll(ua)
	}
	h.mu.Lock()
	st := h.status()
	h.mu.Unlock()
	if st.Phones[0].App != "0.3.0" || st.Phones[0].AppCode != 3000 {
		t.Errorf("phone's app: %q %d", st.Phones[0].App, st.Phones[0].AppCode)
	}
	if b := h.Behind(3000, "0.3.0"); len(b) != 0 {
		t.Errorf("behind its own version: %v", b)
	}
	b := h.Behind(3001, "0.3.1")
	if len(b) != 1 || b[0] != "nPhone" {
		t.Fatalf("behind: %v", b)
	}
	if err := h.OfferApp(context.Background(), fakeAPK(t, "cloche"), "0.3.1", b); err != nil {
		t.Fatal(err)
	}
	if b := h.Behind(3001, "0.3.1"); len(b) != 0 {
		t.Errorf("offered twice: %v", b)
	}
	h2, err := Open(Config{Dir: h.dir, Peers: h.peers})
	if err != nil {
		t.Fatal(err)
	}
	if b := h2.Behind(3001, "0.3.1"); len(b) != 0 {
		t.Errorf("reopened and offered again: %v", b)
	}
}

func TestPairLinkOpensThePairPage(t *testing.T) {
	h := newHost(t)
	p, err := h.StartPairing(time.Minute, "")
	if err != nil {
		t.Fatal(err)
	}
	frag, ok := strings.CutPrefix(p.URL, "https://ahmd-sh.github.io/cloche/pair/#")
	if !ok {
		t.Fatalf("pairing link %s", p.URL)
	}
	q, err := url.ParseQuery(frag)
	if err != nil || q.Get("c") != p.Code || q.Get("a") != "100.64.0.1:41778" || q.Get("h") != "mac" || len(q.Get("f")) != 43 {
		t.Errorf("pairing link %s: %v", p.URL, q)
	}
}
