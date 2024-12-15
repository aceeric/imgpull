package imgpull

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/opencontainers/go-digest"
)

const (
	mebibytes        = 1024 * 1024
	maxManifestBytes = 100 * mebibytes
	maxBlobBytes     = 100 * mebibytes
)

// AuthHeader is a key/value struct that supports creating and setting an auth
// header for the supported auth type (basic, bearer).
type AuthHeader struct {
	key   string
	value string
}

// RegClient has everything needed to talk to an OCI Distribution server for the purposes
// of pulling an image.
type RegClient struct {
	// ImgRef is the parsed image url, e.g.: 'docker.io/hello-world:latest'
	ImgRef ImageRef
	// Client is the HTTP client
	Client *http.Client
	// Namespace supports pull-through
	Namespace string
	// AuthHdr supports the various auth types (basic, bearer)
	AuthHdr AuthHeader
}

// allManifestTypes lists all of the manifest types that this package
// will operate on.
var allManifestTypes []string = []string{
	V2dockerManifestListMt,
	V2dockerManifestMt,
	V1ociIndexMt,
	V1ociManifestMt,
}

// calls the 'v2' endpoint which typically either returns OK or unauthorized. It is the first
// API call made to an OCI Distribution server to initiate an image pull.
func (c RegClient) v2() (int, []string, error) {
	url := fmt.Sprintf("%s/v2/", c.ImgRef.ServerUrl())
	resp, err := c.Client.Head(url)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return 0, nil, err
	}
	auth := getWwwAuthenticateHdrs(resp)
	return resp.StatusCode, auth, err
}

// v2Basic calls the 'v2' endpoint with a basic auth header formed from
// the username and password encoded in the passed string. If successful, the
// credentials are returned to the caller for use on subsequent calls.
func (c RegClient) v2Basic(encoded string) (BasicAuth, error) {
	url := fmt.Sprintf("%s/v2/", c.ImgRef.ServerUrl())
	req, _ := http.NewRequest(http.MethodHead, url, nil)
	req.Header.Set("Authorization", "Basic "+encoded)
	resp, err := c.Client.Do(req)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return BasicAuth{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return BasicAuth{}, fmt.Errorf("basic auth returned status code %d", resp.StatusCode)
	}
	return BasicAuth{Encoded: encoded}, nil
}

// v2Auth calls the 'v2/auth' endpoint with the passed bearer struct which has
// realm and service. These are used to build the auth URL. The realm might be different
// than the server that we have been requested to pull from.  If successful, the
// bearer token is returned to the caller for use on subsequent calls.
func (c RegClient) v2Auth(ba BearerAuth) (BearerToken, error) {
	url := fmt.Sprintf("%s?scope=repository:%s:pull&service=%s", ba.Realm, c.ImgRef.Repository, ba.Service)
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	resp, err := c.Client.Do(req)
	if err != nil {
		return BearerToken{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return BearerToken{}, fmt.Errorf("auth attempt failed. Status: %d", resp.StatusCode)
	}
	if resp != nil {
		defer resp.Body.Close()
	}
	var token BearerToken
	decoder := json.NewDecoder(resp.Body)
	err = decoder.Decode(&token)
	if err != nil {
		return BearerToken{}, err
	}
	return token, nil
}

// v2Blobs calls the 'v2/<repository>/blobs' endpoint to get a blob by the digest in the passed
// 'layer' arg. The blob is stored in the location specified by 'toPath'. The 'isConfig'
// var indicates that the blob is a config blob. Objects are stored with their digest as the file
// name.
func (c RegClient) v2Blobs(layer Layer, toPath string, isConfig bool) error {
	url := fmt.Sprintf("%s/v2/%s/blobs/%s%s", c.ImgRef.ServerUrl(), c.ImgRef.Repository, layer.Digest, c.nsQueryParm())
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	c.setAuthHdr(req)
	resp, err := c.Client.Do(req)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return err
	}
	fName := filepath.Join(toPath, layer.Digest)
	if !isConfig {
		fName = strings.Replace(filepath.Join(fName+".tar.gz"), "sha256:", "", -1)
	}
	blobFile, err := os.Create(fName)
	if err != nil {
		return err
	}
	defer blobFile.Close()

	bytesRead := 0
	for {
		part, err := io.ReadAll(io.LimitReader(resp.Body, maxBlobBytes))
		if err != nil {
			return err
		}
		if len(part) == 0 {
			break
		}
		bytesRead += len(part)
		blobFile.Write(part)
	}
	if bytesRead != layer.Size {
		return fmt.Errorf("error getting blob - expected %d bytes, got %d bytes instead", layer.Size, bytesRead)
	}
	return nil
}

