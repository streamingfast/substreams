---
description: Running the StreamingFast Substreams service locally
---

# Running Substreams locally

## Local Substreams overview

Run Substreams locally by running StreamingFast Firehose on the same machine.

### Download Firehose

Full information for the installation and operation of Firehose is available in the [Firehose documentation](https://firehose.streamingfast.io/).

The full source code is available in the official [Firehose GitHub repository](https://github.com/streamingfast/firehose-ethereum).

Install Firehose locally from source or by using a [Firehose Docker release](https://github.com/orgs/streamingfast/packages/container/package/sf-ethereum).

### Get some data

Instruct Firehose to generate merged blocks files for Substreams by using:

{% code overflow="wrap" %}
```bash
# Downloads 2.6GB of data
fireeth tools download-from-firehose \
  mainnet.eth.streamingfast.io:443 \
  6810000 6820000 \
  ./localblocks
# You can skip this one:
fireeth tools generate-irreversible-index ./localblocks ./localirr 6810000 6819700
```
{% endcode %}

### Write a configuration file

The code in the `config.yaml` file is used for the Firehose configuration file. Additional information for configuration is [available in the Firehose documentation](https://firehose.streamingfast.io/).

{% code title="config.yaml" overflow="wrap" lineNumbers="true" %}
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

### Run `firehose`

Start Firehose passing it the `config.yaml` configuration file by using:

```bash
$ fireeth start -c config.yaml
```

### Running Substreams and Firehose

Run the `substreams` [`run`](https://substreams.streamingfast.io/reference-and-specs/command-line-interface#run) command for the Firehose deployment by using:

```bash
substreams run -p -e localhost:9000
```

Use the `-p` flag to specify plaintext mode when running the command for the local Firehose server in an insecure manner.
