package main

import (
	"context"
	"flag"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ahmd-sh/cloche/internal/host"
	"github.com/ahmd-sh/cloche/internal/wire"
)

func push(args []string) error {
	fs := flag.NewFlagSet("push", flag.ExitOnError)
	note := fs.String("m", "", "one line on what changed, shown with the build")
	wait := fs.Bool("wait", false, "wait until it's installed, declined or refused")
	timeout := fs.Duration("timeout", 30*time.Minute, "how long -wait waits")
	to := fs.String("to", "", "friends or phones to send it to, by name, comma-separated (default: your own phones)")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: cloche push [-m note] [-wait] [-timeout 30m] [-to names] <file.apk | file.ipa | App.app>")
		fs.PrintDefaults()
	}
	rest := parseInterleaved(fs, args)
	if len(rest) != 1 {
		fs.Usage()
		return exit{code: 2}
	}
	path, err := filepath.Abs(rest[0])
	if err != nil {
		return err
	}
	ctx := context.Background()
	l := dial()

	var s host.Summary
	if err := l.do(ctx, "POST", "/push", nil, host.PushRequest{Path: path, Note: *note, Source: source(filepath.Dir(path)), To: names(*to)}, &s); err != nil {
		return err
	}
	if s.IPA != nil {
		return pushedIPA(ctx, l, s, *wait, *timeout)
	}
	fmt.Printf("pushed %s · %s\n", what(s), mb(s.File.Size))
	if src := s.Source; src != nil && src.Commit != "" {
		fmt.Printf("  from %s\n", from(src))
	}
	if len(s.Phones) == 0 {
		fmt.Println("  no Android phone of yours is paired yet: run cloche pair -android. A phone that pairs within the hour gets this.")
		if *wait {
			return exit{4, "nothing to wait for without a phone"}
		}
		return nil
	}

	tctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	err = l.do(tctx, "GET", "/deliveries/"+s.ID, url.Values{"until": {"taken"}, "wait": {"10"}}, nil, &s)
	cancel()
	if err != nil && tctx.Err() == nil {
		return err
	}
	for _, p := range s.Phones {
		fmt.Printf("  %s\n", arrival(p))
	}
	if !*wait {
		return nil
	}
	return follow(ctx, l, s, *timeout, androidStep)
}

func names(list string) []string {
	var out []string
	for _, n := range strings.Split(list, ",") {
		if n = strings.TrimSpace(n); n != "" {
			out = append(out, n)
		}
	}
	return out
}

func arrival(p host.Progress) string {
	switch {
	case !p.Taken.IsZero():
		return p.Phone + " has it; it notifies once it's downloaded and checked"
	case p.Live:
		return p.Phone + " is connected but hasn't picked it up yet"
	case !p.Seen.IsZero():
		return fmt.Sprintf("%s is offline (last seen %s); it gets this when it's back", p.Phone, ago(p.Seen))
	}
	return p.Phone + " hasn't connected since it paired; it gets this when it does"
}

type step struct {
	line string
	done bool
	ok   bool
	err  error
}

func androidStep(p host.Progress) step {
	r := p.Result
	if r == nil {
		return step{}
	}
	detail := ""
	if r.Detail != "" {
		detail = ": " + r.Detail
	}
	switch r.Status {
	case wire.Ready:
		return step{line: "ready on " + p.Phone + ", waiting for a tap"}
	case wire.Installing:
		return step{line: "installing on " + p.Phone}
	case wire.Installed:
		return step{line: "installed on " + p.Phone, done: true, ok: true}
	case wire.Superseded:
		return step{line: "superseded on " + p.Phone + " by a newer push", done: true, err: exit{3, "superseded by a newer push"}}
	}
	msg := r.Status + " on " + p.Phone + detail
	return step{line: msg, done: true, err: exit{3, msg}}
}

func iphoneStep(sideload bool) func(host.Progress) step {
	return func(p host.Progress) step {
		r := p.Result
		switch {
		case r == nil:
		case r.Status == wire.Asked:
			return step{line: "Install tapped: iOS is asking " + p.Phone}
		case r.Status == wire.Installing:
			return step{line: p.Phone + " is downloading it"}
		case r.Status == wire.Downloaded && sideload:
			return step{line: p.Phone + " downloaded it, for SideStore or AltStore", done: true, ok: true}
		case r.Status == wire.Downloaded:
			return step{line: p.Phone + " downloaded it; iOS installs it from there, and doesn't say when it's done", done: true, ok: true}
		}
		return step{}
	}
}

