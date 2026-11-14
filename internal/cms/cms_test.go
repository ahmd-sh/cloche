package cms

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

type contentInfo struct {
	Type    asn1.ObjectIdentifier
	Content asn1.RawValue `asn1:"explicit,tag:0"`
}

type signedData struct {
	Version    int
	DigestAlgs asn1.RawValue
	Encap      struct {
		Type    asn1.ObjectIdentifier
		Content []byte `asn1:"explicit,tag:0"`
	}
	Certs       asn1.RawValue `asn1:"optional,tag:0"`
	SignerInfos []signerInfo  `asn1:"set"`
}

type signerInfo struct {
	Version int
	SID     struct {
		Issuer asn1.RawValue
		Serial *big.Int
	}
	DigestAlg asn1.RawValue
	Attrs     asn1.RawValue `asn1:"tag:0"`
	SigAlg    struct{ Algorithm asn1.ObjectIdentifier }
	Sig       []byte
}

type attribute struct {
	Type   asn1.ObjectIdentifier
	Values asn1.RawValue `asn1:"set"`
}

func chain(t *testing.T, leafKey crypto.Signer) ([][]byte, *x509.CertPool) {
	t.Helper()
	caKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	caTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, caKey.Public(), caKey)
	if err != nil {
		t.Fatal(err)
	}
	ca, _ := x509.ParseCertificate(caDER)
	serial := new(big.Int).SetBytes([]byte{0x80, 1, 2, 3, 4, 5, 6, 7, 8})
	leafTmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "mac.tail1234.ts.net"},
		DNSNames:     []string{"mac.tail1234.ts.net"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTmpl, ca, leafKey.Public(), caKey)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(ca)
	return [][]byte{leafDER, caDER}, roots
}

func TestSign(t *testing.T) {
	ec, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	rs, _ := rsa.GenerateKey(rand.Reader, 2048)
	content := []byte(`<?xml version="1.0"?><plist version="1.0"><dict><key>PayloadType</key><string>Profile Service</string></dict></plist>` + string(bytes.Repeat([]byte("x"), 300)))
	for name, key := range map[string]crypto.Signer{"ecdsa": ec, "rsa": rs} {
		t.Run(name, func(t *testing.T) {
			certs, roots := chain(t, key)
			out, err := Sign(content, certs, key, time.Now())
			if err != nil {
				t.Fatal(err)
			}
			check(t, out, content, certs, roots)
			openssl(t, out, content, certs)
		})
	}
}

func check(t *testing.T, out, content []byte, certs [][]byte, roots *x509.CertPool) {
	t.Helper()
	var ci contentInfo
	if rest, err := asn1.Unmarshal(out, &ci); err != nil || len(rest) > 0 {
		t.Fatalf("ContentInfo: %v, %d left", err, len(rest))
	}
	if !ci.Type.Equal(oidSignedData) {
		t.Fatalf("type %v", ci.Type)
	}
	var sd signedData
	if _, err := asn1.Unmarshal(ci.Content.Bytes, &sd); err != nil {
		t.Fatalf("SignedData: %v", err)
	}
	if !bytes.Equal(sd.Encap.Content, content) || !sd.Encap.Type.Equal(oidData) {
		t.Fatal("content isn't what was signed")
	}
	if !bytes.Equal(sd.Certs.Bytes, append(append([]byte{}, certs[0]...), certs[1]...)) {
		t.Error("certificates aren't the chain")
	}
	if len(sd.SignerInfos) != 1 {
		t.Fatalf("%d signers", len(sd.SignerInfos))
	}
	si := sd.SignerInfos[0]
	leaf, _ := x509.ParseCertificate(certs[0])
	if !bytes.Equal(si.SID.Issuer.FullBytes, leaf.RawIssuer) || si.SID.Serial.Cmp(leaf.SerialNumber) != 0 {
		t.Error("signer isn't the leaf")
	}
	if _, err := leaf.Verify(x509.VerifyOptions{Roots: roots, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageAny}}); err != nil {
		t.Error(err)
	}

	signed := set(si.Attrs.Bytes)
	var attrs []attribute
	if _, err := asn1.UnmarshalWithParams(signed, &attrs, "set"); err != nil {
		t.Fatalf("attributes: %v", err)
	}
	for i := 1; i < len(attrs); i++ {
		a, _ := asn1.Marshal(attrs[i-1])
		b, _ := asn1.Marshal(attrs[i])
		if bytes.Compare(a, b) > 0 {
			t.Error("attributes out of DER order")
		}
	}
	found := map[string]bool{}
	for _, a := range attrs {
		found[a.Type.String()] = true
		if a.Type.Equal(oidMessageDigest) {
			var d []byte
			asn1.Unmarshal(a.Values.Bytes, &d)
			sum := sha256.Sum256(content)
			if !bytes.Equal(d, sum[:]) {
				t.Error("messageDigest isn't the content's")
			}
		}
	}
	if len(found) != 3 || !found[oidContentType.String()] || !found[oidSigningTime.String()] || !found[oidMessageDigest.String()] {
		t.Errorf("attributes %v", found)
	}
	alg := x509.ECDSAWithSHA256
	if si.SigAlg.Algorithm.Equal(oidRSA) {
		alg = x509.SHA256WithRSA
	}
	if err := leaf.CheckSignature(alg, signed, si.Sig); err != nil {
		t.Errorf("signature: %v", err)
	}
}

func openssl(t *testing.T, out, content []byte, certs [][]byte) {
	t.Helper()
	bin, err := exec.LookPath("openssl")
	if err != nil {
		t.Log("no openssl to check against")
		return
	}
	f := filepath.Join(t.TempDir(), "signed.der")
	os.WriteFile(f, out, 0o600)
	got, err := exec.Command(bin, "smime", "-verify", "-inform", "der", "-in", f, "-noverify").Output()
	if err != nil {
		t.Fatalf("openssl smime -verify: %v", err)
	}
	if !bytes.Equal(got, content) {
		t.Error("openssl got other content")
	}
}

func TestSignRefuses(t *testing.T) {
	ec, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	other, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	certs, _ := chain(t, ec)
	if _, err := Sign([]byte("x"), certs, other, time.Now()); err == nil {
		t.Error("signed with a key that isn't the certificate's")
	}
	if _, err := Sign([]byte("x"), nil, ec, time.Now()); err == nil {
		t.Error("signed with no certificate")
	}
}
