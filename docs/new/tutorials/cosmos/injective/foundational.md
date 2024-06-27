The [Injective Foundational Substreams](https://github.com/streamingfast/substreams-foundational-modules/injective-common) contains Substreams modules, which retrieve fundammental data on the Injective blockchain.

You can use the Injective Foundational Modules as the input for your Substreams or subgraph.

The Foundational Modules are the base of the code generation tools provided by the Substreams CLI.

## Before You Begin

- [Install the Substreams CLI](../../../common/installing-the-cli.md)
- [Get an authentication token](../../../common/authentication.md)
- [Learn about the basics of the Substreams](../../../common/manifest-modules.md)
- [Go through the Block Stats Substreams tutorial](./block-stats.md)

Clone the [Foundational Substreams GitHub repository](https://github.com/streamingfast/substreams-foundational-modules), move to the `injective-common` folder, and open it in an IDE of your choice (for example, VSCode).

## The Foundational Modules

First, take a look at the Substreams manifest (`substreams.yaml`), which contains the declaration of all the Injective Foundational Modules.

```yaml
...output omitted...

modules:
  - name: all_transactions # 1.
    kind: map
    initialBlock: 0
    inputs:
      - source: sf.cosmos.type.v2.Block
    output:
      type: proto:sf.substreams.cosmos.v1.TransactionList

  - name: all_events # 2.
    kind: map
    initialBlock: 0
    inputs:
      - source: sf.cosmos.type.v2.Block
    output:
      type: proto:sf.substreams.cosmos.v1.EventList

  - name: index_events # 3.
    kind: blockIndex
    inputs:
      - map: all_events
    output:
      type: proto:sf.substreams.index.v1.Keys
    doc: |
      `index_events` sets the keys corresponding to every event 'type' 
      ex: `coin_received`, `message` or `injective.peggy.v1.EventDepositClaim`

  - name: filtered_events # 4.
    kind: map
    blockFilter:
      module: index_events
      query:
        params: true
    inputs:
      - params: string
      - map: all_events
    output:
      type: proto:sf.substreams.cosmos.v1.EventList
    doc: |
      `filtered_events` reads from `all_events` and applies a filter on the event types, only outputing the events that match the filter. 
      The only operator that you should need to use this filter is the logical or `||`, because each event can only match one type.
```
1. The `all_transactions` module provides access to all the transactions of the Injective blockchain.
It receives a raw Injective block object as input (`sf.cosmos.type.v2.Block`), and outputs a list of transactions object (`sf.substreams.cosmos.v1.TransactionList`).
2. The `all_events` module provides access to all the events in the Injective blockchain.
It receives a raw Injective block as input (`sf.cosmos.type.v2.Block`), and outputs a list of events object (`sf.substreams.cosmos.v1.EventList`).
3. The `index_events` module uses the `all_events` module to create a cache where events are sorted based on their `type` field. This cache helps in the performance of the module. You can read more about _index modules_ in the [correspoding documentation](../../../develop/indexes).
4. The `filtered_events` allows you to use the `index_events` module (i.e. using the cache of events), to filter only the event types you are interested in.
The string parameter passed as input is used to specify which events you want to consume.

## Use The Foundational Modules

All this module are pre-programmed and ready to use in your Substreams or your subgraphs.

### Use in a Substreams

Using another module as input for your Substreams is very easy: you just have to declare it in the manifest.

For example, the following declaration of the `my_test_module` module receives the `all_transactions` module as input:

```yaml
- name: my_test_module
  kind: map
  inputs:
    - map: all_transactions
  output:
    type: proto:sf.test.MyOutputObject
```

Then, in the Rust handler declaration, you can simply receive the output object of the `all_transactions` module:

```rust
#[substreams::handlers::map]
fn my_test_module(transactions: TransactionList) -> Result<MyOutputObject, Error> {
    // Your code here
}
```

### Use in a Subgraph

You can easily import a Substreams module into a subgraph by defining it in the subgraph manifest (`subgraph.yaml`):

```yaml
specVersion: 1.0.0
indexerHints:
  prune: auto
schema:
  file: ./schema.graphql
dataSources:
  - kind: substreams
    name: Events
    network: injective-mainnet
    source:
      package:
        file: injective-foundational-v0.1.0.spkg # 1.
        moduleName: all_events # 2.
    mapping:
      apiVersion: 0.0.7
      kind: substreams/graph-entities
      file: ./src/mapping.ts # 3.
      handler: handleEvents # 4.
```
1. Define the Substreams package (`.spkg`) containing the module of your choice.
2. Define the module that you want to use (which must be contained in the package).
3. Define the file where you will create the handler.
4. Define the handler name.

Then, in the `mappings.ts` file you can create the `handleEvents` function.

```ts
export function handleEvents(bytes: Uint8Array): void { // 1.
    const eventList: EventList = Protobuf.decode<EventList>(bytes, EventList.decode); // 2.
    const events = eventList.events;

    // Your code here
}
```
1. Definition of the function, which receives the raw bytes of the Substreams.
2. Decode the bytes into the `EventList` Protobuf object, which is the output of the Substreams.

You can check out a full example in the [USDT Exchange Volume tutorial](../injective/usdt-exchanges.md)