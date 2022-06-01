# Running the Substreams service locally

You can run the Substreams service locally this way:

1. Get a recent release of [the Ethereum Firehose](https://github.com/streamingfast/sf-ethereum), and install `sfeth`.

Alternatively, you can use this Docker image: `ghcr.io/streamingfast/sf-ethereum:6aa70ca`, known to work with version v0.0.5-beta of the `substreams` release herein.

1. Get some data (merged blocks) to play with locally (here on BSC mainnet):

```bash
# Downloads 2.6GB of data
sfeth tools download-from-firehose bsc-dev.streamingfast.io:443 6810000 6820000 ./localblocks
# You can skip this one:
sfeth tools generate-irreversible-index ./localblocks ./localirr 6810000 6819700
```

1. Write a config file:

{% code title="config.yaml" %}
```yaml
start:
  args:
    - firehose
  flags:
    log-to-file: false
    common-blockstream-addr:
    common-blocks-store-url: ./localblocks
    firehose-grpc-listen-addr: ":9000"
    substreams-enabled: true
    substreams-sub-request-parallel-jobs: 10
    substreams-partial-mode-enabled: true
    substreams-rpc-endpoints: "$MY_SUBSTREAMS_RPC_ENDPOINT" # If using eth_calls
    substreams-sub-request-block-range-size: 100
    substreams-stores-save-interval: 100

```
{% endcode %}

1. Then run the `firehose` service locally in a terminal, reading blocks from your disk:

```bash
sfeth start -c config.yaml
```

1. And then run the `substreams` command against your local deployment (checkout `substreams-playground` in the _Run remotely_ section above):

```bash
substreams run -k -e localhost:9000  # ...
```
