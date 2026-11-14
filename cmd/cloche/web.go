package main

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"log"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ahmd-sh/cloche/internal/cms"
	"github.com/ahmd-sh/cloche/internal/host"
	"github.com/ahmd-sh/cloche/internal/tailnet"
)

type certs struct {
	certFile, keyFile string
	cur               atomic.Pointer[tls.Certificate]
}

func (c *certs) load() error {
	pair, err := tls.LoadX509KeyPair(c.certFile, c.keyFile)
	if err != nil {
		return err
	}
	c.cur.Store(&pair)
	return nil
}

func (c *certs) get(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	if p := c.cur.Load(); p != nil {
		return p, nil
	}
	return nil, errors.New("no certificate yet")
}

func (c *certs) valid() bool {
	p := c.cur.Load()
	if p == nil || len(p.Certificate) == 0 {
		return false
	}
	leaf, err := x509.ParseCertificate(p.Certificate[0])
	return err == nil && time.Until(leaf.NotAfter) > 24*time.Hour
}

func (c *certs) sign(b []byte) ([]byte, error) {
	p := c.cur.Load()
	if p == nil {
		return nil, errors.New("no certificate yet")
	}
	key, ok := p.PrivateKey.(crypto.Signer)
	if !ok {
		return nil, errors.New("the certificate's key can't sign")
	}
	return cms.Sign(b, p.Certificate, key, time.Now())
}

func tailnetCerts(name, dir string) *certs {
	os.MkdirAll(filepath.Join(dir, "tls"), 0o700)
	c := &certs{certFile: filepath.Join(dir, "tls", name+".crt"), keyFile: filepath.Join(dir, "tls", name+".key")}
	c.load()
	return c
}

type webState struct {
	h            *host.Host
	mu           sync.Mutex
	cert, listen string
}

func (s *webState) setCert(msg string)   { s.set(&s.cert, msg) }
func (s *webState) setListen(msg string) { s.set(&s.listen, msg) }

func (s *webState) set(field *string, msg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	*field = msg
	if s.cert != "" {
		s.h.SetWebErr(s.cert)
	} else {
		s.h.SetWebErr(s.listen)
	}
}

func (c *certs) keep(ctx context.Context, ts *tailnet.CLI, name string, state *webState, logger *log.Logger, ready func()) {
	go func() {
		for {
			wait := 24 * time.Hour
			_, https, err := ts.DNSName(ctx)
			switch {
			case err != nil:
			case !https:
				err = errors.New("HTTPS certificates are off for this tailnet: turn them on at https://login.tailscale.com/admin/dns")
			default:
				if err = ts.Cert(ctx, name, c.certFile, c.keyFile); err == nil {
					err = c.load()
				}
			}
			if err != nil {
				logger.Printf("web app certificate: %v", err)
				wait = 10 * time.Minute
			}
			if c.valid() {
				state.setCert("")
				ready()
			} else if err != nil {
				state.setCert(err.Error())
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(wait):
			}
		}
	}()
}

func devCerts(dir string) (*certs, string, error) {
	caFile := filepath.Join(dir, "dev-ca.pem")
	caKeyFile := filepath.Join(dir, "dev-ca-key.pem")
	ca, caKey, err := loadOrMakeCA(caFile, caKeyFile)
	if err != nil {
		return nil, "", err
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, "", err
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: "localhost"},
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(300 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca, &key.PublicKey, caKey)
	if err != nil {
		return nil, "", err
	}
	kder, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, "", err
	}
	os.MkdirAll(filepath.Join(dir, "tls"), 0o700)
	c := &certs{certFile: filepath.Join(dir, "tls", "localhost.crt"), keyFile: filepath.Join(dir, "tls", "localhost.key")}
	chain := append(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ca.Raw})...)
	if err := os.WriteFile(c.certFile, chain, 0o600); err != nil {
		return nil, "", err
	}
	if err := os.WriteFile(c.keyFile, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: kder}), 0o600); err != nil {
		return nil, "", err
	}
	return c, caFile, c.load()
}

func loadOrMakeCA(certFile, keyFile string) (*x509.Certificate, *ecdsa.PrivateKey, error) {
	if cb, err := os.ReadFile(certFile); err == nil {
		kb, err := os.ReadFile(keyFile)
		if err != nil {
			return nil, nil, err
		}
		cblock, _ := pem.Decode(cb)
		kblock, _ := pem.Decode(kb)
		if cblock == nil || kblock == nil {
			return nil, nil, errors.New("dev CA: bad PEM")
		}
		ca, err := x509.ParseCertificate(cblock.Bytes)
		if err != nil {
			return nil, nil, err
		}
		if ca.PermittedDNSDomainsCritical {
			k, err := x509.ParsePKCS8PrivateKey(kblock.Bytes)
			if err != nil {
				return nil, nil, err
			}
			ek, ok := k.(*ecdsa.PrivateKey)
			if !ok {
				return nil, nil, errors.New("dev CA: not an EC key")
			}
			return ca, ek, nil
		}
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	tmpl := &x509.Certificate{
		SerialNumber:                big.NewInt(1),
		Subject:                     pkix.Name{CommonName: "cloche dev CA"},
		NotBefore:                   time.Now().Add(-time.Hour),
		NotAfter:                    time.Now().Add(10 * 365 * 24 * time.Hour),
		IsCA:                        true,
		BasicConstraintsValid:       true,
		KeyUsage:                    x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		PermittedDNSDomainsCritical: true,
		PermittedDNSDomains:         []string{"localhost"},
		PermittedIPRanges: []*net.IPNet{
			{IP: net.IPv4(127, 0, 0, 0).To4(), Mask: net.CIDRMask(8, 32)},
			{IP: net.IPv6loopback, Mask: net.CIDRMask(128, 128)},
		},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, nil, err
	}
	kder, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, nil, err
	}
	if err := os.WriteFile(certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		return nil, nil, err
	}
	if err := os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: kder}), 0o600); err != nil {
		return nil, nil, err
	}
	ca, err := x509.ParseCertificate(der)
	return ca, key, err
}

func serveWeb(ctx context.Context, addr string, c *certs, h *host.Host, state *webState, logger *log.Logger) *http.Server {
	srv := &http.Server{
		Handler:           h.WebApp(),
		TLSConfig:         &tls.Config{GetCertificate: c.get, MinVersion: tls.VersionTLS12},
		ReadHeaderTimeout: 15 * time.Second,
		IdleTimeout:       2 * time.Minute,
		ErrorLog:          log.New(filteredLog{logger}, "", 0),
	}
	go func() {
		for {
			l, err := net.Listen("tcp", addr)
			if err == nil {
				logger.Printf("web app for iPhones on %s", addr)
				state.setListen("")
				err = srv.ServeTLS(l, "", "")
				if errors.Is(err, http.ErrServerClosed) {
					return
				}
				err = fmt.Errorf("stopped serving on %s: %w", addr, err)
			} else {
				err = fmt.Errorf("can't listen on %s: %w", addr, err)
			}
			logger.Printf("web app: %v", err)
			state.setListen(err.Error())
			select {
			case <-ctx.Done():
				return
			case <-time.After(10 * time.Second):
			}
		}
	}()
	return srv
}

type filteredLog struct{ l *log.Logger }

func (f filteredLog) Write(b []byte) (int, error) {
	if bytes.Contains(b, []byte("TLS handshake error")) {
		return len(b), nil
	}
	f.l.Print(string(b))
	return len(b), nil
}
