package distsrv

import (
	"archive/tar"
	"io"
	"os"
	"path/filepath"
)

func toTar(tm DockerTarManifest, tarfile string, sourceDir string) error {
	file, err := os.Create(tarfile)
	if err != nil {
		return err
	}
	defer file.Close()
	tw := tar.NewWriter(file)
	defer tw.Close()
	addFile(tw, filepath.Join(sourceDir, "manifest.json"))
	addFile(tw, filepath.Join(sourceDir, tm.Config))
	for _, layer := range tm.Layers {
		addFile(tw, filepath.Join(sourceDir, layer))
	}
	return nil
}

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
