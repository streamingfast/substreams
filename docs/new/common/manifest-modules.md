---
description: Learn the basics about modules and manifests
---

## Modules and Manifests

In Substreams, manifests and modules are concepts tighly related because they are fundamental to understand how Substreams works.

In simple terms, a Substreams module is a Rust function that receives an input and returns an output. For example, the following Rust function receives an Ethereum block and returns a custom object containing fields such as block number, hash or parent hash.

```rust
fn get_my_block(blk: Block) -> Result<MyBlock, substreams::errors::Error> {
    let header = blk.header.as_ref().unwrap();

    Ok(MyBlock {
        number: blk.number,
        hash: Hex::encode(&blk.hash),
        parent_hash: Hex::encode(&header.parent_hash),
    })
}
```

And also in simple terms, a Substreams manifest (`substreams.yaml`) is a configuration file (a YAML file) for your Substreams, which defines the different modules (functions) for your Substreams, among other configurations. For example, the following manifest receives a raw Ethereum block as input (`sf.ethereum.type.v2.Block`) and outputs a custom object (`eth.example.MyBlock`).

```yaml
modules:
  - name: map_block
    kind: map
    initialBlock: 12287507
    inputs:
      - source: sf.ethereum.type.v2.Block
    output:
      type: proto:eth.example.MyBlock
```

Among other things, the manifest allows you to define:
- How many modules your Substreams uses, along with their corresponding inputs and outputs.
- The schema(s) (i.e. the data model) your Substreams uses.
- How you will consume the data emitted by your Substreams (SQL, Webhooks...).

## Module Chaining

Modules were built with composability in mind, so it is possible to chain them. Given two modules, `module1` and `module2`, you can set the output of `module1` to be the input of `module2`, creating a chain of interconnected Substreams modules. Let's take a look at the following example:

```yaml
modules:
  - name: map_events
    kind: map
    initialBlock: 4634748
    inputs:
      - source: sf.ethereum.type.v2.Block
    output:
      type: proto:contract.v1.Events

  - name: db_out
    kind: map
    initialBlock: 4634748
    inputs:
      - map: map_events
    output:
      type: proto:sf.substreams.sink.database.v1.DatabaseChanges
```

There are two modules defined: `map_events` and `graph_out`.
- The `map_events` module receives a `sf.ethereum.type.v2.Block` object (a raw Ethereum block) as a parameter and outputs a custom `contract.v1.Events` object.
- The `db_out` module receives `map_events`'s output as an input, and outputs another custom object, `sf.substreams.sink.database.v1.DatabaseChanges`.

Technically, modules have one or more inputs, which can be in the form of a `map` or `store`, or a `Block` or `Clock` object received from the blockchain's data source. Every time a new `Block` is processed, all of the modules are executed as a directed acyclic graph (DAG).

## Module Kinds

There are two types of modules: `map` and `store`. `map` modules are used for stateless transformations and `store` modules are used for stateful transformations.

Substreams executes the Rust function associated with module for every block on the blockchain, but there will be times when you will have to save data between blocks. `store` modules allow you to save in-memory data.

### `map` modules

`map` modules are used for data extraction, filtering, and transformation. They should be used when direct extraction is needed avoiding the need to reuse them later in the DAG.

To optimize performance, you should use a single `map` module instead of multiple `map` modules to extract single events or functions. It is more efficient to perform the maximum amount of extraction in a single top-level `map` module and then pass the data to other Substreams modules for consumption. This is the recommended, simplest approach for both backend and consumer development experiences.

Functional `map` modules have several important use cases and facts to consider, including:

* Extracting model data from an event or function's inputs.
* Reading data from a block and transforming it into a custom protobuf structure.
* Filtering out events or functions for any given number of contracts.

### `store` modules

`store` modules are used for the aggregation of values and to persist state that temporarily exists across a block.

{% hint style="warning" %}
**Important:** Stores should not be used for temporary, free-form data persistence.
{% endhint %}

Unbounded `store` modules are discouraged. `store` modules shouldn't be used as an infinite bucket to dump data into.

Notable facts and use cases for working `store` modules include:

* `store` modules should only be used when reading data from another downstream Substreams module.
* `store` modules cannot be output as a stream, except in development mode.
* `store` modules are used to implement the Dynamic Data Sources pattern from Subgraphs, keeping track of contracts created to filter the next block with that information.
* Downstream of the Substreams output, do not use `store` modules to query anything from them. Instead, use a sink to shape the data for proper querying.

## Defining Modules

Modules are defined as a YAML list under the `modules` section of the manifest. In the following example, a `map_events` module is defined:

```yaml
modules:
  - name: map_events
    kind: map
    initialBlock: 4634748
    inputs:
      - source: sf.ethereum.type.v2.Block
    output:
      type: proto:contract.v1.Events
```

Then, you create the corresponding Rust function under the `src/lib.rs` file.

```rust
#[substreams::handlers::map]
fn map_events(blk: eth::Block) -> Result<contract::Events, substreams::errors::Error> {

...output omitted...

}
```
