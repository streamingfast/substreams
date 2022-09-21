# Inputs

### Module Data Inputs

A `map` and `store` module can define one or multiple inputs. The possible inputs are `map`, `store`, and `source.`

### `Source`

An _Input_ of type `source` represent a chain-specific, firehose-provisioned protobuf object.

{% hint style="info" %}
See the list of [supported Protocols here](../../reference-and-specs/protocols.md) and their corresponding message type.
{% endhint %}

For example, for Substreams on Ethereum you would specify `sf.ethereum.type.v2.Block.`

```yaml
  inputs:
    - source: sf.ethereum.type.v2.Block
```

Another `source` type available on any chains is the `sf.substreams.v1.Clock` object, representing the block number, a block ID and a block timestamp.

### `Map`

An _Input_ of type `map` represents the output of another `map` module. The type of the object would be type defined in the [`output.type`](../../reference-and-specs/manifests.md#modules-.output) attribute of the `map` module. **A map module cannot depend on itself.**

Example:

```yaml
  inputs:
    - map: my_map
```

Read more about maps [here](../../concepts/modules.md#the-map-module-type).

### `Store`

An _Input_ of type `store` is the state of another store.

Example:

```yaml
  inputs:
    - store: my_store
      mode: deltas
    - store: my_store # defaults to mode: get
```

There are two possible `mode` that you can define:

* `get`: in this mode you will be provided with a key/value store that is guaranteed to be synced up to the block being processed, readily queryable. **This is the default value.**
* `delta`: in this mode you will be provided with a protobuf _object_ containing all the changes that occurred in the `store` module in the same block.

{% hint style="warning" %}
Here are some constraints on stores:

* Stores received as `inputs` are _read-only_.
* A `store` cannot depend on itself
{% endhint %}

Read more about stores [here](../../concepts/modules.md#the-store-module-type).
