package main

import (
	"context"
	"net"
	"net/http"
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
