package methods

import (
	"encoding/json"
	"fmt"
	"imgpull/internal/blobsync"
	"imgpull/internal/imgref"
	"imgpull/internal/util"
	"imgpull/pkg/imgpull/types"
	"io"
	"net/http"
	"os"
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
	Key   string
	Value string
}

// RegClient has everything needed to talk to an OCI Distribution server for the purposes
// of pulling an image. It is a subset of the 'Puller' struct.
type RegClient struct {
	// ImgRef is the parsed image url, e.g.: 'docker.io/hello-world:latest'
	ImgRef imgref.ImageRef
	// Client is the HTTP Client
	Client *http.Client
	// Namespace supports pull-through
	Namespace string
	// AuthHdr supports the various auth types (basic, bearer)
	AuthHdr AuthHeader
}

type ManifestGetResult struct {
	MediaType      string
	ManifestBytes  []byte
	ManifestDigest string
}

// allManifestTypes lists all of the manifest types that this package
// will operate on.
var allManifestTypes []string = []string{
	types.V2dockerManifestListMt,
	types.V2dockerManifestMt,
	types.V1ociIndexMt,
	types.V1ociManifestMt,
}

// V2 calls the 'v2' endpoint which typically either returns OK or unauthorized. It is the first
// API call made to an OCI Distribution server to initiate an image pull. Returns the http
// status code, an array of auth headers (which could be empty), and an error if one occurred
// or nil.
func (rc RegClient) V2() (int, []string, error) {
	url := fmt.Sprintf("%s/v2/", rc.ImgRef.ServerUrl())
	resp, err := rc.Client.Head(url)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return 0, nil, err
	}
	auth := getWwwAuthenticateHdrs(resp)
	return resp.StatusCode, auth, err
}

// V2Basic calls the 'v2' endpoint with a basic auth header formed from
// the username and password encoded in the passed string. If successful, the
// credentials are returned to the caller for use on subsequent calls.
func (rc RegClient) V2Basic(encoded string) (types.BasicAuth, error) {
	url := fmt.Sprintf("%s/v2/", rc.ImgRef.ServerUrl())
	req, _ := http.NewRequest(http.MethodHead, url, nil)
	req.Header.Set("Authorization", "Basic "+encoded)
	resp, err := rc.Client.Do(req)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return types.BasicAuth{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return types.BasicAuth{}, fmt.Errorf("basic auth returned status code %d", resp.StatusCode)
	}
	return types.BasicAuth{Encoded: encoded}, nil
}

