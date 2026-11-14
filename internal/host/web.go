package host

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ahmd-sh/cloche/internal/tailnet"
	"github.com/ahmd-sh/cloche/internal/webpush"
	"github.com/ahmd-sh/cloche/internal/wire"
)

var staticTypes = map[string]string{
	".js":          "text/javascript; charset=utf-8",
	".css":         "text/css; charset=utf-8",
	".webmanifest": "application/manifest+json",
	".ttf":         "font/ttf",
	".png":         "image/png",
}

const WebPort = 41780

var serviceWorker = sync.OnceValue(func() []byte {
	h := sha256.New()
	fs.WalkDir(webFiles, "web", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := webFiles.ReadFile(p)
		if err != nil {
			return err
		}
		fmt.Fprintf(h, "%s %d\n", p, len(b))
		h.Write(b)
		return nil
	})
	b, _ := webFiles.ReadFile("web/sw.js")
	return bytes.Replace(b, []byte("'cloche-files'"), []byte("'cloche-"+hex.EncodeToString(h.Sum(nil))[:12]+"'"), 1)
})

//go:embed web
var webFiles embed.FS

func (h *Host) WebApp() http.Handler {
	static, _ := fs.Sub(webFiles, "web")
	files := http.FileServerFS(static)
	mux := http.NewServeMux()
	shell := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeFileFS(w, r, static, "index.html")
	}
	mux.HandleFunc("GET /{$}", shell)
	mux.HandleFunc("GET /pair", shell)
	mux.HandleFunc("GET /b/{id}", shell)
	mux.HandleFunc("GET /sw.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Content-Type", staticTypes[".js"])
		w.Write(serviceWorker())
	})
	mux.Handle("GET /", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		for ext, typ := range staticTypes {
			if strings.HasSuffix(r.URL.Path, ext) {
				w.Header().Set("Content-Type", typ)
			}
		}
		files.ServeHTTP(w, r)
	}))

	mux.HandleFunc("GET /api/me", h.webMe)
	mux.HandleFunc("POST /api/pair", h.sameOrigin(h.webPair))
	mux.HandleFunc("POST /api/notify", h.sameOrigin(h.paired(h.webSubscribe)))
	mux.HandleFunc("POST /api/unpair", h.sameOrigin(h.paired(h.webUnpair)))
	mux.HandleFunc("GET /api/builds/{id}", h.paired(h.webBuild))
	mux.HandleFunc("POST /api/builds/{id}/clear", h.sameOrigin(h.paired(h.webClear)))
	mux.HandleFunc("GET /udid.mobileconfig", h.paired(h.webUDIDProfile))
	mux.HandleFunc("POST /udid/{token}", h.webUDID)
	mux.HandleFunc("GET /m/{id}/manifest.plist", h.paired(h.webManifest))
	mux.HandleFunc("GET /m/{id}/app.ipa", h.paired(h.webIPA))
	mux.HandleFunc("GET /m/{id}/icon.png", h.paired(h.webIcon))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if u, err := url.Parse(h.web); err != nil || r.Host != u.Host {
			http.Error(w, "this is "+h.web, http.StatusMisdirectedRequest)
			return
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; base-uri 'none'; form-action 'self'; frame-ancestors 'none'")
		w.Header().Set("X-Frame-Options", "DENY")
		mux.ServeHTTP(w, r)
	})
}

func (h *Host) sameOrigin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != h.web && !(origin == "" && r.Header.Get("Sec-Fetch-Site") == "same-origin") {
			fail(w, http.StatusForbidden, "cross-site request")
			return
		}
		next(w, r)
	}
}

type ctxKey struct{}

func (h *Host) paired(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, ok := h.webPeer(w, r)
		if !ok {
			return
		}
		h.mu.Lock()
		ip := h.iphone(p.Node)
		if ip != nil {
			h.seenAt[ip.Node] = h.now()
		}
		h.mu.Unlock()
		if ip == nil {
			fail(w, http.StatusUnauthorized, "this device isn't paired")
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, ip.Node)))
	}
}

func nodeOf(r *http.Request) string {
	n, _ := r.Context().Value(ctxKey{}).(string)
	return n
}

func (h *Host) webPeer(w http.ResponseWriter, r *http.Request) (tailnet.Peer, bool) {
	p, err := h.peer(r)
	if err != nil {
		h.log.Printf("web %s %s from %s: %v", r.Method, r.URL.Path, r.RemoteAddr, err)
		fail(w, http.StatusServiceUnavailable, "can't ask Tailscale who this is")
		return p, false
	}
	return p, true
}

