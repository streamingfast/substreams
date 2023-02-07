---
description: StreamingFast Substreams module inputs
---

# Inputs

## `inputs` overview

Modules receive `inputs` of three types:

* `source`
* `map`
* `store`
* `params`

## Input type `source`

An `inputs` of type `source` represents a chain-specific, Firehose-provisioned protobuf object. Learn more about the supported protocols and their corresponding message types in the [Chains and inputs documentation](../../reference-and-specs/chains-and-endpoints.md).

{% hint style="info" %}
**Note**: The different blockchains reference different `Block` objects. For example, Solana references its `Block` object as `sf.solana.type.v1.Block`. Ethereum-based Substreams modules specify `sf.ethereum.type.v2.Block.`
{% endhint %}

The `source` `inputs` type \_\_ is defined in the Substreams manifest. It is important to specify the correct `Block` object for the chain.

<pre class="language-yaml" data-title="manifest excerpt"><code class="lang-yaml">modules:
- name: my_mod
  inputs:
  - <a data-footnote-ref href="#user-content-fn-1">source: sf.ethereum.type.v2.Block</a>
</code></pre>

#### `Clock` object

The `sf.substreams.v1.Clock` object is another source type available on any of the supported chains.

The `sf.substreams.v1.Clock` represents:

* `Block` `number`
* `Block` `ID`
* `Block` `timestamp`

## Input type `params`

An `inputs` of type `params` represents a parameterizable module input. Those parameters can be specified either:

* in the `params` section of the manifest,
* on the command-line (using `substreams run -p` for instance),
* by tweaking the protobuf objects directly when consuming from your favorite language

See the [Manifest's `params` manifest section of the Reference & specs](../../reference-and-specs/manifests.md#params) for more details.

## Input type `map`

An input of type `map` represents the output of another `map` module. It defines a parent-child relationship between modules.

The object's type is defined in the [`output.type`](../../reference-and-specs/manifests.md#modules-.output) attribute of the `map` module.

{% hint style="warning" %}
**Important**_**:**_ The graph built by input dependencies is a Directed Acyclic Graph, which means there can be no circular dependencies.
{% endhint %}

Define the `map` input type in the manifest and choose a name for the `map` reflecting the logic contained within it.

{% code title="manifest excerpt" %}
```yaml
  inputs:
    - map: my_map
```
{% endcode %}

[Learn more about `maps`](../../concepts-and-fundamentals/modules.md#the-map-module-type) in the Substreams modules documentation.

## Input type `store`

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

### Store access `mode`

Substreams uses two types of `mode` for modules:

* `get`
* `delta`

### Store constraints

* A `store` can only receive `inputs` as read-only.
* A `store` cannot depend on itself.

### `get` mode

`get` mode provides a key-value store readily queryable and guaranteed to be in sync with the block being processed.

{% hint style="success" %}
**Tip**_**:**_ `get` `mode` is the default mode for modules.
{% endhint %}

### `delta` mode

`delta` `mode` modules are [protobuf objects](../../../pb/sf/substreams/v1/substreams.proto#L124) containing all the changes occurring in the `store` module available in the same block.

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

[^1]: `source` input
