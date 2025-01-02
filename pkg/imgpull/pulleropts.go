package imgpull

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"runtime"
	"slices"
	"strings"
)

// PullerOpts defines all the configurable behaviors of the Puller.
type PullerOpts struct {
	// Url is the image Url like 'docker.io/hello-world:latest'.
	Url string
	// Scheme is 'http' or 'https'.
	Scheme string
	// OStype is the operating system type, e.g.: 'linux'.
	OStype string
	// ArchType is the architecture, e.g.: 'amd64'.
	ArchType string
	// Username is the user name for basic auth.
	Username string
	// Password is the Password for basic auth.
	Password string
	// TlsCert is the path on the file system to a client pki certificate for mTLS.
	TlsCert string
	// TlsKey is the path on the file system to a client pki key for mTLS.
	TlsKey string
	// CaCert is the path on the file system to a client CA if the host truststore cannot verify the
	// server cert.
	CaCert string
	// Insecure skips server cert validation for the upstream registry (https-only.)
	Insecure bool
	// Namespace supports pull-through and mirroring, i.e. pull 'localhost:5000/hello-world:latest'
	// with Namespace 'docker.io' to pull from localhost if localhost is a mirror
	// or a pull-through registry.
	Namespace string
}

// NewPullerOpts is a convenience function that initializes and returns a PullerOpts struct
// for the most common use case: https to the upstream distribution server, and OS and
// architecture based on your system.
func NewPullerOpts(url string) PullerOpts {
	return PullerOpts{
		Url:      url,
		Scheme:   "https",
		OStype:   runtime.GOOS,
		ArchType: runtime.GOARCH,
	}
}

// validate performs option validation and returns an error if any options are
// invalid.
func (o PullerOpts) validate() error {
	if !o.validateOsAndArch() {
		return fmt.Errorf("operating system %q and/or architecture %q are not valid", o.OStype, o.ArchType)
	}
	if o.Url == "" {
		return fmt.Errorf("url is undefined")
	}
	if o.Scheme == "" {
		return fmt.Errorf("scheme is undefined")
	} else {
		validSchemes := []string{"http", "https"}
		o.Scheme = strings.ToLower(o.Scheme)
		if !slices.Contains(validSchemes, o.Scheme) {
			return fmt.Errorf("invalid scheme %q: must be \"http\" or \"https\"", o.Scheme)
		}

	}
	return nil
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
	if o.CaCert != "" {
		if caCert, err := os.ReadFile(o.CaCert); err != nil {
			return nil, err
		} else {
			cp := x509.NewCertPool()
			cp.AppendCertsFromPEM(caCert)
			cfg.RootCAs = cp
			hasCfg = true
		}
	}
	if o.Insecure {
		cfg.InsecureSkipVerify = true
		hasCfg = true
	}

	if hasCfg {
		return cfg, nil
	}
	return nil, nil
}

// validateOsAndArch validates the OS and architecture in the receiver as well as
// their combination together.
func (o PullerOpts) validateOsAndArch() bool {
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
		if os == o.OStype {
			return slices.Contains(archs, o.ArchType)
		}
	}
	return false
}
