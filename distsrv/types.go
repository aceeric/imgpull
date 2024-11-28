package distsrv

import (
	"imgpull/distsrv/v1oci"
	"imgpull/distsrv/v2docker"
)

const (
	V2dockerManifestListMt = "application/vnd.docker.distribution.manifest.list.v2+json"
	V2dockerManifestMt     = "application/vnd.docker.distribution.manifest.v2+json"
	V1ociIndexMt           = "application/vnd.oci.image.index.v1+json"
	V1ociManifestMt        = "application/vnd.oci.image.manifest.v1+json"
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

type Layer struct {
	MediaType string `json:"mediaType"`
	Digest    string `json:"digest"`
	Size      int    `json:"size"`
}

func IsImageManifest(mediaType string) bool {
	return mediaType != V2dockerManifestListMt &&
		mediaType != V1ociIndexMt
}
