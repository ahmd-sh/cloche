package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"
)

const usage = `cloche puts builds on your phone from anywhere on your tailnet.

usage:
  cloche push [-m note] [-wait] <file.apk>   deliver a build to the phone
  cloche push [-to name] <App.ipa | App.app> deliver an iOS build to an iPhone
  cloche status                              the phones, and recent deliveries
  cloche pair [-ttl 15m]                     pair a phone (replaces the paired one)
  cloche pair -ios                           pair an iPhone of yours
  cloche pair -friend name                   pair a friend's iPhone, on their Tailscale
  cloche unpair [iphone]                     forget the phone, or an iPhone
  cloche setup                               check Tailscale, keep cloche serve running, add the agent skill
  cloche uninstall                           stop cloche serve and remove what setup added
  cloche serve [-listen addr] [-dev]         the computer's half, kept running in the background
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	var err error
	switch cmd, args := os.Args[1], os.Args[2:]; cmd {
	case "serve":
		err = serve(args)
	case "setup":
		err = setup(args)
	case "uninstall":
		err = uninstall(args)
	case "push":
		err = push(args)
	case "pair":
		err = pair(args)
	case "status":
		err = status(args)
	case "unpair":
		err = unpair(args)
	case "help", "-h", "-help", "--help":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "cloche: no command %q\n\n%s", cmd, usage)
		os.Exit(2)
	}
	if err != nil {
		var ex exit
		if errors.As(err, &ex) {
			if ex.msg != "" {
				fmt.Fprintln(os.Stderr, "cloche:", ex.msg)
			}
			os.Exit(ex.code)
		}
		fmt.Fprintln(os.Stderr, "cloche:", err)
		os.Exit(1)
	}
}

type exit struct {
	code int
	msg  string
}

func (e exit) Error() string { return e.msg }

func home() string {
	if d := os.Getenv("CLOCHE_HOME"); d != "" {
		return d
	}
	d, err := os.UserConfigDir()
	if err != nil {
		h, _ := os.UserHomeDir()
		d = filepath.Join(h, ".config")
	}
	return filepath.Join(d, "cloche")
}

func socketPath() string {
	p := filepath.Join(home(), "cloche.sock")
	if len(p) < 100 {
		return p
	}
	d := sha256.Sum256([]byte(home()))
	return filepath.Join(os.TempDir(), "cloche-"+hex.EncodeToString(d[:6])+".sock")
}

type local struct{ http *http.Client }

func dial() local {
	return local{&http.Client{Transport: &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", socketPath())
		},
	}}}
}

var errNotRunning = errors.New("cloche isn't running on this computer: run cloche setup")

func (l local) do(ctx context.Context, method, path string, q url.Values, body, out any) error {
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		r = bytes.NewReader(b)
	}
	u := "http://cloche" + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, u, r)
	if err != nil {
		return err
	}
	res, err := l.http.Do(req)
	if err != nil {
		var op *net.OpError
		if errors.As(err, &op) && op.Op == "dial" {
			return errNotRunning
		}
		return err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		var e struct{ Error string }
		json.NewDecoder(res.Body).Decode(&e)
		if e.Error == "" {
			e.Error = res.Status
		}
		return errors.New(e.Error)
	}
	if out == nil || res.StatusCode == http.StatusNoContent {
		return nil
	}
	return json.NewDecoder(res.Body).Decode(out)
}

func ago(t time.Time) string {
	d := time.Since(t).Round(time.Second)
	switch {
	case d < 5*time.Second:
		return "just now"
	case d < time.Minute:
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
	return t.Local().Format("Jan 2 15:04")
}

func mb(n int64) string { return fmt.Sprintf("%.1f MB", float64(n)/(1<<20)) }
