package imgpull

import "net/http"

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
	ImgPull   ImageRef
	Client    *http.Client
	Token     BearerToken
	Basic     BasicAuth
	Connected bool
}

// NewPuller initializes and returns a Puller struct from the passed options. Part
// of that involves parsing and validing the 'Url' member of the options, for example
// docker.io/hello-world@latest). The url MUST begin with a registry ref (e.g. quay.io) -
// it is not inferred by the function.
func NewPuller(o PullerOpts) (Puller, error) {
	if pr, err := NewImageRef(o.Url, o.Scheme); err != nil {
		return Puller{}, err
	} else {
		return Puller{
			ImgPull: pr,
			Client:  &http.Client{},
			Opts:    o,
		}, nil
	}
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
