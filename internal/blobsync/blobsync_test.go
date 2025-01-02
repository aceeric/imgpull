package blobsync

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestSetConcur(t *testing.T) {
	SetConcurrentBlobs(42)
	if blobTimeoutSec != 42 || blobPulls.pullMap == nil {
		t.Fail()
	}
}

// Tests that concurrent requests for the same digest will result in
// only one goroutine executing the pull logic, simulated here with
// incrementing a counter.
func TestQueue(t *testing.T) {
	var counter atomic.Uint64
	var wg sync.WaitGroup
	digest := "frobozz"
	SetConcurrentBlobs(10)

	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			so := EnqueueGet(digest)
			go func() {
				if so.Result == NotEnqueued {
					counter.Add(1)
					time.Sleep(1 * time.Second)
					DoneGet(digest)
				}
			}()
			if Wait(so) != nil {
				t.Fail()
			}
		}()
	}
	wg.Wait()
	if counter.Load() != 1 {
		t.Fail()
	}
}
