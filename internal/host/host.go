package host

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base32"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/ahmd-sh/cloche/internal/apk"
	"github.com/ahmd-sh/cloche/internal/ipa"
	"github.com/ahmd-sh/cloche/internal/tailnet"
	"github.com/ahmd-sh/cloche/internal/webpush"
	"github.com/ahmd-sh/cloche/internal/wire"
)

const Keep = 20

const MaxSize = 1 << 30

type Config struct {
	Dir      string
	Name     string
	Addr     string
	Peers    tailnet.Whois
	Owner    string
	AnyOwner bool
	DNSName  string
	Log      *log.Logger
	Now      func() time.Time

	Web         string
	SignProfile func([]byte) ([]byte, error)
	PushHTTP    *http.Client

	Version string
}

type Host struct {
	dir      string
	name     string
	addr     string
	key      *ecdsa.PrivateKey
	spki     []byte
	peers    tailnet.Whois
	owner    string
	anyOwner bool
	dnsName  string
	log      *log.Logger
	now      func() time.Time

	web         string
	signProfile func([]byte) ([]byte, error)
	pusher      *webpush.Sender

	mu      sync.Mutex
	state   state
	records []*record
	changed chan struct{}
	pairing *pairing
	polls   int
	seen    time.Time
	closed  chan struct{}
	webErr  string
	seenAt  map[string]time.Time
	served  map[string]int64
	epoch   int64

	version string
	update  string
}

type state struct {
	Seq     int64     `json:"seq"`
	Phone   *phone    `json:"phone,omitempty"`
	IPhones []*iphone `json:"iphones,omitempty"`
}

type phone struct {
	Node     string    `json:"node"`
	Name     string    `json:"name"`
	Device   string    `json:"device"`
	Secret   string    `json:"secret_sha256"`
	PairedAt time.Time `json:"paired_at"`

	App        string `json:"app,omitempty"`
	AppCode    int64  `json:"app_code,omitempty"`
	AppOffered string `json:"app_offered,omitempty"`
}

type record struct {
	Delivery wire.Delivery `json:"delivery"`
	Taken    time.Time     `json:"taken,omitzero"`
	Fetched  time.Time     `json:"fetched,omitzero"`
	Result   *wire.Result  `json:"result,omitempty"`
	ResultAt time.Time     `json:"result_at,omitzero"`

	To        string    `json:"to,omitempty"`
	IPA       *ipa.Info `json:"ipa,omitempty"`
	NotifyErr string    `json:"notify_error,omitempty"`
	Opened    time.Time `json:"opened,omitzero"`
	Asked     time.Time `json:"asked,omitzero"`
	Cleared   bool      `json:"cleared,omitempty"`

	m wire.Manifest
}

func (r *record) ext() string {
	if r.To != "" {
		return ".ipa"
	}
	return ".apk"
}

type pairing struct {
	code     string
	expires  time.Time
	attempts int
	ios      bool
	friend   string
}

func Open(c Config) (*Host, error) {
	if c.Log == nil {
		c.Log = log.New(io.Discard, "", 0)
	}
	if c.Now == nil {
		c.Now = time.Now
	}
	if err := os.MkdirAll(filepath.Join(c.Dir, "deliveries"), 0o700); err != nil {
		return nil, err
	}
	if err := os.Chmod(c.Dir, 0o700); err != nil {
		return nil, err
	}
	key, err := loadKey(filepath.Join(c.Dir, "key.pem"))
	if err != nil {
		return nil, err
	}
	spki, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		return nil, err
	}
	pushKey, err := loadKey(filepath.Join(c.Dir, "webpush.pem"))
	if err != nil {
		return nil, err
	}
	h := &Host{
		dir: c.Dir, name: c.Name, addr: c.Addr, key: key, spki: spki,
		peers: c.Peers, owner: c.Owner, anyOwner: c.AnyOwner, dnsName: strings.TrimSuffix(c.DNSName, "."), log: c.Log, now: c.Now,
		web: strings.TrimSuffix(c.Web, "/"), signProfile: c.SignProfile,
		changed: make(chan struct{}), closed: make(chan struct{}),
		seenAt: map[string]time.Time{}, served: map[string]int64{},
		version: c.Version,
	}
	h.pusher = &webpush.Sender{Key: pushKey, Subject: h.web, HTTP: c.PushHTTP}
	if h.pusher.Subject == "" {
		h.pusher.Subject = "mailto:cloche@localhost"
	}
	if b, err := os.ReadFile(h.path("state.json")); err == nil {
		if err := json.Unmarshal(b, &h.state); err != nil {
			return nil, fmt.Errorf("state.json: %w", err)
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	if err := h.loadRecords(); err != nil {
		return nil, err
	}
	return h, nil
}

func (h *Host) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	select {
	case <-h.closed:
	default:
		close(h.closed)
	}
}

