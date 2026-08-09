package main

import (
	"os"

	"github.com/aceeric/imgpull/pkg/imgpull"
)

// The program pulls an image tarball to the current working directory. The
// image will match the OS and architecture of the host.
func main() {
	tarball := os.Args[1]
	itb := imgpull.OpenImageTarBall
}
