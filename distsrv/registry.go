package distsrv

import "net/http"

type Registry struct {
	ImgPull  ImagePull
	Client   *http.Client
	Token    BearerToken
	OSType   string
	ArchType string
}

// TODO accept arch as x,y,z and parse to array
// NewRegistry parses the passed image url (e.g. docker.io/hello-world:latest,
// or docker.io/library/hello-world@sha256:...) into a 'PullRequest' struct. The url
// MUST begin with a registry ref (e.g. quay.io) - it is not inferred.
func NewRegistry(url string, os string, arch string) (Registry, error) {
	if pr, err := NewImagePull(url); err != nil {
		return Registry{}, err
	} else {
		return Registry{
			ImgPull:  pr,
			Client:   &http.Client{},
			OSType:   os,
			ArchType: arch,
		}, nil
	}
}
