package imgpull

import (
	"imgpull/pkg/imgpull/v2docker"
)

const (
	V2dockerManifestListMt = "application/vnd.docker.distribution.manifest.list.v2+json"
	V2dockerManifestMt     = "application/vnd.docker.distribution.manifest.v2+json"
	V2dockerImageConfigMt  = "application/vnd.docker.container.image.v1+json"
	V2dockerLayerMt        = "application/vnd.docker.image.rootfs.diff.tar"
	V2dockerLayerGzipMt    = "application/vnd.docker.image.rootfs.diff.tar.gzip"
	V2dockerLayerZstdMt    = "application/vnd.docker.image.rootfs.diff.tar.zstd"
	V1ociIndexMt           = "application/vnd.oci.image.index.v1+json"
	V1ociManifestMt        = "application/vnd.oci.image.manifest.v1+json"
	V1ociImageConfigMt     = "application/vnd.oci.image.config.v1+json"
	V1ociLayerMt           = "application/vnd.oci.image.layer.v1.tar"
	V1ociLayerGzipMt       = "application/vnd.oci.image.layer.v1.tar+gzip"
	V1ociLayerZstdMt       = "application/vnd.oci.image.layer.v1.tar+zstd"
)

// BearerAuth has the two parts of a bearer auth header that we need to
// request a bearer token from an OCI distribution server.
type BearerAuth struct {
	Realm   string
	Service string
}

type BearerToken struct {
	Token string `json:"token"`
}

type BasicAuth struct {
	Encoded string `json:"encoded"`
}

// ManifestDescriptor has the information returned from a v2 manifests
// HEAD request to an OCI distribution server.
type ManifestDescriptor struct {
	MediaType string `json:"mediaType,omitempty"`
	Digest    string `json:"digest,omitempty"`
	Size      int    `json:"size"`
}

// DockerTarManifest is the structure of 'manifest.json' that you would find
// in a tarball produced by 'docker save'.
type DockerTarManifest struct {
	Config       string                         `json:"config"`
	RepoTags     []string                       `json:"repoTags"`
	Layers       []string                       `json:"layers"`
	LayerSources map[string]v2docker.Descriptor `json:",omitempty"`
}

// Layer has the parts of the 'Descriptor' struct that minimally describe a
// layer. Since the Descriptor is a different type for Docker vs OCI, the other
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
