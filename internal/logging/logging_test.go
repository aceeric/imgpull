package logging

import (
	"fmt"
	"testing"
)

type recorder struct{ lines []string }

func (r *recorder) Debugf(f string, a ...any) {
	r.lines = append(r.lines, "DEBUG "+fmt.Sprintf(f, a...))
}
func (r *recorder) Infof(f string, a ...any) { r.lines = append(r.lines, "INFO "+fmt.Sprintf(f, a...)) }
func (r *recorder) Errorf(f string, a ...any) {
	r.lines = append(r.lines, "ERROR "+fmt.Sprintf(f, a...))
}

// Tests that the default logger discards, that Set routes to the passed logger, and that
// Set(nil) restores the default.
func TestSetLogger(t *testing.T) {
	if _, ok := L().(discard); !ok {
		t.Fatal("the default logger should discard")
	}
	L().Infof("dropped") // must not panic
	r := &recorder{}
	Set(r)
	L().Debugf("d %d", 1)
	L().Infof("i %d", 2)
	L().Errorf("e %d", 3)
	if fmt.Sprint(r.lines) != "[DEBUG d 1 INFO i 2 ERROR e 3]" {
		t.Fatalf("unexpected lines: %v", r.lines)
	}
	Set(nil)
	if _, ok := L().(discard); !ok {
		t.Fatal("Set(nil) should restore the default")
	}
}
