# Running the Substreams service locally

You can run the Substreams service locally this way:

### Get a release

For Ethereum, grab a release of [the Ethereum Firehose](https://github.com/streamingfast/sf-ethereum), get a [Docker release](https://github.com/orgs/streamingfast/packages/container/package/sf-ethereum), or build from source, until you can run `sfeth`.

### Get some data

&#x20;Get some merged blocks to play with locally:

```bash
# Downloads 2.6GB of data
sfeth tools download-from-firehose \
  api-dev.streamingfast.io:443 \
  6810000 6820000 \
  ./localblocks
# You can skip this one:
sfeth tools generate-irreversible-index ./localblocks ./localirr 6810000 6819700
```

### Write a config file

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

### Run the `firehose`

Now run the Substreams-enabled `firehose` using the config file:

```bash
$ sfeth start -c config.yaml
```

### Stream against your server

And then run the `substreams` command against your local deployment with:

```bash
substreams run -p -e localhost:9000  # ...
```

Where `-p` means plaintext, to run insecurely against your unsecured local server.
