package tarball

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	imgpulltar "github.com/aceeric/imgpull/internal/tar"
	"github.com/aceeric/imgpull/pkg/imgpull/types"
	"github.com/aceeric/imgpull/pkg/imgpull/v2docker"
)

// dockerSaveImage is one manifest.json array element, already resolved
// down to the plain, ManifestHolder-agnostic values Manifests() needs to
// yield - built once in parseDockerSave rather than deferred to iteration
// time, since building the synthesized manifest bytes and building the
// shared digest->filename lookup (dockerBlobFiles) both require reading
// and hashing the same referenced files anyway; doing that once here means
// Blobs() never has to re-hash.
type dockerSaveImage struct {
	ref            string
	mediaType      string
	manifestBytes  []byte
	manifestDigest string
	buildErr       error // set if this entry couldn't be built; reported by Manifests(), not fatal to Open
}

// parseDockerSave reads manifest.json (a JSON array, one element per image)
// using internal/tar's own DockerTarManifest struct, so this package and
// the writer share one definition of the format rather than two
// independently-maintained ones. Every referenced file (Config, each
// Layers entry) is looked up by its exact literal name as given in
// manifest.json - no assumption about naming convention, extension, or
// nesting, since real `docker save` and this project's own ToTar name
// these files differently from each other.
//
// If r.wantOs/r.wantArch is non-empty, an entry whose image config's own
// os/architecture doesn't match is excluded from r.dockerManifests
// entirely (not yielded later as an error - it's a normal, expected
// exclusion, same as a non-matching OCI-layout list member). This format
// has no list structure to filter within (see dockerSaveManifests), so
// filtering has to happen per flat entry, against the one place platform
// info actually lives for this format: the image config JSON, not
// anything in manifest.json itself. An entry whose config can't even be
// read is still recorded (as a buildErr) regardless of the filter - a
// real read failure is worth surfacing even if it might have been
// filtered out.
func (r *Reader) parseDockerSave() error {
	var dtms []imgpulltar.DockerTarManifest
	if err := r.unmarshalIndexed("manifest.json", &dtms); err != nil {
		return err
	}
	r.dockerBlobFiles = map[string]string{}
	for _, dtm := range dtms {
		ref := ""
		if len(dtm.RepoTags) > 0 {
			ref = dtm.RepoTags[0] // additional tags beyond the first aren't modeled by this package today
		}
		mediaType, manifestBytes, manifestDigest, plat, err := r.buildDockerSaveManifest(dtm)
		if err != nil {
			r.dockerManifests = append(r.dockerManifests, dockerSaveImage{ref: ref, buildErr: err})
			continue
		}
		if !platformMatches(r.wantOs, r.wantArch, plat.os, plat.arch) {
			continue // excluded by platform filter - not an error
		}
		entry := dockerSaveImage{mediaType: mediaType, manifestBytes: manifestBytes, manifestDigest: manifestDigest, ref: ref}
		if ref == "" {
			entry = dockerSaveImage{buildErr: fmt.Errorf("image (config %s) has no RepoTags - a ref is required", dtm.Config)}
		}
		r.dockerManifests = append(r.dockerManifests, entry)
	}
	return nil
}

// platform is the subset of an image config JSON's fields this package
// needs to filter docker-save entries - deliberately not the full OCI
// image-config schema, just enough to compare against r.wantOs/r.wantArch.
type platform struct {
	os   string
	arch string
}

