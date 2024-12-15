package imgpull

import (
	"fmt"
	"imgpull/mock"
	"net/http"
	"runtime"
	"testing"
)

func TestToDo(t *testing.T) {
	server, url := mock.Server(mock.NewMockParams(mock.BEARER, mock.HTTP))
	defer server.Close()
	pullOpts := PullerOpts{
		Url:      fmt.Sprintf("%s/hello-world:latest", url),
		Scheme:   "http",
		OStype:   runtime.GOOS,
		ArchType: runtime.GOARCH,
	}
	p, err := NewPullerWith(pullOpts)
	if err != nil {
		t.Fail()
	}
	status, auth, err := p.regCliFrom().v2()
	if err != nil {
		t.Fail()
	}
	if status != http.StatusUnauthorized {
		t.Fail()
	}
	if len(auth) != 1 {
		t.Fail()
	}
}
