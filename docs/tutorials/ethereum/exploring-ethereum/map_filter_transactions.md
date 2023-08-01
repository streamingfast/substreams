## The "map_filter_transactions" Module

This module iterates over all the blockchain transactions, and filters them by some of their fields (the `from` and `to` fields).
For example, if that you want to retrieve all the trasactions initiated by the address `0x`, you set the filter `from = 0x`.

### Running the Substreams

First, generate the Protobuf modules and build the Rust code:

```bash
$ make protogen
```

```bash
$ make build
```

Now, you can run the Substreams:

```bash
substreams run -e mainnet.eth.streamingfast.io:443 substreams.yaml map_filter_transactions --start-block 17712038 --stop-block +3

Connected (trace ID 412780151f980b265382c5df35789a0c)
Progress messages received: 0 (0/sec)
Backprocessing history up to requested target block 17712038:
(hit 'm' to switch mode)

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
      {
        "from": "0543ba40d4b8b33dc5f7d163f41c6dc54cf1d923",
        "to": "dac17f958d2ee523a2206206994597c13d831ec7",
        "hash": "51071d7a94fc6ecfec2aba477c26ff5098db3e36a287d43d13b763b3118b160b"
      },
      {
        "from": "09e52cbb57dce8cd2836effc44686b6008a84914",
        "to": "dac17f958d2ee523a2206206994597c13d831ec7",
        "hash": "806ef36a1e022d52d00a288bc150676af0cb2bad6b5500378c8fc7253a0434fa"
      },
      {
        "from": "2f13d388b85e0ecd32e7c3d7f36d1053354ef104",
        "to": "dac17f958d2ee523a2206206994597c13d831ec7",
        "hash": "0a9a5707b5d4047b1e44de9283f0c88606eac49b4eb132a61df0dffc20668ad0"
      },
      {
        "from": "4fac9d83ffad797072db8bd72cc544ad5ec45e4f",
        "to": "dac17f958d2ee523a2206206994597c13d831ec7",
        "hash": "8466f371eed9b742a2ed869213dde10661e3df22366e258e09f68e37ca47b2c1"
      },
      {
        "from": "48c04ed5691981c42154c6167398f95e8f38a7ff",
        "to": "dac17f958d2ee523a2206206994597c13d831ec7",
        "hash": "571670afd47e29fe901c1b17ed21fca6088cc9540efd684c5b7b4c1c1e748612"
      },
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
      {
        "from": "75e89d5979e4f6fba9f97c104c2f0afb3f1dcb88",
        "to": "dac17f958d2ee523a2206206994597c13d831ec7",
        "hash": "619d94c33b027df694cbf32659aae51743623b4d1cb11c69d7d0e95cad63b712"
      },
      {
        "from": "75e89d5979e4f6fba9f97c104c2f0afb3f1dcb88",
        "to": "dac17f958d2ee523a2206206994597c13d831ec7",
        "hash": "027cccdba1a127bcfb5bb39b5d89e3552e83c8c3c6dd13cf779d7720241e71b9"
      },
      {
        "from": "3d1d8a1d418220fd53c18744d44c182c46f47468",
        "to": "dac17f958d2ee523a2206206994597c13d831ec7",
        "hash": "762350dcf3ab62ad515331436ce952ba5b3641bbf87c7d56c1e8a9f21473875c"
      },
      {
        "from": "a45c27ef3df487525b33a70cb0020de792dc7a3f",
        "to": "dac17f958d2ee523a2206206994597c13d831ec7",
        "hash": "b9e08dfe7b1f4971ea96d1424c32548028bdeb62b2ee7f6775dd55d05c4d4ad6"
      },
      {
        "from": "9696f59e4d72e237be84ffd425dcad154bf96976",
        "to": "dac17f958d2ee523a2206206994597c13d831ec7",
        "hash": "44f36363290969d8b581bb9a856bc9f2ca9a64e4a12e4db054927a45795480fa"
      },
      {
        "from": "e074f1967080cd7b9352c8cbe2d1d9cd121d4daf",
        "to": "dac17f958d2ee523a2206206994597c13d831ec7",
        "hash": "8795aa5088fb13a21048c592316ad7da850a8f80f3ce417bc4d7d2bbeca3f596"
      },
      {
        "from": "fb8131c260749c7835a08ccbdb64728de432858e",
        "to": "dac17f958d2ee523a2206206994597c13d831ec7",
        "hash": "0a907108aecaf909452f7035070a28f9cad6c51896763e760ea1f544a9b9edf3"
      },
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
      {
        "from": "7c0a7899f69a7034325ffee90355906cf72aeebb",
        "to": "dac17f958d2ee523a2206206994597c13d831ec7",
        "hash": "132fc93b8a155c614001665a40381c8de9ad7519034352628c075e17a06d884b"
      },
      {
        "from": "180277c2f8bd489a4e27e261c6fbca079b6fa58f",
        "to": "dac17f958d2ee523a2206206994597c13d831ec7",
        "hash": "06e74e08b51a0c03219c3aa12a871595516c1d466611ed848ea2ae8cbfb083ea"
      },
      {
        "from": "1440ec793ae50fa046b95bfeca5af475b6003f9e",
        "to": "dac17f958d2ee523a2206206994597c13d831ec7",
        "hash": "83862ea45a6f777acd81a3469c54e347d3eb527cbee9fb673c6e312f7ae6fb83"
      },
      {
        "from": "89e51fa8ca5d66cd220baed62ed01e8951aa7c40",
        "to": "dac17f958d2ee523a2206206994597c13d831ec7",
        "hash": "1b9e5059181ca90969ee423beea3073cf99faf8a91b73890303531ebd6c197ec"
      },
      {
        "from": "89e51fa8ca5d66cd220baed62ed01e8951aa7c40",
        "to": "dac17f958d2ee523a2206206994597c13d831ec7",
        "hash": "ca1750068bee961ccd2e45679c9d9dadc5ba93fd3212c0f31361d39abe3ed36c"
      },
      {
        "from": "82cbcb64a2eb51622fb847c9c957fdac532712ac",
        "to": "dac17f958d2ee523a2206206994597c13d831ec7",
        "hash": "284b6359cf66a010798738bb764f5cd015658e8f59273a49e19a855731f22bb8"
      },
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

In the previous command, you are filtering all the transactions from blocks `17712038` to `17712041`, where `to = 0xdac17f958d2ee523a2206206994597c13d831ec7` (the USDT smart contract address). The filters are specified in the `params` section of the Substreams manifest (`substreams.yml`):

```yml
map_filter_transactions: "to=0xdAC17F958D2ee523a2206206994597C13D831ec7"
```

### Applying Filters

The filters are specified as a query-encoded string (`param1=value1&param2=value2&param3=value3`). In this example, only two parameters are support, `from` and `to`, which you can use to create filters, such as:

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
For Substreams, the parameter specified in the manifest is a simple String. The query-enconded format is just an abstraction that you must parse.
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

    // At this point, filters are correct. If not, a Vec<substreams::errors::Error> object is returned.
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
This struct is part of the Protobuf declarations, and is part of the output of the Substreams module.
4. Finally, all the transactions are collected into a vector of type  `pb::eth::transaction::v1::Transaction`.

Let's take a look at the `apply_filter` function, which returns `true` if the current transaction in the iterator must be filtered or `false` otherwise.

```rust
fn apply_filter(transaction: &TransactionTrace, filters: &TransactionFilterParams) -> bool {
    if !filter_by_parameter(&filters.from, &transaction.from)
        || !filter_by_parameter(&filters.to, &transaction.to)
        || transaction.status != (TransactionTraceStatus::Succeeded as i32)
    {
        return false;
    }

    true
}
```

The function receives two parameters: `TransactionTrace`, which contains all the information about a specific transaction, and `TransactionFilterParams`, which contains the filters provided by the user. If 





