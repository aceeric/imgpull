package imgpull

import (
	"os"
	"path/filepath"
	"regexp"
)

var (
	pat = `.*\b([a-f0-9]{64})\b.*`
	re  = regexp.MustCompile(pat)
)

// digestFrom looks in the passed arg for a 64-character digest and, if
// found, returns the bare digest (without any prefix. If no digest is found
// then the empty string is returned. The digest has to be bounded on both
// sides by a word boundary.
func digestFrom(str string) string {
	tmpdgst := re.FindStringSubmatch(str)
	if len(tmpdgst) == 2 {
		return tmpdgst[1]
	}
	return ""
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
