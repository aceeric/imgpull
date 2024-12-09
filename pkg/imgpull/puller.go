package imgpull

import (
	"fmt"
	"net/http"
)

type PullerOpts struct {
	Url       string
	Scheme    string
	Dest      string
	OSType    string
	ArchType  string
	Username  string
	Password  string
	TlsCert   string
	TlsKey    string
	CACert    string
	Namespace string
}

type Puller struct {
	Opts      PullerOpts
	ImgRef    ImageRef
	Client    *http.Client
	Token     BearerToken
	Basic     BasicAuth
	Connected bool
}

type PullOpt func(*PullerOpts)

// NewPuller creates a Puller from the passed url and any additional options
// from the opts variadic list. Example: The puller defaults to https. Suppose
// you need to pull from an http registry instead. Then:
//
//	http := func() PullOpt {
//		return func(p *PullerOpts) {
//			p.Scheme = "http"
//		}
//	}
//	p, err := NewPuller("docker.io/hello-world:latest", http())
func NewPuller(url string, opts ...PullOpt) (Puller, error) {
	o := PullerOpts{
		Url: url,
	}
	for _, opt := range opts {
		opt(&o)
	}
	return NewPullerWith(o)
	//if !checkPlatform(o.OSType, o.ArchType) {
	//	return Puller{}, fmt.Errorf("operating system %q and/or architecture %q are not valid", o.OSType, o.ArchType)
	//}
	//if pr, err := NewImageRef(o.Url, o.Scheme); err != nil {
	//	return Puller{}, err
	//} else {
	//	return Puller{
	//		ImgRef: pr,
	//		Client: &http.Client{},
	//		Opts:   o,
	//	}, nil
	//}
}

// NewPullerWith initializes and returns a Puller from the passed options. Part of
// that involves parsing and validing the 'Url' member of the options, for example
// docker.io/hello-world@latest). The url MUST begin with a registry ref (e.g. quay.io) -
// it is not inferred by the function.
func NewPullerWith(o PullerOpts) (Puller, error) {
	if !checkPlatform(o.OSType, o.ArchType) {
		return Puller{}, fmt.Errorf("operating system %q and/or architecture %q are not valid", o.OSType, o.ArchType)
	}
	if pr, err := NewImageRef(o.Url, o.Scheme); err != nil {
		return Puller{}, err
	} else {
		return Puller{
			ImgRef: pr,
			Client: &http.Client{},
			Opts:   o,
		}, nil
	}
}

// NewPullerFrom creates a new puller from the receiver with overrides applied
// from the the passed PullerOpts. The receiver is unmodified.
func (p *Puller) NewPullerFrom(newOpts PullerOpts) (Puller, error) {
	o := p.Opts
	if newOpts.Url != "" {
		o.Url = newOpts.Url
	}
	if newOpts.Scheme != "" {
		o.Scheme = newOpts.Scheme
	}
	if newOpts.Dest != "" {
		o.Dest = newOpts.Dest
	}
	if newOpts.OSType != "" {
		o.Dest = newOpts.Dest
	}
	if newOpts.ArchType != "" {
		o.ArchType = newOpts.ArchType
	}
	if newOpts.Username != "" {
		o.Username = newOpts.Username
	}
	if newOpts.Password != "" {
		o.Password = newOpts.Password
	}
	if newOpts.TlsCert != "" {
		o.TlsCert = newOpts.TlsCert
	}
	if newOpts.TlsKey != "" {
		o.TlsKey = newOpts.TlsKey
	}
	if newOpts.CACert != "" {
		o.CACert = newOpts.CACert
	}
	if newOpts.Namespace != "" {
		o.Namespace = newOpts.Namespace
	}
	return NewPullerWith(o)
}

func (p *Puller) authHdr() (string, string) {
	if p.Token != (BearerToken{}) {
		return "Authorization", "Bearer " + p.Token.Token
	} else if p.Opts.Username != "" {
		return "Authorization", "Basic " + p.Basic.Encoded
	}
	return "", ""
}

func (p *Puller) hasAuth() bool {
	if p.Token != (BearerToken{}) {
		return true
	} else if p.Basic != (BasicAuth{}) {
		return true
	}
	return false
}

func (p *Puller) nsQueryParm() string {
	if p.Opts.Namespace != "" {
		return "?ns=" + p.Opts.Namespace
	} else {
		return ""
	}
}

func checkPlatform(OS string, Architecture string) bool {
	validCombins := map[string][]string{
		"android":   {"arm"},
		"darwin":    {"386", "amd64", "arm", "arm64"},
		"dragonfly": {"amd64"},
		"freebsd":   {"386", "amd64", "arm"},
		"linux":     {"386", "amd64", "arm", "arm64", "ppc64", "ppc64le", "mips64", "mips64le", "s390x", "riscv64"},
		"netbsd":    {"386", "amd64", "arm"},
		"openbsd":   {"386", "amd64", "arm"},
		"plan9":     {"386", "amd64"},
		"solaris":   {"amd64"},
		"windows":   {"386", "amd64"}}
	for os, archs := range validCombins {
		if os == OS {
			for _, arch := range archs {
				if arch == Architecture {
					return true
				}
			}
			return false
		}
	}
	return false
}
