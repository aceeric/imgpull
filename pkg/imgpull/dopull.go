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

// HTTP status codes that we will interpret as un-authorized
var unauth = []int{http.StatusUnauthorized, http.StatusForbidden}

// PullTar pulls an image tarball from a registry based on the configuration
// options in the receiver and writes it to the file name (and optionally path
// name) specified in the 'dest' arg.
func (p *Puller) PullTar(dest string) error {
	if dest == "" {
		return fmt.Errorf("no destination specified for pull of %q", p.Opts.Url)
	}
	tmpDir, err := os.MkdirTemp("/tmp", "imgpull.")
	if err != nil {
		return err
	}
	defer os.Remove(tmpDir)
	if dtm, err := p.Pull(tmpDir); err != nil {
		return err
	} else {
		return dtm.toTar(tmpDir, dest)
	}
}

// PullManifest pulls an image manifest or an image list manifest based on the value
// of the 'mpt' arg.
func (p *Puller) PullManifest(mpt ManifestPullType) (ManifestHolder, error) {
	if err := p.Connect(); err != nil {
		return ManifestHolder{}, err
	}
	rc := p.regCliFrom()
	mh, err := rc.v2Manifests("")
	if err != nil {
		return ManifestHolder{}, err
	}
	if mh.IsManifestList() {
		if mpt == ImageList {
			return mh, nil
		}
		digest, err := mh.GetImageDigestFor(p.Opts.OStype, p.Opts.ArchType)
		if err != nil {
			return ManifestHolder{}, err
		}
		im, err := rc.v2Manifests(digest)
		if err != nil {
			return ManifestHolder{}, err
		}
		return im, nil
	}
	if mpt == Image {
		return mh, nil
	} else {
		return ManifestHolder{}, fmt.Errorf("server did not provide a manifest list for %q", p.ImgRef.ImageUrl())
	}
}

// Pull pulls the image specified in the receiver to the passed 'toPath'. A
// 'DockerTarManifest' is returned that describes the pulled image. The directory
// specfied by'toPath' will be populated with:
//
//  1. The image list manifest for the URL,
//  2. The image manifest.
//  3. The docker TAR manifest (the same one returned from the function), and
//  4. The blobs, including the configuration blob.
//
// In other words everything needed to create a tarball that looks like a
// 'docker save' tarball.
//
// The image list manifest and the image manifest don't get included in the
// image tarball but they are populated in the directory in case the caller
// wants them.
func (p *Puller) Pull(toPath string) (DockerTarManifest, error) {
	if err := p.Connect(); err != nil {
		return DockerTarManifest{}, err
	}
	rc := p.regCliFrom()
	mh, err := rc.v2Manifests("")
	if err != nil {
		return DockerTarManifest{}, err
	}
	if mh.IsManifestList() {
		err := mh.saveManifest(toPath, "image-index.json")
		if err != nil {
			return DockerTarManifest{}, err
		}
		digest, err := mh.GetImageDigestFor(p.Opts.OStype, p.Opts.ArchType)
		if err != nil {
			return DockerTarManifest{}, err
		}
		im, err := rc.v2Manifests(digest)
		if err != nil {
			return DockerTarManifest{}, err
		}
		mh = im
	}
	err = mh.saveManifest(toPath, "image.json")
	if err != nil {
		return DockerTarManifest{}, err
	}
	configDigest, err := mh.GetImageConfig()
	if err != nil {
		return DockerTarManifest{}, err
	}
	// get the config blob to the file system
	if err := rc.v2Blobs(configDigest, toPath, true); err != nil {
		return DockerTarManifest{}, err
	}
	// get the layer blobs to the file system
	for _, layer := range mh.Layers() {
		if err := rc.v2Blobs(layer, toPath, false); err != nil {
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

// HeadManifest does a HEAD request for the image URL in the receiver. The
// 'ManifestDescriptor' returned to the caller contains the image digest,
// media type and manifest size, as provided by the upstream distribution
// server.
func (p *Puller) HeadManifest() (ManifestDescriptor, error) {
	if err := p.Connect(); err != nil {
		return ManifestDescriptor{}, err
	}
	return p.regCliFrom().v2ManifestsHead()
}

// GetManifest gets a manifest for the image in the receiver. If the receiver
// is configured with a tag then the manifest returned is determined by the
// upstream registry: if an image list manifest is available, it will be provided by
// the registry. If no image list manifest is available then an image manifest
// will be provided by the registry if available. Whatever the registry provides
// is returned in a 'ManifestHolder' which holds all four supported manifest types.
func (p *Puller) GetManifest() (ManifestHolder, error) {
	if err := p.Connect(); err != nil {
		return ManifestHolder{}, err
	}
	return p.regCliFrom().v2Manifests("")
}

// GetManifestByDigest gets an image manifest by digest. Basically when building
// the API url it replaces the tag in the receiver with the passed digest. This
// function always returns an image manifest if one is available matching the
// passed digest.
func (p *Puller) GetManifestByDigest(digest string) (ManifestHolder, error) {
	if err := p.Connect(); err != nil {
		return ManifestHolder{}, err
	}
	return p.regCliFrom().v2Manifests(digest)
}

// Connect calls the 'v2' endpoint and looks for an auth header. If an auth
// header is provided by the remote registry then this function will attempt
// to negotiate the auth handshake for Bearer if the remote requests it, or
// Basic using the user/pass in the receiver. Once successfully authenticated,
// the auth credential (bearer token or encrypted user/pass) are retained in
// the receiver for all the other API methods to build an auth header with.
//
// If the function has already been called on the receiver, it immediately
// returns taking no action.
func (p *Puller) Connect() error {
	if p.Connected {
		return nil
	}
	status, auth, err := p.regCliFrom().v2()
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
	rc := p.regCliFrom()
	for _, hdr := range auth {
		if strings.HasPrefix(strings.ToLower(hdr), "bearer") {
			ba := parseBearer(hdr)
			t, err := rc.v2Auth(ba)
			if err != nil {
				return err
			}
			p.Token = t
			return nil
		} else if strings.HasPrefix(strings.ToLower(hdr), "basic") {
			delimited := fmt.Sprintf("%s:%s", p.Opts.Username, p.Opts.Password)
			encoded := base64.StdEncoding.EncodeToString([]byte(delimited))
			ba, err := rc.v2Basic(encoded)
			if err != nil {
				return err
			}
			p.Basic = ba
			return nil
		}
	}
	return fmt.Errorf("unable to parse auth param: %v", auth)
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

// regCliFrom creates a 'RegClient' from the receiver, consisting of a subset of receiver
// fields needed to interact with the OCI Distribution Server V2 REST API. It supports
// a looser coupling of the Puller from actually interacting with the distribution server.
//
// If this function is intended to return a RegClient to make API calls that require auth
// headers, then the Connect function must previously have been called on the receiver so
// that the auth struct in the receiver is initialized by virtue of that call. The auth
// struct is copied into the returned RegClient struct which is used to set auth headers.
func (p *Puller) regCliFrom() RegClient {
	c := RegClient{
		ImgRef:    p.ImgRef,
		Client:    p.Client,
		Namespace: p.Opts.Namespace,
	}
	if k, v := p.authHdr(); k != "" {
		c.AuthHdr = AuthHeader{
			key:   k,
			value: v,
		}
	}
	return c
}
