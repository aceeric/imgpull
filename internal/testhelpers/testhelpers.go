package testhelpers

import (
	"archive/tar"
	"bufio"
	"fmt"
	"io"
	"math/rand/v2"
	"os"
	"path/filepath"

	"github.com/opencontainers/go-digest"
)

// makeDigest generates a random digest
func MakeDigest() string {
	foo := fmt.Sprintf("%d", rand.Uint64())
	return digest.FromBytes([]byte(foo)).Hex()
}

// UntarFile is a test helper that un-tars the passed file and appends
// ".extracted" to the name of each extracted file. Files are co-located
// with the tarfile. Limitation: it doesn't process directories.
func UntarFile(tarfile string) error {
	f, err := os.Open(tarfile)
	if err != nil {
		return err
	}
	defer f.Close()
	r := bufio.NewReader(f)
	tarReader := tar.NewReader(r)
	for {
		header, err := tarReader.Next()
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			return err
		}
		if header == nil {
			continue
		}
		switch header.Typeflag {
		case tar.TypeDir:
			// ignore directories
			continue
		case tar.TypeReg:
			p := filepath.Join(filepath.Dir(tarfile), header.Name+".extracted")
			f, err := os.OpenFile(p, os.O_CREATE|os.O_RDWR, 0766)
			if err != nil {
				return err
			}
			defer f.Close()
			if _, err := io.Copy(f, tarReader); err != nil {
				return err
			}
		}
	}
	return nil
}
