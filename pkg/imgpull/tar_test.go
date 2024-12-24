package imgpull

import (
	"archive/tar"
	"bufio"
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"math/rand/v2"
	"os"
	"path/filepath"
	"testing"
)

// TestWriteFiles tests writing a physical file and a string "file"
// to a tarfile.
func TestWriteFiles(t *testing.T) {
	d, err := os.MkdirTemp("", "")
	if err != nil {
		t.Fail()
	}
	defer os.RemoveAll(d)
	foobar, err := os.Create(filepath.Join(d, "foobar"))
	if err != nil {
		t.Fail()
	}
	_, err = foobar.WriteString("frobozz")
	if err != nil {
		t.Fail()
	}
	foobar.Close()
	tarfile, err := os.Create(filepath.Join(d, "foobar.tar"))
	if err != nil {
		t.Fail()
	}
	defer tarfile.Close()
	tw := tar.NewWriter(tarfile)
	defer tw.Close()
	if err := addFile(tw, foobar.Name(), foobar.Name()); err != nil {
		t.Fail()
	}
	if err := addString(tw, "flathead", "flathead"); err != nil {
		t.Fail()
	}
	tw.Close()
	tarfile.Close()

	untarFile(filepath.Join(d, "foobar.tar"))

	b1, err1 := os.ReadFile(filepath.Join(d, "foobar"))
	b2, err2 := os.ReadFile(filepath.Join(d, "foobar_extracted"))
	if err1 != nil || err2 != nil {
		t.Fail()
	}
	if !bytes.Equal(b1, b2) {
		t.Fail()
	}
	b1, err1 = os.ReadFile(filepath.Join(d, "flathead_extracted"))
	if err1 != nil {
		t.Fail()
	}
	if string(b1) != "flathead" {
		t.Fail()
	}
}

// TestTarNew creates an image tarball and then extracts it to make sure the files
// were put in correctly.
func TestTarNew(t *testing.T) {
	d, err := os.MkdirTemp("", "")
	if err != nil {
		t.Fail()
	}
	defer os.RemoveAll(d)
	tarfile := filepath.Join(d, "test.tar")
	configDigest := makeDigest()
	err = os.WriteFile(filepath.Join(d, configDigest), []byte(configDigest), 0644)
	if err != nil {
		t.Fail()
	}
	url := "flathead.io/frobozz/fizzbin:v1.2.3"
	layerDigest := makeDigest()
	err = os.WriteFile(filepath.Join(d, layerDigest), []byte(layerDigest), 0644)
	if err != nil {
		t.Fail()
	}
	layers := []Layer{
		{
			MediaType: V2dockerLayerGzipMt,
			Digest:    layerDigest,
			Size:      62,
		},
	}
	dtm, err := imageTarball{
		sourceDir:    d,
		configDigest: configDigest,
		imageUrl:     url,
		layers:       layers,
	}.toTar(tarfile)
	if err != nil {
		t.Fail()
	}
	err = untarFile(tarfile)
	if err != nil {
		t.Fail()
	}
	b, err := os.ReadFile(filepath.Join(d, fmt.Sprintf("sha256:%s_extracted", configDigest)))
	if err != nil {
		t.Fail()
	}
	if string(b) != configDigest {
		t.Fail()
	}
	b, err = os.ReadFile(filepath.Join(d, layerDigest+".tar.gz_extracted"))
	if err != nil {
		t.Fail()
	}
	if string(b) != layerDigest {
		t.Fail()
	}
	b, err = os.ReadFile(filepath.Join(d, "manifest.json_extracted"))
	if err != nil {
		t.Fail()
	}
	manifest, err := dtm.toString()
	if err != nil {
		t.Fail()
	}
	if !bytes.Equal(b, manifest) {
		t.Fail()
	}
}

// untarFile is a test helper that un-tars the passed file and appends
// "_extracted" to the name of each extracted file. Files are co-located
// with the tarfile. Limitation: it doesn't process directories.
func untarFile(tarfile string) error {
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
			p := filepath.Join(filepath.Dir(tarfile), header.Name+"_extracted")
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

func makeDigest() string {
	foo := fmt.Sprintf("%d", rand.Uint64())
	hasher := sha256.New()
	hasher.Write([]byte(foo))
	return fmt.Sprintf("%x", hasher.Sum(nil))
}
