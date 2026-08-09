package main

import (
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/aceeric/imgpull/pkg/imgpull"
)

func main() {
	cmdline, err := parseArgs()
	if err != nil {
		fmt.Println(err)
		showUsageAndExit(nil)
	}
	if cmdline.getVal(listTarOpt) != "" {
		if err := listTar(cmdline.getVal(listTarOpt)); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		os.Exit(0)
	}
	puller, err := imgpull.NewPullerWith(pullerOptsFrom(cmdline))
	if err == nil {
		if cmdline.getVal(manifestOpt) != "" {
			err = showManifest(puller, cmdline.getVal(manifestOpt))
		} else {
			err = pullTar(puller, cmdline.getVal(destOpt))
		}
	}
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func showManifest(puller imgpull.Puller, manifestType string) error {
	mt := imgpull.ManifestPullTypeFrom[manifestType]
	if mh, err := puller.GetManifestByType(mt); err != nil {
		return err
	} else if manifest, err := mh.ToString(); err != nil {
		return err
	} else {
		fmt.Printf("MANIFEST:\n%s\nMANIFEST DIGEST: %s\nIMAGE URL: %s\n", manifest, mh.Digest, mh.ImageUrl)
	}
	return nil
}

func pullTar(puller imgpull.Puller, tarFile string) error {
	start := time.Now()
	if err := puller.PullTar(tarFile); err != nil {
		return err
	} else {
		fmt.Printf("image %q saved to %q in %s\n", puller.GetUrl(), tarFile, time.Since(start))
	}
	return nil
}

func listTar(tarball string) error {
	if itb, err := imgpull.OpenImageTarBall(tarball, runtime.GOOS, runtime.GOARCH); err != nil {
		return err
	} else {
		defer itb.Close()
		for mh, err := range itb.TarManifestReader() {
			if err != nil {
				return err
			} else {
				fmt.Printf("%s (%s)\n", mh.ImageUrl, mh.MediaType())
			}
			if mh.IsImageManifest() {
				for blob, err := range itb.TarBlobReader(mh) {
					if err != nil {
						return err
					}
					fmt.Printf("  blob digest %s\n", blob.Digest)
				}
			}
		}
	}
	return nil
}
