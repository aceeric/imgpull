package tarball

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"iter"
	"os"
	"strings"

	"github.com/aceeric/imgpull/pkg/imgpull/types"
)

// ManifestEntry is one manifest found in a tarball - a plain, pkg/imgpull-
// agnostic carrier for exactly what imgpull.NewManifestHolder needs
// (mediaType, bytes, digest, imageUrl) - see the package doc for why this
// package can't construct a ManifestHolder itself.
type ManifestEntry struct {
	MediaType string
	Bytes     []byte
	Digest    string
	Ref       string
}

// Blob pairs one blob's digest with a reader over its content - the plain,
// pkg/imgpull-agnostic carrier Blobs() yields, mirroring ManifestEntry for
// manifests. Bundling these two values into one struct (rather than
// yielding them separately) is required, not stylistic: Go's range-over-
// func only supports 0, 1, or 2 iteration variables - there is no 3-value
// form regardless of the yield function's shape - so (digest, reader,
// error) as three separate range values was never actually valid Go.
type Blob struct {
	Digest string
	Reader io.Reader
}

// tarEntry records where one regular file lives in the plain (post-gzip-
// decompression, if needed) tar, so later reads can go straight to that
// offset via ReadAt instead of re-scanning the archive.
type tarEntry struct {
	offset int64
	size   int64
}

// format identifies which of the two supported tarball shapes was detected.
type format int

const (
	formatDockerSave format = iota
	formatOciLayout
)

// Reader provides read access to the manifests and blobs in an image
// tarball. Create with Open; call Close when done - this releases the open
// file handle and, if the source tarball was gzip-compressed, removes the
// temp file created to hold the decompressed copy.
//
// Not safe for concurrent use: the offset index and format are written
// once in Open and only read thereafter, so concurrent calls to
// Manifests/Blobs on the SAME already-constructed Reader are fine (each
// read goes through file.ReadAt, which is safe for concurrent use at the
// OS level) - but Close racing against an in-flight read is not supported.
type Reader struct {
	file    *os.File
	tmpFile string // non-empty if file is a temp gzip-decompressed copy Close should remove
	index   map[string]tarEntry
	format  format

	// wantOs/wantArch filter which manifest-list members get walked/yielded
	// (see childDigestsOf) and which docker-save entries get yielded (see
	// parseDockerSave). Empty string means "don't filter on this
	// dimension" - both empty means no filtering at all.
	wantOs   string
	wantArch string

	dockerManifests []dockerSaveImage // populated by parseDockerSave; used only when format == formatDockerSave
	dockerBlobFiles map[string]string // digest -> tar entry name, built once at parse time; used only when format == formatDockerSave
	ociIndex        ociLayoutIndex    // populated by parseOciLayout; used only when format == formatOciLayout
}

// Open opens path for reading, transparently decompressing to a temp file
// first if it's gzip-compressed (.tar.gz/.tgz - detected by magic bytes,
// not file extension), indexes every regular file's offset/size with a
// single sequential pass, and classifies the tarball as docker-save or
// OCI-layout shaped (oci-layout marker takes precedence if both are
// present, matching how modern `docker save` - containerd image store
// backend - writes both formats into one tarball for compatibility).
//
// wantOs/wantArch filter Manifests(): pass "" for either (or both) to
// disable filtering on that dimension. See childDigestsOf (OCI-layout) and
// parseDockerSave (docker-save) for exactly how each format applies the
// filter - the two formats necessarily do this differently, since only
// OCI-layout manifest lists carry per-member platform metadata directly;
// docker-save has to be filtered by each flat entry's own image config.
func Open(path string, wantOs string, wantArch string) (*Reader, error) {
	file, tmpFile, err := openPlainTar(path)
	if err != nil {
		return nil, err
	}
	r := &Reader{file: file, tmpFile: tmpFile, index: map[string]tarEntry{}, wantOs: wantOs, wantArch: wantArch}

	if err := r.buildIndex(path); err != nil {
		r.Close()
		return nil, err
	}

	switch {
	case r.hasEntry("oci-layout"):
		r.format = formatOciLayout
		if err := r.parseOciLayout(); err != nil {
			r.Close()
			return nil, err
		}
	case r.hasEntry("manifest.json"):
		r.format = formatDockerSave
		if err := r.parseDockerSave(); err != nil {
			r.Close()
			return nil, err
		}
	default:
		r.Close()
		return nil, fmt.Errorf("%q is not a recognized image tarball: no oci-layout marker or manifest.json found at the tar root", path)
	}

	return r, nil
}

// Close releases the open file handle and removes the temp file created if
// the source tarball was gzip-compressed.
func (r *Reader) Close() error {
	var err error
	if r.file != nil {
		err = r.file.Close()
	}
	if r.tmpFile != "" {
		if rmErr := os.Remove(r.tmpFile); rmErr != nil && err == nil {
			err = rmErr
		}
	}
	return err
}

