package host

import (
	"context"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ahmd-sh/cloche/internal/wire"
)

func pairFrom(t *testing.T, h *Host, from, friend string) wire.PairResponse {
	t.Helper()
	p, err := h.StartPairing(time.Minute, friend)
	if err != nil {
		t.Fatal(err)
	}
	w := call(t, h.PhoneAPI(), from, "POST", "/v1/pair", "", wire.PairRequest{Code: p.Code, Device: "Pixel"})
	if w.Code != 200 {
		t.Fatalf("pair from %s: %d %s", from, w.Code, w.Body)
	}
	var res wire.PairResponse
	json.Unmarshal(w.Body.Bytes(), &res)
	return res
}

func inbox(t *testing.T, h *Host, from, secret string) []string {
	t.Helper()
	w := call(t, h.PhoneAPI(), from, "GET", "/v1/inbox?after=0&wait=0", secret, nil)
	if w.Code != 200 {
		t.Fatalf("inbox from %s: %d %s", from, w.Code, w.Body)
	}
	var in wire.Inbox
	json.Unmarshal(w.Body.Bytes(), &in)
	var ids []string
	for _, d := range in.Deliveries {
		var m wire.Manifest
		json.Unmarshal(d.Manifest, &m)
		ids = append(ids, m.ID)
	}
	return ids
}

