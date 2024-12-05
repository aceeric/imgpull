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

// ImagePull parses the components of an image pull. If url is
// `foo.io/bar/baz:1.2.3` then:
//
//	raw        := foo.io/bar/baz:v1.2.3
//	PullType   := byTag
//	Registry   := foo.io
//	Server     := foo.io
//	Repository := bar/baz
//	Org        := bar
//	Image      := baz
//	Ref        := v1.2.3
type ImagePull struct {
	raw        string
	PullType   pullType
	Registry   string
	Server     string
	Repository string
	Org        string
	Image      string
	Ref        string
	Scheme     string
}

// NewImagePull parses the passed image url (e.g. docker.io/hello-world:latest,
// or docker.io/library/hello-world@sha256:...) into a 'ImagePull' struct. The url
// MUST begin with a registry ref (e.g. quay.io) - it is not (and cannot be) inferred.
func NewImagePull(url, scheme string) (ImagePull, error) {
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

	// TODO CHANGED FROM registry-1 Mon 25th
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
		return ImagePull{}, fmt.Errorf("unable to parse image url: %s", url)
	}

	ref_separators := []struct {
		separator string
		pt        pullType
	}{{separator: "@", pt: byDigest}, {separator: ":", pt: byTag}}

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
		return ImagePull{}, fmt.Errorf("unable to parse image url: %s", url)
	}

	return ImagePull{
		raw:        url,
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

// ImageUrl formats the ImagePull as an image reference like
// 'quay.io/appzygy/ociregistry:1.5.0'
func (ip *ImagePull) ImageUrl() string {
	separator := ":"
	if strings.HasPrefix(ip.Ref, "sha256:") {
		separator = "@"
	}
	if ip.Org == "" {
		return fmt.Sprintf("%s/%s%s%s", ip.Registry, ip.Image, separator, ip.Ref)
	}
	return fmt.Sprintf("%s/%s/%s%s%s", ip.Registry, ip.Org, ip.Image, separator, ip.Ref)
}

func (ip *ImagePull) RegistryUrl() string {
	return fmt.Sprintf("%s://%s", ip.Scheme, ip.Server)
}
