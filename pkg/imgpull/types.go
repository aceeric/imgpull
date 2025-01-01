package imgpull

// ManifestDescriptor has the information returned from a v2 manifests
// HEAD request to an OCI distribution server. A HEAD request returns a subset
// if manifest info.
type ManifestDescriptor struct {
	MediaType string `json:"mediaType,omitempty"`
	Digest    string `json:"digest,omitempty"`
	Size      int    `json:"size"`
}

//// Layer has the parts of the 'Descriptor' struct that minimally describe a
//// layer. Since the Descriptor is a different type for Docker vs OCI with overlap, the other
//// option was to embed the original struct here and then have getters based on
//// type but that seemed overly complex based on the simple need to just use this
//// to carry a layer digest. This is the same information as the ManifestDescriptor
//// struct but since it really is derived from a layer, it is represented as
//// a separate struct.
//type Layer struct {
//	MediaType string `json:"mediaType"`
//	Digest    string `json:"digest"`
//	Size      int    `json:"size"`
//}

//// newLayer returns a new 'Layer' struct from the passed args
//func newLayer(mediaType string, digest string, size int64) Layer {
//	return Layer{
//		MediaType: mediaType,
//		Digest:    digest,
//		Size:      int(size),
//	}
//}

const SHA256PREFIX = "sha256:"

// bearerAuth has the two parts of a bearer auth header that we need, in
// order to request a bearer token from an OCI distribution server.
type bearerAuth struct {
	realm   string
	service string
}

// bearerToken holds the bearer token value.
type bearerToken struct {
	token string
}

// basicAuth holds the encoded username and password.
type basicAuth struct {
	encoded string
}
