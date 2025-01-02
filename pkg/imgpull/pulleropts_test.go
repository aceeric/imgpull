package imgpull

import (
	"testing"
)

func TestPullerOpts(t *testing.T) {
	url := "docker.io/hello-world:latest"
	p := NewPullerOpts(url)
	if p.Url != url {
		t.Fail()
	}
}

func TestValidate(t *testing.T) {
	for _, po := range []struct {
		opts  PullerOpts
		valid bool
	}{
		{PullerOpts{}, false},
		{PullerOpts{Url: "foo"}, false},
		{opts: PullerOpts{Url: "foo", Scheme: "https"}, valid: false},
		{opts: PullerOpts{Url: "foo", Scheme: "https", OStype: "linux"}, valid: false},
		{opts: PullerOpts{Url: "foo", Scheme: "https", OStype: "linux", ArchType: "amd64"}, valid: true},
		{opts: PullerOpts{Url: "foo", Scheme: "x", OStype: "linux", ArchType: "amd64"}, valid: false},
		{opts: PullerOpts{Url: "foo", Scheme: "https", OStype: "x", ArchType: "amd64"}, valid: false},
		{opts: PullerOpts{Url: "foo", Scheme: "https", OStype: "linux", ArchType: "x"}, valid: false},
	} {
		err := po.opts.validate()
		if po.valid && err != nil {
			t.Fail()
		}
		if !po.valid && err == nil {
			t.Fail()
		}
	}
}
