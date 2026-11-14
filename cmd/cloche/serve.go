package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net"
	"net/http"
	"net/netip"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/ahmd-sh/cloche/internal/host"
	"github.com/ahmd-sh/cloche/internal/tailnet"
	"github.com/ahmd-sh/cloche/internal/wire"
)

func serve(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	listen := fs.String("listen", "", "address for the phone (default: this computer's tailnet IPv4, port "+strconv.Itoa(wire.Port)+")")
	dev := fs.Bool("dev", false, "listen on loopback with no Tailscale checks, for an emulator through adb reverse")
	fs.Parse(args)

	background()
	logger := log.New(os.Stderr, "", log.LstdFlags)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	sock := socketPath()
	if c, err := net.Dial("unix", sock); err == nil {
		c.Close()
		return errors.New("cloche is already running")
	}

	var (
		peers   tailnet.Whois
		addr    string
		name    string
		owner   string
		ts      *tailnet.CLI
		dnsName string
		webAddr string
		webOff  string
	)
	if *dev {
		peers, addr = tailnet.Anyone{}, *listen
		if addr == "" {
			addr = net.JoinHostPort("127.0.0.1", strconv.Itoa(wire.Port))
		}
		if ap, err := netip.ParseAddrPort(addr); err != nil || !ap.Addr().IsLoopback() {
			return errors.New("-dev listens on loopback only")
		}
		name, _ = os.Hostname()
		name = strings.TrimSuffix(name, ".local")
		dnsName, webAddr = "localhost", net.JoinHostPort("127.0.0.1", strconv.Itoa(host.WebPort))
	} else {
		var err error
		ts, err = tailnet.Find()
		if err != nil {
			return err
		}
		peers = ts
		for {
			ip, self, err := ts.Self(ctx)
			if err == nil {
				name, owner = self.Name, self.Login
				if self.Tagged {
					owner = ""
					logger.Printf("this computer is a tagged node, so Tailscale doesn't say whose it is: only friends' iPhones can pair")
				}
				addr = *listen
				if addr == "" {
					addr = net.JoinHostPort(ip.String(), strconv.Itoa(wire.Port))
				}
				webAddr = net.JoinHostPort(ip.String(), strconv.Itoa(host.WebPort))
				if dnsName, _, err = ts.DNSName(ctx); err != nil {
					logger.Printf("no web app for iPhones: %v", err)
					webOff = err.Error() + " (restart cloche once it's fixed)"
				}
				break
			}
			logger.Printf("waiting for Tailscale: %v", err)
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(10 * time.Second):
			}
		}
	}

	var webCerts *certs
	web := ""
	if dnsName != "" {
		web = "https://" + net.JoinHostPort(dnsName, strconv.Itoa(host.WebPort))
	}
	h, err := host.Open(host.Config{
		Dir: home(), Name: name, Addr: addr, Peers: peers, Owner: owner, AnyOwner: *dev, DNSName: dnsName, Log: logger,
		Web: web,
		SignProfile: func(b []byte) ([]byte, error) {
			if webCerts == nil {
				return nil, errors.New("no certificate")
			}
			return webCerts.sign(b)
		},
	})
	if err != nil {
		return err
	}
	defer h.Close()

	var (
		webMu  sync.Mutex
		webSrv *http.Server
		once   sync.Once
	)
	state := &webState{h: h}
	start := func() {
		once.Do(func() {
			webMu.Lock()
			defer webMu.Unlock()
			webSrv = serveWeb(ctx, webAddr, webCerts, h, state, logger)
		})
	}
	switch {
	case *dev:
		c, ca, err := devCerts(home())
		if err != nil {
			return err
		}
		webCerts = c
		start()
		logger.Printf("development: the web app is %s; trust %s in a simulator with xcrun simctl keychain booted add-root-cert", web, ca)
	case web != "":
		webCerts = tailnetCerts(dnsName, home())
		if webCerts.valid() {
			start()
		} else {
			state.setCert("getting its certificate from Tailscale")
		}
		webCerts.keep(ctx, ts, dnsName, state, logger, start)
	case webOff != "":
		h.SetWebErr(webOff)
	}

	os.Remove(sock)
	restore := private()
	ll, err := net.Listen("unix", sock)
	restore()
	if err != nil {
		return err
	}
	os.Chmod(sock, 0o600)
	localSrv := &http.Server{Handler: h.LocalAPI()}
	go localSrv.Serve(ll)

	var pl net.Listener
	for {
		pl, err = net.Listen("tcp", addr)
		if err == nil {
			break
		}
		logger.Printf("can't listen on %s yet: %v", addr, err)
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(10 * time.Second):
		}
	}
	phoneSrv := &http.Server{
		Handler:           h.PhoneAPI(),
		ReadHeaderTimeout: 15 * time.Second,
		IdleTimeout:       host.MaxWait + time.Minute,
		ErrorLog:          logger,
	}
	go phoneSrv.Serve(pl)
	logger.Printf("cloche on %s for the phone, key %s", addr, h.Fingerprint())
	if *dev {
		logger.Printf("development: anyone on loopback is the phone (adb reverse tcp:%d tcp:%d)", wire.Port, wire.Port)
	}

	<-ctx.Done()
	logger.Printf("stopping")
	h.Close()
	sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	phoneSrv.Shutdown(sctx)
	localSrv.Shutdown(sctx)
	webMu.Lock()
	if webSrv != nil {
		webSrv.Shutdown(sctx)
	}
	webMu.Unlock()
	os.Remove(sock)
	return nil
}
