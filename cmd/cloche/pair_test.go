package main

import (
	"io"
	"strings"
	"testing"
	"time"

	"github.com/ahmd-sh/cloche/internal/host"
	"github.com/ahmd-sh/cloche/internal/wire"
)

func TestOfferPairing(t *testing.T) {
	for _, c := range []struct {
		in       io.Reader
		pairs    string
		commands bool
	}{
		{nil, "", true},
		{strings.NewReader(""), "", true},
		{strings.NewReader("a\n"), "android", false},
		{strings.NewReader("x\niphone\n"), "iphone", false},
	} {
		var out strings.Builder
		pairs := ""
		err := offerPairing(c.in, &out, func(ios bool) error {
			pairs = map[bool]string{false: "android", true: "iphone"}[ios]
			return nil
		})
		if err != nil || pairs != c.pairs || strings.Contains(out.String(), "cloche pair -android") != c.commands {
			t.Errorf("in %v: paired %q, err %v, said %q", c.in, pairs, err, out.String())
		}
	}
}

func TestChooseKind(t *testing.T) {
	for in, want := range map[string]bool{
		"a\n":              false,
		"i\n":              true,
		"x\nAndroid\n":     false,
		"\n  iPhone  \n":   true,
		"ipad":             true,
		"what\nios\nlater": true,
	} {
		got, err := chooseKind(strings.NewReader(in), io.Discard)
		if err != nil || got != want {
			t.Errorf("chooseKind(%q) = %v, %v", in, got, err)
		}
	}
	if _, err := chooseKind(strings.NewReader("nope\n"), io.Discard); err == nil {
		t.Error("no answer picked a kind")
	}
}

func TestDeliveryState(t *testing.T) {
	now := time.Now()
	for _, c := range []struct {
		d    host.Summary
		want string
	}{
		{host.Summary{}, "waiting for a phone"},
		{host.Summary{For: []string{"sam"}}, "for sam, whose phones were unpaired"},
		{host.Summary{Phones: []host.Progress{{Phone: "pixel", Result: &wire.Result{Status: "installed"}}}}, "installed"},
		{host.Summary{Phones: []host.Progress{{Phone: "sam", Friend: "sam", Taken: now}}}, "sam on its way"},
		{host.Summary{Phones: []host.Progress{
			{Phone: "pixel", Result: &wire.Result{Status: "blocked", Detail: "older"}},
			{Phone: "tablet", Fetched: now},
		}}, "pixel blocked: older; tablet downloading"},
	} {
		if got := deliveryState(c.d); got != c.want {
			t.Errorf("deliveryState(%+v) = %q, want %q", c.d, got, c.want)
		}
	}
}
