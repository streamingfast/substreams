# Substreams

DEVELOPER PREVIEW OF SUBSTREAMS

Substreams is a powerful blockchain indexing technology, developed for The Graph Network.

It enables you to write Rust modules, composing data streams alongside the community, and provides extremely high performance indexing by virtue of parallelization, in a streaming first fashion.

It has all the benefits of the Firehose, like low cost caching and archiving of blockchain data, high throughput processing, and cursor-based reorgs handling.

Substreams is the successor of [https://github.com/streamingfast/sparkle](https://github.com/streamingfast/sparkle), enabling greater composability, yet similar powers of parallelization, and a much simpler model to work with.

## Getting Started

### Installing the `Substreams` command-line tool

The `substreams` CLI allows you to interact with Substreams endpoints, stream data in real-time, as well as package your own Substreams modules.

#### From brew (for Max OS)

```
brew install streamingfast/tap/substreams
```

#### From pre-compiled binary

Download the binary

```bash
# Use correct binary for your platform
wget https://github.com/streamingfast/substreams/releases/download/v0.0.5-beta3/substreams_0.0.5-beta3_linux_x86_64.tar.gz
tar -xzvf substreams_0.0.5-beta3_linux_x86_64.tar.gz
export PATH="`pwd`:$PATH"
```

{% hint style="info" %}
Check [https://github.com/streamingfast/substreams/releases](https://github.com/streamingfast/substreams/releases) and use the latest release available
{% endhint %}

#### From Source

```bash
git clone git@github.com:streamingfast/substreams
cd substreams
go install -v ./cmd/substreams
```

### Validation

Ensure that `substreams` CLI works as expected:

```bash
substreams -v
version 0.0.5-beta3 (Commit 61cc596, Built 2022-05-09T19:35:11Z)
```

### Ressources

* Checkout our [Getting Started Guide](docs/developer-guide/overview.md)
* Take a look at the [Subtreams Template](https://github.com/streamingfast/substreams-template) repository for a sample Substreams
* Take a look at the [Substreams Playground](https://github.com/streamingfast/substreams-playground) repository for more learnings and examples

## Community

Need any help? Reach out!

* [StreamingFast Discord](https://discord.gg/jZwqxJAvRs)
* [The Graph Discord](https://discord.gg/vtvv7FP)

## Licenses

[Apache 2.0](LICENSE/)
