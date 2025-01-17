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
	if p.GetOpts().Url != url || p.GetOpts().Scheme != scheme || p.GetOpts().OStype != ostype || p.GetOpts().ArchType != archtype {
		t.Fail()
	}
}
