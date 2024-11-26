package distsrv

import (
	"encoding/json"
	"fmt"
)

func NewManifestHolder(contentType string, bytes []byte) (ManifestHolder, error) {
	mt := ToManifestType(contentType)
	if mt == Unknown {
		return ManifestHolder{}, fmt.Errorf("unknown manifest type: %s", contentType)
	}
	mh := ManifestHolder{}
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
		return V1ociDescriptor
	default:
		return Unknown
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
	case V1ociDescriptor:
		err = json.Unmarshal(bytes, &mh.V1ociDescriptor)
	default:
		err = fmt.Errorf("unknown manifest type: %d", mt)
	}
	return err
}