func TestManyPhones(t *testing.T) {
	h := newHost(t)
	api := h.PhoneAPI()
	ctx := context.Background()

	p, _ := h.StartPairing(time.Minute, "")
	if w := call(t, api, strangerIP, "POST", "/v1/pair", "", wire.PairRequest{Code: p.Code}); w.Code != 403 {
		t.Errorf("a stranger paired with my own code: %d", w.Code)
	}
	mine := pairFrom(t, h, phoneIP, "")
	other := pairFrom(t, h, otherIP, "")
	sams := pairFrom(t, h, strangerIP, "sam")
	if len(h.state.Phones) != 3 {
		t.Fatalf("phones: %+v", h.state.Phones)
	}
	var names []string
	for _, p := range h.state.Phones {
		names = append(names, p.Name+":"+p.Friend)
	}
	if strings.Join(names, " ") != "phone: laptop: sam:sam" {
		t.Errorf("names: %v", names)
	}

	own, err := h.Push(ctx, PushRequest{Path: fakeAPK(t, "own")})
	if err != nil {
		t.Fatal(err)
	}
	theirs, err := h.Push(ctx, PushRequest{Path: fakeAPK(t, "theirs"), To: []string{"sam"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(own.Phones) != 2 || own.Phones[0].Phone != "phone" || own.Phones[1].Phone != "laptop" {
		t.Errorf("own push went to %+v", own.Phones)
	}
	if len(theirs.Phones) != 1 || theirs.Phones[0].Phone != "sam" || theirs.Phones[0].Friend != "sam" || theirs.For[0] != "sam" {
		t.Errorf("sam's push went to %+v", theirs.Phones)
	}

	for _, c := range []struct {
		from, secret, want string
	}{
		{phoneIP, mine.Secret, own.ID},
		{otherIP, other.Secret, own.ID},
		{strangerIP, sams.Secret, theirs.ID},
	} {
		if got := inbox(t, h, c.from, c.secret); len(got) != 1 || got[0] != c.want {
			t.Errorf("%s's inbox: %v, want %s", c.from, got, c.want)
		}
	}
	if w := call(t, api, phoneIP, "GET", "/v1/inbox?after=0&wait=0", sams.Secret, nil); w.Code != 401 {
		t.Errorf("my phone with sam's secret: %d", w.Code)
	}
	if w := call(t, api, phoneIP, "GET", "/v1/apk/"+theirs.ID, mine.Secret, nil); w.Code != 404 {
		t.Errorf("my phone fetched sam's build: %d", w.Code)
	}
	if w := call(t, api, strangerIP, "GET", "/v1/apk/"+own.ID, sams.Secret, nil); w.Code != 404 {
		t.Errorf("sam fetched my build: %d", w.Code)
	}
	if w := call(t, api, strangerIP, "POST", "/v1/result/"+own.ID, sams.Secret, wire.Result{Status: wire.Installed}); w.Code != 404 {
		t.Errorf("sam reported on my build: %d", w.Code)
	}
	call(t, api, otherIP, "POST", "/v1/result/"+own.ID, other.Secret, wire.Result{Status: wire.Declined})
	call(t, api, phoneIP, "POST", "/v1/result/"+own.ID, mine.Secret, wire.Result{Status: wire.Installed})
	h.mu.Lock()
	s := h.summary(h.find(own.ID))
	h.mu.Unlock()
	if s.Phones[0].Result.Status != wire.Installed || s.Phones[1].Result.Status != wire.Declined {
		t.Errorf("each phone's result: %+v", s.Phones)
	}

	if _, err := h.Unpair(""); err == nil {
		t.Error("unpaired without a name with three phones paired")
	}
	if gone, err := h.Unpair("sam"); err != nil || len(gone) != 1 || len(h.state.Phones) != 2 {
		t.Errorf("unpair sam: %v %v, %d left", gone, err, len(h.state.Phones))
	}
	if _, err := h.Push(ctx, PushRequest{Path: fakeAPK(t, "x"), To: []string{"sam"}}); err == nil {
		t.Error("pushed to a friend with no phones")
	}
	if w := call(t, api, strangerIP, "GET", "/v1/inbox?after=0&wait=0", sams.Secret, nil); w.Code != 401 {
		t.Errorf("sam polled after unpairing: %d", w.Code)
	}
}

func TestPairingAgainKeepsOneEntry(t *testing.T) {
	h := newHost(t)
	first := pairFrom(t, h, phoneIP, "")
	second := pairFrom(t, h, phoneIP, "")
	if len(h.state.Phones) != 1 || h.state.Phones[0].Name != "phone" {
		t.Errorf("phones after pairing twice: %+v", h.state.Phones)
	}
	if w := call(t, h.PhoneAPI(), phoneIP, "GET", "/v1/inbox?after=0&wait=0", first.Secret, nil); w.Code != 401 {
		t.Errorf("the old secret still works: %d", w.Code)
	}
	if got := inbox(t, h, phoneIP, second.Secret); len(got) != 0 {
		t.Errorf("inbox: %v", got)
	}
}

func TestNames(t *testing.T) {
	h := newHost(t)
	pairFrom(t, h, phoneIP, "")
	if _, err := h.StartPairing(time.Minute, "phone"); err == nil {
		t.Error("a friend took the name of one of my phones")
	}
	if _, err := h.StartPairing(time.Minute, "Sam!"); err == nil {
		t.Error("a friend's name with a capital and a !")
	}
	pairFrom(t, h, strangerIP, "sam")
	if _, err := h.Push(context.Background(), PushRequest{Path: fakeAPK(t, "x"), To: []string{"nobody"}}); err == nil || !strings.Contains(err.Error(), "paired: phone, sam") {
		t.Errorf("push to nobody: %v", err)
	}
	s, err := h.Push(context.Background(), PushRequest{Path: fakeAPK(t, "x"), To: []string{"phone", "sam", "SAM"}})
	if err != nil || len(s.Phones) != 2 {
		t.Errorf("push to a phone and a friend: %+v %v", s.Phones, err)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if f := h.friends(); len(f) != 1 || f[0].Name != "sam" || len(f[0].Phones) != 1 || f[0].Last == nil || f[0].Last.ID != s.ID {
		t.Errorf("friends: %+v", f)
	}
}

func TestDisplayName(t *testing.T) {
	h := newHost(t)
	if err := h.SetDisplay("  Sam's\x1b[2J builds "); err != nil {
		t.Fatal(err)
	}
	p, _ := h.StartPairing(time.Minute, "")
	frag, _ := strings.CutPrefix(p.URL, PairPage+"#")
	if q, _ := url.ParseQuery(frag); q.Get("n") != "Sam's[2J builds" || q.Get("h") != "mac" || q.Has("friend") {
		t.Errorf("pairing link %s", p.URL)
	}
	res := pairPhone(t, h)
	if res.Name != "Sam's[2J builds" || res.Host != "mac" {
		t.Errorf("pair response: %+v", res)
	}
	w := call(t, h.PhoneAPI(), phoneIP, "GET", "/v1/inbox?after=0&wait=0", res.Secret, nil)
	var in wire.Inbox
	json.Unmarshal(w.Body.Bytes(), &in)
	if in.Name != "Sam's[2J builds" {
		t.Errorf("inbox: %s", w.Body)
	}
	f, _ := h.StartPairing(time.Minute, "alex")
	frag, _ = strings.CutPrefix(f.URL, PairPage+"#")
	if q, _ := url.ParseQuery(frag); q.Get("friend") != "1" {
		t.Errorf("friend's link %s", f.URL)
	}
}

func TestUpgradeFrom03(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "deliveries"), 0o700)
	state := `{
  "seq": 2,
  "phone": {"node": "nPhone", "name": "Pixel 10 Pro", "device": "Pixel", "secret_sha256": "` + hash("s3cret") + `", "paired_at": "2026-11-01T10:00:00Z", "app": "0.3.0", "app_code": 3000},
  "iphones": [{"node": "nFriend", "name": "marwan", "machine": "iphone", "login": "m@icloud", "friend": true, "token": "t", "paired_at": "2026-11-01T10:00:00Z"}]
}`
	os.WriteFile(filepath.Join(dir, "state.json"), []byte(state), 0o600)
	h0 := newHost(t)
	a, _ := h0.Push(context.Background(), PushRequest{Path: fakeAPK(t, "a")})
	b, _ := json.Marshal(map[string]any{
		"delivery": h0.find(a.ID).Delivery, "taken": "2026-11-01T11:00:00Z",
		"result": map[string]string{"status": "installed"}, "result_at": "2026-11-01T11:01:00Z",
	})
	os.WriteFile(filepath.Join(dir, "deliveries", a.ID+".json"), b, 0o600)
	c, _ := h0.Push(context.Background(), PushRequest{Path: fakeAPK(t, "c")})
	ib, _ := json.Marshal(map[string]any{
		"delivery": h0.find(c.ID).Delivery, "to": "nFriend", "ipa": map[string]string{"bundle_id": "com.example"},
		"opened": "2026-11-01T11:00:00Z", "cleared": true,
	})
	os.WriteFile(filepath.Join(dir, "deliveries", c.ID+".json"), ib, 0o600)

	h, err := Open(Config{Dir: dir, Name: "mac", Addr: "100.64.0.1:41778", Owner: "me@github", Peers: h0.peers})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(h.Close)
	if len(h.state.Phones) != 1 || h.state.Phone03 != nil {
		t.Fatalf("phones: %+v", h.state.Phones)
	}
	p := h.state.Phones[0]
	if p.Name != "pixel-10-pro" || p.Machine != "Pixel 10 Pro" || p.Friend != "" || p.AppCode != 3000 {
		t.Errorf("the 0.3 phone: %+v", p)
	}
	if ip := h.state.IPhones[0]; ip.FriendName != "marwan" {
		t.Errorf("the 0.3 friend: %+v", ip)
	}
	if got := inbox(t, h, phoneIP, "s3cret"); len(got) != 1 || got[0] != a.ID {
		t.Errorf("the 0.3 phone's inbox: %v", got)
	}
	r := h.find(a.ID)
	if g := r.of("nPhone"); g.Result == nil || g.Result.Status != wire.Installed || g.Taken.IsZero() {
		t.Errorf("the 0.3 record's progress: %+v", r.Got)
	}
	if ir := h.find(c.ID); ir == nil || !ir.ios() || len(ir.Phones) != 1 || ir.Phones[0] != "nFriend" || !ir.of("nFriend").Cleared || ir.of("nFriend").Opened.IsZero() {
		t.Errorf("the 0.3 iPhone record: %+v", h.find(c.ID))
	}

	h.mu.Lock()
	h.saveState()
	h.mu.Unlock()
	saved, _ := os.ReadFile(filepath.Join(dir, "state.json"))
	if strings.Contains(string(saved), `"phone":`) || !strings.Contains(string(saved), `"phones":`) || !strings.Contains(string(saved), `"friend": true`) {
		t.Errorf("saved: %s", saved)
	}
}
