// Package mock runs an OCI distribution server that only allows pulling and
// only serves docker.io/hello-world:latest. The server supports getting both
// docker.io/library/hello-world:latest as well as docker.io/hello-world:latest.
// The server supports basic and bearer auth, 1-way TLS, and mTLS.
//
// There are some things the mock server doesn't do because they don't really
// enhance testing of the image puller and at the end of the day any server in the
// wild will do this.
//
//  1. Doesn't validate the bearer token
//  2. Doesn't validate the basic auth credentials
//  3. Doesn't validate the client certs in mTLS - only requests them
package mock
