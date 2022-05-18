Substreams - A streaming data engine for The Graph - by StreamingFast
=====================================================================

DEVELOPER PREVIEW OF SUBSTREAMS

Substreams is a powerful blockchain indexing technology, developed for The Graph Network.

It enables you to write Rust modules, composing data streams alongside
the community, and provides extremely high performance indexing by
virtue of parallelization, in a streaming first fashion.

It has all the benefits of the Firehose, like low cost caching and
archiving of blockchain data, high throughput processing, and
cursor-based reorgs handling.

Substreams is the successor of
https://github.com/streamingfast/sparkle, enabling greater
composability, yet similar powers of parallelization, and a much
simpler model to work with.


## Documentation

Visit the [Documentation](./docs) section for details.


## Install the `substreams` command-line tool

The `substreams` CLI allows you to interact with Substreams endpoints,
stream data in real-time, as well as package your own Substreams modules.

1. Get a [release](https://github.com/streamingfast/substreams/releases).

<!--
2. Or build from source quickly:

```
go install github.com/streamingfast/substreams/cmd/substreams@latest
```
-->

2. Or build from source and start hacking:

```
git clone git@github.com:streamingfast/substreams
cd substreams
go install -v ./cmd/substreams
```


## Usage

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


## Packages

Substreams packages, ending in `.spkg` contain all what is needed to
start streaming from Substreams endpoints from client consumers in any
language supported by Protobuf.

They contain compiled WASM code, dependent modules, documentation,
protobuf schemas, all in one file.

Build yourself a package using:

```
substreams pack ./substreams.yaml
```

See [sample packages here](https://github.com/streamingfast/substreams-playground/releases).



## Develop your own Substreams Modules


### Install rust

We're going to be using the [Rust programming language](https://www.rust-lang.org/), to develop Substreams Modules.

There are [several ways to install Rust](https://www.rust-lang.org/tools/install), but for the sake of brevity:

```bash
curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh
```


## Running locally

You can run the Substreams service locally this way:

1. Get a recent release of [the Ethereum Firehose](https://github.com/streamingfast/sf-ethereum), and install `sfeth`.

Alternatively, you can use this Docker image: `ghcr.io/streamingfast/sf-ethereum:6aa70ca`, known to work with version v0.0.5-beta of the `substreams` release herein.

2. Get some data (merged blocks) to play with locally (here on BSC mainnet):

```bash
# Downloads 2.6GB of data
sfeth tools download-from-firehose bsc-dev.streamingfast.io:443 6810000 6820000 ./localblocks
sfeth tools generate-irreversible-index ./localblocks ./localirr 6810000 6819700
```

3. Then run the `firehose` service locally in a terminal, reading blocks from your disk:

```bash
sfeth start firehose  --config-file=  --log-to-file=false  --common-blockstream-addr=  --common-blocks-store-url=./localdata --firehose-grpc-listen-addr=:9000* --substreams-enabled --substreams-rpc-endpoint=https://URL.POINTING.TO.A.BSC.ARCHIVE.NODE/if-you/want-to-use/eth_call/within/substreams
```

4. And then run the `substreams` command against your local deployment (checkout `substreams-playground` in the _Run remotely_ section above):

```bash
substreams run -k -e localhost:9000  # ...
```


## Community

Need any help? Reach out!

* [StreamingFast Discord](https://discord.gg/jZwqxJAvRs)
* [The Graph Discord](https://discord.gg/vtvv7FP)


## License

[Apache 2.0](LICENSE)
