package imgpull

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"slices"
	"strings"
)

// PullTar pulls an image tarball from a registry based on the configuration
// options in the receiver.
func (p *Puller) PullTar() error {
	tmpDir, err := os.MkdirTemp("/tmp", "imgpull.")
	if err != nil {
		return err
	}
	defer func() {
		err := os.Remove(tmpDir)
		if err != nil {
			// what can be done
		}
	}()
	if dtm, err := p.Pull(tmpDir); err != nil {
		return err
	} else {
		return dtm.toTar(p.Opts.Dest, tmpDir)
	}
}

// Pull pulls the image specified in the receiver to the passed 'toPath'. A
// 'DockerTarManifest' is returned that describes the pulled image. This return
// val can be provided to the 'toTar' function to create a tarball, just as
// 'docker save' would do.
//
// The directory specfied by'toPath' will be populated with the image list
// manifest for the URL, the image manifest, the docker TAR manifest, and
// the blobs, including the configuration blob. In other words everything needed
// to create a tarball that looks like a 'docker save' tarball.
func (p *Puller) Pull(toPath string) (DockerTarManifest, error) {
	if err := p.Connect(); err != nil {
		return DockerTarManifest{}, err
	}
	mh, err := p.v2Manifests("")
	if err != nil {
		return DockerTarManifest{}, err
	}
	if mh.IsManifestList() {
		err := saveManifest(mh, toPath, "image-index.json")
		if err != nil {
			return DockerTarManifest{}, err
		}
		digest, err := mh.GetImageDigestFor(p.Opts.OSType, p.Opts.ArchType)
		if err != nil {
			return DockerTarManifest{}, err
		}
		im, err := p.v2Manifests(digest)
		if err != nil {
			return DockerTarManifest{}, err
		}
		mh = im
	}
	err = saveManifest(mh, toPath, "image.json")
	if err != nil {
		return DockerTarManifest{}, err
	}
	configDigest, err := mh.GetImageConfig()
	if err != nil {
		return DockerTarManifest{}, err
	}
	// get the config blob to the file system
	if err := p.v2Blobs(configDigest, toPath, true); err != nil {
		return DockerTarManifest{}, err
	}
	// get the layer blobs to the file system
	for _, layer := range mh.Layers() {
		if err := p.v2Blobs(layer, toPath, false); err != nil {
			return DockerTarManifest{}, err
		}
	}
	dtm, err := mh.NewDockerTarManifest(p.ImgRef, p.Opts.Namespace)
	if err != nil {
		return DockerTarManifest{}, err
	}
	dtm.saveDockerTarManifest(toPath, "manifest.json")

	return dtm, nil
}

// HeadManifest does a HEAD requests for the image URL in the receiver. The
// 'ManifestDescriptor' returned to the caller contains the image digest,
// media type and manifest size.
func (p *Puller) HeadManifest() (ManifestDescriptor, error) {
	if err := p.Connect(); err != nil {
		return ManifestDescriptor{}, err
	}
	return p.v2ManifestsHead()
}

// GetManifest gets a manifest for the image in the receiver. If the receiver
// is configured with a tag then the manifest returned is determined by the
// registry. If an image list manifest is available, it will be provided by
// the registry. If no image list manifest is available then an image manifest
// will be provided by the registry. Whatever the registry provides is returned
// in a 'ManifestHolder' which holds all four supported manifest types.
func (p *Puller) GetManifest() (ManifestHolder, error) {
	if err := p.Connect(); err != nil {
		return ManifestHolder{}, err
	}
	return p.v2Manifests("")
}

// GetManifestByDigest gets an image manifest by digest. Basically when building
// the API url it replaces the tag in the receiver with the passed digest. This
// function always returns an image manifest if one is available matching the
// passed digest.
func (p *Puller) GetManifestByDigest(digest string) (ManifestHolder, error) {
	if err := p.Connect(); err != nil {
		return ManifestHolder{}, err
	}
	return p.v2Manifests(digest)
}

// Connect calls the 'v2' endpoint and looks for an auth header. If an auth
// header is provided by the remote registry then this function will attempt
// to negotiate the auth handshake for Bearer if the remote requests it, or
// Basic using the user/pass in the receiver. Once successfully authenticated,
// the auth credential (bearer token or encrypted user/pass) are retained in
// the receiver for all the other API methods to build an auth header with.
func (p *Puller) Connect() error {
	// HTTP status codes that we will interpret as un-authorized
	unauth := []int{http.StatusUnauthorized, http.StatusForbidden}

	if p.Connected {
		return nil
	}
	status, auth, err := p.v2()
	if err != nil {
		return err
	}
	if status != http.StatusOK && slices.Contains(unauth, status) {
		err := p.authenticate(auth)
		if err != nil {
			return err
		}
	}
	p.Connected = true
	return nil
}

// authenticate scans the passed list of auth headers received from a distribution
// server and attempts to perform authentication for each in the following order:
//
//  1. bearer
//  2. basic (using the user/pass that the puller receiver was initialized from)
//
// If successful then the receiver is initialized with the corresponding auth
// struct so that it is available to be used for all subsequent API calls to the
// distribution server. For example if 'bearer' then the token received from the
// remote registry will be added to the receiver.
func (p *Puller) authenticate(auth []string) error {
	for _, hdr := range auth {
		if strings.HasPrefix(strings.ToLower(hdr), "bearer") {
			ba := parseBearer(hdr)
			return p.v2Auth(ba)
		} else if strings.HasPrefix(strings.ToLower(hdr), "basic") {
			delimited := fmt.Sprintf("%s:%s", p.Opts.Username, p.Opts.Password)
			encoded := base64.StdEncoding.EncodeToString([]byte(delimited))
			return p.v2Basic(encoded)
		}
	}
	return fmt.Errorf("unable to parse auth param: %v", auth)
}

// saveManifest extracts the manifest from the passed 'ManifestHolder' and
// saves it to a file with the passed name in the passed path.
func saveManifest(mh ManifestHolder, toPath string, name string) error {
	json, err := mh.ToString()
	if err != nil {
		return err
	}
	return saveFile([]byte(json), toPath, name)
}

// parseBearer parses the passed auth header which the caller should ensure is a bearer
// type "www-authenticate" header like:
//
//	Bearer realm="https://auth.docker.io/token",service="registry.docker.io"
//
// The function returns the parsed result in the 'BearerAuth' struct.
func parseBearer(authHdr string) BearerAuth {
	ba := BearerAuth{}
	parts := []string{"realm", "service"}
	expr := `%s[\s]*=[\s]*"{1}([0-9A-Za-z\-:/.,]*)"{1}`
	for _, part := range parts {
		srch := fmt.Sprintf(expr, part)
		m := regexp.MustCompile(srch)
		matches := m.FindStringSubmatch(authHdr)
		if len(matches) == 2 {
			if part == "realm" {
				ba.Realm = strings.ReplaceAll(matches[1], "\"", "")
			} else {
				ba.Service = strings.ReplaceAll(matches[1], "\"", "")
			}
		}
	}
	return ba
}
