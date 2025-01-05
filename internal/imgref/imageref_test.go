package imgref

import (
	"testing"
)

type parsetest struct {
	url         string
	shouldParse bool
	parsedUrl   string
}

// this test shows a current weakness which is that it's not possible
// to validate these URLs in all cases because the code can't know
// if the leftmost component which is supposed to be a host (such as
// docker.io) is actually a host.
func TestPRs(t *testing.T) {
	urls := []parsetest{
		{"foo.io/bar/baz:v1.2.3", true, "foo.io/bar/baz:v1.2.3"},
		{"foo.io/baz:v1.2.3", true, "foo.io/baz:v1.2.3"},
		{"foo.io/bar/baz@sha256:123", true, "foo.io/bar/baz@sha256:123"},
		{"foo.io/baz@sha256:123", true, "foo.io/baz@sha256:123"},
		{"docker.io/baz:v8.8.8", true, "docker.io/library/baz:v8.8.8"},
		{"docker.io/baz@sha256:123", true, "docker.io/library/baz@sha256:123"},
		{"docker.io/bar/baz:v8.8.8", true, "docker.io/bar/baz:v8.8.8"},
		{"docker.io/bar/baz@sha256:123", true, "docker.io/bar/baz@sha256:123"},
		{"bar/baz:v1.2.3", true, "bar/baz:v1.2.3"},
		{"baz:v1.2.3", false, ""},
		{"bar/baz@sha256:123", true, "bar/baz@sha256:123"},
		{"baz@sha256:123", false, ""},
		{"foo.io/bar/baz/frobozz:v1.2.3", false, ""},
	}
	for _, url := range urls {
		ir, err := NewImageRef(url.url, "https", "")
		if url.shouldParse && err != nil {
			t.Fail()
		} else if !url.shouldParse && err == nil {
			t.Fail()
		} else if url.shouldParse && ir.Url() != url.parsedUrl {
			t.Fail()
		}
	}
}

// Test make url with permutations of tag, digest, namespace y/n, sha override y/n
func TestMakeUrl(t *testing.T) {
	refs := []string{
		"frobozz.registry.io/foo:v1.2.3",
		"frobozz.registry.io/foo@sha256:123",
	}
	testDigest := "4639e50633756e99edc56b04f814a887c0eb958004c87a95f323558054cc7ef3"
	ns := []string{"", "flathead.com"}
	useNs := []bool{false, true}
	sha := []string{"", testDigest}
	expUrls := []string{
		"frobozz.registry.io/foo:v1.2.3",
		"frobozz.registry.io/foo@sha256:" + testDigest,
		"flathead.com/foo:v1.2.3",
		"flathead.com/foo@sha256:" + testDigest,
		"frobozz.registry.io/foo@sha256:123",
		"frobozz.registry.io/foo@sha256:123",
		"flathead.com/foo@sha256:123",
		"flathead.com/foo@sha256:123",
	}
	urlIdx := 0
	for i := 0; i < len(refs); i++ {
		for j := 0; j < len(ns); j++ {
			for c := 0; c < len(sha); c++ {
				ir, err := NewImageRef(refs[i], "http", ns[j])
				if err != nil {
					t.Fail()
				}
				url := ir.makeUrl(sha[c], useNs[j])
				if url != expUrls[urlIdx] {
					t.Fail()
				}
				urlIdx++
			}
		}
	}
}
