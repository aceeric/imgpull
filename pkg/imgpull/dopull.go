package imgpull

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/aceeric/imgpull/internal/imgref"
	"github.com/aceeric/imgpull/internal/logging"
	"github.com/aceeric/imgpull/internal/methods"
	"github.com/aceeric/imgpull/internal/tar"
	"github.com/aceeric/imgpull/internal/util"
	"github.com/aceeric/imgpull/pkg/imgpull/types"
)

// Puller is the interface to the package for pulling images and manifests.
type Puller interface {
	// GetManifestByType pulls an image manifest or an image list manifest based on the value
	// of the 'mpt' arg.
	GetManifestByType(mpt ManifestPullType) (ManifestHolder, error)
	// GetManifest gets a manifest for the image in the receiver. If the receiver
	// is configured with a tag then the manifest returned is determined by the
	// upstream registry: if an image list manifest is available, it will be provided by
	// the registry. If no image list manifest is available then an image manifest
	// will be provided by the registry if available. Whatever the registry provides
	// is returned in a 'ManifestHolder' which holds all four supported manifest types,
	// only one of which will be populated.
	GetManifest() (ManifestHolder, error)
	// GetManifestByDigest is like GetManifest except uses the passed digest
	GetManifestByDigest(digest string) (ManifestHolder, error)
	// HeadManifest does a HEAD request for the image URL in the receiver. The
	// 'ManifestDescriptor' returned to the caller contains the image digest,
	// media type and manifest size, as provided by the upstream distribution
	// server.
	HeadManifest() (types.ManifestDescriptor, error)
	// PullBlobs pulls the blobs for an image, writing them into 'blobDir'.
	PullBlobs(mh ManifestHolder, blobDir string) error
	// PullTar pulls an image tarball from a registry based on the configuration
	// options in the receiver and writes it to the path/file name specified in the
	// 'dest' arg.
	PullTar(dest string) error
	// GetUrl returns the image ref from the receiver
	GetUrl() string
	// SetUrl supports reusing a puller with a different image ref.
	SetUrl(url string) error
	// GetOpts returns puller options
	GetOpts() PullerOpts
	// Close closes the puller
	Close()
}

// HTTP status codes that we will interpret as un-authorized
var unauth = []int{http.StatusUnauthorized, http.StatusForbidden}

func (p *puller) PullTar(dest string) error {
	if dest == "" {
		return fmt.Errorf("no destination specified for pull of %q", p.Opts.Url)
	}
	tmpDir, err := os.MkdirTemp("/tmp", "imgpull.")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)
	if itb, err := p.pull(tmpDir); err != nil {
		return err
	} else {
		_, err := itb.ToTar(dest)
		return err
	}
}

func (p *puller) GetManifestByType(mpt ManifestPullType) (ManifestHolder, error) {
	if err := p.connect(); err != nil {
		return ManifestHolder{}, err
	}
	rc := p.regCliFrom()
	mr, err := rc.V2Manifests("")
	if err != nil {
		return ManifestHolder{}, err
	}
	mh, err := newManifestHolder(mr.MediaType, mr.ManifestBytes, mr.ManifestDigest, rc.ImgRef.Url())
	if err != nil {
		return ManifestHolder{}, err
	}
	if mh.IsManifestList() {
		if mpt == ImageList {
			return mh, nil
		}
		digest, err := mh.GetImageDigestFor(p.Opts.OStype, p.Opts.ArchType)
		if err != nil {
			return ManifestHolder{}, err
		}
		mr, err := rc.V2Manifests(digest)
		if err != nil {
			return ManifestHolder{}, err
		}
		mh, err = newManifestHolder(mr.MediaType, mr.ManifestBytes, mr.ManifestDigest, rc.ImgRef.UrlWithDigest(digest))
		if err != nil {
			return ManifestHolder{}, err
		}
		return mh, nil
	}
	// if we get here, then the registry did not have a manifest list and so
	// it provided an image manifest
	if mpt == Image {
		return mh, nil
	} else {
		return ManifestHolder{}, fmt.Errorf("server did not provide a manifest for %q", p.ImgRef.Url())
	}
}

