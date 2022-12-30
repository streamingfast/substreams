---
description: StreamingFast Substreams module types
---

# Module types

## Module types overview

Substreams uses two types of modules, `map` and `store`.&#x20;

* Map modules are functions receiving bytes as input and output. These bytes are encoded protobuf messages.
* Store modules are stateful, saving and tracking data through the use of key-value stores.

### Store modules

Store modules write to key-value stores.&#x20;

{% hint style="info" %}
**Note**: To ensure successful and proper parallelization store modules are not permitted to read any of their own data or values.
{% endhint %}

Stores declaring their own data types expose methods capable of mutating keys within the store.

### Core principle usage of stores

* Do not store keys in stores _unless they are to be read by a downstream module_. Substreams stores are a means to do aggregations, but it is not a storage layer.
* Do not store all transfers of a chain in a `store` module, rather, output them in a mapper and have a downstream system store them for querying.

### Important store properties

The two important store properties are `valueType,`and `updatePolicy`.

#### `valueType` property

The `valueType` property instructs the Substreams runtime of the data to be saved in the `stores`.

| Value                          | Description                                                                      |
| ------------------------------ | -------------------------------------------------------------------------------- |
| `bytes`                        | A basic list of bytes                                                            |
| `string`                       | A UTF-8 string                                                                   |
| `proto:fully.qualified.Object` | Decode bytes by using the protobuf definition `fully.qualified.Object`           |
| `int64`                        | A string-serialized integer by using int64 arithmetic operations                 |
| `float64`                      | A string-serialized floating point value, used for float64 arithmetic operations |
| `bigint`                       | A string-serialized integer, supporting precision of any depth                   |
| `bigfloat`                     | A string-serialized floating point value, supporting precision up to 100 digits  |

#### `updatePolicy` property

The `updatePolicy` property determines what methods are available in the runtime.&#x20;

The `updatePolicy` also defines the merging strategy for identical keys found in two contiguous stores produced through parallel processing.

| Method              | Supported Value Types                    | Merge strategy\*                                                                                                                                                                                                                   |
| ------------------- | ---------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `set`               | `bytes`, `string`, `proto:...`           | The last key wins                                                                                                                                                                                                                  |
| `set_if_not_exists` | `bytes`, `string`, `proto:...`           | The first key wins                                                                                                                                                                                                                 |
| `add`               | `int64`, `bigint`, `bigfloat`, `float64` | Values are summed up                                                                                                                                                                                                               |
| `min`               | `int64`, `bigint`, `bigfloat`, `float64` | The lowest value is kept                                                                                                                                                                                                           |
| `max`               | `int64`, `bigint`, `bigfloat`, `float64` | The highest value is kept                                                                                                                                                                                                          |
| `append`            | `string`, `bytes`                        | Both keys are concatenated in order. Appended values are limited to 8Kb.  Aggregation pattern examples are available in the [`lib.rs`](https://github.com/streamingfast/substreams-uniswap-v3/blob/develop/src/lib.rs#L760) file.  |



{% hint style="info" %}
**Note**: all update policies provide the `delete_prefix` method.
{% endhint %}

{% hint style="info" %}
**Note**_**:** _ The **merge strategy** is applied **during** parallel processing.&#x20;

* A module has built two partial stores containing keys for segment A (blocks 0-1000) and segment B (blocks 1000-2000) and is prepared to merge them into a complete store.
* The complete store is represented acting as if the processing was done in a linear fashion, starting at block 0 and proceeding up to block 2000.
{% endhint %}

{% hint style="warning" %}
**Important**_**:** _ To preserve the parallelization capabilities of the system, Substreams is not permitted to read what it has written or read from a store actively being written.

A downstream module is created to read from a store by using one of its inputs to point to the output of the store module.
{% endhint %}

### Ordinals

Ordinals allow a key-value store to have multiple versions of a key within a single block. The store APIs contain different methods of `ordinal` or `ord`.

For example, the price for a token can change after transaction B and transaction D, and a downstream module might want to know the value of a key before transaction B _and between B and D._&#x20;

{% hint style="warning" %}
**Important**: Ordinals _**must be set every time a key is set**_ and _**you can only set keys in increasing ordinal order**_, or by using an ordinal equal to the previous.
{% endhint %}

In situations where a single key for a block is required and ordering in the store is not important, the ordinal uses a value of zero.

### Store modes

You can consume data in one of two modes when declaring a `store` as an input to a module.

#### `get Mode`

The `get mode` function provides the module with a key-value store that is guaranteed to be synchronized up to the block being processed. It's possible to query `stores` by using the `get_at`, `get_last` and `get_first` methods.

{% hint style="success" %}
**Tip:** Lookups are local, in-memory, and extremely high-speed!
{% endhint %}

{% hint style="info" %}
**Note:** Store method behavior is defined as:

* The `get_last` method is the fastest because it queries the store directly.&#x20;
* The `get_first` method first goes through the current block's deltas in reverse order, before querying the store, in case the key being queried was mutated in the block.&#x20;
* The `get_at` method unwinds deltas up to a specific ordinal, ensuring values for keys set midway through a block are still reachable.
{% endhint %}

#### `deltas mode`

Deltas mode provides the module with _all_ _the_ _changes_ occurring in the source `store` module. Updates, creates, and deletes of the keys mutated during the block processing become available.

{% hint style="info" %}
**Note:** When a `store` is set as an input to the module, it is read-only and you cannot modify, update or mutate them.
{% endhint %}
