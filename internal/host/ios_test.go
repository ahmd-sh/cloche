package host

import (
	"bytes"
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ahmd-sh/cloche/internal/webpush"
	"github.com/ahmd-sh/cloche/internal/wire"
)

const (
	web      = "https://mac.example.ts.net:41780"
	iphoneIP = "100.64.0.5"
	friendIP = "100.64.0.6"
	udid     = "00008140-001A2B3C4D5E6F70"
)

type pushService struct {
	mu     sync.Mutex
	got    []http.Header
	status int
}

func (p *pushService) start(t *testing.T) (*httptest.Server, webpush.Subscription) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		p.mu.Lock()
		defer p.mu.Unlock()
		p.got = append(p.got, r.Header)
		if p.status != 0 {
			w.WriteHeader(p.status)
			return
		}
		w.WriteHeader(http.StatusCreated)
	}))
	t.Cleanup(srv.Close)
	ua, _ := ecdh.P256().GenerateKey(rand.Reader)
	auth := make([]byte, 16)
	rand.Read(auth)
	var sub webpush.Subscription
	sub.Endpoint = srv.URL + "/push/1"
	sub.Keys.P256dh = base64.RawURLEncoding.EncodeToString(ua.PublicKey().Bytes())
	sub.Keys.Auth = base64.RawURLEncoding.EncodeToString(auth)
	return srv, sub
}

func (p *pushService) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.got)
}

