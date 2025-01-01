package util

import (
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
func DigestFrom(str string) string {
	tmpdgst := re.FindStringSubmatch(str)
	if len(tmpdgst) == 2 {
		return tmpdgst[1]
	}
	return ""
}
