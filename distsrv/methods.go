package distsrv

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
	kib           = 1024
	mib           = 1024 * kib
	manifestLimit = 100 * mib
	maxBlobRead   = 100 * mib
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

func (r *Registry) v2Blobs(layer Layer, destPath string) error {
	url := fmt.Sprintf("%s/v2/%s/blobs/%s", r.ImgPull.RegistryUrl(), r.ImgPull.Repository, layer.Digest)
	resp, err := r.Client.Get(url)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return err
	}
	fName := strings.Replace(filepath.Join(destPath, layer.Digest), "sha256:", "", -1)
	blobFile, err := os.Create(fName)
	if err != nil {
		return err
	}
	defer blobFile.Close()

	bytesRead := 0
	for {
		part, err := io.ReadAll(io.LimitReader(resp.Body, maxBlobRead))
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
// TODO /home/eace/go/pkg/mod/github.com/google/go-containerregistry@v0.20.2/pkg/v1/remote/fetcher.go

func (r *Registry) v2Manifests(digest string) (ManifestHolder, error) {
	ref := r.ImgPull.Ref
	if digest != "" {
		ref = digest
	}
	url := fmt.Sprintf("%s/v2/%s/manifests/%s", r.ImgPull.RegistryUrl(), r.ImgPull.Repository, ref)
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Accept", strings.Join(allManifestTypes, ","))
	req.Header.Set("Bearer", r.Token.Token)
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
	manifestBytes, err := io.ReadAll(io.LimitReader(resp.Body, manifestLimit))
	if err != nil {
		return ManifestHolder{}, err
	}
	mh, err := NewManifestHolder(ct, manifestBytes)
	return mh, err
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
