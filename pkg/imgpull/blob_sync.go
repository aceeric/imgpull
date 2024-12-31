package imgpull

import (
	"errors"
	"sync"
	"time"
)

// enqueueResult represents the result of enqueing a blob pull.
type enqueueResult bool

// isEnqueued means that another goroutine already requested a blob for a
// given digest.
const isEnqueued enqueueResult = true

// notEnqueued means no other goroutine has requested a blob with a given
// digest and so the caller must pull it.
const notEnqueued enqueueResult = false

// syncObj has a channel created by an enqueueing action, and the
// result of the enqueueing.
type syncObj struct {
	ch     chan bool
	result enqueueResult
}

// pullMap supports multiple threads attempting to pull the same blob concurrently.
// The pullMap struct member is a map of digests, each having 1+ channel(s) waiting
// for the blob for that digest to finish pulling. The goroutine doing the pulling
// also has a channel in that map.
type pullMap struct {
	mu      sync.Mutex
	pullMap map[string][]chan bool
}

var (
	// concurrency blob pull synchronization is off by default.
	concurrentBlobs = false
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
	blobPulls.pullMap = make(map[string][]chan bool)
	concurrentBlobs = true
}

// enqueueGet enqueues a pull for a blob using the passed digest. If there are
// no other requesters, then the function returns 'notEnqueued' - meaning the caller
// is the first requester and therefore will have to actually pull the blob. If a
// request was previously enqueued for the blob then 'isEnqueued' is returned meaning
// the caller should simply wait for a signal on the channel in the returned syncObj
// struct.
func enqueueGet(digest string) syncObj {
	so := syncObj{
		ch:     make(chan bool),
		result: notEnqueued,
	}
	blobPulls.mu.Lock()
	chans, exists := blobPulls.pullMap[digest]
	if exists {
		blobPulls.pullMap[digest] = append(chans, so.ch)
		so.result = isEnqueued
	} else {
		blobPulls.pullMap[digest] = []chan bool{so.ch}
	}
	blobPulls.mu.Unlock()
	return so
}

// doneGet signals all waiters that are associated with the digest in arg 1.
func doneGet(digest string) {
	blobPulls.mu.Lock()
	chans, exists := blobPulls.pullMap[digest]
	if exists {
		for _, ch := range chans {
			// signal in a func so that if we write on a closed channel we can
			// recover and keep looping
			func() {
				defer func() {
					if err := recover(); err != nil {
						// nop
					}
				}()
				ch <- true
			}()
		}
		delete(blobPulls.pullMap, digest)
	}
	blobPulls.mu.Unlock()
}

// wait waits to be signaled on the channel in the passed syncObj, or times out
// based on the value of the package blobTimeoutSec variable.
func wait(so syncObj) error {
	select {
	case <-so.ch:
		return nil
	case <-time.After(time.Duration(blobTimeoutSec) * time.Second):
		return errors.New("timeout exceeded pulling image")
	}
}
