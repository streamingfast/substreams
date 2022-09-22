---
description: StreamingFast Substreams modules
---

# Modules

### What are Modules?

Modules are small pieces of code, running in a WebAssembly virtual machine. Modules exist within the stream of blocks arriving from a blockchain node.&#x20;

Modules can also process network history from flat files backed by StreamingFast Firehose. See [Firehose documentation](http://firehose.streamingfast.io/) for additional information.

Modules can have one or more inputs. The inputs can be in the form of a `map` or `store,` or a `Block` or `Clock` received from the blockchain's data source.

{% hint style="info" %}
Multiple inputs are made possible because blockchains are clocked. Blockchains allow synchronization between multiple execution streams opening up great performance improvements over comparable traditional streaming engines.
{% endhint %}

Modules have a single output, that can be typed, to inform consumers what to expect and how to interpret the bytes being sent from the module.

Modules can be formed into a graph. Data that is output from one module is used as the input for the subsequent module.

In the diagram shown below the `transfer_map` module extracts all transfers in each `Block,` and the  `transfer_count` store module tracks the number of transfers that have occurred.

{% embed url="https://mermaid.ink/svg/pako:eNp1kM0KwjAQhF8l7NkWvEbwIPUJ9NYUWZKtLTZJ2WwEEd_dCAr-4GFhd_h2GOYKNjoCDUfGeVD7ZmWCUqmvSQZiyr6Wy0z1eVlvpmhPbYqZLen_RKeqaq2EMaSe-OBxfhi-320Z_aF8_diYgxC3SSKT_tE7WIAn9ji6kvv6sDdQsngyoMvqqMc8iQETbgXNs0OhrRuLG-gep0QLwCxxdwkWtHCmF9SMWGrwT-p2B02rZZY" %}
Substreams modules data interaction digram
{% endembed %}

Modules can also take in multiple inputs as seen in the `counters` store example diagram below. Two modules feed into a `store`, effectively tracking multiple `counters`.

{% embed url="https://mermaid.ink/svg/pako:eNqdkE1qAzEMha9itE4GsnWgi5KcINmNh6LamozJeGxsuSGE3L1KW1PIptCdnnjv088NbHQEGk4Z06SOu61ZlHqfoz33JdZsSasydsQTZaqh42ui7mPTvT4cg1qvX1TA9HbxPLmMF5zLv_KOUiyev8JPvF60fm5-J22sC1MufeGYZVDTQ8M07C-jdf4AwAoC5YDeyWtuD5wBOSGQAS2loxHrzAbMchdrTQ6Z9s4LBfQo-9EKsHI8XBcLmnOlZtp5lE-HH9f9EylZic0" %}
Modules with multiple inputs diagram
{% endembed %}

All of the modules are executed as a directed acrylic graph (DAG) each time a new `Block` is processed.

_Note, The top-level data source is always a protocol's `Block` protobuf model, and is deterministic in its execution._

## Module Types

Substreams uses two types of modules, `map` and `store`. Map modules send and receive bytes. Store modules are stateful. They save and track data using simple key-value stores.

### Store Modules

Store modules write to key-value stores.&#x20;

To ensure successful and proper parallelization store modules are not permitted to read any of their own data or values.

Stores that declare their own data types will have methods exposed that are able to mutate the store's keys.

The two important properties that exist stores are `valueType,`and `updatePolicy`.

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

_Note, all update policies provide the `delete_prefix` method._

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

### Store Modules

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
