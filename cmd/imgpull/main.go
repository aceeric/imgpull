package main

import (
	"fmt"
	"imgpull/pkg/imgpull"
)

func main() {
	opts, err := parseArgs()
	if err != nil {
		fmt.Println(err)
		showUsageAndExit(nil)
	}
	p, err := imgpull.NewPullerWith(toPullerOpts(opts))
	if err != nil {
		fmt.Println(err)
		return
	}
	//	// TESTS
	//	granular(p)
	//	os.Exit(0)
	//
	//	mh, err := p.HeadManifest()
	//	fmt.Println(mh, err)

	err = p.PullTar(opts.getVal(destOpt))
	if err != nil {
		fmt.Println(err)
		return
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
