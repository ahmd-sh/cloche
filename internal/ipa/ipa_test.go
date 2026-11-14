package ipa

import (
	"archive/zip"
	"bytes"
	"compress/flate"
	"encoding/binary"
	"hash/crc32"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

const infoPlist = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleIdentifier</key><string>com.example.satchel</string>
	<key>CFBundleDisplayName</key><string>Satchel</string>
	<key>CFBundleName</key><string>SatchelApp</string>
	<key>CFBundleShortVersionString</key><string>0.3.0</string>
	<key>CFBundleVersion</key><string>12</string>
	<key>MinimumOSVersion</key><string>17.0</string>
	<key>CFBundleSupportedPlatforms</key><array><string>iPhoneOS</string></array>
	<key>CFBundleIcons</key>
	<dict>
		<key>CFBundlePrimaryIcon</key>
		<dict>
			<key>CFBundleIconFiles</key><array><string>AppIcon60x60</string></array>
			<key>CFBundleIconName</key><string>AppIcon</string>
		</dict>
	</dict>
	<key>UIRequiresFullScreen</key><true/>
	<key>LSRequiresIPhoneOS</key><true/>
</dict>
</plist>`

func profile(kind string) string {
	devices := `<key>ProvisionedDevices</key><array><string>00008140-001A2B3C4D5E6F70</string><string>00008150-0000AAAA0000BBBB</string></array>`
	taskAllow := "<false/>"
	switch kind {
	case Development:
		taskAllow = "<true/>"
	case Enterprise:
		devices = `<key>ProvisionsAllDevices</key><true/>`
	case AppStore:
		devices = ""
	}
	return "0\x82\x10\x01junk-before" + `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>AppIDName</key><string>Satchel</string>
	<key>CreationDate</key><date>2026-11-01T10:00:00Z</date>
	<key>DeveloperCertificates</key><array><data>MIIBAAAA
	AAAA</data></array>
	<key>Entitlements</key>
	<dict>
		<key>application-identifier</key><string>ABCDE12345.com.example.satchel</string>
		<key>get-task-allow</key>` + taskAllow + `
		<key>keychain-access-groups</key><array><string>ABCDE12345.*</string></array>
	</dict>
	<key>ExpirationDate</key><date>2027-11-01T10:00:00Z</date>
	<key>Name</key><string>Satchel Ad Hoc</string>
	` + devices + `
	<key>TeamIdentifier</key><array><string>ABCDE12345</string></array>
	<key>TeamName</key><string>Example Ltd</string>
	<key>TimeToLive</key><integer>365</integer>
	<key>Version</key><integer>1</integer>
</dict>
</plist>` + "\xa0\x82junk-after"
}

func cgbiPNG(t *testing.T) []byte {
	t.Helper()
	rgba := [][4]byte{{255, 0, 0, 255}, {0, 255, 0, 255}, {0, 0, 255, 255}, {255, 255, 255, 128}, {255, 255, 255, 128}, {255, 255, 255, 128}}
	var rows [2][]byte
	for y := range 2 {
		for x := range 3 {
			p := rgba[y*3+x]
			a := int(p[3])
			pm := func(c byte) byte { return byte(int(c) * a / 255) }
			rows[y] = append(rows[y], pm(p[2]), pm(p[1]), pm(p[0]), p[3])
		}
	}
	var raw []byte
	raw = append(raw, 1)
	for i, b := range rows[0] {
		if i >= 4 {
			b -= rows[0][i-4]
		}
		raw = append(raw, b)
	}
	raw = append(raw, 2)
	for i, b := range rows[1] {
		raw = append(raw, b-rows[0][i])
	}
	var z bytes.Buffer
	fw, _ := flate.NewWriter(&z, flate.BestCompression)
	fw.Write(raw)
	fw.Close()

	var out bytes.Buffer
	out.Write(pngSig)
	chunk := func(typ string, data []byte) {
		binary.Write(&out, binary.BigEndian, uint32(len(data)))
		out.WriteString(typ)
		out.Write(data)
		binary.Write(&out, binary.BigEndian, crc32.ChecksumIEEE(append([]byte(typ), data...)))
	}
	chunk("CgBI", []byte{0x50, 0x00, 0x20, 0x06})
	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr, 3)
	binary.BigEndian.PutUint32(ihdr[4:], 2)
	ihdr[8], ihdr[9] = 8, 6
	chunk("IHDR", ihdr)
	chunk("IDAT", z.Bytes())
	chunk("IEND", nil)
	return out.Bytes()
}

func makeApp(t *testing.T, kind string, binaryInfo bool) string {
	t.Helper()
	app := filepath.Join(t.TempDir(), "SatchelApp.app")
	os.MkdirAll(filepath.Join(app, "Frameworks", "Lib.framework"), 0o755)
	write := func(name string, b []byte, mode os.FileMode) {
		if err := os.WriteFile(filepath.Join(app, name), b, mode); err != nil {
			t.Fatal(err)
		}
	}
	if binaryInfo {
		b, err := os.ReadFile(filepath.Join("testdata", "Info.bplist"))
		if err != nil {
			t.Fatal(err)
		}
		write("Info.plist", b, 0o644)
	} else {
		write("Info.plist", []byte(infoPlist), 0o644)
	}
	write("SatchelApp", []byte("\xcf\xfa\xed\xfe"), 0o755)
	write("Frameworks/Lib.framework/Lib", []byte("lib"), 0o755)
	if kind != "" {
		write("embedded.mobileprovision", []byte(profile(kind)), 0o644)
	}
	write("AppIcon60x60@2x.png", cgbiPNG(t), 0o644)
	write("Small.png", []byte("not this one"), 0o644)
	return app
}

func pack(t *testing.T, app string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "Satchel.ipa")
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := Pack(app, f); err != nil {
		t.Fatal(err)
	}
	f.Close()
	return p
}

func TestRead(t *testing.T) {
	for _, binaryInfo := range []bool{false, true} {
		p := pack(t, makeApp(t, AdHoc, binaryInfo))
		if !IsIPA(p) {
			t.Fatal("packed .ipa isn't an IPA")
		}
		info, err := Read(p)
		if err != nil {
			t.Fatal(err)
		}
		if info.BundleID != "com.example.satchel" || info.Name != "Satchel" || info.Version != "0.3.0" || info.Build != "12" || info.MinOS != "17.0" {
			t.Errorf("binary=%v: %+v", binaryInfo, info)
		}
		pr := info.Profile
		if pr == nil {
			t.Fatal("no profile")
		}
		if pr.Kind != AdHoc || pr.Team != "Example Ltd" || pr.TeamID != "ABCDE12345" || pr.Name != "Satchel Ad Hoc" ||
			pr.AppID != "ABCDE12345.com.example.satchel" || !pr.Expires.Equal(time.Date(2027, 11, 1, 10, 0, 0, 0, time.UTC)) || len(pr.Devices) != 2 {
			t.Errorf("profile: %+v", pr)
		}
		if !pr.Covers("00008140-001a2b3c4d5e6f70") || pr.Covers("00008140-FFFFFFFFFFFFFFFF") {
			t.Error("Covers")
		}
	}
}

func TestKinds(t *testing.T) {
	for _, kind := range []string{Development, AdHoc, Enterprise, AppStore} {
		info, err := Read(pack(t, makeApp(t, kind, false)))
		if err != nil {
			t.Fatal(err)
		}
		if info.Profile.Kind != kind {
			t.Errorf("%s read as %s", kind, info.Profile.Kind)
		}
		if kind == Enterprise && !info.Profile.Covers("anything") {
			t.Error("an enterprise profile covers every device")
		}
	}
	info, err := Read(pack(t, makeApp(t, "", false)))
	if err != nil || info.Profile != nil {
		t.Errorf("unsigned: %+v %v", info, err)
	}
}

func TestIcon(t *testing.T) {
	info, err := Read(pack(t, makeApp(t, AdHoc, false)))
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(info.Icon))
	if err != nil {
		t.Fatalf("icon: %v", err)
	}
	if b := img.Bounds(); b.Dx() != 3 || b.Dy() != 2 {
		t.Fatalf("icon is %v", b)
	}
	for _, c := range []struct {
		x, y       int
		r, g, b, a uint32
	}{
		{0, 0, 255, 0, 0, 255}, {1, 0, 0, 255, 0, 255}, {2, 0, 0, 0, 255, 255},
		{1, 1, 128, 128, 128, 128},
	} {
		r, g, b, a := img.At(c.x, c.y).RGBA()
		if r>>8 != c.r || g>>8 != c.g || b>>8 != c.b || a>>8 != c.a {
			t.Errorf("pixel %d,%d: %d %d %d %d", c.x, c.y, r>>8, g>>8, b>>8, a>>8)
		}
	}
}

func TestPackKeepsModes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows files have no mode bits to keep")
	}
	z, err := zip.OpenReader(pack(t, makeApp(t, AdHoc, false)))
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	for _, f := range z.File {
		if f.Name == "Payload/SatchelApp.app/SatchelApp" {
			if f.Mode()&0o111 == 0 {
				t.Errorf("executable lost its mode: %v", f.Mode())
			}
			return
		}
	}
	t.Error("no executable in the IPA")
}

func TestNotAnIPA(t *testing.T) {
	p := filepath.Join(t.TempDir(), "x.apk")
	os.WriteFile(p, []byte("PK\x03\x04nope"), 0o644)
	if IsIPA(p) {
		t.Error("a broken zip is an IPA")
	}
	if _, err := Read(p); err == nil {
		t.Error("read a broken zip")
	}
}
