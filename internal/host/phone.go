package host

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"net/netip"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/ahmd-sh/cloche/internal/tailnet"
	"github.com/ahmd-sh/cloche/internal/wire"
)

const MaxWait = 300 * time.Second

func (h *Host) PhoneAPI() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/pair", h.pair)
	mux.HandleFunc("GET /v1/inbox", h.authed(h.inbox))
	mux.HandleFunc("GET /v1/apk/{id}", h.authed(h.apk))
	mux.HandleFunc("POST /v1/result/{id}", h.authed(h.result))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !h.ourName(r.Host) {
			fail(w, http.StatusMisdirectedRequest, "this is "+h.addr)
			return
		}
		mux.ServeHTTP(w, r)
	})
}

func (h *Host) peer(r *http.Request) (tailnet.Peer, error) {
	ap, err := netip.ParseAddrPort(r.RemoteAddr)
	if err != nil {
		return tailnet.Peer{}, err
	}
	return h.peers.Whois(r.Context(), ap.Addr())
}

func (h *Host) pair(w http.ResponseWriter, r *http.Request) {
	p, err := h.peer(r)
	if err != nil {
		h.log.Printf("pair from %s: %v", r.RemoteAddr, err)
		fail(w, http.StatusServiceUnavailable, "can't ask Tailscale who this is")
		return
	}
	var req wire.PairRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&req); err != nil {
		fail(w, http.StatusBadRequest, "bad request")
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	pr := h.pairing
	if pr == nil || !h.now().Before(pr.expires) {
		fail(w, http.StatusForbidden, "no pairing open: run cloche pair on the computer")
		return
	}
	if pr.ios {
		fail(w, http.StatusForbidden, "this pairing is for an iPhone: run cloche pair -android on the computer")
		return
	}
	if pr.friend == "" && !h.ownedBy(p) {
		if h.owner == "" {
			h.log.Printf("pair from %s refused: %v", p.Name, errNoOwner)
			fail(w, http.StatusForbidden, "this computer's owner is unknown to Tailscale: it pairs only friends' phones")
			return
		}
		h.log.Printf("pair refused: %s is %s's, not %s's", p.Name, p.Login, h.owner)
		fail(w, http.StatusForbidden, "not one of "+h.owner+"'s devices: a friend's phone needs a friend's code")
		return
	}
	if subtle.ConstantTimeCompare([]byte(normalCode(req.Code)), []byte(normalCode(pr.code))) != 1 {
		pr.attempts++
		h.log.Printf("pair from %s: wrong code (%d)", p.Name, pr.attempts)
		if pr.attempts >= 5 {
			h.pairing = nil
			h.broadcast()
			fail(w, http.StatusForbidden, "wrong code, too many times: run cloche pair again")
			return
		}
		fail(w, http.StatusForbidden, "wrong code")
		return
	}
	secret := newSecret()
	ph := &phone{
		Node: p.Node, Machine: p.Name, Login: p.Login, Friend: pr.friend,
		Device: clean(req.Device, 64), Secret: hash(secret), PairedAt: h.now().UTC(),
	}
	if i := slices.IndexFunc(h.state.Phones, func(x *phone) bool { return x.Node == p.Node }); i >= 0 {
		h.state.Phones[i] = ph
	} else {
		h.state.Phones = append(h.state.Phones, ph)
	}
	ph.Name = h.uniqueName(ph, phoneName(pr.friend, "android", p.Name))
	h.pairing = nil
	h.epoch++
	if err := h.saveState(); err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.broadcast()
	h.log.Printf("paired with %s (%s, %s, %s)", ph.Name, p.Name, p.Node, p.Login)
	reply(w, wire.PairResponse{Host: h.name, Name: h.state.Display, Addr: h.addr, Key: h.spki, Secret: secret, Head: h.headForNewPhone()})
}

type phoneKey struct{}

func phoneOf(r *http.Request) *phone {
	p, _ := r.Context().Value(phoneKey{}).(*phone)
	return p
}

func (h *Host) authed(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, err := h.peer(r)
		if err != nil {
			h.log.Printf("%s %s from %s: %v", r.Method, r.URL.Path, r.RemoteAddr, err)
			fail(w, http.StatusServiceUnavailable, "can't ask Tailscale who this is")
			return
		}
		h.mu.Lock()
		ph := h.phone(p.Node)
		want := ""
		if ph != nil {
			want = ph.Secret
		}
		h.mu.Unlock()
		token, _ := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		switch {
		case ph == nil:
			h.log.Printf("refused %s %s from %s (%s): not a paired phone", r.Method, r.URL.Path, p.Name, p.Node)
			fail(w, http.StatusUnauthorized, "this phone isn't paired with this computer: pair again")
		case subtle.ConstantTimeCompare([]byte(hash(token)), []byte(want)) != 1:
			h.log.Printf("refused %s %s from %s: wrong secret", r.Method, r.URL.Path, p.Name)
			fail(w, http.StatusUnauthorized, "unknown secret: pair again")
		default:
			next(w, r.WithContext(context.WithValue(r.Context(), phoneKey{}, ph)))
		}
	}
}