func (h *Host) path(parts ...string) string {
	return filepath.Join(append([]string{h.dir}, parts...)...)
}

func (h *Host) Fingerprint() string { return Fingerprint(h.spki) }

func Fingerprint(spki []byte) string {
	d := sha256.Sum256(spki)
	x := strings.ToUpper(hex.EncodeToString(d[:8]))
	return x[0:4] + " " + x[4:8] + " " + x[8:12] + " " + x[12:16]
}

type PushRequest struct {
	Path   string       `json:"path"`
	Note   string       `json:"note,omitempty"`
	Source *wire.Source `json:"source,omitempty"`
	To     string       `json:"to,omitempty"`
}

func (h *Host) Push(ctx context.Context, req PushRequest) (Summary, error) {
	st, err := os.Stat(req.Path)
	switch {
	case err != nil:
		return Summary{}, err
	case st.IsDir() && strings.HasSuffix(filepath.Clean(req.Path), ".app"),
		st.Mode().IsRegular() && (strings.HasSuffix(req.Path, ".ipa") || ipa.IsIPA(req.Path)):
		return h.pushIPA(req, st.IsDir())
	case req.To != "":
		return Summary{}, errors.New("-to is for iPhones: this isn't an .ipa")
	case !st.Mode().IsRegular():
		return Summary{}, fmt.Errorf("%s isn't a file", req.Path)
	case st.Size() > MaxSize:
		return Summary{}, fmt.Errorf("%s is over %d MB", req.Path, MaxSize>>20)
	case !apk.IsZip(req.Path):
		return Summary{}, fmt.Errorf("%s isn't an APK", req.Path)
	}
	id := newID()
	sum, size, err := h.copyIn(req.Path, id)
	if err != nil {
		return Summary{}, err
	}
	info := cleanAPK(apk.Inspect(ctx, h.path("deliveries", id+".apk")))

	h.mu.Lock()
	defer h.mu.Unlock()
	m := wire.Manifest{
		V: wire.Version, ID: id, Seq: h.state.Seq + 1, Host: h.name, PushedAt: h.now().UTC().Truncate(time.Second),
		File:   wire.File{Name: clean(filepath.Base(req.Path), 255), Size: size, SHA256: sum},
		APK:    info,
		Note:   clean(req.Note, 500),
		Source: cleanSource(req.Source),
	}
	b, err := json.Marshal(m)
	if err != nil {
		return Summary{}, err
	}
	sig, err := h.sign(b)
	if err != nil {
		return Summary{}, err
	}
	r := &record{Delivery: wire.Delivery{Manifest: b, Sig: sig}, m: m}
	if err := h.saveRecord(r); err != nil {
		return Summary{}, err
	}
	h.state.Seq = m.Seq
	if err := h.saveState(); err != nil {
		h.state.Seq--
		os.Remove(h.path("deliveries", id+".json"))
		return Summary{}, err
	}
	h.records = append(h.records, r)
	h.prune()
	h.broadcast()
	h.log.Printf("pushed %s: %s", id, describe(m))
	return h.summary(r), nil
}

func (h *Host) copyIn(src, id string) (sum string, size int64, err error) {
	return h.copyInAs(src, id+".apk")
}

func (h *Host) copyInAs(src, name string) (sum string, size int64, err error) {
	in, err := os.Open(src)
	if err != nil {
		return "", 0, err
	}
	defer in.Close()
	dst := h.path("deliveries", name)
	out, err := os.OpenFile(dst+".tmp", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return "", 0, err
	}
	hash := sha256.New()
	size, err = io.Copy(io.MultiWriter(out, hash), io.LimitReader(in, MaxSize+1))
	if err == nil && size > MaxSize {
		err = errors.New("file grew past the limit while copying")
	}
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(dst+".tmp", dst)
	}
	if err != nil {
		os.Remove(dst + ".tmp")
		return "", 0, err
	}
	return hex.EncodeToString(hash.Sum(nil)), size, nil
}

func (h *Host) sign(b []byte) ([]byte, error) {
	d := sha256.Sum256(b)
	return ecdsa.SignASN1(rand.Reader, h.key, d[:])
}

