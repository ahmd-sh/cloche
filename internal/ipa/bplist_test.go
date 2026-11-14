package ipa

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestBinaryPlist(t *testing.T) {
	read := func(name string) any {
		t.Helper()
		b, err := os.ReadFile(filepath.Join("testdata", name))
		if err != nil {
			t.Fatal(err)
		}
		v, err := parsePlist(b)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		return v
	}
	xml, bin := read("rich.plist"), read("rich.bplist")
	if !reflect.DeepEqual(xml, bin) {
		t.Errorf("binary differs from XML:\nxml %#v\nbin %#v", xml, bin)
	}
	m := bin.(map[string]any)
	for k, want := range map[string]any{
		"Unicode":    "Café ☕ 日本語 𝄞",
		"Negative":   int64(-5),
		"EightBytes": int64(1 << 40),
		"Date":       time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC),
		"OldDate":    time.Date(1999, 12, 31, 23, 59, 59, 0, time.UTC),
	} {
		if got := m[k]; !reflect.DeepEqual(got, want) {
			t.Errorf("%s = %#v, want %#v", k, got, want)
		}
	}
	if n := len(m["Many"].([]any)); n != 17 {
		t.Errorf("Many has %d", n)
	}

	info, err := parsePlist([]byte(infoPlist))
	if err != nil {
		t.Fatal(err)
	}
	if bin := read("Info.bplist"); !reflect.DeepEqual(info, bin) {
		t.Errorf("Info.bplist isn't infoPlist:\nxml %#v\nbin %#v", info, bin)
	}
}

func bplistOf(top int, objects ...[]byte) []byte {
	b := []byte("bplist00")
	var offsets []byte
	for _, o := range objects {
		offsets = append(offsets, byte(len(b)>>8), byte(len(b)))
		b = append(b, o...)
	}
	table := len(b)
	b = append(b, offsets...)
	trailer := make([]byte, 32)
	trailer[6], trailer[7] = 2, 1
	binary.BigEndian.PutUint64(trailer[8:], uint64(len(objects)))
	binary.BigEndian.PutUint64(trailer[16:], uint64(top))
	binary.BigEndian.PutUint64(trailer[24:], uint64(table))
	return append(b, trailer...)
}

func TestBinaryPlistRefuses(t *testing.T) {
	ok := bplistOf(0, []byte{0xd1, 1, 2}, []byte("\x51k"), []byte("\x51v"))
	if v, err := parseBinaryPlist(ok); err != nil || !reflect.DeepEqual(v, map[string]any{"k": "v"}) {
		t.Fatalf("the well-formed one: %v %v", v, err)
	}
	for name, b := range map[string][]byte{
		"short":           []byte("bplist00"),
		"xml header":      append([]byte("bplist01"), ok[8:]...),
		"loop":            bplistOf(0, []byte{0xa1, 0}),
		"loop via dict":   bplistOf(0, []byte{0xd1, 1, 0}, []byte("\x51k")),
		"ref past end":    bplistOf(0, []byte{0xa1, 9}),
		"top past end":    bplistOf(3, []byte{0x09}),
		"string too long": bplistOf(0, []byte("\x5fz")),
		"count too big":   bplistOf(0, []byte{0x5f, 0x13, 0x7f, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}),
		"negative count":  bplistOf(0, []byte{0x4f, 0x13, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}),
		"utf16 too long":  bplistOf(0, []byte{0x6f, 0x12, 0x40, 0, 0, 0}),
		"128-bit int":     bplistOf(0, append([]byte{0x14, 0, 0, 0, 0, 0, 0, 0, 1}, make([]byte, 8)...)),
		"past int64":      bplistOf(0, append([]byte{0x14, 0, 0, 0, 0, 0, 0, 0, 0, 0x80}, make([]byte, 7)...)),
		"int key":         bplistOf(0, []byte{0xd1, 1, 1}, []byte{0x10, 1}),
		"null":            bplistOf(0, []byte{0x00}),
		"NaN date":        bplistOf(0, []byte{0x33, 0x7f, 0xf8, 0, 0, 0, 0, 0, 1}),
		"truncated real":  bplistOf(0, []byte{0x23, 0, 0}),
	} {
		if v, err := parseBinaryPlist(b); err == nil {
			t.Errorf("%s: read as %#v", name, v)
		}
	}

	for want, b := range map[int64][]byte{
		3:  {0x14, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 3},
		-2: {0x14, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xfe},
	} {
		if v, err := parseBinaryPlist(bplistOf(0, b)); err != nil || v != want {
			t.Errorf("16-byte %d: %v %v", want, v, err)
		}
	}

	var objects [][]byte
	for i := range 40 {
		objects = append(objects, []byte{0xa2, byte(i + 1), byte(i + 1)})
	}
	objects = append(objects, []byte("\x51x"))
	if _, err := parseBinaryPlist(bplistOf(0, objects...)); err != nil {
		t.Errorf("shared objects: %v", err)
	}

	objects = nil
	for i := range 200 {
		objects = append(objects, []byte{0xa1, byte(i + 1)})
	}
	objects = append(objects, []byte("\x51x"))
	if _, err := parseBinaryPlist(bplistOf(0, objects...)); err == nil || !strings.Contains(err.Error(), "deep") {
		t.Errorf("200 deep: %v", err)
	}
}

func FuzzBinaryPlist(f *testing.F) {
	for _, name := range []string{"rich.bplist", "Info.bplist"} {
		b, err := os.ReadFile(filepath.Join("testdata", name))
		if err != nil {
			f.Fatal(err)
		}
		f.Add(b)
	}
	f.Fuzz(func(t *testing.T, b []byte) {
		parsePlist(b)
	})
}
