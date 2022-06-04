# Substreams

> Developer Preview

Substreams is a powerful blockchain indexing technology, developed for The Graph Network.

It enables you to write Rust modules, composing data streams alongside the community, and provides extremely high performance indexing by virtue of parallelization, in a streaming-first fashion.

It has all the benefits of the Firehose, like low-cost caching and archiving of blockchain data, high throughput processing, and cursor-based reorgs handling.

Substreams is the successor of [https://github.com/streamingfast/sparkle](https://github.com/streamingfast/sparkle). This iteration enables greater composability, provides similar powers of parallelization, and is a much simpler model to work with.

## Getting Started

### Installing the `Substreams` command-line tool

The `substreams` CLI allows you to interact with Substreams endpoints, stream data in real time, as well as package your own Substreams modules.

{% hint style="info" %}
Alternatively to installing the `substreams` locally, you can [use Gitpod to get started quickly](developer-guide/overview.md#gitpod-quick-start).
{% endhint %}

#### From brew (for Mac OS)

```
brew install streamingfast/tap/substreams
```

#### From pre-compiled binary

Download the binary

```bash
# Use correct binary for your platform
LINK=$(curl -s https://api.github.com/repos/streamingfast/substreams/releases/latest%7C awk '/download.url.*linux/ {print $2}' | sed 's/"//g')
curl -L  $LINK  | tar xf -
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
substreams --version
substreams version 0.0.12 (Commit 7b30088, Built 2022-06-03T18:32:00Z)
```

### Run your first stream

Jump into the docs, and [run your first stream](getting-started/your-first-stream.md).

## Resources

* Checkout our [Getting Started Guide](developer-guide/overview.md)
* Take a look at the [Substreams Template](https://github.com/streamingfast/substreams-template) repository for a sample Substreams
* Take a look at the [Substreams Playground](https://github.com/streamingfast/substreams-playground) repository for more learnings and examples

## Community

Need any help? Reach out!

* [StreamingFast Discord](https://discord.gg/jZwqxJAvRs)
* [The Graph Discord](https://discord.gg/vtvv7FP)

## License

[Apache 2.0](../LICENSE/)
