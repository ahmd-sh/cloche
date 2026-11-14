package cms

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/asn1"
	"errors"
	"slices"
	"time"
)

var (
	oidData          = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 7, 1}
	oidSignedData    = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 7, 2}
	oidContentType   = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 3}
	oidMessageDigest = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 4}
	oidSigningTime   = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 5}
	oidSHA256        = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 2, 1}
	oidECDSASHA256   = asn1.ObjectIdentifier{1, 2, 840, 10045, 4, 3, 2}
	oidRSA           = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 1, 1}
)

func Sign(content []byte, chain [][]byte, key crypto.Signer, now time.Time) ([]byte, error) {
	if len(chain) == 0 {
		return nil, errors.New("cms: no certificate")
	}
	leaf, err := x509.ParseCertificate(chain[0])
	if err != nil {
		return nil, err
	}
	var sigAlg []byte
	switch pub := key.Public().(type) {
	case *ecdsa.PublicKey:
		if !pub.Equal(leaf.PublicKey) {
			return nil, errors.New("cms: the key isn't the certificate's")
		}
		sigAlg = seq(oid(oidECDSASHA256))
	case *rsa.PublicKey:
		if !pub.Equal(leaf.PublicKey) {
			return nil, errors.New("cms: the key isn't the certificate's")
		}
		sigAlg = seq(oid(oidRSA), null)
	default:
		return nil, errors.New("cms: the key is neither ECDSA nor RSA")
	}
	digestAlg := seq(oid(oidSHA256))

	sum := sha256.Sum256(content)
	signingTime, err := asn1.Marshal(now.UTC())
	if err != nil {
		return nil, err
	}
	attrs := [][]byte{
		seq(oid(oidContentType), set(oid(oidData))),
		seq(oid(oidSigningTime), set(signingTime)),
		seq(oid(oidMessageDigest), set(tlv(0x04, sum[:]))),
	}
	slices.SortFunc(attrs, bytes.Compare)
	signed := set(attrs...)
	h := sha256.Sum256(signed)
	sig, err := key.Sign(rand.Reader, h[:], crypto.SHA256)
	if err != nil {
		return nil, err
	}

	serial, err := asn1.Marshal(leaf.SerialNumber)
	if err != nil {
		return nil, err
	}
	signerInfo := seq(
		version1,
		seq(leaf.RawIssuer, serial),
		digestAlg,
		tlv(0xa0, attrs...),
		sigAlg,
		tlv(0x04, sig),
	)
	signedData := seq(
		version1,
		set(digestAlg),
		seq(oid(oidData), tlv(0xa0, tlv(0x04, content))),
		tlv(0xa0, chain...),
		set(signerInfo),
	)
	return seq(oid(oidSignedData), tlv(0xa0, signedData)), nil
}

var (
	null     = []byte{0x05, 0x00}
	version1 = []byte{0x02, 0x01, 0x01}
)

func oid(o asn1.ObjectIdentifier) []byte {
	b, err := asn1.Marshal(o)
	if err != nil {
		panic(err)
	}
	return b
}

func seq(parts ...[]byte) []byte { return tlv(0x30, parts...) }
func set(parts ...[]byte) []byte { return tlv(0x31, parts...) }

func tlv(tag byte, parts ...[]byte) []byte {
	n := 0
	for _, p := range parts {
		n += len(p)
	}
	b := []byte{tag}
	switch {
	case n < 0x80:
		b = append(b, byte(n))
	default:
		var l []byte
		for x := n; x > 0; x >>= 8 {
			l = append([]byte{byte(x)}, l...)
		}
		b = append(b, 0x80|byte(len(l)))
		b = append(b, l...)
	}
	for _, p := range parts {
		b = append(b, p...)
	}
	return b
}
