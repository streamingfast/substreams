# Installing Dependencies

{% hint style="success" %}
#### Develop in the cloud with Gitpod

Optionally, and instead of installing dependencies locally, you can use [Gitpod](https://www.gitpod.io/) to launch a developer environment purely in the cloud, through your browser:

1. First, [copy this repository](https://github.com/streamingfast/substreams-template/generate)
2. Grab a StreamingFast key from [https://app.dfuse.io/](https://app.dfuse.io/)
3. Create a [Gitpod](https://gitpod.io/) account
4. Configure a `STREAMINGFAST_KEY` variable in your [Gitpod account settings](https://gitpod.io/variables)
5. Open your repository as a [Gitpod workspace](https://gitpod.io/workspaces)
6. The substream template comes with a `Makefile` that makes building and running the substream easy:
   1. `make build` will rebuild your substream. Run this whenever you have made changes.
   2. `make stream` will run the stream for a few blocks. As you make changes to your substream, you'll want to change this command to use your own substream modules and a block range more suitable to the data your indexing. Simply edit `Makefile` to do this.
{% endhint %}

### Install the `substreams` CLI

If you haven't already, make sure that you [install the `substreams` command-line interface](../getting-started/installing-the-cli.md).

### Install Rust

Before we start creating any Substreams, we will need to setup our development environment. Substreams are written in the [Rust programming language](https://www.rust-lang.org/).

There are [several ways to install Rust](https://www.rust-lang.org/tools/install), but for the sake of brevity, this is the easiest:

```bash
curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh
source $HOME/.cargo/env # to configure your current shell
```

### Install `buf`

[https://buf.build](https://buf.build) is a tool used to simplify the generation of typed structures in any language. It invokes `protoc` and simplifies a good number of things. Substreams packages are compatible with [buf Images](https://docs.buf.build/reference/images).

See the [installation instructions here](https://docs.buf.build/installation).