type webBuild struct {
	ID       string       `json:"id"`
	Name     string       `json:"name"`
	BundleID string       `json:"bundle_id"`
	Version  string       `json:"version"`
	Note     string       `json:"note,omitempty"`
	Source   *wire.Source `json:"source,omitempty"`
	PushedAt time.Time    `json:"pushed_at"`
	Size     int64        `json:"size"`
	Icon     bool         `json:"icon"`
	Kind     string       `json:"kind,omitempty"`
	Team     string       `json:"team,omitempty"`
	Expires  time.Time    `json:"expires,omitzero"`
	MinOS    string       `json:"min_os,omitempty"`
	Covers   *bool        `json:"covers,omitempty"`
	Status   string       `json:"status"`
	StatusAt time.Time    `json:"status_at,omitzero"`
	Install  string       `json:"install"`
	Sideload bool         `json:"sideload,omitempty"`
	Download string       `json:"download"`
}

func (h *Host) webBuildOf(r *record, ip *iphone) webBuild {
	var src *wire.Source
	if s := r.m.Source; s != nil {
		c := *s
		c.Dir = ""
		src = &c
	}
	b := webBuild{
		ID: r.m.ID, Name: r.IPA.Name, BundleID: r.IPA.BundleID, Version: version(r.IPA), Note: r.m.Note, Source: src,
		PushedAt: r.m.PushedAt, Size: r.m.File.Size, MinOS: r.IPA.MinOS, Status: "new",
		Install:  "itms-services://?action=download-manifest&url=" + url.QueryEscape(h.web+"/m/"+r.m.ID+"/manifest.plist"),
		Download: "/m/" + r.m.ID + "/app.ipa?download=1",
		Sideload: r.IPA.Profile == nil,
	}
	if _, err := os.Stat(h.path("deliveries", r.m.ID+".png")); err == nil {
		b.Icon = true
	}
	if p := r.IPA.Profile; p != nil {
		b.Kind, b.Team, b.Expires = p.Kind, p.Team, p.Expires
		if ip.UDID != "" || p.AllDevices {
			c := p.Covers(ip.UDID)
			b.Covers = &c
		}
	}
	switch {
	case r.Result != nil && r.Result.Status == wire.Downloaded:
		b.Status, b.StatusAt = "downloaded", r.ResultAt
	case r.Result != nil && r.Result.Status == wire.Installing:
		b.Status, b.StatusAt = "downloading", r.ResultAt
	case r.Result != nil && r.Result.Status == wire.Asked:
		b.Status, b.StatusAt = "asked", r.ResultAt
	case !r.Opened.IsZero():
		b.Status, b.StatusAt = "seen", r.Opened
	}
	return b
}

