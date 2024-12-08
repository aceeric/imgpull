package imgpull

import (
	"encoding/json"
	"fmt"
	"imgpull/pkg/imgpull/v1oci"
	"imgpull/pkg/imgpull/v2docker"
	"strings"
)

// ManifestType identifies the type of manifest the package can operate on.
type ManifestType int

const (
	Undefined ManifestType = iota
	V2dockerManifestList
	V2dockerManifest
	V1ociIndex
	V1ociManifest
)

// manifestTypeToString has string representations for all supported
// 'ManifestType's.
var manifestTypeToString = map[ManifestType]string{
	Undefined:            "Undefined",
	V2dockerManifestList: "V2dockerManifestList",
	V2dockerManifest:     "V2dockerManifest",
	V1ociIndex:           "V1ociIndex",
	V1ociManifest:        "V1ociManifest",
}

// ManifestHolder holds one of: v1 oci manifest list, v1 oci manifest, docker v2
// manifest list, or docker v2 manifest.
type ManifestHolder struct {
	Type                 ManifestType          `json:"type"`
	V1ociIndex           v1oci.Index           `json:"v1.oci.index"`
	V1ociManifest        v1oci.Manifest        `json:"v1.oci.manifest"`
	V2dockerManifestList v2docker.ManifestList `json:"v2.docker.manifestList"`
	V2dockerManifest     v2docker.Manifest     `json:"v2.docker.Manifest"`
}

// NewManifestHolder initializes and returns a ManifestHolder struct for the passed
// manifest bytes. The manifest bytes will be deserialized into one of the four manifest
// variables based on the 'mediaType' arg.
func NewManifestHolder(mediaType string, bytes []byte) (ManifestHolder, error) {
	mt := ToManifestType(mediaType)
	if mt == Undefined {
		return ManifestHolder{}, fmt.Errorf("unknown manifest type: %s", mediaType)
	}
	mh := ManifestHolder{
		Type: mt,
	}
	err := mh.UnMarshalManifest(mt, bytes)
	if err != nil {
		return ManifestHolder{}, err
	}
	return mh, nil
}

// ToManifestType returns the 'ManifestType' corrresponding to the passed
// 'mediaType'.
func ToManifestType(mediaType string) ManifestType {
	switch mediaType {
	case V2dockerManifestListMt:
		return V2dockerManifestList
	case V2dockerManifestMt:
		return V2dockerManifest
	case V1ociIndexMt:
		return V1ociIndex
	case V1ociManifestMt:
		return V1ociManifest
	default:
		return Undefined
	}
}

// UnMarshalManifest unmarshals the passed bytes and stores the resulting typed manifest struct
// in the corresponding manifest variable in the receiver struct.
func (mh *ManifestHolder) UnMarshalManifest(mt ManifestType, bytes []byte) error {
	var err error
	switch mt {
	case V2dockerManifestList:
		err = json.Unmarshal(bytes, &mh.V2dockerManifestList)
	case V2dockerManifest:
		err = json.Unmarshal(bytes, &mh.V2dockerManifest)
	case V1ociIndex:
		err = json.Unmarshal(bytes, &mh.V1ociIndex)
	case V1ociManifest:
		err = json.Unmarshal(bytes, &mh.V1ociManifest)
	default:
		err = fmt.Errorf("unknown manifest type: %d", mt)
	}
	return err
}

// IsManifestList returns true of the manifest held by the ManifestHolder
// receiver is a manifest list (not an image manifest.)
func (mh *ManifestHolder) IsManifestList() bool {
	return mh.Type == V2dockerManifestList || mh.Type == V1ociIndex
}

// Layers returns the count of layers in the manifest held by the ManifestHolder
// receiver.
func (mh *ManifestHolder) Layers() int {
	switch mh.Type {
	case V2dockerManifest:
		return len(mh.V2dockerManifest.Layers)
	case V1ociManifest:
		return len(mh.V1ociManifest.Layers)
	default:
		return 0
	}
}

// Layer gets the passed layer from the manifest embedded in the ManifestHolder receciver.
// An empty layer is returned if the index is out of bounds, or the holder is not holding
// an image manifest. Returning an error as well makes iterating hard so - an empty layer
// means the function has been mis-used.
func (mh *ManifestHolder) Layer(idx int) Layer {
	layer := Layer{}
	switch mh.Type {
	case V2dockerManifest:
		if idx >= 0 && idx < len(mh.V2dockerManifest.Layers) {
			layer.Digest = mh.V2dockerManifest.Layers[idx].Digest
			layer.MediaType = mh.V2dockerManifest.Layers[idx].MediaType
			layer.Size = int(mh.V2dockerManifest.Layers[idx].Size)
		}
	case V1ociManifest:
		if idx >= 0 && idx < len(mh.V1ociManifest.Layers) {
			layer.Digest = mh.V1ociManifest.Layers[idx].Digest
			layer.MediaType = mh.V1ociManifest.Layers[idx].MediaType
			layer.Size = int(mh.V1ociManifest.Layers[idx].Size)
		}
	}
	return layer
}

