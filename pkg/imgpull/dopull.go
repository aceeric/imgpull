package imgpull

// TODO HIDE ALL FUNCTIONS EXCEPT API!

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// unauth has the HTTP status codes that we will interpret as un-authorized
var unauth []int = []int{http.StatusUnauthorized, http.StatusForbidden}

func (r *Registry) PullTar() error {
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
	if tm, err := r.Pull(tmpDir); err != nil {
		return err
	} else {
		return toTar(tm, r.Opts.Dest, tmpDir)
	}
}

func (r *Registry) Pull(toPath string) (DockerTarManifest, error) {
	if err := r.Connect(); err != nil {
		return DockerTarManifest{}, err
	}
	mh, err := r.v2Manifests("")
	if err != nil {
		return DockerTarManifest{}, err
	}
	if mh.IsManifestList() {
		err := saveManifest(mh, toPath, "image-index.json")
		if err != nil {
			return DockerTarManifest{}, err
		}
		digest, err := mh.GetImageDigestFor(r.Opts.OSType, r.Opts.ArchType)
		if err != nil {
			return DockerTarManifest{}, err
		}
		im, err := r.v2Manifests(digest)
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
	if err := r.v2Blobs(configDigest, toPath, true); err != nil {
		return DockerTarManifest{}, err
	}
	for {
		layer, err := mh.NextLayer()
		if err != nil {
			return DockerTarManifest{}, err
		}
		if layer == (Layer{}) {
			break
		}
		if err := r.v2Blobs(layer, toPath, false); err != nil {
			return DockerTarManifest{}, err
		}
	}
	tm, err := mh.NewDockerTarManifest(r.ImgPull, r.Opts.Namespace)
	if err != nil {
		return DockerTarManifest{}, err
	}
	saveDockerTarManifest(tm, toPath, "manifest.json")

	return tm, nil
}

// IT IS REASONABLE TO HEAD AND GET WITH THE SAME REGISTRY!!
func (r *Registry) HeadManifest() (ManifestHead, error) {
	if err := r.Connect(); err != nil {
		return ManifestHead{}, err
	}
	return r.v2HeadManifests()
}

// TODO connecting with one url means the token is wired to that URL !!!!!!
// THATS OK the registry is single-use!! IT isn't something to connect to a registry
// and then use for multiple pulls thats why the term REGISTRY is misleading !!!!!
// TODO group all implementations of (r *Registry) in one file ? or sub-packages??
//
//	pkg/imgpull/registry.go
//	pkg/imgpull/dopull.go
//	pkg/imgpull/methods.go
func (r *Registry) Connect() error {
	if r.Connected {
		return nil
	}
	status, auth, err := r.v2()
	if err != nil {
		return err
	}
	if status != http.StatusOK && slices.Contains(unauth, status) {
		err := r.authenticate(auth)
		if err != nil {
			return err
		}
	}
	r.Connected = true
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
func (r *Registry) authenticate(auth []string) error {
	fmt.Println(auth)
	for _, hdr := range auth {
		if strings.HasPrefix(strings.ToLower(hdr), "bearer") {
			ba := ParseBearer(hdr)
			return r.v2Auth(ba)
		} else if strings.HasPrefix(strings.ToLower(hdr), "basic") {
			delimited := fmt.Sprintf("%s:%s", r.Opts.Username, r.Opts.Password)
			encoded := base64.StdEncoding.EncodeToString([]byte(delimited))
			return r.v2Basic(encoded)
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