// Manifests iterates every manifest in the tarball - image manifests and
// image-list manifests alike, flattened into a single depth-first stream
// with parents yielded before their children (docker-save never has
// children to yield; OCI-layout sometimes does - see ocilayout.go).
//
// If wantOs/wantArch were set in Open, filtering is applied: for OCI-
// layout, a manifest-list's own top-level entry is still always yielded
// (it represents a whole logical image, not a specific platform, so there
// is nothing to filter it against), but its members are only walked/
// yielded if their own platform matches - see childDigestsOf. For docker-
// save, which has no list structure to filter within, an entire flat entry
// is included or excluded based on its own image config's os/architecture
// - see parseDockerSave. Excluded entries are simply absent from the
// stream, not reported as errors.
func (r *Reader) Manifests() iter.Seq2[ManifestEntry, error] {
	return func(yield func(ManifestEntry, error) bool) {
		switch r.format {
		case formatDockerSave:
			r.dockerSaveManifests(yield)
		case formatOciLayout:
			r.ociLayoutManifests(yield)
		}
	}
}

// Blobs iterates every blob in wanted, yielding (Blob, err) for each. Blob
// bundles the digest and a reader over that blob's content together (see
// the Blob doc comment for why). wanted is normally the result of calling
// Layers() on the ManifestHolder built from a ManifestEntry this Reader
// itself yielded - this package has no ManifestHolder of its own to
// derive it from, hence it being a plain parameter rather than something
// Blobs figures out itself (see the package doc).
func (r *Reader) Blobs(wanted []types.Layer) iter.Seq2[Blob, error] {
	return func(yield func(Blob, error) bool) {
		switch r.format {
		case formatDockerSave:
			r.dockerSaveBlobs(wanted, yield)
		case formatOciLayout:
			r.ociLayoutBlobs(wanted, yield)
		}
	}
}

// openPlainTar opens path, transparently gunzipping to a temp file first if
// the content is gzip-compressed (sniffed by magic bytes). Returns the
// *os.File to use and, if a temp file was created, its path so the caller
// can remove it on Close.
func openPlainTar(path string) (*os.File, string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, "", err
	}

	magic := make([]byte, 2)
	n, _ := io.ReadFull(f, magic)
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		f.Close()
		return nil, "", err
	}
	if n != 2 || magic[0] != 0x1f || magic[1] != 0x8b {
		return f, "", nil // already a plain tar
	}

	gz, gzErr := gzip.NewReader(f)
	if gzErr != nil {
		f.Close()
		return nil, "", fmt.Errorf("error opening %q as gzip: %w", path, gzErr)
	}
	defer gz.Close()
	defer f.Close()

	out, err := os.CreateTemp("", "imgpull-tarball-*.tar")
	if err != nil {
		return nil, "", err
	}
	if _, err := io.Copy(out, gz); err != nil {
		out.Close()
		os.Remove(out.Name())
		return nil, "", err
	}
	if _, err := out.Seek(0, io.SeekStart); err != nil {
		out.Close()
		os.Remove(out.Name())
		return nil, "", err
	}
	return out, out.Name(), nil
}

// buildIndex does a single sequential pass over r.file, recording every
// regular file entry's offset and size so readAt/sectionReaderFor can
// later access any of them directly without re-scanning.
func (r *Reader) buildIndex(path string) error {
	if _, err := r.file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	tr := tar.NewReader(r.file)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("error reading tar %q: %w", path, err)
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		offset, err := r.file.Seek(0, io.SeekCurrent)
		if err != nil {
			return err
		}
		r.index[hdr.Name] = tarEntry{offset: offset, size: hdr.Size}
	}
	return nil
}

func (r *Reader) hasEntry(name string) bool {
	_, ok := r.index[name]
	return ok
}

// readAt returns the bytes for a previously-indexed tar entry name.
func (r *Reader) readAt(name string) ([]byte, error) {
	e, ok := r.index[name]
	if !ok {
		return nil, fmt.Errorf("%q not found in tarball", name)
	}
	buf := make([]byte, e.size)
	if _, err := r.file.ReadAt(buf, e.offset); err != nil && err != io.EOF {
		return nil, err
	}
	return buf, nil
}

// sectionReaderFor returns a fresh io.Reader over a previously-indexed tar
// entry's bytes, for streaming without buffering the whole thing at once -
// used for blob content. readAt is used instead for the small
// manifest/config/index JSON files, where buffering the whole thing is
// fine and simpler.
func (r *Reader) sectionReaderFor(name string) (io.Reader, error) {
	e, ok := r.index[name]
	if !ok {
		return nil, fmt.Errorf("%q not found in tarball", name)
	}
	return io.NewSectionReader(r.file, e.offset, e.size), nil
}

// unmarshalIndexed reads a previously-indexed entry and unmarshals it as
// JSON into v.
func (r *Reader) unmarshalIndexed(name string, v any) error {
	b, err := r.readAt(name)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, v); err != nil {
		return fmt.Errorf("error parsing %q: %w", name, err)
	}
	return nil
}

// platformMatches reports whether (os, arch) satisfies the (wantOs,
// wantArch) filter - case-insensitive (matching ManifestHolder.IsLatest's
// existing tag-comparison convention elsewhere in this codebase). An empty
// want value always matches on that dimension - passing "" for both
// wantOs and wantArch disables filtering entirely. Shared by both formats:
// dockersave.go compares against an image config's own os/architecture
// fields; ocilayout.go compares against a manifest-list member
// descriptor's platform.
func platformMatches(wantOs string, wantArch string, os string, arch string) bool {
	if wantOs != "" && !strings.EqualFold(wantOs, os) {
		return false
	}
	if wantArch != "" && !strings.EqualFold(wantArch, arch) {
		return false
	}
	return true
}
