**With Substreams, you can extract data from the Injective blockchain**. Then, you can consume the data in several ways, such as a subgraph, streaming or using a SQL database.

<figure><img src="../../.gitbook/assets/intro/injective-logo.png" width="100%" /></figure>

# Getting Started

There are two main concepts in Substreams: packages and modules. Essentially, **a _module_ is a Rust function that contains definitions to extract data from the blockchain**. Modules are grouped in **_packages_, which are binary files (`.spkg`) that contain one or several modules**. For example, you might have a module called `injective-common`, which has two modules: `get_transactions` and `get_events`.

Developing a Substreams modules requires some knowledge of Rust, but the **StreaminFast team has already created several _foundational modules_, which extract the most basic data from the Injective blockchain**, such as transactions or events. You can simply import these modules and start using them!

To consume Injective data with The Graph you have two options:

**- Substreams-powered Subgraphs:** you import a Substreams module into the subgraph. Essentially, Substreams acts just as the extraction layer, but the business logic lives in the subgraph. The Substreams only provides _raw_ data (you use AssemblyScript to code your subgraph).

**- Substreams directly:** you develop your own Substreams module (you use Rust to code your Substreams).

Choosing one over the other is up to you. Consider the needs of your application and the skills needed to develop.

## Use Substreams-powered Subgraphs

First, if you do not know what subgraphs are, please take a look at [The Graph documentation](https://thegraph.com/docs/en/quick-start/).

Substreams can bridge Injective data to a subgraph through the [Substreams triggers](../consume/subgraph/triggers.md), which essentially allow you to import Substreams data into a subgraph. You can easily import any of the [Injective Substreams Foundational Modules](../tutorials/cosmos/injective/foundational.md) into your subgraph (for example, Injective transactions or events).

The following YAML file is the definition a subgraph that imports a Substreams module called `all_events`, which will stream all the events in the Injective blockchain.

```yaml
specVersion: 1.0.0
indexerHints:
  prune: auto
schema:
  file: ./schema.graphql
dataSources:
  - kind: substreams
    name: Events
    network: injective-mainnet
    source:
      package:
        file: injective-foundational-v0.1.0.spkg # 1.
        moduleName: all_events # 2.
    mapping:
      apiVersion: 0.0.7
      kind: substreams/graph-entities
      file: ./src/mapping.ts
      handler: handleEvents
```
1. The Substreams package you want to import into your subgraph.
2. The module (contained within the package) that you want to consume data from.

Although developing a Substreams-powered subgraph should be easy enough without knowing about the internal of Substreams, we still recommend to go through the following sections of the documentation:

- [Install the Substreams CLI](./installing-the-cli.md)
- [Authentication](./authentication.md)
- [Packages](./packages.md)
- [Injective Foundational Modules](../tutorials/cosmos/injective/foundational.md)
- [Set up a subgraph local environment](../tutorials/graph-node/local-development.md)
- [Injective Subgraph Example](../tutorials/cosmos/injective/usdt-exchanges.md)
- [Publish to The Graph decentralized network](../tutorials/graph-node/publish-decentralized-network.md)

## Use Subtreams Directly

If you don't want to use subgraphs, you can consume or develop a Substreams module.

First, you must consider whether you want to develop your own Substreams or consume a ready-to-use Substreams. **It is possible that someone has already built a Substreams package to extract the data you want**; you can explore Substreams packages in the [Substreams Registry](https://substreams.dev).

**If you have found a Substreams package that fits your needs**, then explore the [Consume Substreams](../consume/consume.md) section. At the most basic level you should cover:

- [Install the Substreams CLI](./installing-the-cli.md)
- [Authentication](./authentication.md)
- [Packages](./packages.md)
- Choose how you want to consume the data:
    - [Send the data to a SQL database.](./../consume/sql/sql.md)
    - [Stream the data from your application.](../consume/stream/stream.md)

**If you can't find a Substreams package that fits your needs**, then you can go ahead and develop your own Substreams by writing a Rust program. The [Develop Substreams](../develop/develop.md) section of the documentation covers everything you need to know about building a Substreams from scratch. At the most basic level, you should cover:

- [Install the Substreams CLI](./installing-the-cli.md)
- [Authentication](./authentication.md)
- [Manifest & Modules](./../common/manifest-modules.md)
- [Protobuf defitions](./../develop/creating-protobuf-schemas.md)
- [Packages](./packages.md)
- [Run a Substreams](./running-substreams.md)
- [Choose how you want to consume the data](./../consume/consume.md)

## The Injective Data Model

Substreams provides you access to the raw full Injective block through a [Protobuf schema](https://protobuf.dev/). You can use the [Block Protobuf](https://github.com/streamingfast/firehose-cosmos/blob/develop/cosmos/pb/sf/cosmos/type/v2/block.pb.go#L75) to retrieve all the information contained in an Injective block, such as transactions or events.

{% hint style="info" %}
**Note**: All Cosmos blockchains share the same data model, so the [Block Protobuf](https://github.com/streamingfast/firehose-cosmos/blob/develop/cosmos/pb/sf/cosmos/type/v2/block.pb.go#L75) used for Injective is the same for any other Cosmos blockchain.
{% endhint %}

You can use the Rust programming language to access this `Block` object and select which specific data you want to retrieve from the blockchain. For example, the following example receives the `Block` object as a parameter and returns a user-defined object, `BlockStats`.

```rust
pub fn block_to_stats(block: Block) -> Result<BlockStats, Error> { // 1.
    let mut stats = BlockStats::default(); // 2.
    let header =  block.header.as_ref().unwrap();
    let last_block_id = header.last_block_id.as_ref().unwrap();

    stats.block_height = block.height as u64; // 3.
    stats.block_hash = hex::encode(block.hash);
    stats.block_time = block.time;
    stats.block_proposer = hex::encode(&header.proposer_address);
    stats.parent_hash = hex::encode(&last_block_id.hash);
    stats.parent_height = block.height - 1i64;

    stats.num_txs = block.txs.len() as u64;

    Ok(stats)
}
```
1. Declaration of the Rust function.
**Input:** Injective block.
**Output:** `BlockStats` object, which is defined by the user and is consumable from the outside world.
2. Creation of the `BlockStats` object.
3. Add data from the `Block` Injective object to user-defined `BlockStats` object.

## Next Steps

To start developing Injective Substreams, take a look at the [BlockStats tutorial](../tutorials/cosmos/injective/block-stats.md), which inspects the code of a very simple Substreams. It's the best way to get familiar with the Substreams concepts!