package imgpull

import (
	"fmt"
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
