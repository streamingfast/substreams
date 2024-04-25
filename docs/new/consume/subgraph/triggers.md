**Substreams triggers allow you to embed Substreams data directly in your subgraph**. Essentially, you **import the Protobufs emitted by your Substreams module** and you receive the data in the handler of your subgraph.

For example, consider that you want to consume the transactions emitted by the [map_filter_transactions module](https://github.com/streamingfast/substreams-explorers/blob/main/ethereum-explorer/src/map_filter_transactions.rs) of the Ethereum Explorer Substreams. With Substreams triggers, you can define a special subgraph handler that imports the Protobuf object and lets you manipulate the data just like a usual AssemblyScript object.

The Protobuf used in the `map_filter_transactions` module is the following:

```protobuf
syntax = "proto3";

package eth.transaction.v1;

message Transactions {
  repeated Transaction transactions = 1;
}

message Transaction {
  string from = 1;
  string to = 2;
  string hash = 3;
}
```

You can generate the previous Protobuf in AssemblyScript and import it as part of your subgraph in a special handler:

```ts
export function handleTransactions(bytes: Uint8Array): void {
    let transactions = assembly.eth.transaction.v1.Transactions.decode(bytes.buffer).trasanctions; // 1.
    if (transactions.length == 0) {
        log.info("No transactions found", []);
        return;
    }

    for (let i = 0; i < transactions.length; i++) { // 2.
        let transaction = transactions[i];

        let entity = new Transaction(transaction.hash); // 3.
        entity.from = transaction.from;
        entity.to = transaction.to;
        entity.save();
    }
}
```
1. You decode the bytes (which contains the Substreams data) into the generated object, `Transactions`.
Now you can use the object like any other AssemblyScript object.
2. Loop over the transactions
3. Create a new subgraph entity for every transaction.

## Tutorial: Import the Substreams Explorer package transactions into a subgraph

Following the previous example, the [Substreams Sink Examples](https://github.com/streamingfast/substreams-sink-examples) repository, contains a subgraph importing the transactions through Substreams triggers. Clone the repository and move to the `subgraph-triggers-transactions`. To run this tutorial you will need:

- The Graph CLI, to build and deploy the subgraph.
- Node (>17) and NPM installed.
- The `protoc` command installed, to generate the Protobuf schemas.

### Install the Dependencies

If you are familiar with subgraphs, then the structure of the project should be easy to understand:

// image

1. The `proto` folder contains the Protobuf definitions of your Substreams (i.e. the definitions you want to import into the subgraph).
2. The `src` folder contains the source code. Mainly, the `mapping.ts` file with the handlers and the `pb` folder with the generated TS code for the Protobuf.
3. Currently, it is necessary to have the Substreams package (`spkg`) that you want to import in your filesystem, so that you subgraph can read it.


- To get started, install the dependencies of the project:

```bash
npm install
```

- Generate the Substreams Protobufs:

```bash
protoc --plugin=./node_modules/protobuf-as/bin/protoc-gen-as --as_out=src/pb/ ./proto/*.proto
```

The previous command takes any Protobuf file contained in the `proto` folder and generates a TypeScript model in the `src/pb` folder, so that you can import the TS code into your subgraph.

- Generate the subgraph schema:

```bash
graph codegen
```

### Inspect the Code

- The `subgraph.yaml` file defines the Substreams triggers as a data source.
The subgraph will be deploy on Ethereum Mainnet (`mainnet`).

```yaml
specVersion: 1.0.0
indexerHints:
  prune: auto
schema:
  file: ./schema.graphql
dataSources:
  - kind: substreams
    name: Transaction
    network: mainnet # Ethereum mainnet
    source:
      startBlock: 17239000
      package:
        moduleName: map_filter_transactions # Module name
        file: ethereum-explorer-v0.1.2.spkg # Package
    mapping:
      apiVersion: 0.0.7
      kind: substreams/graph-entities
      file: ./src/mapping.ts # Path of the mapping file.
      handler: handleTransactions # Name of the handler function of the trigger
```

- The `src/mapping.ts` contains the handler of the trigger, `handleTransactions`:

```ts
import { log } from "@graphprotocol/graph-ts";
import * as assembly from "./pb/assembly"; // 1.
import { Transaction } from "../generated/schema"; // 2.

export function handleTransactions(bytes: Uint8Array): void {
    let transactions = assembly.eth.transaction.v1.Transactions.decode(bytes.buffer).transactions;
    if (transactions.length == 0) {
        log.info("No transactions found", []);
        return;
    }

    for (let i = 0; i < transactions.length; i++) {
        let transaction = transactions[i];

        let entity = new Transaction(transaction.hash);
        entity.from = transaction.from;
        entity.to = transaction.to;
        entity.save();
    }
}
```
1. Import the generated Substreams Protobuf.
2. Import the generated GraphQL schema.
3. Decode the Substreams Protobuf.
4. Create and save the subgraph entity.

### Build and Deploy the Subgraph

To test the application, you can deploy the subgraph to [The Graph Studio](https://thegraph.com/studio/). You must create an account and authentication your computer first. The [official documentation](https://thegraph.com/docs/en/deploying/subgraph-studio/) covers the steps needed to deploy a subgraph to the Studio.

- Build the subgraph:

```bash
graph build
```

- Deploy the subgraph:

```bash
graph deploy
```

Now, you can access and query the subgraph in the Studio.
