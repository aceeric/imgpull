package main

import (
	"fmt"
	"imgpull/distsrv"
	"os"
)

// TODO two NewRegistry options: One with an initialized optinos struct. One with positional params.
// have the arg parser return a typed  optinos struct not a map.

// bin/imgpull docker.io/hello-world:latest ./hello-world.tar
func main() {
	opts, ok := distsrv.ParseArgs()
	if !ok {
		distsrv.Usage()
		os.Exit(1)
	}
	r, err := distsrv.NewRegistry(opts.Val(distsrv.ImageOpt), opts.Val(distsrv.OsOpt),
		opts.Val(distsrv.ArchOpt), opts.Val(distsrv.UsernameOpt), opts.Val(distsrv.PasswordOpt),
		opts.Val(distsrv.SchemeOpt))
	if err != nil {
		fmt.Println(err)
		return
	}
	// TODO why not just put everything into Registry (and rename it)
	err = r.PullTar(opts.Val(distsrv.DestOpt))
	if err != nil {
		fmt.Println(err)
		return
	}
}
