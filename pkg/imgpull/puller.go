package imgpull

import (
	"net/http"

	"github.com/aceeric/imgpull/internal/imgref"
	"github.com/aceeric/imgpull/pkg/imgpull/types"
)

// Puller is the top-level abstraction. It carries everything that is needed to pull
// an OCI image from an upstream OCI distribution server.
type Puller struct {
	// Opts defines all the configurable behaviors of the puller.
	Opts PullerOpts
	// ImgRef is the parsed image url, e.g.: 'docker.io/hello-world:latest'
	ImgRef imgref.ImageRef
	// Client is the HTTP client
	Client *http.Client
	// If the upstream requires bearer auth, this is the token received from
	// the upstream registry
	Token types.BearerToken
	// If the upstream requires basic auth, this is the encoded user/pass
	// from 'Opts'
	Basic types.BasicAuth
	// Indicates that the struct has been used to negotiate a connection to
	// the upstream OCI distribution server.
	Connected bool
}

// PullOpt supports creating a Puller with variadic args.
type PullOpt func(*PullerOpts)

// NewPuller creates a Puller from the passed url and any additional options
// from the opts variadic list. Example: The puller defaults to https. Suppose
// you need to pull from an http registry instead. Then:
//
//	http := func() PullOpt {
//		return func(p *imgpull.PullerOpts) {
//			p.Scheme = "http"
//		}
//	}
//	p, err := imgpull.NewPuller("my.http.registry:5000/hello-world:latest", http())
func NewPuller(url string, opts ...PullOpt) (Puller, error) {
	o := PullerOpts{
		Url:    url,
		Scheme: "https",
	}
	for _, opt := range opts {
		opt(&o)
	}
	return NewPullerWith(o)
}

// NewPullerWith initializes and returns a Puller from the passed options. The Url
// in the passed PullerOpts MUST begin with a registry reference (e.g. quay.io): it is
// not inferred - and cannot be inferred - by the function.
func NewPullerWith(o PullerOpts) (Puller, error) {
	if err := o.validate(); err != nil {
		return Puller{}, err
	}
	if ir, err := imgref.NewImageRef(o.Url, o.Scheme, o.Namespace); err != nil {
		return Puller{}, err
	} else {
		c := &http.Client{}
		if cfg, err := o.configureTls(); err != nil {
			return Puller{}, err
		} else if cfg != nil {
			c.Transport = &http.Transport{
				TLSClientConfig: cfg,
			}
		}
		return Puller{
			ImgRef: ir,
			Client: c,
			Opts:   o,
		}, nil
	}
}

// authHdr returns a key/value pair to set an auth header based on whether
// the receiver is configured for bearer or basic auth.
func (p *Puller) authHdr() (string, string) {
	if p.Token != (types.BearerToken{}) {
		return "Authorization", "Bearer " + p.Token.Token
	} else if p.Opts.Username != "" {
		return "Authorization", "Basic " + p.Basic.Encoded
	}
	return "", ""
}
