package main

import (
	"fmt"
	"imgpull/pkg/imgpull"
)

func main() {
	opts, ok := parseArgs()
	if !ok {
		showUsageAndExit(nil)
	}
	r, err := imgpull.NewPuller(toPullerOpts(opts))
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
