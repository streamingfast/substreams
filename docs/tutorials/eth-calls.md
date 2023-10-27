---
description: Learn how to perform Contract Calls (eth_calls) in EVM-compatible Substreams
---

# Contract Calls

EVM-compatible smart contracts are queryable, which means that you can get real-time data from the contract's internal database.
In this tutorial, you will learn how to perform contract calls (`eth_calls`) through Substreams.

Specifically, you will query the USDT smart contract (`0xdac17f958d2ee523a2206206994597c13d831ec7`) to get the number of decimals used by the token.
The USDT smart contract exposes a read function called `decimals`.

## Pre-requisites

- You have some knowledge about Substreams ([modules](https://substreams.streamingfast.io/concepts-and-fundamentals/modules) and [fundamentals](https://substreams.streamingfast.io/concepts-and-fundamentals/fundamentals)).
- You have the latest version of the [CLI](https://substreams.streamingfast.io/getting-started/installing-the-cli) installed.

## Querying on EthScan

You can query the `decimals` function by [visiting EthScan](https://etherscan.io/address/0xdac17f958d2ee523a2206206994597c13d831ec7#readContract).


<figure><img src="../.gitbook/assets/eth-scan-calls.png" width="100%" /><figcaption><p>USDT contract on EthScan</p></figcaption></figure>

## Initializing the Substreams project

1. First, let's use `substreams init` to scaffold a new Substreams project that uses the USDT smart contract:

```bash
substreams init
```

Complete the information required by the previous command, such as name of the project or smart contract to track.
In the `Contract address to track` step, write `0xdac17f958d2ee523a2206206994597c13d831ec7`, the address of the USDT smart contract.

```bash
Project name (lowercase, numbers, undescores): usdttracker
Protocol: Ethereum
Ethereum chain: Mainnet
Contract address to track (leave empty to use "Bored Ape Yacht Club"): 0xdac17f958d2ee523a2206206994597c13d831ec7
Would you like to track another contract? (Leave empty if not): 
Retrieving Ethereum Mainnet contract information (ABI & creation block)
Fetched contract ABI for dac17f958d2ee523a2206206994597c13d831ec7
Fetched initial block 4634748 for dac17f958d2ee523a2206206994597c13d831ec7 (lowest 4634748)
Generating ABI Event models for 
  Generating ABI Events for AddedBlackList (_user)
  Generating ABI Events for Approval (owner,spender,value)
  Generating ABI Events for Deprecate (newAddress)
  Generating ABI Events for DestroyedBlackFunds (_blackListedUser,_balance)
  Generating ABI Events for Issue (amount)
  Generating ABI Events for Params (feeBasisPoints,maxFee)
  Generating ABI Events for Redeem (amount)
  Generating ABI Events for RemovedBlackList (_user)
  Generating ABI Events for Transfer (from,to,value)
Writing project files
Generating Protobuf Rust code
```

2. Move to the project folder and build the Substreams.

```bash
make build
```

3. Then, verify that the Substreams runs correctly. By default, it will output all the events of the smart contract.

```bash
substreams run -e mainnet.eth.streamingfast.io:443 \            
   substreams.yaml \
   map_events \ 
   --start-block 12292922 \
   --stop-block +1
```

The previous command will output the following:

```bash
Progress messages received: 0 (0/sec)
Backprocessing history up to requested target block 12292922:
(hit 'm' to switch mode)

----------- BLOCK #12,292,922 (e2d521d11856591b77506a383033cf85e1d46f1669321859154ab38643244293) ---------------
{
  "@module": "map_events",
  "@block": 12292922,
  "@type": "contract.v1.Events",
  "@data": {
    "transfers": [
      {
        "evtTxHash": "90e4fd16c989cdc7ecdfd0b6f458eb4be1c538901106bb794bb608f38ac9dd9f",
        "evtIndex": 1,
        "evtBlockTime": "2021-04-22T23:13:40Z",
        "evtBlockNumber": "12292922",
        "from": "odjZclYML4FEr4cdtQjwsLEKP78=",
        "to": "XmM2sGcWQDHSwcLHo85fcWEdAcw=",
        "value": "372200000"
      }
    ]
  }
}

all done
```

## Adding Calls to the Substreams

The `substreams init` command generates Rust structures based on the ABI of the smart contract provided. All the calls are available under the `abi::contract::functions` namespace of the generated code. Let's take a look.

1. Open the project in an editor of your choice (for example, VS Code) and navigate to the `lib.rs` file, which contains the main Substreams code.

2. Create a new function, `get_decimals`, which returns a `BigInt` struct:

```rust
fn get_decimals() -> substreams::scalar::BigInt {

}
```

3. Import the `abi::contract::functions::Decimals` struct from the generated ABI code.

```rust
fn get_decimals() -> substreams::scalar::BigInt {
    let decimals = abi::contract::functions::Decimals {};

}
```

4. Next, use the `call` method to make the actual _eth_call_ by providing the smart contract address:

```rust
fn get_decimals() -> substreams::scalar::BigInt {
    let decimals = abi::contract::functions::Decimals {};
    let decimals_option = decimals.call(TRACKED_CONTRACT.to_vec());

    decimals_option.unwrap()
}
```

In this case, the `call` method returns a `substreams::scalar::BigInt` struct containing the number of decimals used in the USDT token (`6`).

5. You can include this function in the `map_events` module just for testing purposes:

```rust
#[substreams::handlers::map]
fn map_events(blk: eth::Block) -> Result<contract::Events, substreams::errors::Error> {
    let evt_block_time =
        (blk.timestamp().seconds as u64 * 1000) + (blk.timestamp().nanos as u64 / 1000000);

    // Using the decimals function
    let decimals = get_decimals();
    substreams::log::info!("Number of decimals for the USDT token: {}", decimals.to_string());

...output omitted...
}
```

{% hint style="warning" %}
**Important:** Remember that this tutorial shows how to call the `decimals` function, but all the available calls are under the `abi::contract::functions` namespace, so you should be able to find them just by exploring the auto-generated ABI Rust files.
{% endhint %}

6. To see it in action, just re-build and re-run the Substreams:

```bash
make build
```

```bash
substreams run -e mainnet.eth.streamingfast.io:443 \            
   substreams.yaml \
   map_events \ 
   --start-block 12292922 \
   --stop-block +1
```

The output should be similar to the following:

```bash
Connected (trace ID 6fb1a55ed17001d850d8c6655226ef6f)
Progress messages received: 0 (0/sec)
Backprocessing history up to requested target block 12292922:
(hit 'm' to switch mode)

----------- BLOCK #12,292,922 (e2d521d11856591b77506a383033cf85e1d46f1669321859154ab38643244293) ---------------
map_events: log: Number of decimals for the USDT token: 6
{
  "@module": "map_events",
  "@block": 12292922,
  "@type": "contract.v1.Events",
  "@data": {
    "transfers": [
      {
        "evtTxHash": "90e4fd16c989cdc7ecdfd0b6f458eb4be1c538901106bb794bb608f38ac9dd9f",
        "evtIndex": 1,
        "evtBlockTime": "1619133220000",
        "evtBlockNumber": "12292922",
        "from": "odjZclYML4FEr4cdtQjwsLEKP78=",
        "to": "XmM2sGcWQDHSwcLHo85fcWEdAcw=",
        "value": "372200000"
      }
    ]
  }
}

all done
```


