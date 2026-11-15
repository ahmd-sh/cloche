package main

import (
	"context"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fakeServe(t *testing.T) *http.Server {
	t.Helper()
	ln, err := net.Listen("unix", socketPath())
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"pid":4242}`))
	})}
	go srv.Serve(ln)
	return srv
}

func TestOtherServe(t *testing.T) {
	t.Setenv("CLOCHE_HOME", t.TempDir())
	ctx := context.Background()

	if _, ok := otherServe(ctx, time.Second); ok {
		t.Fatal("found a cloche serve with nothing running")
	}

	srv := fakeServe(t)
	if pid, ok := otherServe(ctx, 300*time.Millisecond); !ok || pid != 4242 {
		t.Errorf("a cloche serve that stays: %d %v", pid, ok)
	}
	srv.Close()

	srv = fakeServe(t)
	time.AfterFunc(400*time.Millisecond, func() { srv.Close() })
	start := time.Now()
	if _, ok := otherServe(ctx, 5*time.Second); ok {
		t.Error("a cloche serve that stopped counted as running")
	}
	if time.Since(start) > 3*time.Second {
		t.Error("waited the whole time for a cloche serve that stopped")
	}
}

func TestSkillHomes(t *testing.T) {
	h := t.TempDir()
	t.Setenv("HOME", h)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("CODEX_HOME", "")
	claude := filepath.Join(h, ".claude", "skills", "cloche", "SKILL.md")
	agents := filepath.Join(h, ".agents", "skills", "cloche", "SKILL.md")
	exists := func(p string) bool { _, err := os.Stat(p); return err == nil }

	if done, err := installSkill(); err != nil || len(done) != 0 {
		t.Fatalf("with no agents: %v %v", done, err)
	}
	if exists(filepath.Join(h, ".agents")) || exists(filepath.Join(h, ".claude")) {
		t.Fatal("made a skills folder with no agent to read it")
	}

	for _, agent := range []string{".codex", ".pi", ".config/opencode"} {
		if err := os.MkdirAll(filepath.Join(h, agent), 0o755); err != nil {
			t.Fatal(err)
		}
		done, err := installSkill()
		if err != nil || len(done) != 1 || !strings.Contains(done[0], "Codex") || !exists(agents) || exists(claude) {
			t.Fatalf("with %s: %v %v", agent, done, err)
		}
		os.RemoveAll(filepath.Join(h, ".agents"))
		os.RemoveAll(filepath.Join(h, agent))
	}

	os.MkdirAll(filepath.Join(h, ".claude"), 0o755)
	os.MkdirAll(filepath.Join(h, ".codex"), 0o755)
	if done, err := installSkill(); err != nil || len(done) != 2 || !exists(claude) || !exists(agents) {
		t.Fatalf("with Claude Code and Codex: %v %v", done, err)
	}

	homes, err := skillHomes()
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range homes {
		if gone, err := removeSkill(s.dir); !gone || err != nil {
			t.Errorf("removing %s: %v %v", s.dir, gone, err)
		}
	}
	if exists(claude) || exists(agents) {
		t.Error("a skill is still there after uninstall")
	}
	if gone, err := removeSkill(homes[0].dir); gone || err != nil {
		t.Errorf("removing twice: %v %v", gone, err)
	}
}
