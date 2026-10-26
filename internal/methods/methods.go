package methods

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aceeric/imgpull/internal/blobsync"
	"github.com/aceeric/imgpull/internal/imgref"
	"github.com/aceeric/imgpull/internal/util"
	"github.com/aceeric/imgpull/pkg/imgpull/types"

	"github.com/opencontainers/go-digest"
)

const (
	maxManifestBytes = 25 * 1024
	maxBlobBytes     = 10 * 1024 * 1024
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
	// AuthHdr supports the various auth types (basic, bearer)
	AuthHdr AuthHeader
}

// ManifestGetResult is returned by the 'V2Manifests' function in this
// package. The manifest is contained within the 'ManifestBytes' struct
// member.
type ManifestGetResult struct {
	MediaType      types.MediaType
	ManifestBytes  []byte
	ManifestDigest string
}

// idleReader resets an idle timer on every read. When the timer fires, it cancels
// the request context, and the blocked Read returns an error.
type idleReader struct {
	r io.Reader
	t *time.Timer
	d time.Duration
}

// allManifestTypes lists all of the manifest types that this package
// will operate on.
var allManifestTypes []types.MediaType = []types.MediaType{
	types.V2dockerManifestListMt,
	types.V2dockerManifestMt,
	types.V1ociIndexMt,
	types.V1ociManifestMt,
}

// blobIdleTimeout aborts a blob download if no bytes arrive for this long. It's a var
// so tests can shorten it.
var blobIdleTimeout = 60 * time.Second

func (i *idleReader) Read(p []byte) (int, error) {
	n, err := i.r.Read(p)
	i.t.Reset(i.d)
	return n, err
}

// V2ManifestsAuth does a HEAD request for the manifest in the receiver, only looking for
// OK or unauthorized. Returns the http status code, an array of auth headers (which could
// be empty), and an error if one occurred or nil.
func (rc RegClient) V2ManifestsAuth() (int, []string, error) {
	url := rc.makeManifestUrl("")
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
func (rc RegClient) V2Auth(ba types.BearerAuth, encoded string) (types.BearerToken, error) {
	url := fmt.Sprintf("%s?scope=repository:%s:pull&service=%s", ba.Realm, rc.ImgRef.Repository(), ba.Service)
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	if encoded != "" {
		req.Header.Set("Authorization", "Basic "+encoded)
	}
	resp, err := rc.Client.Do(req)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return types.BearerToken{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return types.BearerToken{}, fmt.Errorf("auth attempt failed. Status: %d", resp.StatusCode)
	}
	var token types.BearerToken
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return types.BearerToken{}, err
	}
	bodyString := string(bodyBytes)
	reader := strings.NewReader(bodyString)
	decoder := json.NewDecoder(reader)
	err = decoder.Decode(&token)
	if err != nil {
		return types.BearerToken{}, err
	}
	return token, nil
}

// V2Blobs wraps a call to 'v2BlobsInternal' in concurrency handling if needed.
// This supports using the package as a library by synchronizing multiple goroutines
// pulling the same blob.
func (rc RegClient) V2Blobs(layer types.Layer, toFile string) error {
	if blobComplete(toFile, layer.Size) {
		return nil
	}
	if !blobsync.ConcurrentBlobs {
		return rc.V2BlobsInternal(layer, toFile)
	}
	p, mustPull := blobsync.Enqueue(layer.Digest)
	if !mustPull {
		return blobsync.Wait(p)
	}
	err := errors.New("blob pull aborted") // reported to waiters if we panic
	defer func() { blobsync.Done(layer.Digest, p, err) }()
	if blobComplete(toFile, layer.Size) { // a puller may have finished since our first check
		err = nil
		return nil
	}
	err = rc.V2BlobsInternal(layer, toFile)
	return err
}

