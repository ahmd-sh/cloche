package wire

import "time"

const Version = 1

const Port = 41778

type Manifest struct {
	V        int       `json:"v"`
	ID       string    `json:"id"`
	Seq      int64     `json:"seq"`
	Host     string    `json:"host"`
	PushedAt time.Time `json:"pushed_at"`
	File     File      `json:"file"`
	APK      *APK      `json:"apk,omitempty"`
	Note     string    `json:"note,omitempty"`
	Source   *Source   `json:"source,omitempty"`
}

type File struct {
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

type APK struct {
	Package     string `json:"package"`
	VersionCode int64  `json:"version_code"`
	VersionName string `json:"version_name,omitempty"`
	Label       string `json:"label,omitempty"`
	Signer      string `json:"signer,omitempty"`
}

type Source struct {
	Dir    string `json:"dir"`
	Repo   string `json:"repo,omitempty"`
	Branch string `json:"branch,omitempty"`
	Commit string `json:"commit,omitempty"`
	Dirty  bool   `json:"dirty,omitempty"`
}

type Delivery struct {
	Manifest []byte `json:"manifest"`
	Sig      []byte `json:"sig"`
}

type Inbox struct {
	Head       int64      `json:"head"`
	Name       string     `json:"name,omitempty"`
	Deliveries []Delivery `json:"deliveries"`
}

type PairRequest struct {
	Code   string `json:"code"`
	Device string `json:"device"`
}

type PairResponse struct {
	Host   string `json:"host"`
	Name   string `json:"name,omitempty"`
	Addr   string `json:"addr"`
	Key    []byte `json:"key"`
	Secret string `json:"secret"`
	Head   int64  `json:"head"`
}

type Result struct {
	Status      string `json:"status"`
	Detail      string `json:"detail,omitempty"`
	Package     string `json:"package,omitempty"`
	VersionCode int64  `json:"version_code,omitempty"`
	VersionName string `json:"version_name,omitempty"`
	Signer      string `json:"signer,omitempty"`
}

const (
	Ready      = "ready"
	Blocked    = "blocked"
	Installing = "installing"
	Installed  = "installed"
	Failed     = "failed"
	Declined   = "declined"
	Superseded = "superseded"

	Asked      = "asked"
	Downloaded = "downloaded"
)

func (r Result) Final() bool {
	switch r.Status {
	case Blocked, Installed, Failed, Declined, Superseded, Downloaded:
		return true
	}
	return false
}
