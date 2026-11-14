package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
)

func unitPath() (string, error) {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		h, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(h, ".config")
	}
	return filepath.Join(dir, "systemd", "user", "cloche.service"), nil
}

func systemctl(args ...string) error {
	cmd := exec.Command("systemctl", append([]string{"--user"}, args...)...)
	if os.Getenv("XDG_RUNTIME_DIR") == "" {
		run := filepath.Join("/run/user", strconv.Itoa(os.Getuid()))
		if _, err := os.Stat(filepath.Join(run, "bus")); err == nil {
			cmd.Env = append(os.Environ(), "XDG_RUNTIME_DIR="+run)
		}
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := string(bytes.TrimSpace(out))
		if msg == "" {
			msg = err.Error()
		}
		if strings.Contains(msg, "connect to bus") {
			msg += "\nsystemd isn't running for your user. Log in to this computer directly, or run sudo loginctl enable-linger $USER, then run cloche setup again."
		}
		return fmt.Errorf("systemctl --user %s: %s", strings.Join(args, " "), msg)
	}
	return nil
}

func installService(bin string, env map[string]string) (service, error) {
	if _, err := exec.LookPath("systemctl"); err != nil {
		return service{}, errors.New("systemd isn't here to keep cloche serve running: start cloche serve some other way, like your init system or tmux")
	}
	unit, err := unitPath()
	if err != nil {
		return service{}, err
	}
	os.MkdirAll(filepath.Dir(unit), 0o755)
	if err := os.WriteFile(unit, []byte(systemdUnit(bin, env)), 0o644); err != nil {
		return service{}, err
	}
	for _, args := range [][]string{{"daemon-reload"}, {"enable", "cloche.service"}, {"restart", "cloche.service"}} {
		if err := systemctl(args...); err != nil {
			return service{}, err
		}
	}
	s := service{how: "systemd, logs: journalctl --user -u cloche"}
	if u, err := user.Current(); err == nil {
		out, _ := exec.Command("loginctl", "show-user", u.Username, "--property=Linger").Output()
		if strings.TrimSpace(string(out)) == "Linger=no" {
			s.notes = append(s.notes, "cloche serve stops when you log out. To keep it running: loginctl enable-linger")
		}
	}
	return s, nil
}

func removeService() (string, error) {
	unit, err := unitPath()
	if err != nil {
		return "", err
	}
	systemctl("disable", "--now", "cloche.service")
	if err := os.Remove(unit); err != nil && !os.IsNotExist(err) {
		return "", err
	}
	systemctl("daemon-reload")
	return "systemd", nil
}

func serviceInstalled() bool {
	unit, err := unitPath()
	if err != nil {
		return false
	}
	_, err = os.Stat(unit)
	return err == nil
}
