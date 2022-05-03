Substreams - A streaming data engine for The Graph - by StreamingFast
=====================================================================

DEVELOPER PREVIEW OF SUBSTREAMS

Think Fluvio for deterministic blockchain data.

The successor of https://github.com/streamingfast/sparkle, enabling greater composability, yet similar powers of parallelisation, and a much simpler model to work with.



Install client
--------------

This client will allow you to interact with Substreams endpoints, and stream data in real-time.x

Get a [release](https://github.com/streamingfast/substreams/releases).

From source:

```
git clone git@github.com:streamingfast/substreams
cd substreams
go install -v ./cmd/substreams
```

From source without checkout:

```
go install github.com/streamingfast/substreams/cmd/substreams@latest
```


Install dependencies to build Substreams
----------------------------------------

This will allow you to develop Substreams modules locally, and run them remotely.


### Install rust

We're going to be using the [Rust programming language](https://www.rust-lang.org/), to develop some custom logic.

There are [several ways to install Rust](https://www.rust-lang.org/tools/install), but for the sake of brevity:

```bash
curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh
```

### Install wasm-pack

Finally, [wasm-pack](https://rustwasm.github.io/wasm-pack/book/introduction.html) is a tool which allows us to work wi
th Rust-generated WASM code.

Their [quicktart docs can be found here](https://rustwasm.github.io/wasm-pack/book/quickstart.html), but again:

```bash
curl https://rustwasm.github.io/wasm-pack/installer/init.sh -sSf | sh
```


Run remotely
------------

Using StreamingFast's infrastructure


Dump that somewhere like `.bashrc`:
```bash
export STREAMINGFAST_KEY=server_YOUR_KEY_HERE  # Ask us on Discord for a key
function sftoken {
    export FIREHOSE_API_TOKEN=$(curl https://auth.dfuse.io/v1/auth/issue -s --data-binary '{"api_key":"'$STREAMINGFAST_KEY'"}' | jq -r .token)
	export SUBSTREAMS_API_TOKEN=$FIREHOSE_API_TOKEN
    echo Token set on FIREHOSE_API_TOKEN and SUBSTREAMS_API_TOKEN
}
```

Then in your shell, load a key in an env var with:

```bash
sftoken
```

Then, try to run the [PancakeSwap Substreams](https://github.com/streamingfast/substreams-playground)

> The below commands will be run from `substreams-playground`

```
cd ./pcs-rust/ && ./build.sh
cd ../eth-token/ && ./build.sh
cd ..
substreams run -e bsc-dev.streamingfast.io:443 ./pcs-rust/substreams.yaml pairs,block_to_pairs,volumes,totals,db_out -s 6810706 -t 6810711
```

Run locally
-----------

You can run the substreams service locally this way:

Get a recent release of [the Ethereum Firehose](https://github.com/streamingfast/sf-ethereum), and install `sfeth`.

Alternatively, you can use this Docker image: `ghcr.io/streamingfast/sf-ethereum:6aa70ca`, known to work with version v0.0.5-beta of the `substreams` release herein.

Get some data (merged blocks) to play with locally (here on BSC mainnet):

```bash
# Downloads 2.6GB of data
sfeth tools download-from-firehose bsc-dev.streamingfast.io:443 6810000 6820000 ./localblocks
sfeth tools generate-irreversible-index ./localblocks ./localirr 6810000 6819700
```

Then run the `firehose` service locally in a terminal, reading blocks from your disk:

```bash
sfeth start firehose  --config-file=  --log-to-file=false  --common-blockstream-addr=  --common-blocks-store-url=./localdata --firehose-grpc-listen-addr=:9000* --substreams-enabled --substreams-rpc-endpoint=https://URL.POINTING.TO.A.BSC.ARCHIVE.NODE/if-you/want-to-use/eth_call/within/substreams
```

And then run the `substreams` command against your local deployment (checkout `substreams-playground` in the _Run remotely_ section above):

```bash
substreams run -k -e localhost:9000 wasm_substreams_manifest.yaml pairs,block_to_pairs,db_out,volumes,totals -s 6810706 -t 6810711
```
