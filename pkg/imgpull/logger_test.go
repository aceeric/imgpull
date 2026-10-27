package imgpull

import (
	"encoding/base64"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

// recorder is a Logger that keeps every message.
type recorder struct {
	mu    sync.Mutex
	lines []string
}

func (r *recorder) Debugf(format string, args ...any) { r.add("DEBUG", format, args...) }
func (r *recorder) Infof(format string, args ...any)  { r.add("INFO", format, args...) }
func (r *recorder) Errorf(format string, args ...any) { r.add("ERROR", format, args...) }

func (r *recorder) add(level, format string, args ...any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lines = append(r.lines, level+" "+fmt.Sprintf(format, args...))
}

// Tests the diagnostics for a pull that renews its token: the token, renewal and blob
// messages are logged, and no token value or credential appears in any message.
func TestPullLogging(t *testing.T) {
	for _, response := range []string{"", "jwt", "jwt-access"} {
		t.Run(response, func(t *testing.T) {
			rec := &recorder{}
			SetLogger(rec)
			defer SetLogger(nil)
			reg, p, mh := tokenFixture(t, 2*time.Second, 1200*time.Millisecond)
			reg.response = response
			p.(*puller).Opts.Username = "frobozz"
			p.(*puller).Opts.Password = "xyzzy-secret"
			if err := p.PullBlobs(mh, t.TempDir()); err != nil {
				t.Fatalf("pull failed: %s", err)
			}
			all := strings.Join(rec.lines, "\n")
			for _, want := range []string{"INFO pulling 3 blob(s)", "INFO got a bearer token", "INFO renewing the bearer token", "INFO pulled blob", "DEBUG GET "} {
				if !strings.Contains(all, want) {
					t.Errorf("expected a message containing %q, got:\n%s", want, all)
				}
			}
			reg.mu.Lock()
			defer reg.mu.Unlock()
			for token := range reg.issued {
				if strings.Contains(all, token) {
					t.Errorf("token %q was logged:\n%s", token, all)
				}
			}
			encoded := base64.StdEncoding.EncodeToString([]byte("frobozz:xyzzy-secret"))
			if strings.Contains(all, "xyzzy-secret") || strings.Contains(all, encoded) {
				t.Errorf("password was logged:\n%s", all)
			}
		})
	}
}
