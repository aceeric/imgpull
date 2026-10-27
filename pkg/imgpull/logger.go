package imgpull

import "github.com/aceeric/imgpull/internal/logging"

// Logger receives imgpull's diagnostics: Debugf for each request, Infof for progress such as
// token renewals and completed blobs, and Errorf for failures. logrus's *Logger satisfies it,
// so a program using logrus can pass its logger directly. Credentials and token values are
// never logged.
type Logger = logging.Logger

// SetLogger makes the passed logger receive imgpull's diagnostics. By default they are
// discarded, so the library writes nothing unless a logger is set. nil restores the default.
// It is safe to call while pulls are running.
func SetLogger(l Logger) {
	logging.Set(l)
}
