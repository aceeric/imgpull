package tarball

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestOpen_UnrecognizedFormat(t *testing.T) {
	dir := t.TempDir()
	tarPath := filepath.Join(dir, "not-an-image.tar")
	writeTarFile(t, tarPath, map[string][]byte{"README.txt": []byte("not an image tarball")})

	if _, err := Open(tarPath, "", ""); err == nil {
		t.Fatalf("expected an error opening a tarball with neither oci-layout nor manifest.json")
	}
}

// TestOpen_PrefersOciLayoutWhenBothPresent reproduces the real-world shape
// confirmed against an actual `docker save` (containerd image store
// backend) tarball: both oci-layout/index.json AND manifest.json present
// in the same tarball. OCI-layout should be used - it's the richer,
// content-addressed format, and doesn't require the digest-by-hashing
// synthesis the docker-save path needs.
func TestOpen_PrefersOciLayoutWhenBothPresent(t *testing.T) {
	dir := t.TempDir()
	entries := map[string][]byte{}
	_, manifestDigestHex := buildOciManifestBlob(t, entries, ociManifestSpec{os: "linux", arch: "amd64", content: "hello"})
	ref := "test.example.io/hello:v1"
	index := map[string]any{
		"schemaVersion": 2,
		"manifests": []map[string]any{
			{
				"mediaType": "application/vnd.oci.image.manifest.v1+json",
				"digest":    "sha256:" + manifestDigestHex,
				"size":      len(entries["blobs/sha256/"+manifestDigestHex]),
				"annotations": map[string]string{
					"org.opencontainers.image.ref.name": ref,
				},
			},
		},
	}
	indexBytes, err := json.Marshal(index)
	if err != nil {
		t.Fatalf("marshalling index.json: %s", err)
	}
	entries["index.json"] = indexBytes
	entries["oci-layout"] = []byte(`{"imageLayoutVersion":"1.0.0"}`)

	// A decoy manifest.json with a DIFFERENT ref, referencing a config file
	// that doesn't even exist - if this ever got used instead of index.json,
	// Manifests() would yield an error, not the decoy ref, making a mix-up
	// impossible to miss.
	decoyManifestJSON, _ := json.Marshal([]map[string]any{
		{"Config": "does-not-exist.json", "RepoTags": []string{"wrong.example.io/decoy:v1"}, "Layers": []string{}},
	})
	entries["manifest.json"] = decoyManifestJSON

	tarPath := filepath.Join(dir, "hybrid.tar")
	writeTarFile(t, tarPath, entries)

	r, err := Open(tarPath, "", "")
	if err != nil {
		t.Fatalf("Open: %s", err)
	}
	defer r.Close()

	if r.format != formatOciLayout {
		t.Fatalf("expected OCI-layout to be preferred when both markers are present")
	}

	var got string
	for me, err := range r.Manifests() {
		if err != nil {
			t.Fatalf("Manifests: %s", err)
		}
		got = me.Ref
	}
	if got != ref {
		t.Fatalf("expected ref %q from index.json, got %q", ref, got)
	}
}

func TestOpen_GzipTransparency(t *testing.T) {
	dir := t.TempDir()
	ref := "test.example.io/hello:v1"
	plainTarPath := buildOciLayoutTarFlat(t, dir, ref, ociManifestSpec{os: "linux", arch: "amd64", content: "hello"})

	for _, ext := range []string{".tar.gz", ".tgz"} {
		t.Run(ext, func(t *testing.T) {
			gzPath := filepath.Join(dir, "compressed"+ext)
			gzipFile(t, plainTarPath, gzPath)

			r, err := Open(gzPath, "", "")
			if err != nil {
				t.Fatalf("Open(%q): %s", gzPath, err)
			}
			defer r.Close()

			var got string
			for me, err := range r.Manifests() {
				if err != nil {
					t.Fatalf("Manifests: %s", err)
				}
				got = me.Ref
			}
			if got != ref {
				t.Fatalf("expected ref %q, got %q", ref, got)
			}
		})
	}
}

func TestOpen_NonexistentFile(t *testing.T) {
	if _, err := Open("/does/not/exist.tar", "", ""); err == nil {
		t.Fatalf("expected an error opening a nonexistent path")
	}
}

func TestPlatformMatches(t *testing.T) {
	cases := []struct {
		name                       string
		wantOs, wantArch, os, arch string
		want                       bool
	}{
		{"no filter matches anything", "", "", "windows", "arm", true},
		{"exact match", "linux", "amd64", "linux", "amd64", true},
		{"case insensitive", "Linux", "AMD64", "linux", "amd64", true},
		{"os mismatch", "linux", "amd64", "windows", "amd64", false},
		{"arch mismatch", "linux", "amd64", "linux", "arm64", false},
		{"only os filtered, matches", "linux", "", "linux", "anything", true},
		{"only os filtered, mismatch", "linux", "", "windows", "anything", false},
		{"only arch filtered, mismatch", "", "amd64", "anything", "arm64", false},
	}
	for _, tc := range cases {
		if got := platformMatches(tc.wantOs, tc.wantArch, tc.os, tc.arch); got != tc.want {
			t.Errorf("%s: platformMatches(%q,%q,%q,%q) = %v, want %v",
				tc.name, tc.wantOs, tc.wantArch, tc.os, tc.arch, got, tc.want)
		}
	}
}
