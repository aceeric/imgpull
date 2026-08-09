package tarball

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"
)

// digestHex returns the sha256 digest of b as bare hex (no "sha256:"
// prefix). Deliberately duplicated from this package's own digestOf rather
// than calling it, so a test's expectations aren't derived from the same
// code being tested.
func digestHex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// gzipLayer builds a tiny single-file tar (a stand-in "layer") and returns
// both its raw (uncompressed) and gzip-compressed bytes. Content doesn't
// matter for these tests beyond being real: real gzip stream, real digest
// of real bytes - not faked, since this package's reader actually parses
// and hashes what's there.
func gzipLayer(t *testing.T, fileContent string) (raw []byte, gz []byte) {
	t.Helper()
	var rawBuf bytes.Buffer
	tw := tar.NewWriter(&rawBuf)
	hdr := &tar.Header{Name: "hello.txt", Mode: 0644, Size: int64(len(fileContent))}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatalf("writing tar header: %s", err)
	}
	if _, err := tw.Write([]byte(fileContent)); err != nil {
		t.Fatalf("writing tar content: %s", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("closing tar writer: %s", err)
	}
	raw = rawBuf.Bytes()

	var gzBuf bytes.Buffer
	gw := gzip.NewWriter(&gzBuf)
	if _, err := gw.Write(raw); err != nil {
		t.Fatalf("writing gzip content: %s", err)
	}
	if err := gw.Close(); err != nil {
		t.Fatalf("closing gzip writer: %s", err)
	}
	return raw, gzBuf.Bytes()
}

// imageConfig builds a minimal, valid OCI/docker image config JSON with the
// given os/architecture - the only fields this package's platform
// filtering (platformMatches, via buildDockerSaveManifest/childDigestsOf)
// actually reads.
func imageConfig(os string, arch string) []byte {
	cfg := map[string]any{
		"architecture": arch,
		"os":           os,
		"config":       map[string]any{},
		"rootfs": map[string]any{
			"type":     "layers",
			"diff_ids": []string{},
		},
	}
	b, _ := json.Marshal(cfg)
	return b
}

// writeTarFile writes entries (tar path -> content) as a plain
// (uncompressed) tar file at tarPath.
func writeTarFile(t *testing.T, tarPath string, entries map[string][]byte) {
	t.Helper()
	f, err := os.Create(tarPath)
	if err != nil {
		t.Fatalf("creating tar file: %s", err)
	}
	defer f.Close()
	tw := tar.NewWriter(f)
	defer tw.Close()
	for name, content := range entries {
		hdr := &tar.Header{Name: name, Mode: 0644, Size: int64(len(content))}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("writing header for %q: %s", name, err)
		}
		if _, err := tw.Write(content); err != nil {
			t.Fatalf("writing content for %q: %s", name, err)
		}
	}
}

// gzipFile reads plainPath and writes a gzip-compressed copy at gzPath -
// for exercising openPlainTar's decompression against a real gzip stream,
// not a renamed plain file.
func gzipFile(t *testing.T, plainPath string, gzPath string) {
	t.Helper()
	raw, err := os.ReadFile(plainPath)
	if err != nil {
		t.Fatalf("reading %q: %s", plainPath, err)
	}
	out, err := os.Create(gzPath)
	if err != nil {
		t.Fatalf("creating %q: %s", gzPath, err)
	}
	defer out.Close()
	gw := gzip.NewWriter(out)
	if _, err := gw.Write(raw); err != nil {
		t.Fatalf("gzipping %q: %s", gzPath, err)
	}
	if err := gw.Close(); err != nil {
		t.Fatalf("closing gzip writer for %q: %s", gzPath, err)
	}
}
