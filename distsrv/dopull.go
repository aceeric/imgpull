package distsrv

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

var unauth []int = []int{http.StatusUnauthorized, http.StatusForbidden}

func (r *Registry) PullTar(toFile string) error {
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
		return toTar(tm, toFile, tmpDir)
	}
}

func (r *Registry) Pull(toPath string) (DockerTarManifest, error) {
	status, auth, err := r.v2()
	if err != nil {
		return DockerTarManifest{}, err
	}
	// TODO add 200ish check to below
	if slices.Contains(unauth, status) {
		err := r.authenticate(auth)
		if err != nil {
			return DockerTarManifest{}, err
		}
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
		digest, err := mh.GetImageDigestFor(r.OSType, r.ArchType)
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
	tm, err := mh.NewDockerTarManifest(r.ImgPull)
	if err != nil {
		return DockerTarManifest{}, err
	}
	saveDockerTarManifest(tm, toPath, "manifest.json")

	return tm, nil
}

func (r *Registry) authenticate(auth []string) error {
	fmt.Println(auth)
	for _, hdr := range auth {
		if strings.HasPrefix(strings.ToLower(hdr), "bearer") {
			ba := ParseBearer(hdr)
			return r.v2Auth(ba)
		}
	}
	return fmt.Errorf("unable to parse auth param: %v", auth)
}

func saveManifest(mh ManifestHolder, toPath string, name string) error {
	json, err := mh.ToString()
	if err != nil {
		return err
	}
	return saveFile([]byte(json), toPath, name)
}

// it has to be written as an array of []tarexport.manifestItem
func saveDockerTarManifest(tm DockerTarManifest, toPath string, name string) error {
	manifestArray := make([]DockerTarManifest, 1)
	manifestArray[0] = tm
	marshalled, err := json.MarshalIndent(manifestArray, "", "   ")
	if err != nil {
		return err
	}
	return saveFile(marshalled, toPath, name)
}

func saveFile(manifest []byte, toPath string, name string) error {
	file, err := os.Create(filepath.Join(toPath, name))
	if err != nil {
		return err
	}
	defer file.Close()
	file.Write(manifest)
	return nil
}
