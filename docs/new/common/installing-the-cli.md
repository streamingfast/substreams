---
description: StreamingFast Substreams CLI installation documentation
---

# Installing the Substreams CLI

## Dependency installation

Substreams requires a number of different applications and tools. Instructions and links are provided to assist in the installation of the required dependencies for Substreams.

{% hint style="success" %}
**Tip**: Instructions are also provided for cloud-based Gitpod setups.
{% endhint %}

### Rust installation

Developing Substreams modules requires a working [Rust](https://www.rust-lang.org/) compilation environment.

There are [several ways to install Rust](https://www.rust-lang.org/tools/install)**.**  Install Rust through `curl` by using:

```bash
curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh
source $HOME/.cargo/env # to configure your current shell
```

#### `wasm32-unknown-unknown` target

Ensure you have the `wasm32-unknown-unknown` target installed on your Rust installation, if unsure, you can install it with:

```bash
rustup target add wasm32-unknown-unknown
```

### Buf installation

Buf simplifies the generation of typed structures in any language. Buf uses a remote builder executed on the Buf server, so an internet connection is required to generate Rust bindings from Protobuf definitions.

Visit the [Buf website](https://buf.build/) for additional information and [installation instructions](https://docs.buf.build/installation).

{% hint style="info" %}
**Note**_:_ [Substreams packages](../reference-and-specs/packages.md) and [Buf images](https://docs.buf.build/reference/images) are compatible.
{% endhint %}

## Install the `substreams` CLI

Used for connecting to endpoints, streaming data in real time, and packaging custom modules.

### Homebrew installation

```
brew install streamingfast/tap/substreams
```

### Pre-compiled binary installation

There are several CLI binaries available for different operating systems. Choose the correct platform in the [CLI releases page](https://github.com/streamingfast/substreams/releases).

If you are on MacOS, you can use the following command:

```bash
LINK=$(curl -s https://api.github.com/repos/streamingfast/substreams/releases/latest | awk "/download.url.*$(uname -s | tr '[:upper:]' '[:lower:]')_$(uname -m)/ {print \$2}" | sed 's/"//g')
curl -L  $LINK  | tar zxf -
```

If you are on Linux, you can use the following command:

```bash
# Use correct binary for your platform
LINK=$(curl -s https://api.github.com/repos/streamingfast/substreams/releases/latest | awk "/download.url.*linux_$(uname -m)/ {print \$2}" | sed 's/"//g')
curl -L  $LINK  | tar zxf -
```

### Installation from source

```bash
git clone https://github.com/streamingfast/substreams
cd substreams
go install -v ./cmd/substreams
```

{% hint style="warning" %}
**Important**: Add $HOME/go/bin to the system path if it's not already present.
{% endhint %}

## Validation of installation

Run the [`substreams` CLI](../reference-and-specs/command-line-interface.md) passing the `--version` flag to check the success of the installation.

```bash
substreams --version
```

A successful installation will print the version that you have installed.

```bash
substreams version dev
```

{% hint style="info" %}
**Note**: You can [also use Gitpod](../developers-guide/installation-requirements.md) instead of a local installation.
{% endhint %}