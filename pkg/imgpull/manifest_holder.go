package imgpull

import (
	"encoding/json"
	"fmt"

	"github.com/aceeric/imgpull/internal/imgref"
	"github.com/aceeric/imgpull/internal/tar"
	"github.com/aceeric/imgpull/internal/util"
	"github.com/aceeric/imgpull/pkg/imgpull/types"
	"github.com/aceeric/imgpull/pkg/imgpull/v1oci"
	"github.com/aceeric/imgpull/pkg/imgpull/v2docker"
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
// manifest list, or docker v2 manifest. The original data from the upstream
// is also in the struct.
type ManifestHolder struct {
	Type                 ManifestType          `json:"type"`
	Digest               string                `json:"digest"`
	ImageUrl             string                `json:"imageUrl"`
	Data                 []byte                `json:"data,omitempty"`
	V1ociIndex           v1oci.Index           `json:"v1.oci.index"`
	V1ociManifest        v1oci.Manifest        `json:"v1.oci.manifest"`
	V2dockerManifestList v2docker.ManifestList `json:"v2.docker.manifestList"`
	V2dockerManifest     v2docker.Manifest     `json:"v2.docker.Manifest"`
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

// newManifestHolder initializes and returns a ManifestHolder struct for the passed
// manifest bytes. The manifest bytes will be deserialized into one of the four manifest
// variables based on the 'mediaType' arg.
func newManifestHolder(mediaType types.MediaType, bytes []byte, digest string, imageUrl string) (ManifestHolder, error) {
	mt := toManifestType(mediaType)
	if mt == Undefined {
		return ManifestHolder{}, fmt.Errorf("unknown manifest type %q", mediaType)
	}
	mh := ManifestHolder{
		Type:     mt,
		Digest:   digest,
		ImageUrl: imageUrl,
		Data:     bytes,
	}
	err := mh.unMarshalManifest(mt, bytes)
	if err != nil {
		return ManifestHolder{}, err
	}
	return mh, nil
}

// toManifestType returns the 'ManifestType' corrresponding to the passed
// 'mediaType'. If the media type does not match one of the supported types then
// the function returns 'Undefined'.
func toManifestType(mediaType types.MediaType) ManifestType {
	switch mediaType {
	case types.V2dockerManifestListMt:
		return V2dockerManifestList
	case types.V2dockerManifestMt:
		return V2dockerManifest
	case types.V1ociIndexMt:
		return V1ociIndex
	case types.V1ociManifestMt:
		return V1ociManifest
	default:
		return Undefined
	}
}

// Bytes marshals the JSON manifest of the holder into a byte array and returns it
func (mh *ManifestHolder) Bytes() ([]byte, error) {
	var err error
	var marshalled []byte
	switch mh.Type {
	case V2dockerManifestList:
		marshalled, err = json.Marshal(mh.V2dockerManifestList)
	case V2dockerManifest:
		marshalled, err = json.Marshal(mh.V2dockerManifest)
	case V1ociIndex:
		marshalled, err = json.Marshal(mh.V1ociIndex)
	case V1ociManifest:
		marshalled, err = json.Marshal(mh.V1ociManifest)
	}
	return marshalled, err
}

// MediaType returns the string media type of the receiver
func (mh *ManifestHolder) MediaType() string {
	switch mh.Type {
	case V2dockerManifestList:
		return string(types.V2dockerManifestListMt)
	case V2dockerManifest:
		return string(types.V2dockerManifestMt)
	case V1ociIndex:
		return string(types.V1ociIndexMt)
	case V1ociManifest:
		return string(types.V1ociManifestMt)
	default:
		return ""
	}
}

// unMarshalManifest unmarshals the passed bytes and stores the resulting typed manifest struct
// in the corresponding manifest variable in the receiver struct.
func (mh *ManifestHolder) unMarshalManifest(mt ManifestType, bytes []byte) error {
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

// Layers returns an array of 'Layer' for the manifest contained by the ManifestHolder
// receiver. The Config is also returned since that is obtained using the v2/blobs
// endpoint just like the image Layers.
func (mh *ManifestHolder) Layers() []types.Layer {
	layers := make([]types.Layer, 0)
	switch mh.Type {
	case V2dockerManifest:
		for _, l := range mh.V2dockerManifest.Layers {
			nl := types.Layer{
				Digest:    l.Digest,
				MediaType: types.MediaType(l.MediaType),
				Size:      int(l.Size),
			}
			layers = append(layers, nl)
		}
		nl := types.Layer{
			Digest:    mh.V2dockerManifest.Config.Digest,
			MediaType: types.MediaType(mh.V2dockerManifest.Config.MediaType),
			Size:      int(mh.V2dockerManifest.Config.Size),
		}
		layers = append(layers, nl)
	case V1ociManifest:
		for _, l := range mh.V1ociManifest.Layers {
			nl := types.Layer{
				Digest:    l.Digest,
				MediaType: types.MediaType(l.MediaType),
				Size:      int(l.Size),
			}
			layers = append(layers, nl)
		}
		nl := types.Layer{
			Digest:    mh.V1ociManifest.Config.Digest,
			MediaType: types.MediaType(mh.V1ociManifest.Config.MediaType),
			Size:      int(mh.V1ociManifest.Config.Size),
		}
		layers = append(layers, nl)
	}
	return layers
}

// getImageDigestFor looks in the manifest list in the receiver for a manifest in the list
// matching the passed OS and architecture and if found returns it. Otherwise an error is
// returned.
func (mh *ManifestHolder) getImageDigestFor(os string, arch string) (string, error) {
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

// newImageTarball creates an 'imageTarball' struct from the passed receiver and args.
// The 'sourceDir' arg specifies where the blob files can be found. The function doesn't
// create the tarball but the struct that is returned has everything needed for the
// caller to create the tarball.
func (mh *ManifestHolder) newImageTarball(iref imgref.ImageRef, sourceDir string) (tar.ImageTarball, error) {
	itb := tar.ImageTarball{
		SourceDir: sourceDir,
	}
	switch mh.Type {
	case V2dockerManifest:
		itb.ConfigDigest = util.DigestFrom(mh.V2dockerManifest.Config.Digest)
		itb.ImageUrl = iref.UrlWithNs()
		for _, layer := range mh.V2dockerManifest.Layers {
			itb.Layers = append(itb.Layers, types.NewLayer(types.MediaType(layer.MediaType), layer.Digest, layer.Size))
		}
	case V1ociManifest:
		itb.ConfigDigest = util.DigestFrom(mh.V1ociManifest.Config.Digest)
		itb.ImageUrl = iref.UrlWithNs()
		for _, layer := range mh.V1ociManifest.Layers {
			itb.Layers = append(itb.Layers, types.NewLayer(types.MediaType(layer.MediaType), layer.Digest, layer.Size))
		}
	default:
		return itb, fmt.Errorf("can't create docker tar manifest from %q kind of manifest", manifestTypeToString[mh.Type])
	}
	return itb, nil
}
