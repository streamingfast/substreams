# Modules

Modules are small pieces of code, running in a WebAssembly virtual machine, amidst the stream of blocks arriving from a blockchain node. They can also process the network's history out of flat files, backed by the Firehose. See the [Firehose documentation](http://firehose.streamingfast.io/) for more details.

Modules may have one or more inputs (from multiple modules, be them `map`s or `store`s, and/or from the blockchain's data source in the form of a _Block_ or a _Clock_).

{% hint style="info" %}
Multiple inputs are made possible because blockchains are clocked, and they allow synchronization between multiple execution streams, opening up great performance improvements even over your comparable traditional streaming engine.
{% endhint %}

Modules have a single output, that can be typed, to inform consumers what to expect and how to interpret the bytes coming out.

Modules can form a graph of modules, taking each other's output as the next module's input, like so:

{% embed url="https://mermaid.ink/svg/pako:eNp1kM0KwjAQhF8l7NkWvEbwIPUJ9NYUWZKtLTZJ2WwEEd_dCAr-4GFhd_h2GOYKNjoCDUfGeVD7ZmWCUqmvSQZiyr6Wy0z1eVlvpmhPbYqZLen_RKeqaq2EMaSe-OBxfhi-320Z_aF8_diYgxC3SSKT_tE7WIAn9ji6kvv6sDdQsngyoMvqqMc8iQETbgXNs0OhrRuLG-gep0QLwCxxdwkWtHCmF9SMWGrwT-p2B02rZZY" %}
The `transfer_map` module could extract all transfers in each Block, and  `transfer_count` - a`store` module - could keep track of how many transfers occurred.
{% endembed %}

Modules can also take in multiple inputs, like this `counters` store:

{% embed url="https://mermaid.ink/svg/pako:eNqdkE1qAzEMha9itE4GsnWgi5KcINmNh6LamozJeGxsuSGE3L1KW1PIptCdnnjv088NbHQEGk4Z06SOu61ZlHqfoz33JdZsSasydsQTZaqh42ui7mPTvT4cg1qvX1TA9HbxPLmMF5zLv_KOUiyev8JPvF60fm5-J22sC1MufeGYZVDTQ8M07C-jdf4AwAoC5YDeyWtuD5wBOSGQAS2loxHrzAbMchdrTQ6Z9s4LBfQo-9EKsHI8XBcLmnOlZtp5lE-HH9f9EylZic0" %}
Two modules feed into a `store` which keeps track of multiple counters.
{% endembed %}

All of the modules are executed as a DAG, each time a new Block is processed.

The top-level data source is always a protocol's `Block` protobuf model, and is deterministic in its execution.

## Module Types

There are two types of modules, a `map` module, and a `store` module.

### The `map` module type

A `map` module takes bytes in, and outputs bytes. In the [manifest](../reference/manifest.md), you would declare the protobuf types to help users decode the streams, and help generate some code to get you off the ground faster.

### The `store` module type

A `store` module is different from a `map` in that it is a _stateful_ module. It holds and builds a simple and fast _key/value_ store.

#### Writing

A  `kind: store` module's code is able to write to the key/value store, but - in order to ensure parallelization is always possible and deterministic - it _cannot read_ any of its values.&#x20;

A store can also declare its data type, in which case different methods become available to mutate its keys.

Two important properties exist on the `store`:

1. The `valueType`
2. The `updatePolicy`

The first, `valueType`, instructs the Substreams runtime of the data that will be stored in the stores:

| Value                          | Description                                                                      |
| ------------------------------ | -------------------------------------------------------------------------------- |
| `bytes`                        | A simple list of bytes                                                           |
| `string`                       | A UTF-8 string                                                                   |
| `proto:fully.qualified.Object` | Bytes that can be decoded using the protobuf definition `fully.qualified.Object` |
| `int64`                        | A string-serialized integer, that uses int64 arithmetic operations.              |
| `float64`                      | A string-serialized floating point value, using float64 arithmetic operations.   |
| `bigint`                       | A string-serialized integer, with precision of any depth                         |
| `bigfloat`                     | A string-serialized floating point value, with a precision up to 100 digits.     |

The second, `updatePolicy` determines what methods are available in the runtime, as well as the merging strategy for identical keys found in two contiguous stores produced by parallel processing:

| Method              | Supported Value Types                    | Merge strategy\*          |
| ------------------- | ---------------------------------------- | ------------------------- |
| `set`               | `bytes`, `string`, `proto:...`           | The last key wins         |
| `set_if_not_exists` | `bytes`, `string`, `proto:...`           | The first key wins        |
| `add`               | `int64`, `bigint`, `bigfloat`, `float64` | Values are summed up      |
| `min`               | `int64`, `bigint`, `bigfloat`, `float64` | The lowest value is kept  |
| `max`               | `int64`, `bigint`, `bigfloat`, `float64` | The highest value is kept |

All update policies provide the `delete_prefix` method.

{% hint style="info" %}
The **merge strategy** is applied when, while doing parallel processing, a module has built two _partial_ stores store with keys for a segment A (say blocks 0-1000) and a contiguous segment B (say blocks 1000-2000), and is ready to merge those two _partial_ stores to make it a _complete_ store.

The _complete_ store should be exactly as it would be if processing had been done linearly, processing from block 0 up to 2000.&#x20;
{% endhint %}

{% hint style="warning" %}
To preserve the parallelization capabilities of the system, you can never _read_ what you have written, nor read from a store that you are currently writing to.

To read from a store, create a downstream module with one of its inputs pointing to the store's output.
{% endhint %}

#### Ordinals

You will see `ordinal` or `ord` in different methods of the store APIs.

Ordinals allows a key/value store to have multiple versions of a key within a single block. For example, the price for a token could change after transaction B and transaction D, and a downstream module might want to know the value of a key before transaction B and between B and D. That is why you will need to set an ordinal each time you set a key.

{% hint style="warning" %}
You can only set keys in increasing _ordinal_ order.&#x20;
{% endhint %}

If you want to have a single key per block, and you don't care about ordering in your store, you can safely use an _ordinal_ value of `0`.

#### Reading

When declaring a `store` as an input to a module, you can consume its data in one of two modes:

1. `get`
2. `deltas`

The first mode - `get` - provides your module with the _key/value_ store guaranteed to be in sync up to the block being processed, readily queried by methods such as `get_at`, `get_last` and `get_first` (see the [modules API docs](../reference/api-reference.md)) from your module's Rust code. Lookups are local, in-memory, and very fast.

{% hint style="info" %}
The fastest is `get_last` as it queries the store directly. `get_first` will first go through the current block's _deltas_ in reverse order, before querying the store. `get_at` will unwind deltas up to a certain ordinal, so you can get values for keys set midway through a block.
{% endhint %}

The second mode - `deltas` - provides your module with all the _changes_ that occurred in the source `store` module. See the [protobuf model here](../../proto/sf/substreams/v1/substreams.proto#L110). You are then free to pick up on updates, creates, and deletes of the different keys that were mutated during that block.

When a store is set as an input to your module, you can only _read_ from it, not write back to it.
