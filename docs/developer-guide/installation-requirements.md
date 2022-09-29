---
description: StreamingFast Substreams dependency installation
---

# Dependency Installation

### Dependencies Overview

Working with Substreams requires a few applications and tools: the `substreams` CLI, Rust, `buf` and `protoc-gen-prost`.

Instructions and links are provided below to assist with the installation of the required dependencies.

{% hint style="success" %}
See [below](installation-requirements.md#cloud-based-gitpod-installation) for cloud-based Gitpod installation
{% endhint %}

## Local installation

### `substreams` CLI Installation

The CLI is required and is essentially the user interface for working with Substreams.

> See the [`substreams` installation page](../getting-started/installing-the-cli.md) for instructions.

### Rust Installation

Developing Substreams modules requires a working [Rust](https://www.rust-lang.org/) compilation environment.

There are [several ways to install Rust](https://www.rust-lang.org/tools/install), but for the sake of simplicity using `curl` from the terminal is the quickest and easiest.

```bash
curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh
source $HOME/.cargo/env # to configure your current shell
```

### `buf` Installation

Buf simplifies the generation of typed structures in any language.

Buf invokes `protoc` and simplifies the process of working with Substreams. Visit the [Buf website](https://buf.build/) for additional information and [installation instructions](https://docs.buf.build/installation).

{% hint style="info" %}
_Note:_ [_Substreams packages_](../reference-and-specs/packages.md) _are compatible with_ [_Buf images_](https://docs.buf.build/reference/images)_._
{% endhint %}

macOS users can simply install Buf using Homebrew:

```bash
$ brew install bufbuild/buf/buf
```

### `protoc-gen-prost` Installation

The `protoc-gen-prost` crate is used to generate protobuf files. Once Rust is installed, install `protoc-gen-prost` using `cargo` with the following command:

```bash
$ cargo install protoc-gen-prost
```

{% hint style="warning" %}
Linux-based machines require `cmake` and `build-essential` to install the `protoc-gen-prost` cargo crate.

#### CMake

Visit the [Installing CMake page](https://cmake.org/install/) for further information on `cmake`.

#### Build Essential

Find additional information for `build-essential` on the [Build Essential Package page](https://itsfoss.com/build-essential-ubuntu/).

Run the following commands to install build-essential.

```
apt update
apt install cmake build-essential
```
{% endhint %}

## Cloud-based environment with Gitpod

[Gitpod](https://www.gitpod.io/) can be used in place of a local installation on a developer's machine.

To use Gitpod with Substreams:

1. First, [copy this repository](https://github.com/streamingfast/substreams-template/generate)
2. Obtain a StreamingFast key from [https://app.streamingfast.io/](https://app.streamingfast.io/)
3. Create a [Gitpod](https://gitpod.io/) account
4. Configure a `STREAMINGFAST_KEY` variable in the [Gitpod account settings](https://gitpod.io/variables)
5. Open the repository copied in step 1 as a [Gitpod workspace](https://gitpod.io/workspaces).
6. The substreams template comes with a `Makefile` that makes building and running the substream easy:
   1. `make build` will rebuild the substream. Run this whenever changes have been made.
   2. `make stream` will run the stream for a few blocks. As changes are made to the Substream, edit `Makefile` to change the `substreams` invocation to meet your needs.
