package imgpull

import "github.com/aceeric/imgpull/internal/blobsync"

// SetConcurrentBlobs exposes the ability to configure blob download concurrency
// at the package level since this function is encapsulated within the 'blobsync'
// internal package. The 'timeoutSec' arg indicates how long a blob pull will
// run before timing out, and is intended to accommodate slow or degraded network
// connectivity to the upstream.
func SetConcurrentBlobs(timeoutSec int) {
	blobsync.SetConcurrentBlobs(timeoutSec)
}
