// Package blobsync supports using the library to concurrently pull blobs
// from multiple goroutines. Rather than have multiple goroutines attempt to
// pull the same blob at the same time, blob pulls are enqueued and only the
// first one in does the pull - the other goroutines wait and simply use the
// blob pulled by the first goroutine.
//
// Concurrency is not enabled by default, which supports using the project
// as a CLI to simply pull image tarballs. To enable blob concurrency with
// a sixty second timeout on all blob pulls:
//
//	sixtySeconds := 60
//	blobsync.SetConcurrentBlobs(sixtySeconds)
package blobsync
