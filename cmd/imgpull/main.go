package main

import (
	"fmt"
	"time"

	"github.com/aceeric/imgpull/pkg/imgpull"
)

func main() {
	if cmdline, err := parseArgs(); err != nil {
		fmt.Println(err)
		showUsageAndExit(nil)
	} else if p, err := imgpull.NewPullerWith(pullerOptsFrom(cmdline)); err != nil {
		fmt.Println(err)
	} else if cmdline.getVal(manifestOpt) != "" {
		showManifest(p, cmdline.getVal(manifestOpt))
	} else {
		pullTar(p, cmdline.getVal(destOpt))
	}
}

func showManifest(p imgpull.Puller, manifestType string) {
	mt := imgpull.ManifestPullTypeFrom[manifestType]
	if mh, err := p.GetManifestByType(mt); err != nil {
		fmt.Println(err)
	} else if manifest, err := mh.ToString(); err != nil {
		fmt.Println(err)
	} else {
		fmt.Printf("MANIFEST:\n%s\nMANIFEST DIGEST: %s\nIMAGE URL: %s\n", manifest, mh.Digest, mh.ImageUrl)
	}
}

func pullTar(p imgpull.Puller, tarFile string) {
	start := time.Now()
	if err := p.PullTar(tarFile); err != nil {
		fmt.Println(err)
	} else {
		fmt.Printf("image %q saved to %q in %s\n", p.GetUrl(), tarFile, time.Since(start))
	}
}
