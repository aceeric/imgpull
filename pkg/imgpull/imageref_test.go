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
		{"foo.io/bar/baz:tag", true, "foo.io/bar/baz:tag"},
		{"foo.io/baz:tag", true, "foo.io/library/baz:tag"},
		{"foo.io/bar/baz@sha256:123", true, "foo.io/bar/baz@sha256:123"},
		{"foo.io/baz@sha256:123", true, "foo.io/library/baz@sha256:123"},
		{"bar/baz:tag", true, "bar/library/baz:tag"},
		{"baz:tag", false, ""},
		{"bar/baz@sha256:123", true, "bar/library/baz@sha256:123"},
		{"baz@sha256:123", false, ""},
	}
	for _, url := range urls {
		pr, err := newImageRef(url.url, "https")
		if url.shouldParse && err != nil {
			t.Fail()
		} else if !url.shouldParse && err == nil {
			t.Fail()
		} else if url.shouldParse && pr.imageUrlWithNs("") != url.parsedUrl {
			imageUrl := pr.imageUrlWithNs("")
			fmt.Println(imageUrl)
			t.Fail()
		}
	}
}
