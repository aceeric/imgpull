package imgpull

import (
	"fmt"
	"strings"
)

// imgPullType specifies whether pulling my tag or digest
type imgPullType int

const (
	// Pull by tag
	byTag imgPullType = iota
	// Pull by digest
	byDigest
)

// imageRef has the components of an image reference. Namespace is
// intentionally not in here. This is purely the image reference.
type imageRef struct {
	// e.g.: foo.io/bar/baz:v1.2.3
	raw string
	// if 'Raw' is foo.io/bar/baz:v1.2.3 then 'pullType' is 'byTag'
	pullType imgPullType
	// if 'Raw' is foo.io/bar/baz:v1.2.3 then 'registry' is 'foo.io'
	registry string
	// if 'Raw' is foo.io/bar/baz:v1.2.3 then 'server' is 'foo.io'
	server string
	// if 'Raw' is foo.io/bar/baz:v1.2.3 then 'repository' is 'bar/baz'
	repository string
	// if 'Raw' is foo.io/bar/baz:v1.2.3 then 'org' is 'bar'
	org string
	// if 'Raw' is foo.io/bar/baz:v1.2.3 then 'image' is 'baz'
	image string
	// if 'Raw' is foo.io/bar/baz:v1.2.3 then 'ref' is 'v1.2.3'
	ref string
	// 'http' or 'https'
	scheme string
}

// ImageUrl returns the imageRef receiver URL-related content as an image reference suitable for
// a 'docker pull' command. E.g.: 'quay.io/appzygy/ociregistry:1.5.0'.
func (ip *imageRef) ImageUrl() string {
	return ip.imageUrlWithNs("")
}

// newImageRef parses the passed image url (e.g. docker.io/hello-world:latest,
// or docker.io/library/hello-world@sha256:...) into an 'imageRef' struct. The url
// MUST begin with a registry hostname (e.g. quay.io) - it is not (and cannot be)
// inferred.
func newImageRef(url, scheme string) (imageRef, error) {
	ir := imageRef{
		raw:      strings.ToLower(url),
		pullType: byTag,
		scheme:   strings.ToLower(scheme),
	}
	parts := strings.Split(ir.raw, "/")
	ir.registry = parts[0]
	ir.server = ir.registry
	if ir.registry == "docker.io" {
		ir.server = "index.docker.io"
	}
	if len(parts) == 2 {
		ir.image = parts[1]
		if ir.registry == "docker.io" {
			ir.org = "library"
		}
	} else if len(parts) == 3 {
		ir.org = parts[1]
		ir.image = parts[2]
	} else {
		return imageRef{}, fmt.Errorf("unable to parse image url %q", ir.raw)
	}
	ref_separators := []struct {
		separator string
		pt        imgPullType
	}{
		{separator: "@", pt: byDigest},
		{separator: ":", pt: byTag},
	}
	// split image and tag or digest
	for _, rs := range ref_separators {
		if strings.Contains(ir.image, rs.separator) {
			imgParts := strings.Split(ir.image, rs.separator)
			ir.image = imgParts[0]
			ir.ref = imgParts[1]
			ir.pullType = rs.pt
			break
		}
	}
	if ir.org == "" {
		ir.repository = ir.image
	} else {
		ir.repository = fmt.Sprintf("%s/%s", ir.org, ir.image)
	}
	if ir.image == "" {
		return imageRef{}, fmt.Errorf("unable to parse image url: %q", url)
	}
	return ir, nil
}

// imageUrlWithNs returns the imageRef receiver URL-related content as an image reference suitable for
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
func (ip *imageRef) imageUrlWithNs(namespace string) string {
	separator := ":"
	reg := ip.registry
	if namespace != "" {
		reg = namespace
	}
	if strings.HasPrefix(ip.ref, SHA256PREFIX) {
		separator = "@"
	}
	if ip.org == "" {
		return fmt.Sprintf("%s/%s%s%s", reg, ip.image, separator, ip.ref)
	}
	return fmt.Sprintf("%s/%s/%s%s%s", reg, ip.org, ip.image, separator, ip.ref)
}

// serverUrl handles the case where an image is pulled from docker.io but the package
// has to access the DockerHub API on host index.docker.io so the receiver would have
// a 'Registry' value of docker.io and a 'Server' value of index.docker.io. This function
// is used whenver API calls are made - to return 'Server'.
func (ip *imageRef) serverUrl() string {
	return fmt.Sprintf("%s://%s", ip.scheme, ip.server)
}
