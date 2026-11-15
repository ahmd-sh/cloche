package main

import (
	"bufio"
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
	"github.com/ahmd-sh/cloche/internal/wire"
)

func pair(args []string) error {
	fs := flag.NewFlagSet("pair", flag.ExitOnError)
	ttl := fs.Duration("ttl", 0, "how long the code works (default 15m; 24h for -friend)")
	android := fs.Bool("android", false, "pair an Android phone, through the Cloche app")
	ios := fs.Bool("ios", false, "pair an iPhone or iPad, through its web app")
	friend := fs.String("friend", "", "pair a friend's phone, on their own Tailscale account, and call them this")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: cloche pair -android | -ios [-friend name] [-ttl 15m]")
		fs.PrintDefaults()
	}
	fs.Parse(args)
	if fs.NArg() > 0 {
		fs.Usage()
		return exit{code: 2}
	}
	switch {
	case *android && *ios:
		return exit{2, "pick one: -android or -ios"}
	case !*android && !*ios:
		if !terminal(os.Stdin) {
			return exit{2, "say which kind of phone: cloche pair -android, or cloche pair -ios"}
		}
		k, err := chooseKind(os.Stdin, os.Stdout)
		if err != nil {
			return err
		}
		*ios = k
	}
	if *ttl == 0 {
		*ttl = 15 * time.Minute
		if *friend != "" {
			*ttl = 24 * time.Hour
		}
	}
	ctx := context.Background()
	l := dial()
	if *ios {
		return pairIPhone(ctx, l, *ttl, *friend)
	}
	return pairAndroid(ctx, l, *ttl, *friend)
}

func chooseKind(in io.Reader, out io.Writer) (ios bool, err error) {
	r := bufio.NewReader(in)
	for {
		fmt.Fprint(out, "Pair an Android phone or an iPhone? [a/i] ")
		line, err := r.ReadString('\n')
		switch strings.ToLower(strings.TrimSpace(line)) {
		case "a", "android":
			return false, nil
		case "i", "ios", "iphone", "ipad":
			return true, nil
		}
		if err != nil {
			fmt.Fprintln(out)
			return false, exit{2, "say which kind of phone: cloche pair -android, or cloche pair -ios"}
		}
	}
}

func pairAndroid(ctx context.Context, l local, ttl time.Duration, friend string) error {
	var before host.Status
	if err := l.do(ctx, "GET", "/status", nil, nil, &before); err != nil {
		return err
	}
	q := url.Values{"ttl": {strconv.Itoa(int(ttl.Seconds()))}}
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
Scan it with the phone's camera, or open the link on the phone. Its page
helps install Tailscale and Cloche, then pairs. Or type the code into
Cloche:

  computer  %s  (%s)
  code      %s
  link      %s

The computer's key is %s; Cloche shows it as it pairs.
The code works once, until %s.
`, p.Host, p.Addr, p.Code, p.URL, p.Fingerprint, p.Expires.Local().Format("15:04"))
	} else {
		fmt.Printf(`Send %s this link, to open on their Android phone. Its page helps them
install Cloche, then pairs:

  %s

%s
Builds go to %s only when a push names them: -to %s.
The link works once, until %s.
`, friend, p.URL, sharing("phone"), friend, friend, p.Expires.Local().Format("Mon 15:04"))
	}

	epoch := before.Epoch
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
		if ph := newPhone(before, st); ph != nil {
			if ph.Friend != "" {
				fmt.Printf("\npaired with %s's phone, %s (%s, %s)\n", ph.Friend, ph.Name, ph.Device, ph.Login)
			} else {
				fmt.Printf("\npaired with %s (%s)\n", ph.Name, ph.Device)
			}
			return nil
		}
		if st.Pairing.IsZero() {
			break
		}
	}
	return exit{1, "pairing closed without a phone"}
}

func newPhone(before, now host.Status) *host.PhoneStatus {
	for i, x := range now.Phones {
		fresh := true
		for _, b := range before.Phones {
			if b.Machine == x.Machine && !x.PairedAt.After(b.PairedAt) {
				fresh = false
			}
		}
		if fresh {
			return &now.Phones[i]
		}
	}
	return nil
}

func sharing(what string) string {
	return fmt.Sprintf(`Their %s needs Tailscale, with this computer
shared to them. First, in the admin console's access controls, let people
you share with reach only cloche's ports. Otherwise they reach every port
on this computer:

  {"src": ["autogroup:shared"], "dst": ["*"], "ip": ["tcp:%d", "tcp:%d"]}

and make sure no other rule's src covers them ("*" does: use
"autogroup:member" for your own devices). Then open this computer's menu
(...) under Machines, choose Share, and send them the invite. They accept
it signed in to Tailscale on their %s.
`, what, wire.Port, host.WebPort, what)
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

%s
Builds go to %s only when a push names them: -to %s.
The link works once, until %s.
`, friend, p.URL, sharing("iPhone"), friend, friend, p.Expires.Local().Format("Mon 15:04"))
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