// buildDockerSaveManifest synthesizes a real v2docker.Manifest for one
// manifest.json entry - manifest.json itself is docker's own
// {Config,RepoTags,Layers} bookkeeping format, not a distribution-spec
// manifest, so one has to be constructed, referencing the config and layer
// blobs by their real content digests (computed by hashing, since neither
// real `docker save` nor ToTar guarantee the referenced filenames
// themselves are content-addressed). Every referenced file's digest is
// also recorded in r.dockerBlobFiles as a side effect, so Blobs() can look
// blobs up in O(1) later instead of re-hashing. The returned platform
// comes from the image config's own top-level "os"/"architecture" fields
// (per the OCI image-config spec) - the only place platform info exists
// for a flat docker-save entry, since (unlike an OCI-layout list member)
// there's no descriptor with its own Platform field to read instead.
func (r *Reader) buildDockerSaveManifest(dtm imgpulltar.DockerTarManifest) (mediaType string, manifestBytes []byte, manifestDigest string, plat platform, err error) {
	configBytes, err := r.readAt(dtm.Config)
	if err != nil {
		return "", nil, "", platform{}, fmt.Errorf("config %q referenced by manifest.json not found in tarball: %w", dtm.Config, err)
	}
	configDigest := digestOf(configBytes)
	r.dockerBlobFiles[configDigest] = dtm.Config

	var cfg struct {
		Architecture string `json:"architecture"`
		Os           string `json:"os"`
	}
	if err := json.Unmarshal(configBytes, &cfg); err != nil {
		return "", nil, "", platform{}, fmt.Errorf("config %q is not valid JSON: %w", dtm.Config, err)
	}
	plat = platform{os: cfg.Os, arch: cfg.Architecture}

	var layerDescs []v2docker.Descriptor
	for _, layerPath := range dtm.Layers {
		layerBytes, err := r.readAt(layerPath)
		if err != nil {
			return "", nil, "", platform{}, fmt.Errorf("layer %q referenced by manifest.json not found in tarball: %w", layerPath, err)
		}
		digest := digestOf(layerBytes)
		r.dockerBlobFiles[digest] = layerPath
		layerDescs = append(layerDescs, v2docker.Descriptor{
			MediaType: string(layerMediaType(layerBytes)),
			Digest:    digest,
			Size:      int64(len(layerBytes)),
		})
	}

	manifest := v2docker.Manifest{
		SchemaVersion: 2,
		MediaType:     string(types.V2dockerManifestMt),
		Config: v2docker.Descriptor{
			MediaType: "application/vnd.docker.container.image.v1+json",
			Digest:    configDigest,
			Size:      int64(len(configBytes)),
		},
		Layers: layerDescs,
	}
	manifestBytes, err = json.Marshal(manifest)
	if err != nil {
		return "", nil, "", platform{}, err
	}
	return string(types.V2dockerManifestMt), manifestBytes, digestOf(manifestBytes), plat, nil
}

// dockerSaveManifests yields one ManifestEntry per (already platform-
// filtered, in parseDockerSave) manifest.json array element. There is no
// list-then-children structure to recurse into for this format: real
// `docker save` only ever bundles what's already been resolved to
// specific platform(s) locally, and internal/tar.ToTar only ever writes
// one already-flattened image - a manifest.json entry that is itself an
// image-list does not occur in practice. This is a genuine property of the
// format, not a current limitation of this function.
func (r *Reader) dockerSaveManifests(yield func(ManifestEntry, error) bool) {
	for _, entry := range r.dockerManifests {
		if entry.buildErr != nil {
			if !yield(ManifestEntry{}, entry.buildErr) {
				return
			}
			continue
		}
		me := ManifestEntry{
			MediaType: entry.mediaType,
			Bytes:     entry.manifestBytes,
			Digest:    entry.manifestDigest,
			Ref:       entry.ref,
		}
		if !yield(me, nil) {
			return
		}
	}
}

// digestOf returns the sha256 digest of b in "sha256:<hex>" form.
func digestOf(b []byte) string {
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// layerMediaType sniffs the gzip magic bytes to tell a compressed layer tar
// from an uncompressed one, since manifest.json itself doesn't declare it.
func layerMediaType(b []byte) types.MediaType {
	if len(b) >= 2 && b[0] == 0x1f && b[1] == 0x8b {
		return types.V2dockerLayerGzipMt
	}
	return types.V2dockerLayerMt
}

// dockerSaveBlobs yields (Blob, err) for every entry in wanted, via the
// digest->filename map built once in parseDockerSave - O(1) per lookup, no
// re-hashing.
func (r *Reader) dockerSaveBlobs(wanted []types.Layer, yield func(Blob, error) bool) {
	for _, l := range wanted {
		name, ok := r.dockerBlobFiles[l.Digest]
		if !ok {
			if !yield(Blob{}, fmt.Errorf("blob %s not found in tarball", l.Digest)) {
				return
			}
			continue
		}
		reader, err := r.sectionReaderFor(name)
		if err != nil {
			if !yield(Blob{}, err) {
				return
			}
			continue
		}
		if !yield(Blob{Digest: l.Digest, Reader: reader}, nil) {
			return
		}
	}
}