func (p *puller) PullBlobs(mh ManifestHolder, blobDir string) error {
	if err := p.connect(); err != nil {
		return err
	}
	if err := os.MkdirAll(blobDir, 0755); err != nil {
		return fmt.Errorf("unable to create directory %q, error: %q", blobDir, err)
	}
	logging.L().Infof("pulling %d blob(s) for %s", len(mh.Layers()), p.ImgRef.Url())
	for _, layer := range mh.Layers() {
		if err := p.pullBlob(layer, blobDir); err != nil {
			return err
		}
	}
	return nil
}

// pullBlob pulls one blob into blobDir. A bearer token that is about to expire is renewed
// first: on a slow link, pulling earlier (large) blobs can take longer than the token's
// lifetime. The registry client is built per blob so it carries the current token.
func (p *puller) pullBlob(layer types.Layer, blobDir string) error {
	if err := p.renewTokenIfExpiring(); err != nil {
		return err
	}
	return p.regCliFrom().V2Blobs(layer, filepath.Join(blobDir, util.DigestFrom(layer.Digest)))
}

func (p *puller) HeadManifest() (types.ManifestDescriptor, error) {
	if err := p.connect(); err != nil {
		return types.ManifestDescriptor{}, err
	}
	return p.regCliFrom().V2ManifestsHead()
}

func (p *puller) GetManifest() (ManifestHolder, error) {
	return p.internalGetManifest("")
}

func (p *puller) GetManifestByDigest(digest string) (ManifestHolder, error) {
	return p.internalGetManifest(digest)
}

func (p *puller) internalGetManifest(digest string) (ManifestHolder, error) {
	if err := p.connect(); err != nil {
		return ManifestHolder{}, err
	}
	rc := p.regCliFrom()
	mr, err := rc.V2Manifests(digest)
	if err != nil {
		return ManifestHolder{}, err
	}
	return newManifestHolder(mr.MediaType, mr.ManifestBytes, mr.ManifestDigest, rc.ImgRef.Url())
}

func (p *puller) GetUrl() string {
	return p.ImgRef.Url()
}

func (p *puller) SetUrl(url string) error {
	if ir, err := imgref.NewImageRef(url, p.Opts.Scheme, p.Opts.Namespace); err != nil {
		return err
	} else if p.ImgRef.Registry() != ir.Registry() {
		return fmt.Errorf("incoming registry %s must match existing %s", ir.Registry(), p.ImgRef.Registry())
	} else {
		p.ImgRef = ir
	}
	return nil
}

func (p *puller) GetOpts() PullerOpts {
	return p.Opts
}

func (p *puller) Close() {
	if p.Client != nil {
		p.Client.CloseIdleConnections()
	}
}

// pull pulls the image specified in the receiver, saving blobs to the passed 'blobDir'.
// An 'imageTarball' struct is returned that describes the pulled image. The directory
// specfied by 'blobDir' will be populated with:
//
//  1. The configuration blob
//  2. The layer blobs.
//
// All blobs are saved into this directory with filenames consisting of 64-character digests.
func (p *puller) pull(blobDir string) (tar.ImageTarball, error) {
	if err := p.connect(); err != nil {
		return tar.ImageTarball{}, err
	}
	rc := p.regCliFrom()
	mr, err := rc.V2Manifests("")
	if err != nil {
		return tar.ImageTarball{}, err
	}
	mh, err := newManifestHolder(mr.MediaType, mr.ManifestBytes, mr.ManifestDigest, rc.ImgRef.Url())
	if err != nil {
		return tar.ImageTarball{}, err
	}
	if mh.IsManifestList() {
		digest, err := mh.GetImageDigestFor(p.Opts.OStype, p.Opts.ArchType)
		if err != nil {
			return tar.ImageTarball{}, err
		}
		mr, err := rc.V2Manifests(digest)
		if err != nil {
			return tar.ImageTarball{}, err
		}
		mh, err = newManifestHolder(mr.MediaType, mr.ManifestBytes, mr.ManifestDigest, rc.ImgRef.UrlWithDigest(digest))
		if err != nil {
			return tar.ImageTarball{}, err
		}
	}
	for _, layer := range mh.Layers() {
		if err := p.pullBlob(layer, blobDir); err != nil {
			return tar.ImageTarball{}, err
		}
	}
	return mh.newImageTarball(p.ImgRef, blobDir)
}

