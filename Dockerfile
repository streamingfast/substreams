FROM --platform=$BUILDPLATFORM golang:1.25-bookworm AS build

WORKDIR /src

ARG TARGETOS TARGETARCH VERSION=dev

RUN --mount=target=. \
      --mount=type=cache,target=/root/.cache/go-build \
      --mount=type=cache,target=/go/pkg \
      GOOS=$TARGETOS GOARCH=$TARGETARCH go build -ldflags "-X \"main.version=$VERSION\"" -o /app/substreams ./cmd/substreams

FROM ubuntu:24.04

RUN DEBIAN_FRONTEND=noninteractive apt-get update && \
      apt-get -y install -y ca-certificates libssl3

COPY --from=build /app/substreams /app/substreams

ENTRYPOINT ["/app/substreams"]
