Check out the [Getting Started Guide](./intro-your-first-application.md) for more information on how to initialize your project. There are two options within `substreams init` to initialize your Solana Substreams:

- `sol-minimal`: Creates a simple Substreams that extracts raw data from the block.
- `sol-transactions`: Creates a Substreams that extracts Solana transactions filtered by one or more Program IDs and Account IDs.

## Solana Foundational Modules

The `sol-transactions` path of the codegen, which filters the transactions, relies on the [Solana Foundational Modules](https://github.com/streamingfast/substreams-foundational-modules/tree/develop/solana-common). A Foundational Module extracts the most relevant data from blockchain, so that you don't have to code it yourself.

Specifically, the `sol-transactions` path uses the [filtered_transactions_without_votes](https://github.com/streamingfast/substreams-foundational-modules/blob/develop/solana-common/substreams-v0.2.0.yaml#L49) module, which accepts a regex (regular expression) as input to filter the transactions.

<figure><img src="../../../.gitbook/assets/intro/solana-logo.png" width="100%" /></figure>