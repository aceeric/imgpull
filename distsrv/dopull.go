package distsrv

import (
	"fmt"
	"net/http"
	"slices"
	"strings"
)

var unauth []int = []int{http.StatusUnauthorized, http.StatusForbidden}

// TODO NEED OS AND ARCH
// X v2
// X if 401
// X   v2Auth or fail
// X get manifest list or fail
//   if its a manifest list
//     select image manifest using OS and arch or fail
//     TODO /home/eace/projects/docker-distribution/vendor/go.opentelemetry.io/otel/semconv/v1.10.0/resource.go
//   get manifest
//   for blob in blob
//     get blof

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
	mh, err := r.v2Manifests()
	if err != nil {
		return err
	}
	fmt.Println(mh)

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
