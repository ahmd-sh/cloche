package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/ahmd-sh/cloche/internal/host"
	"github.com/ahmd-sh/cloche/internal/tailnet"
	"github.com/ahmd-sh/cloche/skill"
)

const appURL = "https://github.com/ahmd-sh/cloche/releases/latest/download/cloche.apk"

const adminDNS = "https://login.tailscale.com/admin/dns"

func setup(args []string) error {
	fs := flag.NewFlagSet("setup", flag.ExitOnError)
	fs.Parse(args)
	ctx := context.Background()

	bin, err := os.Executable()
	if err != nil {
		return err
	}
	if strings.Contains(bin, "go-build") {
		return errors.New("cloche setup needs an installed cloche: go install ./cmd/cloche, then run it from there")
	}

	ts, err := tailnet.Find()
	if err != nil {
		return exit{1, err.Error() + "\nSign in to it, then run cloche setup again."}
	}
	health, err := ts.Health(ctx)
	if err != nil {
		return exit{1, "Tailscale isn't running: start it, then run cloche setup again.\n" + err.Error()}
	}
	switch health.State {
	case "Running":
	case "NeedsLogin", "NeedsMachineAuth":
		return exit{1, "Tailscale needs you to sign in: run tailscale up, or open the Tailscale app. Then run cloche setup again."}
	default:
		return exit{1, fmt.Sprintf("Tailscale is %s: turn it on with tailscale up, or from the Tailscale app. Then run cloche setup again.", strings.ToLower(health.State))}
	}
	_, self, err := ts.Self(ctx)
	if err != nil {
		return err
	}

	var notes []string
	switch {
	case health.DNSName == "":
		notes = append(notes, "iPhones need MagicDNS and HTTPS certificates: turn both on at "+adminDNS)
	case !health.HTTPS:
		notes = append(notes, "iPhones need HTTPS certificates: turn them on at "+adminDNS)
	case runtime.GOOS == "linux" && os.Getuid() != 0:
		if u, err := user.Current(); err == nil {
			if op, err := ts.Operator(ctx); err == nil && op != u.Username {
				notes = append(notes, "iPhones need this computer's HTTPS certificate, and Tailscale gives it to its operator alone: sudo tailscale set --operator=$USER")
			}
		}
	}
	if health.Tagged {
		notes = append(notes, "This computer is a tagged node. Tailscale names no owner for it, and only friends' iPhones can pair with it.")
	}

	svc, err := installService(bin, serviceEnv())
	if err != nil {
		return err
	}
	notes = append(notes, svc.notes...)

	l := dial()
	var st host.Status
	deadline := time.Now().Add(20 * time.Second)
	for {
		err = l.do(ctx, "GET", "/status", nil, nil, &st)
		if err == nil || time.Now().After(deadline) {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if err != nil {
		return exit{1, "cloche serve didn't start. Its logs say why: " + svc.how}
	}

	who := self.Login
	if self.Tagged {
		who = "a tagged node"
	}
	fmt.Printf("tailscale  on, signed in as %s\n", who)
	fmt.Printf("service    running, and starts when you log in (%s)\n", svc.how)
	if where, err := installSkill(); err != nil {
		notes = append(notes, "The agent skill didn't install: "+err.Error())
	} else if where != "" {
		fmt.Printf("skill      %s\n", where)
	}
	if note := onPath(bin); note != "" {
		notes = append(notes, note)
	}
	if st.Phone != nil {
		fmt.Printf("phone      %s\n", st.Phone.Name)
	}
	for _, ip := range st.IPhones {
		fmt.Printf("iphone     %s\n", ip.Name)
	}
	for _, n := range notes {
		fmt.Printf("\n%s\n", n)
	}
	if st.Phone != nil || len(st.IPhones) > 0 {
		return nil
	}

	signIn := "sign in"
	if !self.Tagged {
		signIn = "sign in as " + self.Login
	}
	fmt.Printf(`
Next, on your Android phone:

  1. Install Tailscale from the Play Store and %s.
  2. Install Cloche. Scan this with the phone's camera, or open
     %s

`, signIn, appURL)
	if err := printQR(os.Stdout, appURL); err != nil {
		return err
	}
	fmt.Print(`
  3. Run cloche pair here, and scan its code with the phone's camera.

For an iPhone, run cloche pair -ios.
`)
	return nil
}

func onPath(bin string) string {
	found, err := exec.LookPath("cloche")
	if err != nil {
		return fmt.Sprintf("cloche isn't on your PATH. Add %s to it, for you and your agents.", filepath.Dir(bin))
	}
	a, errA := filepath.EvalSymlinks(found)
	b, errB := filepath.EvalSymlinks(bin)
	if errA == nil && errB == nil && a != b {
		return fmt.Sprintf("The cloche on your PATH is %s, and the service runs %s.", found, bin)
	}
	return ""
}

func skillDir() (string, error) {
	h, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	claude := filepath.Join(h, ".claude")
	if st, err := os.Stat(claude); err != nil || !st.IsDir() {
		return "", nil
	}
	return filepath.Join(claude, "skills", "cloche"), nil
}

func installSkill() (string, error) {
	dir, err := skillDir()
	if err != nil || dir == "" {
		return "", err
	}
	if fi, err := os.Lstat(dir); err == nil && fi.Mode()&os.ModeSymlink != 0 {
		return tilde(dir) + ", a link, left as it is", nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), skill.File, 0o644); err != nil {
		return "", err
	}
	return tilde(dir) + ", for Claude Code", nil
}

func removeSkill() (string, error) {
	dir, err := skillDir()
	if err != nil || dir == "" {
		return "", err
	}
	fi, err := os.Lstat(dir)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		return tilde(dir), os.Remove(dir)
	}
	if err := os.Remove(filepath.Join(dir, "SKILL.md")); err != nil && !os.IsNotExist(err) {
		return "", err
	}
	os.Remove(dir)
	return tilde(dir), nil
}

func uninstall(args []string) error {
	fs := flag.NewFlagSet("uninstall", flag.ExitOnError)
	fs.Parse(args)
	how, err := removeService()
	if err != nil {
		return err
	}
	fmt.Printf("service    removed from %s: cloche serve is stopped\n", how)
	if where, err := removeSkill(); err != nil {
		fmt.Printf("skill      still at %s: %v\n", where, err)
	} else if where != "" {
		fmt.Printf("skill      removed from %s\n", where)
	}
	fmt.Printf("\nPairings and builds stay in %s. Delete it to forget them.\n", tilde(home()))
	return nil
}
