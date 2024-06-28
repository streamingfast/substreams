The [USDT Exchanges Volume Subgraph](https://github.com/streamingfast/injective-subgraph-template) tracks the historical USDT volume for the `INJ-USDT` pair in the Dojo DEX.

{% hint style="success" %}
**Tip**: This tutorial teaches you how to build a Substreams from scratch.

Remember that you can auto-generate your Substreams module by usig the [code-generation tools](../../../getting-started/injective/injective-first-sps.md).
{% endhint %}

The subgraph uses the [Substreams triggers](../../../consume/subgraph/triggers.md) to import data from the Injective foundational modules.

## Before You Begin

- [Install the Substreams CLI](../../../common/installing-the-cli.md)
- [Get an authentication token](../../../common/authentication.md)
- [Learn about the basics of the Substreams](../../../common/manifest-modules.md)
- [Complete the Block Stats Substreams tutorial](./block-stats.md)
- [Complete the Foundational Modules tutorial](./foundational.md)

## Inspect the Project

The `subgraph.yaml` file defines the configuration of the data sources (i.e. where the subgraph should get the data from).

```yaml
specVersion: 1.0.0
indexerHints:
  prune: auto
schema:
  file: ./schema.graphql # 1.
dataSources:
  - kind: substreams
    name: Events
    network: injective-mainnet # 2.
    source:
      package:
        moduleName: wasm_events # 3.
        file: wasm-events-v0.1.0.spkg # 4.
    mapping: # 5.
      apiVersion: 0.0.7
      kind: substreams/graph-entities
      file: ./src/mapping.ts
      handler: handleEvents
```
1. Path to the GraphQL schema, which defines the entities of the subgraph.
2. Network where data should be indexed. In this case, `injective-mainnet`.
3. Substreams module imported in the subgraph. This module extracts all the events with `type = wasm`.
4. Substreams package (`.spkg`) that contains the `wasm_events` module.
5. Definition of mappings. The `handleEvents` function will receive the data from the `wasm_events` Substreams module to be processed by the subgraph.

## Inspect the Schema

The `schema.graphql` schema defines only one entity, `USDTExchangeVolume`, which holds the historical amount of the USDT exchanged in the Dojo DEX for the `INJ-USDT` pair.

```graphql
type USDTExchangeVolume @entity {
  id: ID!
  amount: String!
}
```

The `amount` field is updated every time that a new exchange happens in the DEX.

## Inspect the Code

The `handleEvents` function in the `mappings.ts` file receives the filtered events of the Substreams (those with `type = wasm`). The logic of the code finds out the USDT amount exchanged in the swap and updates the `USDTExchangeVolume` entity, adding up the amount.

```ts
export function handleEvents(bytes: Uint8Array): void { // 1.
    const eventList: EventList = Protobuf.decode<EventList>(bytes, EventList.decode); // 2.
    const events = eventList.events;

    log.info("Protobuf decoded, length: {}", [events.length.toString()]);

    let entity = USDTExchangeVolume.load(ID); // 3.
    if (entity == null) {
        log.info("Entity not found, creating one...", []);
        entity = new USDTExchangeVolume(ID);
        entity.amount = '0';
    }

    for (let i = 0; i < events.length; i++) { // 4.
        const event = events[i].event;
        if (event == null || event.type != "wasm") { // should be filtered by substreams
            continue;
        }

        let contract_addr = "";
        let action = "";
        let ask_asset = "";
        let ask_amount = "";
        let offer_asset = "";
        let offer_amount = "";

        for (let i = 0; i < event.attributes.length; ++i) { // 5.
            const attr = event.attributes[i];
            if (attr.key == '_contract_addr') {
                    contract_addr = attr.value;
            } else if (attr.key == '_action') {
                    action = attr.value;
            } else if (attr.key == 'ask_asset') {
                    ask_asset = attr.value;
            } else if (attr.key == 'ask_amount' || attr.key == 'return_amount') {
                    ask_amount = attr.value;
            } else if (attr.key == 'offer_asset') {
                    offer_asset = attr.value;
            } else if (attr.key == 'offer_amount') {
                    offer_amount = attr.value;
            }
        }
        if (contract_addr != DOJO_addr) { // 6.
            continue;
        }

        let exchangeAmountStr = "";

        if (ask_asset == USDT_addr && ask_amount != "") {
            exchangeAmountStr = ask_amount;
        } 
        if (offer_asset == USDT_addr && offer_amount != "") {
            exchangeAmountStr = ask_amount;
        }
        if (exchangeAmountStr == "") {
            continue;
        }

        const exchangeAmount = BigInt.fromString(exchangeAmountStr);
        const entityAmount = BigInt.fromString(entity.amount);
        const sumResult = entityAmount.plus(exchangeAmount); // 7.
        entity.amount = sumResult.toString();
        entity.save();
        log.debug("Entity saved: {}", [entity.amount]);
    }
}
```
1. Definition of the `handleEvents` function. As a parameter, it receives an array of bytes, representing the events consumed from the Substreams.
2. Decode the byte array into the `EventList` Protobuf object, which is the output of the Substreams.
3. Load the `USDTExchangeVolume` subgraph entity, which will store the historical volume.
If it is the first trade, then the entity will not exist, and it must be created.
4. Iterate over the events and verify that the event type is `wasm` (`type == wasm`). This should be already filtered by the Substreams, but it is also nice to re-check it.
5. Iterate over the attributes of every event, finding out the neccesary information (contract address, action, ask amount, offer amount...).
6. Verify that the contract where the event is executed corresponds to the `INJ-USDT` pair in the Dojo DEX.
7. Update the entity.

## Deploy to a Local Graph Node

You can test your Substreams-powered Subgraph by deploying to a local Graph Node set-up. Take a look at the the [Graph Node Local Development tutorial](../../graph-node/local-development.md), which provides information on how to spin up a local environment for Graph Node.

First, clone the [Substreams Development Environment GitHub respository](https://github.com/streamingfast/substreams-dev-environment) and move to the `graph-node` folder. Execute the `start.sh` command with the Injective information (make sure you have Docker running in your computer).

```bash
./start.sh injective-mainnet https://mainnet.injective.streamingfast.io:443
```

The previous command will spin up a local IPFS node, a local Postgres database and a local Graph Node instance. Now, you can create a new subgraph in the Graph Node:

```bash
graph create usdt-exchange-volume --node=http://localhost:8020
```

Then, you can deploy:

```bash
graph deploy --node http://localhost:8020/ --ipfs http://localhost:5001 usdt-exchange-volume
```

The subgraph will start indexing in the Graph Node, and you check out the different logs emitted by the subgraph.