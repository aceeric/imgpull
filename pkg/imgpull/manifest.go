package imgpull

import (
	"encoding/json"
	"fmt"
	"strings"
)

func NewManifestHolder(contentType string, bytes []byte) (ManifestHolder, error) {
	mt := ToManifestType(contentType)
	if mt == Undefined {
		return ManifestHolder{}, fmt.Errorf("unknown manifest type: %s", contentType)
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

func ToManifestType(contentType string) ManifestType {
	switch contentType {
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

func (mh *ManifestHolder) IsManifestList() bool {
	return mh.Type == V2dockerManifestList || mh.Type == V1ociIndex
}

func (mh *ManifestHolder) NextLayer() (Layer, error) {
	layer := Layer{}
	switch mh.Type {
	case V2dockerManifest:
		if mh.CurBlob < len(mh.V2dockerManifest.Layers) {
			layer.Digest = mh.V2dockerManifest.Layers[mh.CurBlob].Digest
			layer.MediaType = mh.V2dockerManifest.Layers[mh.CurBlob].MediaType
			layer.Size = int(mh.V2dockerManifest.Layers[mh.CurBlob].Size)
			mh.CurBlob++
		}
	case V1ociManifest:
		if mh.CurBlob < len(mh.V1ociManifest.Layers) {
			layer.Digest = mh.V1ociManifest.Layers[mh.CurBlob].Digest
			layer.MediaType = mh.V1ociManifest.Layers[mh.CurBlob].MediaType
			layer.Size = int(mh.V1ociManifest.Layers[mh.CurBlob].Size)
			mh.CurBlob++
		}
	default:
		return layer, fmt.Errorf("can't request layer from this kind of manifest: %s", ManifestTypeToString[mh.Type])
	}
	return layer, nil
}

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

// GetImageConfig gets the 'Config' layer from the instance manifest, or an error
// if unable to do so.
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
		return layer, fmt.Errorf("can't get image config from this kind of manifest: %s", ManifestTypeToString[mh.Type])
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

func (mh *ManifestHolder) NewDockerTarManifest(ip ImagePull) (DockerTarManifest, error) {
	m := DockerTarManifest{}
	switch mh.Type {
	case V2dockerManifest:
		m.Config = mh.V2dockerManifest.Config.Digest
		m.RepoTags = []string{ip.ImageUrl()}
		for _, layer := range mh.V2dockerManifest.Layers {
			if ext, err := ExtensionForLayer(layer.MediaType); err != nil {
				return m, err
			} else {
				m.Layers = append(m.Layers, layer.Digest+ext)
			}
		}
	case V1ociManifest:
		m.Config = mh.V1ociManifest.Config.Digest
		m.RepoTags = []string{ip.ImageUrl()}
		for _, layer := range mh.V1ociManifest.Layers {
			if ext, err := ExtensionForLayer(layer.MediaType); err != nil {
				return m, err
			} else {
				m.Layers = append(m.Layers, layer.Digest+ext)
			}
		}
	default:
		return m, fmt.Errorf("can't create docker tar manifest from this kind of manifest: %s", ManifestTypeToString[mh.Type])
	}
	for idx, layer := range m.Layers {
		m.Layers[idx] = strings.Replace(layer, "sha256:", "", -1)
	}
	return m, nil
}
