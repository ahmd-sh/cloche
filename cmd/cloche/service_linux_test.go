package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestSystemdAccepts(t *testing.T) {
	analyze, err := exec.LookPath("systemd-analyze")
	if err != nil || os.Getenv("CI") == "" {
		t.Skip("checked in CI, where systemd is")
	}
	f := filepath.Join(t.TempDir(), "cloche.service")
	os.WriteFile(f, []byte(systemdUnit("/bin/true", map[string]string{"CLOCHE_HOME": "/tmp/x"})), 0o644)
	if out, err := exec.Command(analyze, "verify", f).CombinedOutput(); err != nil {
		t.Errorf("systemd-analyze verify: %v %s", err, out)
	}
}
