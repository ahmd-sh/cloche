package ipa

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"
)

type Info struct {
	BundleID  string   `json:"bundle_id"`
	Name      string   `json:"name"`
	Version   string   `json:"version,omitempty"`
	Build     string   `json:"build,omitempty"`
	MinOS     string   `json:"min_os,omitempty"`
	Platforms []string `json:"platforms,omitempty"`
	Profile   *Profile `json:"profile,omitempty"`
	Signed    bool     `json:"signed"`

	Icon []byte `json:"-"`
}

type Profile struct {
	Name       string    `json:"name"`
	Kind       string    `json:"kind"`
	Team       string    `json:"team,omitempty"`
	TeamID     string    `json:"team_id,omitempty"`
	AppID      string    `json:"app_id,omitempty"`
	Expires    time.Time `json:"expires"`
	Devices    []string  `json:"devices,omitempty"`
	AllDevices bool      `json:"all_devices,omitempty"`
}

const (
	Development = "development"
	AdHoc       = "ad hoc"
	Enterprise  = "enterprise"
	AppStore    = "app store"
)

func (p *Profile) Covers(udid string) bool {
	if p.AllDevices {
		return true
	}
	return slices.ContainsFunc(p.Devices, func(d string) bool { return strings.EqualFold(d, udid) })
}

func IsIPA(p string) bool {
	z, err := zip.OpenReader(p)
	if err != nil {
		return false
	}
	defer z.Close()
	_, err = appDir(&z.Reader)
	return err == nil
}

