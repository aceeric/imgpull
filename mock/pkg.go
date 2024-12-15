// Package mock runs an OCI distribution server that only allows pulling and
// only serves docker.io/hello-world:latest. The server supports getting both
// docker.io/library/hello-world:latest as well as docker.io/hello-world:latest.
// The server supports basic auth, 1-way TLS, and mTLS.

package mock
