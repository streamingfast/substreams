Integrating Substreams can be quick and easy. This guide will help you get started with consuming ready-made Substreams packages or developing your own. Substreams are permissionless. Grab a key [here](https://thegraph.market/), no personal information required, and start streaming on-chain data.

# Build

## Explore Available Substreams Packages

There are many ready-to-use Substreams packages available. You can explore these packages using the [**Substreams Registry**](https://substreams.dev). The registry lets you search for and find packages that meet your needs.

Once you find a package that fits your needs, you can choose how you want to consume the data:
- **[SQL Database](../how-to-guides/sinks/sql/sql.md)**: Send the data to a database.
- **[Direct Streaming](../how-to-guides/sinks/stream/stream.md)**: Stream data directly to your application.
- **[PubSub](../how-to-guides/sinks/pubsub.md)**: Send data to a PubSub topic.

<figure><img src=".gitbook/assets/intro/consume-flow.png" width="100%" /></figure>

## Optionally Develop Your Own Substreams

If you can't find a Substreams package that meets your specific needs, you can develop your own. Substreams are built with Rust, so you’ll write functions that extract and filter the data you need from the blockchain. The easiest way to get started is by referring to the [tutorial](../tutorials/intro-to-tutorials.md) section, enabling you to quickly filter data: 

- [EVM](../tutorials/evm.md)
- [Solana](../tutorials/solana/solana.md)
- [Starknet](../tutorials/starknet.md)
- [Injective](../tutorials/cosmos-compatible/injective.md)
- [MANTRA](../tutorials/cosmos-compatible/mantra.md)

To build and optimize your Substreams from zero, use the minimal path within the [Dev Container](../references/devcontainer-ref.md) to setup your environment and follow the [How-To Guides](../how-to-guides/develop-your-own-substreams/develop-your-own-substreams.md).

## Learn

- **Substreams Reliability Guarantees**: With a simple reconnection policy, Substreams guarantees you'll [Never Miss Data](../references/reliability-guarantees.md).
- **Substreams Architecture**: For a deeper understanding of how Substreams works, explore the [architectural overview](../references/architecture.md) of the data service.
- **Supported Networks**: Check-out which endpoints are supported [here](../references/chains-and-endpoints.md).
