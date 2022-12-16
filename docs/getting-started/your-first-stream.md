---
description: Get streaming with StreamingFast Substreams
---

# Start Streaming

### Authentication

A StreamingFast authentication token is required to connect to the Substreams server. Additional information is provided for [obtaining an authentication token](../reference-and-specs/authentication.md) in the references section of the documentation.

{% hint style="warning" %}
_Important: The Substreams CLI must be_ [_installed_](installing-the-cli.md) _to continue._
{% endhint %}

### Run First Substream

Once the StreamingFast authentication token has been attained and the Substreams CLI has been installed it's time to run Substreams.

```bash
substreams run -e mainnet.eth.streamingfast.io:443 https://github.com/streamingfast/substreams-template/releases/download/v0.2.0/substreams-template-v0.2.0.spkg map_transfers --start-block 12292922 --stop-block +1
```

{% hint style="info" %}
Note: A full explanation for the Substreams run command is provided in the [Using the CLI documentation](../reference-and-specs/using-the-cli.md).
{% endhint %}

### Explanation

#### Substreams Run

First, start the Substreams CLI tool using the `run` command.

#### Endpoint

The endpoint is required by Substreams to connect to for data retrieval. The data provider for Substreams is located at the address. This is a running Firehose instance.\
`-e mainnet.eth.streamingfast.io:443`

#### Substreams Package

The Substreams package being run is also passed to the `substreams` command. [https://github.com/streamingfast/substreams-template/releases/download/v0.2.0/substreams-template-v0.2.0.spkg ](https://github.com/streamingfast/substreams-template/releases/download/v0.2.0/substreams-template-v0.2.0.spkg)\

This example points to the StreamingFast Substreams Template in the form of a `.spkg` file. In full Substreams setups, a configuration file `substreams.yaml` is generally used.

#### Module

The `map_transfers` module is defined in the manifest and it is the module that will be run by Substreams.

#### Block Mapping

Start mapping at the specific block 12292922 by using passing the flag and block number. \
`--start-block 12292922`

Cease block processing with `--stop-block +1.` The +1 option will request a single block. In the example, the next block would be 12292923.

### Next steps

At this point, the Substreams CLI is installed and should be functioning correctly. Simple data was sent back to the terminal to provide an idea of the possibilities. The [Developer Guide](https://substreams.streamingfast.io/developer-guide/overview) provides an in-depth look at Substreams and how to target specific blockchain data.

StreamingFast and the Substreams community have provided an assortment of examples to explore and learn from. Find them on the [examples page](https://substreams.streamingfast.io/reference-and-specs/examples) in the Substreams documentation.
