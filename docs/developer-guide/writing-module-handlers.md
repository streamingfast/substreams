# Writing Module Handlers

Now that we have our ABI and `Protobuf` Rust code generated, let's write our handler code in `src/lib.rs` as such:

{% code title="src/lib.rs" %}
```rust
mod abi;
mod pb;
use hex_literal::hex;
use pb::erc721;
use substreams::{log, store, Hex};
use substreams_ethereum::{pb::eth::v1 as eth, NULL_ADDRESS};

// Bored Ape Yacht Club Contract
const TRACKED_CONTRACT: [u8; 20] = hex!("bc4ca0eda7647a8ab7c2061c2e118a18a936f13d");

substreams_ethereum::init!();

/// Extracts transfer events from the contract
#[substreams::handlers::map]
fn block_to_transfers(blk: eth::Block) -> Result<erc721::Transfers, substreams::errors::Error> {
    let mut transfers: Vec<erc721::Transfer> = vec![];
    for trx in blk.transaction_traces {
        transfers.extend(trx.receipt.unwrap().logs.iter().filter_map(|log| {
            if log.address != TRACKED_CONTRACT {
                return None;
            }

            log::debug!("NFT Contract {} invoked", Hex(&TRACKED_CONTRACT));

            if !abi::erc721::events::Transfer::match_log(log) {
                return None;
            }

            let transfer = abi::erc721::events::Transfer::must_decode(log);

            Some(erc721::Transfer {
                trx_hash: trx.hash.clone(),
                from: transfer.from,
                to: transfer.to,
                token_id: transfer.token_id.low_u64(),
                ordinal: log.block_index as u64,
            })
        }));
    }

    Ok(erc721::Transfers { transfers })
}

// Store the total balance of NFT tokens by address for the specific TRACKED_CONTRACT by holder
#[substreams::handlers::store]
fn nft_state(transfers: erc721::Transfers, s: store::StoreAddInt64) {
    log::info!("NFT state builder");
    for transfer in transfers.transfers {
        if transfer.from != NULL_ADDRESS {
            log::info!("Found a transfer out");

            s.add(transfer.ordinal, generate_key(&transfer.from), -1);
        }

        if transfer.to != NULL_ADDRESS {
            log::info!("Found a transfer in");

            s.add(transfer.ordinal, generate_key(&transfer.to), 1);
        }
    }
}

fn generate_key(holder: &Vec<u8>) -> String {
    return format!("total:{}:{}", Hex(holder), Hex(TRACKED_CONTRACT));
}


```
{% endcode %}

**Let's break it down**

Firstly, we setup our imports

```rust
mod abi;
mod pb;
use hex_literal::hex;
use pb::erc721;
use substreams::{log, store, Hex};
use substreams_ethereum::{pb::eth::v1 as eth, NULL_ADDRESS};
...
```

We then store the contract that we're tracking as a `constant`, and initiate our Ethereum Substreams

```rust
...

// Bored Ape Yacht Club Contract
const TRACKED_CONTRACT: [u8; 20] = hex!("bc4ca0eda7647a8ab7c2061c2e118a18a936f13d");

substreams_ethereum::init!();

...
```

We then define our first `map` module. As a reminder, here is the module definition in the Manifiest that we created:&#x20;

```yaml
  - name: block_to_transfers
    kind: map
    initialBlock: 12287507
    inputs:
      - source: sf.ethereum.type.v1.Block
    output:
      type: proto:eth.erc721.v1.Transfers
```

Notice the: `name: block_to_transfers`. This name should correspond to our handler function name.&#x20;

Second, we have defined one input and one output. The input has a type of `sf.ethereum.type.v1.Block` which is a standard Ethereum block provided by the `substreams-ethereum` crate. The output has a type of `proto:eth.erc721.v1.Transfers` which is our custom `Protobuf` definition and is provided by the generated Rust code we did in the prior steps. This yields the following function signature:

```rust
...

/// Extracts transfers events from the contract
#[substreams::handlers::map]
fn block_to_transfers(blk: eth::Block) -> Result<erc721::Transfers, substreams::errors::Error> {
    ...
}

...
```

{% hint style="info" %}
**Rust Macros**

