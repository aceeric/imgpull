// Package logging holds the logger imgpull writes its diagnostics to. Until a program sets a
// logger with imgpull.SetLogger, everything is discarded, so the library is silent by default.
package logging

import "sync/atomic"

// Logger receives diagnostics. It is satisfied by logrus's *Logger, among others.
type Logger interface {
	Debugf(format string, args ...any)
	Infof(format string, args ...any)
	Errorf(format string, args ...any)
}

// discard is the default Logger: it drops everything.
type discard struct{}

func (discard) Debugf(string, ...any) {}
func (discard) Infof(string, ...any)  {}
func (discard) Errorf(string, ...any) {}

// box lets an interface value be stored in an atomic.Pointer.
type box struct{ Logger }

// current is the logger in use. It's atomic so a logger can be set while pulls are running.
var current atomic.Pointer[box]

// Set makes the passed logger receive imgpull's diagnostics. nil restores the default, which
// discards them.
func Set(l Logger) {
	if l == nil {
		l = discard{}
	}
	current.Store(&box{l})
}

// L returns the logger in use.
func L() Logger {
	if b := current.Load(); b != nil {
		return b.Logger
	}
	return discard{}
}
