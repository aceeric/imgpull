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
	image := "quay.io/prometheus/alertmanager:v0.28.0-rc.0"
	// rate limiting:
	//image := "docker.io/hello-world:latest"
	r, err := distsrv.NewRegistry(image)
	if err != nil {
		fmt.Println(err)
		return
	}
	err = r.Pull()
	fmt.Println(err)
}

// func heroku_test() {
// 	url := "https://quay.io/"
// 	username := "" // anonymous
// 	password := "" // anonymous
// 	hub, err := registry.New(url, username, password)
// 	if err != nil {
// 		fmt.Printf("%s\n", err)
// 		return
// 	}
// 	manifest, err := hub.ManifestList("appzygy/ociregistry", "1.5.0")
// 	if err != nil {
// 		fmt.Printf("%s\n", err)
// 		return
// 	}
// 	marshaled, err := json.MarshalIndent(manifest.ManifestList, "", "   ")
// 	if err != nil {
// 		log.Fatalf("marshaling error: %s", err)
// 	}
// 	fmt.Println(string(marshaled))
// }