// ToString marshalls the manifest held by the ManifestHolder receiver. Only the
// embedded manifest is returned - which will be a docker or oci manifest list, or
// a docker or oci image manifest.
func (mh *ManifestHolder) ToString() (string, error) {
	var err error
	var marshalled []byte
	switch mh.Type {
	case V2dockerManifestList:
		marshalled, err = json.MarshalIndent(mh.V2dockerManifestList, "", "   ")
	case V2dockerManifest:
		marshalled, err = json.MarshalIndent(mh.V2dockerManifest, "", "   ")
	case V1ociIndex:
		marshalled, err = json.MarshalIndent(mh.V1ociIndex, "", "   ")
	case V1ociManifest:
		marshalled, err = json.MarshalIndent(mh.V1ociManifest, "", "   ")
	}
	return string(marshalled), err
}

// GetImageConfig gets the 'Config' layer from the ManafestHolder receiver, or
// an error if unable to do so.
func (mh *ManifestHolder) GetImageConfig() (Layer, error) {
	layer := Layer{}
	switch mh.Type {
	case V2dockerManifest:
		layer.Digest = mh.V2dockerManifest.Config.Digest
		layer.MediaType = mh.V2dockerManifest.Config.MediaType
		layer.Size = int(mh.V2dockerManifest.Config.Size)
	case V1ociManifest:
		layer.Digest = mh.V1ociManifest.Config.Digest
		layer.MediaType = mh.V1ociManifest.Config.MediaType
		layer.Size = int(mh.V1ociManifest.Config.Size)
	default:
		return layer, fmt.Errorf("can't get image config from this kind of manifest: %s", manifestTypeToString[mh.Type])
	}
	return layer, nil
}

func (mh *ManifestHolder) GetImageDigestFor(os string, arch string) (string, error) {
	switch mh.Type {
	case V2dockerManifestList:
		for _, mfst := range mh.V2dockerManifestList.Manifests {
			if mfst.Platform.OS == os && mfst.Platform.Architecture == arch {
				return mfst.Digest, nil
			}
		}
	case V1ociIndex:
		for _, mfst := range mh.V1ociIndex.Manifests {
			if mfst.Platform.Os == os && mfst.Platform.Architecture == arch {
				return mfst.Digest, nil
			}
		}
	}
	return "", fmt.Errorf("unable to get manifest SHA for os %s, arch %s", os, arch)
}

// NewDockerTarManifest creates a 'DockerTarManifest' from the passed image ref. It supports
// pull-though by virtue of the 'namespace' arg.
func (mh *ManifestHolder) NewDockerTarManifest(ip ImageRef, namespace string) (DockerTarManifest, error) {
	m := DockerTarManifest{}
	switch mh.Type {
	case V2dockerManifest:
		m.Config = mh.V2dockerManifest.Config.Digest
		m.RepoTags = []string{ip.ImageUrl(namespace)}
		for _, layer := range mh.V2dockerManifest.Layers {
			if ext, err := extensionForLayer(layer.MediaType); err != nil {
				return m, err
			} else {
				m.Layers = append(m.Layers, layer.Digest+ext)
			}
		}
	case V1ociManifest:
		m.Config = mh.V1ociManifest.Config.Digest
		m.RepoTags = []string{ip.ImageUrl(namespace)}
		for _, layer := range mh.V1ociManifest.Layers {
			if ext, err := extensionForLayer(layer.MediaType); err != nil {
				return m, err
			} else {
				m.Layers = append(m.Layers, layer.Digest+ext)
			}
		}
	default:
		return m, fmt.Errorf("can't create docker tar manifest from this kind of manifest: %s", manifestTypeToString[mh.Type])
	}
	for idx, layer := range m.Layers {
		m.Layers[idx] = strings.Replace(layer, "sha256:", "", -1)
	}
	return m, nil
}

// extensionForLayer returns '.tar', '.tar.gz', or '.tar.zstd' based on the
// passed media type.
func extensionForLayer(mediaType string) (string, error) {
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
