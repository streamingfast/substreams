---
description: StreamingFast Substreams modules
---

# Modules

### What are Modules?

Modules are small pieces of code, running in a WebAssembly virtual machine, amidst the stream of blocks arriving from a blockchain node. They can also process the network's history out of flat files, backed by the Firehose. See the [Firehose documentation](http://firehose.streamingfast.io/) for more details.

Modules may have one or more inputs. The inputs can be  `map, store` or in the form of a `Block` or `Clock` received from the blockchain's data source.

{% hint style="info" %}
Multiple inputs are made possible because blockchains are clocked, and they allow synchronization between multiple execution streams, opening up great performance improvements, even over comparable traditional streaming engines.
{% endhint %}

Modules have a single output, that can be typed, to inform consumers what to expect and how to interpret the bytes being sent out of the module.

Modules can form a graph of modules, taking each other's output as the next module's input.

The `transfer_map` module extracts all transfers in each `Block`, and `transfer_count` as a `store` module and could even track how many transfers have occurred.

{% embed url="https://mermaid.ink/svg/pako:eNp1kM0KwjAQhF8l7NkWvEbwIPUJ9NYUWZKtLTZJ2WwEEd_dCAr-4GFhd_h2GOYKNjoCDUfGeVD7ZmWCUqmvSQZiyr6Wy0z1eVlvpmhPbYqZLen_RKeqaq2EMaSe-OBxfhi-320Z_aF8_diYgxC3SSKT_tE7WIAn9ji6kvv6sDdQsngyoMvqqMc8iQETbgXNs0OhrRuLG-gep0QLwCxxdwkWtHCmF9SMWGrwT-p2B02rZZY" %}
Substreams modules data interaction
{% endembed %}

Modules can also take in multiple inputs as seen in the `counters` store example diagram below. Two modules feed into a `store` effectively tracking multiple `counters`.

{% embed url="https://mermaid.ink/svg/pako:eNqdkE1qAzEMha9itE4GsnWgi5KcINmNh6LamozJeGxsuSGE3L1KW1PIptCdnnjv088NbHQEGk4Z06SOu61ZlHqfoz33JdZsSasydsQTZaqh42ui7mPTvT4cg1qvX1TA9HbxPLmMF5zLv_KOUiyev8JPvF60fm5-J22sC1MufeGYZVDTQ8M07C-jdf4AwAoC5YDeyWtuD5wBOSGQAS2loxHrzAbMchdrTQ6Z9s4LBfQo-9EKsHI8XBcLmnOlZtp5lE-HH9f9EylZic0" %}
Modules with multiple inputs diagram
{% endembed %}

All of the modules are executed as a directed acrylic graph (DAG), each time a new `Block` is processed.

_Note, The top-level data source is always a protocol's `Block` protobuf model, and is deterministic in its execution._

## Module Types

There are two types of modules, a `map` module, and a `store` module.

### The `map` module type

A `map` module takes in bytes and also outputs them. In the Substreams manifest, you would declare the protobuf types to help users decode the streams and generate code used to work with Substreams.

### `store` Modules

Modules of the store type are stateful, meaning they track and save state information in the form of a simple and fast key-value data store.

### Writing Stores

Store modules write to the key value stores however to ensure successful and proper parallelization they are not permitted to read any of its own data or values.

If stores declare their own data types methods become exposed that are able to mutate the store's keys.

Two important properties exist on the `store.`

* `valueType`
* `updatePolicy`

#### `valueType`

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

#### `updatePolicy`

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

All update policies provide the `delete_prefix` method.

{% hint style="info" %}
The **merge strategy** is applied during parallel processing. A module has built two _partial_ stores with keys for segment A, blocks 0-1000, and a contiguous segment B, blocks 1000-2000, and is ready to merge those two _partial_ stores to make it a _complete_ store.

The _complete_ store will be represented as if processing had been done linearly, that is processing from block 0 up to 2000 linearly.
{% endhint %}

{% hint style="warning" %}
To preserve the parallelization capabilities of the system, Substreams can never _read_ what it has written, nor read from a store that is currently being written to.

To read from a store, create a downstream module with one of its inputs pointing to the store's output.
{% endhint %}

#### Ordinals

Ordinals allow a key/value store to have multiple versions of a key within a single block. The store APIs contain different methods of `ordinal` or `ord`.

The price for a token could change after transaction B and transaction D, and a downstream module might want to know the value of a key before transaction B and between B and D.&#x20;

Oridinals must be set each time a key is set.

{% hint style="warning" %}
Keys can only be set in increasing _ordinal_ order, or with an _ordinal_ equal to the previous.
{% endhint %}

For instances that require only a single key per block, and ordering in the store isn't important, the ordinal can simply use a zero value; the numeric 0.

### Reading Stores

When declaring a `store` as an input to a module data can be consumed in one of two modes.

* `get`
* `deltas`

#### `get`

`Get` provides the module with the _key/value_ store guaranteed to be in sync up to the block being processed, readily queried by methods such as `get_at`, `get_last` and `get_first.` _Note, lookups are local, in-memory, and very fast._

{% hint style="info" %}
The fastest is `get_last` as it queries the store directly. `get_first` will first go through the current block's _deltas_ in reverse order, before querying the store, in case the key being queried was mutated in this block.&#x20;

`get_at` will unwind deltas up to a certain ordinal. This ensures values for keys set midway through a block can still be accessed.
{% endhint %}

#### `deltas`

`Deltas` provide the module with all the _changes_ that occurred in the source `store` module. Updates, creates, and deletes of the different keys that were mutated during that block become available.

When a store is set as an input to your module, it is read-only and cannot be modified, updated, or mutated in any way.
