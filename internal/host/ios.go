package host

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/ahmd-sh/cloche/internal/ipa"
	"github.com/ahmd-sh/cloche/internal/webpush"
	"github.com/ahmd-sh/cloche/internal/wire"
)

type iphone struct {
	Node     string                `json:"node"`
	Name     string                `json:"name"`
	Machine  string                `json:"machine"`
	Login    string                `json:"login"`
	Friend   bool                  `json:"friend,omitempty"`
	Token    string                `json:"token"`
	UDID     string                `json:"udid,omitempty"`
	Model    string                `json:"model,omitempty"`
	OS       string                `json:"os,omitempty"`
	Push     *webpush.Subscription `json:"push,omitempty"`
	PairedAt time.Time             `json:"paired_at"`
}

type IPhoneStatus struct {
	Name     string    `json:"name"`
	Machine  string    `json:"machine"`
	Login    string    `json:"login"`
	Friend   bool      `json:"friend,omitempty"`
	UDID     string    `json:"udid,omitempty"`
	Model    string    `json:"model,omitempty"`
	OS       string    `json:"os,omitempty"`
	Notify   bool      `json:"notify"`
	PairedAt time.Time `json:"paired_at"`
	Seen     time.Time `json:"seen,omitzero"`
}

func (h *Host) iphoneStatus(ip *iphone) IPhoneStatus {
	return IPhoneStatus{
		Name: ip.Name, Machine: ip.Machine, Login: ip.Login, Friend: ip.Friend, UDID: ip.UDID, Model: ip.Model, OS: ip.OS,
		Notify: ip.Push != nil, PairedAt: ip.PairedAt, Seen: h.seenAt[ip.Node],
	}
}

func (h *Host) iphone(node string) *iphone {
	for _, ip := range h.state.IPhones {
		if ip.Node == node {
			return ip
		}
	}
	return nil
}

func (h *Host) iphoneNamed(name string) *iphone {
	for _, ip := range h.state.IPhones {
		if strings.EqualFold(ip.Name, name) {
			return ip
		}
	}
	var found *iphone
	for _, ip := range h.state.IPhones {
		if strings.EqualFold(ip.Machine, name) {
			if found != nil || ip.Friend {
				return nil
			}
			found = ip
		}
	}
	return found
}

