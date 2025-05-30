package mock

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// These are the objects returned by the mock server
var (
	manifestList  []byte
	imageManifest []byte
	d2c9          []byte
	c1ec          []byte
)

// SchemeType specifies http or https
type SchemeType string

const (
	HTTP  SchemeType = "http"
	HTTPS SchemeType = "https"
)

// AuthType specifies the supported auth types
type AuthType int

const (
	BASIC AuthType = iota
	BEARER
	NONE
)

// MockParams supports different configurations for the mock OCI
// Distribution Server
type MockParams struct {
	Auth      AuthType
	Scheme    SchemeType
	TlsConfig *tls.Config
	CliAuth   tls.ClientAuthType
	Certs     CertSetup
}

// fileToLoad has a test file to load and the pointer of the variable to load it in to.
type fileToLoad struct {
	fname string
	vname *[]byte
	strip bool
}

// NewMockParams returns a 'MockParams' struct from the passed args.
func NewMockParams(auth AuthType, tt TlsType, certSetup CertSetup) MockParams {
	mp := MockParams{
		Auth:      auth,
		TlsConfig: &tls.Config{},
		Scheme:    HTTP,
	}
	if tt == NOTLS {
		return mp
	}
	// tls from here down
	mp.Scheme = HTTPS
	mp.TlsConfig = &tls.Config{}
	// if any TLS, the server will present certs
	mp.TlsConfig.Certificates = []tls.Certificate{certSetup.ServerCert}
	if tt == ONEWAY_INSECURE || tt == ONEWAY_SECURE {
		mp.TlsConfig.ClientAuth = tls.NoClientCert
	} else {
		mp.TlsConfig.ClientAuth = tls.RequireAnyClientCert
	}
	mp.Certs = certSetup
	return mp
}

// Server runs the mock OCI distribution server. It returns a ref to the server, and a
// server url (without the scheme - like 'localhost:12345').
func Server(params MockParams) (*httptest.Server, string) {
	var err error
	testFilesDir := getTestFilesDir()

	filesToLoad := []fileToLoad{
		{fname: "manifestList.json", vname: &manifestList, strip: true},
		{fname: "imageManifest.json", vname: &imageManifest, strip: false},
		{fname: "d2c9.json", vname: &d2c9, strip: false},
		{fname: "c1ec31eb59444d78df06a974d155e597c894ab4cda84f08294145e845394988e.tar.gz", vname: &c1ec, strip: false},
	}

	m1 := regexp.MustCompile(`[\r\n\t ]{1}`)
	for _, testFile := range filesToLoad {
		*testFile.vname, err = os.ReadFile(filepath.Join(testFilesDir, testFile.fname))
		if err != nil {
			panic(err)
		}
		if testFile.strip {
			*testFile.vname = []byte(m1.ReplaceAllString(string(*testFile.vname), ""))
		}
	}

	gmtTimeLoc := time.FixedZone("GMT", 0)
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.Replace(r.URL.Path, "/library/", "/", 1)
		if p == "/v2/" {
			if params.Auth == NONE {
				w.WriteHeader(http.StatusOK)
			} else {
				if r.Header.Get("Authorization") != "" {
					// just believe the client
					w.WriteHeader(http.StatusOK)
				} else {
					body := []byte(`{"errors":[{"code":"UNAUTHORIZED","message":"authentication required","detail":null}]}`)
					authUrl := `Basic realm="%s://%s"`
					if params.Auth == BEARER {
						authUrl = `Bearer realm="%s://%s/v2/auth",service="registry.docker.io"`
					}
					authHdr := fmt.Sprintf(authUrl, params.Scheme, r.Host)
					w.Header().Set("Content-Length", strconv.Itoa(len(body)))
					w.Header().Set("Content-Type", "application/json")
					w.Header().Set("Date", time.Now().In(gmtTimeLoc).Format(http.TimeFormat))
					w.Header().Set("Docker-Distribution-Api-Version", "registry/2.0")
					w.Header().Set("Www-Authenticate", authHdr)
					w.WriteHeader(http.StatusUnauthorized)
					w.Write(body)
				}
			}
		} else if p == "/v2/auth" {
			if params.Auth != BEARER {
				w.WriteHeader(http.StatusUnauthorized)
			} else {
				w.Header().Set("Content-Length", "19")
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`{"token":"FROBOZZ"}`))
			}
		} else if p == "/v2/hello-world/manifests/latest" {
			w.Header().Set("Content-Length", strconv.Itoa(len(manifestList))) // 9125
			w.Header().Set("Content-Type", "application/vnd.oci.image.index.v1+json")
			w.Header().Set("Date", time.Now().In(gmtTimeLoc).Format(http.TimeFormat))
			w.Header().Set("Docker-Content-Digest", "sha256:e4ccfd825622441dcee5123f9d4a48b2eb8787d858de346106a83f0c745cc255")
			w.Header().Set("Docker-Distribution-Api-Version", "registry/2.0")
			w.Write([]byte(manifestList))
		} else if p == "/v2/hello-world/manifests/sha256:e2fc4e5012d16e7fe466f5291c476431beaa1f9b90a5c2125b493ed28e2aba57" {
			w.Header().Add("Content-Length", strconv.Itoa(len(imageManifest)))
			w.Header().Add("Content-Type", "application/vnd.oci.image.manifest.v1+json")
			w.Header().Add("Date", time.Now().In(gmtTimeLoc).Format(http.TimeFormat))
			w.Header().Add("Docker-Content-Digest", "sha256:e2fc4e5012d16e7fe466f5291c476431beaa1f9b90a5c2125b493ed28e2aba57")
			w.Header().Add("Docker-Distribution-Api-Version", "registry/2.0")
			w.Write([]byte(imageManifest))
		} else if p == "/v2/hello-world/blobs/sha256:d2c94e258dcb3c5ac2798d32e1249e42ef01cba4841c2234249495f87264ac5a" {
			w.Header().Add("Content-Length", strconv.Itoa(len(d2c9)))
			w.Header().Add("Content-Type", "application/octet-stream")
			w.Header().Add("Date", time.Now().In(gmtTimeLoc).Format(http.TimeFormat))
			w.Write([]byte(d2c9))
		} else if p == "/v2/hello-world/blobs/sha256:c1ec31eb59444d78df06a974d155e597c894ab4cda84f08294145e845394988e" {
			w.Header().Add("Content-Length", strconv.Itoa(len(c1ec)))
			w.Header().Add("Content-Type", "application/octet-stream")
			w.Header().Add("Date", time.Now().In(gmtTimeLoc).Format(http.TimeFormat))
			w.Write([]byte(c1ec))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	if params.Scheme == HTTPS {
		server.TLS = params.TlsConfig
		server.StartTLS()
	} else {
		server.Start()
	}
	return server, regexp.MustCompile(`https://|http://`).ReplaceAllString(server.URL, "")
}

// getTestFilesDir finds the directory that this file is in because the
// mock registry server could be used from other test directories but it
// needs files in this directory.
func getTestFilesDir() string {
	for d, _ := os.Getwd(); d != "/"; d = filepath.Dir(d) {
		if _, err := os.Stat(filepath.Join(d, "go.mod")); err == nil {
			return filepath.Join(d, "mock/testfiles")
		}
	}
	panic(errors.New("no go.mod?"))
}
