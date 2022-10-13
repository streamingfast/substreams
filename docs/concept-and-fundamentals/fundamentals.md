---
description: StreamingFast Substreams fundamental knowledge
---

# Fundamentals

### Working with Substreams Fundamentals

Developers working with Substreams will create and touch many separate pieces of technology including the Substreams engine, command line interface, configuration files, Rust modules, and protobufs.

This documentation aims to outline information to further help developers working with Substreams. Specifically, how the different pieces fit together including the manifest, protobufs, Rust modules, module handlers, WASM, and Substreams CLI.

<figure><img src="../.gitbook/assets/Screen Shot 2022-10-11 at 3.00.58 PM.png" alt=""><figcaption><p>Substreams in Action</p></figcaption></figure>

### Building a Substream Key Steps

Key steps in the process include:

* Identify smart contract addresses of interest (wallets, DEXs, etc.)
* Create protobufs
* Write Rust Substreams event handler functions
* Update substreams manifest, point to protobufs and handlers

### **The Substreams Engine**

The Substreams engine basically is the CPU, or brain, of the Substreams system. The engine handles requests, communication and orchestrates the transformation of blockchain data.

The engine is responsible for running data transformations defined by developers to process targeted blockchain data.&#x20;

Developers will send commands, flags, and a reference to the manifest configuration file through the Substreams CLI to the Substreams engine.&#x20;

Developers create the data transformation strategies in Substreams “module handlers” defined using the Rust programming language. The module handlers act on protobuf-based data models referenced in from within the Substreams manifest.

### **How Substreams Modules Communicate**

The Substreams engine runs the code defined by developers in the Rust-based module handlers. Substreams modules have a uni-directional flow of data. The data can be passed from one module to another, but only in a single direction.&#x20;

The flow of data is defined in the Substreams manifest through the “inputs” and “outputs” fields of the configuration file. These fields generally reference the protobuf definitions for the targeted blockchain data. The flow of data can also be defined using the “input” field to send data directly from one module to another.

### **What is a Substreams DAG?**

Substreams modules are composed through a directed acyclic graph (DAG).&#x20;

The flow of data from one module to another is determined by the fundamental rules and principles of DAGs. The Substreams manifest references the modules, the handlers defined within them, and lays out the intention of how each is used by the Substreams engine.&#x20;

Directed acyclic graphs contain nodes, in this case, modules, that communicate in only one direction, passing from one node to another.

The Substreams engine creates the “compute graph”, or “dependency graph” at runtime through commands sent to the CLI.

### **Protobufs for Substreams**

<figure><img src="../.gitbook/assets/Screen Shot 2022-10-12 at 2.56.56 PM.png" alt=""><figcaption><p><strong>Substreams Module Handlers &#x26; Protobufs</strong></p></figcaption></figure>

Protobufs are the data models operated on by the Rust-based module handler functions. Data models are defined and outlined in the protobufs, including the names of the data objects and the fields accessible within them.&#x20;

Many of the protobuf definitions have already been created, such as the erc721 token model, that can be used by developers creating Substreams data transformation strategies.

Custom smart contracts targeted by developers can also have protobuf definitions created for them, such as UniSwap. The custom data models are referenced in the Substreams manifest and made available to module handler functions.&#x20;

In object-oriented programming terminology, the protobufs are the objects or object models. In front-end web development terms, protobufs are similar to the REST, SOAP, or other data access API.&#x20;

Protobufs essentially provide the API to the targeted data, usually associated with a smart contract address.

### **Writing Rust Modules for Substreams**

<figure><img src="../.gitbook/assets/Screen Shot 2022-10-11 at 2.48.46 PM.png" alt=""><figcaption><p>Writing Rust Modules for Substreams</p></figcaption></figure>

Designing an overall strategy for how to manage and transform data is the first thing developers will do when creating a Substreams implementation. Substreams modules are processed by the engine with the relationships between them defined in the manifest.&#x20;

The design and complexity of the modules and the way they work together will be based on the smart contracts and data being targeted by the developer.&#x20;

Substreams modules work together by passing data from one to another until finally returning an output transformed according to the rules in the manifest, modules, and module handler functions.&#x20;

Two types of module handlers are defined within the Rust modules; maps and stores. The two module types work in conjunction to soft, sift, temporarily store and transform blockchain data from smart contracts for use in data sinks, such as databases or subgraphs.
