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

// allManifestTypes lists all of the manifest types that this package
// will operate on.
var allManifestTypes []string = []string{
	V2dockerManifestListMt,
	V2dockerManifestMt,
	V1ociIndexMt,
	V1ociManifestMt,
}

// calls the 'v2' endpoint which typically either returns OK or
// unauthorized.
func (p *Puller) v2() (int, []string, error) {
	url := fmt.Sprintf("%s/v2/", p.ImgRef.RegistryUrl())
	resp, err := p.Client.Head(url)
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
// credentials are stored in the receiver for use on subsequent calls.
func (p *Puller) v2Basic(encoded string) error {
	url := fmt.Sprintf("%s/v2/", p.ImgRef.RegistryUrl())
	req, _ := http.NewRequest("HEAD", url, nil)
	req.Header.Set("Authorization", "Basic "+encoded)
	resp, err := p.Client.Do(req)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("basic auth returned status code %d", resp.StatusCode)
	}
	p.Basic = BasicAuth{Encoded: encoded}
	return nil
}

// v2Auth calls the 'v2/auth' endpoint with the passed bearer struct which has
// realm and service. These are used to build the auth URL. The realm might be different
// than the server that we have been requested to pull from.
func (p *Puller) v2Auth(ba BearerAuth) error {
	url := fmt.Sprintf("%s?scope=repository:%s:pull&service=%s", ba.Realm, p.ImgRef.Repository, ba.Service)
	req, _ := http.NewRequest("GET", url, nil)
	resp, err := p.Client.Do(req)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("auth attempt failed. Status: %d", resp.StatusCode)
	}
	if resp != nil {
		defer resp.Body.Close()
	}
	var token BearerToken
	decoder := json.NewDecoder(resp.Body)
	err = decoder.Decode(&token)
	if err != nil {
		return err
	}
	p.Token = token
	return nil
}

// v2Blobs calls the 'v2/<repository>/blobs' endpoint to get a blob by the digest in the passed
// 'layer' arg. The blob is stored in the location specified by 'destPath'. The 'isConfig'
// var indicates that the blob is a config blob.
func (p *Puller) v2Blobs(layer Layer, destPath string, isConfig bool) error {
	url := fmt.Sprintf("%s/v2/%s/blobs/%s%s", p.ImgRef.RegistryUrl(), p.ImgRef.Repository, layer.Digest, p.nsQueryParm())
	req, _ := http.NewRequest("GET", url, nil)
	if p.hasAuth() {
		req.Header.Set(p.authHdr())
	}
	resp, err := p.Client.Do(req)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return err
	}
	fName := filepath.Join(destPath, layer.Digest)
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
// If you pass an empty string in digest, the the GET will use the image url that was used to initialize
// the Puller. (Probably used a tag.) If you provide a digest, the digest will override the tag.
func (p *Puller) v2Manifests(sha string) (ManifestHolder, error) {
	ref := p.ImgRef.Ref
	if sha != "" {
		ref = sha
	}
	url := fmt.Sprintf("%s/v2/%s/manifests/%s%s", p.ImgRef.RegistryUrl(), p.ImgRef.Repository, ref, p.nsQueryParm())
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Accept", strings.Join(allManifestTypes, ","))
	if p.hasAuth() {
		req.Header.Set(p.authHdr())
	}
	resp, err := p.Client.Do(req)
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
			return ManifestHolder{}, fmt.Errorf("digest mismatch for %s", ref)
		}
	}
	mh, err := NewManifestHolder(mediaType, manifestBytes)
	return mh, err
}

// v2ManifestsHead is like v2Manifests but does a HEAD request. The result is returned in a
// smaller struct with only media type, digest, and size (of manifest).
func (p *Puller) v2ManifestsHead() (ManifestDescriptor, error) {
	url := fmt.Sprintf("%s/v2/%s/manifests/%s%s", p.ImgRef.RegistryUrl(), p.ImgRef.Repository, p.ImgRef.Ref, p.nsQueryParm())
	req, _ := http.NewRequest("HEAD", url, nil)
	req.Header.Set("Accept", strings.Join(allManifestTypes, ","))
	if p.hasAuth() {
		req.Header.Set(p.authHdr())
	}
	resp, err := p.Client.Do(req)
	if err != nil {
		return ManifestDescriptor{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return ManifestDescriptor{}, fmt.Errorf("head manifests for %s failed. Status: %d", url, resp.StatusCode)
	}
	if resp != nil {
		defer resp.Body.Close()
	}
	mediaType := resp.Header.Get("Content-Type")
	if mediaType == "" {
		return ManifestDescriptor{}, fmt.Errorf("head manifests for %s did not return content type", url)
	}
	digest := resp.Header.Get("Docker-Content-Digest")
	if digest == "" {
		return ManifestDescriptor{}, fmt.Errorf("head manifests for %s did not return digest", url)
	}
	return ManifestDescriptor{
		MediaType: mediaType,
		Digest:    digest,
		Size:      int(resp.ContentLength),
	}, nil
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
