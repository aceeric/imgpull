package tarball

import (
	"encoding/json"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aceeric/imgpull/pkg/imgpull/types"
)

// ociManifestSpec describes one leaf image manifest to embed as a
// content-addressed blob.
type ociManifestSpec struct {
	os      string
	arch    string
	content string
}

// buildOciManifestBlob builds a real (tiny) OCI image manifest plus its
// config and layer blobs, writing them into entries and returning the
// manifest's own bytes and digest (hex, no prefix).
func buildOciManifestBlob(t *testing.T, entries map[string][]byte, spec ociManifestSpec) (manifestBytes []byte, manifestDigestHex string) {
	t.Helper()
	_, gz := gzipLayer(t, spec.content)
	layerHex := digestHex(gz)
	entries["blobs/sha256/"+layerHex] = gz

	cfg := imageConfig(spec.os, spec.arch)
	cfgHex := digestHex(cfg)
	entries["blobs/sha256/"+cfgHex] = cfg

	manifest := map[string]any{
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
	}
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshalling manifest: %s", err)
	}
	manifestDigestHex = digestHex(manifestBytes)
	entries["blobs/sha256/"+manifestDigestHex] = manifestBytes
	return manifestBytes, manifestDigestHex
}

// buildOciLayoutTarFlat builds an OCI-layout tarball with a single, flat
// (non-list) top-level manifest.
func buildOciLayoutTarFlat(t *testing.T, dir string, ref string, spec ociManifestSpec) string {
	t.Helper()
	entries := map[string][]byte{}
	_, manifestDigestHex := buildOciManifestBlob(t, entries, spec)

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

	tarPath := filepath.Join(dir, "oci-flat.tar")
	writeTarFile(t, tarPath, entries)
	return tarPath
}

// ociListMember describes one platform member of a manifest list. If omit
// is true, the member's digest is referenced in the list but its blob is
// never written - reproducing the real, confirmed `ctr images export`
// behavior of listing platforms it didn't actually fetch.
type ociListMember struct {
	os      string
	arch    string
	content string
	omit    bool
}

