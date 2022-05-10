# syntax=docker/dockerfile:1.2

FROM ubuntu:20.04

RUN DEBIAN_FRONTEND=noninteractive apt-get update && \
    apt-get -y install -y \
    libssl-dev pkg-config protobuf-compiler \
    ca-certificates libssl1.1 vim htop iotop sysstat \
    dstat strace lsof curl jq tzdata && \
    rm -rf /var/cache/apt /var/lib/apt/lists/*

RUN rm /etc/localtime && ln -snf /usr/share/zoneinfo/America/Montreal /etc/localtime && dpkg-reconfigure -f noninteractive tzdata

# How could we optimize that in a separate builder?
RUN curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh && rustup target install wasm32-unknown-unknown

ADD /substreams /app/substreams

ENV PATH "$PATH:/app"

ENTRYPOINT /app/substreams
