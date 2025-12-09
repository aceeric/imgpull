package imgref

import (
	"reflect"
	"testing"
)

type testCase struct {
	num       int
	input     string
	scheme    string
	namespace string
	shouldErr bool
	expected  ImageRef
}

// the digest matcher in the url parser needs a 64-position digest
const sha = "1234567890123456789012345678901234567890123456789012345678901234"

var testCases = []testCase{
	{1, "docker.io/foo", "https", "", false, ImageRef{registry: "docker.io", pullType: byTag, server: "index.docker.io", repository: "foo", ref: "latest", scheme: "https", namespace: "", nsInPath: false, library: true}},
	{2, "docker.io/foo:latest", "https", "", false, ImageRef{registry: "docker.io", pullType: byTag, server: "index.docker.io", repository: "foo", ref: "latest", scheme: "https", namespace: "", nsInPath: false, library: true}},
	{3, "docker.io:5000/foo/bar", "https", "", false, ImageRef{registry: "docker.io:5000", pullType: byTag, server: "docker.io:5000", repository: "foo/bar", ref: "latest", scheme: "https", namespace: "", nsInPath: false, library: false}},
	{4, "docker.io:5000/foo/bar:latest", "https", "", false, ImageRef{registry: "docker.io:5000", pullType: byTag, server: "docker.io:5000", repository: "foo/bar", ref: "latest", scheme: "https", namespace: "", nsInPath: false, library: false}},
	{5, "docker.io:5000/foo/bar/baz", "https", "", false, ImageRef{registry: "docker.io:5000", pullType: byTag, server: "docker.io:5000", repository: "foo/bar/baz", ref: "latest", scheme: "https", namespace: "", nsInPath: false, library: false}},
	{6, "docker.io:5000/foo/bar/baz:latest", "https", "", false, ImageRef{registry: "docker.io:5000", pullType: byTag, server: "docker.io:5000", repository: "foo/bar/baz", ref: "latest", scheme: "https", namespace: "", nsInPath: false, library: false}},
	{7, "docker.io/foo:v1.2.3", "http", "", false, ImageRef{registry: "docker.io", pullType: byTag, server: "index.docker.io", repository: "foo", ref: "v1.2.3", scheme: "http", namespace: "", nsInPath: false, library: true}},
	{8, "docker.io/foo/bar:v1.2.3", "https", "", false, ImageRef{registry: "docker.io", pullType: byTag, server: "index.docker.io", repository: "foo/bar", ref: "v1.2.3", scheme: "https", namespace: "", nsInPath: false, library: false}},
	{9, "docker.io/foo/bar/baz:v1.2.3", "https", "", false, ImageRef{registry: "docker.io", pullType: byTag, server: "index.docker.io", repository: "foo/bar/baz", ref: "v1.2.3", scheme: "https", namespace: "", nsInPath: false, library: false}},
	{10, "docker.io/foo@sha256:" + sha, "https", "", false, ImageRef{registry: "docker.io", pullType: byDigest, server: "index.docker.io", repository: "foo", ref: "sha256:" + sha, scheme: "https", namespace: "", nsInPath: false, library: true}},
	{11, "docker.io/foo/bar@sha256:" + sha, "https", "", false, ImageRef{registry: "docker.io", pullType: byDigest, server: "index.docker.io", repository: "foo/bar", ref: "sha256:" + sha, scheme: "https", namespace: "", nsInPath: false, library: false}},
	{12, "docker.io/foo/bar/baz@sha256:" + sha, "https", "", false, ImageRef{registry: "docker.io", pullType: byDigest, server: "index.docker.io", repository: "foo/bar/baz", ref: "sha256:" + sha, scheme: "https", namespace: "", nsInPath: false, library: false}},
	{13, "localhost:8888/docker.io/foo", "https", "", false, ImageRef{registry: "localhost:8888", pullType: byTag, server: "localhost:8888", repository: "foo", ref: "latest", scheme: "https", namespace: "docker.io", nsInPath: true, library: false}},
	{14, "localhost:8888/docker.io/foo:latest", "https", "", false, ImageRef{registry: "localhost:8888", pullType: byTag, server: "localhost:8888", repository: "foo", ref: "latest", scheme: "https", namespace: "docker.io", nsInPath: true, library: false}},
	{15, "localhost:8888/docker.io/foo/bar:latest", "https", "", false, ImageRef{registry: "localhost:8888", pullType: byTag, server: "localhost:8888", repository: "foo/bar", ref: "latest", scheme: "https", namespace: "docker.io", nsInPath: true, library: false}},
	{16, "localhost:8888/docker.io/foo/bar/baz:latest", "https", "", false, ImageRef{registry: "localhost:8888", pullType: byTag, server: "localhost:8888", repository: "foo/bar/baz", ref: "latest", scheme: "https", namespace: "docker.io", nsInPath: true, library: false}},
	{17, "localhost:8888/docker.io/foo/bar", "https", "", false, ImageRef{registry: "localhost:8888", pullType: byTag, server: "localhost:8888", repository: "foo/bar", ref: "latest", scheme: "https", namespace: "docker.io", nsInPath: true, library: false}},
	{18, "localhost:8888/docker.io/foo/bar/baz", "https", "", false, ImageRef{registry: "localhost:8888", pullType: byTag, server: "localhost:8888", repository: "foo/bar/baz", ref: "latest", scheme: "https", namespace: "docker.io", nsInPath: true, library: false}},
	{19, "localhost:8888/docker.io:5000/foo/bar", "https", "", false, ImageRef{registry: "localhost:8888", pullType: byTag, server: "localhost:8888", repository: "foo/bar", ref: "latest", scheme: "https", namespace: "docker.io:5000", nsInPath: true, library: false}},
	{20, "localhost:8888/docker.io/foo:v1.2.3", "http", "", false, ImageRef{registry: "localhost:8888", pullType: byTag, server: "localhost:8888", repository: "foo", ref: "v1.2.3", scheme: "http", namespace: "docker.io", nsInPath: true, library: false}},
	{21, "localhost:8888/docker.io/foo/bar:v1.2.3", "https", "", false, ImageRef{registry: "localhost:8888", pullType: byTag, server: "localhost:8888", repository: "foo/bar", ref: "v1.2.3", scheme: "https", namespace: "docker.io", nsInPath: true, library: false}},
	{22, "localhost:8888/docker.io/foo/bar/baz:v1.2.3", "https", "", false, ImageRef{registry: "localhost:8888", pullType: byTag, server: "localhost:8888", repository: "foo/bar/baz", ref: "v1.2.3", scheme: "https", namespace: "docker.io", nsInPath: true, library: false}},
	{23, "localhost:8888/docker.io/foo@sha256:" + sha, "https", "", false, ImageRef{registry: "localhost:8888", pullType: byDigest, server: "localhost:8888", repository: "foo", ref: "sha256:" + sha, scheme: "https", namespace: "docker.io", nsInPath: true, library: false}},
	{24, "localhost:8888/docker.io/foo/bar@sha256:" + sha, "https", "", false, ImageRef{registry: "localhost:8888", pullType: byDigest, server: "localhost:8888", repository: "foo/bar", ref: "sha256:" + sha, scheme: "https", namespace: "docker.io", nsInPath: true, library: false}},
	{25, "localhost:8888/docker.io/foo/bar/baz@sha256:" + sha, "https", "", false, ImageRef{registry: "localhost:8888", pullType: byDigest, server: "localhost:8888", repository: "foo/bar/baz", ref: "sha256:" + sha, scheme: "https", namespace: "docker.io", nsInPath: true, library: false}},
	{26, "localhost:8888/docker/foo:v1.2.3", "https", "", false, ImageRef{registry: "localhost:8888", pullType: byTag, server: "localhost:8888", repository: "docker/foo", ref: "v1.2.3", scheme: "https", namespace: "", nsInPath: false, library: false}},
	{27, "localhost:8888/foo:v1.2.3", "https", "default.ns", false, ImageRef{registry: "localhost:8888", pullType: byTag, server: "localhost:8888", repository: "foo", ref: "v1.2.3", scheme: "https", namespace: "default.ns", nsInPath: false, library: false}},
	{28, "docker.io/foo", "https", "xyz.io", false, ImageRef{registry: "docker.io", pullType: byTag, server: "index.docker.io", repository: "foo", ref: "latest", scheme: "https", namespace: "xyz.io", nsInPath: false, library: true}},
	{29, "docker.io:5000/foo/bar", "http", "xyz.io", false, ImageRef{registry: "docker.io:5000", pullType: byTag, server: "docker.io:5000", repository: "foo/bar", ref: "latest", scheme: "http", namespace: "xyz.io", nsInPath: false, library: false}},
	{30, "docker.io/foo:v1.2.3", "https", "xyz.io", false, ImageRef{registry: "docker.io", pullType: byTag, server: "index.docker.io", repository: "foo", ref: "v1.2.3", scheme: "https", namespace: "xyz.io", nsInPath: false, library: true}},
	{31, "docker.io/foo@sha256:" + sha, "https", "xyz.io", false, ImageRef{registry: "docker.io", pullType: byDigest, server: "index.docker.io", repository: "foo", ref: "sha256:" + sha, scheme: "https", namespace: "xyz.io", nsInPath: false, library: true}},
	{32, "invalid-ref", "https", "", true, ImageRef{}},
	{33, "docker.io/frobozz.io:v1.1.1", "https", "", true, ImageRef{}},
	{34, "docker.io/frobozz.io@sha256:" + sha, "https", "", true, ImageRef{}},
	{35, "docker.io/frobozz.io:8888:v1.1.1", "https", "", true, ImageRef{}},
	{36, "docker.io/frobozz.io:8888@sha256:" + sha, "https", "", true, ImageRef{}},
}

func Test_UrlParse(t *testing.T) {
	for _, tc := range testCases {
		actual, err := NewImageRef(tc.input, tc.scheme, tc.namespace)
		if tc.shouldErr {
			if err == nil {
				t.FailNow()
			}
			continue
		}
		if err != nil {
			t.FailNow()
		} else if !reflect.DeepEqual(actual, tc.expected) {
			t.FailNow()
		}
	}
}
