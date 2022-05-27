# Rust APIs

The `substreams` Rust crate provides the following APIs:

* a `store` API to save data in specific type of stored
* a `log` API to log debug messages
* a `hex` API to manipulate and print hex types
* ~~a `rpc` API to perform ETH rpc calls~~

## Documentation

See the [official crate documentation](https://docs.rs/substreams).&#x20;

## Store API

The `store` API exposes different types of store, that you can leverage to create your `store` modules. The type of store your `store` module will use is based on the [`updatePolicy`](../manifests.md#modules-.updatepolicy) and  [`valueType`](../manifests.md#modules-.valuetype) of your store module.  Each `writable` store type are constrained in the way they are to enable high parallelization of processes.

When processing segments of history in parallel, two partial stores have a merge or squashing strategy particular to their data type, and/or the way keys are set.

The **merge strategy** below explains what happens when we have two stores that processed segments of history in parallel, that need to be squashed together.

#### Ordinal

You will notice Store functions usually take an `ordinal`. This is because `store`s keep track of changes to the key/values inside a block, and produces [_StoreDeltas_ as referenced here](../../proto/sf/substreams/v1/substreams.proto). This allows keys to be set multiple times in a module that is dealing with multiple transactions.

The `ordinal` is therefore an index that helps sort and order events from multiple modules written by different people, around the `ordinal` of each event in the blockchain data.

> In a traditional blockchain Block, think of ordinals as a number that would increase each time a transaction starts, each time there is a change to the state, a change to some balances, a new internal transaction, a transaction that terminates, or any event that can be ordered relative to one another.
>
> See the [Ethereum data model](https://github.com/streamingfast/sf-ethereum/blob/develop/proto/sf/ethereum/type/v1/type.proto), and search for `ordinal` for an example.

### UpdateWriter

```rust
impl UpdateWriter {    
    pub fn set(&self, ord: i64, key: String, value: &Vec<u8>)
    pub fn delete_prefix(&self, ord: i64, prefix: &String)
}
```

`UpdateWriter` can only be called on `store` modules defined with `updatePolicy`=`replace`

**`set(&self, ord: i64, key: String, value: &Vec<u8>)`**

The `set` function will simply set a given key to a given value. And if the key existed before, it will be replaced.

**merge strategy**: _<mark style="color:red;">last key wins</mark>_

### Conditional Writer

```rust
impl ConditionalWriter {    
    pub fn set_if_not_exists(&self, ord: i64, key: String, value: &Vec<u8>)
    pub fn delete_prefix(&self, ord: i64, prefix: &String)
}
```

`ConditionalWriter` can only be called on `store` modules defined with `updatePolicy` = `ignore`

```rust
pub fn set_if_not_exists(&self, ord: i64, key: String, value: &Vec<u8>)
```

The `set_if_not_exists` function sets a key. If the key existed before, however, it will be ignored and not set.

```rust
pub fn delete_prefix(&self, ord: i64, prefix: &String)
```

The `delete_prefix` functions allows one to delete a set of keys by prefix. Do not use this to delete individual keys if you want consistent highly performant parallelized operations. Rather, design key spaces where you can delete large number of keys in one swift using a meaningful prefix.

**merge strategy**: _<mark style="color:red;">first key wins</mark>_<mark style="color:red;">.</mark>

### SumInt64Writer

```rust
impl SumInt64Writer {
    pub fn sum(&self, ord: i64, key: String, value: i64)
    pub fn delete_prefix(&self, ord: i64, prefix: &String)
}
```

`SumInt64Writer` can only be called on `store` modules defined with `updatePolicy` = `sum` and `valueType` = `int64`

**`int64`**: integer `value` as native _i64_ WASM type param; sum operation using native 64 bits arithmetic; store value as string.&#x20;

```rust
pub fn sum(&self, ord: i64, key: String, value: i64)
```

the `sum` function will sum the value already present in `key` (or default to zero if the key was not present).

```rust
pub fn delete_prefix(&self, ord: i64, prefix: &String)
```

The `delete_prefix` functions allows one to delete a set of keys by prefix. Do not use this to delete individual keys if you want consistent highly performant parallelized operations. Rather, design key spaces where you can delete large number of keys in one swift using a meaningful prefix.

**merge strategy**: <mark style="color:red;">sum up the same keys present in both stores, using the appropriate arithmetic for the type</mark>

### SumFloat64Writer

```rust
impl SumFloat64Writer {
    pub fn sum(&self, ord: i64, key: String, value: f64)
    pub fn delete_prefix(&self, ord: i64, prefix: &String)
}
```

`SumFloat64Writer` can only be called on `store` modules defined with `updatePolicy` = `sum` and `valueType` = `float64`

**`float64`**: float `value` as native _f64_ WASM type param; sum operation using native 64 bits arithmetic; store value as string.&#x20;

```rust
pub fn sum(&self, ord: i64, key: String, value: f64)
```

the `sum` function will sum the value already present in `key` (or default to zero if the key was not present).

```rust
pub fn delete_prefix(&self, ord: i64, prefix: &String)
```

The `delete_prefix` functions allows one to delete a set of keys by prefix. Do not use this to delete individual keys if you want consistent highly performant parallelized operations. Rather, design key spaces where you can delete large number of keys in one swift using a meaningful prefix.

**merge strategy**: <mark style="color:red;">sum up the same keys present in both stores, using the appropriate arithmetic for the type</mark>

### SumBigIntWriter

```rust
impl SumBigIntWriter {
    pub fn sum(&self, ord: i64, key: String, value: &BigInt)
    pub fn delete_prefix(&self, ord: i64, prefix: &String)
}
```

`SumBigIntWriter` can only be called on `store` modules defined with `updatePolicy` = `sum` and `valueType` = `bigint`

**`bigint`**: integer `value` as string param; sum operations using BigInt arithmetics; store value as string.

```rust
pub fn sum(&self, ord: i64, key: String, value: &BigInt)
```

the `sum` function will sum the value already present in `key` (or default to zero if the key was not present).

```rust
pub fn delete_prefix(&self, ord: i64, prefix: &String)
```

The `delete_prefix` functions allows one to delete a set of keys by prefix. Do not use this to delete individual keys if you want consistent highly performant parallelized operations. Rather, design key spaces where you can delete large number of keys in one swift using a meaningful prefix.

**merge strategy**: <mark style="color:red;">sum up the same keys present in both stores, using the appropriate arithmetic for the type</mark>

### SumBigFloatWriter

```rust
impl SumBigFloatWriter {
    pub fn sum(&self, ord: i64, key: String, value: &BigDecimal)
    pub fn delete_prefix(&self, ord: i64, prefix: &String)
}
```

`SumBigFloatWriter` can only be called on `store` modules defined with `updatePolicy` = `sum` and `valueType` = `bigfloat`

**`bigfloat`**: floating point `value` as string param; sum operations using 100 decimals BigFloat arithmetic; store value as string.

```rust
pub fn sum(&self, ord: i64, key: String, value: &BigDecimal)
```

the `sum` function will sum the value already present in `key` (or default to zero if the key was not present).

```rust
pub fn delete_prefix(&self, ord: i64, prefix: &String)
```

The `delete_prefix` functions allows one to delete a set of keys by prefix. Do not use this to delete individual keys if you want consistent highly performant parallelized operations. Rather, design key spaces where you can delete large number of keys in one swift using a meaningful prefix.

**merge strategy**: <mark style="color:red;">sum up the same keys present in both stores, using the appropriate arithmetic for the type</mark>

### MaxInt64Writer

```rust
impl MaxInt64Writer {
    pub fn max(&self, ord: i64, key: String, value: i64)
    pub fn delete_prefix(&self, ord: i64, prefix: &String)
}
```

`MaxInt64Writer` can only be called on `store` modules defined with `updatePolicy` = `max` and `valueType` = `int64`

```rust
pub fn max(&self, ord: i64, key: String, value: i64)
```

`max` functions will set the provided `key` in the _store_ only if the `value` received in parameter is bigger than the one already present in the store, with a default of the zero value when the key is absent.

```rust
pub fn delete_prefix(&self, ord: i64, prefix: &String)
```

The `delete_prefix` functions allows one to delete a set of keys by prefix. Do not use this to delete individual keys if you want consistent highly performant parallelized operations. Rather, design key spaces where you can delete large number of keys in one swift using a meaningful prefix.

**Merge strategy**: <mark style="color:red;">keeps the key that contains the biggest value</mark>

### MaxFloat64Writer

```rust
impl MaxFloat64Writer {
    pub fn max(&self, ord: i64, key: String, value: f64)
    pub fn delete_prefix(&self, ord: i64, prefix: &String)
}
```

`MaxFloat64Writer` can only be called on `store` modules defined with `updatePolicy` = `max` and `valueType` = `float64`

```rust
pub fn max(&self, ord: i64, key: String, value: f64)
```

`max` functions will set the provided `key` in the _store_ only if the `value` received in parameter is bigger than the one already present in the store, with a default of the zero value when the key is absent.

```rust
pub fn delete_prefix(&self, ord: i64, prefix: &String)
```

The `delete_prefix` functions allows one to delete a set of keys by prefix. Do not use this to delete individual keys if you want consistent highly performant parallelized operations. Rather, design key spaces where you can delete large number of keys in one swift using a meaningful prefix.

**Merge strategy**: <mark style="color:red;">keeps the key that contains the biggest value</mark>

### MaxBigIntWriter

```rust
impl MaxBigIntWriter {
    pub fn max(&self, ord: i64, key: String, value: &BigInt)
    pub fn delete_prefix(&self, ord: i64, prefix: &String)
}
```

`MaxBigIntWriter` can only be called on `store` modules defined with `updatePolicy` = `max` and `valueType` = `bigint`

```rust
pub fn max(&self, ord: i64, key: String, value: &BigInt)
```

`max` functions will set the provided `key` in the _store_ only if the `value` received in parameter is bigger than the one already present in the store, with a default of the zero value when the key is absent.

```rust
pub fn delete_prefix(&self, ord: i64, prefix: &String)
```

The `delete_prefix` functions allows one to delete a set of keys by prefix. Do not use this to delete individual keys if you want consistent highly performant parallelized operations. Rather, design key spaces where you can delete large number of keys in one swift using a meaningful prefix.

**Merge strategy**: <mark style="color:red;">keeps the key that contains the biggest value</mark>

### MaxBigFloatWriter

```rust
impl MaxBigFloatWriter {
    pub fn max(&self, ord: i64, key: String, value: &BigDecimal)
    pub fn delete_prefix(&self, ord: i64, prefix: &String)
}
```

`MaxBigFloatWriter` can only be called on `store` modules defined with `updatePolicy` = `max` and `valueType` = `bigint`

```rust
pub fn max(&self, ord: i64, key: String, value: &BigDecimal)
```

`max` functions will set the provided `key` in the _store_ only if the `value` received in parameter is bigger than the one already present in the store, with a default of the zero value when the key is absent.

```rust
pub fn delete_prefix(&self, ord: i64, prefix: &String)
```

The `delete_prefix` functions allows one to delete a set of keys by prefix. Do not use this to delete individual keys if you want consistent highly performant parallelized operations. Rather, design key spaces where you can delete large number of keys in one swift using a meaningful prefix.

**Merge strategy**: <mark style="color:red;">keeps the key that contains the smallest value</mark>

### MinInt64Writer

```rust
impl MinInt64Writer {
    pub fn min(&self, ord: i64, key: String, value: i64)
    pub fn delete_prefix(&self, ord: i64, prefix: &String)
}
```

`MinInt64Writer` can only be called on `store` modules defined with `updatePolicy` = `min` and `valueType` = `int64`

```rust
pub fn min(&self, ord: i64, key: String, value: i64)
```

`min` functions will set the provided `key` in the _store_ only if the `value` received in parameter is smaller than the one already present in the store, with a default of the zero value when the key is absent.

```rust
pub fn delete_prefix(&self, ord: i64, prefix: &String)
```

The `delete_prefix` functions allows one to delete a set of keys by prefix. Do not use this to delete individual keys if you want consistent highly performant parallelized operations. Rather, design key spaces where you can delete large number of keys in one swift using a meaningful prefix.

**Merge strategy**: <mark style="color:red;">keeps the key that contains the smallest value</mark>

### MinFloat64Writer

```rust
impl MinFloat64Writer {
    pub fn min(&self, ord: i64, key: String, value: f64)
    pub fn delete_prefix(&self, ord: i64, prefix: &String)
}
```

`MinFloat64Writer` can only be called on `store` modules defined with `updatePolicy` = `min` and `valueType` = `float64`

```rust
pub fn min(&self, ord: i64, key: String, value: f64)
```

`min` functions will set the provided `key` in the _store_ only if the `value` received in parameter is smaller than the one already present in the store, with a default of the zero value when the key is absent.

```rust
pub fn delete_prefix(&self, ord: i64, prefix: &String)
```

The `delete_prefix` functions allows one to delete a set of keys by prefix. Do not use this to delete individual keys if you want consistent highly performant parallelized operations. Rather, design key spaces where you can delete large number of keys in one swift using a meaningful prefix.

**Merge strategy**: <mark style="color:red;">keeps the key that contains the smallest value</mark>

### MinBigIntWriter

```rust
impl MinBigIntWriter {
    pub fn min(&self, ord: i64, key: String, value: &BigInt)
    pub fn delete_prefix(&self, ord: i64, prefix: &String)
}
```

`MinBigIntWriter` can only be called on `store` modules defined with `updatePolicy` = `min` and `valueType` = `bigint`

```rust
pub fn min(&self, ord: i64, key: String, value: &BigInt)
```

`min` functions will set the provided `key` in the _store_ only if the `value` received in parameter is smaller than the one already present in the store, with a default of the zero value when the key is absent.

```rust
pub fn delete_prefix(&self, ord: i64, prefix: &String)
```

The `delete_prefix` functions allows one to delete a set of keys by prefix. Do not use this to delete individual keys if you want consistent highly performant parallelized operations. Rather, design key spaces where you can delete large number of keys in one swift using a meaningful prefix.

**Merge strategy**: <mark style="color:red;">keeps the key that contains the smallest value</mark>

### MinBigFloatWriter

```rust
impl MinBigFloatWriter {
    pub fn min(&self, ord: i64, key: String, value: &BigDecimal)
    pub fn delete_prefix(&self, ord: i64, prefix: &String)
}
```

`MinBigFloatWriter` can only be called on `store` modules defined with `updatePolicy` = `min` and `valueType` = `bigfloat`

```rust
pub fn min(&self, ord: i64, key: String, value: &BigDecimal)
```

`min` functions will set the provided `key` in the _store_ only if the `value` received in parameter is smaller than the one already present in the store, with a default of the zero value when the key is absent.

```rust
pub fn delete_prefix(&self, ord: i64, prefix: &String)
```

The `delete_prefix` functions allows one to delete a set of keys by prefix. Do not use this to delete individual keys if you want consistent highly performant parallelized operations. Rather, design key spaces where you can delete large number of keys in one swift using a meaningful prefix.

**Merge strategy**: <mark style="color:red;">keeps the key that contains the smallest value</mark>

### Reader

```rust
impl Reader {
    pub fn get_at(&self, ord: i64, key: &String) -> Option<Vec<u8>>
    pub fn get_first(&self, key: &String) -> Option<Vec<u8>>
    pub fn get_last(&self, key: &String) -> Option<Vec<u8>>
}
```

The `Reader` store is only available when a `store` has been declared as a dependency in the `inputs` section of a module. It is not possible to read stores while you are writing them. You can however, write multiple store modules that depend on one another to achieve something similar. The reason is to keep parallelization possible.

&#x20;The read functions are When reading from a store, the runtime guarantees that the store is ready and has been processed from its `startBlock` onwards; that keys made available for query reflect linear processing of all history between its `startBlock` and the block currently being processed.

```rust
pub fn get_at(&self, ord: i64, key: &String) -> Option<Vec<u8>>
```

`get_at` allows you to read a single `key` from the store. The type of its value can be anything, and is usually declared in the `output` section of the [manifest](../manifests.md).

The `ordinal` is used here to go query a key that might have changed mid-block by the `store` module that built it.

```rust
pub fn get_first(&self, key: &String) -> Option<Vec<u8>>
```

`get_first` also retrieves a key from the `store`, like `get_at`, but querying the state of the store as of the beginning of the block being processed, before any changes were applied within the current block.

However, it needs to unwind any keys that would have changed mid-block, so will be slightly less performant.

```rust
pub fn get_last(&self, key: &String) -> Option<Vec<u8>>
```

`get_last` is the fastest as it does not need to rewind any changes in the middle of the block.

## Log API

the `log` API exposes logging functions, that allow substream developer to log messages at `INFO` & `DEBUG` severity on the current substream's logger using interpolation of runtime expressions

```rust
log::info!("test");
log::info!("hello {}", "world!");
log::info!("x = {}, y = {y}", 10, y = 30);
log::debug!("test");
log::debug!("hello {}", "world!");
log::debug!("x = {}, y = {y}", 10, y = 30);
```

the `log` function `panics` if a formatting trait implementation returns an error.

## Hex API

the `hex` API exposes helpful function for `encoding`, `decoding` and printing hexadecimal types.&#x20;

## RPC Api

the `RPC` api is available on the ethereum implementation of Substreams import to do Ethereum `eth_calls`

```rust
pub fn eth_call(input: Vec<u8>) -> Vec<u8> 
```

* `input`: the input parameter is a protobuf encoded `substreams::pb::eth::RpcCalls`
* `output`: the output is the protobuf encoded `substreams::pb::eth::RpcResponses`

below is an example where we are performing multiple `eth_call` to retrieve an ERC20 `name`, `symbol` & \`decimals

```rust
pub fn rpc_calls(pair_token_address: &String) -> Token {
    let rpc_calls = create_rpc_calls(&address_decode(pair_token_address));

    let rpc_responses_marshalled: Vec<u8> =
        substreams::rpc::eth_call(substreams::proto::encode(&rpc_calls).unwrap());
    let rpc_responses_unmarshalled: substreams::pb::eth::RpcResponses =
        substreams::proto::decode(&rpc_responses_marshalled).unwrap();

    if rpc_responses_unmarshalled.responses[0].failed
        || rpc_responses_unmarshalled.responses[1].failed
        || rpc_responses_unmarshalled.responses[2].failed
    {
        panic!(
            "not a token because of a failure: {}",
            address_pretty(pair_token_address.as_bytes())
        )
    };

    if !(rpc_responses_unmarshalled.responses[1].raw.len() >= 96)
        || rpc_responses_unmarshalled.responses[0].raw.len() != 32
        || !(rpc_responses_unmarshalled.responses[2].raw.len() >= 96)
    {
        panic!(
            "not a token because response length: {}",
            address_pretty(pair_token_address.as_bytes())
        )
    };

    let decoded_decimals = decode_uint32(rpc_responses_unmarshalled.responses[0].raw.as_ref());
    let decoded_name = decode_string(rpc_responses_unmarshalled.responses[1].raw.as_ref());
    let decoded_symbol = decode_string(rpc_responses_unmarshalled.responses[2].raw.as_ref());

    Token {
        address: pair_token_address.to_string(),
        name: decoded_name,
        symbol: decoded_symbol,
        decimals: decoded_decimals as u64,
    }
}


pub fn create_rpc_calls(addr: &Vec<u8>) -> substreams::pb::eth::RpcCalls {
    let decimals = hex::decode("313ce567").unwrap();
    let name = hex::decode("06fdde03").unwrap();
    let symbol = hex::decode("95d89b41").unwrap();

    return substreams::pb::eth::RpcCalls {
        calls: vec![
            substreams::pb::eth::RpcCall {
                to_addr: Vec::from(addr.clone()),
                method_signature: decimals,
            },
            substreams::pb::eth::RpcCall {
                to_addr: Vec::from(addr.clone()),
                method_signature: name,
            },
            substreams::pb::eth::RpcCall {
                to_addr: Vec::from(addr.clone()),
                method_signature: symbol,
            },
        ],
    };
}

let as_array: [u8; 4] = input[28..32].try_into().unwrap();
    u32::from_be_bytes(as_array)
}

pub fn decode_string(input: &[u8]) -> String {
    if input.len() < 96 {
        panic!("input length too small: {}", input.len());
    }

    let next = decode_uint32(&input[0..32]);
    if next != 32 {
        panic!("invalid input, first part should be 32");
    };

    let size: usize = decode_uint32(&input[32..64]) as usize;
    let end: usize = (size) + 64;

    if end > input.len() {
        panic!(
            "invalid input: end {:?}, length: {:?}, next: {:?}, size: {:?}, whole: {:?}",
            end,
            input.len(),
            next,
            size,
            hex::encode(&input[32..64])
        );
    }

    String::from_utf8_lossy(&input[64..end]).to_string()
}

```
