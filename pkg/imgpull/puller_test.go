package imgpull

import (
	"testing"
)

func TestPullerOptfunc(t *testing.T) {
	url := "docker.io/hello-world:latest"
	scheme := "http"
	ostype := "linux"
	archtype := "amd64"

	http := func() PullOpt {
		return func(p *PullerOpts) {
			p.Scheme = scheme
		}
	}
	linux := func() PullOpt {
		return func(p *PullerOpts) {
			p.OStype = ostype
		}
	}
	amd64 := func() PullOpt {
		return func(p *PullerOpts) {
			p.ArchType = archtype
		}
	}
	p, err := NewPuller(url, http(), linux(), amd64())
	if err != nil {
		t.Fail()
	}
	if p.Opts.Url != url || p.Opts.Scheme != scheme || p.Opts.OStype != ostype || p.Opts.ArchType != archtype {
		t.Fail()
	}
}
