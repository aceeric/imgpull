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
)

func (p *Puller) PullTar() error {
	tmpDir, err := os.MkdirTemp("/tmp", "imgpull.")
	if err != nil {
		return err
	}
	defer func() {
		err := os.Remove(tmpDir)
		if err != nil {
			// what can we do?
		}
	}()
	if tm, err := p.Pull(tmpDir); err != nil {
		return err
	} else {
		return toTar(tm, p.Opts.Dest, tmpDir)
	}
}

func (p *Puller) Pull(toPath string) (DockerTarManifest, error) {
	if err := p.Connect(); err != nil {
		return DockerTarManifest{}, err
	}
	mh, err := p.v2Manifests("")
	if err != nil {
		return DockerTarManifest{}, err
	}
	if mh.IsManifestList() {
		err := saveManifest(mh, toPath, "image-index.json")
		if err != nil {
			return DockerTarManifest{}, err
		}
		digest, err := mh.GetImageDigestFor(p.Opts.OSType, p.Opts.ArchType)
		if err != nil {
			return DockerTarManifest{}, err
		}
		im, err := p.v2Manifests(digest)
		if err != nil {
			return DockerTarManifest{}, err
		}
		mh = im
	}
	err = saveManifest(mh, toPath, "image.json")
	if err != nil {
		return DockerTarManifest{}, err
	}
	configDigest, err := mh.GetImageConfig()
	if err != nil {
		return DockerTarManifest{}, err
	}
	if err := p.v2Blobs(configDigest, toPath, true); err != nil {
		return DockerTarManifest{}, err
	}
	for i := 0; i < mh.Layers(); i++ {
		layer := mh.Layer(i)
		if err := p.v2Blobs(layer, toPath, false); err != nil {
			return DockerTarManifest{}, err
		}
	}
	tm, err := mh.NewDockerTarManifest(p.ImgPull, p.Opts.Namespace)
	if err != nil {
		return DockerTarManifest{}, err
	}
	saveDockerTarManifest(tm, toPath, "manifest.json")

	return tm, nil
}

func (p *Puller) HeadManifest() (ManifestDescriptor, error) {
	if err := p.Connect(); err != nil {
		return ManifestDescriptor{}, err
	}
	return p.v2ManifestsHead()
}

func (p *Puller) Connect() error {
	// HTTP status codes that we will interpret as un-authorized
	unauth := []int{http.StatusUnauthorized, http.StatusForbidden}

	if p.Connected {
		return nil
	}
	status, auth, err := p.v2()
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
//  2. basic (using the user/pass that the registry instance was initialized from)
//
// If successful then the instance is initialized with the corresponding auth
// struct so that it is available to be used for all subsequent API calls to the
// distribution server. For example if 'bearer' then the token received from the
// remote registry will be added to the instance.
func (p *Puller) authenticate(auth []string) error {
	fmt.Println(auth)
	for _, hdr := range auth {
		if strings.HasPrefix(strings.ToLower(hdr), "bearer") {
			ba := parseBearer(hdr)
			return p.v2Auth(ba)
		} else if strings.HasPrefix(strings.ToLower(hdr), "basic") {
			delimited := fmt.Sprintf("%s:%s", p.Opts.Username, p.Opts.Password)
			encoded := base64.StdEncoding.EncodeToString([]byte(delimited))
			return p.v2Basic(encoded)
		}
	}
	return fmt.Errorf("unable to parse auth param: %v", auth)
}

// saveManifest extracts the manifest from the passed 'ManifestHolder' and
// saves it to a file with the passed name in the passed path.
func saveManifest(mh ManifestHolder, toPath string, name string) error {
	json, err := mh.ToString()
	if err != nil {
		return err
	}
	return saveFile([]byte(json), toPath, name)
}

// saveDockerTarManifest saves the passed docker tar manifest which is required to be contained in
// an image tarball, i.e. a tarball that can be loaded with 'docker load'.
func saveDockerTarManifest(tm DockerTarManifest, toPath string, name string) error {
	// it has to be written as an array of []tarexport.manifestItem
	manifestArray := make([]DockerTarManifest, 1)
	manifestArray[0] = tm
	marshalled, err := json.MarshalIndent(manifestArray, "", "   ")
	if err != nil {
		return err
	}
	return saveFile(marshalled, toPath, name)
}

// saveFile is a low level util function that saves the passed bytes
// to a file with the passed name in the passed path.
func saveFile(manifest []byte, toPath string, name string) error {
	file, err := os.Create(filepath.Join(toPath, name))
	if err != nil {
		return err
	}
	defer file.Close()
	file.Write(manifest)
	return nil
}

// parseBearer parses the passed auth header which the caller should ensure is a bearer
// type www-authenticate header like 'Bearer realm="https://auth.docker.io/token",service="registry.docker.io"'
// and returns the parsed result in the 'BearerAuth' struct.
func parseBearer(authHdr string) BearerAuth {
	ba := BearerAuth{}
	parts := []string{"realm", "service"}
	mexpr := `%s[\s]*=[\s]*"{1}([0-9A-Za-z\-:/.,]*)"{1}`
	for _, part := range parts {
		srch := fmt.Sprintf(mexpr, part)
		m := regexp.MustCompile(srch)
		matches := m.FindStringSubmatch(authHdr)
		if len(matches) == 2 {
			if part == "realm" {
				ba.Realm = strings.ReplaceAll(matches[1], "\"", "")
			} else {
				ba.Service = strings.ReplaceAll(matches[1], "\"", "")
			}
		}
	}
	return ba
}