Notice the `#[substreams::handlers::map]` above the function, this is a [rust macro](https://doc.rust-lang.org/book/ch19-06-macros.html) that is provided by the `substreams` crate. This macro decorates our handler function as a `map`. There is also a macro used to decorate handler of kind `store`:&#x20;

`#[substreams::handlers::store]`&#x20;
{% endhint %}

The goal of the `map` we are building is to extract `ERC721` Transfers from a given block. We can achieve this by finding all the `Transfer` events that are emitted by the contract we are tracking. Once we find such an event we will decode it and create a `Transfer` object

```rust
...

/// Extracts transfer events from the contract
#[substreams::handlers::map]
fn block_to_transfers(blk: eth::Block) -> Result<erc721::Transfers, substreams::errors::Error> {
    // variable to store the transfers we find
    let mut transfers: Vec<erc721::Transfer> = vec![];
    // loop through the block's transaction
    for trx in blk.transaction_traces {
        // iterate over the transaction logs
        transfers.extend(trx.receipt.unwrap().logs.iter().filter_map(|log| {
            // verifying that the logs emitted are from the contract we are tracking
            if log.address != TRACKED_CONTRACT {
                return None;
            }

            log::debug!("NFT Contract {} invoked", Hex(&TRACKED_CONTRACT));
            // verify if the log matches a Transfer Event
            if !abi::erc721::events::Transfer::match_log(log) {
                return None;
            }
            
            // decode the event and store it
            let transfer = abi::erc721::events::Transfer::must_decode(log);
            Some(erc721::Transfer {
                trx_hash: trx.hash.clone(),
                from: transfer.from,
                to: transfer.to,
                token_id: transfer.token_id.low_u64(),
                ordinal: log.block_index as u64,
            })
        }));
    }
    
    // return our list of transfers for the given block
    Ok(erc721::Transfers { transfers })
}

```

Let's now define our `store` module. As a reminder, here is the module definition in the Manifiest&#x20;

```yaml
  - name: nft_state
    kind: store
    initialBlock: 12287507
    updatePolicy: add
    valueType: int64
    inputs:
      - map: block_to_transfers

```

First, notice the: `name: nft_state`. This name should also correspond to our handler function name.&#x20;

Second, we have defined one input. The input corresponds to the output of the `map` module `block_to_transfers`, which is of type `proto:eth.erc721.v1.Transfers`. This is our custom `Protobuf` definition and is provided by the generated Rust code we did in the prior steps. This yields the following function signature:

```rust
...

/// Store the total balance of NFT tokens for the specific TRACKED_CONTRACT by holder
#[substreams::handlers::store]
fn nft_state(transfers: erc721::Transfers, s: store::StoreAddInt64) {
    ...
}

```

Note that the `store` will always take as its **last input** the writable store itself. In this example the `store` module has an `updatePolicy: add` and a `valueType: int64` this yields a writable store of type `StoreAddInt64`

{% hint style="info" %}
**Store Types**

The last parameter of a `store` module function should always be the writable store itself. The type of said writable store is based on your `store` module `updatePolicy` and `valueType`. You can see all the possible types of store [here](../../rust/substreams/src/store.rs).
{% endhint %}

The goal of the `store` we are building is to keep track of a holder's current NFT count for the given contract. We will achieve this by analyzing the transfers.&#x20;

* if the transfer's `from` address field is the null address (`0x0000000000000000000000000000000000000000`) and the `to` address field is not the null address, we know the `to` address field is minting a token, and we should increment his count.&#x20;
* if the transfer's `from` address field is not the null address and the `to` address field is the null address, we know the `from` address field is burning a token, and we should decrement his count.
* If the `from` address field and the `to` address field is not the null address, we should decrement the count of the `from` address and increment the count of the `to` address field as this is a basic transfer.

When writing to a store, there are generally three concepts you must consider:

1. `ordinal`: this represents the order in which your `store` operations will be applied. Consider the following: your `store` handler will be called once per `block`- during that execution it may call the `add` operation multiple times, for multiple reasons (found a relevant event, saw a call that triggered a method call). Since a blockchain execution model is linear and deterministic, we need to make sure we can apply your `add` operations linearly and deterministically. By having to specify an ordinal, we can guarantee the order of execution. In other words, given one execution of your `store` handler for given inputs (in this example a list of transfers), your code should emit the same number of `add` calls with the same ordinal values.&#x20;
2. `key`: Since our stores are [key/value stores](https://en.wikipedia.org/wiki/Key%E2%80%93value\_database), we need to take care in crafting the key, to ensure that it is unique and flexible. In our example, if the `generate_key` function would simply return a key that is the `TRACKED_CONTRACT` address it would not be unique between different token holders. If the `generate_key` function would return a key that is only the holder's address, though it would be unique amongst holders, we would run into issues if we wanted to track multiple contracts.
3. `value`: The value we are storing, the type is dependant on the store type we are using.

```rust
/// Store the total balance of NFT tokens for the specific TRACKED_CONTRACT by holder
#[substreams::handlers::store]
fn nft_state(transfers: erc721::Transfers, s: store::StoreAddInt64) {
    log::info!("NFT state builder");
    // iterate over the transfers event
    for transfer in transfers.transfers {
        // check if the from address field is not the NULL address
        if transfer.from != NULL_ADDRESS {
            log::info!("Found a transfer out");
            // decrement the count
            s.add(transfer.ordinal, generate_key(&transfer.from), -1);
        }
        // check if the to address field is not the NULL address
        if transfer.to != NULL_ADDRESS {
            log::info!("Found a transfer in");
            // increment the count
            s.add(transfer.ordinal, generate_key(&transfer.to), 1);
        }
    }
}

fn generate_key(holder: &Vec<u8>) -> String {
    return format!("total:{}:{}", Hex(holder), Hex(TRACKED_CONTRACT));
}

```

### Summary

We have created both of our handler functions, one for extracting transfers that are of interest to us, and a second to store the token count per recipient. At this point you should be able to build your Substreams.

```
cargo build --target wasm32-unknown-unknown --release
```
