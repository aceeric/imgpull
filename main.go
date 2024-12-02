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
	r, err := distsrv.NewRegistry(opts.Val(distsrv.ImageOpt), opts.Val(distsrv.OsOpt), opts.Val(distsrv.ArchOpt))
	if err != nil {
		fmt.Println(err)
		return
	}
	err = r.PullTar(opts.Val(distsrv.DestOpt))
	if err != nil {
		fmt.Println(err)
		return
	}
}
