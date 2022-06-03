# syntax=docker/dockerfile:1.2

FROM rust:1.60-bullseye as rust

FROM ubuntu:20.04

RUN DEBIAN_FRONTEND=noninteractive apt-get update && \
    DEBIAN_FRONTEND=noninteractive apt-get -y install \
    gcc libssl-dev pkg-config protobuf-compiler \
    ca-certificates libssl1.1 vim strace lsof curl jq && \
    rm -rf /var/cache/apt /var/lib/apt/lists/*

ENV RUSTUP_HOME=/usr/local/rustup \
    CARGO_HOME=/usr/local/cargo \
    PATH=/usr/local/cargo/bin:$PATH \
    RUST_VERSION=1.60.0

COPY --from=rust /usr/local/cargo /usr/local/cargo/
COPY --from=rust /usr/local/rustup /usr/local/rustup/

# The `cargo install rustfmt || true` part serves the purposes of updating the crate registry, it's really
# hard to update the registry standalone without a package, so we take a detour by installing a component
# that will requires to update the crate registry
RUN rustup target install wasm32-unknown-unknown && rustup component add rustfmt && cargo install rustfmt || true

ADD /substreams /app/substreams

# ENV PATH "/app:$HOME/.cargo/bin:$PATH"
ENV PATH "/app:/usr/local/cargo/bin:$PATH"

ENTRYPOINT ["/app/substreams"]
