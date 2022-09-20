---
description: Get streaming with StreamingFast Substreams
---

# Start Streaming

### Authentication

Follow [these steps](../reference-and-specs/authentication.md) to obtain a StreamingFast authentication token. The token is required to connect to the Substreams server.

_Note, make sure the Substreams CLI has_ [_been installed_](installing-the-cli.md) _before proceeding._

### Run First Substream

Once the StreamingFast authentication token has been attained and the Substreams CLI has been installed it's time to run Substreams.

```bash
substreams run -e api-dev.streamingfast.io:443 https://github.com/streamingfast/substreams-template/releases/download/v0.2.0/substreams-template-v0.2.0.spkg map_transfers --start-block 12292922 --stop-block +1
```

Breaking down the elements of the command above.

* The `substreams` command is running the Substreams CLI tool.
* The data provider for Substreams is located at the address.\
  `-e api-dev.streamingfast.io:443`
* The Substreams package being run is also passed to the `substreams` command. `https://github.com/../substreams-template-v0.5.0.spkg.` \
  This example points to the StreamingFast[ Substreams template](https://github.com/streamingfast/substreams-template) in the form of a `.spkg` file or a `substreams.yaml` configuration file.
* The `map_transfers` module is defined in the manifest and it is the module that will be run by Substreams.
* Start mapping at the specific block 12292922 by using passing the flag and block number. \
  `--start-block 12292922`
* Cease block processing with `--stop-block +1.` The +1 option will request a single block. In the example, the next block would be 12292923.

{% hint style="info" %}
**Packages & Manifest**

The example above runs a Substreams based on a published `.spkg` file (a.k.a [Package](../reference-and-specs/packages.md)). You can also run a Substreams by pointing it directly to a .yaml file (a.k.a [Manifest](../reference-and-specs/manifests.md)).

You can think of Packages as published Substreams that can be used to start streaming data immediately.
{% endhint %}
