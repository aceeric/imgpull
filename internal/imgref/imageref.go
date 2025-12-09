package imgref

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/aceeric/imgpull/internal/util"
)

// imgPullType specifies whether pulling my tag or digest
type imgPullType int

const (
	// Undefined pull type
	undefinedPullType imgPullType = iota
	// Pull by tag
	byTag
	// Pull by digest
	byDigest
)

// ImageRef has the components of an image reference.
type ImageRef struct {
	// if input is foo.io/bar/baz:v1.2.3 then 'registry' is 'foo.io'
	registry string
	// if input is foo.io/bar/baz:v1.2.3 then 'pullType' is 'byTag'
	pullType imgPullType
	// if input is foo.io/bar/baz:v1.2.3 then 'server' is 'foo.io'
	server string
	// if input is foo.io/bar/baz:v1.2.3 then 'repository' is 'bar/baz'
	repository string
	// if input is foo.io/bar/baz:v1.2.3 then 'ref' is 'v1.2.3'
	ref string
	// 'http' or 'https'
	scheme string
	// namespace supports pull-through and mirroring, i.e. pull
	// 'localhost:5000/hello-world:latest' with namespace 'docker.io' to
	// pull from localhost if localhost is a mirror or a pull-through
	// registry.
	namespace string
	// if the url was provided with the namespace in the path like
	// localhost:8080/docker.io/hello-world:latest then this is set to
	// true, else it is false.
	nsInPath bool
	// like when docker.io/hello-world is requested then have
	// to talk to docker api with .../library/hello-world/...
	library bool
}

var (
	digestRe   = regexp.MustCompile(`(.*)@(sha256:[a-f0-9]{64})\b`)
	tagRe      = regexp.MustCompile(`(.*):(.*)\b`)
	dockerRegs = []string{"docker.io", "index.docker.io"}
)

// NewImageRef parses the passed image url (e.g. docker.io/hello-world:latest) into
// an 'imageRef' struct. The url MUST begin with a registry hostname (e.g. quay.io or
// localhost:8080) - it is not (and cannot be) inferred.
func NewImageRef(url, scheme, namespace string) (ImageRef, error) {
	ir := ImageRef{
		scheme:    scheme,
		namespace: namespace,
	}
	before, after, found := strings.Cut(url, "/")
	if !found || after == "" {
		return ImageRef{}, fmt.Errorf("unable to parse image url %q (at least two segments required)", url)
	}
	ir.registry = before
	ir.server = ir.registry
	if ir.server == "docker.io" {
		ir.server = "index.docker.io"
	}
	// check for in-path namespace
	ns, remainder, found := strings.Cut(after, "/")
	if found && strings.Contains(ns, ".") {
		ir.namespace = ns
		after = remainder
		ir.nsInPath = true
	}
	remainder, ref, pullType := parseAfterReg(after)
	ir.pullType = pullType
	ir.ref = ref
	ir.repository = remainder
	if strings.Contains(ir.repository, ".") {
		return ImageRef{}, fmt.Errorf("unable to parse image url %q (period in repository not allowed)", url)

	}
	_, _, found = strings.Cut(ir.repository, "/")
	if !found && slices.Contains(dockerRegs, ir.server) {
		// pulling from dockerhub without bare repo like "hello-world" and
		// without "library/" in the repository name
		ir.library = true
	}
	return ir, nil
}

// Repository  returns the image url as it is valid to use in upstream API calls.
// In all cases except pulling from docker.io the function simply returns the
// repository. But if docker.io AND the incoming url did not have "library" in it
// then its return with "library/" prepended.
func (ir *ImageRef) Repository() string {
	if ir.library {
		return strings.Join([]string{"library", ir.repository}, "/")
	}
	return ir.repository
}

// Namespace gets the namespace.
func (ir *ImageRef) Namespace() string {
	return ir.namespace
}

// Namespace gets the namespace.
func (ir *ImageRef) Ref() string {
	return ir.ref
}

// Namespace gets the namespace.
func (ir *ImageRef) Registry() string {
	return ir.registry
}

// Namespace gets the namespace.
func (ir *ImageRef) NsInPath() bool {
	return ir.nsInPath
}

// Url returns the image url in the receiver exactly as represented in
// the receiver.
func (ir *ImageRef) Url() string {
	return ir.makeUrl("", false)
}

// UrlWithNs returns the image url in the receiver with registry component
// replaced by the namespace in the receiver if the namespace is non-empty.
// E.g. if the image url used to actually pull an image is
// 'localhost:8080/jetstack/cert-manager-controller:v1.16.2' and the namespace
// in the receiver is 'quay.io' then the function returns:
// quay.io/jetstack/cert-manager-controller:v1.16.2"
func (ir *ImageRef) UrlWithNs() string {
	return ir.makeUrl("", true)
}

// UrlWithDigest returns the image url in the receiver allowing to override
// the image reference (i.e. tag) in the receiver with the passed digest.
func (ir *ImageRef) UrlWithDigest(digest string) string {
	return ir.makeUrl(digest, false)
}

// ServerUrl handles the case where an image is pulled from docker.io but the package
// has to access the DockerHub API on host index.docker.io so the receiver would have
// a 'Registry' value of docker.io and a 'Server' value of index.docker.io. This function
// is used whenver API calls are made - to return 'Server'. This seems to be unique to
// DockerHub.
func (ir *ImageRef) ServerUrl() string {
	return fmt.Sprintf("%s://%s", ir.scheme, ir.server)
}

// parseAfterReg tries to parse the passed string as having either a digest reference or
// a tag reference. If neither then it is treated as by tag with tag "latest".
func parseAfterReg(urlPart string) (string, string, imgPullType) {
	if result := digestRe.FindStringSubmatch(urlPart); len(result) == 3 {
		return result[1], result[2], byDigest
	} else if result := tagRe.FindStringSubmatch(urlPart); len(result) == 3 {
		return result[1], result[2], byTag
	}
	return urlPart, "latest", byTag
}

// makeUrl does the actual work for 'ImageUrl', 'UrlWithNs', and
// 'UrlWithDigest'
func (ir *ImageRef) makeUrl(sha string, withNs bool) string {
	regToUse := ir.registry
	if withNs && ir.namespace != "" {
		regToUse = ir.namespace
	}
	var refToUse string
	if strings.HasPrefix(ir.ref, "sha256:") {
		refToUse = "@" + ir.ref
	} else if sha != "" {
		refToUse = "@sha256:" + util.DigestFrom(sha)
	} else {
		refToUse = ":" + ir.ref
	}
	return fmt.Sprintf("%s/%s%s", regToUse, ir.repository, refToUse)
}