func (h *Host) prune() {
	var phone, iphones int
	for _, r := range h.records {
		if r.To != "" {
			iphones++
		} else {
			phone++
		}
	}
	h.records = slices.DeleteFunc(h.records, func(r *record) bool {
		n := &phone
		if r.To != "" {
			n = &iphones
		}
		if *n <= Keep {
			return false
		}
		*n--
		os.Remove(h.path("deliveries", r.m.ID+r.ext()))
		os.Remove(h.path("deliveries", r.m.ID+".png"))
		os.Remove(h.path("deliveries", r.m.ID+".json"))
		delete(h.served, r.m.ID)
		return true
	})
}

func (h *Host) find(id string) *record {
	for _, r := range h.records {
		if r.m.ID == id {
			return r
		}
	}
	return nil
}

func (h *Host) broadcast() {
	close(h.changed)
	h.changed = make(chan struct{})
}

func (h *Host) wait(ctx context.Context, done func() bool) bool {
	for {
		h.mu.Lock()
		if done() {
			h.mu.Unlock()
			return true
		}
		ch := h.changed
		h.mu.Unlock()
		select {
		case <-ch:
		case <-ctx.Done():
			return false
		case <-h.closed:
			return false
		}
	}
}

func (h *Host) StartPairing(ttl time.Duration) (PairStart, error) {
	return h.startPairing(ttl, false, "")
}

func (h *Host) StartPairingIPhone(ttl time.Duration, friend string) (PairStart, error) {
	return h.startPairing(ttl, true, friend)
}

func (h *Host) startPairing(ttl time.Duration, ios bool, friend string) (PairStart, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if ios && h.web == "" {
		return PairStart{}, errors.New("iPhones need the web app, and it's off: see cloche status")
	}
	if ios && h.webErr != "" {
		return PairStart{}, errors.New("the web app for iPhones isn't up: " + h.webErr)
	}
	if friend != "" {
		if err := checkName(friend); err != nil {
			return PairStart{}, err
		}
	} else if !h.anyOwner && h.owner == "" {
		return PairStart{}, errNoOwner
	}
	code := newCode()
	h.pairing = &pairing{code: code, expires: h.now().Add(ttl), ios: ios, friend: friend}
	h.broadcast()
	d := sha256.Sum256(h.spki)
	p := PairStart{
		Host: h.name, Addr: h.addr, Code: code, Fingerprint: h.Fingerprint(), Expires: h.pairing.expires,
		URL: PairPage + "#" + url.Values{"h": {h.name}, "a": {h.addr}, "c": {code}, "f": {base64.RawURLEncoding.EncodeToString(d[:])}}.Encode(),
		IOS: ios, Friend: friend,
	}
	if ios {
		p.URL = h.web + "/pair#" + code
	}
	return p, nil
}

const PairPage = "https://ahmd-sh.github.io/cloche/pair/"

type PairStart struct {
	Host        string    `json:"host"`
	Addr        string    `json:"addr"`
	Code        string    `json:"code"`
	URL         string    `json:"url"`
	Fingerprint string    `json:"fingerprint"`
	Expires     time.Time `json:"expires"`
	IOS         bool      `json:"ios,omitempty"`
	Friend      string    `json:"friend,omitempty"`
}

func (h *Host) Unpair() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.state.Phone = nil
	h.epoch++
	h.broadcast()
	return h.saveState()
}

func (h *Host) headForNewPhone() int64 {
	since := h.now().Add(-time.Hour)
	for _, r := range h.records {
		if !r.m.PushedAt.Before(since) {
			return r.m.Seq - 1
		}
	}
	return h.state.Seq
}

type Summary struct {
	ID       string       `json:"id"`
	Seq      int64        `json:"seq"`
	PushedAt time.Time    `json:"pushed_at"`
	File     wire.File    `json:"file"`
	APK      *wire.APK    `json:"apk,omitempty"`
	Note     string       `json:"note,omitempty"`
	Source   *wire.Source `json:"source,omitempty"`
	Taken    time.Time    `json:"taken,omitzero"`
	Fetched  time.Time    `json:"fetched,omitzero"`
	Result   *wire.Result `json:"result,omitempty"`
	ResultAt time.Time    `json:"result_at,omitzero"`

	IPA       *ipa.Info `json:"ipa,omitempty"`
	To        string    `json:"to,omitempty"`
	NotifyErr string    `json:"notify_error,omitempty"`
	Opened    time.Time `json:"opened,omitzero"`
	Asked     time.Time `json:"asked,omitzero"`
	Warnings  []string  `json:"warnings,omitempty"`
}

