package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"path/filepath"
	"testing"
	"time"
)

func TestDevCAOnlyLoopback(t *testing.T) {
	dir := t.TempDir()
	ca, key, err := loadOrMakeCA(filepath.Join(dir, "ca.pem"), filepath.Join(dir, "ca.key"))
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(ca)
	leaf := func(dns string, ip net.IP) *x509.Certificate {
		k, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		tmpl := &x509.Certificate{
			SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: dns},
			NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
			ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		}
		if dns != "" {
			tmpl.DNSNames = []string{dns}
		}
		if ip != nil {
			tmpl.IPAddresses = []net.IP{ip}
		}
		der, err := x509.CreateCertificate(rand.Reader, tmpl, ca, &k.PublicKey, key)
		if err != nil {
			t.Fatal(err)
		}
		c, _ := x509.ParseCertificate(der)
		return c
	}
	verify := func(c *x509.Certificate, name string) error {
		_, err := c.Verify(x509.VerifyOptions{Roots: roots, DNSName: name})
		return err
	}
	if err := verify(leaf("localhost", net.ParseIP("127.0.0.1")), "localhost"); err != nil {
		t.Errorf("localhost: %v", err)
	}
	if err := verify(leaf("", net.ParseIP("127.0.0.1")), "127.0.0.1"); err != nil {
		t.Errorf("127.0.0.1: %v", err)
	}
	if err := verify(leaf("bank.example", nil), "bank.example"); err == nil {
		t.Error("the dev CA vouched for bank.example")
	}
	if err := verify(leaf("", net.ParseIP("100.64.0.1")), "100.64.0.1"); err == nil {
		t.Error("the dev CA vouched for a tailnet address")
	}

	again, _, err := loadOrMakeCA(filepath.Join(dir, "ca.pem"), filepath.Join(dir, "ca.key"))
	if err != nil || !again.Equal(ca) {
		t.Errorf("not loaded again: %v", err)
	}
}
