package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"
)

func launchdPaths() (plist, logs string, err error) {
	h, err := os.UserHomeDir()
	if err != nil {
		return "", "", err
	}
	return filepath.Join(h, "Library", "LaunchAgents", serviceLabel+".plist"), filepath.Join(h, "Library", "Logs", "cloche.log"), nil
}

func launchdDomain() string { return "gui/" + strconv.Itoa(os.Getuid()) }

func installService(bin string, env map[string]string) (service, error) {
	plist, logs, err := launchdPaths()
	if err != nil {
		return service{}, err
	}
	os.MkdirAll(filepath.Dir(plist), 0o755)
	os.MkdirAll(filepath.Dir(logs), 0o755)
	if err := os.WriteFile(plist, []byte(launchdPlist(bin, logs, env)), 0o644); err != nil {
		return service{}, err
	}
	exec.Command("launchctl", "bootout", launchdDomain()+"/"+serviceLabel).Run()
	var out []byte
	for range 10 {
		out, err = exec.Command("launchctl", "bootstrap", launchdDomain(), plist).CombinedOutput()
		if err == nil {
			return service{how: "launchd, logs in " + tilde(logs)}, nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return service{}, fmt.Errorf("launchctl bootstrap: %v %s", err, bytes.TrimSpace(out))
}

func removeService() (string, error) {
	plist, _, err := launchdPaths()
	if err != nil {
		return "", err
	}
	exec.Command("launchctl", "bootout", launchdDomain()+"/"+serviceLabel).Run()
	if err := os.Remove(plist); err != nil && !os.IsNotExist(err) {
		return "", err
	}
	return "launchd", nil
}
