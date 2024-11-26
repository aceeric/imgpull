package distsrv

import (
	"fmt"
	"regexp"
	"strings"
)

func ParseBearer(authHdr string) BearerAuth {
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
