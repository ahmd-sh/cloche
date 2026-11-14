package webpush

import (
	"context"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func dec(s string) []byte {
	b, err := base64.RawURLEncoding.DecodeString(strings.Join(strings.Fields(s), ""))
	if err != nil {
		panic(err)
	}
	return b
}

func TestRFC8291(t *testing.T) {
	as, err := ecdh.P256().NewPrivateKey(dec("yfWPiYE-n46HLnH0KqZOF1fJJU3MYrct3AELtAQ-oRw"))
	if err != nil {
		t.Fatal(err)
	}
	var sub Subscription
	sub.Keys.P256dh = "BCVxsr7N_eNgVRqvHtD0zTZsEc6-VV-JvLexhqUzORcxaOzi6-AYWXvTBHm4bjyPjs7Vd8pZGH6SRpkNtoIAiw4"
	sub.Keys.Auth = "BTBZMqHH6r4Tts7J_aSIgg"
	got, err := encrypt(sub, []byte("When I grow up, I want to be a watermelon"), as, dec("DGv6ra1nlYgDCS1FRnbzlw"))
	if err != nil {
		t.Fatal(err)
	}
	want := `DGv6ra1nlYgDCS1FRnbzlwAAEABBBP4z9KsN6nGRTbVYI_c7VJSPQTBtkgcy27ml
	mlMoZIIgDll6e3vCYLocInmYWAmS6TlzAC8wEqKK6PBru3jl7A_yl95bQpu6cVPT
	pK4Mqgkf1CXztLVBSt2Ks3oZwbuwXPXLWyouBWLVWGNWQexSgSxsj_Qulcy4a-fN`
	if g := base64.RawURLEncoding.EncodeToString(got); g != strings.Join(strings.Fields(want), "") {
		t.Errorf("got  %s", g)
	}
}

func TestSend(t *testing.T) {
	allow := AllowHost
	AllowHost = func(string) bool { return true }
	defer func() { AllowHost = allow }()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	s := &Sender{Key: key, Subject: "https://mac.example.ts.net"}
	ua, _ := ecdh.P256().GenerateKey(rand.Reader)
	auth := make([]byte, 16)
	rand.Read(auth)

	var gotAuth string
	var gotHeaders http.Header
	var gotBody []byte
	status := http.StatusCreated
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth, gotHeaders = r.Header.Get("Authorization"), r.Header
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(status)
	}))
	defer srv.Close()
	s.HTTP = srv.Client()

	var sub Subscription
	sub.Endpoint = srv.URL + "/push/abc"
	sub.Keys.P256dh = base64.RawURLEncoding.EncodeToString(ua.PublicKey().Bytes())
	sub.Keys.Auth = base64.RawURLEncoding.EncodeToString(auth)
	if err := sub.Check(); err != nil {
		t.Fatal(err)
	}
	if err := s.Send(context.Background(), sub, Message{Payload: []byte(`{"title":"hi"}`), TTL: time.Hour, Urgency: "high"}); err != nil {
		t.Fatal(err)
	}
	if gotHeaders.Get("TTL") != "3600" || gotHeaders.Get("Content-Encoding") != "aes128gcm" || gotHeaders.Get("Urgency") != "high" {
		t.Errorf("headers: %v", gotHeaders)
	}
	if len(gotBody) != 86+len(`{"title":"hi"}`)+1+16 || string(gotBody[21:86]) == "" {
		t.Errorf("body is %d bytes: header, the payload and its delimiter, the tag", len(gotBody))
	}

	tok, k, _ := strings.Cut(strings.TrimPrefix(gotAuth, "vapid t="), ", k=")
	if k != s.PublicKey() {
		t.Errorf("k=%s", k)
	}
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		t.Fatalf("token %q", tok)
	}
	var claims struct {
		Aud string `json:"aud"`
		Exp int64  `json:"exp"`
		Sub string `json:"sub"`
	}
	json.Unmarshal(dec(parts[1]), &claims)
	if claims.Aud != srv.URL || claims.Sub != s.Subject || claims.Exp < time.Now().Unix() || claims.Exp > time.Now().Add(2*time.Hour).Unix() {
		t.Errorf("claims %+v", claims)
	}
	sig := dec(parts[2])
	d := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if len(sig) != 64 || !ecdsa.Verify(&key.PublicKey, d[:], new(big.Int).SetBytes(sig[:32]), new(big.Int).SetBytes(sig[32:])) {
		t.Error("token signature doesn't verify")
	}

	status = http.StatusGone
	if err := s.Send(context.Background(), sub, Message{Payload: []byte("x")}); err != ErrGone {
		t.Errorf("410: %v", err)
	}
	status = http.StatusForbidden
	if err := s.Send(context.Background(), sub, Message{Payload: []byte("x")}); err == nil || err == ErrGone {
		t.Errorf("403: %v", err)
	}
}

func TestCheck(t *testing.T) {
	ua, _ := ecdh.P256().GenerateKey(rand.Reader)
	var sub Subscription
	sub.Keys.P256dh = base64.RawURLEncoding.EncodeToString(ua.PublicKey().Bytes())
	sub.Keys.Auth = base64.RawURLEncoding.EncodeToString(make([]byte, 16))
	for _, c := range []struct {
		endpoint string
		ok       bool
	}{
		{"https://web.push.apple.com/QGx", true},
		{"https://web.push.apple.com:443/QGx", true},
		{"https://fcm.googleapis.com/fcm/send/x", true},
		{"https://updates.push.services.mozilla.com/wpush/v2/x", true},
		{"http://web.push.apple.com/QGx", false},
		{"https://web.push.apple.com:8443/QGx", false},
		{"https://push.apple.com.evil.example/x", false},
		{"https://evilpush.apple.com.example/x", false},
		{"https://192.168.1.1/admin", false},
		{"https://100.64.0.1:41778/v1/inbox", false},
		{"https://localhost/x", false},
		{"https://user@web.push.apple.com/x", false},
	} {
		sub.Endpoint = c.endpoint
		if err := sub.Check(); (err == nil) != c.ok {
			t.Errorf("%s: %v", c.endpoint, err)
		}
	}
}

func TestNoRedirects(t *testing.T) {
	allow := AllowHost
	AllowHost = func(string) bool { return true }
	defer func() { AllowHost = allow }()
	followed := false
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/inside" {
			followed = true
			w.Write([]byte("secret"))
			return
		}
		http.Redirect(w, r, "/inside", http.StatusFound)
	}))
	defer srv.Close()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	s := &Sender{Key: key, Subject: "https://mac.example", HTTP: srv.Client()}
	ua, _ := ecdh.P256().GenerateKey(rand.Reader)
	var sub Subscription
	sub.Endpoint = srv.URL + "/push"
	sub.Keys.P256dh = base64.RawURLEncoding.EncodeToString(ua.PublicKey().Bytes())
	sub.Keys.Auth = base64.RawURLEncoding.EncodeToString(make([]byte, 16))
	err := s.Send(context.Background(), sub, Message{Payload: []byte("x")})
	var refused *Refused
	if followed || !errors.As(err, &refused) || refused.Status != http.StatusFound {
		t.Errorf("followed=%v err=%v", followed, err)
	}
}
