

Substreams allows you to pass parameters to your modules by specifying them in the manifest.

## Parameterization of a Factory contract

It's quite common for a smart contract to be deployed on different networks or even by different dApps within the same network. Uniswap Factory smart contract is a good example of that.

When running Substreams for a dApp, you need to know the smart contract deployment address and for obvious reasons, this address will be different for each deployment.

Instead of hard-coding the address in the Substreams binary, you can customize it without having to rebuild or even repackage the Substreams package. The consumer can then just provide the address as a parameter.

First, you need to add the `params` field as an input. Note that it's always a string and it's always the first input for the module:

```yaml
modules:
  - name: map_pools_created
    kind: map
    inputs:
      - params: string
      - source: sf.ethereum.type.v2.Block
    output:
      type: proto:uniswap.types.v1.Pools
params:
  map_params: 1f98431c8ad98523631ae4a59f267346ea31f984
```

You can specify the default value directly in the manifest. In this case, we use `0x1f98431c8ad98523631ae4a59f267346ea31f984` - the deployment address for UniswapV3 contract on Ethereum Mainnet.

Handling the parameter in the module is easy. The module handler receives it as a first input parameter and you can use it to filter transactions instead of the hard-coded value:

```rust
#[substreams::handlers::map]
pub fn map_pools_created(params: String, block: Block) -> Result<Pools, Error> {
    let factory_address = Hex::decode(params).unwrap();
    Ok(Pools {
        pools: block
            .events::<abi::factory::events::PoolCreated>(&[&factory_address])
            .filter_map(|(event, log)| {
                // skipped: extracting pool information from the transaction
                Some(Pool {
                    address,
                    token0,
                    token1,
                    ..Default::default()
                })
            })
            .collect(),
    })
}
```

To pass the parameter to the module using `substreams` CLI you can use `-p` key:

```bash
substreams gui -e $SUBSTREAMS_ENDPOINT map_pools_created -t +1000 -p map_pools_created="1f98431c8ad98523631ae4a59f267346ea31f984"`
```

### Documenting parameters
It's always a good idea to document what the params represent and how they are structured, so the consumers of your modules know how to properly parameterize them. You can use `doc` field for the module definition in the manifest.

```yaml
modules:
  - name: map_pools_created
    kind: map
    inputs:
      - source: sf.ethereum.type.v2.Block
      - params: string
    output:
      type: proto:uniswap.types.v1.Pools
    doc: |
      Params contains Uniswap factory smart contract address without `0x` prefix, i.e. 1f98431c8ad98523631ae4a59f267346ea31f984 for Ethereum Mainnet
```

## Advanced parameters

Sometimes you may need to use multiple parameters for a module. To pass multiple parameters, you can encode them as a URL-encoded query string, i.e. `param1=value1&param2=value2`.

Suppose you want to track transfers to/from a certain address exceeding a certain amount of ETH. Your module manifest could look like this:

```yaml
modules:
  - name: map_whale_transfers
    kind: map
    inputs:
      - params: string
      - source: sf.ethereum.type.v2.Block
    output:
      type: proto:Transfers
params:
  map_params: address=aaa..aaa&amount=100
```

Our module gets a params string with two parameters: `address` and `amount`.

In your module handler, you can decode your parameters using one of the URL decoding crates such as `serde_qs`, `serde_urlencoded` or your own helper functions. Here's an example using `serde_qs`:

```rust
#[derive(Debug, Deserialize)]
struct Params {
    address: String,
    amount: u64,
}

#[substreams::handlers::map]
pub fn map_whale_transfers(params: String, block: Block) -> Result<Transfers, Error> {
    let query: Params = serde_qs::from_str(params.as_str()).unwrap();
    log::info!("Tracking transfers for address: {} of more than {} ETH", query.address, query.amount);

    // filter transfers by address and amount
}
```

Sometimes parameters can be optional, i.e. you want to track all transfers rather than a specific address. Decoding will look like this in that case:

```rust
#[derive(Debug, Deserialize)]
struct QueryParams {
    address: Option<String>,
    amount: u64,
}

#[substreams::handlers::map]
pub fn map_whale_transfers(params: String, block: Block) -> Result<Transfers, Error> {
    let query: QueryParams = serde_qs::from_str(params.as_str()).unwrap();

    if query.address.is_none() {
      log::info!("Tracking all of more than {} ETH", query.amount);
    }
    else {
      log::info!("Tracking transfers for address: {} of more than {} ETH", query.address, query.amount);
    }
}
```

You can even pass a vector of addresses to track multiple specific whales in our example:

```rust
#[derive(Debug, Deserialize)]
struct QueryParams {
    address: Vec<String>,
    amount: u64,
}

#[substreams::handlers::map]
pub fn map_whale_transfers(params: String, block: Block) -> Result<Transfers, Error> {
    let query: QueryParams = serde_qs::from_str(params.as_str()).unwrap();
    log::info!("Tracking transfers for addresses: {:?} of more than {} ETH", query.address, query.amount);
}
```

Depending on the crate you use to decode params string, you can pass them to Substreams CLI like this for example:

```bash
substreams gui map_whale_transfers -p map_whale_transfers="address[]=aaa..aaa&address[]=bbb..bbb&amount=100"
```
