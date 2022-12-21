---
description: StreamingFast Substreams module inputs
---

# Inputs

Modules can receive inputs of three types: `source`, `map`, and `store`.

## Input type `source`

An Input of type `source` represents a chain-specific, Firehose-provisioned protobuf object.

{% hint style="info" %}
**Note**_**:** Find the supported protocols and their corresponding message types in the_ [_Chains & Inputs documentation_](../../reference-and-specs/chains-and-endpoints.md)_._
{% endhint %}

{% hint style="success" %}
**Tip**: Ethereum-based Substreams implementations would specify `sf.ethereum.type.v2.Block.`&#x20;
{% endhint %}

{% hint style="info" %}
**Note**: Each of the different blockchains will reference a different Block specific to the chain being targeted. For example, Solana references its Block object as `sf.solana.type.v1.Block`.&#x20;
{% endhint %}

The `source` inputs type __ is defined in the Substreams manifest as seen below. As previously mentioned, it's crucial to make sure the correct Block object for the chain being targeted has been specified.

```yaml
  inputs:
    - source: sf.ethereum.type.v2.Block
```

#### Clock Object

The `sf.substreams.v1.Clock` object is another source type available on any of the supported chains.

The `sf.substreams.v1.Clock` represents:

* the block number,&#x20;
* a block ID,&#x20;
* and a block timestamp.

## Input type `map`

An Input of type `map` represents the output of another `map` module.&#x20;

The object's type is defined in the [`output.type`](../../reference-and-specs/manifests.md#modules-.output) attribute of the `map` module.&#x20;

{% hint style="warning" %}
**Important**_**:** _ Map modules _**cannot depend on themselves**_. Modules that attempt to do so create what's known as a circular dependency, and is not desired.
{% endhint %}

The `map` inputs type __ is defined in the Substreams manifest as seen below. The name of the map is chosen by the developer and should be representative of the logic contained within.

```yaml
  inputs:
    - map: my_map
```

Additional information regarding `maps` is located in the Substreams [modules documentation](../../concepts-and-fundamentals/modules.md#the-map-module-type).

## Input type `store`

An Input of type `store` represents the state of another store used with the Substreams implementation being created.

The `store` inputs type __ is defined in the Substreams manifest as seen below. Similar to maps, stores should be named appropriately indicating the logic contained within them.

Store modules are set to `get` mode by default as illustrated in the following manifest code excerpt.

```yaml
  inputs:
    - store: my_store # defaults to mode: get
```

Alternatively, stores can be set to deltas mode as illustrated in the following manifest code excerpt.

```yaml
  inputs:
    - store: my_delta_store
      mode: deltas
```

### Module Modes

There are **two possible modes** that can be defined for modules.

* `get`
* `delta`

### Store Constraints

Constraints for stores are defined as follows.

* Stores received as `inputs` are read-only.
* Stores cannot depend on themselves.

### `get` mode

Get mode provides a key/value store that is readily queryable and guaranteed to be in sync with the block being processed.&#x20;

{% hint style="success" %}
**Tip**_**:** `get` mode is the default mode for modules._
{% endhint %}

### `delta` mode

Modules using delta mode are [protobuf objects](../../../proto/sf/substreams/v1/substreams.proto#L124) and contain all the changes that have occurred in the `store` module available in the same block.&#x20;

Delta mode enables developers with the ability to loop through keys decoding old and new values that were mutated in the module.

#### Store Deltas Example

The following code example illustrates the protobuf model for StoreDeltas.

```protobuf
message StoreDeltas {
  repeated StoreDelta deltas = 1;
}

message StoreDelta {
  enum Operation {
    UNSET = 0;
    CREATE = 1;
    UPDATE = 2;
    DELETE = 3;
  }
  Operation operation = 1;
  uint64 ordinal = 2;
  string key = 3;
  bytes old_value = 4;
  bytes new_value = 5;
}
```
