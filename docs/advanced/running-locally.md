---
description: Running the StreamingFast Substreams service locally
---

# Running Substreams locally

You can run Substreams locally by running StreamingFast Firehose on the same machine.

### Download Firehose

Full information for the installation and operation of Firehose is available in the [Firehose documentation](https://firehose.streamingfast.io/).

The full source code is available in the official [Firehose GitHub repository](https://github.com/streamingfast/firehose-ethereum).

Firehose can be built from source or installed using a [Firehose Docker release](https://github.com/orgs/streamingfast/packages/container/package/sf-ethereum).

### Get some data

The following code will instruct Firehose to generate merged blocks files to use with Substreams.

```bash
# Downloads 2.6GB of data
fireeth tools download-from-firehose \
  mainnet.eth.streamingfast.io:443 \
  6810000 6820000 \
  ./localblocks
# You can skip this one:
fireeth tools generate-irreversible-index ./localblocks ./localirr 6810000 6819700
```

### Write a config file

Use the following for the Firehose configuration file. Additional information is available in the [Firehose documentation](https://firehose.streamingfast.io/).

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

Start Firehose and pass it the config file.

```bash
$ fireeth start -c config.yaml
```

### Run Substreams with Firehose

Run the `substreams` command against the Firehose deployment using the following command.

```bash
substreams run -p -e localhost:9000  # ...
```

The `-p` flag indicates plaintext, running insecurely against the unsecured Firehose local server.
