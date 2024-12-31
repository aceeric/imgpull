package imgpull

import (
	"fmt"
	"imgpull/mock"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Tests bearer auth using the 'v2/' endpoint
func TestV2(t *testing.T) {
	server, url := mock.Server(mock.NewMockParams(mock.BEARER, mock.NOTLS, mock.CertSetup{}))
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

// If a blob file already exists on the file system then a call to v2/blobs
// should do nothing.
func TestV2BlobsExists(t *testing.T) {
	d, _ := os.MkdirTemp("", "")
	defer os.RemoveAll(d)
	digest := makeDigest()
	blobFile := filepath.Join(d, digest)
	err := os.WriteFile(blobFile, []byte(digest), 0644)
	if err != nil {
		t.Fail()
	}
	layer := Layer{
		MediaType: "unused.by.this.test",
		Digest:    digest,
		Size:      len(digest),
	}
	// if the logic that immediately returns if the blob file
	// already exists is executed, then the empty regClient struct
	// is ingored.
	if (regClient{}).v2Blobs(layer, blobFile) != nil {
		t.Fail()
	}
}

// Tests getting a blob
func TestV2BlobsSimple(t *testing.T) {
	server, url := mock.Server(mock.NewMockParams(mock.NONE, mock.NOTLS, mock.CertSetup{}))
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
	d, _ := os.MkdirTemp("", "")
	defer os.RemoveAll(d)

	digest := "sha256:d2c94e258dcb3c5ac2798d32e1249e42ef01cba4841c2234249495f87264ac5a"
	blobFile := filepath.Join(d, digest)

	rc := p.regCliFrom()
	layer := Layer{
		MediaType: V2dockerLayerGzipMt,
		Digest:    digest,
		Size:      581, // mock/testfiles/d2c9.json
	}
	if rc.v2Blobs(layer, blobFile) != nil {
		t.Fail()
	}
}

// Tests concurrent blob fetch. Spins up multiple goroutines to get the
// same blob and verifies that only one goroutine actually called the
// v2/blobs endpoint. (The others were therefore enqueued.)
func TestV2BlobsConcur(t *testing.T) {
	blob := "zzzz"
	digest := makeDigest()

	var httpMethodCnt atomic.Uint64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/hello-world/blobs/sha256:"+digest {
			t.Fail()
		}
		httpMethodCnt.Add(1)
		w.WriteHeader(http.StatusOK)
		w.Header().Add("Content-Length", strconv.Itoa(len(blob)))
		w.Header().Add("Content-Type", "application/octet-stream")
		for i := 0; i < len(blob); i++ {
			w.Write([]byte(blob[i : i+1]))
			time.Sleep(500 * time.Millisecond)
		}
	}))
	defer server.Close()

	pullOpts := PullerOpts{
		Url:      strings.ReplaceAll(fmt.Sprintf("%s/hello-world:latest", server.URL), "http://", ""),
		Scheme:   "http",
		OStype:   "linux",
		ArchType: "amd64",
	}

	d, _ := os.MkdirTemp("", "")
	defer os.RemoveAll(d)

	SetConcurrentBlobs(10)

	var wg sync.WaitGroup
	blobPullerCnt := 6
	for i := 0; i < blobPullerCnt; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p, err := NewPullerWith(pullOpts)
			if err != nil {
				t.Fail()
			}
			rc := p.regCliFrom()
			layer := Layer{
				MediaType: V2dockerLayerGzipMt,
				Digest:    "sha256:" + digest,
				Size:      len(blob),
			}
			if rc.v2Blobs(layer, filepath.Join(d, digest)) != nil {
				fmt.Println(err)
				t.Fail()
			}
		}()
		time.Sleep(100 * time.Millisecond)
	}
	wg.Wait()
	if int(httpMethodCnt.Load()) != 1 {
		t.Fail()
	}
}
