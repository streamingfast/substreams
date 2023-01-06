---
description: StreamingFast Substreams module inputs
---

# Inputs

## `inputs` overview

Modules receive `inputs` of three types:&#x20;

* `source`
* `map`
* `store`

## `inputs` type `source`

An `inputs` of type `source` represents a chain-specific, Firehose-provisioned protobuf object. Learn more about the supported protocols and their corresponding message types in the [Chains and inputs documentation](../../reference-and-specs/chains-and-endpoints.md).

{% hint style="info" %}
**Note**: The different blockchains reference different `Block` objects. For example, Solana references its `Block` object as `sf.solana.type.v1.Block`. Ethereum-based Substreams modules specify `sf.ethereum.type.v2.Block.`
{% endhint %}

The `source` `inputs` type __ is defined in the Substreams manifest. It is important to specify the correct `Block` object for the chain.

{% code title="manifest excerpt" %}
```yaml
  inputs:
    - source: sf.ethereum.type.v2.Block
```
{% endcode %}

#### `Clock` object

The `sf.substreams.v1.Clock` object is another source type available on any of the supported chains.

The `sf.substreams.v1.Clock` represents:

* `Block` `number`
* `Block` `ID`
* `Block` `timestamp`

## `inputs` type `map`

An `inputs` of type `map` represents the output of another `map` module.&#x20;

The object's type is defined in the [`output.type`](../../reference-and-specs/manifests.md#modules-.output) attribute of the `map` module.&#x20;

{% hint style="warning" %}
**Important**_**:**_ `map` modules _**cannot depend on themselves**_. When modules depend on themselves they create an unwanted circular dependency.
{% endhint %}

You define the `map` `inputs` type in the Substreams manifest and choose a name for the `map` reflecting the logic contained within it.

{% code title="manifest excerpt" %}
```yaml
  inputs:
    - map: my_map
```
{% endcode %}

[Learn more about `maps`](../../concepts-and-fundamentals/modules.md#the-map-module-type) in the Substreams modules documentation.

## `inputs` type `store`

A `store inputs` type represents the state of another `store` used by the Substreams module being created.

The developer defines the `store` `inputs` type in the Substreams manifest and gives the `store` a descriptive name that reflects the logic contained within it, similar to a `map`.

Store modules are set to `get` mode by default:

{% code title="manifest excerpt" %}
```yaml
  inputs:
    - store: my_store # defaults to mode: get
```
{% endcode %}

Alternatively, set `stores` to `deltas` mode by using:

{% code title="manifest excerpt" %}
```yaml
  inputs:
    - store: my_delta_store
      mode: deltas
```
{% endcode %}

### Module `mode`

Substreams uses two types of `mode` for modules:

* `get`
* `delta`

### Store constraints

* A `store` can only receive `inputs` as read-only.
* A `store` cannot depend on itself.

### `get` `mode`

`get` mode provides a key-value store readily queryable and guaranteed to be in sync with the block being processed.&#x20;

{% hint style="success" %}
**Tip**_**:**_ `get` `mode` is the default mode for modules.
{% endhint %}

### `delta` `mode`

`delta` `mode` modules are [protobuf objects](../../../pb/sf/substreams/v1/substreams.proto#L124) containing all the changes occurring in the `store` module available in the same block.&#x20;

`delta` mode enables you to loop through keys and decode values mutated in the module.

#### `store` `deltas`

The protobuf model for `StoreDeltas` is defined by using:

{% code overflow="wrap" %}
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
{% endcode %}
