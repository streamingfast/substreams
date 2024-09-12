The `substreams init` command includes several ways of initializing your Solana Substreams.This command sets up an code-generated Substreams project, from which you can easily build either a subgraph or an SQL-based solution for handling data.

<figure><img src="../../../.gitbook/assets/intro/solana-logo.png" width="100%" /></figure>

## Create the Substreams Project

{% hint style="info" %}
**Important:** Take a look at the general [Getting Started Guide](../intro-how-to-guides.md) for more information on how to initialize your project.
{% endhint %}

1. Run the `substreams init` command to view a list of options to create the Substreams project.
1. There are two options to initialize a Solana Substreams:
    - `sol-minimal`: creates a simple Substreams that extracts raw data from the block (generates Rust code).
    - `sol-transactions`: creates a Substreams that extracts Solana transactions filtered by one or several Program IDs.
1. Complete the rest of questions, providing useful information, such as the **Program IDs that you want to use to filter the transactions**, or the name of the project.
1. After answering all the questions, a project will be generated. Follow the instructions to build, autenthicate and test your Substreams.
1. If you want to consume the Substreams data in a subgraph, use the `substreams codegen subgraph` command. Use the `substreams codegen sql` command to consume it in a SQL database.

**Tips:**

- The project generated by the codegen will NOT include Solana voting transactions. By excluding voting transactions, we reduce significantly the costs of running the Substreams. You can always consume voting transactions by using a Solana raw block.

## Solana Foundational Modules

The `sol-transactions` path of the codegen, which filters the transactions, relies on the [Solana Foundational Modules](https://github.com/streamingfast/substreams-foundational-modules/tree/develop/solana-common). A Foundational Module extracts the most relevant data from blockchain, so that you don't have to code it yourself.

Specifically, the `sol-transactions` path uses the [filtered_transactions_without_votes](https://github.com/streamingfast/substreams-foundational-modules/blob/develop/solana-common/substreams-v0.2.0.yaml#L49) module, which accepts a regex (regular expression) as input to filter the transactions.