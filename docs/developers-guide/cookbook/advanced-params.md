---
description: Advanced params usage
---

# Advanced parameters

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
