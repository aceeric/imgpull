package blobsync

import (
	"errors"
	"sync"
	"time"
)

// EnqueueResult represents the result of enqueing a blob pull.
type EnqueueResult bool

// IsEnqueued means that another goroutine already requested a blob for a
// given digest.
const IsEnqueued EnqueueResult = true

// NotEnqueued means no other goroutine has requested a blob with a given
// digest and so the caller must pull it.
const NotEnqueued EnqueueResult = false

// SyncObj has a channel created by an enqueueing action, and the
// result of the enqueueing.
type SyncObj struct {
	Ch     chan bool
	Result EnqueueResult
}

// pull is one in-flight blob download. err is written once, before done is closed;
// closing done guarantees that waiters see err after <-done returns.
type pull struct {
	done chan struct{}
	err  error
}

// pullMap supports multiple threads attempting to pull the same blob concurrently.
// The pulls member is a map of digests, each pointing to a 'pull' struct. The pulling
// goroutine will signal the channel in the struct when it finishes allowing waiting
// goroutines (and the pulling goroutine) to return the blob.
type pullMap struct {
	mu    sync.Mutex
	pulls map[string]*pull
}

var (
	// concurrency blob pull synchronization is off by default.
	ConcurrentBlobs = false
	// blobTimeoutSec specifies - for the concurrent write syncer - how long
	// to wait to be signaled when the blob is done pulling. It is ignored
	// unless concurrency is enabled.
	blobTimeoutSec = 0
	// blobPulls is the synchronized maps of pulls in progress. It is ignored
	// unless concurrency is enabled.
	blobPulls = pullMap{}
)

// SetConcurrentBlobs enables concurrency management for pulling blobs. The function
// is intended to be used when the package is used as a library as an initialization
// step by the code that uses the library. The 'timeoutSec' arg indicate how many
// seconds an enqueued goroutine will wait for a blob download before erroring.
func SetConcurrentBlobs(timeoutSec int) {
	blobTimeoutSec = timeoutSec
	blobPulls.pulls = make(map[string]*pull)
	ConcurrentBlobs = true
}

// Enqueue returns the in-flight pull for the digest, and true if the caller
// must perform the download (false means wait on the returned pull).
func Enqueue(digest string) (*pull, bool) {
	blobPulls.mu.Lock()
	defer blobPulls.mu.Unlock()
	if p, exists := blobPulls.pulls[digest]; exists {
		return p, false
	}
	p := &pull{done: make(chan struct{})}
	blobPulls.pulls[digest] = p
	return p, true
}

// Done records the outcome of the download and releases all waiters.
func Done(digest string, p *pull, err error) {
	blobPulls.mu.Lock()
	defer blobPulls.mu.Unlock()
	p.err = err
	close(p.done)
	delete(blobPulls.pulls, digest)
}

// Wait blocks until the download completes, returning its error, or times out.
func Wait(p *pull) error {
	select {
	case <-p.done:
		return p.err
	case <-time.After(time.Duration(blobTimeoutSec) * time.Second):
		return errors.New("timeout exceeded waiting for blob")
	}
}
