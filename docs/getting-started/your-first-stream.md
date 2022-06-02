# Your First Stream

#### Substreams `CLI`

To run your first Substreams you will first need to install the `CLI` tool. You. Run through [Getting Started](../#getting-started) to set it up.

#### Authentication

To connect to the Substreams server you will need to get a StreamingFast authentication token. Follow [these steps](../reference-and-specs/authentication.md).

### Run your first Stream

Once you have setup your StreamingFast authentication token and your `CLI` tool you can now run your Substreams.

```bash
substreams run -e api-dev.streamingfast.io:443 \
   https://github.com/streamingfast/substreams-template/releases/download/v0.1.0/substreams-template-v0.1.0.spkg \
   map_transfers \
   --start-block 12292922 \
   --stop-block +1
```

Let's break down everything happening above.

* `substreams`: is our executable (the `CLI` tool you installed)
* `-e api-dev.streamingfast.io:443`: is the provider that will run your Substreams
* `https://github.com/../substreams-template-v0.5.0.spkg` : Path to the Substreams you wish to run. This examples points to our [template Substreams](https://github.com/streamingfast/substreams-template). This can be an `.spkg` or a `substreams.yaml` file.
* `map_transfers`: this is the module which we want to run, defined in the manifest
* `--start-block 12292922`: start mapping as of block `12292922`
* `--stop-block +1` only request a single block (stop block will be start block + 1)\


{% hint style="info" %}
**Packages & Manifest**

The example above runs a Substreams based on a published `.spkg` file (a.k.a [Package](../reference-and-specs/packages.md)). You can also run a Substreams by pointing it directly to a .yaml file (a.k.a [Manifest](../reference-and-specs/manifests.md)).&#x20;

You can think of Package as published Substreams that you can utilize and start streaming data right away.&#x20;
{% endhint %}
