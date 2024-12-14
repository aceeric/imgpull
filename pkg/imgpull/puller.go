package imgpull

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"os"
	"slices"
	"strconv"
	"strings"
)

// PullerOpts defines all the configurable behaviors of the puller.
type PullerOpts struct {
	// Url is the image url like 'docker.io/hello-world:latest'.
	Url string
	// Scheme is 'http' or 'https'.
	Scheme string
	// OSType is the operating system type, e.g.: 'linux'.
	OSType string
	// OSType is the architecture, e.g.: 'amd64'.
	ArchType string
	// Username is the user name for basic auth.
	Username string
	// Username is the password for basic auth.
	Password string
	// TlsCert is the client pki certificate for mTLS.
	TlsCert string
	// TlsKey is the client pki key for mTLS.
	TlsKey string
	// CACert is the client CA if the host truststore cannot verify the
	// server cert.
	CACert string
	// Insecure skips server cert validation for the upstream registry (https-only)
	Insecure string
	// Namespace supports pull-through, i.e. pull 'localhost:5000/hello-world:latest'
	// with namespace 'docker.io' to pull through localhost to dockerhub if you
	// have a pull-through registry that supports that.
	Namespace string
}

// Puller is the top-level abstraction. It carries everything that is needed to pull
// an OCI image from an upstream OCI distribution server.
type Puller struct {
	// Opts defines all the configurable behaviors of the puller.
	Opts PullerOpts
	// ImgRef is the parsed image url, e.g.: 'docker.io/hello-world:latest'
	ImgRef ImageRef
	// Client is the HTTP client
	Client *http.Client
	// If the upstream requires bearer auth, this is the token received from
	// the upstream registry
	Token BearerToken
	// If the upstream requires basic auth, this is the encoded user/pass
	// from 'Opts'
	Basic BasicAuth
	// Indicates that the struct has been used to negotiate a connection to
	// the upstream OCI distribution server.
	Connected bool
}

// PullOpt supports creating a puller with variadic args.
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
//	p, err := NewPuller("my.http.registry:5000/hello-world:latest", http())
func NewPuller(url string, opts ...PullOpt) (Puller, error) {
	o := PullerOpts{
		Url: url,
	}
	for _, opt := range opts {
		opt(&o)
	}
	return NewPullerWith(o)
}

// NewPullerWith initializes and returns a Puller from the passed options. Part of
// that involves parsing and validing the 'Url' member of the options, for example
// docker.io/hello-world@latest). The url MUST begin with a registry ref (e.g. quay.io) -
// it is not inferred - and cannot be inferred - by the function.
func NewPullerWith(o PullerOpts) (Puller, error) {
	if !o.checkPlatform() {
		return Puller{}, fmt.Errorf("operating system %q and/or architecture %q are not valid", o.OSType, o.ArchType)
	}
	if _, err := strconv.ParseBool(o.Insecure); err != nil {
		return Puller{}, fmt.Errorf("value %q for insecure does not parse as a boolean", o.Insecure)
	}
	o.Scheme = strings.ToLower(o.Scheme)
	if pr, err := NewImageRef(o.Url, o.Scheme); err != nil {
		return Puller{}, err
	} else {
		c := &http.Client{}
		cfg, err := o.configureTls()
		if err != nil {
			return Puller{}, err
		}
		if cfg != nil {
			tr := &http.Transport{
				TLSClientConfig: cfg,
			}
			c.Transport = tr
		}
		return Puller{
			ImgRef: pr,
			Client: c,
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
	if newOpts.OSType != "" {
		o.OSType = newOpts.OSType
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
	if newOpts.Insecure != "" {
		o.Insecure = newOpts.Insecure
	}
	if newOpts.Namespace != "" {
		o.Namespace = newOpts.Namespace
	}
	return NewPullerWith(o)
}

// authHdr returns a key/value pair to set an auth header based on whether
// the receiver is configured for bearer or basic auth.
func (p *Puller) authHdr() (string, string) {
	if p.Token != (BearerToken{}) {
		return "Authorization", "Bearer " + p.Token.Token
	} else if p.Opts.Username != "" {
		return "Authorization", "Basic " + p.Basic.Encoded
	}
	return "", ""
}

// checkPlatform validates the passed OS and architecture as well as
// their combination together.
func (o PullerOpts) checkPlatform() bool {
	validOsArch := map[string][]string{
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
	for os, archs := range validOsArch {
		if os == o.OSType {
			return slices.Contains(archs, o.ArchType)
		}
	}
	return false
}

// configureTls initializes and returns a pointer to a 'tls.Config' struct based
// on TLS-related variables in the receiver. If there are no TLS-related variables in
// the receiver then nil is returned.
func (o PullerOpts) configureTls() (*tls.Config, error) {
	if o.Scheme == "http" {
		return nil, nil
	}
	cfg := &tls.Config{}
	hasCfg := false
	if o.TlsCert != "" && o.TlsKey != "" {
		if cert, err := tls.LoadX509KeyPair(o.TlsCert, o.TlsKey); err != nil {
			return nil, err
		} else {
			cfg.Certificates = []tls.Certificate{cert}
			hasCfg = true
		}
	}
	if o.CACert != "" {
		cp := x509.NewCertPool()
		if caCert, err := os.ReadFile(o.CACert); err != nil {
			return nil, err
		} else {
			cp.AppendCertsFromPEM(caCert)
			hasCfg = true
		}
	}
	if insecure, _ := strconv.ParseBool(o.Insecure); insecure {
		cfg.InsecureSkipVerify = insecure
		hasCfg = true
	}

	if hasCfg {
		return cfg, nil
	}
	return nil, nil
}
