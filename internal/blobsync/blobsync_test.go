package blobsync

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestSetConcur(t *testing.T) {
	SetConcurrentBlobs(42)
	if blobTimeoutSec != 42 || blobPulls.pulls == nil {
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

	for range 5 {
		wg.Go(func() {
			p, mustPull := Enqueue(digest)
			if mustPull {
				counter.Add(1)
				time.Sleep(1 * time.Second)
				Done(digest, p, nil)
				return
			}
			if Wait(p) != nil {
				t.Error("waiter got error")
			}
		})
	}
	wg.Wait()
	if counter.Load() != 1 {
		t.Fail()
	}
}

func TestWaiterSeesPullerError(t *testing.T) {
	SetConcurrentBlobs(2)
	p, mustPull := Enqueue("d1")
	w, mustPull2 := Enqueue("d1")
	if !mustPull || mustPull2 || p != w {
		t.Fatal("expected one puller and one waiter on the same pull")
	}
	want := errors.New("boom")
	doneCh := make(chan struct{})
	go func() { Done("d1", p, want); close(doneCh) }()
	err := Wait(w)
	<-doneCh // don't let Done outlive the test
	if err != want {
		t.Fatalf("waiter got %v, want %v", err, want)
	}
}

func TestDoneAfterWaiterTimeout(t *testing.T) {
	SetConcurrentBlobs(0) // waiters time out immediately
	p, _ := Enqueue("d2")
	w, _ := Enqueue("d2")
	if Wait(w) == nil {
		t.Fatal("expected timeout")
	}
	done := make(chan struct{})
	go func() { Done("d2", p, nil); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Done blocked")
	}
	if _, mustPull := Enqueue("d2"); !mustPull {
		t.Fatal("expected a new puller after Done")
	}
}
