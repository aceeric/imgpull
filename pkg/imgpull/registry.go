package imgpull

import "net/http"

// globally todo for error messages should I imclude the url or leave that to the caller?
type RegistryOpts struct {
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

type Registry struct {
	Opts      RegistryOpts
	ImgPull   ImagePull
	Client    *http.Client
	Token     BearerToken
	Basic     BasicAuth
	Connected bool
}

// NewRegistry initializes and returns a Registry struct from the passed options. Part
// of that involves parsing and validing the 'Url' member of the options, for example
// docker.io/hello-world@latest). The url MUST begin with a registry ref (e.g. quay.io) -
// it is not inferred by the function.
// TODO accept arch as x,y,z and parse to array
func NewRegistry(o RegistryOpts) (Registry, error) {
	if pr, err := NewImagePull(o.Url, o.Scheme); err != nil {
		return Registry{}, err
	} else {
		return Registry{
			ImgPull: pr,
			Client:  &http.Client{},
			Opts:    o,
		}, nil
	}
}

func (r *Registry) authHdr() (string, string) {
	if r.Token != (BearerToken{}) {
		return "Authorization", "Bearer " + r.Token.Token
	} else if r.Opts.Username != "" {
		return "Authorization", "Basic " + r.Basic.Encoded
	}
	return "", ""
}

func (r *Registry) hasAuth() bool {
	if r.Token != (BearerToken{}) {
		return true
	} else if r.Basic != (BasicAuth{}) {
		return true
	}
	return false
}

func (r *Registry) nsQueryParm() string {
	if r.Opts.Namespace != "" {
		return "?ns=" + r.Opts.Namespace
	} else {
		return ""
	}
}
