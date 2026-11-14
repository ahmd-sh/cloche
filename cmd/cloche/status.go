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
	switch p := st.Phone; {
	case p == nil:
		fmt.Println("phone   none paired: run cloche pair")
	case p.Live:
		fmt.Printf("phone   %s · connected%s\n", p.Name, app(p))
	case !p.Seen.IsZero():
		fmt.Printf("phone   %s · offline, last seen %s%s\n", p.Name, ago(p.Seen), app(p))
	default:
		fmt.Printf("phone   %s · hasn't connected since pairing\n", p.Name)
	}
	for _, ip := range st.IPhones {
		var bits []string
		if ip.Friend {
			bits = append(bits, "friend's, on "+ip.Login)
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
		state := "waiting for the phone"
		switch {
		case d.IPA != nil:
			state = iphoneState(d)
		case d.Result != nil:
			state = d.Result.Status
			if d.Result.Detail != "" {
				state += ": " + d.Result.Detail
			}
		case !d.Fetched.IsZero():
			state = "downloading"
		case !d.Taken.IsZero():
			state = "on its way"
		}
		line := fmt.Sprintf("#%-3d %-10s %s · %s · %s", d.Seq, ago(d.PushedAt), what(d), mb(d.File.Size), state)
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
	return nil
}

func app(p *host.PhoneStatus) string {
	if p.App == "" {
		return ""
	}
	return " · Cloche " + p.App
}

func iphoneState(d host.Summary) string {
	switch {
	case d.Result != nil && d.Result.Status == wire.Downloaded:
		return "downloaded by " + d.To
	case d.Result != nil && d.Result.Status == wire.Installing:
		return "downloading to " + d.To
	case d.Result != nil && d.Result.Status == wire.Asked:
		return "Install tapped on " + d.To
	case !d.Opened.IsZero():
		return "looked at on " + d.To
	case d.NotifyErr != "":
		return "for " + d.To + ", not notified: " + d.NotifyErr
	case !d.Taken.IsZero():
		return d.To + " notified"
	}
	return "for " + d.To
}

func unpair(args []string) error {
	if len(args) > 1 {
		return exit{2, "usage: cloche unpair [iphone]"}
	}
	if len(args) == 1 {
		if err := dial().do(context.Background(), "POST", "/unpair", url.Values{"iphone": {args[0]}}, nil, nil); err != nil {
			return err
		}
		fmt.Printf("unpaired %s: it can't reach this computer's web app until it pairs again\n", args[0])
		return nil
	}
	if err := dial().do(context.Background(), "POST", "/unpair", nil, nil, nil); err != nil {
		return err
	}
	fmt.Println("unpaired: the phone can't reach this computer until it pairs again")
	return nil
}
