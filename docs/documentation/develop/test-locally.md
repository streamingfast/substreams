# Test Locally

You can run a Substreams server locally to run e2e testing.

It requires a local copy of full merged-blocks files for the range you want to test over. You can easily download those files from a Firehose endpoint.

Then run the Substreams software locally, and run tests against it

## Install binaries

{% tabs %}
{% tab title="Firehose Core" %}
Install the `firehose-core` single binary for most chains (those without chain-specific extensions) with brew:

```bash
brew install streamingfast/tap/firehose-core
```

or get a release from: [https://github.com/streamingfast/firehose-core/releases](https://github.com/streamingfast/firehose-core/releases)
{% endtab %}

{% tab title="Firehose Ethereum" %}
Install the `firehose-ethereum` single binary with brew:

```bash
brew install streamingfast/tap/firehose-ethereum
```

or get a release from: [https://github.com/streamingfast/firehose-ethereum/releases](https://github.com/streamingfast/firehose-core/releases)
{% endtab %}
{% endtabs %}

## Download merged blocks locally

Run against an endpoint for the chain you're interested in. For example:

```bash
fireeth tools download-from-firehose mainnet.eth.streamingfast.io:443 1000 2000 \
   ./firehose-data/storage/merged-blocks
```

which will download merged blocks to your local disk. You might need to be authenticated.

## Run the Substreams engine locally

Run:

```bash
fireeth start substreams-tier1,substreams-tier2 --config-file= \
  --common-live-blocks-addr= --common-first-streamable-block=1000 \
  --substreams-state-bundle-size=10
```

**Notes**:

1. the `--common-first-streamable-block` must be the lowest block available on disk, otherwise the server will fail to start.
2. if you need to do `eth_calls` with the Ethereum, you can add: `--substreams-rpc-endpoints https://example.com/json-rpc/somekeysometimes`
3. the `--substreams-state-bundle-size=10` flag will write smaller stores snapshot, suitable for dev

This will run a fully workable stack

## Stream against your local instance

Test with:

```bash
substreams run -e localhost:10016 --plaintext \
  https://spkg.io/streamingfast/ethereum-explorer-v0.1.2.spkg \
  map_block_meta -s 1000 -t +10
```

and enjoy.
