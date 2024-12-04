package main

import (
	"fmt"
	"imgpull/distsrv"
	"os"
)

// bin/imgpull docker.io/hello-world:latest ./hello-world.tar
func main() {
	opts, ok := distsrv.ParseArgs()
	if !ok {
		distsrv.Usage()
		os.Exit(1)
	}
	r, err := distsrv.NewRegistry(distsrv.ToRegistryOpts(opts))
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
