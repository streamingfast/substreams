---
description: A streaming data engine for The Graph - by StreamingFast
---

# Substreams

DEVELOPER PREVIEW OF SUBSTREAMS

Substreams is a powerful blockchain indexing technology, developed for The Graph Network.

It enables you to write Rust modules, composing data streams alongside the community, and provides extremely high performance indexing by virtue of parallelization, in a streaming first fashion.

It has all the benefits of the Firehose, like low cost caching and archiving of blockchain data, high throughput processing, and cursor-based reorgs handling.

Substreams is the successor of [https://github.com/streamingfast/sparkle](https://github.com/streamingfast/sparkle), enabling greater composability, yet similar powers of parallelization, and a much simpler model to work with.

## Documentation

Visit the [Documentation](broken-reference) section for details.

## Getting Started

### Installing the `substreams` command-line tool

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



### Consuming

Get streaming right away using. To use StreamingFast's infrastructure, dump that somewhere like `.bashrc`:

```bash
export STREAMINGFAST_KEY=server_YOUR_KEY_HERE  # Ask us on Discord for a key
function sftoken {
    export FIREHOSE_API_TOKEN=$(curl https://auth.dfuse.io/v1/auth/issue -s --data-binary '{"api_key":"'$STREAMINGFAST_KEY'"}' | jq -r .token)
	export SUBSTREAMS_API_TOKEN=$FIREHOSE_API_TOKEN
    echo Token set on FIREHOSE_API_TOKEN and SUBSTREAMS_API_TOKEN
}
```

Then in your shell, load a key into an environment variable with:

```bash
sftoken
```

And run:

```
substreams run -e bsc-dev.streamingfast.io:443 \
   https://github.com/streamingfast/substreams-playground/releases/download/v0.5.0/pcs-v0.5.0.spkg \
   block_to_pairs,pairs,db_out \
   -s 6810706 -t 6810711
```

### Developing Substreams Modules

Install the [**Rust** programming language](https://www.rust-lang.org/). This is the language used to develop Substreams Modules.

There are [several ways to install Rust](https://www.rust-lang.org/tools/install), but for the sake of brevity:

```bash
curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh
```

## Community

Need any help? Reach out!

* [StreamingFast Discord](https://discord.gg/jZwqxJAvRs)
* [The Graph Discord](https://discord.gg/vtvv7FP)

## License

[Apache 2.0](LICENSE/)
