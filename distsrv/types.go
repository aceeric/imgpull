package distsrv

import (
	"fmt"
	"imgpull/distsrv/v1oci"
	"imgpull/distsrv/v2docker"
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

type ManifestType int

const (
	Undefined ManifestType = iota
	V2dockerManifestList
	V2dockerManifest
	V1ociIndex
	//V1ociDescriptor
	V1ociManifest
)

var ManifestTypeToString = map[ManifestType]string{
	Undefined:            "Undefined",
	V2dockerManifestList: "V2dockerManifestList",
	V2dockerManifest:     "V2dockerManifest",
	V1ociIndex:           "V1ociIndex",
	V1ociManifest:        "V1ociManifest",
}

var allManifestTypes []string = []string{
	V2dockerManifestListMt,
	V2dockerManifestMt,
	V1ociIndexMt,
	V1ociManifestMt,
}

type BearerAuth struct {
	Realm   string
	Service string
}

type BearerToken struct {
	Token string `json:"token"`
}

type ManifestHolder struct {
	Type       ManifestType `json:"type"`
	CurBlob    int          `json:"curBlob"`
	ImageUrl   string       `json:"imageUrl"`
	MediaType  string       `json:"mediaType"`
	Digest     string       `json:"digest"`
	Size       int          `json:"size"`
	V1ociIndex v1oci.Index  `json:"v1.oci.index"`
	//V1ociDescriptor      v1oci.Descriptor      `json:"v1.oci.descriptor"`
	V1ociManifest        v1oci.Manifest        `json:"v1.oci.manifest"`
	V2dockerManifestList v2docker.ManifestList `json:"v2.docker.manifestList"`
	V2dockerManifest     v2docker.Manifest     `json:"v2.docker.Manifest"`
}

// Parent is a digest
type DockerTarManifest struct {
	Config       string
	RepoTags     []string
	Layers       []string
	LayerSources map[string]v2docker.Descriptor `json:",omitempty"`
}

type Layer struct {
	MediaType string `json:"mediaType"`
	Digest    string `json:"digest"`
	Size      int    `json:"size"`
}

func IsImageManifest(mediaType string) bool {
	return mediaType != V2dockerManifestListMt &&
		mediaType != V1ociIndexMt
}

func ExtensionForLayer(mediaType string) (string, error) {
	switch mediaType {
	case V1ociLayerMt, V2dockerLayerMt:
		return ".tar", nil
	case "", V2dockerLayerGzipMt, V1ociLayerGzipMt:
		return ".tar.gz", nil
	case V2dockerLayerZstdMt, V1ociLayerZstdMt:
		return ".tar.zstd", nil
	}
	return "", fmt.Errorf("unsupported layer media type: %s", mediaType)
}
