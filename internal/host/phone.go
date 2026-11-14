package host

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"net/netip"
	"os"
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
	if !h.ownedBy(p) {
		if h.owner == "" {
			h.log.Printf("pair from %s refused: %v", p.Name, errNoOwner)
			fail(w, http.StatusForbidden, "this computer's owner is unknown to Tailscale: it can't pair a phone")
			return
		}
		h.log.Printf("pair refused: %s is %s's, not %s's", p.Name, p.Login, h.owner)
		fail(w, http.StatusForbidden, "not one of "+h.owner+"'s devices")
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
		fail(w, http.StatusForbidden, "this pairing is for an iPhone: run cloche pair on the computer")
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
	h.state.Phone = &phone{Node: p.Node, Name: p.Name, Device: clean(req.Device, 64), Secret: hash(secret), PairedAt: h.now().UTC()}
	h.pairing = nil
	h.epoch++
	if err := h.saveState(); err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.broadcast()
	h.log.Printf("paired with %s (%s)", p.Name, p.Node)
	reply(w, wire.PairResponse{Host: h.name, Addr: h.addr, Key: h.spki, Secret: secret, Head: h.headForNewPhone()})
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
		ph := h.state.Phone
		h.mu.Unlock()
		token, _ := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		switch {
		case ph == nil:
			fail(w, http.StatusUnauthorized, "no phone is paired: pair again")
		case p.Node != ph.Node:
			h.log.Printf("refused %s %s from %s (%s): not the paired phone", r.Method, r.URL.Path, p.Name, p.Node)
			fail(w, http.StatusUnauthorized, "this isn't the paired phone: pair again")
		case subtle.ConstantTimeCompare([]byte(hash(token)), []byte(ph.Secret)) != 1:
			h.log.Printf("refused %s %s from %s: wrong secret", r.Method, r.URL.Path, p.Name)
			fail(w, http.StatusUnauthorized, "unknown secret: pair again")
		default:
			next(w, r)
		}
	}
}

func (h *Host) inbox(w http.ResponseWriter, r *http.Request) {
	after, _ := strconv.ParseInt(r.URL.Query().Get("after"), 10, 64)
	wait, _ := strconv.Atoi(r.URL.Query().Get("wait"))
	d := min(max(time.Duration(wait)*time.Second, 0), MaxWait)

	h.mu.Lock()
	h.polls++
	h.seen = h.now()
	paired := h.state.Phone
	h.broadcast()
	h.mu.Unlock()
	defer func() {
		h.mu.Lock()
		h.polls--
		h.seen = h.now()
		h.mu.Unlock()
	}()

	ctx, cancel := context.WithTimeout(r.Context(), d)
	defer cancel()
	h.wait(ctx, func() bool {
		return after > h.state.Seq || len(h.records) > 0 && h.records[len(h.records)-1].m.Seq > after || h.state.Phone != paired
	})

	h.mu.Lock()
	if h.state.Phone != paired {
		h.mu.Unlock()
		fail(w, http.StatusUnauthorized, "unpaired on the computer: pair again")
		return
	}
	in := wire.Inbox{Head: h.state.Seq, Deliveries: []wire.Delivery{}}
	if after <= h.state.Seq {
		for _, rec := range h.records {
			if rec.m.Seq <= after || rec.To != "" {
				continue
			}
			in.Deliveries = append(in.Deliveries, rec.Delivery)
			if rec.Taken.IsZero() {
				rec.Taken = h.now().UTC()
				h.saveRecord(rec)
			}
		}
	}
	if len(in.Deliveries) > 0 {
		h.broadcast()
	}
	h.mu.Unlock()
	reply(w, in)
}

func (h *Host) apk(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	h.mu.Lock()
	rec := h.find(id)
	if rec != nil && rec.To != "" {
		rec = nil
	}
	if rec != nil && rec.Fetched.IsZero() {
		rec.Fetched = h.now().UTC()
		h.saveRecord(rec)
		h.broadcast()
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
	h.mu.Lock()
	defer h.mu.Unlock()
	rec := h.find(r.PathValue("id"))
	if rec == nil || rec.To != "" {
		fail(w, http.StatusNotFound, "no such delivery")
		return
	}
	rec.Result, rec.ResultAt = &res, h.now().UTC()
	if err := h.saveRecord(rec); err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.broadcast()
	h.log.Printf("%s: %s %s", rec.m.ID, res.Status, res.Detail)
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