var nameRE = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,31}$`)

func checkName(name string) error {
	if !nameRE.MatchString(name) {
		return fmt.Errorf("%q: a name is up to 32 lowercase letters, digits and dashes", name)
	}
	return nil
}

func (h *Host) SetWebErr(msg string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.webErr = msg
}

func (h *Host) UnpairIPhone(name string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	ip := h.iphoneNamed(name)
	if ip == nil {
		return fmt.Errorf("no iPhone called %s is paired", name)
	}
	return h.forgetIPhone(ip)
}

func (h *Host) forgetIPhone(ip *iphone) error {
	h.state.IPhones = slices.DeleteFunc(h.state.IPhones, func(x *iphone) bool { return x == ip })
	h.epoch++
	h.broadcast()
	h.log.Printf("unpaired iPhone %s (%s)", ip.Name, ip.Node)
	return h.saveState()
}

func (h *Host) target(name string) (*iphone, error) {
	if name != "" {
		if ip := h.iphoneNamed(name); ip != nil {
			return ip, nil
		}
		return nil, fmt.Errorf("no iPhone called %s is paired%s", name, h.iphoneNames())
	}
	var mine []*iphone
	for _, ip := range h.state.IPhones {
		if !ip.Friend {
			mine = append(mine, ip)
		}
	}
	switch {
	case len(mine) == 1:
		return mine[0], nil
	case len(mine) > 1:
		return nil, fmt.Errorf("more than one iPhone of yours is paired: say which with -to%s", h.iphoneNames())
	case len(h.state.IPhones) > 0:
		return nil, fmt.Errorf("no iPhone of yours is paired: to send it to a friend's, name it with -to%s", h.iphoneNames())
	}
	return nil, errors.New("no iPhone is paired: run cloche pair -ios")
}

func (h *Host) iphoneNames() string {
	if len(h.state.IPhones) == 0 {
		return ""
	}
	var names []string
	for _, ip := range h.state.IPhones {
		names = append(names, ip.Name)
	}
	return " (paired: " + strings.Join(names, ", ") + ")"
}

func (h *Host) pushIPA(req PushRequest, isApp bool) (Summary, error) {
	h.mu.Lock()
	if h.web == "" {
		h.mu.Unlock()
		return Summary{}, errors.New("iPhones need the web app, and it's off: see cloche status")
	}
	if h.webErr != "" {
		h.mu.Unlock()
		return Summary{}, errors.New("the web app for iPhones isn't up: " + h.webErr)
	}
	ip, err := h.target(req.To)
	var (
		node, name, udid string
	)
	if ip != nil {
		node, name, udid = ip.Node, ip.Name, ip.UDID
	}
	h.mu.Unlock()
	if err != nil {
		return Summary{}, err
	}

	id := newID()
	file := h.path("deliveries", id+".ipa")
	var sum string
	var size int64
	if isApp {
		sum, size, err = h.packIn(req.Path, id)
	} else {
		sum, size, err = h.copyInAs(req.Path, id+".ipa")
	}
	if err != nil {
		return Summary{}, err
	}
	keep := false
	defer func() {
		if !keep {
			os.Remove(file)
			os.Remove(h.path("deliveries", id+".png"))
		}
	}()

	info, err := ipa.Read(file)
	if err != nil {
		return Summary{}, err
	}
	cleanIPA(info)
	warnings, err := check(info, name, udid, h.now())
	if err != nil {
		return Summary{}, err
	}
	if len(info.Icon) > 0 {
		writeFile(h.path("deliveries", id+".png"), info.Icon)
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	if h.iphone(node) == nil {
		return Summary{}, fmt.Errorf("%s was unpaired while the build was read", name)
	}
	fname := clean(filepath.Base(req.Path), 255)
	if isApp {
		fname = strings.TrimSuffix(fname, ".app") + ".ipa"
	}
	m := wire.Manifest{
		V: wire.Version, ID: id, Seq: h.state.Seq + 1, Host: h.name, PushedAt: h.now().UTC().Truncate(time.Second),
		File:   wire.File{Name: fname, Size: size, SHA256: sum},
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
	r := &record{Delivery: wire.Delivery{Manifest: b, Sig: sig}, m: m, To: node, IPA: info}
	if err := h.saveRecord(r); err != nil {
		return Summary{}, err
	}
	h.state.Seq = m.Seq
	if err := h.saveState(); err != nil {
		h.state.Seq--
		os.Remove(h.path("deliveries", id+".json"))
		return Summary{}, err
	}
	keep = true
	h.records = append(h.records, r)
	h.prune()
	h.broadcast()
	h.log.Printf("pushed %s: %s %s (%s) for %s, %.1f MB", id, info.BundleID, info.Version, info.Build, name, float64(size)/(1<<20))
	go h.notify(id)
	s := h.summary(r)
	s.Warnings = warnings
	return s, nil
}

func check(info *ipa.Info, name, udid string, now time.Time) (warnings []string, err error) {
	if slices.Contains(info.Platforms, "iPhoneSimulator") {
		return nil, errors.New("it's built for the Simulator: build it for a device (generic/platform=iOS)")
	}
	p := info.Profile
	switch {
	case p == nil:
		return []string{"it's unsigned: " + name + " gets it as a download, to install with SideStore or AltStore"}, nil
	case p.Kind == ipa.AppStore:
		return nil, errors.New("it's signed for the App Store, and iOS installs those only from the App Store or TestFlight: export it for ad hoc (release-testing) or development")
	case !p.Expires.IsZero() && now.After(p.Expires):
		return nil, fmt.Errorf("its provisioning profile %q expired on %s", p.Name, p.Expires.Local().Format("Jan 2, 2006"))
	case udid != "" && !p.Covers(udid):
		return nil, fmt.Errorf("%s isn't in its provisioning profile %q: register UDID %s in your Apple developer account, then build it again", name, p.Name, udid)
	}
	if udid == "" && !p.AllDevices {
		warnings = append(warnings, name+" hasn't sent its UDID, so cloche can't check the build is signed for it")
	}
	if p.Kind == ipa.Development {
		warnings = append(warnings, "a development build: "+name+" needs Developer Mode on (Settings, Privacy & Security)")
	}
	if !p.Expires.IsZero() && p.Expires.Sub(now) < 7*24*time.Hour {
		warnings = append(warnings, fmt.Sprintf("its profile expires %s: the app stops opening then", p.Expires.Local().Format("Jan 2 15:04")))
	}
	return warnings, nil
}

func (h *Host) packIn(app, id string) (sum string, size int64, err error) {
	tmp, err := os.CreateTemp(h.path("deliveries"), id+"-*.tmp")
	if err != nil {
		return "", 0, err
	}
	err = ipa.Pack(app, tmp)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	defer os.Remove(tmp.Name())
	if err != nil {
		return "", 0, err
	}
	return h.copyInAs(tmp.Name(), id+".ipa")
}

func (h *Host) notify(id string) {
	h.mu.Lock()
	r := h.find(id)
	if r == nil {
		h.mu.Unlock()
		return
	}
	ip := h.iphone(r.To)
	if ip == nil || ip.Push == nil {
		r.NotifyErr = "its notifications aren't on: the build is waiting in Cloche on it"
		h.saveRecord(r)
		h.broadcast()
		h.mu.Unlock()
		return
	}
	sub := *ip.Push
	title := r.IPA.Name
	if v := version(r.IPA); v != "" {
		title += " " + v
	}
	body := r.m.Note
	if body == "" {
		body = "Ready to install"
	}
	msg, _ := json.Marshal(map[string]string{"title": title, "body": body, "url": "/b/" + id, "tag": r.IPA.BundleID})
	h.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	err := h.pusher.Send(ctx, sub, webpush.Message{Payload: msg, TTL: 7 * 24 * time.Hour, Urgency: "high"})

	h.mu.Lock()
	defer h.mu.Unlock()
	if h.find(id) != r {
		return
	}
	switch {
	case errors.Is(err, webpush.ErrGone):
		if ip := h.iphone(r.To); ip != nil && ip.Push != nil && ip.Push.Endpoint == sub.Endpoint {
			ip.Push = nil
			h.saveState()
		}
		r.NotifyErr = "its notifications were turned off: the build is waiting in Cloche on it"
	case err != nil:
		r.NotifyErr = "couldn't notify it: " + err.Error()
	default:
		r.Taken, r.NotifyErr = h.now().UTC(), ""
	}
	if err != nil {
		h.log.Printf("%s: notify: %v", id, err)
	}
	h.saveRecord(r)
	h.broadcast()
}

func version(i *ipa.Info) string {
	switch {
	case i.Version != "" && i.Build != "" && i.Build != i.Version:
		return i.Version + " (" + i.Build + ")"
	case i.Version != "":
		return i.Version
	}
	return i.Build
}
