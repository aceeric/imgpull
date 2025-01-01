package types

// media types
const (
	V2dockerManifestListMt = "application/vnd.docker.distribution.manifest.list.v2+json"
	V2dockerManifestMt     = "application/vnd.docker.distribution.manifest.v2+json"
	V1ociIndexMt           = "application/vnd.oci.image.index.v1+json"
	V1ociManifestMt        = "application/vnd.oci.image.manifest.v1+json"
	V2dockerLayerMt        = "application/vnd.docker.image.rootfs.diff.tar"
	V2dockerLayerGzipMt    = "application/vnd.docker.image.rootfs.diff.tar.gzip"
	V2dockerLayerZstdMt    = "application/vnd.docker.image.rootfs.diff.tar.zstd"
	V1ociLayerMt           = "application/vnd.oci.image.layer.v1.tar"
	V1ociLayerGzipMt       = "application/vnd.oci.image.layer.v1.tar+gzip"
	V1ociLayerZstdMt       = "application/vnd.oci.image.layer.v1.tar+zstd"
)

// Layer has the parts of the 'Descriptor' struct that minimally describe a
// layer. Since the Descriptor is a different type for Docker vs OCI with overlap, the other
// option was to embed the original struct here and then have getters based on
// type but that seemed overly complex based on the simple need to just use this
// to carry a layer digest. This is the same information as the ManifestDescriptor
// struct but since it really is derived from a layer, it is represented as
// a separate struct.
type Layer struct {
	MediaType string `json:"mediaType"`
	Digest    string `json:"digest"`
	Size      int    `json:"size"`
}

// NewLayer returns a new 'Layer' struct from the passed args
func NewLayer(mediaType string, digest string, size int64) Layer {
	return Layer{
		MediaType: mediaType,
		Digest:    digest,
		Size:      int(size),
	}
}
