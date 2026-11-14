package host

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"
)

func (h *Host) LocalAPI() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /push", h.localPush)
	mux.HandleFunc("GET /deliveries/{id}", h.localDelivery)
	mux.HandleFunc("POST /pair", h.localPair)
	mux.HandleFunc("POST /unpair", h.localUnpair)
	mux.HandleFunc("GET /status", h.localStatus)
	return mux
}

func (h *Host) localPush(w http.ResponseWriter, r *http.Request) {
	var req PushRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	s, err := h.Push(r.Context(), req)
	if err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	reply(w, s)
}

func (h *Host) localDelivery(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	until := r.URL.Query().Get("until")
	ctx, cancel := waitFor(r)
	defer cancel()
	found := true
	h.wait(ctx, func() bool {
		rec := h.find(id)
		if rec == nil {
			found = false
			return true
		}
		switch until {
		case "taken":
			return !rec.Taken.IsZero() || rec.NotifyErr != ""
		case "fetched":
			return !rec.Fetched.IsZero() || rec.Result != nil
		case "final":
			return rec.Result != nil && rec.Result.Final()
		case "change":
			from := r.URL.Query().Get("from")
			return rec.Result != nil && (rec.Result.Status != from || rec.Result.Final())
		}
		return true
	})
	h.mu.Lock()
	defer h.mu.Unlock()
	rec := h.find(id)
	if !found || rec == nil {
		fail(w, http.StatusNotFound, "no such delivery")
		return
	}
	reply(w, h.summary(rec))
}

func (h *Host) localPair(w http.ResponseWriter, r *http.Request) {
	ttl := 15 * time.Minute
	if s, err := strconv.Atoi(r.URL.Query().Get("ttl")); err == nil && s > 0 {
		ttl = min(time.Duration(s)*time.Second, 7*24*time.Hour)
	}
	q := r.URL.Query()
	if q.Get("ios") == "" {
		p, err := h.StartPairing(min(ttl, 24*time.Hour))
		if err != nil {
			fail(w, http.StatusBadRequest, err.Error())
			return
		}
		reply(w, p)
		return
	}
	p, err := h.StartPairingIPhone(ttl, q.Get("friend"))
	if err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	reply(w, p)
}

func (h *Host) localUnpair(w http.ResponseWriter, r *http.Request) {
	var err error
	if name := r.URL.Query().Get("iphone"); name != "" {
		err = h.UnpairIPhone(name)
	} else {
		err = h.Unpair()
	}
	if err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Host) localStatus(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Query().Get("until") {
	case "paired":
		ctx, cancel := waitFor(r)
		defer cancel()
		h.mu.Lock()
		before := h.state.Phone
		h.mu.Unlock()
		h.wait(ctx, func() bool {
			return h.state.Phone != before || h.pairing == nil || !h.now().Before(h.pairing.expires)
		})
	case "epoch":
		since, _ := strconv.ParseInt(r.URL.Query().Get("since"), 10, 64)
		ctx, cancel := waitFor(r)
		defer cancel()
		h.wait(ctx, func() bool { return h.epoch != since })
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	reply(w, h.status())
}

func waitFor(r *http.Request) (context.Context, context.CancelFunc) {
	s, _ := strconv.Atoi(r.URL.Query().Get("wait"))
	return context.WithTimeout(r.Context(), time.Duration(max(s, 0))*time.Second)
}