func (h *Host) summary(r *record) Summary {
	s := Summary{
		ID: r.m.ID, Seq: r.m.Seq, PushedAt: r.m.PushedAt, File: r.m.File, APK: r.m.APK, Note: r.m.Note, Source: r.m.Source,
		Taken: r.Taken, Fetched: r.Fetched, Result: r.Result, ResultAt: r.ResultAt,
		IPA: r.IPA, NotifyErr: r.NotifyErr, Opened: r.Opened, Asked: r.Asked,
	}
	if r.To != "" {
		s.To = r.To
		if ip := h.iphone(r.To); ip != nil {
			s.To = ip.Name
		}
	}
	return s
}

type Status struct {
	Host        string         `json:"host"`
	Addr        string         `json:"addr"`
	Fingerprint string         `json:"fingerprint"`
	Phone       *PhoneStatus   `json:"phone,omitempty"`
	IPhones     []IPhoneStatus `json:"iphones,omitempty"`
	Web         string         `json:"web,omitempty"`
	WebErr      string         `json:"web_error,omitempty"`
	Pairing     time.Time      `json:"pairing,omitzero"`
	Epoch       int64          `json:"epoch"`
	Deliveries  []Summary      `json:"deliveries"`
	Version     string         `json:"version,omitempty"`
	Update      string         `json:"update,omitempty"`
}

type PhoneStatus struct {
	Name     string    `json:"name"`
	Device   string    `json:"device"`
	PairedAt time.Time `json:"paired_at"`
	Live     bool      `json:"live"`
	Seen     time.Time `json:"seen,omitzero"`
	App      string    `json:"app,omitempty"`
	AppCode  int64     `json:"app_code,omitempty"`
}

func (h *Host) status() Status {
	s := Status{Host: h.name, Addr: h.addr, Fingerprint: h.Fingerprint(), Deliveries: []Summary{}}
	if p := h.state.Phone; p != nil {
		s.Phone = &PhoneStatus{Name: p.Name, Device: p.Device, PairedAt: p.PairedAt, Live: h.polls > 0, Seen: h.seen, App: p.App, AppCode: p.AppCode}
	}
	if h.pairing != nil && h.now().Before(h.pairing.expires) {
		s.Pairing = h.pairing.expires
	}
	s.Web, s.WebErr, s.Epoch = h.web, h.webErr, h.epoch
	s.Version, s.Update = h.version, h.update
	for _, ip := range h.state.IPhones {
		s.IPhones = append(s.IPhones, h.iphoneStatus(ip))
	}
	for i := len(h.records) - 1; i >= 0 && len(s.Deliveries) < 10; i-- {
		s.Deliveries = append(s.Deliveries, h.summary(h.records[i]))
	}
	return s
}

func (h *Host) loadRecords() error {
	entries, err := os.ReadDir(h.path("deliveries"))
	if err != nil {
		return err
	}
	for _, e := range entries {
		name := e.Name()
		if strings.HasSuffix(name, ".tmp") {
			os.Remove(h.path("deliveries", name))
			continue
		}
		if !strings.HasSuffix(name, ".json") {
			continue
		}
		b, err := os.ReadFile(h.path("deliveries", name))
		if err != nil {
			return err
		}
		r := &record{}
		if err := json.Unmarshal(b, r); err != nil {
			h.log.Printf("skipping %s: %v", name, err)
			continue
		}
		if err := json.Unmarshal(r.Delivery.Manifest, &r.m); err != nil {
			h.log.Printf("skipping %s: %v", name, err)
			continue
		}
		h.records = append(h.records, r)
	}
	slices.SortFunc(h.records, func(a, b *record) int { return int(a.m.Seq - b.m.Seq) })
	return nil
}

func (h *Host) saveRecord(r *record) error {
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return writeFile(h.path("deliveries", r.m.ID+".json"), b)
}

func (h *Host) saveState() error {
	b, err := json.MarshalIndent(h.state, "", "  ")
	if err != nil {
		return err
	}
	return writeFile(h.path("state.json"), b)
}

func writeFile(path string, b []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func loadKey(path string) (*ecdsa.PrivateKey, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			return nil, err
		}
		der, err := x509.MarshalPKCS8PrivateKey(k)
		if err != nil {
			return nil, err
		}
		if err := writeFile(path, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})); err != nil {
			return nil, err
		}
		return k, nil
	}
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(b)
	if block == nil {
		return nil, fmt.Errorf("%s: no PEM", path)
	}
	k, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	ec, ok := k.(*ecdsa.PrivateKey)
	if !ok || ec.Curve != elliptic.P256() {
		return nil, fmt.Errorf("%s: not a P-256 key", path)
	}
	return ec, nil
}

var lower32 = base32.NewEncoding("abcdefghijklmnopqrstuvwxyz234567").WithPadding(base32.NoPadding)

func newID() string {
	b := make([]byte, 5)
	rand.Read(b)
	return lower32.EncodeToString(b)
}

