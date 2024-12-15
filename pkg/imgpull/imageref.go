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

// ImageRef has the components of an image reference. Documentation for
// the individual fields shows how the struct would be initialized if
// NewImageRef was called with url='foo.io/bar/baz:1.2.3', and scheme =
// https.
type ImageRef struct {
	// e.g.: foo.io/bar/baz:v1.2.3
	Raw string
	// if 'Raw' is foo.io/bar/baz:v1.2.3 then 'PullType' is 'byTag'
	PullType pullType
	// if 'Raw' is foo.io/bar/baz:v1.2.3 then 'Registry' is 'foo.io'
	Registry string
	// if 'Raw' is foo.io/bar/baz:v1.2.3 then 'Server' is 'foo.io'
	Server string
	// if 'Raw' is foo.io/bar/baz:v1.2.3 then 'Repository' is 'bar/baz'
	Repository string
	// if 'Raw' is foo.io/bar/baz:v1.2.3 then 'Org' is 'bar'
	Org string
	// if 'Raw' is foo.io/bar/baz:v1.2.3 then 'Image' is 'baz'
	Image string
	// if 'Raw' is foo.io/bar/baz:v1.2.3 then 'Ref' is 'v1.2.3'
	Ref string
	// defaults to 'https' unless overridden
	Scheme string
}

// NewImageRef parses the passed image url (e.g. docker.io/hello-world:latest,
// or docker.io/library/hello-world@sha256:...) into an 'ImageRef' struct. The url
// MUST begin with a registry hostname (e.g. quay.io) - it is not (and cannot be)
// inferred.
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
		// can't make API calls to docker.io
		server = "index.docker.io"
	}

	if len(parts) == 2 {
		org = "library"
		img = parts[1]
	} else if len(parts) == 3 {
		org = parts[1]
		img = parts[2]
	} else {
		return ImageRef{}, fmt.Errorf("unable to parse image url %q", url)
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
		return ImageRef{}, fmt.Errorf("unable to parse image url: %q", url)
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

// ImageUrl returns the ImageRef receiver URL-related content as an image reference suitable for
// a 'docker pull' command. E.g.: 'quay.io/appzygy/ociregistry:1.5.0'.
func (ip *ImageRef) ImageUrl() string {
	return ip.ImageUrlWithNs("")
}

// ImageUrlWithNs returns the ImageRef receiver URL-related content as an image reference suitable for
// a 'docker pull' command. E.g.: 'quay.io/appzygy/ociregistry:1.5.0'.
//
// If the namespace arg is non-empty then the function replaces the registry configured in the
// receiver. E.g.: if the receiver has a reference like 'localhost:8080/appzygy/ociregistry:1.5.0'
// and namespace is passed with 'quay.io' then the function returns
// 'quay.io/appzygy/ociregistry:1.5.0'.
//
// This supports pulling from pull-through registries. The intended purpose of this function
// is to allow an image tarball to be pulled from a pull-through registry but have the
// 'RepoTags' field in the tarball 'manifests.json' look like it was pulled from the registry
// in the namespace rather than from a pull-through registry.
func (ip *ImageRef) ImageUrlWithNs(namespace string) string {
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

func (ip *ImageRef) ImageUrlWithDigest(digest string) string {
	separator := "@"
	reg := ip.Registry
	if ip.Org == "" {
		return fmt.Sprintf("%s/%s%s%s", reg, ip.Image, separator, digest)
	}
	return fmt.Sprintf("%s/%s/%s%s%s", reg, ip.Org, ip.Image, separator, digest)
}

// ServerUrl handles the case where an image is pulled from docker.io but the package
// has to access the DockerHub API on host registry.docker.io so the receiver would have
// a 'Registry' value of docker.io and a 'Server' value of registry.docker.io. This function
// is used whenver API calls are made - to return 'Server'.
func (ip *ImageRef) ServerUrl() string {
	return fmt.Sprintf("%s://%s", ip.Scheme, ip.Server)
}
