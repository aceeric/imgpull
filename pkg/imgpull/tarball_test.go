package imgpull

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- minimal fixture builder ---
//
// Deliberately self-contained (not reusing internal/tarball's own test
// helpers, which are unexported to that package) - this test needs to
// exercise the real public entry point, OpenImageTarBall, from outside
// internal/tarball entirely, the same way an actual external caller would.

func digestHexFor(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func gzipContent(t *testing.T, content string) []byte {
	t.Helper()
	var tarBuf bytes.Buffer
	tw := tar.NewWriter(&tarBuf)
	hdr := &tar.Header{Name: "hello.txt", Mode: 0644, Size: int64(len(content))}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatalf("writing tar header: %s", err)
	}
	if _, err := tw.Write([]byte(content)); err != nil {
		t.Fatalf("writing tar content: %s", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("closing tar writer: %s", err)
	}

	var gzBuf bytes.Buffer
	gw := gzip.NewWriter(&gzBuf)
	if _, err := gw.Write(tarBuf.Bytes()); err != nil {
		t.Fatalf("writing gzip content: %s", err)
	}
	if err := gw.Close(); err != nil {
		t.Fatalf("closing gzip writer: %s", err)
	}
	return gzBuf.Bytes()
}

// buildOciLayoutFixture builds a minimal, valid single-image OCI-layout
// tarball at dir/fixture.tar, returning its path and the manifest's own
// digest in "sha256:hex" form (for comparison against what
// TarManifestReader yields).
func buildOciLayoutFixture(t *testing.T, dir string, ref string) (tarPath string, wantDigest string) {
	t.Helper()

	gz := gzipContent(t, "hello world")
	layerHex := digestHexFor(gz)

	cfg, _ := json.Marshal(map[string]any{
		"architecture": "amd64",
		"os":           "linux",
		"config":       map[string]any{},
		"rootfs":       map[string]any{"type": "layers", "diff_ids": []string{}},
	})
	cfgHex := digestHexFor(cfg)

	manifest, _ := json.Marshal(map[string]any{
		"schemaVersion": 2,
		"mediaType":     "application/vnd.oci.image.manifest.v1+json",
		"config": map[string]any{
			"mediaType": "application/vnd.oci.image.config.v1+json",
			"digest":    "sha256:" + cfgHex,
			"size":      len(cfg),
		},
		"layers": []map[string]any{
			{
				"mediaType": "application/vnd.oci.image.layer.v1.tar+gzip",
				"digest":    "sha256:" + layerHex,
				"size":      len(gz),
			},
		},
	})
	manifestHex := digestHexFor(manifest)

	index, _ := json.Marshal(map[string]any{
		"schemaVersion": 2,
		"manifests": []map[string]any{
			{
				"mediaType": "application/vnd.oci.image.manifest.v1+json",
				"digest":    "sha256:" + manifestHex,
				"size":      len(manifest),
				"annotations": map[string]string{
					"org.opencontainers.image.ref.name": ref,
				},
			},
		},
	})

	entries := map[string][]byte{
		"oci-layout":                  []byte(`{"imageLayoutVersion":"1.0.0"}`),
		"index.json":                  index,
		"blobs/sha256/" + cfgHex:      cfg,
		"blobs/sha256/" + layerHex:    gz,
		"blobs/sha256/" + manifestHex: manifest,
	}

	tarPath = filepath.Join(dir, "fixture.tar")
	f, err := os.Create(tarPath)
	if err != nil {
		t.Fatalf("creating %q: %s", tarPath, err)
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

	return tarPath, "sha256:" + manifestHex
}

// --- the actual test ---

// TestTarManifestReader_DigestIsBareHex guards against the exact bug found
// while testing: ManifestHolder.Digest must be bare hex (no "sha256:"
// prefix), matching the same convention internal/methods.go enforces for a
// live pull (it strips the prefix before ever constructing a
// ManifestHolder - newManifestHolder itself does no stripping). Entries
// yielded by internal/tarball are legitimately prefixed (real descriptor-
// shaped digest fields), so TarManifestReader has to strip it on the way
// out - this test exercises that conversion through the real public entry
// point, OpenImageTarBall, not internal/tarball directly.
func TestTarManifestReader_DigestIsBareHex(t *testing.T) {
	dir := t.TempDir()
	ref := "test.example.io/hello:v1"
	tarPath, wantDigest := buildOciLayoutFixture(t, dir, ref)

	itb, err := OpenImageTarBall(tarPath, "", "")
	if err != nil {
		t.Fatalf("OpenImageTarBall: %s", err)
	}
	defer itb.Close()

	var got []ManifestHolder
	for mh, err := range itb.TarManifestReader() {
		if err != nil {
			t.Fatalf("TarManifestReader: %s", err)
		}
		got = append(got, mh)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 manifest, got %d", len(got))
	}
	mh := got[0]

	if strings.HasPrefix(mh.Digest, "sha256:") {
		t.Fatalf("ManifestHolder.Digest %q retains the sha256: prefix - it should be bare hex", mh.Digest)
	}
	if len(mh.Digest) != 64 {
		t.Fatalf("ManifestHolder.Digest %q is not 64 characters (bare hex) long: len=%d", mh.Digest, len(mh.Digest))
	}
	wantHex := wantDigest[len("sha256:"):]
	if mh.Digest != wantHex {
		t.Fatalf("expected Digest %q, got %q", wantHex, mh.Digest)
	}
}