func follow(ctx context.Context, l local, s host.Summary, timeout time.Duration, at func(host.Progress) step) error {
	deadline := time.Now().Add(timeout)
	said := map[string]string{}
	for {
		open := 0
		var failed error
		for _, p := range s.Phones {
			st := at(p)
			if st.line != "" && said[p.Phone] != st.line {
				said[p.Phone] = st.line
				fmt.Printf("  %s\n", st.line)
			}
			switch {
			case st.ok:
				return nil
			case st.done:
				failed = st.err
			default:
				open++
			}
		}
		if len(s.Phones) > 0 && open == 0 {
			return failed
		}
		left := time.Until(deadline)
		if left <= 0 {
			return exit{4, fmt.Sprintf("no answer after %s", timeout)}
		}
		secs := strconv.Itoa(int(min(left, 5*time.Minute).Seconds()) + 1)
		tctx, cancel := context.WithTimeout(ctx, 6*time.Minute)
		err := l.do(tctx, "GET", "/deliveries/"+s.ID, url.Values{"until": {"change"}, "since": {strconv.FormatInt(s.Rev, 10)}, "wait": {secs}}, nil, &s)
		cancel()
		if err != nil {
			return err
		}
	}
}

func phoneNames(ps []host.Progress) string {
	var n []string
	for _, p := range ps {
		n = append(n, p.Phone)
	}
	return strings.Join(n, ", ")
}

func pushedIPA(ctx context.Context, l local, s host.Summary, wait bool, timeout time.Duration) error {
	fmt.Printf("pushed %s · %s for %s\n", what(s), mb(s.File.Size), phoneNames(s.Phones))
	if p := s.IPA.Profile; p != nil {
		fmt.Printf("  %s profile %q, team %s\n", p.Kind, p.Name, p.Team)
	}
	if src := s.Source; src != nil && src.Commit != "" {
		fmt.Printf("  from %s\n", from(src))
	}
	for _, w := range s.Warnings {
		fmt.Printf("  note: %s\n", w)
	}
	tctx, cancel := context.WithTimeout(ctx, 40*time.Second)
	err := l.do(tctx, "GET", "/deliveries/"+s.ID, url.Values{"until": {"taken"}, "wait": {"35"}}, nil, &s)
	cancel()
	if err != nil {
		return err
	}
	for _, p := range s.Phones {
		switch {
		case p.NotifyErr != "":
			fmt.Printf("  %s: %s\n", p.Phone, p.NotifyErr)
		case !p.Taken.IsZero() && s.IPA.Profile == nil:
			fmt.Printf("  %s was notified; it downloads it from Cloche\n", p.Phone)
		case !p.Taken.IsZero():
			fmt.Printf("  %s was notified; a tap installs it\n", p.Phone)
		default:
			fmt.Printf("  still notifying %s\n", p.Phone)
		}
	}
	if !wait {
		return nil
	}
	return follow(ctx, l, s, timeout, iphoneStep(s.IPA.Profile == nil))
}

func source(dir string) *wire.Source {
	top, err := git(dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil
	}
	s := &wire.Source{Dir: top, Repo: filepath.Base(top)}
	s.Commit, _ = git(dir, "rev-parse", "--short", "HEAD")
	s.Branch, _ = git(dir, "rev-parse", "--abbrev-ref", "HEAD")
	if out, err := git(dir, "status", "--porcelain", "--untracked-files=no"); err == nil && out != "" {
		s.Dirty = true
	}
	return s
}

func git(dir string, args ...string) (string, error) {
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).Output()
	return strings.TrimSpace(string(out)), err
}

func what(s host.Summary) string {
	if i := s.IPA; i != nil {
		v := i.Version
		if i.Build != "" && i.Build != i.Version {
			v += " (" + i.Build + ")"
		}
		return fmt.Sprintf("%s %s · %s", i.Name, v, i.BundleID)
	}
	if a := s.APK; a != nil {
		name := a.Label
		if name == "" {
			name = a.Package
		}
		return fmt.Sprintf("%s %s (%d) · %s", name, a.VersionName, a.VersionCode, a.Package)
	}
	return s.File.Name
}

func from(s *wire.Source) string {
	f := s.Repo + "@" + s.Commit
	if s.Dirty {
		f += " with changes"
	}
	if s.Branch != "" && s.Branch != "HEAD" {
		f += " on " + s.Branch
	}
	return f
}

func parseInterleaved(fs *flag.FlagSet, args []string) []string {
	var rest []string
	for {
		fs.Parse(args)
		args = fs.Args()
		if len(args) == 0 {
			return rest
		}
		rest = append(rest, args[0])
		args = args[1:]
	}
}
