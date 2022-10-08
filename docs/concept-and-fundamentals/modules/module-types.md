---
description: StreamingFast Substreams module types
---

# Module Types

### Two Module Types

Substreams uses two types of modules, `map` and `store`. Map modules send and receive bytes. Store modules are stateful, saving and tracking data using simple key-value stores.

### Store Modules

Store modules write to key-value stores.&#x20;

To ensure successful and proper parallelization store modules are not permitted to read any of their own data or values.

Stores that declare their own data types will have methods exposed that are able to mutate the store's keys.

The two important store properties are `valueType,`and `updatePolicy`.

#### `valueType` Property

The `valueType` property instructs the Substreams runtime of the data that will be saved to the `stores`.

| Value                          | Description                                                                      |
| ------------------------------ | -------------------------------------------------------------------------------- |
| `bytes`                        | A simple list of bytes                                                           |
| `string`                       | A UTF-8 string                                                                   |
| `proto:fully.qualified.Object` | Bytes that can be decoded using the protobuf definition `fully.qualified.Object` |
| `int64`                        | A string-serialized integer, that uses int64 arithmetic operations               |
| `float64`                      | A string-serialized floating point value, using float64 arithmetic operations    |
| `bigint`                       | A string-serialized integer, with precision of any depth                         |
| `bigfloat`                     | A string-serialized floating point value, with a precision up to 100 digits      |

#### `updatePolicy` Property

The `updatePolicy` property determines what methods are available in the runtime.&#x20;

The `updatePolicy` also defines the merging strategy for identical keys found in two contiguous stores produced through parallel processing.

| Method              | Supported Value Types                    | Merge strategy\*                    |
| ------------------- | ---------------------------------------- | ----------------------------------- |
| `set`               | `bytes`, `string`, `proto:...`           | The last key wins                   |
| `set_if_not_exists` | `bytes`, `string`, `proto:...`           | The first key wins                  |
| `add`               | `int64`, `bigint`, `bigfloat`, `float64` | Values are summed up                |
| `min`               | `int64`, `bigint`, `bigfloat`, `float64` | The lowest value is kept            |
| `max`               | `int64`, `bigint`, `bigfloat`, `float64` | The highest value is kept           |
| `append`            | `string`, `bytes`                        | Both keys are concatenated in order |

{% hint style="info" %}
_**Note**: all update policies provide the `delete_prefix` method._
{% endhint %}

{% hint style="info" %}
**Note:** The **merge strategy** is applied during parallel processing. A module has built two _partial_ stores with keys for segment A, blocks 0-1000, and a contiguous segment B, blocks 1000-2000, and is ready to merge those two _partial_ stores to make it a _complete_ store.

The _complete_ store will be represented as if processing had been done linearly, that is processing from block 0 up to 2000 linearly.
{% endhint %}

{% hint style="warning" %}
**Warning:** To preserve the parallelization capabilities of the system Substreams can never _read_ what it has written or read from a store that is currently being written.

To read from a store a downstream module is created with one of its inputs pointing to the store module's output.
{% endhint %}

#### Ordinals

Ordinals allow a key/value store to have multiple versions of a key within a single block. The store APIs contain different methods of `ordinal` or `ord`.

For example, the price for a token could change after transaction B and transaction D, and a downstream module might want to know the value of a key before transaction B _and between B and D._&#x20;

Ordinals _must be set_ each time a key is set and keys can _only be set in increasing ordinal order_, or with an ordinal equal to the previous.

For instances that require only a single key per block, and ordering in the store isn't important, the ordinal can simply use a zero value.

### Store Modes

Data can be consumed in one of two modes when declaring a `store` as an input to a module.

#### `get Mode`

Get mode provides the module with the _key/value_ store guaranteed to be in sync up to the block being processed; readily queried by methods such as `get_at`, `get_last` and `get_first.`&#x20;

{% hint style="info" %}
_**Note:**, Lookups are local, in-memory, and extremely fast!_
{% endhint %}

{% hint style="info" %}
**Note:** `` The `get_last` method is the fastest because it queries the store directly.&#x20;

The `get_first` method will first go through the current block's _deltas_ in reverse order, before querying the store, in case the key being queried was mutated in this block.&#x20;

The `get_at` method will unwind deltas up to a certain ordinal. This ensures values for keys set midway through a block can still be accessed.
{% endhint %}

#### `deltas Mode`

Deltas mode provides the module with _all_ _the_ _changes_ that occurred in the source `store` module. Updates, creates, and deletes of the different keys mutated during that specific block become available.

{% hint style="info" %}
_**Note:** When a store is set as an input to the module, it is read-only and cannot be modified, updated, or mutated in any way._
{% endhint %}
