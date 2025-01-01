package imgpull

import (
	"encoding/base64"
	"fmt"
	"imgpull/internal/util"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// HTTP status codes that we will interpret as un-authorized
var unauth = []int{http.StatusUnauthorized, http.StatusForbidden}

// PullTar pulls an image tarball from a registry based on the configuration
// options in the receiver and writes it to the path/file name specified in the
// 'dest' arg.
func (p *Puller) PullTar(dest string) error {
	if dest == "" {
		return fmt.Errorf("no destination specified for pull of %q", p.Opts.Url)
	}
	tmpDir, err := os.MkdirTemp("/tmp", "imgpull.")
	if err != nil {
		return err
	}
	defer os.Remove(tmpDir)
	if itb, err := p.pull(tmpDir); err != nil {
		return err
	} else {
		_, err := itb.toTar(dest)
		return err
	}
}

// PullManifest pulls an image manifest or an image list manifest based on the value
// of the 'mpt' arg. The intended use case if to populate the Puller with a tag-type
// reference. Then, requesting an image manifest will first get a manifest list if
// available, and then select the proper image manifest based on puller OS/arch
// configuration.
func (p *Puller) PullManifest(mpt ManifestPullType) (ManifestHolder, error) {
	if err := p.connect(); err != nil {
		return ManifestHolder{}, err
	}
	rc := p.regCliFrom()
	mh, err := rc.v2Manifests("")
	if err != nil {
		return ManifestHolder{}, err
	}
	if mh.isManifestList() {
		if mpt == ImageList {
			return mh, nil
		}
		digest, err := mh.getImageDigestFor(p.Opts.OStype, p.Opts.ArchType)
		if err != nil {
			return ManifestHolder{}, err
		}
		im, err := rc.v2Manifests(digest)
		if err != nil {
			return ManifestHolder{}, err
		}
		return im, nil
	}
	// if we get here, then the registry did not have a manifest list and so
	// it provided an image manifest
	if mpt == Image {
		return mh, nil
	} else {
		return ManifestHolder{}, fmt.Errorf("server did not provide a manifest for %q", p.ImgRef.ImageUrl())
	}
}

// PullBlobs pulls the blobs for an image, writing them into 'blobDir'.
func (p *Puller) PullBlobs(mh ManifestHolder, blobDir string) error {
	if err := p.connect(); err != nil {
		return err
	}
	rc := p.regCliFrom()
	for _, layer := range mh.layers() {
		if err := rc.v2Blobs(layer, filepath.Join(blobDir, util.DigestFrom(layer.Digest))); err != nil {
			return err
		}
	}
	return nil
}

// HeadManifest does a HEAD request for the image URL in the receiver. The
// 'ManifestDescriptor' returned to the caller contains the image digest,
// media type and manifest size, as provided by the upstream distribution
// server.
func (p *Puller) HeadManifest() (ManifestDescriptor, error) {
	if err := p.connect(); err != nil {
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
	if err := p.connect(); err != nil {
		return ManifestHolder{}, err
	}
	return p.regCliFrom().v2Manifests("")
}

// pull pulls the image specified in the receiver, saving blobs to the passed 'blobDir'.
// An 'imageTarball' struct is returned that describes the pulled image. The directory
// specfied by 'blobDir' will be populated with:
//
//  1. The configuration blob
//  2. The layer blobs.
//
// All blobs are saved into this directory with filenames consisting of 64-character digests.
func (p *Puller) pull(blobDir string) (imageTarball, error) {
	if err := p.connect(); err != nil {
		return imageTarball{}, err
	}
	rc := p.regCliFrom()
	mh, err := rc.v2Manifests("")
	if err != nil {
		return imageTarball{}, err
	}
	if mh.isManifestList() {
		digest, err := mh.getImageDigestFor(p.Opts.OStype, p.Opts.ArchType)
		if err != nil {
			return imageTarball{}, err
		}
		im, err := rc.v2Manifests(digest)
		if err != nil {
			return imageTarball{}, err
		}
		mh = im
	}
	for _, layer := range mh.layers() {
		if rc.v2Blobs(layer, filepath.Join(blobDir, util.DigestFrom(layer.Digest))) != nil {
			return imageTarball{}, err
		}
	}
	return mh.newImageTarball(p.ImgRef, p.Opts.Namespace, blobDir)
}

// possible future use
// // GetManifestByDigest gets an image manifest by digest. Basically when building
// // the API url it replaces the tag in the receiver with the passed digest. This
// // function always returns an image manifest if one is available matching the
// // passed digest.
// func (p *Puller) GetManifestByDigest(digest string) (ManifestHolder, error) {
// 	if err := p.connect(); err != nil {
// 		return ManifestHolder{}, err
// 	}
// 	return p.regCliFrom().v2Manifests(digest)
// }

// connect calls the 'v2' endpoint and looks for an auth header. If an auth
// header is provided by the remote registry then this function will attempt
// to negotiate the auth handshake for Bearer if the remote requests it, or
// Basic using the user/pass in the receiver. Once successfully authenticated,
// the auth credential (bearer token or encrypted user/pass) are retained in
// the receiver for all the other API methods to build an auth header with.
//
// If the function has already been called on the receiver, it immediately
// returns taking no action.
func (p *Puller) connect() error {
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

// regCliFrom creates a 'RegClient' from the receiver, consisting of a subset of receiver
// fields needed to interact with the OCI Distribution Server V2 REST API. It supports
// a looser coupling of the Puller from actually interacting with the distribution server.
//
// If this function is intended to return a RegClient to make API calls that require auth
// headers, then the Connect function must previously have been called on the receiver so
// that the auth struct in the receiver is initialized by virtue of that call. The auth
// struct is copied into the returned RegClient struct which is used to set auth headers.
func (p *Puller) regCliFrom() regClient {
	c := regClient{
		imgRef:    p.ImgRef,
		client:    p.Client,
		namespace: p.Opts.Namespace,
	}
	if k, v := p.authHdr(); k != "" {
		c.authHdr = authHeader{
			key:   k,
			value: v,
		}
	}
	return c
}

// parseBearer parses the passed auth header which the caller should ensure is a bearer
// type "www-authenticate" header like:
//
//	Bearer realm="https://auth.docker.io/token",service="registry.docker.io"
//
// The function returns the parsed result in the 'BearerAuth' struct.
func parseBearer(authHdr string) bearerAuth {
	ba := bearerAuth{}
	parts := []string{"realm", "service"}
	expr := `%s[\s]*=[\s]*"{1}([0-9A-Za-z\-:/.,]*)"{1}`
	for _, part := range parts {
		srch := fmt.Sprintf(expr, part)
		m := regexp.MustCompile(srch)
		matches := m.FindStringSubmatch(authHdr)
		if len(matches) == 2 {
			if part == "realm" {
				ba.realm = strings.ReplaceAll(matches[1], "\"", "")
			} else {
				ba.service = strings.ReplaceAll(matches[1], "\"", "")
			}
		}
	}
	return ba
}
