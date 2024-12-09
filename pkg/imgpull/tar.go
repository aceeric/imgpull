package imgpull

import (
	"archive/tar"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
)

// DockerTarManifest is the structure of 'manifest.json' that you would find
// in a tarball produced by 'docker save'.
type DockerTarManifest struct {
	Config   string   `json:"config"`
	RepoTags []string `json:"repoTags"`
	Layers   []string `json:"layers"`
}

// toTar creates an image tarball at the path/name specified in 'tarfile' from
// information in the receiver and files in the 'sourceDir' directory. The 'sourceDir'
// is expected to contain:
//
//  1. The image manifest: 'manifest.json'
//  2. A config layer whose name (like sha256:...) matches the config in the receiver
//  3. Layer files named as listed in the Layers array in the receiver
func (dtm *DockerTarManifest) toTar(tarfile string, sourceDir string) error {
	file, err := os.Create(tarfile)
	if err != nil {
		return err
	}
	defer file.Close()
	tw := tar.NewWriter(file)
	defer tw.Close()
	addFile(tw, filepath.Join(sourceDir, "manifest.json"))
	addFile(tw, filepath.Join(sourceDir, dtm.Config))
	for _, layer := range dtm.Layers {
		addFile(tw, filepath.Join(sourceDir, layer))
	}
	return nil
}

// saveDockerTarManifest saves the passed docker tar manifest which is required to be contained in
// an image tarball, i.e. a tarball that can be loaded with 'docker load'.
func (dtm *DockerTarManifest) saveDockerTarManifest(toPath string, name string) error {
	// it has to be written as an array of []tarexport.manifestItem
	manifestArray := make([]DockerTarManifest, 1)
	manifestArray[0] = *dtm
	marshalled, err := json.MarshalIndent(manifestArray, "", "   ")
	if err != nil {
		return err
	}
	return saveFile(marshalled, toPath, name)
}

// addFile adds a file identified by the passed 'fileName' which must be a fqpn
// to the passed tar file.
func addFile(tw *tar.Writer, fileName string) error {
	file, err := os.Open(fileName)
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	header, err := tar.FileInfoHeader(info, info.Name())
	if err != nil {
		return err
	}
	header.Name = filepath.Base(fileName)
	err = tw.WriteHeader(header)
	if err != nil {
		return err
	}
	_, err = io.Copy(tw, file)
	if err != nil {
		return err
	}
	return nil
}
