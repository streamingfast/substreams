---
description: StreamingFast Substreams chains and endpoints
---

# Chains & Endpoints

Protobuf definitions and public endpoints are provided for each of the supported protocols and chains below.

### Ethereum

#### Protobuf definition

****[`sf.ethereum.type.v2.Block`](https://github.com/streamingfast/firehose-ethereum/blob/develop/proto/sf/ethereum/type/v2/type.proto)

#### Endpoints

Mainnet: mainnet.eth.streamingfast.io:443

Görli: goerli.eth.streamingfast.io:443

Polygon Mainnet: polygon.streamingfast.io:443

BNB: bsc.streamingfast.io:443

### Near

#### Protobuf definition ****&#x20;

****[`sf.near.type.v1.Block`](https://github.com/streamingfast/firehose-near/blob/develop/proto/sf/near/type/v1/type.proto)

#### Endpoints

Mainnet: mainnet.near.streamingfast.io:443

**T**estnet: testnet.near.streamingfast.io:443

### Solana

#### Protobuf definition

[`sf.solana.type.v1.Block`](https://github.com/streamingfast/firehose-solana/blob/develop/proto/sf/solana/type/v1/type.proto)

#### Endpoints

Mainnet: mainnet.sol.streamingfast.io:443

### Cosmos

#### Protobuf definition

[`sf.cosmos.type.v1.Block`](https://github.com/figment-networks/proto-cosmos/blob/main/sf/cosmos/type/v1/type.proto)

#### Endpoints

_None available at this time._

### Arweave

#### Protobuf definition

[`sf.arweave.type.v1.Block`](https://github.com/streamingfast/firehose-arweave/blob/develop/proto/sf/arweave/type/v1/type.proto)

#### Endpoints

Mainnet: mainnet.arweave.streamingfast.io:443

### Aptos

#### Protobuf definition

[`aptos.extractor.v1.Block`](https://github.com/aptos-labs/aptos-core/blob/main/crates/aptos-protos/proto/aptos/extractor/v1/extractor.proto)

#### Endpoints

Testnet: testnet.aptos.streamingfast.io:443

Also see the [Firehose _s_chemas documentation](https://firehose.streamingfast.io/references/protobuf-schemas) for what could be made available through Substreams.

{% hint style="info" %}
Each endpoint only serves Substreams that source from the corresponding _Block Message Name_ in its [`source:` field](manifests.md#modules-.inputs).

For example, it is not possible to stream an `sf.near.type.v1.Block` from an Ethereum endpoint.
{% endhint %}

_Note, endpoints need to be_ [_authenticated_](authentication.md)_, unless indicated otherwise._
