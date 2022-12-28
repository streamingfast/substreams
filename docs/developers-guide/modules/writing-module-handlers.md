---
description: StreamingFast Substreams module handler creation
---

# Module handler creation

After the ABI and `Protobuf` Rust code has been generated the handler code needs to be written. The code should be saved into the `src` directory and use the filename `lib.rs.`

{% code title="src/lib.rs" overflow="wrap" lineNumbers="true" %}
```rust
mod abi;
mod pb;
use hex_literal::hex;
use pb::erc721;
use substreams::prelude::*;
use substreams::{log, store::StoreAddInt64, Hex};
use substreams_ethereum::{pb::eth::v2 as eth, NULL_ADDRESS};

// Bored Ape Club Contract
const TRACKED_CONTRACT: [u8; 20] = hex!("bc4ca0eda7647a8ab7c2061c2e118a18a936f13d");

substreams_ethereum::init!();

/// Extracts transfers events from the contract
#[substreams::handlers::map]
fn map_transfers(blk: eth::Block) -> Result<erc721::Transfers, substreams::errors::Error> {
    Ok(erc721::Transfers {
        transfers: blk
            .events::<abi::erc721::events::Transfer>(&[&TRACKED_CONTRACT])
            .map(|(transfer, log)| {
                substreams::log::info!("NFT Transfer seen");

                erc721::Transfer {
                    trx_hash: log.receipt.transaction.hash.clone(),
                    from: transfer.from,
                    to: transfer.to,
                    token_id: transfer.token_id.low_u64(),
                    ordinal: log.block_index() as u64,
                }
            })
            .collect(),
    })
}

/// Store the total balance of NFT tokens for the specific TRACKED_CONTRACT by holder
#[substreams::handlers::store]
fn store_transfers(transfers: erc721::Transfers, s: StoreAddInt64) {
    log::info!("NFT holders state builder");
    for transfer in transfers.transfers {
        if transfer.from != NULL_ADDRESS {
            log::info!("Found a transfer out {}", Hex(&transfer.trx_hash));
            s.add(transfer.ordinal, generate_key(&transfer.from), -1);
        }

        if transfer.to != NULL_ADDRESS {
            log::info!("Found a transfer in {}", Hex(&transfer.trx_hash));
            s.add(transfer.ordinal, generate_key(&transfer.to), 1);
        }
    }
}

fn generate_key(holder: &Vec<u8>) -> String {
    return format!("total:{}:{}", Hex(holder), Hex(TRACKED_CONTRACT));
}
```
{% endcode %}

View this file in the repository:

