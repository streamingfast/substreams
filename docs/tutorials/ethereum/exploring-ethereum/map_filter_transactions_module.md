## Filtering Transactions

This module iterates over all the blockchain transactions and filters them by some of their fields (the `from` and `to` fields).
For example, if you want to retrieve all the transactions initiated by the address `0xb6692f7ae54e89da0269c1bfd685ccdfd41d2bf7`, you set the filter `from = 0xb6692f7ae54e89da0269c1bfd685ccdfd41d2bf7`.

### Running the Substreams

First, generate the Protobuf modules and build the Rust code:

```bash
make protogen
```

```bash
make build
```

Now, you can run the Substreams:

```bash
substreams run -e mainnet.eth.streamingfast.io:443 substreams.yaml map_filter_transactions --start-block 17712038 --stop-block +3
```

The output of the command should be similar to:

```bash
...output omitted...

----------- BLOCK #17,712,038 (b96fc7e71c0daf69b19211c45fbb5c201f4356fb2b5607500b7d88d298599f5b) ---------------
{
  "@module": "map_filter_transactions",
  "@block": 17712038,
  "@type": "eth.transaction.v1.Transactions",
  "@data": {
    "transactions": [
      {
        "from": "b6692f7ae54e89da0269c1bfd685ccdfd41d2bf7",
        "to": "dac17f958d2ee523a2206206994597c13d831ec7",
        "hash": "933b74565234ac9ca8389f7a49fad80099abf1be77e4bef5af69ade30127f30e"
      },

...output omitted...

      {
        "from": "4c8e30406f5dbedfaa18cb6b9d0484cd5390490a",
        "to": "dac17f958d2ee523a2206206994597c13d831ec7",
        "hash": "558031630b43c8c61e36d742a779f967f3f0102fa290111f6f6f9c2acaadf3ea"
      }
    ]
  }
}

----------- BLOCK #17,712,039 (1385f853d28b16ad7ebc5d51b6f2ef6d43df4b57bd4c6fe4ef8ccb6f266d8b91) ---------------
{
  "@module": "map_filter_transactions",
  "@block": 17712039,
  "@type": "eth.transaction.v1.Transactions",
  "@data": {
    "transactions": [
      {
        "from": "75e89d5979e4f6fba9f97c104c2f0afb3f1dcb88",
        "to": "dac17f958d2ee523a2206206994597c13d831ec7",
        "hash": "43e0e1b6315c4cc1608d876f98c9bbf09f2a25404aabaeac045b5cc852df0e85"
      },
      
...output omitted...

      {
        "from": "e41febca31f997718d2ddf6b21b9710c5c7a3425",
        "to": "dac17f958d2ee523a2206206994597c13d831ec7",
        "hash": "45c03fcbefcce9920806dcd7d638cef262ad405f8beae383fbc2695ad4bc9b1b"
      }
    ]
  }
}

----------- BLOCK #17,712,040 (31ad07fed936990d3c75314589b15cbdec91e4cc53a984a43de622b314c38d0b) ---------------
{
  "@module": "map_filter_transactions",
  "@block": 17712040,
  "@type": "eth.transaction.v1.Transactions",
  "@data": {
    "transactions": [
      {
        "from": "48c04ed5691981c42154c6167398f95e8f38a7ff",
        "to": "dac17f958d2ee523a2206206994597c13d831ec7",
        "hash": "137799eea9fa8ae410c913e16ebc5cc8a01352a638f3ce6f3f29a283ad918987"
      },

...output omitted...

      {
        "from": "f89d7b9c864f589bbf53a82105107622b35eaa40",
        "to": "dac17f958d2ee523a2206206994597c13d831ec7",
        "hash": "0544143b459969c9ed36741533fba70d6ea7069f156d2019d5362c06bf8d887f"
      }
    ]
  }
}

all done
```

In the previous command, you are filtering all the transactions from blocks `17712038` to `17712040`, where `to = 0xdac17f958d2ee523a2206206994597c13d831ec7` (the USDT smart contract address). The filters are specified in the `params` section of the Substreams manifest (`substreams.yml`):

```yml
map_filter_transactions: "to=0xdAC17F958D2ee523a2206206994597C13D831ec7"
```

### Applying Filters

