package main

import (
	"fmt"
	"imgpull/pkg/imgpull"
)

// bin/imgpull docker.io/hello-world:latest ./hello-world.tar
func main() {
	opts, ok := parseArgs()
	if !ok {
		showUsageAndExit()
	}
	r, err := imgpull.NewRegistry(toRegistryOpts(opts))
	if err != nil {
		fmt.Println(err)
		return
	}
	// TEST
	//mh, err := r.HeadManifest()
	//fmt.Println(mh, err)
	err = r.PullTar()
	if err != nil {
		fmt.Println(err)
		return
	}
}
