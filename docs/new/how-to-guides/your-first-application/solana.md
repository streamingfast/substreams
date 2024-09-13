Check out the [Getting Started Guide](./intro-your-first-application.md) for more information on how to initialize your project. There are two options within `substreams init` to initialize your Solana Substreams:

- `sol-minimal`: Creates a simple Substreams that extracts raw data from the block.
- `sol-transactions`: Creates a Substreams that extracts Solana transactions filtered by one or more Program IDs and Account IDs.

{% hint style="info" %}
**Note**: The block model in your your generated project will not include Solana voting transactions. Excluding voting transactions reduces the costs and size of processing a Solana full-block by 75%. You can still access voting transactions by consuming a Solana full-block.
{% endhint %}

**Solana Foundational Modules**

The `sol-transactions` codegen path uses [Solana Foundational Modules](https://substreams.dev/streamingfast/solana-common/v0.3.0) to simplify filtering. These modules are designed to extract critical blockchain data, sparing you the need to write custom code. Specifically, the [filtered_transactions_without_votes](https://github.com/streamingfast/substreams-foundational-modules/blob/develop/solana-common/substreams-v0.2.0.yaml#L49) module extracts key data and accepts a regular expression to filter transactions, saving you from writing custom code.

<figure><img src="../../../.gitbook/assets/intro/solana-logo.png" width="100%" /></figure>