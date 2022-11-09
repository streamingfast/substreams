---
description: StreamingFast Substreams protobuf schemas
---

# Protobuf Schemas

### Protobuf Overview

Substreams uses Protocol Buffers (protobufs) as the API for data models specific to each blockchain. Each manifest defines references to the protobufs for the Substreams implementation.&#x20;

{% hint style="success" %}
**Tip**: Protobufs define the input and output for modules.
{% endhint %}

### Protobuf Basics

Protobufs are Google's language-neutral extensible mechanism for serializing structured data. Protobufs are similar to XML but smaller, faster, and simpler.&#x20;

Additional information can be found for Protocol Buffers by visiting the links provided below.&#x20;

**Google Protocol Buffer Documentation**

[https://developers.google.com/protocol-buffers](https://developers.google.com/protocol-buffers)

**Google Protocol Buffer Tutorial**

[https://developers.google.com/protocol-buffers/docs/tutorials](https://developers.google.com/protocol-buffers/docs/tutorials)

### Protobuf Definition

Define a protobuf model as `proto:eth.erc721.v1.Transfers` representing a list of ERC721 transfers.

Create a proto directory in the substreams directory and then create the protobuf definition file.

{% code title="eth/erc721/v1/erc721.proto" %}
```protobuf
syntax = "proto3";

package eth.erc721.v1;

message Transfers {
  repeated Transfer transfers = 1;
}

message Transfer {
  bytes from = 1;
  bytes to = 2;
  uint64 token_id = 3;
  bytes trx_hash = 4;
  uint64 ordinal = 5;
}
```
{% endcode %}

View this file in the repo by visiting the following link.

[https://github.com/streamingfast/substreams-template/blob/develop/proto/erc721.proto](https://github.com/streamingfast/substreams-template/blob/develop/proto/erc721.proto)

#### Identifying Data types

The ERC721 smart contract associated with the Substreams Template example contains a Transfer event. The event is targeted by creating a matching protobuf.&#x20;

A matching Transfer message including the data type’s fields is defined within the protobuf file. The protobuf file serves as the interface between the module handlers and the data being provided by Substreams.&#x20;

{% hint style="info" %}
**Note**: ERC721 smart contracts are generic contracts used across many different Ethereum applications.&#x20;
{% endhint %}

{% hint style="success" %}
**Tip**: Transfer events in this example can be targeted for specific smart contracts, such as Bored Ape Yacht Club.&#x20;
{% endhint %}

Multitudes of more specific data types exist in the Ethereum smart contract ecosystem, some extending the ERC20 and ERC721 base implementations. Developers can create more refined and complex protobufs based on the many custom data types that exist.

{% hint style="success" %}
**Tip**_:_ Using fully qualified paths for protobuf files reduces the risk of naming conflicts when other community members build their [Substreams packages](../reference-and-specs/packages.md#dependencies).
{% endhint %}

### Generating Protobufs

The Substreams CLI is used to generate the associated Rust code for the protobuf.

{% hint style="success" %}
**Tip**: Notice the `protogen` command and Substreams manifest passed into the CLI.
{% endhint %}

```bash
substreams protogen ./substreams.yaml --exclude-paths="sf/ethereum,sf/substreams,google"
```

The generated Rust code is generated and saved into `src/pb/eth.erc721.v1.rs`

Adding a `mod.rs` file in the `src/pb` directory will export the newly generated Rust code.

{% code title="src/pb/mod.rs" %}
```rust
#[path = "eth.erc721.v1.rs"]
#[allow(dead_code)]
pub mod erc721;
```
{% endcode %}

View this file in the repo by visiting the following link.

[https://github.com/streamingfast/substreams-template/blob/develop/src/pb/mod.rs](https://github.com/streamingfast/substreams-template/blob/develop/src/pb/mod.rs)
