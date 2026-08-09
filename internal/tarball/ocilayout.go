package tarball

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/aceeric/imgpull/internal/imgref"
	"github.com/aceeric/imgpull/pkg/imgpull/types"
	"github.com/aceeric/imgpull/pkg/imgpull/v1oci"
	"github.com/aceeric/imgpull/pkg/imgpull/v2docker"
)

// ociLayoutIndex holds the parsed top-level index.json for an OCI-layout
// tarball.
type ociLayoutIndex struct {
	idx v1oci.Index
}

// parseOciLayout reads index.json. The actual manifest/blob content isn't
// read here - just the top-level descriptor list - real parsing happens
// lazily per-entry in ociLayoutManifests/walk, since blobs are already
// content-addressed and cheap to look up on demand.
func (r *Reader) parseOciLayout() error {
	var idx v1oci.Index
	if err := r.unmarshalIndexed("index.json", &idx); err != nil {
		return err
	}
	r.ociIndex = ociLayoutIndex{idx: idx}
	return nil
}

// ociLayoutManifests walks every top-level index.json entry and, for each,
// recursively walks into any image-list to yield its members too - a flat,
// depth-first, parent-before-children stream. A top-level entry itself is
// always yielded regardless of r.wantOs/r.wantArch (it represents a whole
// logical image, not a specific platform - there is nothing on it to
// filter against); filtering is applied to its members instead, in
// childDigestsOf.
func (r *Reader) ociLayoutManifests(yield func(ManifestEntry, error) bool) {
	for _, desc := range r.ociIndex.idx.Manifests {
		ref := refFromAnnotations(desc.Annotations)
		if ref == "" {
			if !yield(ManifestEntry{}, fmt.Errorf("manifest %s has no usable ref (no io.containerd.image.name or org.opencontainers.image.ref.name annotation)", desc.Digest)) {
				return
			}
			continue
		}
		if !r.walk(desc.Digest, ref, yield) {
			return
		}
	}
}

// refFromAnnotations extracts the best available ref from an OCI-layout
// manifest descriptor's annotations. Real producers disagree about what
// goes in the OCI spec's own "org.opencontainers.image.ref.name"
// annotation: per spec it's just a "reference name" - historically meant
// for local lookup via an OCI layout's refs/ directory, not necessarily a
// fully-qualified ref - and Docker's containerd-backed `docker save`
// populates it with exactly that: a bare tag like "3.10.2" (confirmed
// against a real `docker save registry.k8s.io/pause:3.10.2` tarball).
// Docker adds its own "io.containerd.image.name" annotation alongside it,
// carrying the real fully-qualified ref (e.g. "registry.k8s.io/pause:3.10.2")
// - preferred here when present. containerd's own `ctr image export`, by
// contrast, conventionally puts the full ref directly in the spec
// annotation (no io.containerd.image.name at all), so that remains the
// fallback rather than being dropped.
func refFromAnnotations(annotations map[string]string) string {
	if v := annotations["io.containerd.image.name"]; v != "" {
		return v
	}
	return annotations["org.opencontainers.image.ref.name"]
}