func newWebHost(t *testing.T, client *http.Client) *Host {
	t.Helper()
	allow := webpush.AllowHost
	webpush.AllowHost = func(string) bool { return true }
	t.Cleanup(func() { webpush.AllowHost = allow })
	h, err := Open(Config{
		Dir: t.TempDir(), Name: "mac", Addr: "100.64.0.1:41778", Owner: "me@github", Web: web, PushHTTP: client,
		Peers: peers{
			phoneIP:  {Node: "nPhone", Name: "pixel", Login: "me@github"},
			iphoneIP: {Node: "nIPhone", Name: "iPhone 17 Pro", Login: "me@github"},
			friendIP: {Node: "nFriend", Name: "sams-iphone", Login: "sam@icloud"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(h.Close)
	return h
}

func webCall(t *testing.T, h *Host, from, method, target string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var r io.Reader
	if b, ok := body.([]byte); ok {
		r = bytes.NewReader(b)
	} else if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, web+target, r)
	req.RemoteAddr = from + ":40000"
	if method == "POST" {
		req.Header.Set("Origin", web)
	}
	w := httptest.NewRecorder()
	h.WebApp().ServeHTTP(w, req)
	return w
}

func pairIPhone(t *testing.T, h *Host, from, friend string) string {
	t.Helper()
	p, err := h.StartPairingIPhone(time.Minute, friend)
	if err != nil {
		t.Fatal(err)
	}
	code, ok := strings.CutPrefix(p.URL, web+"/pair#")
	if !ok || code != p.Code {
		t.Fatalf("pairing link %s", p.URL)
	}
	w := webCall(t, h, from, "POST", "/api/pair", map[string]string{"code": code})
	if w.Code != 200 {
		t.Fatalf("pair: %d %s", w.Code, w.Body)
	}
	var res struct{ Name string }
	json.Unmarshal(w.Body.Bytes(), &res)
	return res.Name
}

func makeApp(t *testing.T, devices ...string) string {
	t.Helper()
	app := filepath.Join(t.TempDir(), "Satchel.app")
	os.MkdirAll(app, 0o755)
	os.WriteFile(filepath.Join(app, "Info.plist"), []byte(`<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0"><dict>
<key>CFBundleIdentifier</key><string>com.example.satchel</string>
<key>CFBundleDisplayName</key><string>Satchel</string>
<key>CFBundleShortVersionString</key><string>0.3.0</string>
<key>CFBundleVersion</key><string>12</string>
<key>CFBundleSupportedPlatforms</key><array><string>iPhoneOS</string></array>
</dict></plist>`), 0o644)
	os.WriteFile(filepath.Join(app, "Satchel"), []byte("macho"), 0o755)
	if devices != nil {
		var list strings.Builder
		for _, d := range devices {
			list.WriteString("<string>" + d + "</string>")
		}
		os.WriteFile(filepath.Join(app, "embedded.mobileprovision"), []byte(`cms<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0"><dict>
<key>Name</key><string>Satchel Ad Hoc</string>
<key>TeamName</key><string>Example Ltd</string>
<key>TeamIdentifier</key><array><string>ABCDE12345</string></array>
<key>ExpirationDate</key><date>2099-01-01T00:00:00Z</date>
<key>Entitlements</key><dict><key>get-task-allow</key><false/></dict>
<key>ProvisionedDevices</key><array>`+list.String()+`</array>
</dict></plist>sig`), 0o644)
	}
	return app
}

func TestIPhonePairing(t *testing.T) {
	h := newWebHost(t, nil)

	w := webCall(t, h, iphoneIP, "GET", "/api/me", nil)
	if !strings.Contains(w.Body.String(), `"paired":false`) || !strings.Contains(w.Body.String(), `"pairing":false`) {
		t.Errorf("me before pairing: %s", w.Body)
	}

	a, _ := h.StartPairing(time.Minute)
	if w := webCall(t, h, iphoneIP, "POST", "/api/pair", map[string]string{"code": a.Code}); w.Code != 403 {
		t.Errorf("Android code paired an iPhone: %d", w.Code)
	}
	i, _ := h.StartPairingIPhone(time.Minute, "")
	if w := call(t, h.PhoneAPI(), phoneIP, "POST", "/v1/pair", "", wire.PairRequest{Code: i.Code}); w.Code != 403 {
		t.Errorf("iPhone code paired Android: %d", w.Code)
	}

	if w := webCall(t, h, friendIP, "POST", "/api/pair", map[string]string{"code": i.Code}); w.Code != 403 {
		t.Errorf("someone else's iPhone paired as mine: %d", w.Code)
	}
	req := httptest.NewRequest("POST", web+"/api/pair", strings.NewReader(`{"code":"`+i.Code+`"}`))
	req.RemoteAddr, req.Header["Origin"] = iphoneIP+":1", []string{"https://evil.example"}
	rw := httptest.NewRecorder()
	h.WebApp().ServeHTTP(rw, req)
	if rw.Code != 403 {
		t.Errorf("cross-site pair: %d", rw.Code)
	}
	req = httptest.NewRequest("GET", "https://100.64.0.1:41780/api/me", nil)
	req.RemoteAddr = iphoneIP + ":1"
	rw = httptest.NewRecorder()
	h.WebApp().ServeHTTP(rw, req)
	if rw.Code != http.StatusMisdirectedRequest {
		t.Errorf("by IP: %d", rw.Code)
	}

	if name := pairIPhone(t, h, iphoneIP, ""); name != "iphone-17-pro" {
		t.Errorf("my iPhone is called %q", name)
	}
	if name := pairIPhone(t, h, friendIP, "sam"); name != "sam" {
		t.Errorf("friend is called %q", name)
	}
	if len(h.state.IPhones) != 2 || !h.state.IPhones[1].Friend || h.state.IPhones[0].Friend {
		t.Errorf("iPhones: %+v", h.state.IPhones)
	}
	if w := webCall(t, h, phoneIP, "GET", "/api/builds/x", nil); w.Code != 401 {
		t.Errorf("unpaired device: %d", w.Code)
	}

	h2, err := Open(Config{Dir: h.dir, Peers: h.peers, Web: web})
	if err != nil {
		t.Fatal(err)
	}
	if len(h2.state.IPhones) != 2 || h2.pusher.PublicKey() != h.pusher.PublicKey() {
		t.Error("reopened without the iPhones or the push key")
	}
}

func TestUDID(t *testing.T) {
	h := newWebHost(t, nil)
	pairIPhone(t, h, iphoneIP, "")
	w := webCall(t, h, iphoneIP, "GET", "/udid.mobileconfig", nil)
	if w.Code != 200 || w.Header().Get("Content-Type") != "application/x-apple-aspen-config" {
		t.Fatalf("profile: %d %v", w.Code, w.Header())
	}
	token := h.state.IPhones[0].Token
	if !strings.Contains(w.Body.String(), "<string>"+web+"/udid/"+token+"</string>") || !strings.Contains(w.Body.String(), "Profile Service") {
		t.Errorf("profile: %s", w.Body)
	}

	sent := []byte("\x30\x80cms<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<!DOCTYPE plist PUBLIC \"-//Apple//DTD PLIST 1.0//EN\" \"http://www.apple.com/DTDs/PropertyList-1.0.dtd\">\n<plist version=\"1.0\">\n<dict>\n\t<key>PRODUCT</key>\n\t<string>iPhone18,1</string>\n\t<key>UDID</key>\n\t<string>" + udid + "</string>\n\t<key>VERSION</key>\n\t<string>24B91</string>\n</dict>\n</plist>\n\x00\x00sig")
	if w := webCall(t, h, friendIP, "POST", "/udid/"+token, sent); w.Code != 403 {
		t.Errorf("UDID from another node: %d", w.Code)
	}
	if w := webCall(t, h, iphoneIP, "POST", "/udid/wrong", sent); w.Code != 403 {
		t.Errorf("UDID with a wrong token: %d", w.Code)
	}
	w = webCall(t, h, iphoneIP, "POST", "/udid/"+token, sent)
	if w.Code != http.StatusMovedPermanently || w.Header().Get("Location") != web+"/?udid=1" {
		t.Fatalf("UDID: %d %v", w.Code, w.Header())
	}
	if ip := h.state.IPhones[0]; ip.UDID != udid || ip.Model != "iPhone18,1" || ip.OS != "24B91" {
		t.Errorf("kept %+v", ip)
	}
}

func TestIPhoneDelivery(t *testing.T) {
	ps := &pushService{}
	srv, sub := ps.start(t)
	h := newWebHost(t, srv.Client())
	pairIPhone(t, h, iphoneIP, "")
	pairIPhone(t, h, friendIP, "sam")
	h.state.IPhones[0].UDID = udid

	w := webCall(t, h, iphoneIP, "POST", "/api/notify", sub)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"sent":true`) || ps.count() != 1 {
		t.Fatalf("subscribe: %d %s, %d sent", w.Code, w.Body, ps.count())
	}

	ctx := context.Background()
	if _, err := h.Push(ctx, PushRequest{Path: makeApp(t, "00008140-FFFFFFFFFFFFFFFF")}); err == nil || !strings.Contains(err.Error(), udid) {
		t.Errorf("push for another device: %v", err)
	}
	u, err := h.Push(ctx, PushRequest{Path: makeApp(t)})
	if err != nil || len(u.Warnings) != 1 || !strings.Contains(u.Warnings[0], "SideStore") {
		t.Errorf("push unsigned: %+v %v", u.Warnings, err)
	}
	waitFor := func(what string, ok func() bool) {
		t.Helper()
		ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		if !h.wait(ctx, ok) {
			t.Fatalf("waiting for %s", what)
		}
	}
	waitFor("the unsigned build's notification", func() bool { r := h.find(u.ID); return !r.Taken.IsZero() || r.NotifyErr != "" })
	var ub webBuild
	json.Unmarshal(webCall(t, h, iphoneIP, "GET", "/api/builds/"+u.ID, nil).Body.Bytes(), &ub)
	if !ub.Sideload || ub.Download != "/m/"+u.ID+"/app.ipa?download=1" {
		t.Errorf("unsigned build: %+v", ub)
	}
	if w := webCall(t, h, iphoneIP, "GET", ub.Download, nil); w.Code != 200 || w.Header().Get("Content-Disposition") != `attachment; filename=Satchel.ipa` {
		t.Errorf("download: %d %v", w.Code, w.Header())
	}
	webCall(t, h, iphoneIP, "POST", "/api/builds/"+u.ID+"/clear", nil)
	if _, err := h.Push(ctx, PushRequest{Path: makeApp(t, udid), To: "nobody"}); err == nil {
		t.Error("push to nobody")
	}

	s, err := h.Push(ctx, PushRequest{Path: makeApp(t, udid), Note: "first"})
	if err != nil {
		t.Fatal(err)
	}
	if s.To != "iphone-17-pro" || s.IPA == nil || s.IPA.BundleID != "com.example.satchel" || len(s.Warnings) != 0 || s.File.Name != "Satchel.ipa" {
		t.Errorf("summary: %+v", s)
	}
	waitFor("the notification", func() bool { r := h.find(s.ID); return !r.Taken.IsZero() || r.NotifyErr != "" })
	if r := h.find(s.ID); r.NotifyErr != "" || ps.count() != 3 {
		t.Errorf("notify: %q, %d sent", r.NotifyErr, ps.count())
	}

	res := pairPhone(t, h)
	var in wire.Inbox
	json.Unmarshal(call(t, h.PhoneAPI(), phoneIP, "GET", "/v1/inbox?after=0&wait=0", res.Secret, nil).Body.Bytes(), &in)
	if len(in.Deliveries) != 0 {
		t.Errorf("Android got an iPhone's build: %d", len(in.Deliveries))
	}
	if w := call(t, h.PhoneAPI(), phoneIP, "GET", "/v1/apk/"+s.ID, res.Secret, nil); w.Code != 404 {
		t.Errorf("Android fetched an .ipa: %d", w.Code)
	}

	var me struct {
		Paired bool
		Builds []webBuild
	}
	json.Unmarshal(webCall(t, h, iphoneIP, "GET", "/api/me", nil).Body.Bytes(), &me)
	if !me.Paired || len(me.Builds) != 1 || me.Builds[0].Status != "new" || me.Builds[0].Covers == nil || !*me.Builds[0].Covers {
		t.Fatalf("me: %+v", me)
	}
	if want := "itms-services://?action=download-manifest&url=https%3A%2F%2Fmac.example.ts.net%3A41780%2Fm%2F" + s.ID + "%2Fmanifest.plist"; me.Builds[0].Install != want {
		t.Errorf("install link %s", me.Builds[0].Install)
	}
	json.Unmarshal(webCall(t, h, friendIP, "GET", "/api/me", nil).Body.Bytes(), &me)
	if len(me.Builds) != 0 {
		t.Error("the friend sees my build")
	}
	for _, p := range []string{"/api/builds/" + s.ID, "/m/" + s.ID + "/manifest.plist", "/m/" + s.ID + "/app.ipa"} {
		if w := webCall(t, h, friendIP, "GET", p, nil); w.Code != 404 {
			t.Errorf("friend got %s: %d", p, w.Code)
		}
	}

	if w := webCall(t, h, iphoneIP, "GET", "/api/builds/"+s.ID, nil); w.Code != 200 || h.find(s.ID).Opened.IsZero() {
		t.Errorf("build page: %d", w.Code)
	}
	w = webCall(t, h, iphoneIP, "GET", "/m/"+s.ID+"/manifest.plist", nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "<string>"+web+"/m/"+s.ID+"/app.ipa</string>") ||
		!strings.Contains(w.Body.String(), "<string>com.example.satchel</string>") || h.find(s.ID).Result.Status != wire.Asked {
		t.Errorf("manifest: %d %s", w.Code, w.Body)
	}
	req := httptest.NewRequest("GET", web+"/m/"+s.ID+"/app.ipa", nil)
	req.RemoteAddr = iphoneIP + ":1"
	req.Header.Set("Range", "bytes=0-99")
	rw := httptest.NewRecorder()
	h.WebApp().ServeHTTP(rw, req)
	if rw.Code != 206 || h.find(s.ID).Result.Status != wire.Installing {
		t.Errorf("partial: %d %v", rw.Code, h.find(s.ID).Result)
	}
	w = webCall(t, h, iphoneIP, "GET", "/m/"+s.ID+"/app.ipa", nil)
	if w.Code != 200 || int64(w.Body.Len()) != s.File.Size || h.find(s.ID).Result.Status != wire.Downloaded || !h.find(s.ID).Result.Final() {
		t.Errorf("download: %d, %d bytes, %v", w.Code, w.Body.Len(), h.find(s.ID).Result)
	}

	f, err := h.Push(ctx, PushRequest{Path: makeApp(t, udid), To: "sam"})
	if err != nil {
		t.Fatal(err)
	}
	if f.To != "sam" || len(f.Warnings) != 1 || !strings.Contains(f.Warnings[0], "UDID") {
		t.Errorf("friend's summary: %+v", f)
	}
	waitFor("the friend's notification", func() bool { return h.find(f.ID).NotifyErr != "" })
	if !strings.Contains(h.find(f.ID).NotifyErr, "notifications aren't on") {
		t.Errorf("friend without notifications: %q", h.find(f.ID).NotifyErr)
	}

	webCall(t, h, iphoneIP, "POST", "/api/builds/"+s.ID+"/clear", nil)
	json.Unmarshal(webCall(t, h, iphoneIP, "GET", "/api/me", nil).Body.Bytes(), &me)
	if len(me.Builds) != 0 {
		t.Error("a cleared build is listed")
	}

	ps.status = http.StatusGone
	g, err := h.Push(ctx, PushRequest{Path: makeApp(t, udid)})
	if err != nil {
		t.Fatal(err)
	}
	waitFor("the failed notification", func() bool { return h.find(g.ID).NotifyErr != "" })
	if h.state.IPhones[0].Push != nil {
		t.Error("kept a gone subscription")
	}

	if w := webCall(t, h, friendIP, "POST", "/api/unpair", nil); w.Code != 204 || len(h.state.IPhones) != 1 {
		t.Errorf("unpair: %d, %d left", w.Code, len(h.state.IPhones))
	}
	if _, err := h.Push(ctx, PushRequest{Path: makeApp(t, udid), To: "sam"}); err == nil {
		t.Error("pushed to an unpaired friend")
	}
}

func TestIPhoneNeedsTheWebApp(t *testing.T) {
	h := newHost(t)
	if _, err := h.StartPairingIPhone(time.Minute, ""); err == nil {
		t.Error("iPhone pairing without the web app")
	}
	if _, err := h.Push(context.Background(), PushRequest{Path: makeApp(t, udid)}); err == nil {
		t.Error("pushed an .app without the web app")
	}
}

func TestToPrefersNames(t *testing.T) {
	h := newWebHost(t, nil)
	pairIPhone(t, h, friendIP, "sam")
	h.state.IPhones[0].Machine = "iPhone 17 Pro"
	if name := pairIPhone(t, h, iphoneIP, ""); name != "iphone-17-pro" {
		t.Fatalf("mine is %q", name)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if ip, _ := h.target("iphone-17-pro"); ip == nil || ip.Node != "nIPhone" {
		t.Errorf("-to iphone-17-pro picked %+v", ip)
	}
	if ip, _ := h.target("iPhone 17 Pro"); ip != nil {
		t.Errorf("an ambiguous machine name picked %s", ip.Name)
	}
	h.state.IPhones[1].Machine = "mine"
	if ip, _ := h.target("iPhone 17 Pro"); ip != nil {
		t.Errorf("a friend's machine name picked %s", ip.Name)
	}
	if ip, _ := h.target("mine"); ip == nil || ip.Node != "nIPhone" {
		t.Errorf("my machine name picked %+v", ip)
	}
}

func TestDownloadStatus(t *testing.T) {
	h := newWebHost(t, nil)
	pairIPhone(t, h, iphoneIP, "")
	s, err := h.Push(context.Background(), PushRequest{Path: makeApp(t, udid)})
	if err != nil {
		t.Fatal(err)
	}
	get := func(method, rng string) int {
		req := httptest.NewRequest(method, web+"/m/"+s.ID+"/app.ipa", nil)
		req.RemoteAddr = iphoneIP + ":1"
		if rng != "" {
			req.Header.Set("Range", rng)
		}
		w := httptest.NewRecorder()
		h.WebApp().ServeHTTP(w, req)
		return w.Code
	}
	status := func() string {
		h.mu.Lock()
		defer h.mu.Unlock()
		if r := h.find(s.ID); r.Result != nil {
			return r.Result.Status
		}
		return ""
	}
	size := s.File.Size

	if code := get("GET", fmt.Sprintf("bytes=%d-", size-22)); code != 206 || status() != wire.Installing {
		t.Errorf("tail: %d %s", code, status())
	}
	get("GET", "bytes=0-99")
	if status() != wire.Installing {
		t.Errorf("first half: %s", status())
	}
	get("GET", "bytes=100-")
	if status() != wire.Downloaded {
		t.Errorf("both halves: %s", status())
	}
	get("HEAD", "")
	get("GET", "bytes=0-9")
	webCall(t, h, iphoneIP, "GET", "/m/"+s.ID+"/manifest.plist", nil)
	if status() != wire.Downloaded {
		t.Errorf("after: %s", status())
	}
}

func TestDeadSubscription(t *testing.T) {
	ps := &pushService{status: http.StatusGone}
	srv, sub := ps.start(t)
	h := newWebHost(t, srv.Client())
	pairIPhone(t, h, iphoneIP, "")
	w := webCall(t, h, iphoneIP, "POST", "/api/notify", sub)
	var res struct {
		Sent, Gone bool
		Error      string
	}
	json.Unmarshal(w.Body.Bytes(), &res)
	if res.Sent || !res.Gone || h.state.IPhones[0].Push != nil {
		t.Errorf("gone: %s, kept %v", w.Body, h.state.IPhones[0].Push)
	}

	ps.status = http.StatusForbidden
	w = webCall(t, h, iphoneIP, "POST", "/api/notify", sub)
	json.Unmarshal(w.Body.Bytes(), &res)
	if res.Sent || res.Gone || res.Error != "the push service refused it (403)" {
		t.Errorf("refused: %s", w.Body)
	}

	webpush.AllowHost = func(h string) bool { return false }
	sub.Endpoint = "https://192.168.1.1/admin"
	if w := webCall(t, h, iphoneIP, "POST", "/api/notify", sub); w.Code != 400 {
		t.Errorf("subscription to a LAN address: %d", w.Code)
	}
}

func TestPruneKeepsEachKind(t *testing.T) {
	h := newWebHost(t, nil)
	pairIPhone(t, h, iphoneIP, "")
	a, err := h.Push(context.Background(), PushRequest{Path: fakeAPK(t, "x")})
	if err != nil {
		t.Fatal(err)
	}
	for range Keep + 2 {
		if _, err := h.Push(context.Background(), PushRequest{Path: makeApp(t, udid)}); err != nil {
			t.Fatal(err)
		}
	}
	if h.find(a.ID) == nil {
		t.Error("iPhone builds pushed out the phone's")
	}
	n := 0
	for _, r := range h.records {
		if r.To != "" {
			n++
		}
	}
	if n != Keep {
		t.Errorf("kept %d iPhone builds", n)
	}
}

func TestWebHeadersAndPaths(t *testing.T) {
	h := newWebHost(t, nil)
	pairIPhone(t, h, iphoneIP, "")
	s, err := h.Push(context.Background(), PushRequest{
		Path:   makeApp(t),
		Source: &wire.Source{Dir: "/Users/me/dev/satchel", Repo: "satchel‮gpj", Commit: "abc\x1b[2J"},
	})
	if err != nil {
		t.Fatal(err)
	}
	w := webCall(t, h, iphoneIP, "GET", "/api/builds/"+s.ID, nil)
	if w.Code != 200 {
		t.Fatalf("build: %d %s", w.Code, w.Body)
	}
	if csp := w.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "frame-ancestors 'none'") || !strings.Contains(csp, "default-src 'self'") {
		t.Errorf("CSP %q", csp)
	}
	if strings.Contains(w.Body.String(), "/Users/me") {
		t.Errorf("an iPhone was sent a path on the computer: %s", w.Body)
	}
	h.mu.Lock()
	src := h.find(s.ID).m.Source
	h.mu.Unlock()
	if src.Repo != "satchelgpj" || src.Commit != "abc[2J" || src.Dir != "/Users/me/dev/satchel" {
		t.Errorf("source kept as %+v", src)
	}
}
