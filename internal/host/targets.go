package host

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

type audience struct {
	For    []string
	Phones []string
}

func (a audience) mine() bool { return len(a.For) == 0 && len(a.Phones) == 0 }

func (h *Host) phone(node string) *phone {
	for _, p := range h.state.Phones {
		if p.Node == node {
			return p
		}
	}
	return nil
}

func (h *Host) ownOnly(a audience) bool {
	if len(a.For) > 0 {
		return false
	}
	for _, node := range a.Phones {
		if p := h.phone(node); p == nil || p.Friend != "" {
			return false
		}
	}
	return true
}

func phoneName(friend, kind, machine string) string {
	if friend != "" {
		return friend + "-" + kind
	}
	return nameFrom(machine, kind)
}

func (h *Host) label() string {
	if h.state.Display != "" {
		return h.state.Display
	}
	return h.name
}

func kind(ios bool) string {
	if ios {
		return "iPhone"
	}
	return "Android phone"
}

func kindFlag(ios bool) string {
	if ios {
		return "-ios"
	}
	return "-android"
}

func (h *Host) friendNamed(name string) string {
	for _, p := range h.state.Phones {
		if p.Friend != "" && strings.EqualFold(p.Friend, name) {
			return p.Friend
		}
	}
	for _, ip := range h.state.IPhones {
		if ip.FriendName != "" && strings.EqualFold(ip.FriendName, name) {
			return ip.FriendName
		}
	}
	return ""
}

func (h *Host) friendHas(friend string, ios bool) bool {
	if ios {
		return slices.ContainsFunc(h.state.IPhones, func(ip *iphone) bool { return ip.FriendName == friend })
	}
	return slices.ContainsFunc(h.state.Phones, func(p *phone) bool { return p.Friend == friend })
}

func (h *Host) deviceNamed(name string) (node string, ios bool) {
	for _, p := range h.state.Phones {
		if strings.EqualFold(p.Name, name) {
			return p.Node, false
		}
	}
	for _, ip := range h.state.IPhones {
		if strings.EqualFold(ip.Name, name) {
			return ip.Node, true
		}
	}
	found, many := "", false
	for _, p := range h.state.Phones {
		if strings.EqualFold(p.Machine, name) {
			many = many || found != "" || p.Friend != ""
			found, ios = p.Node, false
		}
	}
	for _, ip := range h.state.IPhones {
		if strings.EqualFold(ip.Machine, name) {
			many = many || found != "" || ip.FriendName != ""
			found, ios = ip.Node, true
		}
	}
	if many {
		return "", false
	}
	return found, ios
}

func (h *Host) nameTaken(self any, name string) bool {
	for _, p := range h.state.Phones {
		if any(p) != self && strings.EqualFold(p.Name, name) {
			return true
		}
	}
	for _, ip := range h.state.IPhones {
		if any(ip) != self && strings.EqualFold(ip.Name, name) {
			return true
		}
	}
	return false
}

func (h *Host) uniqueName(self any, name string) string {
	friend := ""
	switch x := self.(type) {
	case *phone:
		friend = x.Friend
	case *iphone:
		friend = x.FriendName
	}
	for i := 1; ; i++ {
		n := name
		if i > 1 {
			n = fmt.Sprintf("%s-%d", name, i)
		}
		if h.nameTaken(self, n) {
			continue
		}
		if f := h.friendNamed(n); f != "" && !strings.EqualFold(f, friend) {
			continue
		}
		return n
	}
}

func nameFrom(machine, fallback string) string {
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
		s = fallback
	}
	return s
}

func (h *Host) checkFriendName(friend string) error {
	if err := checkName(friend); err != nil {
		return err
	}
	if h.friendNamed(friend) != "" {
		return nil
	}
	if node, _ := h.deviceNamed(friend); node != "" {
		return fmt.Errorf("%s is the name of one of your phones: call your friend something else", friend)
	}
	return nil
}

func (h *Host) audience(names []string, ios bool) (audience, error) {
	var a audience
	for _, n := range names {
		n = strings.TrimSpace(n)
		if n == "" {
			continue
		}
		if f := h.friendNamed(n); f != "" {
			if !h.friendHas(f, ios) {
				return audience{}, fmt.Errorf("%s has no %s paired: cloche pair %s -friend %s", f, kind(ios), kindFlag(ios), f)
			}
			if !slices.Contains(a.For, f) {
				a.For = append(a.For, f)
			}
			continue
		}
		node, isIOS := h.deviceNamed(n)
		switch {
		case node == "":
			return audience{}, fmt.Errorf("no friend or phone called %s%s", n, h.pairedNames())
		case isIOS && !ios:
			return audience{}, fmt.Errorf("%s is an iPhone: it can't install an APK", n)
		case !isIOS && ios:
			return audience{}, fmt.Errorf("%s is an Android phone: it can't install an iOS build", n)
		}
		if !slices.Contains(a.Phones, node) {
			a.Phones = append(a.Phones, node)
		}
	}
	return a, nil
}

