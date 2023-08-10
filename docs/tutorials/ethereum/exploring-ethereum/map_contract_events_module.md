## Retrieving Events of a Smart Contract

Given a smart contract address passed as a parameter, this module returns the logs attached to the contract.

### Running the Substreams

First, generate the Protobuf modules and build the Rust code:

```bash
make protogen
```

```bash
make build
```

Now, you can run the Substreams. The logs retrieved correspond to the `0xbc4ca0eda7647a8ab7c2061c2e118a18a936f13d` (BoredApeYachtClub smart contract).
To avoid iterating over the full blockchain, the following command starts at block `17717995` and finished at block `17718004`. Therefore, only the BoredApeYachtClub smart contract logs that happened within this block range are printed.

```bash
substreams run -e mainnet.eth.streamingfast.io:443 substreams.yaml map_contract_events --start-block 17717995 --stop-block +10
```

The output of the command should be similar to:

```bash
...output omitted...

----------- BLOCK #17,717,995 (bfecb26963a2cd77700754612185e0074fc9589d2d73abb90e362fe9e7969451) ---------------
----------- BLOCK #17,717,996 (7bf431a4f9df67e1d7e385d9a6cba41c658e66a77f0eb926163a7bbf6619ce20) ---------------
----------- BLOCK #17,717,997 (fa5a57231348f1f138cb71207f0cdcc4a0a267e2688aa63ebff14265b8dae275) ---------------
{
  "@module": "map_contract_events",
  "@block": 17717997,
  "@type": "eth.event.v1.Events",
  "@data": {
    "events": [
      {
        "address": "bc4ca0eda7647a8ab7c2061c2e118a18a936f13d",
        "topics": [
          "8c5be1e5ebec7d5bd14f71427d1e84f3dd0314c0f7b2291e5b200ac8c7c3b925",
          "000000000000000000000000e2a83b15fc300d8457eb9e176f98d92a8ff40a49",
          "0000000000000000000000000000000000000000000000000000000000000000",
          "00000000000000000000000000000000000000000000000000000000000026a7"
        ],
        "txHash": "f18291982e955f3c2112de58c1d0a08b79449fb473e58b173de7e0e189d34939"
      },
      {
        "address": "bc4ca0eda7647a8ab7c2061c2e118a18a936f13d",
        "topics": [
          "ddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef",
          "000000000000000000000000e2a83b15fc300d8457eb9e176f98d92a8ff40a49",
          "000000000000000000000000c67db0df922238979da0fd00d46016e8ae14cecb",
          "00000000000000000000000000000000000000000000000000000000000026a7"
        ],
        "txHash": "f18291982e955f3c2112de58c1d0a08b79449fb473e58b173de7e0e189d34939"
      }
    ]
  }
}

----------- BLOCK #17,717,998 (372ff635821a434c81759b3b23e8dac59393fc27a7ebb88b561c1e5da3c4643a) ---------------
----------- BLOCK #17,717,999 (43f0878e119836cc789ecaf12c3280b82dc49567600cc44f6a042149e2a03779) ---------------
----------- BLOCK #17,718,000 (439efaf9cc0059890a09d34b4cb5a3fe4b61e8ef96ee67673c060d58ff951d4f) ---------------
----------- BLOCK #17,718,001 (c97ca5fd26db28128b0ec2483645348bbfe998e9a6e19e3a442221198254c9ea) ---------------
----------- BLOCK #17,718,002 (9398569e46a954378b16e0e7ce95e49d0f21e6119ed0e3ab84f1c91f16c0c30e) ---------------
----------- BLOCK #17,718,003 (80bcd4c1131c35a413c32903ffa52a14f8c8fe712492a8f6a0feddbb03b10bba) ---------------
----------- BLOCK #17,718,004 (d27309ac29fe47f09fa4987a318818c325403863a53eec6a3676c2c2f8c069d9) ---------------
all done
```

The smart contract address is passed as a parameter defined in the Substreams manifest (`substreams.yml`):

```yml
params:
  map_contract_events: "0xbc4ca0eda7647a8ab7c2061c2e118a18a936f13d"
```

### Inspecting the Code

Declaration of the module in the manifest (`substreams.yml`):

```yml
- name: map_contract_events
    kind: map
    inputs:
      - params: string
      - source: sf.ethereum.type.v2.Block
    output:
      type: proto:eth.event.v1.Events
```

The module expects two inputs: the parameter as a string, and a raw Ethereum block.
The output is the `Events` object defined in the Protobuf.

The corresponding Rust function declaration, which matches the name of the module, `map_contract_events`:

```rust
fn map_contract_events(contract_address: String, blk: Block) -> Result<Events, Error> {
    verify_parameter(&contract_address)?; // Verify address

    let events: Vec<Event> = blk
        .logs() // 1.
        .filter(|log| log.address().to_vec() == Hex::decode(&contract_address).expect("already validated")) // 2.
        .map(|log| Event { // 3.
            address: Hex::encode(log.address()),
            topics: log.topics().into_iter().map(Hex::encode).collect(),
            tx_hash: Hex::encode(&log.receipt.transaction.hash),
        })
        .collect(); // 4.

    Ok(Events { events })
}
```

In this example, you do not need to parse the parameters, as `contract_address` is the only string passed and you can use it directly.
However, it is necessary to verify that the parameter is a valid Ethereum; this verification is performed by the `verify_parameter` function.

Then, you iterate over the events of the contract:
1. The `.logs()` function iterates over the logs of successful transactions within the block.
2. For every log of a successful transaction, you verify if its `address` matches the smart contract address (i.e. you verify if the log was actually emitted by the smart contract). For the comparison, both `log.address()` and `contract_address` are converted to `Vec<u8>`.
3. Every filtered log (i.e. every log that belongs to the smart contract) is mapped to a `pb::eth::event::v1::Event` struct, which was specified in the Protobuf definition.
4. Finally, you collect all the events in a vector.