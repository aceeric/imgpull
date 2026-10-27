package main

import (
	"bytes"
	"strings"
	"testing"
)

// Tests that each level logs its own messages and the less verbose levels' messages, and
// nothing more verbose.
func TestLevelFiltering(t *testing.T) {
	tests := []struct {
		level logLevel
		want  []string
	}{
		{errorLevel, []string{"ERROR e"}},
		{infoLevel, []string{"INFO i", "ERROR e"}},
		{debugLevel, []string{"DEBUG d", "INFO i", "ERROR e"}},
	}
	for _, tst := range tests {
		var buf bytes.Buffer
		l := newLevelLogger(tst.level, &buf)
		l.Debugf("d")
		l.Infof("i")
		l.Errorf("e")
		lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
		if len(lines) != len(tst.want) {
			t.Fatalf("level %s: expected %d line(s), got %q", tst.level, len(tst.want), buf.String())
		}
		for i, want := range tst.want {
			if !strings.HasSuffix(lines[i], want) {
				t.Errorf("level %s: expected line %d to end with %q, got %q", tst.level, i, want, lines[i])
			}
		}
	}
}

// Tests parsing --log-level values.
func TestParseLogLevel(t *testing.T) {
	for s, want := range map[string]logLevel{"error": errorLevel, "INFO": infoLevel, "Debug": debugLevel} {
		if got, err := parseLogLevel(s); err != nil || got != want {
			t.Errorf("%q: expected %s, got %s %v", s, want, got, err)
		}
	}
	for _, s := range []string{"", "warn", "trace"} {
		if _, err := parseLogLevel(s); err == nil {
			t.Errorf("%q: expected an error", s)
		}
	}
}
