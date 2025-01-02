// Package tar supports creating an image tarball from blobs and image
// metadata. The resulting tar should be able to be imported into, for
// example, a docker registry with:
//
//	docker load --input <output of this package>
package tar
