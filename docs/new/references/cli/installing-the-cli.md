---
description: StreamingFast Substreams CLI installation documentation
---

## Install the `substreams` CLI

Used for connecting to endpoints, streaming data in real time, and packaging custom modules.

### Homebrew installation

```
brew install streamingfast/tap/substreams
```

### Docker Alias

You can use our published Substreams CLI Docker image and assign an alias to Docker. We mount the API token as `SF_API_TOKEN` in the alias so that credentials are known to the CLI running inside Docker.

```bash
alias substreams='docker run --rm -it -e="SF_API_TOKEN=$SF_API_TOKEN" ghcr.io/streamingfast/substreams'
```

{% hint style="note" %}
**Note**: Expansion of `$SF_API_TOKEN` above happens at command runtime, so you must ensure that it is set correctly in your own host environment.
{% endhint %}

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

Run the [`substreams` CLI](./command-line-interface.md) passing the `--version` flag to check the success of the installation.

```bash
substreams --version
```

A successful installation will print the version that you have installed.

```bash
substreams version dev
```

## Install Other Developer Dependencies (Only for Substreams Developers)

If you plan to build your own Substreams (i.e. write Rust code to extract data from the blockchain), you will need several dependencies to set up your developer environment:

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
**Note**_:_ [Substreams packages](../substreams-components/packages.md) and [Buf images](https://docs.buf.build/reference/images) are compatible.
{% endhint %}
