package imgpull

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

type digestTest struct {
	tst      string
	expected string
}

func TestDigestFrom(t *testing.T) {
	for _, dt := range []digestTest{
		{"1234567890123456789012345678901234567890123456789012345678901234", "1234567890123456789012345678901234567890123456789012345678901234"},
		{"sha256:1234567890123456789012345678901234567890123456789012345678901234", "1234567890123456789012345678901234567890123456789012345678901234"},
		{"foo.1234567890123456789012345678901234567890123456789012345678901234.bar", "1234567890123456789012345678901234567890123456789012345678901234"},
		{"sha256:12345678901234567890123456789012345678901234567890123456789012345", ""},
		{"123", ""},
		{"sha256:d2c94e258dcb3c5ac2798d32e1249e42ef01cba4841c2234249495f87264ac5a.extracted", "d2c94e258dcb3c5ac2798d32e1249e42ef01cba4841c2234249495f87264ac5a"},
	} {
		actual := digestFrom(dt.tst)
		if actual != dt.expected {
			t.Fail()
		}
	}
}

func TestSaveFile(t *testing.T) {
	expContent := "test"
	d, _ := os.MkdirTemp("", "")
	defer os.RemoveAll(d)
	if saveFile([]byte(expContent), d, "test") != nil {
		t.Fail()
	}
	actContent, err := os.ReadFile(filepath.Join(d, "test"))
	if err != nil {
		t.Fail()
	}
	if !bytes.Equal([]byte(expContent), actContent) {
		t.Fail()
	}
}
