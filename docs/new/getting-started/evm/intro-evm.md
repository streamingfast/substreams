Substreams allows you to easily extract data from the the Solana blockchain. With Substreams, you can retrieve transactions, instructions or accounts, taking advantage of its powerful streaming technology. It's super fast!

<figure><img src="../../../.gitbook/assets/intro/ethereum-logo.png" width="100%" /></figure>

## Getting Started

First, you must consider whether you want to develop your own Substreams or consume a ready-to-use Substreams. It is possible that someone has already built a Substreams package to extract the data you want; you can explore Substreams packages in the [Substreams Registry](https://substreams.dev).

**If you have found a Substreams package that fits your needs**, then explore the [Consume Substreams](../consume/consume.md) section. At the most basic level you should cover:

- [Install the Substreams CLI](./installing-the-cli.md)
- [Authentication](./authentication.md)
- [Packages](./packages.md)
- Choose how you want to consume the data:
    - [Send the data to a SQL database.](./../consume/sql/sql.md)
    - [Stream the data from your application.](../consume/stream/stream.md)
    - [Send the data to a subgraph.]((./../consume/subgraph/subgraph.md))

**If you can't find a Substreams package that fits your needs**, then you can go ahead and develop your own Substreams. The [Develop Substreams](../develop/develop.md) section of the documentation covers everything you need to know about building a Substreams from scratch. At the most basic level, you should cover:

- [Install the Substreams CLI](./installing-the-cli.md)
- [Authentication](./authentication.md)
- [Manifest & Modules](./../common/manifest-modules.md)
- [Protobuf defitions](./../develop/creating-protobuf-schemas.md)
- [Packages](./packages.md)
- [Run a Substreams](./running-substreams.md)
- [Choose how you want to consume the data](./../consume/consume.md)

## The Ethereum Data Model

Substreams provides you access to the raw full Ethereum block through a [Protobuf schema](https://protobuf.dev/). You can use the [Block Protobuf](https://github.com/streamingfast/firehose-ethereum/blob/develop/proto/sf/ethereum/type/v2/type.proto#L51) to retrieve all the information contained in an Ethereum block, such as transactions or events.

{% hint style="info" %}
**Note**: All EVM blockchains share the same data model, so the [Block Protobuf](https://github.com/streamingfast/firehose-ethereum/blob/develop/proto/sf/ethereum/type/v2/type.proto#L51) used for Ethereum is the same for any other EVM blockchain, such as Arbitrum or Polygon.
{% endhint %}

## Tutorials

The [Exploring Ethereum Substreams](../../tutorials/evm/exploring-ethereum/exploring-ethereum.md) tutorial will guide through the most basic concepts of Substreams.

