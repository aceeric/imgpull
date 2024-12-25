package imgpull

import (
	"fmt"
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
		pr, err := newImageRef(url.url, "https")
		if url.shouldParse && err != nil {
			t.Fail()
		} else if !url.shouldParse && err == nil {
			t.Fail()
		} else if url.shouldParse && pr.ImageUrl() != url.parsedUrl {
			imageUrl := pr.ImageUrl()
			fmt.Println(imageUrl)
			t.Fail()
		}
	}
}
