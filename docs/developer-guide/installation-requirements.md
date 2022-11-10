---
description: StreamingFast Substreams dependency installation
---

# Dependency Installation

## Dependencies Overview

Working with Substreams requires a number of different applications and tools. A full list is provided on the Substreams [prerequisites](../getting-started/prerequisites.md) page.&#x20;

Instructions and links are provided below to assist with the installation of the required dependencies for Substreams.

{% hint style="success" %}
**Tip**: Instructions are provided [below](installation-requirements.md#cloud-based-gitpod-installation) for cloud-based Gitpod setups.
{% endhint %}

## Local installation

### `substreams` CLI Installation

The CLI is required and is the primary user interface for working with Substreams.

{% hint style="success" %}
**Tip**: Full setup instructions are available on the [installing the Substreams CLI](../getting-started/installing-the-cli.md) page_._
{% endhint %}

### Rust Installation

Developing Substreams modules requires a working [Rust](https://www.rust-lang.org/) compilation environment.

There are [several ways to install Rust](https://www.rust-lang.org/tools/install)**.**  Using `curl` from the terminal is the quickest and easiest.

```bash
curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh
source $HOME/.cargo/env # to configure your current shell
```

### Buf Installation

Buf simplifies the generation of typed structures in any language. Buf works with remote builder executed on Buf server, so an internet connection is required when generating Rust bindings from Protobuf definitions.&#x20;

Visit the [Buf website](https://buf.build/) for additional information and [installation instructions](https://docs.buf.build/installation).

{% hint style="info" %}
**Note**_:_ [_Substreams packages_](../reference-and-specs/packages.md) _are compatible with_ [_Buf images_](https://docs.buf.build/reference/images)_._
{% endhint %}

## Cloud-based environment with Gitpod

Follow the steps below to use [Gitpod](https://www.gitpod.io/) with Substreams.

1. Copy the [substreams-template repository](https://github.com/streamingfast/substreams-template/generate).
2. Obtain a StreamingFast authentication key from: [https://app.streamingfast.io/](https://app.streamingfast.io/).
3. Create a [Gitpod](https://gitpod.io/) account.
4. Configure a `STREAMINGFAST_KEY` variable in the [Gitpod account settings](https://gitpod.io/variables).
5. Open the repository as a [Gitpod workspace](https://gitpod.io/workspaces).
6. The Substreams Template includes a `Makefile` simplifying the installation process.
   1. Running `make build` rebuilds the Substreams implementation. This should be done after making changes to the code.
   2. `make stream` runs the stream for a few blocks. \
      Edit `Makefile` to change the invocation as changes are made to the Substreams implementation.
