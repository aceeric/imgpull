package imgpull

import (
	"encoding/json"
	"fmt"
	"imgpull/pkg/imgpull/v1oci"
	"imgpull/pkg/imgpull/v2docker"
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

// ManifestPullType indicates whether to pull an image manifest or an
// image list manifest.
type ManifestPullType int

const (
	// An image list manifest
	ImageList ManifestPullType = iota
	// An image manifest
	Image
)

// ManifestPullTypeFrom translates a literal string into a ManifestPullType
var ManifestPullTypeFrom = map[string]ManifestPullType{
	"image": Image,
	"list":  ImageList,
}

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
	Digest               string                `json:"digest"`
	Url                  string                `json:"url"`
	ImageUrl             string                `json:"imageUrl"`
	V1ociIndex           v1oci.Index           `json:"v1.oci.index"`
	V1ociManifest        v1oci.Manifest        `json:"v1.oci.manifest"`
	V2dockerManifestList v2docker.ManifestList `json:"v2.docker.manifestList"`
	V2dockerManifest     v2docker.Manifest     `json:"v2.docker.Manifest"`
}

// NewManifestHolder initializes and returns a ManifestHolder struct for the passed
// manifest bytes. The manifest bytes will be deserialized into one of the four manifest
// variables based on the 'mediaType' arg.
func NewManifestHolder(mediaType string, bytes []byte, digest string, imageUrl string) (ManifestHolder, error) {
	mt := ToManifestType(mediaType)
	if mt == Undefined {
		return ManifestHolder{}, fmt.Errorf("unknown manifest type %q", mediaType)
	}
	mh := ManifestHolder{
		Type:     mt,
		Digest:   digest,
		ImageUrl: imageUrl,
	}
	err := mh.UnMarshalManifest(mt, bytes)
	if err != nil {
		return ManifestHolder{}, err
	}
	return mh, nil
}

// ToManifestType returns the 'ManifestType' corrresponding to the passed
// 'mediaType'. If the media type does not match one of the supported types then
// the function returns 'Undefined'.
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

// Layers returns an array of 'Layer' for the manifest container by the ManifestHolder
// receiver.
func (mh *ManifestHolder) Layers() []Layer {
	layers := make([]Layer, 0)
	switch mh.Type {
	case V2dockerManifest:
		for _, l := range mh.V2dockerManifest.Layers {
			nl := Layer{
				Digest:    l.Digest,
				MediaType: l.MediaType,
				Size:      int(l.Size),
			}
			layers = append(layers, nl)
		}
	case V1ociManifest:
		for _, l := range mh.V1ociManifest.Layers {
			nl := Layer{
				Digest:    l.Digest,
				MediaType: l.MediaType,
				Size:      int(l.Size),
			}
			layers = append(layers, nl)
		}
	}
	return layers
}

// ToString renders the manifest held by the receiver into JSON. Only the
// embedded manifest is returned - which will be a docker or oci manifest
// list, or a docker or oci image manifest.
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

// GetImageConfig gets the 'Config' layer from the receiver, or an error if
// unable to do so.
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
		return layer, fmt.Errorf("can't get image config from %q kind of manifest", manifestTypeToString[mh.Type])
	}
	return layer, nil
}

// GetImageDigestFor looks in the manifest list in the receiver for a manifest in the list
// matching the passed OS and architecture and if found returns it. Otherwise an error is
// returned.
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
	return "", fmt.Errorf("unable to get manifest SHA for os %q, arch %q", os, arch)
}

// NewImageTarball creates an 'imageTarball' struct from the passed receiver and args.
// It supports pull-though by virtue of the 'namespace' arg. The 'sourceDir' arg
// specifies where the blob
// files can be found.
func (mh *ManifestHolder) NewImageTarball(iref imageRef, namespace string, sourceDir string) (imageTarball, error) {
	dtm := imageTarball{
		sourceDir: sourceDir,
	}
	switch mh.Type {
	case V2dockerManifest:
		dtm.configDigest = digestFrom(mh.V2dockerManifest.Config.Digest)
		dtm.imageUrl = iref.imageUrlWithNs(namespace)
		for _, layer := range mh.V2dockerManifest.Layers {
			dtm.layers = append(dtm.layers, newLayer(layer.MediaType, layer.Digest, layer.Size))
		}
	case V1ociManifest:
		dtm.configDigest = digestFrom(mh.V1ociManifest.Config.Digest)
		dtm.imageUrl = iref.imageUrlWithNs(namespace)
		for _, layer := range mh.V1ociManifest.Layers {
			dtm.layers = append(dtm.layers, newLayer(layer.MediaType, layer.Digest, layer.Size))
		}
	default:
		return dtm, fmt.Errorf("can't create docker tar manifest from %q kind of manifest", manifestTypeToString[mh.Type])
	}
	return dtm, nil
}

// saveManifest extracts the manifest from the recevier and save it to a file
// with the passed name in the passed path.
func (mh *ManifestHolder) saveManifest(toPath string, name string) error {
	json, err := mh.ToString()
	if err != nil {
		return err
	}
	return saveFile([]byte(json), toPath, name)
}
