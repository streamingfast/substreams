The `substreams init` command includes several ways of initializing your EVM Substreams. This command sets up an code-generated Substreams project, from which you can easily build either a subgraph or an SQL-based solution for handling data.

<figure><img src="../../../.gitbook/assets/intro/ethereum-logo.png" width="100%" /></figure>

## Create the Substreams Project

{% hint style="info" %}
**Important:** Take a look at the general [Getting Started Guide](../intro-how-to-guides.md) for more information on how to initialize your project.
{% endhint %}

1. Run the `substreams init` command to view a list of options to create the Substreams project.
1. There are two options to initialize an EVM Substreams:
    - `evm-minimal`: creates a simple Substreams that extracts raw data from the block (generates Rust code).
    - `evm-events-calls`: creates a Substreams that extracts EVM events and calls filtered by one or several smart contract addresses.
1. Complete the rest of questions, providing useful information, such as the **smart contract that you want to index**, or the name of the project.
1. After answering all the questions, a project will be generated. Follow the instructions to build, autenthicate and test your Substreams.
1. If you want to consume the Substreams data in a subgraph, use the `substreams codegen subgraph` command. Use the `substreams codegen sql` command to consume it in a SQL database.

## EVM Foundational Modules

The `evm-events-calls` codegen path relies on one of the [EVM Foundational Modules](https://github.com/streamingfast/substreams-foundational-modules/tree/develop/ethereum-common). A Foundational Module extracts the most relevant data from blockchain, so that you don't have to code it yourself.

Specifically, the `evm-events-calls` path uses the [filtered_events_and_calls](https://github.com/streamingfast/substreams-foundational-modules/blob/develop/ethereum-common/substreams.yaml#L142) module from the EVM Foundational Modules to retrieve all the events filtered by specific smart contract addresses.


