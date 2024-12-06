package imgpull

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const (
	mebibytes        = 1024 * 1024
	maxManifestBytes = 100 * mebibytes
	maxBlobBytes     = 100 * mebibytes
)

func (r *Registry) v2() (int, []string, error) {
	url := fmt.Sprintf("%s/v2/", r.ImgPull.RegistryUrl())
	resp, err := r.Client.Head(url)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return 0, nil, err
	}
	auth := getWwwAuthenticateHdrs(resp)
	return resp.StatusCode, auth, err
}

func (r *Registry) v2Basic(encoded string) error {
	url := fmt.Sprintf("%s/v2/", r.ImgPull.RegistryUrl())
	req, _ := http.NewRequest("HEAD", url, nil)
	req.Header.Set("Authorization", "Basic "+encoded)
	resp, err := r.Client.Do(req)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("basic auth returned status code %d", resp.StatusCode)
	}
	r.Basic = BasicAuth{Encoded: encoded}
	return nil
}

// Bearer realm="https://quay.io/v2/auth",service="quay.io"
func (r *Registry) v2Auth(ba BearerAuth) error {
	url := fmt.Sprintf("%s?scope=repository:%s:pull&service=%s", ba.Realm, r.ImgPull.Repository, ba.Service)
	req, _ := http.NewRequest("GET", url, nil)
	resp, err := r.Client.Do(req)
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
	r.Token = token
	return nil
}

// TODO for manifests and blobs check size and digest against expected as in
// /home/eace/projects/go-containerregistry/pkg/v1/remote/fetcher.go

func (r *Registry) v2Blobs(layer Layer, destPath string, isConfig bool) error {
	url := fmt.Sprintf("%s/v2/%s/blobs/%s%s", r.ImgPull.RegistryUrl(), r.ImgPull.Repository, layer.Digest, r.nsQueryParm())
	req, _ := http.NewRequest("GET", url, nil)
	if r.hasAuth() {
		req.Header.Set(r.authHdr())
	}
	resp, err := r.Client.Do(req)
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

// TODO NEED HEAD REQUEST EVENTUALLY FOR COMPAT W/ CONTAINER REGISTRY TO REPLACE CRANE
// TODO /home/eace/projects/go-containerregistry/pkg/v1/remote/fetcher.go
// returns descriptor with digest, size, and media type

func (r *Registry) v2Manifests(digest string) (ManifestHolder, error) {
	ref := r.ImgPull.Ref
	if digest != "" {
		ref = digest
	}
	url := fmt.Sprintf("%s/v2/%s/manifests/%s%s", r.ImgPull.RegistryUrl(), r.ImgPull.Repository, ref, r.nsQueryParm())
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Accept", strings.Join(allManifestTypes, ","))
	if r.hasAuth() {
		req.Header.Set(r.authHdr())
	}
	resp, err := r.Client.Do(req)
	if err != nil {
		return ManifestHolder{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return ManifestHolder{}, fmt.Errorf("get manifests attempt failed. Status: %d", resp.StatusCode)
	}
	if resp != nil {
		defer resp.Body.Close()
	}
	ct := resp.Header.Get("Content-Type")
	manifestBytes, err := io.ReadAll(io.LimitReader(resp.Body, maxManifestBytes))
	if err != nil {
		return ManifestHolder{}, err
	}
	mh, err := NewManifestHolder(ct, manifestBytes)
	return mh, err
}

func (r *Registry) v2HeadManifests() (ManifestHead, error) {
	url := fmt.Sprintf("%s/v2/%s/manifests/%s%s", r.ImgPull.RegistryUrl(), r.ImgPull.Repository, r.ImgPull.Ref, r.nsQueryParm())
	req, _ := http.NewRequest("HEAD", url, nil)
	req.Header.Set("Accept", strings.Join(allManifestTypes, ","))
	if r.hasAuth() {
		req.Header.Set(r.authHdr())
	}
	resp, err := r.Client.Do(req)
	if err != nil {
		return ManifestHead{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return ManifestHead{}, fmt.Errorf("head manifests for %s failed. Status: %d", url, resp.StatusCode)
	}
	if resp != nil {
		defer resp.Body.Close()
	}
	ct := resp.Header.Get("Content-Type")
	if ct == "" {
		return ManifestHead{}, fmt.Errorf("head manifests for %s did not return content type", url)
	}
	dcd := resp.Header.Get("Docker-Content-Digest")
	if dcd == "" {
		return ManifestHead{}, fmt.Errorf("head manifests for %s did not return digest", url)
	}
	return ManifestHead{
		MediaType: ct,
		Digest:    dcd,
	}, nil
}

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
