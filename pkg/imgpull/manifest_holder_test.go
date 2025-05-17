package imgpull

import "testing"

func TestIsLatest(t *testing.T) {
	for _, urlTest := range []struct {
		url      string
		isLatest bool
	}{
		{"quay.io/foo:v1", false},
		{"quay.io/foo:latest", true},
		{"quay.io/foo/bar:v1", false},
		{"quay.io/foo/bar:latest", true},
		{"localhost:8080/quay.io/foo/bar:v1", false},
		{"localhost:8080/quay.io/foo/bar:latest", true},
	} {
		mh := ManifestHolder{
			ImageUrl: urlTest.url,
		}
		if isLatest, err := mh.IsLatest(); err != nil && isLatest != urlTest.isLatest {
			t.FailNow()
		}
	}
}