// v2Manifests calls the 'v2/<repository>/manifests' endpoint. The resulting manifest is returned in
// a ManifestHolder struct and could be any one of the types defined in the 'allManifestTypes' array.
// If you pass an empty string in 'sha', then the GET will use the image url that was used to initialize
// the Puller. (Probably a tag.) If you provide a digest in 'sha', the digest will override the tag.
//
// Generally speaking: pull by tag returns an image list from the registry if one is available and pull
// by digest (SHA) returns an image manifest. But this might not be true all the time.
func (c RegClient) v2Manifests(sha string) (ManifestHolder, error) {
	ref := c.ImgRef.Ref
	if sha != "" {
		ref = sha
	}
	url := fmt.Sprintf("%s/v2/%s/manifests/%s%s", c.ImgRef.ServerUrl(), c.ImgRef.Repository, ref, c.nsQueryParm())
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("Accept", strings.Join(allManifestTypes, ","))
	c.setAuthHdr(req)
	resp, err := c.Client.Do(req)
	if err != nil {
		return ManifestHolder{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return ManifestHolder{}, fmt.Errorf("get manifests attempt failed. Status: %d", resp.StatusCode)
	}
	if resp != nil {
		defer resp.Body.Close()
	}
	mediaType := resp.Header.Get("Content-Type")
	manifestBytes, err := io.ReadAll(io.LimitReader(resp.Body, maxManifestBytes))
	if err != nil {
		return ManifestHolder{}, err
	}
	expDigest := resp.Header.Get("Docker-Content-Digest")
	if expDigest != "" {
		actDigest := digest.FromBytes(manifestBytes).Hex()
		if actDigest != digestFrom(expDigest) {
			return ManifestHolder{}, fmt.Errorf("digest mismatch for %q", ref)
		}
	}
	mh, err := NewManifestHolder(mediaType, manifestBytes)
	return mh, err
}

// v2ManifestsHead is like v2Manifests but does a HEAD request. The result is returned in a
// smaller struct with only media type, digest, and size (of manifest).
func (c RegClient) v2ManifestsHead() (ManifestDescriptor, error) {
	url := fmt.Sprintf("%s/v2/%s/manifests/%s%s", c.ImgRef.ServerUrl(), c.ImgRef.Repository, c.ImgRef.Ref, c.nsQueryParm())
	req, _ := http.NewRequest(http.MethodHead, url, nil)
	req.Header.Set("Accept", strings.Join(allManifestTypes, ","))
	c.setAuthHdr(req)
	resp, err := c.Client.Do(req)
	if err != nil {
		return ManifestDescriptor{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return ManifestDescriptor{}, fmt.Errorf("head manifests for %q failed with status %q", url, resp.StatusCode)
	}
	if resp != nil {
		defer resp.Body.Close()
	}
	mediaType := resp.Header.Get("Content-Type")
	if mediaType == "" {
		return ManifestDescriptor{}, fmt.Errorf("head manifests for %q did not return content type", url)
	}
	digest := resp.Header.Get("Docker-Content-Digest")
	if digest == "" {
		return ManifestDescriptor{}, fmt.Errorf("head manifests for %q did not return digest", url)
	}
	return ManifestDescriptor{
		MediaType: mediaType,
		Digest:    digest,
		Size:      int(resp.ContentLength),
	}, nil
}

// setAuthHdr sets an auth header (e.g. "Bearer", "Basic") on the passed request
// if the receiver is configured with such a header.
func (c RegClient) setAuthHdr(req *http.Request) {
	if c.AuthHdr != (AuthHeader{}) {
		req.Header.Set(c.AuthHdr.key, c.AuthHdr.value)
	}
}

// nsQueryParm checks if the receiver is configured with a namespace for pull-through,
// and if it is, returns the namespace as a query param in the form: '?ns=X' where 'X'
// is the receiver's namespace. If no namespace is configured, then the function
// returns the empty string.
func (c RegClient) nsQueryParm() string {
	if c.Namespace != "" {
		return "?ns=" + c.Namespace
	} else {
		return ""
	}
}

// getWwwAuthenticateHdrs gets all "www-authenticate" headers from
// the passed response.
func getWwwAuthenticateHdrs(r *http.Response) []string {
	hdrs := []string{}
	for key, vals := range r.Header {
		for _, val := range vals {
			if strings.ToLower(key) == "www-authenticate" {
				hdrs = append(hdrs, val)
			}
		}
	}
	return hdrs
}
