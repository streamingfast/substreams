---
description: StreamingFast Substreams module handler creation
---

# Module handler creation

## Module handler creation overview

After generating the ABI and protobuf Rust code, you need to write the handler code. Save the code into the `src` directory and use the filename [`lib.rs`](https://github.com/streamingfast/substreams-template/blob/develop/src/lib.rs).

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

View the [`lib.rs`](https://github.com/streamingfast/substreams-template/blob/develop/src/lib.rs) file in the repository.

### **Module handler breakdown**

The logical sections of the [`lib.rs`](https://github.com/streamingfast/substreams-template/blob/develop/src/lib.rs) file are outlined and described in greater detail.

Import the necessary modules.

{% code title="lib.rs excerpt" overflow="wrap" %}
```rust
mod abi;
mod pb;
use hex_literal::hex;
use pb::erc721;
use substreams::{log, store, Hex};
use substreams_ethereum::{pb::eth::v2 as eth, NULL_ADDRESS, Event};
```
{% endcode %}

Store the tracked contract in the example in a `constant`.

{% code title="lib.rs excerpt" %}
```rust
const TRACKED_CONTRACT: [u8; 20] = hex!("bc4ca0eda7647a8ab7c2061c2e118a18a936f13d");
```
{% endcode %}

Define the `map` module.

{% code title="manifest excerpt" %}
```yaml
- name: map_transfers
  kind: map
  initialBlock: 12287507
  inputs:
    - source: sf.ethereum.type.v2.Block
  output:
    type: proto:eth.erc721.v1.Transfers
```
{% endcode %}

Notice the: `name: map_transfers`, the module in the manifest name matches the handler function name. Also notice, there is one [`inputs`](inputs.md) and one [`output`](outputs.md) definition.

The [`inputs`](inputs.md) uses the standard Ethereum Block, `sf.ethereum.type.v2.Block,` provided by the [`substreams-ethereum` crate](https://crates.io/crates/substreams-ethereum-core).

The output uses the `type` `proto:eth.erc721.v1.Transfers` which is a custom protobuf definition provided by the generated Rust code.

The function signature produced resembles:

{% code title="lib.rs excerpt" %}
```rust
#[substreams::handlers::map]
fn map_transfers(blk: eth::Block) -> Result<erc721::Transfers, substreams::errors::Error> {
    ...
}
```
{% endcode %}

### **Rust macros**

Did you notice the `#[substreams::handlers::map]` on top of the function? It is a [Rust macro](https://doc.rust-lang.org/book/ch19-06-macros.html) provided by the [`substreams` crate](https://docs.rs/substreams/latest/substreams/).

The macro decorates the handler function as a `map.` Define `store` modules by using the syntax `#[substreams::handlers::store]`.

### Module handler function

The `map` extracts ERC721 transfers from a _`Block`_ object. The code finds all the `Transfer` `events` emitted by the tracked smart contract. As the events are encountered they are decoded into `Transfer` objects.

{% code title="lib.rs excerpt" overflow="wrap" %}
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
{% endcode %}

Define the `store` module.

{% code title="manifest excerpt" %}
```yaml
- name: store_transfers
  kind: store
  initialBlock: 12287507
  updatePolicy: add
  valueType: int64
  inputs:
    - map: map_transfers
```
{% endcode %}

{% hint style="info" %}
**Note:** `name: store_transfers` corresponds to the handler function name.
{% endhint %}

The `inputs` corresponds to the `output` of the `map_transfers` `map` module typed as `proto:eth.erc721.v1.Transfers`. The custom protobuf definition is provided by the generated Rust code.

{% code title="lib.rs excerpt" %}
```rust
#[substreams::handlers::store]
fn store_transfers(transfers: erc721::Transfers, s: store::StoreAddInt64) {
    ...
}
```
{% endcode %}

{% hint style="info" %}
**Note**: __ the `store` always receives itself as its own last input.
{% endhint %}

In the example the `store` module uses an `updatePolicy` set to `add` and a `valueType set` to `int64` yielding a writable `store` typed as `StoreAddInt64`.

{% hint style="info" %}
**Note**: **Store types**

* The writable `store` is always the last parameter of a `store` module function.
* The `type` of the writable `store` is determined by the `updatePolicy` and `valueType` of the `store` module.
{% endhint %}

The goal of the `store` in the example is to track a holder's current NFT `count` for the smart contract provided. The tracking is achieved through the analysis of `Transfers`.

**`Transfers` in detail**

* If the "`from`" address of the `transfer` is the `null` address (`0x0000000000000000000000000000000000000000`) and the "`to`" address is not the `null` address, the "`to`" address is minting a token, which results in the `count` being incremented.
* If the "`from`" address of the `transfer` is not the `null` address and the "`to`" address is the `null` address, the "`from`" address has burned a token, which results in the `count` being decremented.
* If both the "`from`" and the "`to`" address is not the `null` address, the `count` is decremented from the "`from`" address and incremented for the "`to`" address.

### `store` concepts

There are three important things to consider when writing to a `store`:

* `ordinal`
* `key`
* `value`

#### `ordinal`

`ordinal` represents the order in which the `store` operations are applied.

The `store` handler is called once per `block.`

The `add` operation may be called multiple times during execution, for various reasons such as discovering a relevant event or encountering a call responsible for triggering a method call.

{% hint style="info" %}
**Note**: Blockchain execution models are linear. Operations to add must be added linearly and deterministically.
{% endhint %}

If an `ordinal` is specified, the order of execution is guaranteed. In the example, when the `store` handler is executed by a given set of `inputs`, such as a list of `Transfers`, it emits the same number of `add` calls and `ordinal` values for the execution.

#### `key`

Stores are [key-value stores](https://en.wikipedia.org/wiki/Key%E2%80%93value\_database). Care needs to be taken when crafting a `key` to ensure it is unique **and flexible**.

If the `generate_key` function in the example returns the `TRACKED_CONTRACT` address as the `key`, it is not unique among different token holders.

The `generate_key` function returns a unique `key` for holders if it contains only the holder's address.

{% hint style="warning" %}
**Important**: Issues are expected when attempting to track multiple contracts.
{% endhint %}

#### `value`

The value being stored. The `type` is dependent on the `store` `type` being used.

{% code title="lib.rs excerpt" overflow="wrap" %}
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

One handler function for extracting relevant _`transfers`_, and a second to store the token count per recipient.

Build Substreams to continue the setup process.

```bash
cargo build --target wasm32-unknown-unknown --release
```

The next step is to run Substreams with all of the changes made by using the generated code.
