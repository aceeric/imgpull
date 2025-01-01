package methods

import (
	"fmt"
	"imgpull/internal/blobsync"
	"imgpull/internal/imgref"
	"imgpull/internal/testhelpers"
	"imgpull/mock"
	"imgpull/pkg/imgpull/types"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
	rc, err := newRegClient("hello-world:latest", url)
	if err != nil {
		t.Fail()
	}
	status, auth, err := rc.V2()
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
	digest := testhelpers.MakeDigest()
	blobFile := filepath.Join(d, digest)
	err := os.WriteFile(blobFile, []byte(digest), 0644)
	if err != nil {
		t.Fail()
	}
	layer := types.Layer{
		MediaType: "unused.by.this.test",
		Digest:    digest,
		Size:      len(digest),
	}
	// if the logic that immediately returns if the blob file
	// already exists is executed, then the empty regClient struct
	// is ingored.
	if (RegClient{}).V2Blobs(layer, blobFile) != nil {
		t.Fail()
	}
}

// Tests getting a blob
func TestV2BlobsSimple(t *testing.T) {
	server, url := mock.Server(mock.NewMockParams(mock.NONE, mock.NOTLS, mock.CertSetup{}))
	rc, err := newRegClient("hello-world:latest", url)
	if err != nil {
		t.Fail()
	}
	defer server.Close()
	d, _ := os.MkdirTemp("", "")
	defer os.RemoveAll(d)

	digest := "sha256:d2c94e258dcb3c5ac2798d32e1249e42ef01cba4841c2234249495f87264ac5a"
	blobFile := filepath.Join(d, digest)

	layer := types.Layer{
		MediaType: types.V2dockerLayerGzipMt,
		Digest:    digest,
		Size:      581, // mock/testfiles/d2c9.json
	}
	if rc.V2Blobs(layer, blobFile) != nil {
		t.Fail()
	}
}

// Tests concurrent blob fetch. Spins up multiple goroutines to get the
// same blob and verifies that only one goroutine actually called the
// v2/blobs endpoint. (The others were therefore enqueued.)
func TestV2BlobsConcur(t *testing.T) {
	blob := "zzzz"
	digest := testhelpers.MakeDigest()

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

	d, _ := os.MkdirTemp("", "")
	defer os.RemoveAll(d)

	blobsync.SetConcurrentBlobs(10)

	var wg sync.WaitGroup
	blobPullerCnt := 6
	for i := 0; i < blobPullerCnt; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			layer := types.Layer{
				MediaType: types.V2dockerLayerGzipMt,
				Digest:    "sha256:" + digest,
				Size:      len(blob),
			}
			rc, err := newRegClient("hello-world:latest", strings.ReplaceAll(server.URL, "http://", ""))
			if err != nil {
				t.Fail()
			}
			if rc.V2Blobs(layer, filepath.Join(d, digest)) != nil {
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

func newRegClient(image string, url string) (RegClient, error) {
	ir, err := imgref.NewImageRef(fmt.Sprintf("%s/%s", url, image), "http")
	if err != nil {
		return RegClient{}, err
	}
	return RegClient{
		ImgRef:    ir,
		Client:    &http.Client{},
		Namespace: "",
		AuthHdr:   AuthHeader{},
	}, nil
}
