# Change Log

## 27-Sep-2025
Tag: `v1.12.2`

1. Bump go version from `1.24.5` to `1.25.1`

## 13-Aug-2025
Tag: `v1.12.1`

1. Perform auth using `/v2/../manifests/...` endpoint. Previously used the `/v2/` endpoint but `ghcr.io` doesn't support that resulting in 401. This version successfully pulls:
   ```
   imgpull quay.io/cilium/alpine-curl:v1.10.0            quay.io-cilium-alpine-curl-v1.10.0.tar
   imgpull registry.k8s.io/kube-apiserver:v1.31.0        registry.k8s.io-kube-apiserver-v1.31.0.tar
   imgpull docker.io/alpine:3.22.1                       docker.io-alpine-3.22.1.tar
   imgpull ghcr.io/cloudnative-pg/cloudnative-pg:1.26.1  ghcr.io-cloudnative-pg-cloudnative-pg-1.26.1.tar
   ```
2. Bump go version from `1.23.3` to `1.24.5`
