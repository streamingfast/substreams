---
description: StreamingFast Substreams module types
---

# Module types

## Module types overview

Substreams uses two types of modules, `map` and `store`.

* `map` modules are functions receiving bytes as input and output. These bytes are encoded protobuf messages.
* `store` modules are stateful, saving and tracking data through the use of key-value stores.

### `store` modules

`store` modules write to key-value stores.

{% hint style="info" %}
**Note**: To ensure successful and proper parallelization can occur, `store` modules are not permitted to read any of their own data or values.
{% endhint %}

Stores declaring their own data types expose methods capable of mutating keys within the `store`.

### Core principle usage of stores

* Do not save keys in stores **unless they are going to be read by a downstream module**. Substreams stores are a way to aggregate data, but they are **not meant to be a storage layer**.
* Do not save all transfers of a chain in a `store` module, rather, output them in a `map` and have a downstream system store them for querying.

There are limitations impose on store usage. Specifically, each key/value entry must be smaller than 10MiB while a store cannot exceed 1GiB total. Keys being string, each character in the key account for 1 byte of storage space.

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
| `bigfloat` **(DEPRECATED): Use `bigdecimal`**                     | A string-serialized floating point value, supporting precision up to 100 digits  |
| `bigdecimal`                     | A string-serialized decimal value, supporting precision up to 2^63 decimal places  |

#### `updatePolicy` property

The `updatePolicy` property determines what methods are available in the runtime.

The `updatePolicy` also defines the merging strategy for identical keys found in two contiguous stores produced through parallel processing.

| Method              | Supported Value Types                    | Merge strategy\*                                                                                                                                                                                                                 |
| ------------------- | ---------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `set`               | `bytes`, `string`, `proto:...`           | The last key wins                                                                                                                                                                                                                |
| `set_if_not_exists` | `bytes`, `string`, `proto:...`           | The first key wins                                                                                                                                                                                                               |
| `add`               | `int64`, `bigint`, `bigfloat`, `float64` | Values are summed up                                                                                                                                                                                                             |
| `min`               | `int64`, `bigint`, `bigfloat`, `float64` | The lowest value is kept                                                                                                                                                                                                         |
| `max`               | `int64`, `bigint`, `bigfloat`, `float64` | The highest value is kept                                                                                                                                                                                                        |
| `set_sum`           | `int64`, `bigint`, `bigfloat`, `float64` | This type has two methods: `set` to set the value, or `sum` to add the given value to the current value.                                                                                                                         |
| `append`            | `string`, `bytes`                        | Both keys are concatenated in order. Appended values are limited to 8Kb.  Aggregation pattern examples are available in the [`lib.rs`](https://github.com/streamingfast/substreams-uniswap-v3/blob/develop/src/lib.rs#L760) file |

{% hint style="success" %}
**Tip**: All update policies provide the `delete_prefix` method.
{% endhint %}

The merge strategy is **applied during parallel processing**.

* A module has built two partial stores containing keys for segment A, blocks 0-1000, and segment B, blocks 1000-2000, and is prepared to merge them into a complete store.
* The complete store is represented acting as if the processing was done in a linear fashion, starting at block 0 and proceeding up to block 2000.

{% hint style="warning" %}
**Important**_**:** _ To preserve the parallelization capabilities of the system, **Substreams is not permitted to read what it has written or read from a `store` actively being written**.

A downstream module is created to read from a store by using one of its inputs to point to the output of the `store` module.
{% endhint %}

### Ordinals

Ordinals allow a key-value store to have multiple versions of a key within a single block. The `store` APIs contain different methods of `ordinal` or `ord`.

For example, the price for a token can change after transaction B and transaction D, and a downstream module might want to know the value of a key before transaction B **and between B and D**_._

{% hint style="warning" %}
**Important**: Ordinals **must be set every time a key is set** and **you can only set keys in increasing ordinal order**, or by using an ordinal equal to the previous.
{% endhint %}

In situations where a single key for a block is required and ordering in the store is not important, the ordinal uses a value of zero.

### `store` modes

You can consume data in one of two modes when declaring a `store` as an input to a module.

#### `get mode`

The `get mode` function provides the module with a key-value store that is guaranteed to be synchronized up to the block being processed. It's possible to query stores by using the `get_at`, `get_last` and `get_first` methods.

{% hint style="success" %}
**Tip:** Lookups are local, in-memory, and **extremely high-speed**.
{% endhint %}

The definition of `store` method behavior is:

* The `get_last` method is the fastest because it queries the store directly.
* The `get_first` method first goes through the current block's deltas in reverse order, before querying the store, in case the key being queried was mutated in the block.
* The `get_at` method unwinds deltas up to a specific ordinal, ensuring values for keys set midway through a block are still reachable.

**Example:**

Consider that you have a store with the following values:

```rust
let store = StoreUSDPrice {
   Block: #1000,
   Deltas: [
      Ord: 1, Key: "usd", Type: UPDATE, OldValue: 1.45, NewValue: 1.54,
      Ord: 2, Key: "usd", Type: DELETE, OldValue: 1.54, NewValue: <nil>,
      Ord: 3, Key: "usd", Type: INSERT, OldValue: <nil>, NewValue: 1.47,
      Ord: 4, Key: "usd", Type: UPDATE, OldValue: 1.47, NewValue: 1.65,
   ]
}
```

- `store.get_first() == "1.45"`: you get the _OldValue_ of the first delta, which is equivalent to `StoreUSDPrice(Block #999).get_last()`
- `store.get_last() == "1.65"`: you get the _NewValue_ of the last delta which is the state at end of Block #1000
- `store.get_at(1) == "1.47"`: you get the _NewValue_ of the delta with _Ord == 1_, or the closest ordinal is Ord: 1 does not exist

The current implementation is as follows:
- Start with value = get_last() (1.65)
- Iterate ord 4, value = detla.OldValue (1.47)
- Iterate ord 3, value = delta.OldValue (<nil>)
- Iterate ord 2, value = delta.OldValue (1.54)
- Iterate ord 1, ordinal == 1, return value (1.54)

#### `deltas mode`

`deltas` mode provides the module with **all the changes** occurring in the source `store` module. Updates, creates, and deletes of the keys mutated during the block processing become available.

{% hint style="info" %}
**Note:** When a `store` is set as an input to the module, it is read-only and you cannot modify, update or mutate them.
{% endhint %}

{% hint style="info" %}
**Note:** The deltas for a `set_sum` store type are always of type `bytes`, because the values are prepended with either "sum:" or "set:", depending on the method used.
{% endhint %}
