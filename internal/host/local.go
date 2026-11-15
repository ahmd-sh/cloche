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
	mux.HandleFunc("POST /display", h.localDisplay)
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
	q := r.URL.Query()
	since, _ := strconv.ParseInt(q.Get("since"), 10, 64)
	ctx, cancel := waitFor(r)
	defer cancel()
	h.wait(ctx, func() bool {
		rec := h.find(id)
		if rec == nil {
			return true
		}
		switch q.Get("until") {
		case "taken":
			return h.taken(rec)
		case "change":
			return rec.rev != since
		}
		return true
	})
	h.mu.Lock()
	defer h.mu.Unlock()
	rec := h.find(id)
	if rec == nil {
		fail(w, http.StatusNotFound, "no such delivery")
		return
	}
	reply(w, h.summary(rec))
}

func (h *Host) taken(r *record) bool {
	for _, p := range h.summary(r).Phones {
		if p.Taken.IsZero() && p.NotifyErr == "" {
			return false
		}
	}
	return true
}

func (h *Host) localPair(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	friend := q.Get("friend")
	ttl := 15 * time.Minute
	if s, err := strconv.Atoi(q.Get("ttl")); err == nil && s > 0 {
		ttl = min(time.Duration(s)*time.Second, 7*24*time.Hour)
	}
	var p PairStart
	var err error
	if q.Get("ios") == "" {
		if friend == "" {
			ttl = min(ttl, 24*time.Hour)
		}
		p, err = h.StartPairing(ttl, friend)
	} else {
		p, err = h.StartPairingIPhone(ttl, friend)
	}
	if err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	reply(w, p)
}

func (h *Host) localUnpair(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	gone, err := h.Unpair(name)
	if err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	reply(w, map[string][]string{"unpaired": gone})
}

func (h *Host) localDisplay(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&req); err != nil {
		fail(w, http.StatusBadRequest, "bad request")
		return
	}
	if err := h.SetDisplay(req.Name); err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Host) localStatus(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("until") == "epoch" {
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
