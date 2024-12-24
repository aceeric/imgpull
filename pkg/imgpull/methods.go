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

// authHeader is a key/value struct that supports creating and setting an auth
// header for the supported auth type (basic, bearer).
type authHeader struct {
	key   string
	value string
}

// regClient has everything needed to talk to an OCI Distribution server for the purposes
// of pulling an image.
type regClient struct {
	// imgRef is the parsed image url, e.g.: 'docker.io/hello-world:latest'
	imgRef imageRef
	// client is the HTTP client
	client *http.Client
	// namespace supports pull-through
	namespace string
	// authHdr supports the various auth types (basic, bearer)
	authHdr authHeader
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
// API call made to an OCI Distribution server to initiate an image pull. Returns the http
// status code, an array of auth headers (which could be empty), and an error if one occurred
// or nil.
func (rc regClient) v2() (int, []string, error) {
	url := fmt.Sprintf("%s/v2/", rc.imgRef.serverUrl())
	resp, err := rc.client.Head(url)
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
func (rc regClient) v2Basic(encoded string) (BasicAuth, error) {
	url := fmt.Sprintf("%s/v2/", rc.imgRef.serverUrl())
	req, _ := http.NewRequest(http.MethodHead, url, nil)
	req.Header.Set("Authorization", "Basic "+encoded)
	resp, err := rc.client.Do(req)
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
func (rc regClient) v2Auth(ba BearerAuth) (BearerToken, error) {
	url := fmt.Sprintf("%s?scope=repository:%s:pull&service=%s", ba.Realm, rc.imgRef.repository, ba.Service)
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	resp, err := rc.client.Do(req)
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
func (rc regClient) v2Blobs(layer Layer, toPath string) error {
	fName, exists := checkBlobExists(layer, toPath)
	if exists {
		return nil
	}
	url := fmt.Sprintf("%s/v2/%s/blobs/%s%s", rc.imgRef.serverUrl(), rc.imgRef.repository, layer.Digest, rc.nsQueryParm())
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	rc.setAuthHdr(req)
	resp, err := rc.client.Do(req)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return err
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

// checkBlobExists builds a blob path from the passed args and returns the file name,
// along with true if the file already exists, else false.
func checkBlobExists(layer Layer, toPath string) (string, bool) {
	fName := filepath.Join(toPath, digestFrom(layer.Digest))
	if _, err := os.Stat(fName); err == nil {
		return fName, true
	}
	return fName, false
}

// v2Manifests calls the 'v2/<repository>/manifests' endpoint. The resulting manifest is returned in
// a ManifestHolder struct and could be any one of the types defined in the 'allManifestTypes' array.
// If you pass an empty string in 'sha', then the GET will use the image url that was used to initialize
// the Puller. (Probably a tag.) If you provide a digest in 'sha', the digest will override the tag.
//
// Generally speaking: pull by tag returns an image list from the registry if one is available and pull
// by digest (SHA) returns an image manifest. But this might not be true all the time.
func (rc regClient) v2Manifests(sha string) (ManifestHolder, error) {
	ref := rc.imgRef.ref
	if sha != "" {
		ref = sha
	}
	url := fmt.Sprintf("%s/v2/%s/manifests/%s%s", rc.imgRef.serverUrl(), rc.imgRef.repository, ref, rc.nsQueryParm())
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("Accept", strings.Join(allManifestTypes, ","))
	rc.setAuthHdr(req)
	resp, err := rc.client.Do(req)
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
	manifestDigest := resp.Header.Get("Docker-Content-Digest")
	computedDigest := digest.FromBytes(manifestBytes).Hex()
	if manifestDigest == "" {
		manifestDigest = computedDigest
	} else {
		manifestDigest = digestFrom(manifestDigest)
		if computedDigest != manifestDigest {
			return ManifestHolder{}, fmt.Errorf("digest mismatch for %q", ref)
		}
	}
	mh, err := NewManifestHolder(mediaType, manifestBytes, manifestDigest, rc.makeUrl(sha))
	return mh, err
}

// v2ManifestsHead is like v2Manifests but does a HEAD request. The result is returned in a
// smaller struct with only media type, digest, and size (of manifest).
func (rc regClient) v2ManifestsHead() (ManifestDescriptor, error) {
	url := fmt.Sprintf("%s/v2/%s/manifests/%s%s", rc.imgRef.serverUrl(), rc.imgRef.repository, rc.imgRef.ref, rc.nsQueryParm())
	req, _ := http.NewRequest(http.MethodHead, url, nil)
	req.Header.Set("Accept", strings.Join(allManifestTypes, ","))
	rc.setAuthHdr(req)
	resp, err := rc.client.Do(req)
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
func (rc regClient) setAuthHdr(req *http.Request) {
	if rc.authHdr != (authHeader{}) {
		req.Header.Set(rc.authHdr.key, rc.authHdr.value)
	}
}

// nsQueryParm checks if the receiver is configured with a namespace for pull-through,
// and if it is, returns the namespace as a query param in the form: '?ns=X' where 'X'
// is the receiver's namespace. If no namespace is configured, then the function
// returns the empty string.
func (rc regClient) nsQueryParm() string {
	if rc.namespace != "" {
		return "?ns=" + rc.namespace
	} else {
		return ""
	}
}

// makeUrl makes an image ref like docker.io/hello-world:latest. If the receiver
// has a namespace, then the namespace is used for the registry instead of the
// registry in the receiver. If the reference (the tag or digest) in the receiver
// then it is used, else if the passed sha is not the empty string, then it is
// used, otherwise the ref in the receiver (which could be a tag or a digest)
// is used.
func (rc regClient) makeUrl(sha string) string {
	regToUse := rc.imgRef.registry
	if rc.namespace != "" {
		regToUse = rc.namespace
	}
	var refToUse string
	if strings.HasPrefix(rc.imgRef.ref, "sha256:") {
		refToUse = "@" + rc.imgRef.ref
	} else if sha != "" {
		refToUse = "@sha256:" + digestFrom(sha)
	} else {
		refToUse = ":" + rc.imgRef.ref
	}
	return fmt.Sprintf("%s/%s%s", regToUse, rc.imgRef.repository, refToUse)
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