The filters are specified as a query-encoded string (`param1=value1&param2=value2&param3=value3`). In this example, only two parameters are supported, `from` and `to`, which you can use to create filters, such as:

```yml
map_filter_transactions: "from=0x89e51fa8ca5d66cd220baed62ed01e8951aa7c40&to=0xdAC17F958D2ee523a2206206994597C13D831ec7"
```

Retrieve all transactions where `from=0x89e51fa8ca5d66cd220baed62ed01e8951aa7c40` and `to=0xdAC17F958D2ee523a2206206994597C13D831ec7`.

```yml
map_filter_transactions: "from=0x89e51fa8ca5d66cd220baed62ed01e8951aa7c40"
```

Retrieve all transactions where `from=0x89e51fa8ca5d66cd220baed62ed01e8951aa7c40`.

```yml
map_filter_transactions: ""
```

Retrieve all transactions. Without applying any filter.

### Inspecting the Code

Declaration of the module in the manifest (`substreams.yml`):

```yml
- name: map_filter_transactions
    kind: map
    inputs:
      - params: string
      - source: sf.ethereum.type.v2.Block
    output:
      type: proto:eth.transaction.v1.Transactions
```

The module expects two inputs: the parameters string, which contains the filters, plus a raw Ethereum block.
The output is the `Transactions` object declared in the Protobuf.

Now, let's take a look at the actual Rust code:

```rust
#[derive(Deserialize)]
struct TransactionFilterParams {
    to: Option<String>,
    from: Option<String>,
}

#[substreams::handlers::map]
fn map_filter_transactions(params: String, blk: Block) -> Result<Transactions, Vec<substreams::errors::Error>> {
    let filters = parse_filters_from_params(params)?;

    let transactions: Vec<Transaction> = blk
        .transactions()
        .filter(|trans| apply_filter(&trans, &filters))
        .map(|trans| Transaction {
            from: Hex::encode(&trans.from),
            to: Hex::encode(&trans.to),
            hash: Hex::encode(&trans.hash),
        })
        .collect();

    Ok(Transactions { transactions })
}
```

The function name, `map_filter_transactions` matches the name given in the Substreams manifest. Two parameters are passed: `params: String, blk: Block`.
For Substreams, the parameter specified in the manifest is a simple String. The query-encoded format is just an abstraction that you must parse.
The `parse_filters_from_params` parses the string and creates a `TransactionFilterParams` struct.

```rust
let filters = parse_filters_from_params(params)?;
```

```rust
fn parse_filters_from_params(params: String) -> Result<TransactionFilterParams, Vec<substreams::errors::Error>> {
    let parsed_result = serde_qs::from_str(&params);
    if parsed_result.is_err() {
        return Err(Vec::from([anyhow!("Unexpected error while parsing parameters")]));
    }

    let filters = parsed_result.unwrap();
    verify_filters(&filters)?;

    Ok(filters)
}
```

The `serde_qs::from_str(&params)` from the [Serde QS Rust library](https://docs.rs/serde_qs/latest/serde_qs/) parses the parameters and returns the filters struct. Then, you call the `verify_filters(&filters)?` function, which ensures that the filters provided are valid Ethereum addresses.
If there are errors while parsing the parameters, they are collected in a `substreams::errors::Error` vector and returned.

Back in the main function, if the parameters parsing is correct, you start filtering the transactions:

```rust
    let filters = parse_filters_from_params(params)?;

    // At this point, the filters are correct. If not, a Vec<substreams::errors::Error> object is returned.
    let transactions: Vec<Transaction> = blk
        .transactions() // 1.
        .filter(|trans| apply_filter(&trans, &filters)) // 2.
        .map(|trans| Transaction { // 3.
            from: Hex::encode(&trans.from),
            to: Hex::encode(&trans.to),
            hash: Hex::encode(&trans.hash),
        })
        .collect(); // 4.
```
1. The `transactions()` method iterates over all the **successful** transactions of the block.
2. Then, for every successful transaction, the previously parsed filters are applied.
3. Every transaction that complies with the filters provided is mapped into a `pb::eth::transaction::v1::Transaction` struct.
This struct is part of the Protobuf declarations and is part of the output of the Substreams module.
4. Finally, all the transactions are collected into a vector of type  `pb::eth::transaction::v1::Transaction`.