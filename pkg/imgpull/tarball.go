package imgpull

import (
	"fmt"
	"io"
	"iter"
	"os"
	"path/filepath"

	"github.com/aceeric/imgpull/internal/tarball"
	"github.com/aceeric/imgpull/internal/util"
	"github.com/aceeric/imgpull/pkg/imgpull/types"
)

// ImageTarBall provides read access to the manifests and blobs in an image
// tarball - both the docker-save-shaped format (produced by `docker save`,
// and by this project's own ToTar) and genuine OCI-layout tarballs
// (produced by `ctr image export`, buildkit's `type=oci` output, `crane
// ... --format oci`), optionally gzip-compressed (.tar.gz/.tgz).
//
// This type is a thin wrapper over internal/tarball.Reader: all tarball
// format comprehension lives there; this file's only job is converting the
// plain values that package yields (ManifestEntry, Blob) into their
// public-facing equivalents here (ManifestHolder, Blob) - for manifests,
// that's a real conversion via NewManifestHolder; for blobs, it's a 1:1
// field copy into this package's own Blob type, done so that neither this
// package's public API nor its callers ever need to import or name a type
// from internal/tarball, which they couldn't do anyway.
//
// Create with OpenImageTarBall; call Close when done.
type ImageTarBall struct {
	r *tarball.Reader
}

// Blob pairs one blob's digest with a reader over its content, as yielded
// by TarBlobReader.
type Blob struct {
	Digest string
	Reader io.Reader
}

// OpenImageTarBall opens the image tarball at path for reading.
//
// os/arch filter which manifests TarManifestReader yields - pass "" for
// either (or both) to disable filtering on that dimension. For an OCI-
// layout tarball, a manifest-list's own entry is always yielded regardless
// (it isn't itself platform-specific); its members are only yielded if
// their platform matches. For a docker-save tarball, which has no list
// structure, an entire image is included or excluded based on its own
// image config's os/architecture. See internal/tarball.Open for the full
// rationale.
func OpenImageTarBall(path string, os string, arch string) (*ImageTarBall, error) {
	r, err := tarball.Open(path, os, arch)
	if err != nil {
		return nil, err
	}
	return &ImageTarBall{r: r}, nil
}

// Close releases the resources associated with the receiver - the open
// file handle, and, if the source tarball was gzip-compressed, the temp
// file created to hold the decompressed copy.
func (itb *ImageTarBall) Close() error {
	return itb.r.Close()
}

// TarManifestReader iterates every manifest in the tarball - image
// manifests and image-list manifests alike, flattened into a single
// depth-first stream with parents yielded before their children (docker-
// save tarballs never have children to yield; OCI-layout tarballs
// sometimes do), filtered by the os/arch passed to OpenImageTarBall (see
// its doc comment for exactly how each format applies the filter).
//
// err is non-nil for an entry that couldn't be parsed (unreadable blob,
// invalid JSON, missing ref) - the caller decides what to do (stop, skip,
// warn) by returning false from its range body to stop iteration, or
// continuing to let the walk proceed past a bad entry. A platform-excluded
// entry is simply absent from the stream, not reported as an error.
func (itb *ImageTarBall) TarManifestReader() iter.Seq2[ManifestHolder, error] {
	return func(yield func(ManifestHolder, error) bool) {
		for entry, err := range itb.r.Manifests() {
			if err != nil {
				if !yield(ManifestHolder{}, err) {
					return
				}
				continue
			}
			mh, err := NewManifestHolder(entry.MediaType, entry.Bytes, util.DigestFrom(entry.Digest), entry.Ref)
			if !yield(mh, err) {
				return
			}
		}
	}
}

// TarBlobReader iterates every blob (layers + config) that mh.Layers()
// says it needs, yielding (Blob, err) for each. err is set if a
// referenced digest can't be found in this tarball.
//
// mh should be a ManifestHolder obtained from THIS ImageTarBall's own
// TarManifestReader. There is no guard against passing one from elsewhere
// (a different tarball, or one loaded from the file system cache) -
// ManifestHolder carries no backreference to its source, deliberately,
// since it's used elsewhere in this codebase as plain, JSON-serializable
// data. Doing so simply reports "not found" for every requested digest
// rather than producing a wrong result, so no special-case guard is needed
// here.
func (itb *ImageTarBall) TarBlobReader(mh ManifestHolder) iter.Seq2[Blob, error] {
	return func(yield func(Blob, error) bool) {
		for b, err := range itb.r.Blobs(mh.Layers()) {
			if err != nil {
				if !yield(Blob{}, err) {
					return
				}
				continue
			}
			if !yield(Blob{Digest: b.Digest, Reader: b.Reader}, nil) {
				return
			}
		}
	}
}

// SaveBlobs writes every blob (layers + config) mh.Layers() references to
// blobDir, named by bare hex digest (no "sha256:" prefix, via the same
// util.DigestFrom this package already uses elsewhere) - matching exactly
// the on-disk convention Puller.PullBlobs already uses for a live pull, so
// a blob directory looks identical whether it was populated by a network
// pull or a tarball load. blobDir is created if it doesn't exist.
//
// A blob whose target file already exists with the correct size is
// skipped without even being read from the tarball - the same skip-if-
// present check RegClient.V2Blobs already does for a live pull, done here
// before consulting the tarball at all rather than after, for the same
// reason: avoid the work entirely when it's not needed. Like PullBlobs,
// this assumes no concurrent writer to the same blobDir - safe for the
// intended use (loading while ociregistry itself isn't running), not
// intended for use against a live server's blob directory.
//
// Calling this with a manifest-list mh (rather than an image manifest) is
// a harmless no-op: mh.Layers() returns nothing for a list, so this
// creates blobDir (if needed) and returns immediately.
func (itb *ImageTarBall) SaveBlobs(mh ManifestHolder, blobDir string) error {
	if err := os.MkdirAll(blobDir, 0755); err != nil {
		return fmt.Errorf("unable to create directory %q, error: %q", blobDir, err)
	}

	var wanted []types.Layer
	for _, layer := range mh.Layers() {
		toFile := filepath.Join(blobDir, util.DigestFrom(layer.Digest))
		if fi, err := os.Stat(toFile); err == nil && fi.Size() == int64(layer.Size) {
			continue // already present with the correct size - skip
		}
		wanted = append(wanted, layer)
	}
	if len(wanted) == 0 {
		return nil
	}

	for b, err := range itb.r.Blobs(wanted) {
		if err != nil {
			return err
		}
		toFile := filepath.Join(blobDir, util.DigestFrom(b.Digest))
		f, err := os.Create(toFile)
		if err != nil {
			return err
		}
		if _, err := io.Copy(f, b.Reader); err != nil {
			f.Close()
			return err
		}
		if err := f.Close(); err != nil {
			return err
		}
	}
	return nil
}