// connect calls the 'v2' endpoint and looks for an auth header. If an auth
// header is provided by the remote registry then this function will attempt
// to negotiate the auth handshake for Bearer if the remote requests it, or
// Basic using the user/pass in the receiver. Once successfully authenticated,
// the auth credential (bearer token or encrypted user/pass) are retained in
// the receiver for all the other API methods to build an auth header with.
//
// If the function has already been called on the receiver, it immediately
// returns taking no action.
func (p *puller) connect() error {
	if p.Connected {
		return nil
	} else if p.Opts.Token != "" {
		// if a token provided from an external source was provided then we
		// will believe that token is valid and simply use it
		logging.L().Debugf("connect %s: using the provided token", p.ImgRef.Url())
		p.ExtToken.Token = p.Opts.Token
		p.Connected = true
		return nil
	}
	status, auth, err := p.regCliFrom().V2ManifestsAuth()
	if err != nil {
		return err
	}
	if status != http.StatusOK && slices.Contains(unauth, status) {
		err := p.authenticate(auth)
		if err != nil {
			return err
		}
	}
	p.Connected = true
	return nil
}

// authenticate scans the passed list of auth headers received from a distribution
// server and attempts to perform authentication for each in the following order:
//
//  1. bearer
//  2. basic (using the user/pass that the puller receiver was initialized from)
//
// If successful then the receiver is initialized with the corresponding auth
// struct so that it is available to be used for all subsequent API calls to the
// distribution server. For example if 'bearer' then the token received from the
// remote registry will be added to the receiver.
func (p *puller) authenticate(auth []string) error {
	rc := p.regCliFrom()
	for _, hdr := range auth {
		if strings.HasPrefix(strings.ToLower(hdr), "bearer") {
			return p.getBearerToken(parseBearer(hdr))
		} else if strings.HasPrefix(strings.ToLower(hdr), "basic") {
			logging.L().Debugf("connect %s: using basic auth", p.ImgRef.Url())
			delimited := fmt.Sprintf("%s:%s", p.Opts.Username, p.Opts.Password)
			encoded := base64.StdEncoding.EncodeToString([]byte(delimited))
			ba, err := rc.V2Basic(encoded)
			if err != nil {
				return err
			}
			p.Basic = ba
			return nil
		}
	}
	return fmt.Errorf("unable to parse auth param: %v", auth)
}

// defaultTokenLifetime is the lifetime to assume for a bearer token whose response has no
// expires_in, per the Docker token authentication spec.
const defaultTokenLifetime = 60 * time.Second

// tokenRenewWindow: before each blob pull, a bearer token that expires within this window is
// renewed. It's a var so tests can change it.
var tokenRenewWindow = 5 * time.Second

// getBearerToken requests a bearer token for the passed realm, service and scope, using the
// receiver's username and password if it has them, and stores it in the receiver with its
// expiry. The expiry is measured from when the token was requested rather than from the
// token's issued_at, so a difference between the registry's clock and this one doesn't matter.
func (p *puller) getBearerToken(ba types.BearerAuth) error {
	encoded := ""
	if p.Opts.Username != "" && p.Opts.Password != "" {
		delimited := fmt.Sprintf("%s:%s", p.Opts.Username, p.Opts.Password)
		encoded = base64.StdEncoding.EncodeToString([]byte(delimited))
	}
	requested := time.Now()
	logging.L().Debugf("requesting a bearer token: realm %s, service %q, scope %q, credentials: %t", ba.Realm, ba.Service, ba.Scope, encoded != "")
	bt, err := p.regCliFrom().V2Auth(ba, encoded)
	if err != nil {
		logging.L().Errorf("bearer token request to %s: %s", ba.Realm, err)
		return err
	}
	lifetime := tokenLifetime(bt)
	p.Token = bt
	p.bearerAuth = ba
	p.tokenExpiry = requested.Add(lifetime)
	expiresIn, jwt := "none", "not a JWT"
	if bt.ExpiresIn > 0 {
		expiresIn = (time.Duration(bt.ExpiresIn) * time.Second).String()
	}
	if d, ok := jwtLifetime(bearerValue(bt)); ok {
		jwt = d.String()
	}
	logging.L().Infof("got a bearer token from %s, valid for %s (expires_in: %s, JWT: %s)", ba.Realm, lifetime, expiresIn, jwt)
	return nil
}

// bearerValue returns the token to send in the Authorization header. Registries return it
// as "token" and, for OAuth2 compatibility, often also as "access_token"; some return only
// "access_token".
func bearerValue(bt types.BearerToken) string {
	if bt.Token != "" {
		return bt.Token
	}
	return bt.AccessToken
}

