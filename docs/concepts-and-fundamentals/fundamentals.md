---
description: StreamingFast Substreams fundamental knowledge
---

# Fundamentals

## Fundamentals overview

Substreams development involves using several different pieces of technology, including the [Substreams engine](fundamentals.md#the-substreams-engine), [`substreams` command line interface (CLI)](../reference-and-specs/command-line-interface.md), [modules](modules.md), [protobufs](../developers-guide/creating-protobuf-schemas.md), and various configuration files of different types. The documentation explains how these pieces fit together.

<figure><img src="../.gitbook/assets/substreams-breakdown-in-action.png" alt=""><figcaption><p>Substreams in Action</p></figcaption></figure>

### The process to use Substreams includes:

* Choosing the blockchain to capture and process data.
* Identifying interesting smart contract addresses, including wallets and decentralized exchanges (DEXs).
* Identifying data and defining and creating protobufs.
* Writing Rust Substreams module handler functions.
* Updating the Substreams manifest to reference the protobufs and module handlers.
* Using the [`substreams` CLI](../reference-and-specs/command-line-interface.md) to send commands and view results.

### **The Substreams engine**

The Substreams engine serves as the CPU or brain of the Substreams system, handling requests, communication, and orchestrating the transformation of blockchain data.

{% hint style="info" %}
**Note**: The Substreams engine is responsible for running developer-defined data transformations to process blockchain data.
{% endhint %}

Developers use the [`substreams` CLI](../reference-and-specs/command-line-interface.md) to send commands, flags, and a reference to the manifest configuration file to the Substreams engine. They create data transformation strategies in Substreams "_module handlers_" using the Rust programming language, which acts on protobuf-based data models referenced from within the Substreams manifest.

### **Substreams module communication**

The Substreams engine runs the code defined by developers in Rust-based module handlers.

{% hint style="info" %}
**Note**: Substreams modules have **unidirectional data flow,** meaning data is passed from one module to another in a single direction.
{% endhint %}

The data flow is [defined in the Substreams manifest](../reference-and-specs/manifests.md) through the "inputs" and "outputs" fields of the configuration file, which reference the protobuf definitions for blockchain data. The data flow is also defined by using the "inputs" field to send data directly from one module to another.

### **Substreams DAG**

Substreams modules are composed through a [directed acyclic graph](https://en.wikipedia.org/wiki/Directed\_acyclic\_graph) (DAG).

{% hint style="info" %}
**Note**: In DAGs, data flows from one module to another in a one-directional manner, governed by the fundamental rules and principles of DAGs.
{% endhint %}

The Substreams manifest references the modules and the handlers defined within them, forming the intention of how they are used by the Substreams engine.

Directed acyclic graphs contain nodes, which in this case are modules communicating in only one direction, passing from one node or module to another.

The Substreams engine creates the "_compute graph_" or "_dependency graph_" at run time through commands sent to the [`substreams` CLI](../reference-and-specs/command-line-interface.md) using the code in modules referenced by the manifest.

### **Protobufs for Substreams**

<figure><img src="../.gitbook/assets/substreams-breakdown-map-module-handler (1) (1).png" alt=""><figcaption><p>Substreams module handlers linked to protobuf</p></figcaption></figure>

[Protocol buffers or protobufs](https://developers.google.com/protocol-buffers) are the data models operated on by the[ Rust-based module handler functions](../developers-guide/modules/writing-module-handlers.md). They define and outline the data models in the protobufs.

* View the [`erc721.proto`](https://github.com/streamingfast/substreams-template/blob/develop/proto/erc721.proto) protobuf file in the [Substreams Template repository](https://github.com/streamingfast/substreams-template).
* View the Rust module handlers in the [`lib.rs`](https://github.com/streamingfast/substreams-template/blob/develop/src/lib.rs) file in the [Substreams Template repository](https://github.com/streamingfast/substreams-template).

{% hint style="info" %}
**Note:** Protobufs include the names of the data objects and the fields contained and accessible within them.
{% endhint %}

Many protobuf definitions have already been created, such as [the erc721 token model](https://github.com/streamingfast/substreams-template/blob/develop/proto/erc721.proto), for use by developers creating Substreams data transformation strategies.

Custom smart contracts, [like UniSwap](https://github.com/streamingfast/substreams-uniswap-v3/blob/e4b0fb016210870a385484f29bb5116931ea9a50/proto/uniswap/v1/uniswap.proto), also have protobuf definitions that are referenced in the Substreams manifest and made available to module handler functions. Protobufs provide an API to the data for smart contract addresses.

In object-oriented programming terminology, protobufs are the objects or object models. In front-end web development, they are similar to REST or other data APIs.

{% hint style="success" %}
**Tip**: Firehose and Substreams **treat the data as the API**.
{% endhint %}

### **Substreams Rust modules**

<figure><img src="../.gitbook/assets/Screen%20Shot%202022-10-11%20at%202.48.46%20PM%20(1).png" alt=""><figcaption><p>Writing Rust Modules for Substreams</p></figcaption></figure>

The first step in Substreams development is to design an overall strategy for managing and transforming data. The Substreams engine processes modules by using the relationships defined in the manifest.

{% hint style="info" %}
**Note**_:_ Substreams modules work together by passing data from one module to another until they finally return an output transformed according to the rules in the manifest, modules, and module handler functions.
{% endhint %}

Modules define two types of module handlers: `map` and `store`. These two types work together to sort, sift, temporarily store, and transform blockchain data from `Block` objects and smart contracts for use in data sinks such as databases or subgraphs.
