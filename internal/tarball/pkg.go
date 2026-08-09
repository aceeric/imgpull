// Package tarball provides mechanism-only read access to image tarballs -
// both the docker-save-shaped format (produced by `docker save`, and by
// this project's own internal/tar.ToTar) and genuine OCI-layout tarballs
// (produced by `ctr image export`, buildkit's `type=oci` output, `crane
// ... --format oci`), optionally gzip-compressed (.tar.gz/.tgz).
//
// This package deliberately knows nothing about pkg/imgpull.ManifestHolder
// - it yields plain ManifestEntry values (raw manifest bytes, digest,
// media type, ref) rather than constructing a ManifestHolder itself. That
// split exists because ManifestHolder lives in pkg/imgpull, which this
// package would otherwise need to import; pkg/imgpull's own public
// OpenImageTarBall/TarManifestReader/TarBlobReader (in that package) do
// the conversion into ManifestHolder as a thin final step, keeping the
// dependency direction one-way (pkg/imgpull -> internal/tarball, never the
// reverse) - the same shape internal/tar already uses for the write side
// (ManifestHolder.newImageTarball converts INTO a plain tar.ImageTarball
// before internal/tar.ToTar ever sees it; this package is that mirrored on
// the read side).
package tarball