// V2BlobsInternal calls the 'v2/<repository>/blobs' endpoint to get a blob by the digest in the
// passed 'layer' arg. The blob is written to a temp file in the same directory as 'toFile', verified
// against the layer size and digest, then renamed to 'toFile'. So 'toFile' either doesn't exist or
// holds the complete, verified blob. On any error the temp file is removed.
func (rc RegClient) V2BlobsInternal(layer types.Layer, toFile string) (err error) {
	dgst := digest.Digest(layer.Digest)
	if err := dgst.Validate(); err != nil {
		return fmt.Errorf("invalid blob digest %q: %w", layer.Digest, err)
	}
	url := ""
	if rc.ImgRef.NsInPath() {
		url = fmt.Sprintf("%s/v2/%s/%s/blobs/%s", rc.ImgRef.ServerUrl(), rc.ImgRef.Namespace(), rc.ImgRef.Repository(), layer.Digest)
	} else {
		url = fmt.Sprintf("%s/v2/%s/blobs/%s%s", rc.ImgRef.ServerUrl(), rc.ImgRef.Repository(), layer.Digest, rc.nsQueryParm())
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// started now, so it also covers the wait for the response headers
	timer := time.AfterFunc(blobIdleTimeout, cancel)
	defer timer.Stop()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	rc.setAuthHdr(req)
	resp, err := rc.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%d from server for blob digest %q", resp.StatusCode, layer.Digest)
	}

	tmp, err := os.CreateTemp(filepath.Dir(toFile), filepath.Base(toFile)+".*.tmp")
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			tmp.Close() // harmless if already closed
			os.Remove(tmp.Name())
		}
	}()

	verifier := dgst.Verifier()
	body := &idleReader{r: resp.Body, t: timer, d: blobIdleTimeout}
	n, err := io.Copy(io.MultiWriter(tmp, verifier), body)
	if err != nil {
		return fmt.Errorf("error reading blob %q: %w", layer.Digest, err)
	}
	if n != int64(layer.Size) {
		return fmt.Errorf("blob %q: expected %d bytes, got %d", layer.Digest, layer.Size, n)
	}
	if !verifier.Verified() {
		return fmt.Errorf("blob %q: digest mismatch", layer.Digest)
	}
	if err = tmp.Chmod(0644); err != nil { // CreateTemp uses 0600
		return err
	}
	if err = tmp.Sync(); err != nil {
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), toFile)
}

// V2Manifests calls the 'v2/<repository>/manifests' endpoint. The resulting manifest is returned in
// a ManifestHolder struct and could be any one of the types defined in the 'allManifestTypes' array.
// If you pass an empty string in 'sha', then the GET will use the image url that was used to initialize
// the Puller. (Probably a tag.) If you provide a digest in 'sha', the digest will override the tag.
//
// Generally speaking: pull by tag returns an image list from the registry if one is available and pull
// by digest (SHA) returns an image manifest. But this might not be true all the time.
func (rc RegClient) V2Manifests(sha string) (ManifestGetResult, error) {
	url := rc.makeManifestUrl(sha)
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("Accept", allManifestTypesStr())
	rc.setAuthHdr(req)
	resp, err := rc.Client.Do(req)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return ManifestGetResult{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return ManifestGetResult{}, fmt.Errorf("get manifests attempt failed. Status: %d", resp.StatusCode)
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
			return ManifestGetResult{}, fmt.Errorf("digest mismatch for %q", url)
		}
	}
	return ManifestGetResult{
		MediaType:      types.MediaType(mediaType),
		ManifestBytes:  manifestBytes,
		ManifestDigest: manifestDigest,
	}, nil
}

// V2ManifestsHead is like V2Manifests but does a HEAD request. The result is returned in a
// smaller struct with only media type, digest, and size (of manifest). We don't allow overriding
// the ref becuase the use case for this method is to HEAD the manifest list.
func (rc RegClient) V2ManifestsHead() (types.ManifestDescriptor, error) {
	url := rc.makeManifestUrl("")
	req, _ := http.NewRequest(http.MethodHead, url, nil)
	req.Header.Set("Accept", allManifestTypesStr())
	rc.setAuthHdr(req)
	resp, err := rc.Client.Do(req)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return types.ManifestDescriptor{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return types.ManifestDescriptor{}, fmt.Errorf("head manifests for %q failed with status %d", url, resp.StatusCode)
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
		MediaType: types.MediaType(mediaType),
		Digest:    digest,
		Size:      int(resp.ContentLength),
	}, nil
}

// makeManifestUrl is a help that forms  the URL string for the v2/.../manifests API call. It
// returns a URL taking into account whether the image ref in the receiver is namespaced, and
// whether the namespace is path-based or parameter based.
func (rc RegClient) makeManifestUrl(sha string) string {
	ref := rc.ImgRef.Ref()
	if sha != "" {
		ref = sha
	}
	if rc.ImgRef.NsInPath() {
		return fmt.Sprintf("%s/v2/%s/%s/manifests/%s", rc.ImgRef.ServerUrl(), rc.ImgRef.Namespace(), rc.ImgRef.Repository(), ref)
	} else {
		return fmt.Sprintf("%s/v2/%s/manifests/%s%s", rc.ImgRef.ServerUrl(), rc.ImgRef.Repository(), ref, rc.nsQueryParm())
	}
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
	if rc.ImgRef.Namespace() != "" {
		return "?ns=" + rc.ImgRef.Namespace()
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

// blobComplete returns true if the bloc exists and is the expected size.
func blobComplete(path string, size int) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.Size() == int64(size)
}

// allManifestTypesStr concats all the manifest types supported to be pulled
// into a comma-separated string.
func allManifestTypesStr() string {
	toReturn := string(allManifestTypes[0])
	for i := 1; i < len(allManifestTypes); i++ {
		toReturn = fmt.Sprintf("%s,%s", toReturn, allManifestTypes[i])
	}
	return toReturn
}
