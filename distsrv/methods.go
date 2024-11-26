package distsrv

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const (
	kib           = 1024
	mib           = 1024 * kib
	manifestLimit = 100 * mib
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

// TODO NEED HEAD REQUEST
// TODO /home/eace/go/pkg/mod/github.com/google/go-containerregistry@v0.20.2/pkg/v1/remote/fetcher.go

func (r *Registry) v2Manifests() (ManifestHolder, error) {
	url := fmt.Sprintf("%s/v2/%s/manifests/%s", r.ImgPull.RegistryUrl(), r.ImgPull.Repository, r.ImgPull.Ref)
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
