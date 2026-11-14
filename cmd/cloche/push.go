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
	to := fs.String("to", "", "the iPhone to send an .ipa to, by name (default: your one iPhone)")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: cloche push [-m note] [-wait] [-timeout 30m] [-to iphone] <file.apk | file.ipa | App.app>")
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
	if err := l.do(ctx, "POST", "/push", nil, host.PushRequest{Path: path, Note: *note, Source: source(filepath.Dir(path)), To: *to}, &s); err != nil {
		return err
	}
	if s.IPA != nil {
		return pushedIPA(ctx, l, s, *wait, *timeout)
	}
	fmt.Printf("pushed %s · %s\n", what(s), mb(s.File.Size))
	if src := s.Source; src != nil && src.Commit != "" {
		fmt.Printf("  from %s\n", from(src))
	}

	var st host.Status
	if err := l.do(ctx, "GET", "/status", nil, nil, &st); err != nil {
		return err
	}
	if st.Phone == nil {
		fmt.Println("  no phone is paired yet: run cloche pair. A phone that pairs within the hour gets this.")
		if *wait {
			return exit{4, "nothing to wait for without a phone"}
		}
		return nil
	}
	phone := st.Phone.Name

	tctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	err = l.do(tctx, "GET", "/deliveries/"+s.ID, url.Values{"until": {"taken"}, "wait": {"10"}}, nil, &s)
	cancel()
	switch {
	case err == nil && !s.Taken.IsZero():
		fmt.Printf("  %s has it; it notifies once it's downloaded and checked\n", phone)
	case st.Phone.Live:
		fmt.Printf("  %s is connected but hasn't picked it up yet\n", phone)
	case !st.Phone.Seen.IsZero():
		fmt.Printf("  %s is offline (last seen %s); it gets this when it's back\n", phone, ago(st.Phone.Seen))
	default:
		fmt.Printf("  %s hasn't connected since pairing; it gets this when it does\n", phone)
	}
	if !*wait {
		return nil
	}

	deadline := time.Now().Add(*timeout)
	seen := ""
	for {
		if s.Result != nil && s.Result.Status != seen {
			seen = s.Result.Status
			if done, err := report(phone, s.Result); done {
				return err
			}
		}
		left := time.Until(deadline)
		if left <= 0 {
			return exit{4, fmt.Sprintf("no answer from %s after %s", phone, *timeout)}
		}
		secs := strconv.Itoa(int(min(left, 5*time.Minute).Seconds()) + 1)
		tctx, cancel := context.WithTimeout(ctx, 6*time.Minute)
		err := l.do(tctx, "GET", "/deliveries/"+s.ID, url.Values{"until": {"change"}, "from": {seen}, "wait": {secs}}, nil, &s)
		cancel()
		if err != nil {
			return err
		}
	}
}

func report(phone string, r *wire.Result) (bool, error) {
	detail := ""
	if r.Detail != "" {
		detail = ": " + r.Detail
	}
	switch r.Status {
	case wire.Ready:
		fmt.Printf("  ready on %s, waiting for a tap\n", phone)
	case wire.Installing:
		fmt.Printf("  installing on %s\n", phone)
	case wire.Installed:
		fmt.Printf("  installed on %s\n", phone)
		return true, nil
	case wire.Superseded:
		return true, exit{3, "superseded by a newer push"}
	default:
		return true, exit{3, r.Status + " on " + phone + detail}
	}
	return false, nil
}

func pushedIPA(ctx context.Context, l local, s host.Summary, wait bool, timeout time.Duration) error {
	fmt.Printf("pushed %s · %s for %s\n", what(s), mb(s.File.Size), s.To)
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
	switch {
	case err != nil:
		return err
	case s.NotifyErr != "":
		fmt.Printf("  %s: %s\n", s.To, s.NotifyErr)
	case !s.Taken.IsZero() && s.IPA.Profile == nil:
		fmt.Printf("  %s was notified; it downloads it from Cloche\n", s.To)
	case !s.Taken.IsZero():
		fmt.Printf("  %s was notified; a tap installs it\n", s.To)
	default:
		fmt.Printf("  still notifying %s\n", s.To)
	}
	if !wait {
		return nil
	}
	deadline := time.Now().Add(timeout)
	seen := ""
	for {
		if s.Result != nil && s.Result.Status != seen {
			seen = s.Result.Status
			switch seen {
			case wire.Asked:
				fmt.Printf("  Install tapped: iOS is asking %s\n", s.To)
			case wire.Installing:
				fmt.Printf("  %s is downloading it\n", s.To)
			case wire.Downloaded:
				if s.IPA.Profile == nil {
					fmt.Printf("  %s downloaded it, for SideStore or AltStore\n", s.To)
				} else {
					fmt.Printf("  %s downloaded it; iOS installs it from there, and doesn't say when it's done\n", s.To)
				}
				return nil
			}
		}
		left := time.Until(deadline)
		if left <= 0 {
			return exit{4, fmt.Sprintf("%s didn't download it within %s", s.To, timeout)}
		}
		secs := strconv.Itoa(int(min(left, 5*time.Minute).Seconds()) + 1)
		tctx, cancel := context.WithTimeout(ctx, 6*time.Minute)
		err := l.do(tctx, "GET", "/deliveries/"+s.ID, url.Values{"until": {"change"}, "from": {seen}, "wait": {secs}}, nil, &s)
		cancel()
		if err != nil {
			return err
		}
	}
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