[https://github.com/streamingfast/substreams-template/blob/develop/src/lib.rs](https://github.com/streamingfast/substreams-template/blob/develop/src/lib.rs)

### **Module handler breakdown**

Each logical section of the `lib.rs` file is outlined and described in greater detail below.

Import the necessary modules.

```rust
mod abi;
mod pb;
use hex_literal::hex;
use pb::erc721;
use substreams::{log, store, Hex};
use substreams_ethereum::{pb::eth::v2 as eth, NULL_ADDRESS, Event};
```

Store the contract being tracked in the example as a `constant`.

```rust
const TRACKED_CONTRACT: [u8; 20] = hex!("bc4ca0eda7647a8ab7c2061c2e118a18a936f13d");
```

Define the `map` module. Here is the module definition from the example Substreams manifest.

```yaml
- name: map_transfers
  kind: map
  initialBlock: 12287507
  inputs:
    - source: sf.ethereum.type.v2.Block
  output:
    type: proto:eth.erc721.v1.Transfers
```

Notice the: `name: map_transfers`. This name should correspond to our handler function name.

Also notice, there is one input and one output defined.

The input has a type of `sf.ethereum.type.v2.Block`, a standard Ethereum block provided by the `substreams-ethereum` crate.

The output is typed as `proto:eth.erc721.v1.Transfers`. This is the custom protobuf definition and is provided by the generated Rust code. Resulting in the following function signature.

```rust
#[substreams::handlers::map]
fn map_transfers(blk: eth::Block) -> Result<erc721::Transfers, substreams::errors::Error> {
    ...
}
```

{% hint style="info" %}
**Note**: **Rust Macros**

Notice the `#[substreams::handlers::map]` above the function, this is a Rust "macro" that is provided by the substreams crate.

This macro decorates our handler function as a map. There is also a macro used to decorate the handler of kind store represented as follows.

`#[substreams::handlers::store]`
{% endhint %}

The goal of the `map` being built is for extracting ERC721 transfers from a given block.

This can be achieved by finding all the `Transfer` events that are emitted by the tracked contract. As events are found they will be decoded into `Transfer` objects.

```rust
/// Extracts transfers events from the contract
#[substreams::handlers::map]
fn map_transfers(blk: eth::Block) -> Result<erc721::Transfers, substreams::errors::Error> {
    Ok(erc721::Transfers {
        transfers: blk
            .events::<abi::erc721::events::Transfer>(&[&TRACKED_CONTRACT])
            .map(|(transfer, log)| {
                substreams::log::info!("NFT Transfer seen");

                erc721::Transfer {
                    trx_hash: log.receipt.transaction.hash.clone(),
                    from: transfer.from,
                    to: transfer.to,
                    token_id: transfer.token_id.low_u64(),
                    ordinal: log.block_index() as u64,
                }
            })
            .collect(),
    })
}
```

Define the `store` module. As a reminder, here is the module definition from the example Substreams manifest.

```yaml
- name: store_transfers
  kind: store
  initialBlock: 12287507
  updatePolicy: add
  valueType: int64
  inputs:
    - map: map_transfers
```

{% hint style="info" %}
_Note: `name: store_transfers` will also correspond to the handler function name._
{% endhint %}

The input corresponds to the output of the `map_transfers` `map` module typed as `proto:eth.erc721.v1.Transfers`. This is the custom protobuf definition and is provided by the generated Rust code.

```rust
#[substreams::handlers::store]
fn store_transfers(transfers: erc721::Transfers, s: store::StoreAddInt64) {
    ...
}
```

{% hint style="info" %}
**Note**: __ the `store` will always receive itself as its own last input.
{% endhint %}

In this example the `store` module uses an `updatePolicy` set to `add` and a `valueType set` to `int64` yielding a writable store typed as `StoreAddInt64`.

{% hint style="info" %}
**Note**: **Store Types**

The last parameter of a `store` module function should always be the writable store itself.

The type of the writable store is based on the `store` module `updatePolicy` and `valueType`.
{% endhint %}

The goal of the `store` in this example is to track a holder's current NFT count for the contract supplied. This tracking is achieved through the analysis of transfers.

**Transfer in detail**

If the transfer's `from` address field contains the null address (`0x0000000000000000000000000000000000000000`), and the `to` address field is not the null address, the `to` address field is minting a token, so the count should be incremented.

If the transfer's `from` address field is not the null address, _and_ the `to` address field is the null address, the `from` address field is burning a token, so the count should be decremented.

If the `from` address field and the `to` address field is not a null address, the count should be decremented of the `from` address, and increment the count of the `to` address for basic transfers.

### Store concepts

When writing to a store, there are three concepts to consider that include `ordinal`, `key` and `value`. Additional information for each is provided below.

#### Ordinal

Ordinal represents the order in which the `store` operations will be applied.

The `store` handler will be called once per `block.`

During execution, the `add` operation may be called multiple times, for multiple reasons, such as finding a relevant event or seeing a call that triggered a method call.

Blockchain execution models are linear. Operations to add must be added linearly and deterministically.

When an ordinal is specified, the order of execution is guaranteed. For one execution of the `store` handler for given inputs, in this example a list of transfers, the code will emit the same number of `add` calls and ordinal values.

#### Key

Stores are [key/value stores](https://en.wikipedia.org/wiki/Key%E2%80%93value\_database). Care needs to be taken when crafting a key to ensure that it is unique _and flexible_.

In the example, if the `generate_key` function will return a key that is the `TRACKED_CONTRACT` address it will not be unique between different token holders.

If the `generate_key` function returns a key containing only the holder's address it will be unique amongst holders. Issues will be encountered when attempting to track multiple contracts.

#### Value

The value being stored. The type is dependent on the store type being used.

{% code overflow="wrap" %}
```rust
#[substreams::handlers::store]
fn store_transfers(transfers: erc721::Transfers, s: StoreAddInt64) {
    log::info!("NFT holders state builder");
    for transfer in transfers.transfers {
        if transfer.from != NULL_ADDRESS {
            log::info!("Found a transfer out {}", Hex(&transfer.trx_hash));
            s.add(transfer.ordinal, generate_key(&transfer.from), -1);
        }

        if transfer.to != NULL_ADDRESS {
            log::info!("Found a transfer in {}", Hex(&transfer.trx_hash));
            s.add(transfer.ordinal, generate_key(&transfer.to), 1);
        }
    }
}

fn generate_key(holder: &Vec<u8>) -> String {
    return format!("total:{}:{}", Hex(holder), Hex(TRACKED_CONTRACT));
}
```
{% endcode %}

### Summary

Both handler functions have been written.

One handler function for extracting transfers that are of interest, and a second to store the token count per recipient.

Build Substreams to continue the setup process.

```
cargo build --target wasm32-unknown-unknown --release
```

The next step is to run Substreams with all of the changes made using the code that's been generated.