func (h *Host) iphonesFor(a audience) ([]*iphone, error) {
	var out []*iphone
	for _, ip := range h.state.IPhones {
		mine := a.mine() && ip.FriendName == ""
		friend := ip.FriendName != "" && slices.Contains(a.For, ip.FriendName)
		if mine || friend || slices.Contains(a.Phones, ip.Node) {
			out = append(out, ip)
		}
	}
	switch {
	case len(out) > 0:
		return out, nil
	case len(h.state.IPhones) > 0:
		return nil, fmt.Errorf("no iPhone of yours is paired: to send it to a friend's, name them with -to%s", h.pairedNames())
	}
	return nil, errors.New("no iPhone is paired: run cloche pair -ios")
}

func (h *Host) pairedNames() string {
	var names []string
	seen := map[string]bool{}
	add := func(n string) {
		if !seen[strings.ToLower(n)] {
			seen[strings.ToLower(n)] = true
			names = append(names, n)
		}
	}
	for _, p := range h.state.Phones {
		add(firstNonEmpty(p.Friend, p.Name))
	}
	for _, ip := range h.state.IPhones {
		add(firstNonEmpty(ip.FriendName, ip.Name))
	}
	if len(names) == 0 {
		return ""
	}
	return " (paired: " + strings.Join(names, ", ") + ")"
}

func (h *Host) Unpair(name string) ([]string, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	var phones []*phone
	var iphones []*iphone
	switch f := h.friendNamed(name); {
	case name == "" && len(h.state.Phones)+len(h.state.IPhones) == 0:
		return nil, errors.New("no phone is paired")
	case name == "" && len(h.state.Phones) == 1 && len(h.state.IPhones) == 0:
		phones = h.state.Phones
	case name == "" && len(h.state.Phones) == 0 && len(h.state.IPhones) == 1:
		iphones = h.state.IPhones
	case name == "":
		return nil, fmt.Errorf("more than one phone is paired: say which, cloche unpair <name>%s", h.pairedNames())
	case f != "":
		for _, p := range h.state.Phones {
			if p.Friend == f {
				phones = append(phones, p)
			}
		}
		for _, ip := range h.state.IPhones {
			if ip.FriendName == f {
				iphones = append(iphones, ip)
			}
		}
	default:
		node, ios := h.deviceNamed(name)
		switch {
		case node == "":
			return nil, fmt.Errorf("no friend or phone called %s%s", name, h.pairedNames())
		case ios:
			iphones = append(iphones, h.iphone(node))
		default:
			phones = append(phones, h.phone(node))
		}
	}
	var gone []string
	for _, p := range phones {
		gone = append(gone, p.Name)
		h.log.Printf("unpaired %s (%s)", p.Name, p.Node)
	}
	for _, ip := range iphones {
		gone = append(gone, ip.Name)
		h.log.Printf("unpaired iPhone %s (%s)", ip.Name, ip.Node)
	}
	h.state.Phones = slices.DeleteFunc(h.state.Phones, func(p *phone) bool { return slices.Contains(phones, p) })
	h.state.IPhones = slices.DeleteFunc(h.state.IPhones, func(ip *iphone) bool { return slices.Contains(iphones, ip) })
	h.epoch++
	h.broadcast()
	return gone, h.saveState()
}

type FriendStatus struct {
	Name   string        `json:"name"`
	Phones []FriendPhone `json:"phones"`
	Last   *Summary      `json:"last,omitempty"`
}

type FriendPhone struct {
	Name string    `json:"name"`
	IOS  bool      `json:"ios,omitempty"`
	Live bool      `json:"live,omitempty"`
	Seen time.Time `json:"seen,omitzero"`
}

func (h *Host) friends() []FriendStatus {
	var out []FriendStatus
	at := map[string]int{}
	get := func(name string) *FriendStatus {
		i, ok := at[name]
		if !ok {
			i = len(out)
			at[name] = i
			out = append(out, FriendStatus{Name: name})
		}
		return &out[i]
	}
	nodes := map[string]string{}
	for _, p := range h.state.Phones {
		if p.Friend != "" {
			f := get(p.Friend)
			f.Phones = append(f.Phones, FriendPhone{Name: p.Name, Live: h.polls[p.Node] > 0, Seen: h.seenAt[p.Node]})
			nodes[p.Node] = p.Friend
		}
	}
	for _, ip := range h.state.IPhones {
		if ip.FriendName != "" {
			f := get(ip.FriendName)
			f.Phones = append(f.Phones, FriendPhone{Name: ip.Name, IOS: true, Seen: h.seenAt[ip.Node]})
			nodes[ip.Node] = ip.FriendName
		}
	}
	for i := len(h.records) - 1; i >= 0; i-- {
		r := h.records[i]
		var whose []string
		whose = append(whose, r.For...)
		for _, node := range r.Phones {
			if f := nodes[node]; f != "" {
				whose = append(whose, f)
			}
		}
		for _, name := range whose {
			if j, ok := at[name]; ok && out[j].Last == nil {
				s := h.summary(r)
				out[j].Last = &s
			}
		}
	}
	return out
}
