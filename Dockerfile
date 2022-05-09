# syntax=docker/dockerfile:1.2

FROM rust:1.60-bullseye as rust

FROM ubuntu:20.04

RUN DEBIAN_FRONTEND=noninteractive apt-get update && \
    apt-get -y install -y \
    ca-certificates libssl1.1 vim htop iotop sysstat \
    dstat strace lsof curl jq tzdata && \
    rm -rf /var/cache/apt /var/lib/apt/lists/*

RUN rm /etc/localtime && ln -snf /usr/share/zoneinfo/America/Montreal /etc/localtime && dpkg-reconfigure -f noninteractive tzdata

ADD /substreams /app/substreams
COPY --from=rust /usr/local/cargo /usr/local/cargo/

ENV PATH "$PATH:/usr/local/cargo/bin"

ENTRYPOINT /app/substreams
