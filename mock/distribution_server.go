package mock

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
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

// TlsType specifies the supported TLS types. This is used by the puller to
// configure its TLS and by the mock server to configure its TLS. So these values are
// interpreted differently depending on perspective (mock server vs. puller).
type TlsType int

const (
	// HTTP, so, no TLS considerations
	NOTLS TlsType = iota
	// Mock server will present certs to puller. Puller will not present certs. Mock
	// server will not request certs. Server certs will not be validated by puller.
	ONEWAY_INSECURE
	// Mock server will present certs to puller. Puller will not present certs. Mock
	// server will not request certs. Server certs will be validated by puller. Therefore
	// puller will need the test CA Cert.
	ONEWAY_SECURE
	// Mock server will present certs to puller. Puller will present certs. Mock server
	// will request (any) cert. Server certs will not be validated by puller. Therefore
	// puller will not need the test CA Cert. Server will not validate client certs.
	MTLS_INSECURE
	// Mock server will present certs to puller. Puller will present certs. Mock server
	// will request (any) cert. Server certs will be validated by puller. Therefore
	// puller will need the test CA Cert. Server will not validate client certs.
	MTLS_SECURE
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
}

// CertSetup stores the output of the certSetup function
// function.
type CertSetup struct {
	// CaPEM is the CA certificate in PEM form
	CaPEM *bytes.Buffer
	// ServerCert is the server certificate
	ServerCert tls.Certificate
	// ServerCertPEM is the server cert in PEM form
	ServerCertPEM *bytes.Buffer
	// ServerCertPrivKeyPEM is the server key in PEM form
	ServerCertPrivKeyPEM *bytes.Buffer
	// ClientCert is the client certificate
	ClientCert tls.Certificate
	// ClientCertPEM is the client cert in PEM form
	ClientCertPEM *bytes.Buffer
	// ClientCertPrivKeyPEM is the client key in PEM form
	ClientCertPrivKeyPEM *bytes.Buffer
}

// fileToLoad has a test file to load and the pointer of the variable to load it in to.
type fileToLoad struct {
	fname string
	vname *[]byte
	strip bool
}

// NewMockParams returns a 'MockParams' instance from the passed args.
// We don't worry about cert verification in mTLS - we just want to test the mechanics
// of requiring certs from the client.
func NewMockParams(auth AuthType, tt TlsType) MockParams {
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
	cs, err := certSetup()
	if err != nil {
		panic(err)
	}
	mp.TlsConfig = &tls.Config{}
	// if any TLS, the server will present certs
	mp.TlsConfig.Certificates = []tls.Certificate{cs.ServerCert}
	if tt == ONEWAY_INSECURE || tt == ONEWAY_SECURE {
		mp.TlsConfig.ClientAuth = tls.NoClientCert
	} else {
		mp.TlsConfig.ClientAuth = tls.RequireAnyClientCert
	}
	return mp
}

// Server runs the mock OCI distribution server. It returns a ref to the server, and a
// server url (without the scheme).
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
					w.Write([]byte(`{"errors":[{"code":"UNAUTHORIZED","message":"authentication required","detail":null}]}`))
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

// certSetup was adapted from https://gist.github.com/shaneutt/5e1995295cff6721c89a71d13a71c251
// It returns a full-populated 'CertSetup' struct, or an error.
func certSetup() (CertSetup, error) {
	cs := CertSetup{}
	caCert, caPrivKey, caPEM, err := createCACert()
	if err != nil {
		return CertSetup{}, err
	}
	cs.CaPEM = caPEM

	serverCert := newX509("server", false)
	cs.ServerCert, cs.ServerCertPEM, cs.ServerCertPrivKeyPEM, err = createCerts(serverCert, caCert, caPrivKey)
	if err != nil {
		return CertSetup{}, err
	}

	clientCert := newX509("client", false)
	cs.ClientCert, cs.ClientCertPEM, cs.ClientCertPrivKeyPEM, err = createCerts(clientCert, caCert, caPrivKey)
	if err != nil {
		return CertSetup{}, err
	}
	return cs, nil
}

// createCerts returns 1) a tls.Certificate, 2) PEM-encoded cert, and 3) PEM-encoded key from the
// passed x509 cert and ca private key
func createCerts(cert x509.Certificate, caCert x509.Certificate, caPrivKey *rsa.PrivateKey) (tls.Certificate, *bytes.Buffer, *bytes.Buffer, error) {
	pk, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return tls.Certificate{}, nil, nil, err
	}

	certBytes, err := x509.CreateCertificate(rand.Reader, &cert, &caCert, &pk.PublicKey, caPrivKey)
	if err != nil {
		return tls.Certificate{}, nil, nil, err
	}

	certPEM := new(bytes.Buffer)
	pem.Encode(certPEM, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certBytes,
	})

	privKeyPEM := new(bytes.Buffer)
	pem.Encode(privKeyPEM, &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(pk),
	})
	certificate, err := tls.X509KeyPair(certPEM.Bytes(), privKeyPEM.Bytes())
	if err != nil {
		return tls.Certificate{}, nil, nil, err
	}
	return certificate, certPEM, privKeyPEM, nil
}

// createCACert creates a CA cert with Common Name "root". The cert, private key, and
// PEM CA are returned.
func createCACert() (x509.Certificate, *rsa.PrivateKey, *bytes.Buffer, error) {
	ca := newX509("root", true)
	caPrivKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return x509.Certificate{}, nil, nil, err
	}
	caBytes, err := x509.CreateCertificate(rand.Reader, &ca, &ca, &caPrivKey.PublicKey, caPrivKey)
	if err != nil {
		return x509.Certificate{}, nil, nil, err
	}
	caPEM := new(bytes.Buffer)
	pem.Encode(caPEM, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: caBytes,
	})
	return ca, caPrivKey, caPEM, nil
}

// newX509 returns a new x509 cert with the passed common name. If isCA is true then a CA
// cert is generated, otherwise a non-CA cert.
func newX509(cn string, isCA bool) x509.Certificate {
	keyUsage := x509.KeyUsageDigitalSignature
	if isCA {
		keyUsage |= x509.KeyUsageCertSign
	}
	return x509.Certificate{
		SerialNumber: big.NewInt(2019),
		Subject: pkix.Name{
			CommonName: cn,
		},
		IPAddresses:  []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().AddDate(10, 0, 0),
		SubjectKeyId: []byte{1, 2, 3, 4, 6},
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:     keyUsage,
	}
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
