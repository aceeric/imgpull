package imgpull

import (
	"fmt"
	"strings"
)

type pullType int

const (
	byTag pullType = iota
	byDigest
)

// ImageRef has the components of an image reference. If 'raw' is
// 'foo.io/bar/baz:1.2.3' then:
//
//	raw        := foo.io/bar/baz:v1.2.3
//	PullType   := byTag
//	Registry   := foo.io
//	Server     := foo.io
//	Repository := bar/baz
//	Org        := bar
//	Image      := baz
//	Ref        := v1.2.3
type ImageRef struct {
	Raw        string
	PullType   pullType
	Registry   string
	Server     string
	Repository string
	Org        string
	Image      string
	Ref        string
	Scheme     string
}

// NewImageRef parses the passed image url (e.g. docker.io/hello-world:latest,
// or docker.io/library/hello-world@sha256:...) into an 'ImageRef' struct. The url
// MUST begin with a registry ref (e.g. quay.io) - it is not (and cannot be) inferred.
func NewImageRef(url, scheme string) (ImageRef, error) {
	org := ""
	img := ""
	ref := ""
	repository := ""
	pt := byTag
	registry := ""
	server := ""

	parts := strings.Split(url, "/")
	registry = parts[0]
	server = parts[0]

	if strings.ToLower(registry) == "docker.io" {
		server = "registry.docker.io"
	}

	if len(parts) == 2 {
		org = "library"
		img = parts[1]
	} else if len(parts) == 3 {
		org = parts[1]
		img = parts[2]
	} else {
		return ImageRef{}, fmt.Errorf("unable to parse image url: %s", url)
	}

	ref_separators := []struct {
		separator string
		pt        pullType
	}{
		{separator: "@", pt: byDigest},
		{separator: ":", pt: byTag},
	}

	for _, rs := range ref_separators {
		if strings.Contains(img, rs.separator) {
			tmp := strings.Split(img, rs.separator)
			img = tmp[0]
			ref = tmp[1]
			pt = rs.pt
			repository = fmt.Sprintf("%s/%s", org, img)
			break
		}
	}

	if img == "" {
		return ImageRef{}, fmt.Errorf("unable to parse image url: %s", url)
	}

	return ImageRef{
		Raw:        url,
		PullType:   pt,
		Registry:   registry,
		Server:     server,
		Repository: repository,
		Org:        org,
		Image:      img,
		Ref:        ref,
		Scheme:     scheme,
	}, nil
}

// ImageUrl returns the ImageRef receiver as an image reference suitable for a
// 'docker pull' command. E.g.: 'quay.io/appzygy/ociregistry:1.5.0'. If the namespace
// arg is non-empty then it is appended as a query string. E.g. if the receiver has a
// reference like 'localhost:8080/appzygy/ociregistry:1.5.0' and namepace is passed with
// 'quay.io' then the function returns 'localhost:8080/appzygy/ociregistry:1.5.0?ns=quay.io'.
// This supports pulling from pull-through registries.
func (ip *ImageRef) ImageUrl(namespace string) string {
	separator := ":"
	reg := ip.Registry
	if namespace != "" {
		reg = namespace
	}
	if strings.HasPrefix(ip.Ref, "sha256:") {
		separator = "@"
	}
	if ip.Org == "" {
		return fmt.Sprintf("%s/%s%s%s", reg, ip.Image, separator, ip.Ref)
	}
	return fmt.Sprintf("%s/%s/%s%s%s", reg, ip.Org, ip.Image, separator, ip.Ref)
}

// RegistryUrl handles the case where an image is pulled from docker.io but the package
// has to access the DockerHub API on registry.docker.io so the receiver would have a
// 'Registry' value of docker.io and a 'Server' value of registry.docker.io. So this
// function is used whenver API calls are made.
func (ip *ImageRef) RegistryUrl() string {
	return fmt.Sprintf("%s://%s", ip.Scheme, ip.Server)
}
