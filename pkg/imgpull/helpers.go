package imgpull

import (
	"regexp"
)

var srch = `.*([a-f0-9]{64}).*`
var re = regexp.MustCompile(srch)

// digestFrom looks in the passed arg for a 64-character digest and, if
// found, returns the digest *without* a "sha256:" prefix.
func digestFrom(str string) string {
	tmpdgst := re.FindStringSubmatch(str)
	if len(tmpdgst) == 2 {
		return tmpdgst[1]
	}
	return ""
}
