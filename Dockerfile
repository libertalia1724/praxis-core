# Build stage
FROM golang:1.26-alpine AS build-env

# Install build dependencies. build-base (gcc/g++) is required because wasmvm is a cgo
# dependency; curl and file are used for fetching/verifying the static wasmvm library.
RUN apk add --no-cache make git libc-dev bash gcc linux-headers build-base ca-certificates curl file

WORKDIR /go/src/github.com/libertalia1724/praxis-core

# Cache dependencies
COPY go.mod go.sum ./
RUN go mod download

# Static (musl) libwasmvm. The version is read from go.mod so it always matches the Go
# bindings, and the download is verified against the checksums published with the release.
RUN set -eux; \
    WASMVM_VERSION="$(go list -m -f '{{.Version}}' github.com/CosmWasm/wasmvm/v3)"; \
    ARCH="$(uname -m)"; \
    BASE_URL="https://github.com/CosmWasm/wasmvm/releases/download/${WASMVM_VERSION}"; \
    curl -fsSL -o "/lib/libwasmvm_muslc.${ARCH}.a" "${BASE_URL}/libwasmvm_muslc.${ARCH}.a"; \
    curl -fsSL -o /tmp/checksums.txt "${BASE_URL}/checksums.txt"; \
    cd /lib && grep "libwasmvm_muslc.${ARCH}.a" /tmp/checksums.txt | sha256sum -c -

# Copy source and build
COPY . .

# Declared without defaults so BuildKit supplies the build platform. Hardcoding a
# GOARCH here cross-compiles for that architecture regardless of the host, and the
# resulting binary then runs under emulation. Emulated x86 mis-executes the AVX2
# chacha20poly1305 assembly, which breaks the CometBFT P2P handshake and leaves
# localnet nodes unable to peer with each other.
ARG TARGETOS
ARG TARGETARCH

# cgo is mandatory for wasmvm. The muslc tag makes it link against libwasmvm_muslc.<arch>.a
# and the external linker flags produce a fully static binary that runs on the alpine image below.
RUN CGO_ENABLED=1 GOOS=$TARGETOS GOARCH=$TARGETARCH go build \
        -tags "netgo,muslc" \
        -ldflags '-linkmode=external -extldflags "-Wl,-z,muldefs -static"' \
        -o /go/bin/praxisd ./praxisd \
    && file /go/bin/praxisd | grep "statically linked"

FROM alpine:3

RUN apk add --no-cache bash jq sed curl

COPY --from=build-env /go/bin/praxisd /usr/bin/praxisd

EXPOSE 26656 26657 1317 9090

WORKDIR /root

COPY scripts/localnet/wrapper.sh /usr/bin/wrapper.sh
RUN chmod +x /usr/bin/wrapper.sh

ENTRYPOINT []
CMD ["praxisd", "start"]
