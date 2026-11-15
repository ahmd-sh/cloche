package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
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

const adminDNS = "https://login.tailscale.com/admin/dns"

func setup(args []string) error {
	fs := flag.NewFlagSet("setup", flag.ExitOnError)
	name := fs.String("name", "", "the name phones show for this computer (default: its machine name; \"\" to go back to it)")
	fs.Parse(args)
	named := false
	fs.Visit(func(f *flag.Flag) { named = named || f.Name == "name" })
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
		notes = append(notes, "This computer is a tagged node. Tailscale names no owner for it, and only friends' phones can pair with it.")
	}

	stopService()
	if pid, ok := otherServe(ctx, 5*time.Second); ok {
		which := "A cloche serve you started yourself is still running"
		if pid > 0 {
			which += fmt.Sprintf(" (pid %d)", pid)
		}
		return exit{1, which + ". Stop it, then run cloche setup again."}
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
	if named {
		if err := l.do(ctx, "POST", "/display", nil, map[string]string{"name": *name}, nil); err != nil {
			return err
		}
		if err := l.do(ctx, "GET", "/status", nil, nil, &st); err != nil {
			return err
		}
	}

	who := self.Login
	if self.Tagged {
		who = "a tagged node"
	}
	fmt.Printf("tailscale  on, signed in as %s\n", who)
	fmt.Printf("service    running, and starts when you log in (%s)\n", svc.how)
	where, err := installSkill()
	for _, w := range where {
		fmt.Printf("skill      %s\n", w)
	}
	if err != nil {
		notes = append(notes, "The agent skill didn't install: "+err.Error())
	}
	if note := onPath(bin); note != "" {
		notes = append(notes, note)
	}
	if st.Display != "" {
		fmt.Printf("name       %s, as phones show it\n", st.Display)
	}
	for _, p := range st.Phones {
		if p.Friend != "" {
			fmt.Printf("phone      %s, %s's\n", p.Name, p.Friend)
		} else {
			fmt.Printf("phone      %s\n", p.Name)
		}
	}
	for _, ip := range st.IPhones {
		if ip.Friend != "" {
			fmt.Printf("iphone     %s, %s's\n", ip.Name, ip.Friend)
		} else {
			fmt.Printf("iphone     %s\n", ip.Name)
		}
	}
	for _, n := range notes {
		fmt.Printf("\n%s\n", n)
	}
	if len(st.Phones) > 0 || len(st.IPhones) > 0 {
		return nil
	}

	signIn := ""
	var in io.Reader
	if !self.Tagged {
		signIn = ", signed in as " + self.Login
		if f := console(); f != nil {
			defer f.Close()
			in = f
		}
	}
	fmt.Printf("\nNext, pair your phone. It needs Tailscale%s.\n\n", signIn)
	return offerPairing(in, os.Stdout, func(ios bool) error {
		if ios {
			return pairIPhone(ctx, l, 15*time.Minute, "")
		}
		return pairAndroid(ctx, l, 15*time.Minute, "")
	})
}

func console() *os.File {
	if terminal(os.Stdin) {
		return os.Stdin
	}
	if !terminal(os.Stdout) {
		return nil
	}
	f, err := os.Open("/dev/tty")
	if err != nil {
		return nil
	}
	if !terminal(f) {
		f.Close()
		return nil
	}
	return f
}

func offerPairing(in io.Reader, out io.Writer, pair func(ios bool) error) error {
	if in != nil {
		if ios, err := chooseKind(in, out); err == nil {
			fmt.Fprintln(out)
			return pair(ios)
		}
	}
	fmt.Fprint(out, `  cloche pair -android   an Android phone: scan the code with its camera,
                         and the page it opens helps install Cloche
  cloche pair -ios       an iPhone
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

type skillHome struct {
	dir, agents string
	used        bool
}

func skillHomes() ([]skillHome, error) {
	h, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	config := os.Getenv("XDG_CONFIG_HOME")
	if config == "" {
		config = filepath.Join(h, ".config")
	}
	used := func(dirs ...string) bool {
		for _, d := range dirs {
			if st, err := os.Stat(d); d != "" && err == nil && st.IsDir() {
				return true
			}
		}
		return false
	}
	return []skillHome{
		{filepath.Join(h, ".claude", "skills", "cloche"), "Claude Code", used(filepath.Join(h, ".claude"))},
		{filepath.Join(h, ".agents", "skills", "cloche"), "Codex, OpenCode and Pi", used(
			filepath.Join(h, ".agents"), filepath.Join(h, ".codex"), os.Getenv("CODEX_HOME"),
			filepath.Join(h, ".pi"), filepath.Join(config, "opencode"))},
	}, nil
}

func installSkill() ([]string, error) {
	homes, err := skillHomes()
	if err != nil {
		return nil, err
	}
	var done []string
	for _, s := range homes {
		if !s.used {
			continue
		}
		if fi, err := os.Lstat(s.dir); err == nil && fi.Mode()&os.ModeSymlink != 0 {
			done = append(done, tilde(s.dir)+", a link, left as it is")
			continue
		}
		if err := os.MkdirAll(s.dir, 0o755); err != nil {
			return done, err
		}
		if err := os.WriteFile(filepath.Join(s.dir, "SKILL.md"), skill.File, 0o644); err != nil {
			return done, err
		}
		done = append(done, tilde(s.dir)+", for "+s.agents)
	}
	return done, nil
}

func removeSkill(dir string) (bool, error) {
	fi, err := os.Lstat(dir)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		return true, os.Remove(dir)
	}
	if err := os.Remove(filepath.Join(dir, "SKILL.md")); err != nil && !os.IsNotExist(err) {
		return false, err
	}
	os.Remove(dir)
	return true, nil
}

func uninstall(args []string) error {
	fs := flag.NewFlagSet("uninstall", flag.ExitOnError)
	fs.Parse(args)
	how, err := removeService()
	if err != nil {
		return err
	}
	fmt.Printf("service    removed from %s: cloche serve is stopped\n", how)
	homes, err := skillHomes()
	if err != nil {
		fmt.Printf("skill      not removed: %v\n", err)
	}
	for _, s := range homes {
		if gone, err := removeSkill(s.dir); err != nil {
			fmt.Printf("skill      still at %s: %v\n", tilde(s.dir), err)
		} else if gone {
			fmt.Printf("skill      removed from %s\n", tilde(s.dir))
		}
	}
	fmt.Printf("\nPairings and builds stay in %s. Delete it to forget them.\n", tilde(home()))
	return nil
}

func otherServe(ctx context.Context, wait time.Duration) (pid int, running bool) {
	l := dial()
	deadline := time.Now().Add(wait)
	for {
		var st host.Status
		err := l.do(ctx, "GET", "/status", nil, nil, &st)
		if errors.Is(err, errNotRunning) {
			return 0, false
		}
		if time.Now().After(deadline) {
			return st.PID, true
		}
		time.Sleep(200 * time.Millisecond)
	}
}