func (h *Host) webMe(w http.ResponseWriter, r *http.Request) {
	p, ok := h.webPeer(w, r)
	if !ok {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	w.Header().Set("Cache-Control", "no-store")
	ip := h.iphone(p.Node)
	if ip == nil {
		pr := h.pairing
		open := pr != nil && pr.ios && h.now().Before(pr.expires) && (pr.friend != "" || h.owner == "" || p.Login == h.owner)
		reply(w, map[string]any{"host": h.name, "paired": false, "pairing": open})
		return
	}
	h.seenAt[ip.Node] = h.now()
	builds := []webBuild{}
	for i := len(h.records) - 1; i >= 0; i-- {
		if rec := h.records[i]; rec.To == ip.Node && !rec.Cleared && rec.IPA != nil {
			builds = append(builds, h.webBuildOf(rec, ip))
		}
	}
	reply(w, map[string]any{
		"host": h.name, "paired": true, "name": ip.Name, "friend": ip.Friend,
		"udid": ip.UDID != "", "model": ip.Model, "notify": ip.Push != nil,
		"vapid": h.pusher.PublicKey(), "builds": builds,
	})
}

func (h *Host) webPair(w http.ResponseWriter, r *http.Request) {
	p, ok := h.webPeer(w, r)
	if !ok {
		return
	}
	var req struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&req); err != nil {
		fail(w, http.StatusBadRequest, "bad request")
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	pr := h.pairing
	switch {
	case pr == nil || !h.now().Before(pr.expires):
		fail(w, http.StatusForbidden, "pairing isn't open: ask for a new link")
		return
	case !pr.ios:
		fail(w, http.StatusForbidden, "this code is for Cloche on Android")
		return
	case pr.friend == "" && !h.ownedBy(p):
		h.log.Printf("iPhone pair refused: %s is %s's, not %s's", p.Name, p.Login, h.owner)
		fail(w, http.StatusForbidden, "this link is for "+h.owner+"'s own devices")
		return
	}
	if subtle.ConstantTimeCompare([]byte(normalCode(req.Code)), []byte(normalCode(pr.code))) != 1 {
		pr.attempts++
		h.log.Printf("iPhone pair from %s: wrong code (%d)", p.Name, pr.attempts)
		if pr.attempts >= 5 {
			h.pairing = nil
			h.broadcast()
			fail(w, http.StatusForbidden, "wrong code too many times: ask for a new link")
			return
		}
		fail(w, http.StatusForbidden, "wrong code")
		return
	}

	ip := h.iphone(p.Node)
	if ip == nil {
		ip = &iphone{Node: p.Node, Token: newSecret()}
		h.state.IPhones = append(h.state.IPhones, ip)
	}
	ip.Machine, ip.Login, ip.Friend, ip.PairedAt = p.Name, p.Login, pr.friend != "", h.now().UTC()
	ip.Name = h.uniqueName(ip, firstNonEmpty(pr.friend, nameFrom(p.Name)))
	h.pairing = nil
	h.epoch++
	h.seenAt[ip.Node] = h.now()
	if err := h.saveState(); err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.broadcast()
	h.log.Printf("paired with iPhone %s (%s, %s)", ip.Name, p.Name, p.Login)
	reply(w, map[string]any{"name": ip.Name})
}

func nameFrom(machine string) string {
	var b strings.Builder
	for _, c := range strings.ToLower(machine) {
		switch {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9':
			b.WriteRune(c)
		case b.Len() > 0 && !strings.HasSuffix(b.String(), "-"):
			b.WriteByte('-')
		}
	}
	s := strings.Trim(b.String(), "-")
	if len(s) > 28 {
		s = strings.Trim(s[:28], "-")
	}
	if s == "" {
		s = "iphone"
	}
	return s
}

func (h *Host) uniqueName(self *iphone, name string) string {
	for i := 1; ; i++ {
		n := name
		if i > 1 {
			n = fmt.Sprintf("%s-%d", name, i)
		}
		taken := false
		for _, ip := range h.state.IPhones {
			if ip != self && strings.EqualFold(ip.Name, n) {
				taken = true
			}
		}
		if !taken {
			return n
		}
	}
}

func firstNonEmpty(s ...string) string {
	for _, v := range s {
		if v != "" {
			return v
		}
	}
	return ""
}

func (h *Host) webSubscribe(w http.ResponseWriter, r *http.Request) {
	var sub webpush.Subscription
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&sub); err != nil {
		fail(w, http.StatusBadRequest, "bad request")
		return
	}
	if err := sub.Check(); err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	h.mu.Lock()
	ip := h.iphone(nodeOf(r))
	if ip == nil {
		h.mu.Unlock()
		fail(w, http.StatusUnauthorized, "this device isn't paired")
		return
	}
	ip.Push = &sub
	h.epoch++
	err := h.saveState()
	h.broadcast()
	h.mu.Unlock()
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}

	msg, _ := json.Marshal(map[string]string{"title": "Notifications are on", "body": "Builds from " + h.name + " show up here.", "url": "/", "tag": "cloche"})
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	err = h.pusher.Send(ctx, sub, webpush.Message{Payload: msg, TTL: time.Hour, Urgency: "high"})
	if err == nil {
		reply(w, map[string]any{"sent": true})
		return
	}
	h.log.Printf("first notification to %s: %v", nodeOf(r), err)
	gone := errors.Is(err, webpush.ErrGone)
	if gone {
		h.mu.Lock()
		if ip := h.iphone(nodeOf(r)); ip != nil && ip.Push != nil && ip.Push.Endpoint == sub.Endpoint {
			ip.Push = nil
			h.epoch++
			h.saveState()
			h.broadcast()
		}
		h.mu.Unlock()
	}
	var refused *webpush.Refused
	why := "the push service couldn't be reached"
	if errors.As(err, &refused) {
		why = fmt.Sprintf("the push service refused it (%d)", refused.Status)
	}
	reply(w, map[string]any{"sent": false, "gone": gone, "error": why})
}

