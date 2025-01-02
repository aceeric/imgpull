package types

type MediaType string

// media types
const (
	V2dockerManifestListMt MediaType = "application/vnd.docker.distribution.manifest.list.v2+json"
	V2dockerManifestMt     MediaType = "application/vnd.docker.distribution.manifest.v2+json"
	V1ociIndexMt           MediaType = "application/vnd.oci.image.index.v1+json"
	V1ociManifestMt        MediaType = "application/vnd.oci.image.manifest.v1+json"
	V2dockerLayerMt        MediaType = "application/vnd.docker.image.rootfs.diff.tar"
	V2dockerLayerGzipMt    MediaType = "application/vnd.docker.image.rootfs.diff.tar.gzip"
	V2dockerLayerZstdMt    MediaType = "application/vnd.docker.image.rootfs.diff.tar.zstd"
	V1ociLayerMt           MediaType = "application/vnd.oci.image.layer.v1.tar"
	V1ociLayerGzipMt       MediaType = "application/vnd.oci.image.layer.v1.tar+gzip"
	V1ociLayerZstdMt       MediaType = "application/vnd.oci.image.layer.v1.tar+zstd"
)

// ManifestDescriptor has the information returned from a v2 manifests
// HEAD request to an OCI distribution server. A HEAD request returns a subset
// if manifest info.
type ManifestDescriptor struct {
	MediaType MediaType `json:"mediaType,omitempty"`
	Digest    string    `json:"digest,omitempty"`
	Size      int       `json:"size"`
}

// Layer has the parts of the 'Descriptor' struct that minimally describe a
// layer. Since the Descriptor is a different type for Docker vs OCI with overlap, the other
// option was to embed the original struct here and then have getters based on
// type but that seemed overly complex based on the simple need to just use this
// to carry a layer digest. This is the same information as the ManifestDescriptor
// struct but since it really is derived from a layer, it is represented as
// a separate struct.
type Layer struct {
	MediaType MediaType `json:"mediaType"`
	Digest    string    `json:"digest"`
	Size      int       `json:"size"`
}

// NewLayer returns a new 'Layer' struct from the passed args
func NewLayer(mediaType MediaType, digest string, size int64) Layer {
	return Layer{
		MediaType: mediaType,
		Digest:    digest,
		Size:      int(size),
	}
}

// bearerAuth has the two parts of a bearer auth header that we need, in
// order to request a bearer token from an OCI distribution server.
type BearerAuth struct {
	Realm   string
	Service string
}

// bearerToken holds the bearer token value.
type BearerToken struct {
	Token string
}

// basicAuth holds the encoded username and password.
type BasicAuth struct {
	Encoded string
}
