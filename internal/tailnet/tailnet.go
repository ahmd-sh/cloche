package tailnet

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"
)

type Peer struct {
	Node   string
	Name   string
	Login  string
	OS     string
	Tagged bool
}

type Whois interface {
	Whois(ctx context.Context, ip netip.Addr) (Peer, error)
}

type CLI struct {
	Path string

	mu    sync.Mutex
	cache map[netip.Addr]cached
}

type cached struct {
	peer Peer
	at   time.Time
}

const keep = 2 * time.Minute

func Find() (*CLI, error) {
	if p, err := exec.LookPath("tailscale"); err == nil {
		return &CLI{Path: p}, nil
	}
	for _, p := range installed() {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return &CLI{Path: p}, nil
		}
	}
	return nil, errors.New("Tailscale isn't installed: get it at https://tailscale.com/download")
}

func installed() []string {
	if runtime.GOOS == "darwin" {
		return []string{
			"/usr/local/bin/tailscale",
			"/opt/homebrew/bin/tailscale",
			"/Applications/Tailscale.app/Contents/MacOS/Tailscale",
		}
	}
	return []string{"/usr/bin/tailscale", "/usr/local/bin/tailscale", "/usr/sbin/tailscale", "/snap/bin/tailscale"}
}

func (c *CLI) Self(ctx context.Context) (netip.Addr, Peer, error) {
	out, err := c.run(ctx, "ip", "-4")
	if err != nil {
		return netip.Addr{}, Peer{}, err
	}
	ip, err := netip.ParseAddr(strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0]))
	if err != nil {
		return netip.Addr{}, Peer{}, fmt.Errorf("tailscale ip: %w", err)
	}
	p, err := c.Whois(ctx, ip)
	return ip, p, err
}

type Health struct {
	State   string
	DNSName string
	HTTPS   bool
	Tagged  bool
}

func (c *CLI) Health(ctx context.Context) (Health, error) {
	out, err := c.run(ctx, "status", "--json", "--peers=false")
	if err != nil {
		return Health{}, err
	}
	var st struct {
		BackendState string
		Self         struct {
			DNSName string
			Tags    []string
		}
		CertDomains []string
	}
	if err := json.Unmarshal(out, &st); err != nil {
		return Health{}, fmt.Errorf("tailscale status: %w", err)
	}
	h := Health{State: st.BackendState, DNSName: strings.TrimSuffix(st.Self.DNSName, "."), Tagged: len(st.Self.Tags) > 0}
	for _, d := range st.CertDomains {
		if h.DNSName != "" && strings.EqualFold(strings.TrimSuffix(d, "."), h.DNSName) {
			h.HTTPS = true
		}
	}
	return h, nil
}

func (c *CLI) DNSName(ctx context.Context) (name string, https bool, err error) {
	h, err := c.Health(ctx)
	if err != nil {
		return "", false, err
	}
	if h.DNSName == "" {
		return "", false, errors.New("tailscale status: no DNS name: is MagicDNS on?")
	}
	return h.DNSName, h.HTTPS, nil
}

func (c *CLI) Operator(ctx context.Context) (string, error) {
	out, err := c.run(ctx, "debug", "prefs")
	if err != nil {
		return "", err
	}
	var p struct{ OperatorUser string }
	if err := json.Unmarshal(out, &p); err != nil {
		return "", fmt.Errorf("tailscale debug prefs: %w", err)
	}
	return p.OperatorUser, nil
}

func (c *CLI) Cert(ctx context.Context, name, certFile, keyFile string) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	_, err := c.runLong(ctx, "cert", "--cert-file", certFile, "--key-file", keyFile, name)
	return err
}

func (c *CLI) Whois(ctx context.Context, ip netip.Addr) (Peer, error) {
	ip = ip.Unmap()
	c.mu.Lock()
	if e, ok := c.cache[ip]; ok && time.Since(e.at) < keep {
		c.mu.Unlock()
		return e.peer, nil
	}
	c.mu.Unlock()

	out, err := c.run(ctx, "whois", "--json", ip.String())
	if err != nil {
		return Peer{}, err
	}
	p, err := parseWhois(out)
	if err != nil {
		return Peer{}, err
	}
	c.mu.Lock()
	if c.cache == nil {
		c.cache = map[netip.Addr]cached{}
	}
	c.cache[ip] = cached{p, time.Now()}
	c.mu.Unlock()
	return p, nil
}

func parseWhois(b []byte) (Peer, error) {
	var w struct {
		Node struct {
			StableID     string
			ComputedName string
			Hostinfo     struct{ OS string }
			Tags         []string
		}
		UserProfile struct{ LoginName string }
	}
	if err := json.Unmarshal(b, &w); err != nil {
		return Peer{}, fmt.Errorf("tailscale whois: %w", err)
	}
	if w.Node.StableID == "" {
		return Peer{}, errors.New("tailscale whois: no node")
	}
	login := w.UserProfile.LoginName
	return Peer{Node: w.Node.StableID, Name: w.Node.ComputedName, Login: login, OS: w.Node.Hostinfo.OS, Tagged: len(w.Node.Tags) > 0 || login == "tagged-devices"}, nil
}

func (c *CLI) run(ctx context.Context, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return c.runLong(ctx, args...)
}

func (c *CLI) runLong(ctx context.Context, args ...string) ([]byte, error) {
	out, err := exec.CommandContext(ctx, c.Path, args...).Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) && len(ee.Stderr) > 0 {
			return nil, fmt.Errorf("tailscale %s: %s", args[0], strings.TrimSpace(string(ee.Stderr)))
		}
		return nil, fmt.Errorf("tailscale %s: %w", args[0], err)
	}
	return out, nil
}

type Anyone struct{}

func (Anyone) Whois(context.Context, netip.Addr) (Peer, error) {
	return Peer{Node: "loopback", Name: "emulator", OS: "android"}, nil
}