// walk reads the manifest blob at digest, yields it under ref, and - if
// it's a list - recurses into every member, building each member's own
// ImageUrl in digest-form via imgref.UrlWithDigest.
//
// KNOWN BUG THIS FUNCTION WILL FAITHFULLY REPRODUCE, DELIBERATELY LEFT
// UNFIXED FOR NOW: when ref is itself already digest-form (e.g. this whole
// walk started from something like
// "quay.io/cilium/hubble-relay@sha256:<list digest>" - exactly what
// happens resolving a manifest list by digest), internal/imgref.ImageRef's
// makeUrl gives priority to the receiver's OWN already-set digest over the
// digest explicitly passed to UrlWithDigest - so every child in this
// situation incorrectly ends up with the LIST's digest in its ImageUrl
// instead of its own (confirmed via a real repro: `imgpull
// quay.io/cilium/hubble-relay@sha256:...` followed by inspecting the
// resulting tarball's manifest.json). This function calls UrlWithDigest
// exactly as intended - asking for substitution - and gets back the wrong
// answer because the callee ignores the request in this specific case.
// This is a defect in internal/imgref itself (see makeUrl), not something
// introduced here; fixing it there (plus a dedicated regression test in
// that package) will make this function correct automatically, with no
// changes needed in this file.
func (r *Reader) walk(digest string, ref string, yield func(ManifestEntry, error) bool) bool {
	blob, err := r.readAt(ociBlobPath(digest))
	if err != nil {
		return yield(ManifestEntry{}, fmt.Errorf("manifest blob %s not found in tarball: %w", digest, err))
	}

	var probe struct {
		MediaType string `json:"mediaType"`
	}
	if err := json.Unmarshal(blob, &probe); err != nil {
		return yield(ManifestEntry{}, fmt.Errorf("manifest %s is not valid JSON: %w", digest, err))
	}

	if !yield(ManifestEntry{MediaType: probe.MediaType, Bytes: blob, Digest: digest, Ref: ref}, nil) {
		return false
	}

	childDigests, err := childDigestsOf(probe.MediaType, blob, r.wantOs, r.wantArch)
	if err != nil {
		return yield(ManifestEntry{}, fmt.Errorf("parsing manifest %s: %w", digest, err))
	}
	if len(childDigests) == 0 {
		return true
	}

	ir, err := imgref.NewImageRef(ref, "", "")
	if err != nil {
		return yield(ManifestEntry{}, fmt.Errorf("unable to parse ref %q for list %s: %w", ref, digest, err))
	}

	for _, childDigest := range childDigests {
		childRef := ir.UrlWithDigest(childDigest)
		if !r.walk(childDigest, childRef, yield) {
			return false
		}
	}
	return true
}

// childDigestsOf returns the member digests if mediaType/blob represent a
// manifest list/index, or nil if it's a plain image manifest. This mirrors
// what ManifestHolder.ImageManifestDigests() does one level up - it's
// re-implemented here rather than called, since this package can't depend
// on pkg/imgpull (see the package doc).
//
// If wantOs/wantArch is non-empty, a member is excluded unless its own
// platform matches (case-insensitive, via platformMatches). A member with
// no Platform at all is excluded whenever either filter is non-empty,
// rather than assumed to match - the OCI/Docker spec expects every real
// manifest-list member descriptor to declare a platform, so a missing one
// is treated as "unknown, don't guess" rather than "matches everything."
func childDigestsOf(mediaType string, blob []byte, wantOs string, wantArch string) ([]string, error) {
	switch types.MediaType(mediaType) {
	case types.V1ociIndexMt:
		var idx v1oci.Index
		if err := json.Unmarshal(blob, &idx); err != nil {
			return nil, err
		}
		var digests []string
		for _, m := range idx.Manifests {
			if m.Platform == nil {
				if wantOs != "" || wantArch != "" {
					continue
				}
			} else if !platformMatches(wantOs, wantArch, m.Platform.Os, m.Platform.Architecture) {
				continue
			}
			digests = append(digests, m.Digest)
		}
		return digests, nil
	case types.V2dockerManifestListMt:
		var ml v2docker.ManifestList
		if err := json.Unmarshal(blob, &ml); err != nil {
			return nil, err
		}
		var digests []string
		for _, m := range ml.Manifests {
			if m.Platform == nil {
				if wantOs != "" || wantArch != "" {
					continue
				}
			} else if !platformMatches(wantOs, wantArch, m.Platform.OS, m.Platform.Architecture) {
				continue
			}
			digests = append(digests, m.Digest)
		}
		return digests, nil
	default:
		return nil, nil // plain image manifest, no children
	}
}

// ociBlobPath returns the tar entry name for a content-addressed OCI-layout
// blob, e.g. "sha256:<hex>" -> "blobs/sha256/<hex>". digest is always
// "sha256:<hex>" per the OCI/Docker descriptor convention used throughout
// this codebase.
func ociBlobPath(digest string) string {
	return "blobs/sha256/" + strings.TrimPrefix(digest, "sha256:")
}

// ociLayoutBlobs yields (Blob, err) for every entry in wanted - trivial for
// this format since blobs are already content-addressed at
// blobs/sha256/<hex>, no lookup table or hashing needed (contrast with
// dockerSaveBlobs).
func (r *Reader) ociLayoutBlobs(wanted []types.Layer, yield func(Blob, error) bool) {
	for _, l := range wanted {
		reader, err := r.sectionReaderFor(ociBlobPath(l.Digest))
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
