package imgpull

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aceeric/imgpull/pkg/imgpull/types"
	"github.com/aceeric/imgpull/pkg/imgpull/v1oci"
	"github.com/opencontainers/go-digest"
)

// tokenRegistry is a minimal registry that issues bearer tokens with a fixed lifetime, rejects
// expired tokens with 401 like a real registry, and serves each blob slowly, like a large blob
// on a slow link.
type tokenRegistry struct {
	mu        sync.Mutex
	lifetime  time.Duration
	blobDelay time.Duration
	issued    map[string]time.Time // token -> when it was issued
	authCalls int
	blobs     map[string][]byte // digest -> content
}

func (reg *tokenRegistry) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodHead && strings.Contains(r.URL.Path, "/manifests/"):
		// the connect check: always challenge, so the client gets a bearer token
		w.Header().Set("Www-Authenticate", fmt.Sprintf(`Bearer realm="http://%s/token",service="test"`, r.Host))
		w.WriteHeader(http.StatusUnauthorized)
	case r.URL.Path == "/token":
		reg.mu.Lock()
		reg.authCalls++
		token := fmt.Sprintf("T%d", reg.authCalls)
		reg.issued[token] = time.Now()
		reg.mu.Unlock()
		fmt.Fprintf(w, `{"token":%q,"expires_in":%d}`, token, int(reg.lifetime.Seconds()))
	case strings.Contains(r.URL.Path, "/blobs/"):
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		reg.mu.Lock()
		issued, known := reg.issued[token]
		content := reg.blobs[r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]]
		reg.mu.Unlock()
		if !known || time.Since(issued) > reg.lifetime {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		time.Sleep(reg.blobDelay)
		w.Write(content)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

// tokenFixture starts a tokenRegistry with three blobs and returns a puller for it and a
// manifest that references the blobs.
func tokenFixture(t *testing.T, lifetime, blobDelay time.Duration) (*tokenRegistry, Puller, ManifestHolder) {
	t.Helper()
	reg := &tokenRegistry{lifetime: lifetime, blobDelay: blobDelay, issued: map[string]time.Time{}, blobs: map[string][]byte{}}
	descriptors := []v1oci.Descriptor{}
	for i := range 3 {
		content := []byte(fmt.Sprintf("blob %d", i))
		d := digest.FromBytes(content)
		reg.blobs[d.String()] = content
		descriptors = append(descriptors, v1oci.Descriptor{Digest: d.String(), Size: int64(len(content))})
	}
	srv := httptest.NewServer(reg)
	t.Cleanup(srv.Close)
	p, err := NewPuller(strings.TrimPrefix(srv.URL, "http://")+"/test/image:v1", httpOpts)
	if err != nil {
		t.Fatal(err)
	}
	mh := ManifestHolder{
		Type:          V1ociManifest,
		V1ociManifest: v1oci.Manifest{Config: descriptors[0], Layers: descriptors[1:]},
	}
	return reg, p, mh
}

// Tests that a bearer token close to expiry is renewed before each blob, so a pull whose
// earlier blobs take longer than the token's lifetime still succeeds. Each blob takes 1.2s and
// the token lives 2s, so without renewal the third blob is requested with an expired token.
func TestRenewTokenBeforeBlob(t *testing.T) {
	reg, p, mh := tokenFixture(t, 2*time.Second, 1200*time.Millisecond)
	dir := t.TempDir()
	if err := p.PullBlobs(mh, dir); err != nil {
		t.Fatalf("pull failed: %s", err)
	}
	for _, layer := range mh.Layers() {
		if _, err := os.Stat(filepath.Join(dir, strings.TrimPrefix(layer.Digest, "sha256:"))); err != nil {
			t.Fatalf("blob %s not pulled", layer.Digest)
		}
	}
	if reg.authCalls < 2 {
		t.Fatalf("expected the token to be renewed, got %d token request(s)", reg.authCalls)
	}
}

// Tests that a token with plenty of time left is not renewed.
func TestNoRenewWithTimeLeft(t *testing.T) {
	reg, p, mh := tokenFixture(t, 300*time.Second, 0)
	if err := p.PullBlobs(mh, t.TempDir()); err != nil {
		t.Fatalf("pull failed: %s", err)
	}
	if reg.authCalls != 1 {
		t.Fatalf("expected a single token request, got %d", reg.authCalls)
	}
}

// Tests the expiry computed for a token: expires_in when given, otherwise the spec's 60s
// default, measured from when the token was requested.
func TestTokenExpiry(t *testing.T) {
	for expiresIn, want := range map[int]time.Duration{300: 300 * time.Second, 0: defaultTokenLifetime} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintf(w, `{"token":"T","expires_in":%d}`, expiresIn)
		}))
		pl, err := NewPuller(strings.TrimPrefix(srv.URL, "http://")+"/test/image:v1", httpOpts)
		if err != nil {
			t.Fatal(err)
		}
		p := pl.(*puller)
		before := time.Now()
		if err := p.getBearerToken(types.BearerAuth{Realm: srv.URL + "/token", Service: "test"}); err != nil {
			t.Fatal(err)
		}
		if got := p.tokenExpiry.Sub(before); got < want-time.Second || got > want+time.Second {
			t.Errorf("expires_in %d: expected an expiry about %s out, got %s", expiresIn, want, got)
		}
		srv.Close()
	}
}

// httpOpts configures a test puller for a plain http registry.
func httpOpts(o *PullerOpts) {
	o.Scheme = "http"
	o.OStype = "linux"
	o.ArchType = "amd64"
}