// tokenLifetime returns how long the passed bearer token is valid: from the response's
// expires_in and, if the token is a JWT (e.g. from the GitLab container registry, which may
// not send expires_in), from its claims. When both are known the shorter is used. If neither
// is, the Docker token spec's default of 60 seconds is used.
func tokenLifetime(bt types.BearerToken) time.Duration {
	var lifetime time.Duration
	if bt.ExpiresIn > 0 {
		lifetime = time.Duration(bt.ExpiresIn) * time.Second
	}
	if jwt, ok := jwtLifetime(bearerValue(bt)); ok && (lifetime == 0 || jwt < lifetime) {
		lifetime = jwt
	}
	if lifetime <= 0 {
		lifetime = defaultTokenLifetime
	}
	return lifetime
}

// jwtLifetime returns exp - iat (or exp - nbf if there is no iat) from the passed token's
// claims if the token is a JWT. The signature isn't checked: the registry does that, and the
// claims are only used to decide when to renew. Both claims are on the issuer's clock, so a
// difference between that clock and this one doesn't matter. A token that isn't a JWT, or
// whose claims don't give a positive lifetime, returns false.
func jwtLifetime(token string) (time.Duration, bool) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return 0, false
	}
	payload, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(parts[1], "="))
	if err != nil {
		return 0, false
	}
	var claims struct {
		Exp int64 `json:"exp"`
		Iat int64 `json:"iat"`
		Nbf int64 `json:"nbf"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil || claims.Exp == 0 {
		return 0, false
	}
	start := claims.Iat
	if start == 0 {
		start = claims.Nbf
	}
	if start == 0 || claims.Exp <= start {
		return 0, false
	}
	return time.Duration(claims.Exp-start) * time.Second, true
}

// renewTokenIfExpiring renews the receiver's bearer token if it expires within
// tokenRenewWindow. Basic auth and externally provided tokens are not renewed here.
func (p *puller) renewTokenIfExpiring() error {
	if p.Token == (types.BearerToken{}) || time.Until(p.tokenExpiry) > tokenRenewWindow {
		return nil
	}
	logging.L().Infof("renewing the bearer token: %s left", time.Until(p.tokenExpiry).Round(time.Millisecond))
	return p.getBearerToken(p.bearerAuth)
}

// regCliFrom creates a 'RegClient' from the receiver, consisting of a subset of receiver
// fields needed to interact with the OCI Distribution Server V2 REST API. It supports
// a looser coupling of the Puller from actually interacting with the distribution server.
//
// If this function is intended to return a regClient to make API calls that require auth
// headers, then the Connect function must previously have been called on the receiver so
// that the auth struct in the receiver is initialized by virtue of that call. The auth
// struct is copied into the returned regClient struct which is used to set auth headers.
func (p *puller) regCliFrom() methods.RegClient {
	rc := methods.RegClient{
		ImgRef: p.ImgRef,
		Client: p.Client,
	}
	if k, v := p.authHdr(); k != "" {
		rc.AuthHdr = methods.AuthHeader{
			Key:   k,
			Value: v,
		}
	}
	return rc
}

// parseBearer parses the passed auth header which the caller should ensure is a bearer
// type "www-authenticate" header like:
//
//		Bearer realm="https://auth.docker.io/token",service="registry.docker.io"
//	    Bearer realm="https://ghcr.io/token",service="ghcr.io",scope="repository:aceeric/ociregistry:pull"
//
// The function returns the parsed result in a 'BearerAuth' struct.
func parseBearer(authHdr string) types.BearerAuth {
	ba := types.BearerAuth{}
	parts := []string{"realm", "service", "scope"}
	expr := `%s[\s]*=[\s]*"{1}([0-9A-Za-z\-:/.,_]*)"{1}`
	for _, part := range parts {
		srch := fmt.Sprintf(expr, part)
		m := regexp.MustCompile(srch)
		matches := m.FindStringSubmatch(authHdr)
		if len(matches) == 2 {
			switch part {
			case "realm":
				ba.Realm = strings.ReplaceAll(matches[1], "\"", "")
			case "scope":
				ba.Scope = strings.ReplaceAll(matches[1], "\"", "")
			default:
				ba.Service = strings.ReplaceAll(matches[1], "\"", "")
			}
		}
	}
	return ba
}
