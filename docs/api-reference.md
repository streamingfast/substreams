API Reference
=============

- [API Reference](#api-reference)
  - [Store functions](#store-functions)
    - [Store _read_ functions](#store-_read_-functions)
      - [`get_at(ordinal: u64, key: String): (bool, [u8])`](#get_atordinal-u64-key-string-bool-u8)
      - [`get_first(key: String) (bool, [u8])`](#get_firstkey-string-bool-u8)
      - [`get_last(key: String) (bool, [u8])`](#get_lastkey-string-bool-u8)
    - [Store _write_ functions](#store-_write_-functions)
      - [`set(ordinal: u64, key: String, value: [u8])`](#setordinal-u64-key-string-value-u8)
      - [`set_if_not_exists(ordinal: u64, key: String, value[u8])`](#set_if_not_existsordinal-u64-key-string-valueu8)
      - [`sum_*` functions](#sum_-functions)
      - [`set_min_*` and `set_max_*` functions](#set_min_-and-set_max_-functions)
    - [Store _delete_ function](#store-_delete_-function)
      - [`delete_prefix(ordinal: u64, key: String)`](#delete_prefixordinal-u64-key-string)
  - [Ethereum-specific imports](#ethereum-specific-imports)
      - [`rpc.eth_call(request: RpcCalls): RpcResponses`](#rpceth_callrequest-rpccalls-rpcresponses)

This document describes the WASM imports (functions made available to
the code you write in Rust, when building Substreams Modules).

These are low-level constructs that are probably abstracted away by
higher level Rust crates and libraries, but show what Substreams are
made of at the more fundamental level.


## Store functions

You will notice Store functions usually take an `ordinal`. This is
because `store`s keep track of changes to the key/values inside a
block, and produces [_StoreDeltas_ as referenced
here](https://github.com/streamingfast/substreams/blob/develop/proto/sf/substreams/v1/substreams.proto). This
allows keys to be set multiple times in a module that is dealing with
multiple transactions.

The `ordinal` is therefore an index that helps sort and order events
from multiple modules written by different people, around the
`ordinal` of each event in the blockchain data.

> In a traditional blockchain Block, think of ordinals as a number
> that would increase each time a transaction starts, each time there
> is a change to the state, a change to some balances, a new internal
> transaction, a transaction that terminates, or any event that can be
> ordered relative to one another.
>
> See the [Ethereum data
> model](https://github.com/streamingfast/sf-ethereum/blob/develop/proto/sf/ethereum/type/v1/type.proto),
> and search for `ordinal` for an example.


### Store _read_ functions

The read functions are only available when a `store` has been declared
as a dependency in the `inputs` section of a module.  It is not
possible to read stores while you are writing them. You can however,
write multiple store modules that depend on one another to achieve
something similar. The reason is to keep parallelization possible.

When reading from a store, the runtime guarantees that the store is
ready and has been processed from its `startBlock` onwards; that keys
made available for query reflect linear processing of all history
between its `startBlock` and the block currently being processed.


#### `get_at(ordinal: u64, key: String): (bool, [u8])`

`get_at` allows you to read a single `key` from the store. The type of
its value can be anything, and is usually declared in the `output`
section of the [manifest](./manifest.md).

The `ordinal` is used here to go query a key that might have changed
mid-block by the `store` module that built it.

#### `get_first(key: String) (bool, [u8])`

`get_first` also retrieves a key from the `store`, like `get_at`, but
querying the state of the store as of the beginning of the block being
processed, before any changes were applied within the current block.

However, it needs to unwind any keys that would have changed
mid-block, so will be slightly less performant.

#### `get_last(key: String) (bool, [u8])`

`get_last` is the fastest as it does not need to rewind any changes in
the middle of the block.

### Store _write_ functions

These methods can be called on `store` modules only, and are
constrained in the way they are to enable high parallelization of
processes.

When processing segments of history in parallel, two partial stores
have a merge or squashing strategy particular to their data type,
and/or the way keys are set.

The **merge strategy** below explains what happens when we have two
stores that processed segments of history in parallel, that need to be
squashed together.

#### `set(ordinal: u64, key: String, value: [u8])`

The `set` function will simply set a given key to a given value. And
if the key existed before, it will be replaced.

It can only be called on `store` modules defined with `updatePolicy:
replace`.

**Merge strategy**: _last key wins_.



#### `set_if_not_exists(ordinal: u64, key: String, value[u8])`

The `set_if_not_exists` function also sets a key. If the existed
before, however, it will be ignored and not set.

It can only be called on `store` modules defined with `updatePolicy:
ignore`

**Merge strategy**: _first key wins_.



#### `sum_*` functions

> `sum_bigfloat(ordinal: u64, key: String, value: String)`<br/>
> `sum_int64(ordinal: u64, key: String, value: i64)`<br/>
> `sum_bigint(ordinal: u64, key: String, value: String)`

`sum_*` functions will sum the value already present in `key` (or
default to zero if the key was not present).

It can only be called on `store` modules defined with `updatePolicy:
sum`.

Data format of the different data types (as specified in `valueType:`
in the manifest):

* **`bigfloat`**: floating point `value` as string param; sum
  operations using 100 decimals BigFloat arithmetic; store value as
  string.
* **`bigint`**: integer `value` as string param; sum operations using
  BigInt arithmetics; store value as string.
* **`int64`**: integer `value` as native _i64_ WASM type param; sum
  operation using native 64 bits arithmetic; store value as string.

**Merge strategy**: Sum up the same keys present in both stores, using
the appropriate arithmetic for each type.



#### `set_min_*` and `set_max_*` functions

> `set_min_bigfloat(ordinal: u64, key: String, value: String)`<br/>
> `set_max_bigfloat(ordinal: u64, key: String, value: String)`<br/>
> `set_min_int64(ordinal: u64, key: String, value: i64)`<br/>
> `set_max_int64(ordinal: u64, key: String, value: i64)`<br/>
> `set_min_bigint(ordinal: u64, key: String, value: String)`<br/>
> `set_max_bigint(ordinal: u64, key: String, value: String)`

`set_min_*` functions will set the provided `key` in the _store_ only
if the `value` received in parameter is _lower_ than the one already
present in the store, with a default of the zero value when the key is
absent. Similarly `set_max_*` will ensure the highest value of both is
assigned to the key.

These methods can only respectively be set if the `store` modules
defines `updatePolicy: min` or `updatePolicy: max`.

The data format of the different data types (`bigfloat`, `bigint` and
`int64` are the same as for the `sum_*` functions above.

**Merge strategy**: _min()_ or _max()_ of the two stores being merged,
using appropriate arithmetic for each type.


### Store _delete_ function

#### `delete_prefix(ordinal: u64, key: String)`

This allows one to delete a set of keys by prefix. It can be used by
any `updatePolicy`, and any `valueType`.

NOTE: Do not use this to delete individual keys if you want consistent
highly performant parallelized operations. Rather, design key spaces
where you can delete large number of keys in one swift using a
meaningful prefix.

**Merge strategy**: apply delete prefixes on previous store



## Ethereum-specific imports

The Ethereum implementation of Substreams provides an additional import to do Ethereum `eth_call`s

#### `rpc.eth_call(request: RpcCalls): RpcResponses`

[insert docs, figure out where that doc should be]

<!-- TODO: Bring back the data model in `sf-ethereum` and point to the .proto definitions, and the `substreams-ethereum` crates eventually. -->
