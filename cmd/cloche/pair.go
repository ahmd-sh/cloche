package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"rsc.io/qr"

	"github.com/ahmd-sh/cloche/internal/host"
)

func pair(args []string) error {
	fs := flag.NewFlagSet("pair", flag.ExitOnError)
	ttl := fs.Duration("ttl", 0, "how long the code works (default 15m; 24h for -friend)")
	ios := fs.Bool("ios", false, "pair an iPhone or iPad, through its web app")
	friend := fs.String("friend", "", "pair a friend's iPhone, on their own Tailscale account, and call it this (implies -ios)")
	fs.Parse(args)
	ctx := context.Background()
	l := dial()
	if *friend != "" || *ios {
		if *ttl == 0 {
			*ttl = 15 * time.Minute
			if *friend != "" {
				*ttl = 24 * time.Hour
			}
		}
		return pairIPhone(ctx, l, *ttl, *friend)
	}
	if *ttl == 0 {
		*ttl = 15 * time.Minute
	}

	var before host.Status
	if err := l.do(ctx, "GET", "/status", nil, nil, &before); err != nil {
		return err
	}
	var p host.PairStart
	if err := l.do(ctx, "POST", "/pair", url.Values{"ttl": {strconv.Itoa(int(ttl.Seconds()))}}, nil, &p); err != nil {
		return err
	}
	if err := printQR(os.Stdout, p.URL); err != nil {
		return err
	}
	fmt.Printf(`
Scan it with the phone's camera, open the link on the phone, or type
the code into Cloche:

  mac   %s  (%s)
  code  %s
  link  %s

The computer's key is %s; Cloche shows it as it pairs.
The code works once, until %s.
`, p.Host, p.Addr, p.Code, p.URL, p.Fingerprint, p.Expires.Local().Format("15:04"))
	if before.Phone != nil {
		fmt.Printf("Pairing replaces %s.\n", before.Phone.Name)
	}

	for time.Now().Before(p.Expires) {
		var st host.Status
		secs := strconv.Itoa(int(min(time.Until(p.Expires), 5*time.Minute).Seconds()) + 1)
		tctx, cancel := context.WithTimeout(ctx, 6*time.Minute)
		err := l.do(tctx, "GET", "/status", url.Values{"until": {"paired"}, "wait": {secs}}, nil, &st)
		cancel()
		if err != nil {
			return err
		}
		if st.Phone != nil && (before.Phone == nil || st.Phone.PairedAt.After(before.Phone.PairedAt)) {
			fmt.Printf("\npaired with %s (%s)\n", st.Phone.Name, st.Phone.Device)
			return nil
		}
		if st.Pairing.IsZero() {
			break
		}
	}
	return exit{1, "pairing closed without a phone"}
}

func pairIPhone(ctx context.Context, l local, ttl time.Duration, friend string) error {
	var before host.Status
	if err := l.do(ctx, "GET", "/status", nil, nil, &before); err != nil {
		return err
	}
	q := url.Values{"ttl": {strconv.Itoa(int(ttl.Seconds()))}, "ios": {"1"}}
	if friend != "" {
		q.Set("friend", friend)
	}
	var p host.PairStart
	if err := l.do(ctx, "POST", "/pair", q, nil, &p); err != nil {
		return err
	}
	if friend == "" {
		if err := printQR(os.Stdout, p.URL); err != nil {
			return err
		}
		fmt.Printf(`
Scan it with the iPhone's camera, or open the link in Safari on it:

  %s

The iPhone needs Tailscale on, signed in as you.
The link works once, until %s.
`, p.URL, p.Expires.Local().Format("15:04"))
	} else {
		fmt.Printf(`Send %s this link, to open in Safari on their iPhone:

  %s

Their iPhone needs Tailscale, with this computer shared to them. First, in
the admin console's access controls, let people you share with reach
only cloche's port. Otherwise they reach every port on this computer:

  {"src": ["autogroup:shared"], "dst": ["*"], "ip": ["tcp:%d"]}

and make sure no other rule's src covers them ("*" does: use
"autogroup:member" for your own devices). Then open this computer's menu
(...) under Machines, choose Share, and send them the invite. They
accept it signed in to Tailscale on their iPhone.

Builds go to %s only when a push names it: -to %s.
The link works once, until %s.
`, friend, p.URL, host.WebPort, friend, friend, p.Expires.Local().Format("Mon 15:04"))
	}

	epoch := before.Epoch
	var name string
	for time.Now().Before(p.Expires) {
		var st host.Status
		secs := strconv.Itoa(int(min(time.Until(p.Expires), 5*time.Minute).Seconds()) + 1)
		tctx, cancel := context.WithTimeout(ctx, 6*time.Minute)
		err := l.do(tctx, "GET", "/status", url.Values{"until": {"epoch"}, "since": {strconv.FormatInt(epoch, 10)}, "wait": {secs}}, nil, &st)
		cancel()
		if err != nil {
			return err
		}
		epoch = st.Epoch
		ip := newIPhone(before, st)
		if name == "" && ip != nil {
			name = ip.Name
			fmt.Printf("\npaired with %s (%s, %s)\n", ip.Name, ip.Machine, ip.Login)
			if ip.UDID == "" {
				fmt.Println("waiting for its UDID: the web app asks for it (Ctrl-C to stop; cloche status shows it later)")
			}
		}
		if name != "" {
			for _, x := range st.IPhones {
				if x.Name == name && x.UDID != "" {
					printUDID(x)
					return nil
				}
			}
		}
		if name == "" && st.Pairing.IsZero() {
			return exit{1, "pairing closed without an iPhone"}
		}
	}
	if name != "" {
		return nil
	}
	return exit{1, "pairing closed without an iPhone"}
}

func newIPhone(before, now host.Status) *host.IPhoneStatus {
	for i, x := range now.IPhones {
		fresh := true
		for _, b := range before.IPhones {
			if b.Name == x.Name && !x.PairedAt.After(b.PairedAt) {
				fresh = false
			}
		}
		if fresh {
			return &now.IPhones[i]
		}
	}
	return nil
}

func printUDID(ip host.IPhoneStatus) {
	model := ip.Model
	if model != "" {
		model = " (" + model + ")"
	}
	fmt.Printf(`
%s's UDID: %s%s

Builds install on it once its UDID is in their provisioning profile:
register it at https://developer.apple.com/account/resources/devices/add
(or let Xcode do it with -allowProvisioningUpdates), then build again.
`, ip.Name, ip.UDID, model)
}

func printQR(w io.Writer, text string) error {
	c, err := qr.Encode(text, qr.L)
	if err != nil {
		return err
	}
	const quiet = 2
	light := func(x, y int) bool {
		if x < -quiet || y < -quiet || x >= c.Size+quiet || y >= c.Size+quiet {
			return false
		}
		return !c.Black(x, y)
	}
	var b strings.Builder
	for y := -quiet; y < c.Size+quiet; y += 2 {
		b.WriteString("  ")
		for x := -quiet; x < c.Size+quiet; x++ {
			switch top, bottom := light(x, y), light(x, y+1); {
			case top && bottom:
				b.WriteRune('█')
			case top:
				b.WriteRune('▀')
			case bottom:
				b.WriteRune('▄')
			default:
				b.WriteRune(' ')
			}
		}
		b.WriteByte('\n')
	}
	_, err = io.WriteString(w, b.String())
	return err
}
