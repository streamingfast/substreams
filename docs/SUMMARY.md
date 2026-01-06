# Table of contents

* [Introduction](README.md)
* [Getting Started](./getting-started.md)

## Tutorials

* [Generate Your First Substreams](tutorials/intro-to-tutorials.md)
  * [on EVM](tutorials/evm.md)
  * on Solana
    * [Transactions & Instructions](tutorials/solana/solana.md)
    * [Account Changes](tutorials/solana/account-changes.md)
  * [on NEAR](tutorials/near.md)
  * [on Monad](tutorials/monad.md)
  * [on TRON](tutorials/tron.md)
  * [on Injective](tutorials/cosmos-compatible/injective.md)
  * [on MANTRA](tutorials/cosmos-compatible/mantra.md)
  * [on Starknet](tutorials/starknet.md)
  * [on Stellar](tutorials/stellar.md)
  * [on World Chain](tutorials/world-chain.md)
* [Consuming a Foundational Store](tutorials/consuming-foundational-store.md)

## How-To Guides

* [Install Substreams CLI](how-to-guides/cli/installing-the-cli.md)
  * [Substreams CLI Authentication](how-to-guides/cli/authentication.md)
* [Developing Substreams](how-to-guides/develop-your-own-substreams/develop-your-own-substreams.md)
  * [on EVM](how-to-guides/develop-your-own-substreams/evm)
    * Local Development
      * [HardHat](how-to-guides/develop-your-own-substreams/evm/local-development/hardhat.md)
      * [Foundry](how-to-guides/develop-your-own-substreams/evm/local-development/foundry.md)
    * [Exploring Ethereum](how-to-guides/develop-your-own-substreams/evm/exploring-ethereum/exploring-ethereum.md)
      * [Filter Transactions](how-to-guides/develop-your-own-substreams/evm/exploring-ethereum/map_filter_transactions_module.md)
      * [Retrieve Events of a Smart Contract](how-to-guides/develop-your-own-substreams/evm/exploring-ethereum/map_contract_events_module.md)
    * [Making eth\_calls](how-to-guides/develop-your-own-substreams/evm/eth-calls.md)
  * [on Solana](how-to-guides/develop-your-own-substreams/solana/solana.md)
    * Local Development
      * [Anchor](how-to-guides/develop-your-own-substreams/solana/local-development/anchor.md)
    * [Explore Solana](how-to-guides/develop-your-own-substreams/solana/explore-solana/explore-solana.md)
      * [Filter Instructions](how-to-guides/develop-your-own-substreams/solana/explore-solana/filter-instructions.md)
      * [Filter Transactions](how-to-guides/develop-your-own-substreams/solana/explore-solana/filter-transactions.md)
    * [SPL Token Tracker](how-to-guides/develop-your-own-substreams/solana/token-tracker/token-tracker.md)
    * [NFT Trades](how-to-guides/develop-your-own-substreams/solana/top-ledger/nft-trades.md)
    * [DEX Trades](how-to-guides/develop-your-own-substreams/solana/top-ledger/dex-trades.md)
    * [From Yellowstone to Substreams](how-to-guides/develop-your-own-substreams/solana/migrate-from-yellowstone.md)
  * [on Cosmos](how-to-guides/develop-your-own-substreams/cosmos)
    * [Injective](how-to-guides/develop-your-own-substreams/cosmos/injective)
      * [Simple Substreams Example](how-to-guides/develop-your-own-substreams/cosmos/injective/block-stats.md)
      * [Foundational Modules](how-to-guides/develop-your-own-substreams/cosmos/injective/foundational.md)
  * General
    * [Local Development](how-to-guides/develop-your-own-substreams/generic/local-development/README.md)
      * [Troubleshooting](how-to-guides/develop-your-own-substreams/generic/local-development/troubleshooting.md)
    * [Using Rust & Protobuf](how-to-guides/develop-your-own-substreams/generic/using-rust-proto.md)
    * [Rust](how-to-guides/develop-your-own-substreams/generic/rust/rust.md)
      * [Option struct](how-to-guides/develop-your-own-substreams/generic/rust/option.md)
      * [Result struct](how-to-guides/develop-your-own-substreams/generic/rust/result.md)
    * [Protobuf](how-to-guides/develop-your-own-substreams/generic/creating-protobuf-schemas.md)
