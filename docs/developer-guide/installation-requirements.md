---
description: StreamingFast Substreams dependency installation
---

# Dependency Installation

### Dependencies Overview

Working with Substreams requires a few additional applications and tools.

* Substreams CLI
* Rust
* Buf
* `cmake`
* `build-essential`
* `protoc-gen-prost`

Instructions and links are provided below to assist with the installation of the required dependencies.

#### Gitpod Substreams Installations

Substreams can be installed locally or in the cloud using Gitpod. Gitpod [installation instructions](installation-requirements.md#cloud-based-gitpod-installation) for Substreams are available at the bottom of this page. Continue reading for local installation instructions.

### CLI Installation

The CLI is essentially the user interface for working with Substreams and is required.  Follow the steps outlined on the [installation page](../getting-started/installing-the-cli.md) for further information before proceeding.

### Rust Installation

Substreams is written in the [Rust programming language](https://www.rust-lang.org/). Working with Substreams requires a working Rust installation.

There are [several ways to install Rust](https://www.rust-lang.org/tools/install), but for the sake of simplicity using `curl` from the terminal is the quickest and easiest.

```bash
curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh
source $HOME/.cargo/env # to configure your current shell
```

### Buf Installation

Buf simplifies the generation of typed structures in any language.

Buf invokes `protoc` and simplifies the process of working with Substreams.&#x20;

_Note, Substreams packages are compatible with_ [_Buf images_](https://docs.buf.build/reference/images)_._

Visit the [Buf website](https://buf.build/) for additional information and Buf [installation instructions](https://docs.buf.build/installation).

macOS users can simply install Buf with Homebrew.

```bash
brew install bufbuild/buf/buf
```

macOS users can continue to [install `protoc-gen-prost`](installation-requirements.md#protoc-gen-prost).&#x20;

Linux users have a few additional dependencies to install.

### Linux Specific Tools Installation

Linux-based machines require `cmake` and `build-essential` to install the `protoc-gen-prost` cargo crate.

Visit the [Installing CMake page](https://cmake.org/install/) for further information on `cmake`.

Find additional information for `build-essential` on the [Build Essential Package page](https://itsfoss.com/build-essential-ubuntu/).

Run the following commands to install build-essential.

```
apt update
apt install cmake build-essential
```

### `protoc-gen-prost`

Once `cmake` and `build-essential` are properly installed, the `protoc-gen-prost` crate can be used to generate protobuf files.

```
cargo install protoc-gen-prost
```

### Cloud-based Gitpod Installation

{% hint style="success" %}
**Develop in the cloud with Gitpod**

[Gitpod](https://www.gitpod.io/) can be used in place of a local installation on a developer's machine.

To use Gitpod with Substreams:

1. First, [copy this repository](https://github.com/streamingfast/substreams-template/generate)
2. Grab a StreamingFast key from [https://app.dfuse.io/](https://app.dfuse.io/)
3. Create a [Gitpod](https://gitpod.io/) account
4. Configure a `STREAMINGFAST_KEY` variable in your [Gitpod account settings](https://gitpod.io/variables)
5. Open the repository copied in step 1 as a [Gitpod workspace](https://gitpod.io/workspaces).
6. The substream template comes with a `Makefile` that makes building and running the substream easy:
   1. `make build` will rebuild the substream. Run this whenever changes have been made.
   2. `make stream` will run the stream for a few blocks. As you make changes to your substream, you'll want to change this command to use your own substream modules and a block range more suitable to the data your indexing. Simply edit `Makefile` to do this.
{% endhint %}
