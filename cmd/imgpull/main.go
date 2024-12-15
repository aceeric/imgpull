package main

import (
	"fmt"
	"imgpull/pkg/imgpull"
	"time"
)

func main() {
	opts, err := parseArgs()
	if err != nil {
		fmt.Println(err)
		showUsageAndExit(nil)
	}
	p, err := imgpull.NewPullerWith(pullerOptsFrom(opts))
	if err != nil {
		fmt.Println(err)
		return
	}
	if opts.getVal(manifestOpt) == "" {
		pullTar(p, opts.getVal(destOpt))
	} else {
		pullManifest(p, opts.getVal(manifestOpt))
	}
}

func pullTar(p imgpull.Puller, tarFile string) {
	start := time.Now()
	if err := p.PullTar(tarFile); err != nil {
		fmt.Println(err)
		return
	}
	fmt.Printf("image %q saved to %q in %s\n", p.ImgRef.ImageUrl(), tarFile, time.Since(start))
}

func pullManifest(p imgpull.Puller, manifest string) {
	mt := imgpull.ImageList
	if manifest == "image" {
		mt = imgpull.Image
	}
	mh, err := p.PullManifest(mt)
	if err != nil {
		fmt.Println(err)
		return
	}
	m, err := mh.ToString()
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Printf("%s\n", m)
}
