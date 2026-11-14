package main

import (
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"testing"
)

func TestTaskSchedulerAccepts(t *testing.T) {
	if os.Getenv("CI") == "" {
		t.Skip("checked in CI: it registers a task")
	}
	u, err := user.Current()
	if err != nil {
		t.Fatal(err)
	}
	f := filepath.Join(t.TempDir(), "task.xml")
	os.WriteFile(f, utf16File(taskXML(`C:\Windows\System32\whoami.exe`, u.Username)), 0o600)
	const name = "cloche-test"
	if out, err := exec.Command("schtasks", "/Create", "/TN", name, "/XML", f, "/F").CombinedOutput(); err != nil {
		t.Fatalf("schtasks /Create: %v %s", err, out)
	}
	defer exec.Command("schtasks", "/Delete", "/TN", name, "/F").Run()
	if out, err := exec.Command("schtasks", "/Query", "/TN", name, "/XML").CombinedOutput(); err != nil {
		t.Fatalf("schtasks /Query: %v %s", err, out)
	}
}