const crockford = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

func newCode() string {
	b := make([]byte, 12)
	rand.Read(b)
	var s strings.Builder
	for i, c := range b {
		if i > 0 && i%4 == 0 {
			s.WriteByte('-')
		}
		s.WriteByte(crockford[c&31])
	}
	return s.String()
}

func normalCode(s string) string {
	var b strings.Builder
	for _, c := range strings.ToUpper(s) {
		switch c {
		case '-', ' ':
		case 'O':
			b.WriteByte('0')
		case 'I', 'L':
			b.WriteByte('1')
		default:
			b.WriteRune(c)
		}
	}
	return b.String()
}

func newSecret() string {
	b := make([]byte, 32)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func hash(s string) string {
	d := sha256.Sum256([]byte(s))
	return hex.EncodeToString(d[:])
}

func clean(s string, n int) string {
	var b strings.Builder
	count := 0
	for _, c := range strings.TrimSpace(s) {
		if count == n {
			break
		}
		if c == '\n' || c == '\t' {
			c = ' '
		}
		if unicode.IsControl(c) || unicode.Is(unicode.Bidi_Control, c) {
			continue
		}
		b.WriteRune(c)
		count++
	}
	return b.String()
}

func cleanSource(s *wire.Source) *wire.Source {
	if s == nil {
		return nil
	}
	return &wire.Source{Dir: clean(s.Dir, 500), Repo: clean(s.Repo, 200), Branch: clean(s.Branch, 200), Commit: clean(s.Commit, 64), Dirty: s.Dirty}
}

func cleanAPK(a *wire.APK) *wire.APK {
	if a == nil {
		return nil
	}
	a.Package, a.VersionName, a.Label, a.Signer = clean(a.Package, 255), clean(a.VersionName, 100), clean(a.Label, 100), clean(a.Signer, 64)
	return a
}

func cleanIPA(i *ipa.Info) {
	i.BundleID, i.Name, i.Version, i.Build, i.MinOS = clean(i.BundleID, 255), clean(i.Name, 100), clean(i.Version, 100), clean(i.Build, 100), clean(i.MinOS, 32)
	for k, p := range i.Platforms {
		i.Platforms[k] = clean(p, 32)
	}
	if p := i.Profile; p != nil {
		p.Name, p.Team, p.TeamID, p.AppID = clean(p.Name, 200), clean(p.Team, 200), clean(p.TeamID, 32), clean(p.AppID, 255)
		for k, d := range p.Devices {
			p.Devices[k] = clean(d, 64)
		}
	}
}

var errNoOwner = errors.New("Tailscale doesn't say whose this computer is (a tagged node has no owner), so only a friend's iPhone can pair: cloche pair -friend name")

func (h *Host) ownedBy(p tailnet.Peer) bool {
	if h.anyOwner {
		return true
	}
	return h.owner != "" && !p.Tagged && p.Login == h.owner
}

func (h *Host) ourName(hostport string) bool {
	host, _, err := net.SplitHostPort(hostport)
	if err != nil {
		host = hostport
	}
	host = strings.ToLower(strings.TrimSuffix(strings.Trim(host, "[]"), "."))
	if ip, err := netip.ParseAddr(host); err == nil {
		if a, err := netip.ParseAddrPort(h.addr); err == nil && ip.Unmap() == a.Addr().Unmap() {
			return true
		}
		return h.anyOwner && ip.IsLoopback()
	}
	if host == "" {
		return false
	}
	names := []string{strings.ToLower(h.name)}
	if h.dnsName != "" {
		d := strings.ToLower(h.dnsName)
		names = append(names, d, strings.SplitN(d, ".", 2)[0])
	}
	if h.anyOwner {
		names = append(names, "localhost")
	}
	return slices.Contains(names, host)
}

func describe(m wire.Manifest) string {
	what := m.File.Name
	if m.APK != nil {
		what = fmt.Sprintf("%s %s (%d)", m.APK.Package, m.APK.VersionName, m.APK.VersionCode)
	}

	return fmt.Sprintf("%s, %.1f MB", what, float64(m.File.Size)/(1<<20))
}

func (h *Host) SetUpdate(v string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.update = v
}

type PhoneApp struct {
	Code    int64
	Offered string
}

func (h *Host) PhoneApp() (PhoneApp, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	p := h.state.Phone
	if p == nil {
		return PhoneApp{}, false
	}
	return PhoneApp{Code: p.AppCode, Offered: p.AppOffered}, true
}

func (h *Host) SetAppOffered(v string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.state.Phone == nil {
		return nil
	}
	h.state.Phone.AppOffered = v
	return h.saveState()
}
