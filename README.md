Streaming Data Engine based on SF Firehose Tech
-----------------------------------------------

THIS IS AT THE DESIGN/CREATION STAGE. IT IS ONLY FOR DEVELOPER PREVIEW.

Think Fluvio for deterministic blockchain data.

The successor of https://github.com/streamingfast/sparkle, enabling greater composability, yet similar powers of parallelisation, and a much simpler model to work with.



Install
-------

Get a [/streamingfast/substreams/releases](release).

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

Then, try to run the [https://github.com/streamingfast/substreams-playground](PancakeSwap Substreams) (under `pcs-rust/`, we build instructions over there)

```
cd substreams-playground
substreams run -e bsc-dev.streamingfast.io:443 wasm_substreams_manifest.yaml pairs,block_to_pairs,volumes,totals,db_out -s 6810706 -t 6810711
```


Run locally
-----------

You can run the substreams service locally this way:

Get a recent release of [https://github.com/streamingfast/sf-ethereum](the Ethereum Firehose), and install `sfeth`.

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

Examples
--------

https://github.com/streamingfast/substreams-playground
