package imgpull

import (
	"archive/tar"
	"encoding/json"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// DockerTarManifest is the structure of 'manifest.json' that you would find
// in a tarball produced by 'docker save'.
type DockerTarManifest struct {
	Config   string   `json:"config"`
	RepoTags []string `json:"repoTags"`
	Layers   []string `json:"layers"`
}

// toTarNew creates an image tarball at the path/name specified in 'tarfile'. The
// 'sourceDir' is expected to contain:
//
//  1. A config blob named as a digest matching 'configDigest'.
//  2. Layer files named as digests matching the 'Digest' value of each layer in the
//     layer arg.
func toTarNew(sourceDir, tarfile, configDigest, imageUrl string, layers []Layer) (DockerTarManifest, error) {
	dtm := DockerTarManifest{
		Config:   "sha256:" + configDigest,
		RepoTags: []string{imageUrl},
	}

	file, err := os.Create(tarfile)
	if err != nil {
		return DockerTarManifest{}, err
	}
	defer file.Close()
	tw := tar.NewWriter(file)
	defer tw.Close()

	for _, layer := range layers {
		if ext, err := extensionForLayer(layer.MediaType); err != nil {
			return DockerTarManifest{}, err
		} else {
			dtm.Layers = append(dtm.Layers, layer.Digest+ext)
			fname := filepath.Join(sourceDir, layer.Digest)
			err = addFile(tw, fname, fname+ext)
			if err != nil {
				return DockerTarManifest{}, err
			}
		}
	}
	manifest, err := dtm.toString()
	if err != nil {
		return DockerTarManifest{}, err
	}
	err = addString(tw, string(manifest), "manifest.json")
	if err != nil {
		return DockerTarManifest{}, err
	}
	err = addFile(tw, filepath.Join(sourceDir, configDigest), dtm.Config)
	if err != nil {
		return DockerTarManifest{}, err
	}
	return dtm, nil
}

// toTar creates an image tarball at the path/name specified in 'tarfile' from
// information in the receiver and files in the 'sourceDir' directory. The 'sourceDir'
// is expected to contain:
//
//  1. The image manifest: 'manifest.json'
//  2. A config layer whose name (like sha256:...) matches the config in the receiver
//  3. Layer files named as listed in the Layers array in the receiver
func (dtm *DockerTarManifest) toTar(sourceDir string, tarfile string) error {
	//file, err := os.Create(tarfile)
	//if err != nil {
	//	return err
	//}
	//defer file.Close()
	//tw := tar.NewWriter(file)
	//defer tw.Close()
	//addFile(tw, filepath.Join(sourceDir, "manifest.json"))
	//addFile(tw, filepath.Join(sourceDir, dtm.Config))
	//for _, layer := range dtm.Layers {
	//	addFile(tw, filepath.Join(sourceDir, layer))
	//}
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

func (dtm *DockerTarManifest) toString() ([]byte, error) {
	manifestArray := make([]DockerTarManifest, 1)
	manifestArray[0] = *dtm
	return json.MarshalIndent(manifestArray, "", "   ")
}

// addFile adds a file identified by the passed 'actualFile' to the
// passed tar file. The 'fileNameInTar' arg allows to give the file in the
// tarball a filename different from the file name on the file system.
func addFile(tw *tar.Writer, actualFile, fileNameInTar string) error {
	file, err := os.Open(actualFile)
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
	header.Name = filepath.Base(fileNameInTar)
	err = tw.WriteHeader(header)
	if err != nil {
		return err
	}
	_, err = io.Copy(tw, file)
	return err
}

// addString adds the passed string to the tarfile as though it were
// a file. When you untar the file the extracted string behaves like
// any other file in the tar file.
func addString(tw *tar.Writer, content, name string) error {
	u, err := user.Current()
	if err != nil {
		return err
	}
	uid, err := strconv.Atoi(u.Uid)
	if err != nil {
		return err
	}
	gid, err := strconv.Atoi(u.Uid)
	if err != nil {
		return err
	}
	gname := ""
	g, err := user.LookupGroupId(u.Gid)
	if err != nil {
		return err
	} else {
		gname = g.Name
	}
	header := tar.Header{
		Typeflag:   tar.TypeReg,
		Name:       name,
		Linkname:   "",
		Size:       int64(len(content)),
		Mode:       436,
		Uid:        uid,
		Gid:        gid,
		Uname:      u.Username,
		Gname:      gname,
		ModTime:    time.Now(),
		AccessTime: time.Now(),
		ChangeTime: time.Now(),
		Devmajor:   0,
		Devminor:   0,
		Xattrs:     nil,
		PAXRecords: nil,
		Format:     tar.FormatUnknown,
	}
	err = tw.WriteHeader(&header)
	if err != nil {
		return err
	}
	_, err = io.Copy(tw, strings.NewReader(content))
	return err
}
