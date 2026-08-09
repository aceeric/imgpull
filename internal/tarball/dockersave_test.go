package tarball

import (
	"encoding/json"
	"io"
	"path/filepath"
	"testing"

	"github.com/aceeric/imgpull/pkg/imgpull/types"
)

// dockerSaveImageSpec describes one image to embed in a hand-built
// docker-save-shaped tarball.
type dockerSaveImageSpec struct {
	ref     string
	os      string
	arch    string
	content string // layer content - vary per image so digests differ
}

// buildDockerSaveTar hand-builds a tarball shaped like real `docker save`
// output (and like this project's own internal/tar.ToTar output): a root
// manifest.json array, each element referencing a config file and layer
// file by their real content digests.
func buildDockerSaveTar(t *testing.T, dir string, specs []dockerSaveImageSpec) string {
	t.Helper()
	entries := map[string][]byte{}
	manifestEntries := []map[string]any{}

	for _, spec := range specs {
		_, gz := gzipLayer(t, spec.content)
		layerHex := digestHex(gz)
		entries[layerHex+".tar.gz"] = gz

		cfg := imageConfig(spec.os, spec.arch)
		cfgHex := digestHex(cfg)
		entries[cfgHex+".json"] = cfg

		var repoTags []string
		if spec.ref != "" {
			repoTags = []string{spec.ref}
		}
		manifestEntries = append(manifestEntries, map[string]any{
			"Config":   cfgHex + ".json",
			"RepoTags": repoTags,
			"Layers":   []string{layerHex + ".tar.gz"},
		})
	}

	manifestJSON, err := json.Marshal(manifestEntries)
	if err != nil {
		t.Fatalf("marshalling manifest.json: %s", err)
	}
	entries["manifest.json"] = manifestJSON

	tarPath := filepath.Join(dir, "docker-save.tar")
	writeTarFile(t, tarPath, entries)
	return tarPath
}

func TestDockerSave_SingleImage(t *testing.T) {
	dir := t.TempDir()
	ref := "test.example.io/hello:v1"
	tarPath := buildDockerSaveTar(t, dir, []dockerSaveImageSpec{
		{ref: ref, os: "linux", arch: "amd64", content: "hello world"},
	})

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
	if got[0].MediaType != string(types.V2dockerManifestMt) {
		t.Fatalf("unexpected media type %q", got[0].MediaType)
	}
	if "sha256:"+digestHex(got[0].Bytes) != got[0].Digest {
		t.Fatalf("Digest %q does not match sha256 of the yielded Bytes", got[0].Digest)
	}
}

func TestDockerSave_MultiImage(t *testing.T) {
	dir := t.TempDir()
	refs := []string{"test.example.io/one:v1", "test.example.io/two:v1"}
	tarPath := buildDockerSaveTar(t, dir, []dockerSaveImageSpec{
		{ref: refs[0], os: "linux", arch: "amd64", content: "one"},
		{ref: refs[1], os: "linux", arch: "amd64", content: "two"},
	})

	r, err := Open(tarPath, "", "")
	if err != nil {
		t.Fatalf("Open: %s", err)
	}
	defer r.Close()

	found := map[string]bool{}
	var digests []string
	for me, err := range r.Manifests() {
		if err != nil {
			t.Fatalf("Manifests: %s", err)
		}
		found[me.Ref] = true
		digests = append(digests, me.Digest)
	}
	for _, ref := range refs {
		if !found[ref] {
			t.Fatalf("expected a manifest for %q, not found", ref)
		}
	}
	if len(digests) == 2 && digests[0] == digests[1] {
		t.Fatalf("two images with different content produced the same manifest digest")
	}
}

func TestDockerSave_MissingConfig(t *testing.T) {
	dir := t.TempDir()
	tarPath := filepath.Join(dir, "bad.tar")
	manifestJSON, _ := json.Marshal([]map[string]any{
		{"Config": "blobs/sha256/doesnotexist", "RepoTags": []string{"test.example.io/hello:v1"}, "Layers": []string{}},
	})
	writeTarFile(t, tarPath, map[string][]byte{"manifest.json": manifestJSON})

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
		t.Fatalf("expected an error for a manifest.json entry referencing a missing config file")
	}
}

func TestDockerSave_MissingLayer(t *testing.T) {
	dir := t.TempDir()
	cfg := imageConfig("linux", "amd64")
	cfgHex := digestHex(cfg)
	tarPath := filepath.Join(dir, "bad.tar")
	manifestJSON, _ := json.Marshal([]map[string]any{
		{"Config": cfgHex + ".json", "RepoTags": []string{"test.example.io/hello:v1"}, "Layers": []string{"blobs/sha256/doesnotexist"}},
	})
	writeTarFile(t, tarPath, map[string][]byte{
		cfgHex + ".json": cfg,
		"manifest.json":  manifestJSON,
	})

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
		t.Fatalf("expected an error for a manifest.json entry referencing a missing layer file")
	}
}

