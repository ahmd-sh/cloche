package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/ahmd-sh/cloche/internal/host"
	"github.com/ahmd-sh/cloche/internal/release"
)

const (
	noUpdateCheck = "CLOCHE_NO_UPDATE_CHECK"
	installPage   = "https://ahmd-sh.github.io/cloche"
	maxBinary     = 200 << 20
)

func archiveName(goos, goarch string) string {
	if goos == "windows" {
		return "cloche-windows-" + goarch + ".zip"
	}
	return "cloche-" + goos + "-" + goarch + ".tar.gz"
}

func update(args []string) error {
	fs := flag.NewFlagSet("update", flag.ExitOnError)
	fs.Parse(args)
	ctx := context.Background()

	cur := clocheVersion()
	if _, ok := release.Parse(cur); !ok {
		return exit{1, fmt.Sprintf("this cloche (%s) was built from source: build it again, or install a release from %s", cur, installPage)}
	}
	c := release.Client()
	m, err := release.Fetch(ctx, c, release.LatestURL())
	if err != nil {
		return err
	}
	if !release.Newer(m.Version, cur) {
		fmt.Printf("cloche %s is the latest\n", cur)
		return nil
	}
	name := archiveName(runtime.GOOS, runtime.GOARCH)
	sum, ok := m.Files[name]
	if !ok {
		return fmt.Errorf("cloche %s has no build for %s/%s", m.Version, runtime.GOOS, runtime.GOARCH)
	}
	bin, err := os.Executable()
	if err == nil {
		bin, err = filepath.EvalSymlinks(bin)
	}
	if err != nil {
		return err
	}

	fmt.Printf("Downloading cloche %s\n", m.Version)
	var archive bytes.Buffer
	if err := release.Download(ctx, c, release.URL(m.Version, name), sum, maxBinary, &archive); err != nil {
		return err
	}
	exe, err := extract(name, archive.Bytes())
	if err != nil {
		return err
	}
	if err := replace(bin, exe, m.Version); err != nil {
		return err
	}
	fmt.Printf("Updated %s from %s to %s\n", tilde(bin), cur, m.Version)

	if !serviceInstalled() {
		fmt.Println("Restart cloche serve to run the new version.")
		return nil
	}
	fmt.Println()
	cmd := exec.Command(bin, "setup")
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return exit{1, ""}
	}
	return nil
}

func extract(name string, b []byte) ([]byte, error) {
	if strings.HasSuffix(name, ".zip") {
		zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
		if err != nil {
			return nil, err
		}
		for _, f := range zr.File {
			if f.Name == "cloche.exe" && f.Mode().IsRegular() {
				r, err := f.Open()
				if err != nil {
					return nil, err
				}
				defer r.Close()
				return readAtMost(r, maxBinary)
			}
		}
		return nil, fmt.Errorf("%s has no cloche.exe", name)
	}
	gz, err := gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil, fmt.Errorf("%s has no cloche", name)
		}
		if err != nil {
			return nil, err
		}
		if hdr.Name == "cloche" && hdr.Typeflag == tar.TypeReg {
			return readAtMost(tr, maxBinary)
		}
	}
}

func readAtMost(r io.Reader, n int64) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(r, n+1))
	if err == nil && int64(len(b)) > n {
		err = errors.New("the binary is too big")
	}
	return b, err
}

func replace(bin string, exe []byte, version string) error {
	dir := filepath.Dir(bin)
	next := filepath.Join(dir, ".cloche.new")
	if runtime.GOOS == "windows" {
		next += ".exe"
	}
	if err := os.WriteFile(next, exe, 0o755); err != nil {
		if errors.Is(err, os.ErrPermission) {
			return exit{1, fmt.Sprintf("can't write to %s: update cloche the way you installed it, or as the user who owns that folder", dir)}
		}
		return err
	}
	out, err := exec.Command(next, "version").Output()
	if got := strings.TrimSpace(string(out)); err != nil || got != "cloche "+version {
		os.Remove(next)
		return fmt.Errorf("the downloaded cloche doesn't run here (%q, %v)", got, err)
	}
	if runtime.GOOS == "windows" {
		old := bin + ".old"
		os.Remove(old)
		if err := os.Rename(bin, old); err != nil {
			os.Remove(next)
			return err
		}
		if err := os.Rename(next, bin); err != nil {
			os.Rename(old, bin)
			return err
		}
		return nil
	}
	if err := os.Rename(next, bin); err != nil {
		os.Remove(next)
		return err
	}
	return nil
}

func watchReleases(ctx context.Context, h *host.Host, logger *log.Logger) {
	cur := clocheVersion()
	if _, ok := release.Parse(cur); !ok {
		return
	}
	if os.Getenv(noUpdateCheck) != "" {
		logger.Printf("%s is set: not checking for new releases", noUpdateCheck)
		return
	}
	c := release.Client()
	check := time.NewTimer(time.Minute)
	defer check.Stop()
	phone := time.NewTicker(time.Minute)
	defer phone.Stop()
	var tried time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case <-check.C:
			cctx, cancel := context.WithTimeout(ctx, time.Minute)
			m, err := release.Fetch(cctx, c, release.LatestURL())
			cancel()
			if err != nil {
				logger.Printf("checking for a new release: %v", err)
				check.Reset(6 * time.Hour)
				continue
			}
			if release.Newer(m.Version, cur) {
				logger.Printf("cloche %s is out: run cloche update", m.Version)
				h.SetUpdate(m.Version)
			} else {
				h.SetUpdate("")
			}
			check.Reset(24 * time.Hour)
		case <-phone.C:
			app, ok := h.PhoneApp()
			if !ok || app.Code == 0 || app.Code >= release.Code(cur) || app.Offered == cur || time.Since(tried) < time.Hour {
				continue
			}
			tried = time.Now()
			if err := sendApp(ctx, c, h, cur); err != nil {
				logger.Printf("sending Cloche %s to the phone: %v", cur, err)
				continue
			}
			if err := h.SetAppOffered(cur); err != nil {
				logger.Printf("%v", err)
			}
		}
	}
}

func sendApp(ctx context.Context, c *http.Client, h *host.Host, version string) error {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Minute)
	defer cancel()
	m, err := release.Fetch(ctx, c, release.URL(version, "latest.json"))
	if err != nil {
		return err
	}
	if m.Version != version {
		return fmt.Errorf("the %s release says it's %s", version, m.Version)
	}
	dir, err := os.MkdirTemp(home(), "update-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "cloche.apk")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	err = release.Download(ctx, c, release.URL(version, "cloche.apk"), m.Files["cloche.apk"], host.MaxSize, f)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	_, err = h.Push(ctx, host.PushRequest{Path: path, Note: "Cloche " + version})
	return err
}
