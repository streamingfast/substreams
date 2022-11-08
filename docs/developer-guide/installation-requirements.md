---
description: StreamingFast Substreams dependency installation
---

# Dependency Installation

## Dependencies Overview

Working with Substreams requires a few applications and tools. The full list is available in the Sustreams [prerequisites](../getting-started/prerequisites.md).&#x20;

Instructions and links are provided below to assist with the installation of the required dependencies for Substreams.

{% hint style="success" %}
Instructions are provided [below](installation-requirements.md#cloud-based-gitpod-installation) for cloud-based Gitpod setups.
{% endhint %}

### Local installation

### `substreams` CLI Installation

The CLI is required and is the user interface for working with Substreams.

{% hint style="success" %}
**Tip**: _See the_ [_`substreams` installation page_](../getting-started/installing-the-cli.md) _for full setup instructions._
{% endhint %}

### Rust Installation

Developing Substreams modules requires a working [Rust](https://www.rust-lang.org/) compilation environment.

There are [several ways to install Rust](https://www.rust-lang.org/tools/install)**.**  Using `curl` from the terminal is the quickest and easiest.

```bash
curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh
source $HOME/.cargo/env # to configure your current shell
```

### Buf Installation

Buf simplifies the generation of typed structures in any language.

Buf invokes `protoc` and simplifies the process of working with Substreams.&#x20;

_Visit the_ [_Buf website_](https://buf.build/) _for additional information and_ [_installation instructions_](https://docs.buf.build/installation)_._

{% hint style="success" %}
**Tip**_:_ [_Substreams packages_](../reference-and-specs/packages.md) _are compatible with_ [_Buf images_](https://docs.buf.build/reference/images)_._
{% endhint %}

### `protoc-gen-prost` Installation

The `protoc-gen-prost` Rust crate is used to generate protobuf files.&#x20;

Once Rust is installed install `protoc-gen-prost` through `cargo` using the following command.

```bash
$ cargo install protoc-gen-prost
```

### Linux Specific Tools

Linux-based machines require CMake and `build-essential` to install the `protoc-gen-prost` cargo crate.

#### CMake

Find additional information and installation instructions for CMake on the official  [Installing CMake page](https://cmake.org/install/).

#### Build Essential

Find additional information and installation instructions for `build-essential` on the official [Build Essential Package page](https://itsfoss.com/build-essential-ubuntu/).

## Cloud-based environment with Gitpod

[Gitpod](https://www.gitpod.io/) can be used on a developer's machine in place of a local Substreams installation.

To use Gitpod with Substreams:

1. Copy the [substreams-template repository](https://github.com/streamingfast/substreams-template/generate)
2. Obtain a StreamingFast authentication key from [https://app.streamingfast.io/](https://app.streamingfast.io/)
3. Create a [Gitpod](https://gitpod.io/) account
4. Configure a `STREAMINGFAST_KEY` variable in the [Gitpod account settings](https://gitpod.io/variables)
5. Open the repository as a [Gitpod workspace](https://gitpod.io/workspaces)
6. The substreams template comes with a `Makefile` that makes building and running the substream easy:
   1. `make build` will rebuild the substream. Run this whenever changes have been made.
   2. `make stream` will run the stream for a few blocks. As changes are made to the Substream, edit `Makefile` to change the `substreams` invocation to meet your needs.