// buildOciLayoutTarList builds an OCI-layout tarball whose single top-level
// index.json entry is itself a manifest list with the given members.
func buildOciLayoutTarList(t *testing.T, dir string, ref string, members []ociListMember) string {
	t.Helper()
	entries := map[string][]byte{}
	var listManifests []map[string]any

	for _, m := range members {
		if m.omit {
			// A digest with no corresponding blob in entries - deterministic
			// but deliberately never written.
			fakeDigestHex := digestHex([]byte("omitted:" + m.content))
			listManifests = append(listManifests, map[string]any{
				"mediaType": "application/vnd.oci.image.manifest.v1+json",
				"digest":    "sha256:" + fakeDigestHex,
				"size":      0,
				"platform":  map[string]string{"os": m.os, "architecture": m.arch},
			})
			continue
		}
		_, memberDigestHex := buildOciManifestBlob(t, entries, ociManifestSpec{os: m.os, arch: m.arch, content: m.content})
		listManifests = append(listManifests, map[string]any{
			"mediaType": "application/vnd.oci.image.manifest.v1+json",
			"digest":    "sha256:" + memberDigestHex,
			"size":      len(entries["blobs/sha256/"+memberDigestHex]),
			"platform":  map[string]string{"os": m.os, "architecture": m.arch},
		})
	}

	list := map[string]any{
		"schemaVersion": 2,
		"mediaType":     "application/vnd.oci.image.index.v1+json",
		"manifests":     listManifests,
	}
	listBytes, err := json.Marshal(list)
	if err != nil {
		t.Fatalf("marshalling list: %s", err)
	}
	listDigestHex := digestHex(listBytes)
	entries["blobs/sha256/"+listDigestHex] = listBytes

	index := map[string]any{
		"schemaVersion": 2,
		"manifests": []map[string]any{
			{
				"mediaType": "application/vnd.oci.image.index.v1+json",
				"digest":    "sha256:" + listDigestHex,
				"size":      len(listBytes),
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

	tarPath := filepath.Join(dir, "oci-list.tar")
	writeTarFile(t, tarPath, entries)
	return tarPath
}

func TestOciLayout_SingleImage(t *testing.T) {
	dir := t.TempDir()
	ref := "test.example.io/hello:v1"
	tarPath := buildOciLayoutTarFlat(t, dir, ref, ociManifestSpec{os: "linux", arch: "amd64", content: "hello"})

	r, err := Open(tarPath, "", "")
	if err != nil {
		t.Fatalf("Open: %s", err)
	}
	defer r.Close()

	var got []ManifestEntry
	for me, err := range r.Manifests() {
		if err != nil {
			t.Fatalf("Manifests: %s", err)
		}
		got = append(got, me)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 manifest, got %d", len(got))
	}
	if got[0].Ref != ref {
		t.Fatalf("expected ref %q, got %q", ref, got[0].Ref)
	}
	if "sha256:"+digestHex(got[0].Bytes) != got[0].Digest {
		t.Fatalf("Digest %q does not match sha256 of the yielded Bytes", got[0].Digest)
	}
}

// TestOciLayout_ListWithChildren_DigestSubstitution is the internal/tarball-
// level regression test for the real bug found via `imgpull
// quay.io/cilium/hubble-relay@sha256:...` and confirmed again via a real
// `ctr images export` of images referenced by digest: walk's call to
// imgref.UrlWithDigest must produce each child's OWN digest, never the
// list's, including - the actual bug trigger - when the list itself was
// reached by a digest-form ref. This complements (doesn't replace) the
// direct unit test in internal/imgref: this one proves the fix holds
// through this package's real recursion, not just the isolated function.
//
// The list's annotation is set to a fabricated digest-form ref rather than
// its own real computed digest - refFromAnnotations reads whatever string
// is there verbatim, it doesn't cross-check it against the list's actual
// digest, so any digest-form string reproduces the bug's trigger condition
// (receiver's own ref already starts with "sha256:") equally well.
func TestOciLayout_ListWithChildren_DigestSubstitution(t *testing.T) {
	dir := t.TempDir()
	repo := "test.example.io/hello"
	digestRef := repo + "@sha256:" + digestHex([]byte("stand-in for a resolved-by-digest list ref"))

	tarPath := buildOciLayoutTarList(t, dir, digestRef, []ociListMember{
		{os: "linux", arch: "amd64", content: "amd64"},
		{os: "linux", arch: "arm64", content: "arm64"},
	})

	r, err := Open(tarPath, "", "")
	if err != nil {
		t.Fatalf("Open: %s", err)
	}
	defer r.Close()

	var refs []string
	for me, err := range r.Manifests() {
		if err != nil {
			t.Fatalf("Manifests: %s", err)
		}
		refs = append(refs, me.Ref)
	}
	if len(refs) != 3 {
		t.Fatalf("expected 3 manifests (list + 2 children), got %d: %v", len(refs), refs)
	}
	if refs[0] != digestRef {
		t.Fatalf("expected the list itself to keep ref %q, got %q", digestRef, refs[0])
	}
	if refs[1] == refs[0] || refs[2] == refs[0] {
		t.Fatalf("a child ended up with the LIST's digest instead of its own (the bug this test guards against): refs=%v", refs)
	}
	if refs[1] == refs[2] {
		t.Fatalf("two children with different platform content ended up with the same ref: %v", refs)
	}
	for _, cr := range refs[1:] {
		if !strings.HasPrefix(cr, repo+"@sha256:") {
			t.Fatalf("expected child ref in digest form off %q, got %q", repo, cr)
		}
	}
}

func TestOciLayout_ListWithChildren_FlattenedOrder(t *testing.T) {
	dir := t.TempDir()
	ref := "test.example.io/hello:v1"
	tarPath := buildOciLayoutTarList(t, dir, ref, []ociListMember{
		{os: "linux", arch: "amd64", content: "amd64"},
		{os: "linux", arch: "arm64", content: "arm64"},
	})

	r, err := Open(tarPath, "", "")
	if err != nil {
		t.Fatalf("Open: %s", err)
	}
	defer r.Close()

	var kinds []string
	for me, err := range r.Manifests() {
		if err != nil {
			t.Fatalf("Manifests: %s", err)
		}
		if me.MediaType == "application/vnd.oci.image.index.v1+json" {
			kinds = append(kinds, "list")
		} else {
			kinds = append(kinds, "image")
		}
	}
	if len(kinds) != 3 || kinds[0] != "list" || kinds[1] != "image" || kinds[2] != "image" {
		t.Fatalf("expected [list image image] order, got %v", kinds)
	}
}

func TestOciLayout_ListWithMissingChildBlob_YieldsErrorAndContinues(t *testing.T) {
	dir := t.TempDir()
	ref := "test.example.io/hello:v1"
	tarPath := buildOciLayoutTarList(t, dir, ref, []ociListMember{
		{os: "linux", arch: "amd64", content: "amd64"},
		{os: "linux", arch: "arm64", content: "arm64", omit: true},
	})

	r, err := Open(tarPath, "", "")
	if err != nil {
		t.Fatalf("Open: %s", err)
	}
	defer r.Close()

	var errCount, okCount int
	for _, err := range r.Manifests() {
		if err != nil {
			errCount++
			continue
		}
		okCount++
	}
	if errCount != 1 {
		t.Fatalf("expected exactly 1 error (the omitted member), got %d", errCount)
	}
	if okCount != 2 {
		t.Fatalf("expected 2 successful yields (list + the one real amd64 child), got %d", okCount)
	}
}

func TestOciLayout_PlatformFilter(t *testing.T) {
	dir := t.TempDir()
	ref := "test.example.io/hello:v1"
	tarPath := buildOciLayoutTarList(t, dir, ref, []ociListMember{
		{os: "linux", arch: "amd64", content: "amd64"},
		{os: "linux", arch: "arm64", content: "arm64"},
	})

	r, err := Open(tarPath, "linux", "amd64")
	if err != nil {
		t.Fatalf("Open: %s", err)
	}
	defer r.Close()

	var imageCount int
	for me, err := range r.Manifests() {
		if err != nil {
			t.Fatalf("Manifests: %s", err)
		}
		if me.MediaType != "application/vnd.oci.image.index.v1+json" {
			imageCount++
		}
	}
	if imageCount != 1 {
		t.Fatalf("expected exactly 1 image manifest (amd64 only), got %d", imageCount)
	}
}

func TestOciLayout_PlatformFilter_ListItselfAlwaysYielded(t *testing.T) {
	dir := t.TempDir()
	ref := "test.example.io/hello:v1"
	tarPath := buildOciLayoutTarList(t, dir, ref, []ociListMember{
		{os: "linux", arch: "amd64", content: "amd64"},
	})

	// Filter for a platform that doesn't exist at all - the list itself
	// should still be yielded (it isn't platform-specific), only its
	// member should be excluded.
	r, err := Open(tarPath, "windows", "arm")
	if err != nil {
		t.Fatalf("Open: %s", err)
	}
	defer r.Close()

	var kinds []string
	for me, err := range r.Manifests() {
		if err != nil {
			t.Fatalf("Manifests: %s", err)
		}
		if me.MediaType == "application/vnd.oci.image.index.v1+json" {
			kinds = append(kinds, "list")
		} else {
			kinds = append(kinds, "image")
		}
	}
	if len(kinds) != 1 || kinds[0] != "list" {
		t.Fatalf("expected only the list itself, got %v", kinds)
	}
}

func TestRefFromAnnotations(t *testing.T) {
	cases := []struct {
		name        string
		annotations map[string]string
		want        string
	}{
		{
			"prefers containerd name",
			map[string]string{
				"io.containerd.image.name":          "registry.k8s.io/pause:3.10.2",
				"org.opencontainers.image.ref.name": "3.10.2",
			},
			"registry.k8s.io/pause:3.10.2",
		},
		{
			"falls back to spec annotation",
			map[string]string{"org.opencontainers.image.ref.name": "quay.io/cilium/hubble-relay:v1.16.3"},
			"quay.io/cilium/hubble-relay:v1.16.3",
		},
		{"neither present", map[string]string{}, ""},
	}
	for _, tc := range cases {
		if got := refFromAnnotations(tc.annotations); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestOciLayout_NoRefAnnotation(t *testing.T) {
	dir := t.TempDir()
	entries := map[string][]byte{}
	_, manifestDigestHex := buildOciManifestBlob(t, entries, ociManifestSpec{os: "linux", arch: "amd64", content: "hello"})
	index := map[string]any{
		"schemaVersion": 2,
		"manifests": []map[string]any{
			{
				"mediaType":   "application/vnd.oci.image.manifest.v1+json",
				"digest":      "sha256:" + manifestDigestHex,
				"size":        0,
				"annotations": map[string]string{},
			},
		},
	}
	indexBytes, _ := json.Marshal(index)
	entries["index.json"] = indexBytes
	entries["oci-layout"] = []byte(`{"imageLayoutVersion":"1.0.0"}`)
	tarPath := filepath.Join(dir, "no-ref.tar")
	writeTarFile(t, tarPath, entries)

	r, err := Open(tarPath, "", "")
	if err != nil {
		t.Fatalf("Open: %s", err)
	}
	defer r.Close()

	sawErr := false
	for _, err := range r.Manifests() {
		if err != nil {
			sawErr = true
		}
	}
	if !sawErr {
		t.Fatalf("expected an error for a manifest with no usable ref annotation")
	}
}

func TestOciLayout_Blobs(t *testing.T) {
	dir := t.TempDir()
	ref := "test.example.io/hello:v1"
	tarPath := buildOciLayoutTarFlat(t, dir, ref, ociManifestSpec{os: "linux", arch: "amd64", content: "hello"})

	r, err := Open(tarPath, "", "")
	if err != nil {
		t.Fatalf("Open: %s", err)
	}
	defer r.Close()

	var me ManifestEntry
	for entry, err := range r.Manifests() {
		if err != nil {
			t.Fatalf("Manifests: %s", err)
		}
		me = entry
	}

	var m struct {
		Config struct {
			Digest string `json:"digest"`
		} `json:"config"`
		Layers []struct {
			Digest string `json:"digest"`
		} `json:"layers"`
	}
	if err := json.Unmarshal(me.Bytes, &m); err != nil {
		t.Fatalf("parsing manifest: %s", err)
	}
	wanted := []types.Layer{{Digest: m.Config.Digest}}
	for _, l := range m.Layers {
		wanted = append(wanted, types.Layer{Digest: l.Digest})
	}

	count := 0
	for b, err := range r.Blobs(wanted) {
		if err != nil {
			t.Fatalf("Blobs: %s", err)
		}
		content, err := io.ReadAll(b.Reader)
		if err != nil {
			t.Fatalf("reading blob: %s", err)
		}
		if "sha256:"+digestHex(content) != b.Digest {
			t.Fatalf("blob content does not match its own claimed digest")
		}
		count++
	}
	if count != len(wanted) {
		t.Fatalf("expected %d blobs, got %d", len(wanted), count)
	}
}