func (h *Host) inbox(w http.ResponseWriter, r *http.Request) {
	after, _ := strconv.ParseInt(r.URL.Query().Get("after"), 10, 64)
	wait, _ := strconv.Atoi(r.URL.Query().Get("wait"))
	d := min(max(time.Duration(wait)*time.Second, 0), MaxWait)

	paired := phoneOf(r)
	h.mu.Lock()
	h.polls[paired.Node]++
	h.seenAt[paired.Node] = h.now()
	if name, code, ok := appVersion(r.UserAgent()); ok && (paired.App != name || paired.AppCode != code) {
		paired.App, paired.AppCode = name, code
		h.saveState()
	}
	h.broadcast()
	h.mu.Unlock()
	defer func() {
		h.mu.Lock()
		if h.polls[paired.Node]--; h.polls[paired.Node] <= 0 {
			delete(h.polls, paired.Node)
		}
		h.seenAt[paired.Node] = h.now()
		h.broadcast()
		h.mu.Unlock()
	}()

	waiting := func() bool {
		if after > h.state.Seq || h.phone(paired.Node) != paired {
			return true
		}
		for i := len(h.records) - 1; i >= 0 && h.records[i].m.Seq > after; i-- {
			if h.records[i].forPhone(paired) {
				return true
			}
		}
		return false
	}
	ctx, cancel := context.WithTimeout(r.Context(), d)
	defer cancel()
	h.wait(ctx, waiting)

	h.mu.Lock()
	if h.phone(paired.Node) != paired {
		h.mu.Unlock()
		fail(w, http.StatusUnauthorized, "unpaired on the computer: pair again")
		return
	}
	in := wire.Inbox{Head: h.state.Seq, Name: h.state.Display, Deliveries: []wire.Delivery{}}
	if after <= h.state.Seq {
		for _, rec := range h.records {
			if rec.m.Seq <= after || !rec.forPhone(paired) {
				continue
			}
			in.Deliveries = append(in.Deliveries, rec.Delivery)
			if g := rec.at(paired.Node); g.Taken.IsZero() {
				g.Taken = h.now().UTC()
				h.touch(rec)
			}
		}
	}
	h.mu.Unlock()
	reply(w, in)
}

var userAgent = regexp.MustCompile(`^Cloche/([0-9A-Za-z.+-]{1,40}) \(([0-9]{1,10})\)$`)

func appVersion(ua string) (string, int64, bool) {
	m := userAgent.FindStringSubmatch(ua)
	if m == nil {
		return "", 0, false
	}
	code, err := strconv.ParseInt(m[2], 10, 64)
	if err != nil {
		return "", 0, false
	}
	return m[1], code, true
}

func (h *Host) apk(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ph := phoneOf(r)
	h.mu.Lock()
	rec := h.find(id)
	if rec != nil && !rec.forPhone(ph) {
		rec = nil
	}
	if rec != nil {
		if g := rec.at(ph.Node); g.Fetched.IsZero() {
			g.Fetched = h.now().UTC()
			h.touch(rec)
		}
	}
	h.mu.Unlock()
	if rec == nil {
		fail(w, http.StatusNotFound, "no such delivery: it may have been pruned")
		return
	}
	f, err := os.Open(h.path("deliveries", id+".apk"))
	if err != nil {
		fail(w, http.StatusNotFound, "its file is gone")
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/vnd.android.package-archive")
	http.ServeContent(w, r, "", st.ModTime(), f)
}

func (h *Host) result(w http.ResponseWriter, r *http.Request) {
	var res wire.Result
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&res); err != nil {
		fail(w, http.StatusBadRequest, "bad request")
		return
	}
	switch res.Status {
	case wire.Ready, wire.Blocked, wire.Installing, wire.Installed, wire.Failed, wire.Declined, wire.Superseded:
	default:
		fail(w, http.StatusBadRequest, "unknown status")
		return
	}
	res.Detail = clean(res.Detail, 500)
	res.Package, res.VersionName, res.Signer = clean(res.Package, 255), clean(res.VersionName, 100), clean(res.Signer, 200)
	ph := phoneOf(r)
	h.mu.Lock()
	defer h.mu.Unlock()
	rec := h.find(r.PathValue("id"))
	if rec == nil || !rec.forPhone(ph) {
		fail(w, http.StatusNotFound, "no such delivery")
		return
	}
	g := rec.at(ph.Node)
	g.Result, g.ResultAt = &res, h.now().UTC()
	if err := h.touch(rec); err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.log.Printf("%s: %s on %s %s", rec.m.ID, res.Status, ph.Name, res.Detail)
	w.WriteHeader(http.StatusNoContent)
}

func reply(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func fail(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
