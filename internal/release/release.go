package release

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var Base = "https://github.com/ahmd-sh/cloche/releases"

const maxManifest = 64 << 10

type Manifest struct {
	Version string            `json:"version"`
	Files   map[string]string `json:"files"`
}

var (
	semver = regexp.MustCompile(`^(\d{1,3})\.(\d{1,3})\.(\d{1,3})$`)
	digest = regexp.MustCompile(`^[0-9a-f]{64}$`)
	file   = regexp.MustCompile(`^[A-Za-z0-9._-]{1,100}$`)
)

func Parse(v string) ([3]int, bool) {
	m := semver.FindStringSubmatch(v)
	if m == nil {
		return [3]int{}, false
	}
	var p [3]int
	for i := range p {
		p[i], _ = strconv.Atoi(m[i+1])
	}
	return p, true
}

func Newer(v, than string) bool {
	a, okA := Parse(v)
	b, okB := Parse(than)
	if !okA || !okB {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return a[i] > b[i]
		}
	}
	return false
}

func Code(v string) int64 {
	p, ok := Parse(v)
	if !ok {
		return 0
	}
	return int64(p[0])*1_000_000 + int64(p[1])*1_000 + int64(p[2])
}

func LatestURL() string { return Base + "/latest/download/latest.json" }

func URL(version, name string) string { return Base + "/download/v" + version + "/" + name }

func Client() *http.Client {
	return &http.Client{
		Timeout: 10 * time.Minute,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return errors.New("too many redirects")
			}
			if via[0].URL.Scheme == "https" && req.URL.Scheme != "https" {
				return errors.New("redirected away from HTTPS")
			}
			return nil
		},
	}
}

func get(ctx context.Context, c *http.Client, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "cloche")
	res, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	if res.StatusCode != http.StatusOK {
		res.Body.Close()
		return nil, fmt.Errorf("%s: %s", url, res.Status)
	}
	return res, nil
}

func Fetch(ctx context.Context, c *http.Client, url string) (Manifest, error) {
	res, err := get(ctx, c, url)
	if err != nil {
		return Manifest{}, err
	}
	defer res.Body.Close()
	b, err := io.ReadAll(io.LimitReader(res.Body, maxManifest+1))
	if err != nil {
		return Manifest{}, err
	}
	if len(b) > maxManifest {
		return Manifest{}, errors.New("latest.json is too big")
	}
	var m Manifest
	if err := json.Unmarshal(b, &m); err != nil {
		return Manifest{}, fmt.Errorf("latest.json: %w", err)
	}
	if _, ok := Parse(m.Version); !ok {
		return Manifest{}, fmt.Errorf("latest.json: version %q isn't X.Y.Z", m.Version)
	}
	for name, sum := range m.Files {
		if !file.MatchString(name) || !digest.MatchString(sum) {
			return Manifest{}, fmt.Errorf("latest.json: bad entry %q", name)
		}
	}
	return m, nil
}

func Download(ctx context.Context, c *http.Client, url, sum string, limit int64, w io.Writer) error {
	if !digest.MatchString(sum) {
		return errors.New("no checksum to check it against")
	}
	res, err := get(ctx, c, url)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(w, h), io.LimitReader(res.Body, limit+1))
	if err != nil {
		return err
	}
	if n > limit {
		return fmt.Errorf("%s is over %d MB", url, limit>>20)
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != sum {
		return fmt.Errorf("%s doesn't match its checksum", url[strings.LastIndex(url, "/")+1:])
	}
	return nil
}
