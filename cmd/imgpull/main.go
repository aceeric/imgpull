package main

import (
	"fmt"
	"imgpull/pkg/imgpull"
)

func main() {
	if opts, err := parseArgs(); err != nil {
		fmt.Println(err)
		showUsageAndExit(nil)
	} else {
		if p, err := imgpull.NewPullerWith(toPullerOpts(opts)); err != nil {
			fmt.Println(err)
			return
		} else {
			err = p.PullTar(opts.getVal(destOpt))
			if err != nil {
				fmt.Println(err)
				return
			}
			// TODO SOME COMPLETION MESSAGE WITH ELAPSED TIME
		}
	}
}

func granular(p imgpull.Puller) {
	mh, err := p.GetManifest()
	if err != nil {
		fmt.Println(err)
		return
	}
	if mh.IsManifestList() {
		digest, err := mh.GetImageDigestFor(p.Opts.OSType, p.Opts.ArchType)
		if err != nil {
			fmt.Println(err)
			return
		}
		// get an image manifest
		im, err := p.GetManifestByDigest(digest)
		if err != nil {
			fmt.Println(err)
			return
		}
		fmt.Println(im)
		url := p.ImgRef.ImageUrlWithDigest(digest)
		opts := imgpull.PullerOpts{
			Url: url,
		}
		// assign to outer 'p'
		p, err = p.NewPullerFrom(opts)
		if err != nil {
			fmt.Println(err)
			return
		}
	}
	// note this stores the manifest with repoTags = quay.io/curl/curl@sha256:4ea50088...
	// but there's nothing that can be done about it.
	err = p.PullTar("/tmp/frobozz.tar")
	if err != nil {
		fmt.Println(err)
		return
	}
}
