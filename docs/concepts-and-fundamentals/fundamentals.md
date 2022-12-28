---
description: StreamingFast Substreams fundamental knowledge
---

# Fundamentals

### Substreams Fundamentals

You will create and touch several pieces of technology including the [Substreams engine](fundamentals.md#the-substreams-engine), [Substreams command line interface](../reference-and-specs/command-line-interface.md), configuration files, [modules](modules.md), and [protobufs](../developers-guide/creating-protobuf-schemas.md).

The documentation explains how the different pieces fit together.

<figure><img src="../.gitbook/assets/Screen Shot 2022-10-11 at 3.00.58 PM.png" alt=""><figcaption><p>Substreams in Action</p></figcaption></figure>

### Important steps

* Identify smart contract addresses of interest including wallets, and decentralized exchanges (DEXs).
* Identify data, and define and create protobufs.
* Write Rust Substreams event handler functions.
* Update substreams manifest, point to protobufs and handlers.
* Issue command to Substreams CLI passing manifest.

### **The Substreams engine**

The Substreams engine basically is the CPU, or brain, of the Substreams system. The engine handles requests, **and** communication and orchestrates the transformation of blockchain data.

{% hint style="info" %}
Note: _The Substreams engine is responsible for running data transformations defined by developers to process targeted blockchain data._&#x20;
{% endhint %}

Developers send commands, flags, and a reference to the manifest configuration file through the Substreams CLI to the Substreams engine.&#x20;

Developers create the data transformation strategies in Substreams “[module handlers](../developers-guide/modules/setting-up-handlers.md)” defined using the [Rust programming language](https://www.rust-lang.org/). The module handlers act on protobuf-based data models referenced from within the Substreams manifest. Learn more about the protobufs for the different blockchains in the [chains and endpoints](../reference-and-specs/chains-and-endpoints.md) section of the Substreams documentation.

### **How Substreams modules communicate**

The Substreams engine runs the code defined by developers in the Rust-based module handlers.&#x20;

{% hint style="info" %}
**Note**: _**Substreams modules have a uni-directional data flow**_. Data is passed from one module to another, but only in a single direction.&#x20;
{% endhint %}

The data flow is defined in the [Substreams manifest](../reference-and-specs/manifests.md) through the “inputs” and “outputs” fields of the configuration file. These fields reference the protobuf definitions for blockchain data. The data flow is also defined using the “inputs” field to send data directly from one module to another.

### **What is a Substreams DAG?**

Substreams modules are composed through a [directed acyclic graph](https://en.wikipedia.org/wiki/Directed\_acyclic\_graph) (DAG).&#x20;

{% hint style="info" %}
**Note**: The flow of data from one module to another is determined by the fundamental rules and principles of DAGs. DAGs work through a one-directional flow.
{% endhint %}

The Substreams manifest references the modules, and the handlers defined within them and form the intention of how they are used by the Substreams engine.&#x20;

Directed acyclic graphs contain nodes, in this case, modules, that communicate in only one direction, passing from one node, or module, to another.

The Substreams engine creates the “compute graph”, or “dependency graph” at runtime through commands sent to the Substreams CLI using code in modules referenced by the manifest.

### **Protobufs for Substreams**

<figure><img src="../.gitbook/assets/Screen Shot 2022-10-25 at 1.44.19 PM.png" alt=""><figcaption><p>Substreams module handlers linked to protobuf</p></figcaption></figure>

View the protobuf file in the repository:

[https://github.com/streamingfast/substreams-template/blob/develop/proto/erc721.proto](https://github.com/streamingfast/substreams-template/blob/develop/proto/erc721.proto)

View the Rust module handlers in the lib.rs file:

[https://github.com/streamingfast/substreams-template/blob/develop/src/lib.rs](https://github.com/streamingfast/substreams-template/blob/develop/src/lib.rs)

[Protocol buffers](https://developers.google.com/protocol-buffers), or protobufs, are the data models operated on by the [Rust-based module handler functions](../developers-guide/modules/writing-module-handlers.md). Data models are defined and outlined in the protobufs.&#x20;

{% hint style="info" %}
**Note:** _Protobufs include the names of the data objects and the fields contained and accessible within them._&#x20;
{% endhint %}

Many of the protobuf definitions have already been created, such as the [erc721 token model](https://github.com/streamingfast/substreams-template/blob/develop/proto/erc721.proto), that are used by developers creating Substreams data transformation strategies.

Custom smart contracts targeted by developers, such as [UniSwap](https://github.com/streamingfast/substreams-playground/blob/master/modules/uniswap/proto/modules.proto), will have protobuf definitions that have already been created for them by others. The custom data models are referenced in the Substreams manifest and made available to module handler functions.&#x20;

In object-oriented programming terminology, the protobufs are the objects or object models. In front-end web development terms, protobufs are similar to the REST, or other data API.&#x20;

_**Firehose and Substreams treat the data as the API.**_

Protobufs provide the API to the targeted data for a smart contract address.

### **Writing rust modules for Substreams**

<figure><img src="../.gitbook/assets/Screen Shot 2022-10-11 at 2.48.46 PM.png" alt=""><figcaption><p>Writing Rust Modules for Substreams</p></figcaption></figure>

Designing an overall strategy for how to manage and transform data is the first thing developers will do when creating a Substreams implementation. Substreams modules are processed by the engine using the relationships defined in the manifest.&#x20;

The design and complexity of the modules and the way they work together will be based on the smart contracts and data being targeted by the developer.&#x20;

{% hint style="info" %}
_**Note**:_ Substreams modules work together by passing data from one module to another until finally returning an output transformed according to the rules in the manifest, modules, and module handler functions.&#x20;
{% endhint %}

Two types of module handlers are defined within the Rust modules; maps and stores. The two module types work in conjunction to sort, sift, temporarily store and transform blockchain data from smart contracts for use in data sinks, such as databases or subgraphs.

Continue to the modules documentation to learn more about detailed aspects of their use and purpose.
