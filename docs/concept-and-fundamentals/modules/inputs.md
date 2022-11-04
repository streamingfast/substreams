---
description: StreamingFast Substreams module inputs
---

# Inputs

Modules can receive inputs of three types: `source`, `map`, and `store`.

## Input type `source`

An Input of type `source` represents a chain-specific, firehose-provisioned protobuf object.

{% hint style="info" %}
_**Note:** Find the supported protocols and their corresponding message types in the_ [_Chains & Inputs documentation_](../../reference-and-specs/chains-and-endpoints.md)_._
{% endhint %}

Ethereum based Substreams implementations would specify `sf.ethereum.type.v2.Block.`&#x20;

The `source` inputs type __ is defined in the Substreams manifest as seen below.

```yaml
  inputs:
    - source: sf.ethereum.type.v2.Block
```

The `sf.substreams.v1.Clock` object is another source type available on any of the supported chains.

The `sf.substreams.v1.Clock` represents:

* the block number,&#x20;
* a block ID,&#x20;
* and a block timestamp.

## Input type `map`

An Input of type `map` represents the output of another `map` module. The object's type is defined in the [`output.type`](../../reference-and-specs/manifests.md#modules-.output) attribute of the `map` module.&#x20;

{% hint style="info" %}
_**Note:** Map modules **cannot** depend on themselves._
{% endhint %}

The `map` inputs type __ is defined in the Substreams manifest as seen below.

```yaml
  inputs:
    - map: my_map
```

Find additional information about `maps` in the Substreams [modules documentation](../../concepts/modules.md#the-map-module-type).

## Input type `store`

An Input of type `store` is the state of another store used with Substreams.

The `store` inputs type __ is defined in the Substreams manifest as seen below.

```yaml
  inputs:
    - store: my_store
      mode: deltas
    - store: my_store # defaults to mode: get
```

There are **two possible modes** that can be defined for modules:

* `get`
* `delta`

{% hint style="warning" %}
_Important: Stores have constraints defined as_:

* Stores received as `inputs` are read-only.
* Stores cannot depend on themselves.
{% endhint %}

### `get` mode

Get mode provides a key/value store that is readily queryable and guaranteed to be in sync with the block being processed.&#x20;

{% hint style="info" %}
_**Note:** `get` mode is the default mode._
{% endhint %}

### `delta` mode

When mode `delta` is specified, the input of the module will be a [protobuf object](../../../proto/sf/substreams/v1/substreams.proto#L124) containing all the changes that occurred in the `store` module in the same block. You can then loop through keys, decode the old and new values that were mutated in your module.

Here is the protobuf model for StoreDeltas:

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