func (h *Host) webUnpair(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if ip := h.iphone(nodeOf(r)); ip != nil {
		if err := h.forgetIPhone(ip); err != nil {
			fail(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Host) mine(r *http.Request) (*record, *iphone) {
	rec := h.find(r.PathValue("id"))
	ip := h.iphone(nodeOf(r))
	if rec == nil || ip == nil || rec.To != ip.Node || rec.IPA == nil {
		return nil, nil
	}
	return rec, ip
}

func (h *Host) webBuild(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	defer h.mu.Unlock()
	rec, ip := h.mine(r)
	if rec == nil {
		fail(w, http.StatusNotFound, "no such build: it may have been cleared")
		return
	}
	if rec.Opened.IsZero() {
		rec.Opened = h.now().UTC()
		h.saveRecord(rec)
		h.broadcast()
	}
	w.Header().Set("Cache-Control", "no-store")
	reply(w, h.webBuildOf(rec, ip))
}

func (h *Host) webClear(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	defer h.mu.Unlock()
	rec, _ := h.mine(r)
	if rec == nil {
		fail(w, http.StatusNotFound, "no such build")
		return
	}
	rec.Cleared = true
	h.saveRecord(rec)
	h.broadcast()
	w.WriteHeader(http.StatusNoContent)
}

func (h *Host) webUDIDProfile(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	ip := h.iphone(nodeOf(r))
	token := ""
	if ip != nil {
		token = ip.Token
	}
	h.mu.Unlock()
	if ip == nil {
		fail(w, http.StatusUnauthorized, "this device isn't paired")
		return
	}
	uuid := make([]byte, 16)
	rand.Read(uuid)
	profile := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>PayloadContent</key>
	<dict>
		<key>URL</key>
		<string>` + xmlEscape(h.web+"/udid/"+token) + `</string>
		<key>DeviceAttributes</key>
		<array>
			<string>UDID</string>
			<string>PRODUCT</string>
			<string>VERSION</string>
		</array>
	</dict>
	<key>PayloadOrganization</key>
	<string>Cloche</string>
	<key>PayloadDisplayName</key>
	<string>` + xmlEscape("Send this iPhone's ID to "+h.name) + `</string>
	<key>PayloadDescription</key>
	<string>` + xmlEscape("Sends "+h.name+" this device's UDID, model and iOS version, so builds can be signed for it. Nothing is installed.") + `</string>
	<key>PayloadIdentifier</key>
	<string>sh.ahmd.cloche.udid</string>
	<key>PayloadUUID</key>
	<string>` + fmt.Sprintf("%X-%X-%X-%X-%X", uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:]) + `</string>
	<key>PayloadType</key>
	<string>Profile Service</string>
	<key>PayloadVersion</key>
	<integer>1</integer>
</dict>
</plist>
`)
	if h.signProfile != nil {
		if signed, err := h.signProfile(profile); err == nil {
			profile = signed
		} else {
			h.log.Printf("signing the UDID profile: %v", err)
		}
	}
	w.Header().Set("Content-Type", "application/x-apple-aspen-config")
	w.Header().Set("Content-Disposition", `attachment; filename="cloche-udid.mobileconfig"`)
	w.Header().Set("Cache-Control", "no-store")
	w.Write(profile)
}

func (h *Host) webUDID(w http.ResponseWriter, r *http.Request) {
	p, ok := h.webPeer(w, r)
	if !ok {
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 64<<10))
	if err != nil {
		fail(w, http.StatusBadRequest, "bad request")
		return
	}
	start, end := bytes.Index(body, []byte("<?xml")), bytes.LastIndex(body, []byte("</plist>"))
	if start < 0 || end < start {
		fail(w, http.StatusBadRequest, "no plist")
		return
	}
	attrs, err := deviceAttributes(body[start : end+len("</plist>")])
	if err != nil || attrs["UDID"] == "" {
		fail(w, http.StatusBadRequest, "no UDID")
		return
	}

	h.mu.Lock()
	var ip *iphone
	for _, x := range h.state.IPhones {
		if subtle.ConstantTimeCompare([]byte(x.Token), []byte(r.PathValue("token"))) == 1 {
			ip = x
		}
	}
	if ip == nil || ip.Node != p.Node {
		h.mu.Unlock()
		h.log.Printf("UDID from %s refused: not the paired iPhone the link was for", p.Name)
		fail(w, http.StatusForbidden, "not the iPhone this profile was made for")
		return
	}
	ip.UDID, ip.Model, ip.OS = clean(attrs["UDID"], 64), clean(attrs["PRODUCT"], 32), clean(attrs["VERSION"], 32)
	h.epoch++
	h.seenAt[ip.Node] = h.now()
	h.saveState()
	h.broadcast()
	h.log.Printf("iPhone %s sent its UDID %s (%s, %s)", ip.Name, ip.UDID, ip.Model, ip.OS)
	h.mu.Unlock()
	http.Redirect(w, r, h.web+"/?udid=1", http.StatusMovedPermanently)
}

func deviceAttributes(b []byte) (map[string]string, error) {
	d := xml.NewDecoder(bytes.NewReader(b))
	d.Strict = false
	out := map[string]string{}
	key, in := "", ""
	for {
		t, err := d.Token()
		if err == io.EOF {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		switch t := t.(type) {
		case xml.StartElement:
			in = t.Name.Local
		case xml.EndElement:
			in = ""
		case xml.CharData:
			switch in {
			case "key":
				key = string(t)
			case "string":
				out[key] = strings.TrimSpace(string(t))
			}
		}
	}
}

func (h *Host) webManifest(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	rec, _ := h.mine(r)
	if rec == nil {
		h.mu.Unlock()
		http.NotFound(w, r)
		return
	}
	rec.Asked = h.now().UTC()
	if !downloaded(rec) {
		rec.Result, rec.ResultAt = &wire.Result{Status: wire.Asked}, rec.Asked
	}
	h.saveRecord(rec)
	h.broadcast()
	info := rec.IPA
	id := rec.m.ID
	h.mu.Unlock()

	plist := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>items</key>
	<array>
		<dict>
			<key>assets</key>
			<array>
				<dict>
					<key>kind</key>
					<string>software-package</string>
					<key>url</key>
					<string>` + xmlEscape(h.web+"/m/"+id+"/app.ipa") + `</string>
				</dict>
			</array>
			<key>metadata</key>
			<dict>
				<key>bundle-identifier</key>
				<string>` + xmlEscape(info.BundleID) + `</string>
				<key>bundle-version</key>
				<string>` + xmlEscape(firstNonEmpty(info.Version, info.Build, "1")) + `</string>
				<key>kind</key>
				<string>software</string>
				<key>title</key>
				<string>` + xmlEscape(info.Name) + `</string>
			</dict>
		</dict>
	</array>
</dict>
</plist>
`
	w.Header().Set("Content-Type", "text/xml; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	io.WriteString(w, plist)
}

func downloaded(r *record) bool { return r.Result != nil && r.Result.Status == wire.Downloaded }

func (h *Host) webIPA(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	rec, _ := h.mine(r)
	if rec == nil {
		h.mu.Unlock()
		http.NotFound(w, r)
		return
	}
	track := r.Method == http.MethodGet && !downloaded(rec)
	if track {
		now := h.now().UTC()
		if rec.Fetched.IsZero() {
			rec.Fetched = now
		}
		rec.Result, rec.ResultAt = &wire.Result{Status: wire.Installing}, now
		h.saveRecord(rec)
		h.broadcast()
	}
	id, size, name := rec.m.ID, rec.m.File.Size, rec.m.File.Name
	h.mu.Unlock()

	f, err := os.Open(h.path("deliveries", id+".ipa"))
	if err != nil {
		http.Error(w, "its file is gone", http.StatusNotFound)
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	if r.URL.Query().Has("download") {
		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
	}
	cw := &counter{ResponseWriter: w}
	http.ServeContent(cw, r, "", st.ModTime(), f)

	if !track {
		return
	}
	from := int64(-1)
	switch cw.status {
	case 0, http.StatusOK:
		from = 0
	case http.StatusPartialContent:
		if s, ok := strings.CutPrefix(r.Header.Get("Range"), "bytes="); ok && !strings.Contains(s, ",") {
			if a, _, ok := strings.Cut(s, "-"); ok && a != "" {
				from, _ = strconv.ParseInt(a, 10, 64)
			}
		}
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	rec = h.find(id)
	if rec == nil || downloaded(rec) {
		return
	}
	if from >= 0 && from <= h.served[id] {
		h.served[id] = max(h.served[id], from+cw.n)
	}
	if h.served[id] >= size {
		delete(h.served, id)
		rec.Result, rec.ResultAt = &wire.Result{Status: wire.Downloaded}, h.now().UTC()
		h.log.Printf("%s: downloaded by %s", id, h.summary(rec).To)
		h.saveRecord(rec)
		h.broadcast()
	}
}

type counter struct {
	http.ResponseWriter
	status int
	n      int64
}

func (c *counter) WriteHeader(code int) {
	c.status = code
	c.ResponseWriter.WriteHeader(code)
}

func (c *counter) Write(b []byte) (int, error) {
	n, err := c.ResponseWriter.Write(b)
	c.n += int64(n)
	return n, err
}

func (h *Host) webIcon(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	rec, _ := h.mine(r)
	h.mu.Unlock()
	if rec == nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "private, max-age=86400")
	http.ServeFile(w, r, h.path("deliveries", rec.m.ID+".png"))
}

func xmlEscape(s string) string {
	var b bytes.Buffer
	xml.EscapeText(&b, []byte(s))
	return b.String()
}
