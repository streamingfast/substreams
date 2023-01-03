---
description: StreamingFast Substreams dependency installation
---

# Dependency installation

## Dependencies overview

Substreams requires a number of different applications and tools. Instructions and links are provided to assist in the installation of the required dependencies for Substreams.

{% hint style="success" %}
**Tip**: Instructions are also provided for cloud-based Gitpod setups.
{% endhint %}

## Local installation

### `substreams` CLI installation

The [`substreams` CLI](../reference-and-specs/command-line-interface.md) is required and is the primary Substreams user interface.

{% hint style="success" %}
**Tip**: Full setup instructions are available on the [installing the `substreams` CLI](../getting-started/installing-the-cli.md) page.
{% endhint %}

### Rust installation

Developing Substreams modules requires a working [Rust](https://www.rust-lang.org/) compilation environment.

There are [several ways to install Rust](https://www.rust-lang.org/tools/install)**.**  Install Rust through `curl` by using:

```bash
curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh
source $HOME/.cargo/env # to configure your current shell
```

### Buf installation

Buf simplifies the generation of typed structures in any language. Buf uses a remote builder executed on the Buf server, so an internet connection is required to generate Rust bindings from Protobuf definitions.

Visit the [Buf website](https://buf.build/) for additional information and [installation instructions](https://docs.buf.build/installation).

{% hint style="info" %}
**Note**_:_ [Substreams packages](../reference-and-specs/packages.md) and [Buf images](https://docs.buf.build/reference/images) are compatible.
{% endhint %}

## Gitpod cloud-based environment

Follow the steps to use [Gitpod](https://www.gitpod.io/) for Substreams:

1. Copy the [substreams-template repository](https://github.com/streamingfast/substreams-template/generate).
2. Obtain a StreamingFast authentication key from: [https://app.streamingfast.io/](https://app.streamingfast.io/).
3. Create a [Gitpod](https://gitpod.io/) account.
4. Configure a `STREAMINGFAST_KEY` variable in the [Gitpod account settings](https://gitpod.io/variables).
5. Open the repository as a [Gitpod workspace](https://gitpod.io/workspaces).
6. The Substreams Template includes a `Makefile` simplifying the installation process.
   1. Running `make build` rebuilds the Substreams module. Run the command **after making changes to the code**.
   2. `make stream` runs the stream for a few blocks.\
      Edit `Makefile` to change the invocation as updates are made to the Substreams module.