func Read(p string) (*Info, error) {
	z, err := zip.OpenReader(p)
	if err != nil {
		return nil, fmt.Errorf("not an .ipa: %w", err)
	}
	defer z.Close()
	app, err := appDir(&z.Reader)
	if err != nil {
		return nil, err
	}
	files := map[string]*zip.File{}
	for _, f := range z.File {
		if strings.HasPrefix(f.Name, app) {
			files[strings.TrimPrefix(f.Name, app)] = f
		}
	}
	read := func(name string) ([]byte, error) {
		f := files[name]
		if f == nil {
			return nil, fs.ErrNotExist
		}
		if f.UncompressedSize64 > 64<<20 {
			return nil, fmt.Errorf("%s is too big", name)
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		defer rc.Close()
		return io.ReadAll(io.LimitReader(rc, 64<<20))
	}

	b, err := read("Info.plist")
	if err != nil {
		return nil, fmt.Errorf("no Info.plist in %s", strings.TrimSuffix(app, "/"))
	}
	v, err := parsePlist(b)
	if err != nil {
		return nil, fmt.Errorf("Info.plist: %w", err)
	}
	plist, _ := v.(map[string]any)
	info := &Info{
		BundleID:  str(plist, "CFBundleIdentifier"),
		Name:      firstOf(str(plist, "CFBundleDisplayName"), str(plist, "CFBundleName"), strings.TrimSuffix(path.Base(app), ".app")),
		Version:   str(plist, "CFBundleShortVersionString"),
		Build:     str(plist, "CFBundleVersion"),
		MinOS:     str(plist, "MinimumOSVersion"),
		Platforms: strs(plist, "CFBundleSupportedPlatforms"),
	}
	if info.BundleID == "" {
		return nil, errors.New("Info.plist has no CFBundleIdentifier")
	}
	if b, err := read("embedded.mobileprovision"); err == nil {
		if info.Profile, err = parseProfile(b); err != nil {
			return nil, fmt.Errorf("embedded.mobileprovision: %w", err)
		}
	}
	_, info.Signed = files["_CodeSignature/CodeResources"]
	info.Icon = findIcon(plist, files, read)
	return info, nil
}

func appDir(z *zip.Reader) (string, error) {
	for _, f := range z.File {
		name := f.Name
		if !strings.HasPrefix(name, "Payload/") {
			continue
		}
		rest := strings.TrimPrefix(name, "Payload/")
		dir, _, ok := strings.Cut(rest, "/")
		if ok && strings.HasSuffix(dir, ".app") {
			return "Payload/" + dir + "/", nil
		}
	}
	return "", errors.New("no Payload/*.app inside: not an .ipa")
}

func parseProfile(b []byte) (*Profile, error) {
	start := bytes.Index(b, []byte("<?xml"))
	end := bytes.LastIndex(b, []byte("</plist>"))
	if start < 0 || end < start {
		return nil, errors.New("no plist inside")
	}
	v, err := parsePlist(b[start : end+len("</plist>")])
	if err != nil {
		return nil, err
	}
	m, _ := v.(map[string]any)
	if m == nil {
		return nil, errors.New("not a dictionary")
	}
	p := &Profile{
		Name:       str(m, "Name"),
		Team:       str(m, "TeamName"),
		Devices:    strs(m, "ProvisionedDevices"),
		AllDevices: boolean(m, "ProvisionsAllDevices"),
	}
	if ids := strs(m, "TeamIdentifier"); len(ids) > 0 {
		p.TeamID = ids[0]
	}
	p.Expires, _ = m["ExpirationDate"].(time.Time)
	ent := dict(m, "Entitlements")
	p.AppID = str(ent, "application-identifier")
	switch {
	case p.AllDevices:
		p.Kind = Enterprise
	case len(p.Devices) > 0 && boolean(ent, "get-task-allow"):
		p.Kind = Development
	case len(p.Devices) > 0:
		p.Kind = AdHoc
	default:
		p.Kind = AppStore
	}
	return p, nil
}

func findIcon(plist map[string]any, files map[string]*zip.File, read func(string) ([]byte, error)) []byte {
	var names []string
	for _, k := range []string{"CFBundleIcons", "CFBundleIcons~ipad"} {
		names = append(names, strs(dict(dict(plist, k), "CFBundlePrimaryIcon"), "CFBundleIconFiles")...)
	}
	names = append(names, strs(plist, "CFBundleIconFiles")...)
	if f := str(plist, "CFBundleIconFile"); f != "" {
		names = append(names, f)
	}
	type cand struct {
		name string
		size uint64
	}
	var cands []cand
	for file, f := range files {
		if strings.Contains(file, "/") || !strings.HasSuffix(file, ".png") {
			continue
		}
		for _, n := range names {
			if strings.HasPrefix(file, strings.TrimSuffix(n, ".png")) {
				cands = append(cands, cand{file, f.UncompressedSize64})
				break
			}
		}
	}
	sort.Slice(cands, func(i, j int) bool { return cands[i].size > cands[j].size })
	for _, c := range cands {
		b, err := read(c.name)
		if err != nil {
			continue
		}
		if png, err := normalPNG(b); err == nil {
			return png
		}
	}
	return nil
}

func firstOf(s ...string) string {
	for _, v := range s {
		if v != "" {
			return v
		}
	}
	return ""
}

func Pack(app string, w io.Writer) error {
	app = filepath.Clean(app)
	st, err := os.Stat(app)
	if err != nil {
		return err
	}
	if !st.IsDir() || !strings.HasSuffix(app, ".app") {
		return fmt.Errorf("%s isn't an .app", app)
	}
	z := zip.NewWriter(w)
	base := "Payload/" + filepath.Base(app)
	err = filepath.WalkDir(app, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(app, p)
		if err != nil {
			return err
		}
		name := base
		if rel != "." {
			name += "/" + filepath.ToSlash(rel)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		h, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		h.Name = name
		if d.IsDir() {
			h.Name += "/"
			_, err := z.CreateHeader(h)
			return err
		}
		h.Method = zip.Deflate
		out, err := z.CreateHeader(h)
		if err != nil {
			return err
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			target, err := os.Readlink(p)
			if err != nil {
				return err
			}
			_, err = io.WriteString(out, target)
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("%s isn't a file", p)
		}
		f, err := os.Open(p)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.Copy(out, f)
		return err
	})
	if err != nil {
		return err
	}
	return z.Close()
}

var pngSig = []byte("\x89PNG\r\n\x1a\n")
