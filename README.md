# Simple Container Image Puller

This is a small project that **only** pulls OCI container images. The project was created as an exercise to understand the bare minimum required amount of code required for a feature-capable and robust image puller.

The project builds a CLI that pulls images. It supports pulling from mirrors and pullthrough registries via a `--ns` (namespace) arg. Details are presented below.

## Quick Start

After git cloning the project, build the CLI:
```
make imgpull
```

This creates the binary `imgpull` in the `bin` directory. Verify:
```
bin/imgpull --version
```

Success looks like (date and version will differ):
```
imgpull version: 1.1.0 build date: 2024-12-14T00:37:34.83Z
```

## Usage

Two positional parameters are required: 1) an image reference and, 2) a tar file:
```
bin/imgpull [image ref] [tar file]
```

Example:
```
bin/imgpull docker.io/hello-world:latest hello-world-latest.tar
```

## Options

The following command-line options are supported:

**`-o|--os [os]`**

Specifies the operating system. If omitted, the code uses `runtime.GOOS` to determine your OS and will pull the image for that OS.

Example:
```
bin/imgpull docker.io/hello-world:latest hello-world-latest.tar --os linux
```

**`-a|--arch [arch]`**

Specifies the architecture. If omitted, the code uses `runtime.GOARCH` to determine your architecture and will pull the image for that OS.

Example:
```
bin/imgpull docker.io/hello-world:latest hello-world-latest.tar --os linux --arch amd64
```

**`-n|--ns [namespace]`**

Specifies a namespace. This supports pulling from mirroring / pull-through registries and is intended to pull the same way `containerd` does when you've configured `containerd` with a mirror.

Example:
```
bin/imgpull localhost:8080/curl/curl:8.10.1 quay.io-curl-8.1.0.1.tar --ns quay.io
```

In this scenario, there is a pull-through or mirroring registry running on `localhost:8080`. When the image puller interacts with the registry via the V2 OCI Distribution Server API, it will append this namespace to the REST API calls as a query param. In other words, internally the CLI makes calls like:
```
https://localhost:8080/v2/curl/manifests/8.10.1?ns=quay.io
```

Note the query param: `?ns=quay.io`. The pull-through mirroring registry running on `localhost:8080` will check to see if `quay.io/curl/curl:8.10.1` is cached and if not, will pull and cache from the upstream `quay.io` registry. (Or the mirror will fail it only supports mirroring, and the desired image is not already cached.)

For a highly configurable caching pull-through registry that supports this functionality, see https://github.com/aceeric/ociregistry.

**`-u|--user [username]` `-p|--password [password]`**

Specifies the username and password for basic auth.

Example:
```
bin/imgpull docker.io/hello-world:latest hello-world-latest.tar\
  --user jqpubli --password mypass
```
**`-s|--scheme [scheme]`**

Specifies the scheme. The CLI defaults to `https`.

Example:
```
bin/imgpull my.inhouse.http.registry/hello-world:latest hello-world-latest.tar --scheme http
```

This example pulls from a registry over http, rather than https.

**`-c|--cert [tls cert]` `-k|--key [tls key]`**

Configures mTLS. By default, the CLI performs 1-way TLS using the OS trust store. You use these options to configure mTLS. The option values are paths to a PEM-encoded cert and key.

Example:
```
bin/imgpull my.private.mtls.registry/hello-world:latest hello-world-latest.tar\
  --cert /path/to/cert.pem --key /path/to/key.pem
```

**`-x|--cacert [tls ca cert]`**

Configures verification of the server cert using a specified CA. This supports cases where the server presents certs that are not signed by the OS trust store. The option value is a path to a PEM-encoded CA or CA bundle.

Example:
```
bin/imgpull my.private.mtls.registry/hello-world:latest hello-world-latest.tar\
  --ca /path/to/ca.pem
```

**`-i|--insecure`**

Does not verify the server certs presented by the OCI distribution server if the connection to the server is over HTTPS (the default.) Use this option when the registry presents certs that are not signed by OS trust store, and you are unable to provide a CA using the `--cacert` option.

Example:
```
bin/imgpull my.private.registry/hello-world:latest hello-world-latest.tar --insecure
```

**`--parsed`**

Developer troubleshooting option. Prints the parsed command line and exits.

Example:
```
bin/imgpull my.inhouse.http.registry/hello-world:latest hello-world-latest.tar --cacert /path/to/ca.pem --parsed
```

**`-v|--version`**

Prints the version and exits.

Example:
```
bin/imgpull --version
```

**`-h|--help`**

Prints the help and exits.

Example:
```
bin/imgpull --help
```

## Examples

```
bin/imgpull docker.io/hello-world:latest ./hello-world.tar -o linux -a amd64
```

Pulls the image for linux/amd64 to hello-world.tar in the working directory.
