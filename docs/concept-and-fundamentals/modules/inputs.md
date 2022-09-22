---
description: StreamingFast Substreams module inputs
---

# Inputs

### Module Data Inputs

Modules come in two varieties: `map` and `store` and can define one or multiple inputs. The possible inputs are `map`, `store`, and `source.`

### `Source`

An Input of type `source` represents a chain-specific, firehose-provisioned protobuf object.

{% hint style="info" %}
See the list of [supported Protocols here](../../reference-and-specs/protocols.md) and their corresponding message type.
{% endhint %}

For example, Substreams on Ethereum would specify `sf.ethereum.type.v2.Block.`

```yaml
  inputs:
    - source: sf.ethereum.type.v2.Block
```

Another `source` type available on any chains is the `sf.substreams.v1.Clock` object. This object represents the block number, a block ID, and a block timestamp.

### `Map`

An Input of type `map` represents the output of another `map` module. The object's type is defined in the [`output.type`](../../reference-and-specs/manifests.md#modules-.output) attribute of the `map` module. _Note, map modules cannot depend on themselves._

Example:

```yaml
  inputs:
    - map: my_map
```

Find additional information about maps [here](../../concepts/modules.md#the-map-module-type).

### `Store`

An Input of type `store` is the state of another store.

```yaml
  inputs:
    - store: my_store
      mode: deltas
    - store: my_store # defaults to mode: get
```

### Module Inputs Modes

There are two possible modes that can be defined.

#### `get`

Get mode provides a key/value store that is guaranteed to be synced up to the block being processed, and readily queryable. _Note, this is the default value._

#### `delta`

Delta mode provides a protobuf object containing all the changes that occurred in the `store` module in the same block.

{% hint style="warning" %}
Here are some constraints on stores:

* Stores received as `inputs` are read-only.
* A `store` cannot depend on itself!
{% endhint %}

Find additional information about stores [here](../../concepts/modules.md#the-store-module-type).
