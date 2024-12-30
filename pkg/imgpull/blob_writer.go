package imgpull

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

type enqueueResult bool

type syncObj struct {
	ch          chan bool
	enqueResult enqueueResult
}

type WriteSyncer struct {
	// enqueueGet enqueues a pull for a blob digest. If there are no other requesters,
	// then the function returns 'notEnqueued' - meaning the caller is the first requester
	// and therefore will have to actaully pull the blob. If a request was previously
	// enqueued for the blob then 'isEnqueued' is returned meaning the caller should
	// simply wait for a signal on the channel in the returned syncObj struct. In all
	// cases, all callers will be signalled on the returned channel when the blob is
	// pulled and available on the file system.
	enqueueGet func(string) syncObj
	// doneGet signals all waiters that are associated with the digest in arg 1.
	doneGet func(string, syncObj)
	// wait waits to be signaled on the channel in the passed syncObj, or times out
	// based on the value of blobTimeoutSec.
	wait func(syncObj) error
}

// pullMap supports multiple threads attempting to pull the same blob concurrently.
type pullMap struct {
	mu      sync.Mutex
	pullMap map[string][]chan bool
}

var (
	// simple write syncer doesn't support concurrency but it enables the
	// blob puller to have the same structure whether blobs are pulled
	// concurrently or only in one thread of execution.
	writeSyncer = WriteSyncer{
		enqueueGet: func(string) syncObj {
			return syncObj{
				ch:          make(chan bool),
				enqueResult: notEnqueued,
			}
		},
		doneGet: func(digest string, so syncObj) {
			so.ch <- true
		},
		wait: func(so syncObj) error {
			<-so.ch
			return nil
		},
	}
	// isEnqueued means that another goroutine already requested a blob for a
	// given digest.
	isEnqueued enqueueResult = true
	// notEnqueued means no other goroutine has requested a blob with a given
	// digest and so the caller must pull it.
	notEnqueued enqueueResult = false
	// blobTimeoutSec specifies - for the concurrent write syncer - how long
	// to wait to be signaled when the blob is done pulling. It is ignored
	// by the simple write syncer
	blobTimeoutSec = 0
	// blobPulls is the synchronized maps of pulls in progress. It is ignored
	// by the simple write syncer
	blobPulls = pullMap{}
)

// SetConcurrentBlobs initializes the 'writeSyncer' variable with functions
// that support concurrent blob pulling.
func SetConcurrentBlobs(timeoutSec int) {
	blobTimeoutSec = timeoutSec
	blobPulls.pullMap = make(map[string][]chan bool)
	writeSyncer = WriteSyncer{
		enqueueGet: func(digest string) syncObj {
			so := syncObj{
				ch:          make(chan bool),
				enqueResult: notEnqueued,
			}
			blobPulls.mu.Lock()
			chans, exists := blobPulls.pullMap[digest]
			if exists {
				blobPulls.pullMap[digest] = append(chans, so.ch)
				so.enqueResult = isEnqueued
			} else {
				blobPulls.pullMap[digest] = []chan bool{so.ch}
			}
			blobPulls.mu.Unlock()
			return so
		},
		doneGet: func(digest string, so syncObj) {
			blobPulls.mu.Lock()
			chans, exists := blobPulls.pullMap[digest]
			if exists {
				for _, ch := range chans {
					defer func() {
						if err := recover(); err != nil {
							fmt.Printf("attempt to write to closed channel pulling %s", digest)
						}
					}()
					ch <- true
				}
				delete(blobPulls.pullMap, digest)
			}
			blobPulls.mu.Unlock()
		},
		wait: func(so syncObj) error {
			select {
			case <-so.ch:
				return nil
			case <-time.After(time.Duration(blobTimeoutSec) * time.Second):
				return errors.New("timeout exceeded pulling image")
			}
		},
	}
}
