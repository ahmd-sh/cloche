package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/ahmd-sh/cloche/internal/host"
	"github.com/ahmd-sh/cloche/internal/wire"
)

func status(args []string) error {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	asJSON := fs.Bool("json", false, "print as JSON")
	fs.Parse(args)
	var st host.Status
	if err := dial().do(context.Background(), "GET", "/status", nil, nil, &st); err != nil {
		return err
	}
	if *asJSON {
		e := json.NewEncoder(os.Stdout)
		e.SetIndent("", "  ")
		return e.Encode(st)
	}

	fmt.Printf("cloche on %s (%s) · key %s\n", st.Host, st.Addr, st.Fingerprint)
	if st.Display != "" {
		fmt.Printf("name    %s\n", st.Display)
	}
	if len(st.Phones) == 0 && len(st.IPhones) == 0 {
		fmt.Println("phone   none paired: run cloche pair -android, or cloche pair -ios")
	}
	for _, p := range st.Phones {
		fmt.Printf("phone   %s · %s\n", p.Name, strings.Join(phoneBits(p), " · "))
	}
	for _, ip := range st.IPhones {
		var bits []string
		if ip.Friend != "" {
			bits = append(bits, ip.Friend+"'s, on "+ip.Login)
		}
		if ip.UDID != "" {
			bits = append(bits, "UDID "+ip.UDID)
		} else {
			bits = append(bits, "no UDID yet")
		}
		if !ip.Notify {
			bits = append(bits, "notifications off")
		}
		if !ip.Seen.IsZero() {
			bits = append(bits, "seen "+ago(ip.Seen))
		}
		fmt.Printf("iphone  %s · %s\n", ip.Name, strings.Join(bits, " · "))
	}
	switch {
	case st.WebErr != "":
		fmt.Printf("web     down: %s\n", st.WebErr)
	case st.Web != "" && len(st.IPhones) == 0:
		fmt.Printf("web     %s, for iPhones: cloche pair -ios\n", st.Web)
	}
	if st.Update != "" {
		fmt.Printf("update  cloche %s is out: cloche update\n", st.Update)
	}
	if !st.Pairing.IsZero() {
		fmt.Printf("pairing open until %s\n", st.Pairing.Local().Format("15:04"))
	}
	if len(st.Deliveries) == 0 {
		return nil
	}
	fmt.Println()
	for _, d := range st.Deliveries {
		printDelivery(d)
	}
	return nil
}

func phoneBits(p host.PhoneStatus) []string {
	var bits []string
	if p.Friend != "" {
		who := p.Friend + "'s"
		if p.Login != "" {
			who += ", on " + p.Login
		}
		bits = append(bits, who)
	}
	switch {
	case p.Live:
		bits = append(bits, "connected")
	case !p.Seen.IsZero():
		bits = append(bits, "offline, last seen "+ago(p.Seen))
	default:
		bits = append(bits, "not connected since cloche started")
	}
	if p.App != "" {
		bits = append(bits, "Cloche "+p.App)
	}
	return bits
}

func printDelivery(d host.Summary) {
	line := fmt.Sprintf("#%-3d %-10s %s · %s · %s", d.Seq, ago(d.PushedAt), what(d), mb(d.File.Size), deliveryState(d))
	fmt.Println(line)
	var more []string
	if d.Source != nil && d.Source.Commit != "" {
		more = append(more, from(d.Source))
	}
	if d.Note != "" {
		more = append(more, fmt.Sprintf("%q", d.Note))
	}
	if len(more) > 0 {
		fmt.Printf("     %s\n", strings.Join(more, " · "))
	}
}

func deliveryState(d host.Summary) string {
	if len(d.Phones) == 0 {
		if d.IPA != nil {
			return "its iPhones were unpaired"
		}
		if len(d.For) > 0 {
			return "for " + strings.Join(d.For, ", ") + ", whose phones were unpaired"
		}
		return "waiting for a phone"
	}
	var parts []string
	for _, p := range d.Phones {
		if d.IPA != nil {
			parts = append(parts, iphoneState(p))
			continue
		}
		st := androidState(p)
		if len(d.Phones) > 1 || p.Friend != "" {
			st = p.Phone + " " + st
		}
		parts = append(parts, st)
	}
	return strings.Join(parts, "; ")
}

func androidState(p host.Progress) string {
	switch {
	case p.Result != nil && p.Result.Detail != "":
		return p.Result.Status + ": " + p.Result.Detail
	case p.Result != nil:
		return p.Result.Status
	case !p.Fetched.IsZero():
		return "downloading"
	case !p.Taken.IsZero():
		return "on its way"
	}
	return "waiting for the phone"
}

func iphoneState(p host.Progress) string {
	switch {
	case p.Result != nil && p.Result.Status == wire.Downloaded:
		return "downloaded by " + p.Phone
	case p.Result != nil && p.Result.Status == wire.Installing:
		return "downloading to " + p.Phone
	case p.Result != nil && p.Result.Status == wire.Asked:
		return "Install tapped on " + p.Phone
	case !p.Opened.IsZero():
		return "looked at on " + p.Phone
	case p.NotifyErr != "":
		return "for " + p.Phone + ", not notified: " + p.NotifyErr
	case !p.Taken.IsZero():
		return p.Phone + " notified"
	}
	return "for " + p.Phone
}

func unpair(args []string) error {
	if len(args) > 1 || len(args) == 1 && strings.HasPrefix(args[0], "-") {
		return exit{2, "usage: cloche unpair [name]: a phone, or a friend to forget all their phones"}
	}
	q := url.Values{}
	if len(args) == 1 {
		q.Set("name", args[0])
	}
	var res struct {
		Unpaired []string `json:"unpaired"`
	}
	if err := dial().do(context.Background(), "POST", "/unpair", q, nil, &res); err != nil {
		return err
	}
	if len(res.Unpaired) == 1 {
		fmt.Printf("unpaired %s: it can't reach this computer until it pairs again\n", res.Unpaired[0])
	} else {
		fmt.Printf("unpaired %s: they can't reach this computer until they pair again\n", strings.Join(res.Unpaired, ", "))
	}
	return nil
}

func friends(args []string) error {
	fs := flag.NewFlagSet("friends", flag.ExitOnError)
	asJSON := fs.Bool("json", false, "print as JSON")
	fs.Parse(args)
	var st host.Status
	if err := dial().do(context.Background(), "GET", "/status", nil, nil, &st); err != nil {
		return err
	}
	if *asJSON {
		e := json.NewEncoder(os.Stdout)
		e.SetIndent("", "  ")
		if st.Friends == nil {
			st.Friends = []host.FriendStatus{}
		}
		return e.Encode(st.Friends)
	}
	if len(st.Friends) == 0 {
		fmt.Println("no friends' phones are paired: cloche pair -android -friend <name>, or cloche pair -ios -friend <name>")
		return nil
	}
	for i, f := range st.Friends {
		if i > 0 {
			fmt.Println()
		}
		fmt.Println(f.Name)
		for _, p := range f.Phones {
			k, seen := "Android", "not seen since cloche started"
			if p.IOS {
				k = "iPhone"
			}
			switch {
			case p.Live:
				seen = "connected"
			case !p.Seen.IsZero():
				seen = "seen " + ago(p.Seen)
			}
			fmt.Printf("  %-12s %s · %s\n", p.Name, k, seen)
		}
		if f.Last != nil {
			fmt.Printf("  last build   #%d %s · %s · %s\n", f.Last.Seq, what(*f.Last), ago(f.Last.PushedAt), deliveryState(*f.Last))
		} else {
			fmt.Println("  no builds sent yet")
		}
	}
	return nil
}
