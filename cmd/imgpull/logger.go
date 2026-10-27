package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"strings"
)

// logLevel orders the levels from least to most verbose.
type logLevel int

const (
	errorLevel logLevel = iota
	infoLevel
	debugLevel
)

var levelNames = map[logLevel]string{errorLevel: "error", infoLevel: "info", debugLevel: "debug"}

// levelLogger writes messages at or below its level to an io.Writer, with a timestamp and
// level prefix. It satisfies imgpull.Logger.
type levelLogger struct {
	level logLevel
	out   *log.Logger
}

// newStderrLogger returns a levelLogger that writes to stderr, so stdout output, such as
// --manifest, is unaffected.
func newStderrLogger(level logLevel) *levelLogger {
	return newLevelLogger(level, os.Stderr)
}

func newLevelLogger(level logLevel, w io.Writer) *levelLogger {
	return &levelLogger{level: level, out: log.New(w, "", log.LstdFlags|log.Lmicroseconds)}
}

func (l *levelLogger) Debugf(format string, args ...any) { l.logf(debugLevel, format, args...) }
func (l *levelLogger) Infof(format string, args ...any)  { l.logf(infoLevel, format, args...) }
func (l *levelLogger) Errorf(format string, args ...any) { l.logf(errorLevel, format, args...) }

// String returns the level's name.
func (lvl logLevel) String() string {
	return levelNames[lvl]
}

// parseLogLevel returns the level for a --log-level value, ignoring case.
func parseLogLevel(s string) (logLevel, error) {
	for lvl, name := range levelNames {
		if strings.EqualFold(s, name) {
			return lvl, nil
		}
	}
	return errorLevel, fmt.Errorf("invalid value %q for --log-level arg: must be error, info, or debug", s)
}

func (l *levelLogger) logf(lvl logLevel, format string, args ...any) {
	if lvl <= l.level {
		l.out.Printf(strings.ToUpper(lvl.String())+" "+format, args...)
	}
}
