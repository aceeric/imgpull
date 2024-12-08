package imgpull

import (
	"testing"
)

func TestNew1(t *testing.T) {
	url := "docker.io/hello-world:latest"
	scheme := "http"

	http := func() PullOpt {
		return func(p *PullerOpts) {
			p.Scheme = scheme
		}
	}
	p, err := NewPuller(url, http())
	if err != nil {
		t.Fail()
	}
	if p.Opts.Url != url && p.Opts.Scheme != scheme {
		t.Fail()
	}
}
