package main

import (
	"fmt"
	"imgpull/distsrv"
)

// NOTES:
// quay.io/appzygy/ociregistry:1.5.0 - DOES NOT RETURN A MANIFEST LIST!!
//
// just handle the convo:
// as in mock
// /v2/
// auth
// get image list
// get image
//   manifest
//   blobs
// "application/vnd.docker.distribution.manifest.v1+json,application/vnd.docker.distribution.manifest.v1+prettyjws,application/vnd.docker.distribution.manifest.v2+json,application/vnd.oci.image.manifest.v1+json,application/vnd.docker.distribution.manifest.list.v2+json,application/vnd.oci.image.index.v1+json"

func main() {
	image := "quay.io/keycloak/keycloak-operator:26.0"
	arch := "amd64"
	os := "linux"
	r, err := distsrv.NewRegistry(image, os, arch)
	if err != nil {
		fmt.Println(err)
		return
	}
	err = r.PullTar("keycloak-operator-26.0.tar")
	fmt.Println(err)
}
