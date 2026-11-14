package webpush

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Subscription struct {
	Endpoint string `json:"endpoint"`
	Keys     struct {
		P256dh string `json:"p256dh"`
		Auth   string `json:"auth"`
	} `json:"keys"`
}

var AllowHost = func(hostport string) bool {
	host, port, err := net.SplitHostPort(hostport)
	if err != nil {
		host, port = hostport, ""
	}
	if port != "" && port != "443" {
		return false
	}
	for _, s := range []string{"push.apple.com", "fcm.googleapis.com", "push.services.mozilla.com"} {
		if host == s || strings.HasSuffix(host, "."+s) {
			return true
		}
	}
	return false
}

func (s Subscription) Check() error {
	u, err := url.Parse(s.Endpoint)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
		return errors.New("endpoint isn't an https URL")
	}
	if !AllowHost(u.Host) {
		return errors.New("endpoint isn't a push service's")
	}
	if _, err := uaKey(s.Keys.P256dh); err != nil {
		return err
	}
	if a, err := b64(s.Keys.Auth); err != nil || len(a) != 16 {
		return errors.New("auth isn't 16 bytes")
	}
	return nil
}

var ErrGone = errors.New("the subscription is gone")

type Sender struct {
	Key     *ecdsa.PrivateKey
	Subject string
	HTTP    *http.Client
}

func (s *Sender) PublicKey() string {
	pub, _ := s.Key.PublicKey.Bytes()
	return base64.RawURLEncoding.EncodeToString(pub)
}

type Message struct {
	Payload []byte
	TTL     time.Duration
	Urgency string
	Topic   string
}

func (s *Sender) Send(ctx context.Context, sub Subscription, m Message) error {
	if err := sub.Check(); err != nil {
		return err
	}
	body, err := encrypt(sub, m.Payload, nil, nil)
	if err != nil {
		return err
	}
	u, err := url.Parse(sub.Endpoint)
	if err != nil {
		return err
	}
	jwt, err := s.jwt(u.Scheme+"://"+u.Host, time.Now())
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", sub.Endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	ttl := max(m.TTL, 0)
	req.Header.Set("TTL", strconv.Itoa(int(ttl.Seconds())))
	req.Header.Set("Content-Encoding", "aes128gcm")
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Authorization", "vapid t="+jwt+", k="+s.PublicKey())
	if m.Urgency != "" {
		req.Header.Set("Urgency", m.Urgency)
	}
	if m.Topic != "" {
		req.Header.Set("Topic", m.Topic)
	}
	c := &http.Client{Timeout: 30 * time.Second}
	if s.HTTP != nil {
		*c = *s.HTTP
	}
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	res, err := c.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	msg, _ := io.ReadAll(io.LimitReader(res.Body, 2<<10))
	switch {
	case res.StatusCode == http.StatusNotFound || res.StatusCode == http.StatusGone:
		return ErrGone
	case res.StatusCode >= 300:
		return &Refused{Status: res.StatusCode, Body: string(bytes.TrimSpace(msg))}
	}
	return nil
}

type Refused struct {
	Status int
	Body   string
}

func (e *Refused) Error() string {
	return fmt.Sprintf("push service: %d %s %s", e.Status, http.StatusText(e.Status), e.Body)
}

func (s *Sender) jwt(aud string, now time.Time) (string, error) {
	enc := base64.RawURLEncoding
	head := enc.EncodeToString([]byte(`{"typ":"JWT","alg":"ES256"}`))
	claims, err := json.Marshal(map[string]any{"aud": aud, "exp": now.Add(time.Hour).Unix(), "sub": s.Subject})
	if err != nil {
		return "", err
	}
	signing := head + "." + enc.EncodeToString(claims)
	d := sha256.Sum256([]byte(signing))
	r, sv, err := ecdsa.Sign(rand.Reader, s.Key, d[:])
	if err != nil {
		return "", err
	}
	sig := make([]byte, 64)
	r.FillBytes(sig[:32])
	sv.FillBytes(sig[32:])
	return signing + "." + enc.EncodeToString(sig), nil
}

const recordSize = 4096

func encrypt(sub Subscription, plaintext []byte, as *ecdh.PrivateKey, salt []byte) ([]byte, error) {
	ua, err := uaKey(sub.Keys.P256dh)
	if err != nil {
		return nil, err
	}
	auth, err := b64(sub.Keys.Auth)
	if err != nil || len(auth) != 16 {
		return nil, errors.New("auth isn't 16 bytes")
	}
	if len(plaintext) > recordSize-17-1-100 {
		return nil, errors.New("payload too big for a push")
	}
	if as == nil {
		if as, err = ecdh.P256().GenerateKey(rand.Reader); err != nil {
			return nil, err
		}
	}
	if salt == nil {
		salt = make([]byte, 16)
		rand.Read(salt)
	}
	secret, err := as.ECDH(ua)
	if err != nil {
		return nil, err
	}
	asPub := as.PublicKey().Bytes()
	uaPub := ua.Bytes()

	prkKey, err := hkdf.Extract(sha256.New, secret, auth)
	if err != nil {
		return nil, err
	}
	keyInfo := "WebPush: info\x00" + string(uaPub) + string(asPub)
	ikm, err := hkdf.Expand(sha256.New, prkKey, keyInfo, 32)
	if err != nil {
		return nil, err
	}
	prk, err := hkdf.Extract(sha256.New, ikm, salt)
	if err != nil {
		return nil, err
	}
	cek, err := hkdf.Expand(sha256.New, prk, "Content-Encoding: aes128gcm\x00", 16)
	if err != nil {
		return nil, err
	}
	nonce, err := hkdf.Expand(sha256.New, prk, "Content-Encoding: nonce\x00", 12)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(cek)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	var out bytes.Buffer
	out.Write(salt)
	binary.Write(&out, binary.BigEndian, uint32(recordSize))
	out.WriteByte(byte(len(asPub)))
	out.Write(asPub)
	out.Write(gcm.Seal(nil, nonce, append(append([]byte{}, plaintext...), 2), nil))
	return out.Bytes(), nil
}

func uaKey(s string) (*ecdh.PublicKey, error) {
	b, err := b64(s)
	if err != nil {
		return nil, errors.New("p256dh isn't base64url")
	}
	k, err := ecdh.P256().NewPublicKey(b)
	if err != nil {
		return nil, errors.New("p256dh isn't a P-256 point")
	}
	return k, nil
}

func b64(s string) ([]byte, error) {
	if b, err := base64.RawURLEncoding.DecodeString(s); err == nil {
		return b, nil
	}
	return base64.URLEncoding.DecodeString(s)
}
