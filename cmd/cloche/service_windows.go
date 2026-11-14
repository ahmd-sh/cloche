package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
)

const taskName = "cloche"

func schtasks(args ...string) error {
	out, err := exec.Command("schtasks", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("schtasks %s: %v %s", args[0], err, bytes.TrimSpace(out))
	}
	return nil
}

func installService(bin string, env map[string]string) (service, error) {
	u, err := user.Current()
	if err != nil {
		return service{}, err
	}
	if err := os.MkdirAll(home(), 0o700); err != nil {
		return service{}, err
	}
	xml := filepath.Join(home(), "task.xml")
	if err := os.WriteFile(xml, utf16File(taskXML(bin, u.Username)), 0o600); err != nil {
		return service{}, err
	}
	defer os.Remove(xml)
	schtasks("/End", "/TN", taskName)
	if err := schtasks("/Create", "/TN", taskName, "/XML", xml, "/F"); err != nil {
		return service{}, err
	}
	if err := schtasks("/Run", "/TN", taskName); err != nil {
		return service{}, err
	}
	s := service{how: "Task Scheduler, logs in " + filepath.Join(home(), "serve.log")}
	if len(env) > 0 {
		s.notes = append(s.notes, "The task runs with your user's environment variables. If cloche serve needs CLOCHE_HOME or ANDROID_HOME, set them for your user.")
	}
	return s, nil
}

func removeService() (string, error) {
	schtasks("/End", "/TN", taskName)
	if err := schtasks("/Delete", "/TN", taskName, "/F"); err != nil {
		return "", err
	}
	return "Task Scheduler", nil
}