func TestDockerSave_NoRepoTags(t *testing.T) {
	dir := t.TempDir()
	tarPath := buildDockerSaveTar(t, dir, []dockerSaveImageSpec{
		{ref: "", os: "linux", arch: "amd64", content: "hello"},
	})

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
		t.Fatalf("expected an error for an image with no RepoTags")
	}
}

func TestDockerSave_PlatformFilter(t *testing.T) {
	dir := t.TempDir()
	amd64Ref := "test.example.io/hello:amd64"
	arm64Ref := "test.example.io/hello:arm64"
	tarPath := buildDockerSaveTar(t, dir, []dockerSaveImageSpec{
		{ref: amd64Ref, os: "linux", arch: "amd64", content: "amd64 content"},
		{ref: arm64Ref, os: "linux", arch: "arm64", content: "arm64 content"},
	})

	r, err := Open(tarPath, "linux", "amd64")
	if err != nil {
		t.Fatalf("Open: %s", err)
	}
	defer r.Close()

	var got []string
	for me, err := range r.Manifests() {
		if err != nil {
			t.Fatalf("Manifests: %s", err)
		}
		got = append(got, me.Ref)
	}
	if len(got) != 1 || got[0] != amd64Ref {
		t.Fatalf("expected only %q, got %v", amd64Ref, got)
	}
}

func TestDockerSave_PlatformFilter_NoMatchYieldsNothing(t *testing.T) {
	dir := t.TempDir()
	tarPath := buildDockerSaveTar(t, dir, []dockerSaveImageSpec{
		{ref: "test.example.io/hello:v1", os: "linux", arch: "amd64", content: "hello"},
	})

	r, err := Open(tarPath, "linux", "arm64")
	if err != nil {
		t.Fatalf("Open: %s", err)
	}
	defer r.Close()

	count := 0
	for _, err := range r.Manifests() {
		if err != nil {
			t.Fatalf("Manifests: %s", err)
		}
		count++
	}
	if count != 0 {
		t.Fatalf("expected 0 manifests when nothing matches the platform filter, got %d", count)
	}
}

func TestDockerSave_Blobs(t *testing.T) {
	dir := t.TempDir()
	ref := "test.example.io/hello:v1"
	tarPath := buildDockerSaveTar(t, dir, []dockerSaveImageSpec{
		{ref: ref, os: "linux", arch: "amd64", content: "hello world"},
	})

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
		t.Fatalf("parsing synthesized manifest: %s", err)
	}
	wanted := []types.Layer{{Digest: m.Config.Digest}}
	for _, l := range m.Layers {
		wanted = append(wanted, types.Layer{Digest: l.Digest})
	}
	if len(wanted) != 2 {
		t.Fatalf("expected 2 wanted blobs (config + 1 layer), got %d", len(wanted))
	}

	seen := map[string]bool{}
	for b, err := range r.Blobs(wanted) {
		if err != nil {
			t.Fatalf("Blobs: %s", err)
		}
		content, err := io.ReadAll(b.Reader)
		if err != nil {
			t.Fatalf("reading blob %s: %s", b.Digest, err)
		}
		if "sha256:"+digestHex(content) != b.Digest {
			t.Fatalf("blob %s content does not hash to its own claimed digest", b.Digest)
		}
		seen[b.Digest] = true
	}
	if len(seen) != len(wanted) {
		t.Fatalf("expected %d distinct blobs, saw %d", len(wanted), len(seen))
	}
}

func TestDockerSave_Blobs_MissingDigest(t *testing.T) {
	dir := t.TempDir()
	ref := "test.example.io/hello:v1"
	tarPath := buildDockerSaveTar(t, dir, []dockerSaveImageSpec{
		{ref: ref, os: "linux", arch: "amd64", content: "hello world"},
	})

	r, err := Open(tarPath, "", "")
	if err != nil {
		t.Fatalf("Open: %s", err)
	}
	defer r.Close()

	sawErr := false
	for _, err := range r.Blobs([]types.Layer{{Digest: "sha256:" + digestHex([]byte("not actually present"))}}) {
		if err != nil {
			sawErr = true
		}
	}
	if !sawErr {
		t.Fatalf("expected an error requesting a digest that isn't in the tarball")
	}
}
