The [USDT Exchanges Volume Subgraph]() tracks the historical USDT volume for the `INJ-USDT` pair in the Dojo DEX.
It is a subgraph that uses the [Substreams triggers](../../../consume/subgraph/)

## Before You Begin

- [Install the Substreams CLI](../../../common/installing-the-cli.md)
- [Get an authentication token](../../../common/authentication.md)
- [Learn about the basics of the Substreams](../../../common/manifest-modules.md)
- [Complete the Block Stats Substreams tutorial](./block-stats.md)
- [Complete the Transaction Substreams tutorial](./transactions.md)

## Inspect the Project

Only one module, `map_usdt_exchanges`, is defined:

```yaml
modules:
  - name: map_usdt_exchanges
    kind: map
    initialBlock: 59693701
    inputs:
      - source: sf.cosmos.type.v2.Block
    output:
      type: proto:sf.substreams.cosmos.v1.USDTExchangeList
```

The module outputs a list of `USDTExchange` objects. For every swap in the `INJ-USDT` pair in the Dojo DEX, an `USDTExchange` object is emitted with the amount of USDT swapped. Adding up all this amounts allows you to find out the USDT historical volume traded.

```protobuf
syntax = "proto3";

package sf.substreams.cosmos.v1;

message USDTExchange {
    string amount = 1;
}

message USDTExchangeList {
    repeated USDTExchange exchanges = 1;
}
```

## Inspect the Code

The goal of the Substreams is to track all the swaps of the `INJ-USDT` pair and find out the amount of USDT exchanged. The extraction layer of the Substreams consists of:
- Track all the transactions with messages `cosmwasm.wasm.v1.MsgExecuteContract` or `injective.wasmx.v1.MsgExecuteContractCompat`.
- Verify that the contract where the message is executed belongs to the Dojo `INJ-USDT` pair. In this case, it is the `inj1h0mpv48ctcsmydymh2hnkal7hla5gl4gftemqv` contract.
- Iterate over the events looking for an event called `wasm`, which contains all the information about the swap. From this event, you can easily extract the amount of USDT exchanged. 

```rust
#[substreams::handlers::map]
pub fn map_usdt_exchanges(block: Block) -> Result<UsdtExchangeList, Error> {
    // Mutable list to add the output of the Substreams
    let mut usdt_exchanges: Vec<UsdtExchange> = Vec::new();

    if block.txs.len() != block.tx_results.len() {
        return Err(anyhow!("Transaction list and result list do not match"));
    }

    for i in 0..block.txs.len() {
        let tx = block.txs.get(i).unwrap();
        let tx_result = block.tx_results.get(i).unwrap();

        if let Ok(transaction) = Tx::from_bytes(tx) {
            for message in transaction.body.messages {
                if let Some(usdt_exchange) = handle_msg_execute_contract(&message, &tx_result) {
                    usdt_exchanges.push(usdt_exchange)
                }
            }
        }
    }

    Ok(UsdtExchangeList {
        exchanges: usdt_exchanges,
    })
}
```

The `wasm` event contains several fields and the USDT amount could be in the `ask_amount` or the `offer_amount` field, depending on the direction of the swap (`USDT to INJ` or `INJ to USDT`).

```rust
fn extract_data_from_event(event: &Event) -> Option<UsdtExchange> {
    let mut offer_asset = &String::new(); // 1.
    let mut offer_amount = &String::new();
    let mut ask_asset = &String::new();
    let mut ask_amount = &String::new();

    event.attributes.iter().for_each(|att| { // 2.
        if att.key == "offer_asset" {
            offer_asset = &att.value;
        }

        if att.key == "offer_amount" {
            offer_amount = &att.value;
        }

        if att.key == "ask_asset" {
            ask_asset = &att.value;
        }

        if att.key == "ask_amount" || att.key == "return_amount" {
            ask_amount = &att.value;
        }
    });

    if ask_asset == USDT_ADDRESS && !ask_amount.is_empty() { // 3.
        return Some(UsdtExchange {
            amount: ask_amount.to_string(),
        });
    }

    if offer_asset == USDT_ADDRESS && !offer_amount.is_empty() { // 4.
        return Some(UsdtExchange {
            amount: offer_amount.to_string(),
        });
    }

    return None;
}
```