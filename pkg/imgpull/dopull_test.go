package imgpull

import (
	"fmt"
	"imgpull/mock"
	"os"
	"testing"
)

type authHdrTest struct {
	hdr     string
	realm   string
	service string
}

func TestAuthParse(t *testing.T) {
	authHdrTests := []authHdrTest{
		{
			hdr:     `Bearer realm="https://quay.io/v2/auth",service="quay.io"`,
			realm:   "https://quay.io/v2/auth",
			service: "quay.io",
		},
		{
			hdr:     `Bearer realm="https://auth.docker.io/token",service="registry.docker.io"`,
			realm:   "https://auth.docker.io/token",
			service: "registry.docker.io",
		},
	}
	fmt.Println(authHdrTests)
	for _, authHdrTest := range authHdrTests {
		ba := parseBearer(authHdrTest.hdr)
		if ba.Realm != authHdrTest.realm || ba.Service != authHdrTest.service {
			t.Fail()
		}
	}
}

func TestPullManifest(t *testing.T) {
	mp := mock.NewMockParams(mock.BEARER, mock.ONEWAY_SECURE)
	server, url := mock.Server(mp)
	defer server.Close()
	d, _ := os.MkdirTemp("", "")
	caCert := mp.Certs.CaToFile(d, "ca.crt")
	defer os.RemoveAll(d)
	pullOpts := PullerOpts{
		Url:      fmt.Sprintf("%s/hello-world:latest", url),
		Scheme:   "https",
		OStype:   "linux",
		ArchType: "amd64",
		Insecure: true,
		CaCert:   caCert,
	}
	p, err := NewPullerWith(pullOpts)
	if err != nil {
		t.Fail()
	}
	for _, mpt := range []ManifestPullType{ImageList, Image} {
		mh, err := p.PullManifest(mpt)
		if err != nil {
			t.Fail()
		}
		if mpt == ImageList && !mh.IsManifestList() {
			t.Fail()
		}
		if mpt == Image && mh.IsManifestList() {
			t.Fail()
		}
	}
}
