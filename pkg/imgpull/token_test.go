package imgpull

import (
	"encoding/base64"
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
	// response is how tokens are returned: "" for {"token", "expires_in"}, "jwt" for a JWT
	// with no expires_in (like the GitLab container registry), "jwt-access" for a JWT returned
	// only as "access_token" with no expires_in.
	response string
	blobs    map[string][]byte // digest -> content
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
		if reg.response != "" {
			now := time.Now().Unix()
			token = testJwt(fmt.Sprintf(`{"jti":%q,"iat":%d,"nbf":%d,"exp":%d}`, token, now, now-5, now+int64(reg.lifetime.Seconds())))
		}
		reg.issued[token] = time.Now()
		reg.mu.Unlock()
		switch reg.response {
		case "jwt":
			fmt.Fprintf(w, `{"token":%q}`, token)
		case "jwt-access":
			fmt.Fprintf(w, `{"access_token":%q}`, token)
		default:
			fmt.Fprintf(w, `{"token":%q,"expires_in":%d}`, token, int(reg.lifetime.Seconds()))
		}
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

// testJwt returns an unsigned JWT with the passed claims. The signature is never checked by
// the client, so a placeholder is enough.
func testJwt(claims string) string {
	enc := base64.RawURLEncoding
	return enc.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`)) + "." + enc.EncodeToString([]byte(claims)) + ".c2ln"
}

// Tests renewal with GitLab-style tokens: a JWT with no expires_in. Without reading the JWT's
// claims the client assumes 60 seconds, doesn't renew, and the third blob gets a 401.
func TestRenewJwtBeforeBlob(t *testing.T) {
	for _, response := range []string{"jwt", "jwt-access"} {
		t.Run(response, func(t *testing.T) {
			reg, p, mh := tokenFixture(t, 2*time.Second, 1200*time.Millisecond)
			reg.response = response
			if err := p.PullBlobs(mh, t.TempDir()); err != nil {
				t.Fatalf("pull failed: %s", err)
			}
			if reg.authCalls < 2 {
				t.Fatalf("expected the JWT to be renewed, got %d token request(s)", reg.authCalls)
			}
		})
	}
}

// Tests reading a token's lifetime from JWT claims, including tokens that aren't JWTs or
// whose claims can't be used.
func TestJwtLifetime(t *testing.T) {
	enc := base64.RawURLEncoding
	tests := []struct {
		name  string
		token string
		want  time.Duration
		ok    bool
	}{
		{"iat and exp", testJwt(`{"iat":1000,"exp":1300}`), 300 * time.Second, true},
		{"nbf when no iat", testJwt(`{"nbf":1000,"exp":1060}`), 60 * time.Second, true},
		{"iat preferred over nbf", testJwt(`{"iat":1000,"nbf":995,"exp":1300}`), 300 * time.Second, true},
		{"padded payload", "e30." + enc.EncodeToString([]byte(`{"iat":1,"exp":11}`)) + "==.c2ln", 10 * time.Second, true},
		{"opaque token", "FROBOZZ", 0, false},
		{"two parts", "a.b", 0, false},
		{"payload not base64", "e30.!!!.c2ln", 0, false},
		{"payload not json", "e30." + enc.EncodeToString([]byte("not json")) + ".c2ln", 0, false},
		{"no exp", testJwt(`{"iat":1000}`), 0, false},
		{"no iat or nbf", testJwt(`{"exp":1300}`), 0, false},
		{"exp before iat", testJwt(`{"iat":1300,"exp":1000}`), 0, false},
	}
	for _, tst := range tests {
		got, ok := jwtLifetime(tst.token)
		if got != tst.want || ok != tst.ok {
			t.Errorf("%s: expected %s %t, got %s %t", tst.name, tst.want, tst.ok, got, ok)
		}
	}
}

// Tests choosing a token's lifetime: expires_in, the JWT's claims, the shorter of the two
// when both are present, and the 60 second default when neither is.
func TestTokenLifetime(t *testing.T) {
	jwt5m := testJwt(`{"iat":1000,"exp":1300}`)
	tests := []struct {
		name string
		bt   types.BearerToken
		want time.Duration
	}{
		{"expires_in only", types.BearerToken{Token: "FROBOZZ", ExpiresIn: 120}, 120 * time.Second},
		{"jwt only", types.BearerToken{Token: jwt5m}, 300 * time.Second},
		{"jwt as access_token only", types.BearerToken{AccessToken: jwt5m}, 300 * time.Second},
		{"expires_in shorter", types.BearerToken{Token: jwt5m, ExpiresIn: 60}, 60 * time.Second},
		{"jwt shorter", types.BearerToken{Token: jwt5m, ExpiresIn: 900}, 300 * time.Second},
		{"neither", types.BearerToken{Token: "FROBOZZ"}, defaultTokenLifetime},
	}
	for _, tst := range tests {
		if got := tokenLifetime(tst.bt); got != tst.want {
			t.Errorf("%s: expected %s, got %s", tst.name, tst.want, got)
		}
	}
}

// Tests that the Authorization header uses "token", or "access_token" when that's all the
// registry returned.
func TestBearerValue(t *testing.T) {
	if v := bearerValue(types.BearerToken{Token: "A", AccessToken: "B"}); v != "A" {
		t.Errorf("expected token to be preferred, got %q", v)
	}
	if v := bearerValue(types.BearerToken{AccessToken: "B"}); v != "B" {
		t.Errorf("expected access_token when there is no token, got %q", v)
	}
}
