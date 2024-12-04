package distsrv

import "net/http"

type RegistryOpts struct {
	OSType   string
	ArchType string
	Username string
	Password string
	TlsCert  string
	TlsKey   string
	CACert   string
}
type Registry struct {
	ImgPull  ImagePull
	Client   *http.Client
	Token    BearerToken
	OSType   string
	ArchType string
	Username string
	Password string
	Basic    BasicAuth
}

// TODO accept arch as x,y,z and parse to array
// NewRegistry parses the passed image url (e.g. docker.io/hello-world:latest,
// or docker.io/library/hello-world@sha256:...) into a 'PullRequest' struct. The url
// MUST begin with a registry ref (e.g. quay.io) - it is not inferred.
func NewRegistry(url string, os string, arch string, user string, pass string, scheme string) (Registry, error) {
	if pr, err := NewImagePull(url, scheme); err != nil {
		return Registry{}, err
	} else {
		return Registry{
			ImgPull:  pr,
			Client:   &http.Client{},
			OSType:   os,
			ArchType: arch,
			Username: user,
			Password: pass,
		}, nil
	}
}

func (r *Registry) authHdr() (string, string) {
	if r.Token != (BearerToken{}) {
		return "Authorization", "Bearer " + r.Token.Token
	} else if r.Username != "" {
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