* [Composing Substreams](how-to-guides/composing-substreams/composing-substreams.md)
  * [Foundational Modules](how-to-guides/composing-substreams/foundational-modules.md)
  * [Foundational Stores](how-to-guides/composing-substreams/foundational-stores/foundational-stores.md)
    * [Ethereum - ERC20 Token Metadata](how-to-guides/composing-substreams/foundational-stores/ethereum/erc20-token-metadata.md)
    * [Solana - SPL Initialized Account](how-to-guides/composing-substreams/foundational-stores/solana/spl-initialized-account.md)
  * [Published Packages](how-to-guides/composing-substreams/published-packages.md)
* [Consuming Substreams](how-to-guides/sinks/sinks.md)
  * [Substreams:SQL](how-to-guides/sinks/sql/sql.md)
    * [Using Relational Mappings](how-to-guides/sinks/sql/relational-mappings.md)
    * [Using Database Changes](how-to-guides/sinks/sql/db_out.md)
  * [Substreams:Stream](how-to-guides/sinks/stream/stream.md)
    * [JavaScript](how-to-guides/sinks/stream/javascript.md)
    * [Go](how-to-guides/sinks/stream/go.md)
  * [Substreams:PubSub](how-to-guides/sinks/pubsub.md)
  * [Files](how-to-guides/sinks/files.md)
  * [Community Sinks](how-to-guides/sinks/community/other-sinks)
    * [MongoDB](how-to-guides/sinks/community/other-sinks/mongodb.md)
    * [Key-Value Store](how-to-guides/sinks/community/other-sinks/kv.md)
    * [Prometheus](how-to-guides/sinks/community/other-sinks/prometheus.md)
* [Publishing a Substreams Package](how-to-guides/publish-package.md)

## Reference Material

* [CLI Reference](references/cli/command-line-interface.md)
* Core Concepts
  * [Architecture & Parallel Execution](references/architecture.md)
  * [Foundational Stores](references/foundational-store-reference.md)
  * [Module Concepts](references/substreams-components/modules/modules.md)
  * [Reliability Guarantees](references/reliability-guarantees.md)
  * [FAQ](references/faq.md)
* Manifest & Components
  * [Manifests](references/substreams-components/manifests.md)
  * [Packages](references/substreams-components/packages.md)
  * [Module Types](references/substreams-components/modules/types.md)
  * [Module Inputs](references/substreams-components/modules/inputs.md)
  * [Module Outputs](references/substreams-components/modules/outputs.md)
  * [Module Handlers](references/substreams-components/modules/setting-up-handlers.md)
  * [Indexes](references/substreams-components/modules/indexes.md)
  * [Keys in Stores](references/substreams-components/modules/keys-in-stores.md)
  * [Parameterized Modules](references/substreams-components/modules/parameterized-modules.md)
  * [Dynamic Data Sources](references/substreams-components/modules/dynamic-data-sources.md)
  * [Aggregation Windows](references/substreams-components/modules/aggregation-windows.md)
* Chain Support
  * [Chains & Endpoints](references/chains-and-endpoints.md)
  * [Ethereum Data Model](references/ethereum-data-model.md)
  * [Flashblocks support](references/flashblocks.md)
* [Sinks Reference](references/sql/README.md)
  * SQL
    * [Sink Config](references/sql/sink-config.md)
    * [DSN Reference](references/sql/dsn-reference.md)
    * [Reorg Handling](references/sql/reorg-handling.md)
* Operators
  * [Hosting Foundational Stores](./references/foundational-stores/hosting-foundational-stores.md)
* Development Tools
  * [Logging & Debugging](references/log-and-debug.md)
  * [Dev Container Reference](references/devcontainer-ref.md)
* [Change log](release-notes/change-log.md)

## Decentralized Indexing

* [What is The Graph?](https://thegraph.com/docs/en/about/)
