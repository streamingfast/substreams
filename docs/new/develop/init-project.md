
Getting started with Substreams is very easy! Depending on the blockchain that you want to use, the best way to get started might change:

{% tabs %}
{% tab title="Solana" %}
If you want to extract data from Solana, you can take a look at the [Tutorials](../tutorials/solana/solana.md) section, which covers the development of several useful Substreams (SPL tokens, NFT trades, DEX trades...). The [Substreams for Solana Developers](../common/intro-solana.md) section is really useful if this is your first time using Substreams.

The [Substreams Explorer](../tutorials/solana/explore-solana/explore-solana.md) teaches you to most basic extractions you can perform on Solana. More tooling will be developed for Solana soon.
{% endtab %}

{% tab title="EVM" %}
If you have a specific smart contract that you want to extract data from, the `substreams init` initializes a Substreams project that extract the events from the smart contract.

## Tracking a Smart Contract

{% embed url="https://www.youtube.com/watch?v=vWYuOczDiAA" %}
Initialize a Substreams project
{% endembed %}

## Tracking a Factory Smart Contract (Dynamic Datasource)

The `substreams init` command also offers you the possibility to easily track a factory smart contract (in Substreams terminology, a _dynamic datasource_). The following video covers how easy it is to get started with factory contracts on Substreams.

{% embed url="https://www.youtube.com/watch?v=Vn11ovfSpNU" %}
Initialize a Substreams project
{% endembed %}

The following is the `substreams init` execution to create a Substreams that tracks new pools created from the UniswapV3 factory contract.

```bash
✔ Project name (lowercase, numbers, undescores): uniswapv3_factory
Protocol: Ethereum
Ethereum chain: Mainnet
Contract address to track (leave empty to use "Bored Ape Yacht Club"): 0x1f98431c8ad98523631ae4a59f267346ea31f984
Would you like to track another contract? (Leave empty if not): 
Tracking 1 contract(s), let's define a short name for each contract
Choose a short name for 1f98431c8ad98523631ae4a59f267346ea31f984 (lowercase and numbers only): factory
✔ Events only
Retrieving Ethereum Mainnet contract information (ABI & creation block)
Fetched contract ABI for 1f98431c8ad98523631ae4a59f267346ea31f984
Fetched initial block 12369621 for 1f98431c8ad98523631ae4a59f267346ea31f984 (lowest 12369621)
Generating ABI Event models for factory
  Generating ABI Events for FeeAmountEnabled (fee,tickSpacing)
  Generating ABI Events for OwnerChanged (oldOwner,newOwner)
  Generating ABI Events for PoolCreated (token0,token1,fee,tickSpacing,pool)
Track a dynamic datasource: y
Select the event on the factory that triggers the creation of a dynamic datasource:
Event: PoolCreated
Select the field on the factory event that provides the address of the dynamic datasource:
Field: pool
Choose a short name for the created datasource, (lowercase and numbers only): pool
✔ Events only
Enter a reference contract address to fetch the ABI: 0xc2e9f25be6257c210d7adf0d4cd6e3e881ba25f8
adding dynamic datasource pool PoolCreated pool
  Generating ABI Events for Burn (owner,tickLower,tickUpper,amount,amount0,amount1)
  Generating ABI Events for Collect (owner,recipient,tickLower,tickUpper,amount0,amount1)
  Generating ABI Events for CollectProtocol (sender,recipient,amount0,amount1)
  Generating ABI Events for Flash (sender,recipient,amount0,amount1,paid0,paid1)
  Generating ABI Events for IncreaseObservationCardinalityNext (observationCardinalityNextOld,observationCardinalityNextNew)
  Generating ABI Events for Initialize (sqrtPriceX96,tick)
  Generating ABI Events for Mint (sender,owner,tickLower,tickUpper,amount,amount0,amount1)
  Generating ABI Events for SetFeeProtocol (feeProtocol0Old,feeProtocol1Old,feeProtocol0New,feeProtocol1New)
  Generating ABI Events for Swap (sender,recipient,amount0,amount1,sqrtPriceX96,liquidity,tick)
Writing project files
Generating Protobuf Rust code
Project "uniswapv3_factory" initialized at "/Users/enolalvarezdeprado/Documents/projects/substreams/dsds/test"

Run 'make build' to build the wasm code.

The following substreams.yaml files have been created with different sink targets:
 * substreams.yaml: no sink target
 * substreams.sql.yaml: PostgreSQL sink
 * substreams.clickhouse.yaml: Clickhouse sink
 * substreams.subgraph.yaml: Sink into Substreams-based subgraph
```

{% endtab %}
{% endtabs %}