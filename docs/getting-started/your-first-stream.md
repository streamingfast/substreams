---
description: Get streaming with StreamingFast Substreams
---

# Start Streaming

### Authentication

A StreamingFast authentication token is required to connect to the Substreams server. See the [ authentication](../reference-and-specs/authentication.md) section for details.

{% hint style="warning" %}
_Important: The Substreams CLI must be_ [_installed_](installing-the-cli.md) _to continue._
{% endhint %}

### Run Your First Substreams

Once authenticated, run your first Substreams with:

{% code overflow="wrap" %}
```bash
$ substreams run -e mainnet.eth.streamingfast.io:443 https://github.com/streamingfast/substreams-template/releases/download/v0.2.0/substreams-template-v0.2.0.spkg map_transfers --start-block 12292922 --stop-block +1
```
{% endcode %}

This [`run`](../reference-and-specs/using-the-cli.md#run) command starts a consumer, targeting the `--endpoint` serving [a given blockchain](../reference-and-specs/chains-and-endpoints.md), for the given [spkg package](../reference-and-specs/packages.md), starting at the given block, and stopping after processing one block. It will stream the output of the `map_transfers` [module](../developer-guide/setting-up-handlers.md).

{% hint style="info" %}
See also: [Using the CLI documentation](../reference-and-specs/using-the-cli.md).
{% endhint %}

### Next steps

At this point, the Substreams CLI is installed and should be functioning correctly. Simple data was sent back to the terminal to provide an idea of the possibilities. The [Developer Guide](https://substreams.streamingfast.io/developer-guide/overview) provides an in-depth look at Substreams and how to target specific blockchain data.

StreamingFast and the Substreams community have provided an assortment of examples to explore and learn from. Find them on the [examples page](https://substreams.streamingfast.io/reference-and-specs/examples) in the Substreams documentation.
