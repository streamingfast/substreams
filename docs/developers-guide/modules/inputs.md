---
description: StreamingFast Substreams module inputs
---

# Inputs

Modules receive inputs of three types:&#x20;

* `source`
* `map`
* `store`

## Input type `source`

An Input of type `source` represents a chain-specific, Firehose-provisioned protobuf object.

{% hint style="info" %}
**Note**_**:** _ Learn more about the supported protocols and their corresponding message types in the [Chains and inputs documentation](../../reference-and-specs/chains-and-endpoints.md).
{% endhint %}

{% hint style="success" %}
**Tip**: Ethereum-based Substreams modules specify `sf.ethereum.type.v2.Block.`&#x20;
{% endhint %}

{% hint style="info" %}
**Note**: The different blockchains will reference a different `Block`. For example, Solana references its `Block` object as `sf.solana.type.v1.Block`.&#x20;
{% endhint %}

The `source` inputs type __ is defined in the Substreams manifest. It is important to specify the correct Block object for the targeted chain.

```yaml
  inputs:
    - source: sf.ethereum.type.v2.Block
```

#### Clock object

The `sf.substreams.v1.Clock` object is another source type available on any of the supported chains.

The `sf.substreams.v1.Clock` represents:

* Block number
* Block ID
* Block timestamp

## Input type `map`

An Input of type `map` represents the output of another `map` module.&#x20;

The object's type is defined in the [`output.type`](../../reference-and-specs/manifests.md#modules-.output) attribute of the `map` module.&#x20;

{% hint style="warning" %}
**Important**_**:** _ It's not possible for __ Map modules _**to depend on themselves**_. When modules depend on themselves they create a circular dependency.
{% endhint %}

The `map` inputs type __ is defined in the Substreams manifest. The name of the map is chosen by the developer and is representative of the logic contained within.

```yaml
  inputs:
    - map: my_map
```

Additional information regarding `maps` is located in the Substreams [modules documentation](../../concepts-and-fundamentals/modules.md#the-map-module-type).

## Input type `store`

An Input of type `store` represents the state of another store used with the Substreams implementation being created.

The `store` inputs type __ is defined in the Substreams manifest. Similar to maps, stores are named appropriately, indicating the logic contained within them.

Store modules are set to `get` mode by default using:

```yaml
  inputs:
    - store: my_store # defaults to mode: get
```

Alternatively, set `stores` to `deltas` mode using:

```yaml
  inputs:
    - store: my_delta_store
      mode: deltas
```

### Module modes

There are **two modes** defined for modules.

* `get`
* `delta`

### Store constraints

Constraints for stores are defined as:

* Stores received as `inputs` are read-only.
* Stores cannot depend on themselves.

### `get` mode

Get mode provides a key/value store that is readily queryable and guaranteed to be in sync with the block being processed.&#x20;

{% hint style="success" %}
**Tip**_**:**_ `get` mode is the default mode for modules.
{% endhint %}

### `delta` mode

Modules using delta mode are [protobuf objects](../../../proto/sf/substreams/v1/substreams.proto#L124) and contain all the changes that have occurred in the `store` module available in the same block.&#x20;

Delta mode enables developers with the ability to loop through keys decoding values that were mutated in the module.

#### Store deltas example

Code example illustrating the protobuf model for StoreDeltas:

{% code overflow="wrap" lineNumbers="true" %}
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
