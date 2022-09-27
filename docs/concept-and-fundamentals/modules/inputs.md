---
description: StreamingFast Substreams module inputs
---

# Inputs

### Data Inputs

Modules come in two varieties: `map` and `store` and can define one or multiple inputs. The possible inputs are `map`, `store`, and `source.`

#### `Source`

An Input of type `source` represents a chain-specific, firehose-provisioned protobuf object.

{% hint style="info" %}
Note: Find the supported protocols and their corresponding message types in the [Chains & Inputs documentation](../../reference-and-specs/chains-and-endpoints.md).
{% endhint %}

To illustrate, Substreams on Ethereum would specify `sf.ethereum.type.v2.Block.`

```yaml
  inputs:
    - source: sf.ethereum.type.v2.Block
```

Another `source` type available on any chains is the `sf.substreams.v1.Clock` object. This object represents the block number, a block ID, and a block timestamp.

#### `Map`

An Input of type `map` represents the output of another `map` module. The object's type is defined in the [`output.type`](../../reference-and-specs/manifests.md#modules-.output) attribute of the `map` module. _Note, map modules cannot depend on themselves._

```yaml
  inputs:
    - map: my_map
```

Find additional information about `maps` in the [modules documentation](../../concepts/modules.md#the-map-module-type).

#### `Store`

An Input of type `store` is the state of another store.

```yaml
  inputs:
    - store: my_store
      mode: deltas
    - store: my_store # defaults to mode: get
```

### Modes

There are two possible modes that can be defined for modules.

#### `get`

Get mode provides a key/value store that is guaranteed to be synced up to the block being processed, and readily queryable. _Note, this is the default value._

#### `delta`

Delta mode provides a protobuf object containing all the changes that occurred in the `store` module in the same block.

{% hint style="warning" %}
Constraints for stores:

* Stores received as `inputs` are read-only.
* A `store` cannot depend on itself!
{% endhint %}

Find additional information for `stores` in the main [Modules documentation](../../concepts/modules.md#the-store-module-type).
