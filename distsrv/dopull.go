package distsrv

import (
	"fmt"
	"net/http"
	"slices"
	"strings"
)

var unauth []int = []int{http.StatusUnauthorized, http.StatusForbidden}

func (r *Registry) Pull() error {
	status, auth, err := r.v2()
	if err != nil {
		return err
	}
	// TODO add 200ish check to below
	if slices.Contains(unauth, status) {
		err := r.authenticate(auth)
		if err != nil {
			return err
		}
	}
	mh, err := r.v2Manifests("")
	if err != nil {
		return err
	}
	if mh.IsManifestList() {
		digest, err := mh.GetImageDigestFor(r.OSType, r.ArchType)
		if err != nil {
			return err
		}
		im, err := r.v2Manifests(digest)
		if err != nil {
			return err
		}
		mh = im
	}
	//json, err := mh.ToString()
	//if err == nil {
	//	fmt.Println(json)
	//}
	for {
		layer, err := mh.NextLayer()
		if err != nil {
			return err
		}
		if layer == (Layer{}) {
			break
		}
		if err := r.v2Blobs(layer, "/tmp"); err != nil {
			return err
		}
	}

	return nil
}

func (r *Registry) authenticate(auth []string) error {
	fmt.Println(auth)
	for _, hdr := range auth {
		if strings.HasPrefix(strings.ToLower(hdr), "bearer") {
			ba := ParseBearer(hdr)
			return r.v2Auth(ba)
		}
	}
	return fmt.Errorf("unable to parse auth param: %v", auth)
}
