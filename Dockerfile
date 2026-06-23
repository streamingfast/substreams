FROM --platform=$BUILDPLATFORM golang:1.26-bookworm AS build

WORKDIR /src

ARG TARGETOS TARGETARCH VERSION=dev

# Download modules first (leverages cache; uses go.sum only).
# GOWORK=off is critical: the committed go.work references an absolute
# local path to services-control-plane (for dev workspace). In a Docker
# context that path is unavailable, and the RO bind mount prevents writes
# to go.work.sum when Go resolves/updates the workspace graph.
RUN --mount=target=. \
      --mount=type=cache,target=/go/pkg/mod \
      --mount=type=cache,target=/root/.cache/go-build \
      GOWORK=off go mod download

# Build for the target OS/arch. GOWORK=off + committed go.sum means no
# attempt to write go.work.sum on the read-only mount. ldflags quoting
# hardened for VERSION values containing spaces/parentheses.
RUN --mount=target=. \
      --mount=type=cache,target=/go/pkg/mod \
      --mount=type=cache,target=/root/.cache/go-build \
      CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
      GOWORK=off go build -ldflags="-X 'main.version=${VERSION}'" -o /app/substreams ./cmd/substreams

FROM ubuntu:24.04

RUN DEBIAN_FRONTEND=noninteractive apt-get update && \
      apt-get -y install -y ca-certificates libssl3

COPY --from=build /app/substreams /app/substreams

ENTRYPOINT ["/app/substreams"]
