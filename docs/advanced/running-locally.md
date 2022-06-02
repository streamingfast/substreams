# Running the Substreams service locally

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
