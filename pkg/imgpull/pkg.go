// Package imgpull is a library for pulling OCI Images.
//
// The top level functions provided by the library are:
//
//	func NewPuller(url string, opts ...PullOpt) - Returns a new Puller struct
//	func NewPullerWith(o PullerOpts)            - Returns a new Puller struct with explicit options
//
// Once you have a Puller, then:
//
//	func (p *Puller) PullTar(dest string)                         - Pulls an image to a tarfile
//	func (p *Puller) PullManifest(mpt ManifestPullType)           - Pulls an image manifest or manifest list and returns it
//	func (p *Puller) HeadManifest()                               - Heads an image manifest or manifest list and returns it
//	func (p *Puller) PullBlobs(mh ManifestHolder, blobDir string) - Pulls image blobs to a location on the filesystem
package imgpull
