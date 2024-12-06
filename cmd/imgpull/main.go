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
	// TODO consider establishing auth handshake upon creation...
	r, err := imgpull.NewRegistry(toRegistryOpts(opts))
	if err != nil {
		fmt.Println(err)
		return
	}
	err = r.PullTar()
	if err != nil {
		fmt.Println(err)
		return
	}
}