// V2Auth calls the 'v2/auth' endpoint with the passed bearer struct which has
// realm and service. These are used to build the auth URL. The realm might be different
// than the server that we have been requested to pull from.  If successful, the
// bearer token is returned to the caller for use on subsequent calls.
func (rc RegClient) V2Auth(ba types.BearerAuth) (types.BearerToken, error) {
	url := fmt.Sprintf("%s?scope=repository:%s:pull&service=%s", ba.Realm, rc.ImgRef.Repository, ba.Service)
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	resp, err := rc.Client.Do(req)
	if err != nil {
		return types.BearerToken{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return types.BearerToken{}, fmt.Errorf("auth attempt failed. Status: %d", resp.StatusCode)
	}
	if resp != nil {
		defer resp.Body.Close()
	}
	var token types.BearerToken
	decoder := json.NewDecoder(resp.Body)
	err = decoder.Decode(&token)
	if err != nil {
		return types.BearerToken{}, err
	}
	return token, nil
}

// V2Blobs wraps a call to 'v2BlobsInternal' in concurrency handling if needed.
// This supports using the package as a library by synchronizing multiple goroutines
// pulling the same 	"imgpull/pkg/imgpull"
func (rc RegClient) V2Blobs(layer types.Layer, toFile string) error {
	if f, err := os.Stat(toFile); err == nil && f.Size() == int64(layer.Size) {
		// already exists on the file system
		return nil
	}
	if !blobsync.ConcurrentBlobs {
		return rc.V2BlobsInternal(layer, toFile)
	}
	so := blobsync.EnqueueGet(layer.Digest)
	var err error
	go func() {
		if so.Result == blobsync.NotEnqueued {
			defer blobsync.DoneGet(layer.Digest)
			err = rc.V2BlobsInternal(layer, toFile)
		}
	}()
	waitResult := blobsync.Wait(so)
	if err != nil {
		// blob pull err
		return err
	}
	return waitResult
}

// V2BlobsInternal calls the 'v2/<repository>/blobs' endpoint to get a blob by the digest in the
// passed 'layer' arg. The blob is stored in the location specified by 'toFile'.
func (rc RegClient) V2BlobsInternal(layer types.Layer, toFile string) error {
	url := fmt.Sprintf("%s/v2/%s/blobs/%s%s", rc.ImgRef.ServerUrl(), rc.ImgRef.Repository, layer.Digest, rc.nsQueryParm())
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	rc.setAuthHdr(req)
	resp, err := rc.Client.Do(req)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return err
	}
	blobFile, err := os.Create(toFile)
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

// V2Manifests calls the 'v2/<repository>/manifests' endpoint. The resulting manifest is returned in
// a ManifestHolder struct and could be any one of the types defined in the 'allManifestTypes' array.
// If you pass an empty string in 'sha', then the GET will use the image url that was used to initialize
// the Puller. (Probably a tag.) If you provide a digest in 'sha', the digest will override the tag.
//
// Generally speaking: pull by tag returns an image list from the registry if one is available and pull
// by digest (SHA) returns an image manifest. But this might not be true all the time.
func (rc RegClient) V2Manifests(sha string) (ManifestGetResult, error) {
	ref := rc.ImgRef.Ref
	if sha != "" {
		ref = sha
	}
	url := fmt.Sprintf("%s/v2/%s/manifests/%s%s", rc.ImgRef.ServerUrl(), rc.ImgRef.Repository, ref, rc.nsQueryParm())
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("Accept", strings.Join(allManifestTypes, ","))
	rc.setAuthHdr(req)
	resp, err := rc.Client.Do(req)
	if err != nil {
		return ManifestGetResult{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return ManifestGetResult{}, fmt.Errorf("get manifests attempt failed. Status: %d", resp.StatusCode)
	}
	if resp != nil {
		defer resp.Body.Close()
	}
	mediaType := resp.Header.Get("Content-Type")
	manifestBytes, err := io.ReadAll(io.LimitReader(resp.Body, maxManifestBytes))
	if err != nil {
		return ManifestGetResult{}, err
	}
	manifestDigest := resp.Header.Get("Docker-Content-Digest")
	computedDigest := digest.FromBytes(manifestBytes).Hex()
	if manifestDigest == "" {
		manifestDigest = computedDigest
	} else {
		manifestDigest = util.DigestFrom(manifestDigest)
		if computedDigest != manifestDigest {
			return ManifestGetResult{}, fmt.Errorf("digest mismatch for %q", ref)
		}
	}
	return ManifestGetResult{
		MediaType:      mediaType,
		ManifestBytes:  manifestBytes,
		ManifestDigest: manifestDigest,
	}, nil
}

// V2ManifestsHead is like V2Manifests but does a HEAD request. The result is returned in a
// smaller struct with only media type, digest, and size (of manifest).
func (rc RegClient) V2ManifestsHead() (types.ManifestDescriptor, error) {
	url := fmt.Sprintf("%s/v2/%s/manifests/%s%s", rc.ImgRef.ServerUrl(), rc.ImgRef.Repository, rc.ImgRef.Ref, rc.nsQueryParm())
	req, _ := http.NewRequest(http.MethodHead, url, nil)
	req.Header.Set("Accept", strings.Join(allManifestTypes, ","))
	rc.setAuthHdr(req)
	resp, err := rc.Client.Do(req)
	if err != nil {
		return types.ManifestDescriptor{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return types.ManifestDescriptor{}, fmt.Errorf("head manifests for %q failed with status %q", url, resp.StatusCode)
	}
	if resp != nil {
		defer resp.Body.Close()
	}
	mediaType := resp.Header.Get("Content-Type")
	if mediaType == "" {
		return types.ManifestDescriptor{}, fmt.Errorf("head manifests for %q did not return content type", url)
	}
	digest := resp.Header.Get("Docker-Content-Digest")
	if digest == "" {
		return types.ManifestDescriptor{}, fmt.Errorf("head manifests for %q did not return digest", url)
	}
	return types.ManifestDescriptor{
		MediaType: mediaType,
		Digest:    digest,
		Size:      int(resp.ContentLength),
	}, nil
}

// setAuthHdr sets an auth header (e.g. "Bearer", "Basic") on the passed request
// if the receiver is configured with such a header.
func (rc RegClient) setAuthHdr(req *http.Request) {
	if rc.AuthHdr != (AuthHeader{}) {
		req.Header.Set(rc.AuthHdr.Key, rc.AuthHdr.Value)
	}
}

// nsQueryParm checks if the receiver is configured with a namespace for pull-through,
// and if it is, returns the namespace as a query param in the form: '?ns=X' where 'X'
// is the receiver's namespace. If no namespace is configured, then the function
// returns the empty string.
func (rc RegClient) nsQueryParm() string {
	if rc.Namespace != "" {
		return "?ns=" + rc.Namespace
	} else {
		return ""
	}
}

// MakeUrl makes an image ref like docker.io/hello-world:latest. If the receiver
// has a namespace, then the namespace is used for the registry instead of the
// registry in the receiver. If the passed sha is not the empty string, then it is
// used as a digest ref, otherwise the ref in the receiver (which could be a tag
// or a digest is used.
func (rc RegClient) MakeUrl(sha string) string {
	regToUse := rc.ImgRef.Registry
	if rc.Namespace != "" {
		regToUse = rc.Namespace
	}
	var refToUse string
	if strings.HasPrefix(rc.ImgRef.Ref, "sha256:") {
		refToUse = "@" + rc.ImgRef.Ref
	} else if sha != "" {
		refToUse = "@sha256:" + util.DigestFrom(sha)
	} else {
		refToUse = ":" + rc.ImgRef.Ref
	}
	return fmt.Sprintf("%s/%s%s", regToUse, rc.ImgRef.Repository, refToUse)
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