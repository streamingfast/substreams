The `substreams init` command includes several ways of initializing your Injective Substreams. This command sets up an code-generated Substreams project, from which you can easily build either a subgraph or an SQL-based solution for handling data.

<figure><img src="../../../.gitbook/assets/intro/injective-logo.png" width="100%" /></figure>

## Create the Substreams Project

{% hint style="info" %}
**Important:** Take a look at the general [Getting Started Guide](../intro-how-to-guides.md) for more information on how to initialize your project.
{% endhint %}

1. Run the `substreams init` command to view a list of options to create the Substreams project.
1. There are two options to initialize an Injective Substreams:
    - `injective-minimal`: creates a simple Substreams that extracts raw data from the block (generates Rust code).
    - `injective-events`: creates a Substreams that extracts Injective events filtered by _type_ and/or _attributes_.
1. Complete the rest of questions, providing useful information, such as the **the type of events that you want to index**, or the name of the project.
1. After answering all the questions, a project will be generated. Follow the instructions to build, autenthicate and test your Substreams.
1. If you want to consume the Substreams data in a subgraph, use the `substreams codegen subgraph` command. Use the `substreams codegen sql` command to consume it in a SQL database.

## Injective Foundational Modules

The `injective-events` codegen path relies on one of the [Injective Foundational Modules](https://github.com/streamingfast/substreams-foundational-modules/tree/develop/injective-common).  A Foundational Module extracts the most relevant data from blockchain, so that you don't have to code it yourself.

Specifically, the `injective-events` path uses the [filtered_events](https://github.com/streamingfast/substreams-foundational-modules/blob/develop/injective-common/substreams.yaml#L58) module from the Injective Foundational Modules to retrieve all the events filtered by event type and/or attributes.


