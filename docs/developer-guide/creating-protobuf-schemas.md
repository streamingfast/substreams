---
description: StreamingFast Substreams protobuf schemas
---

# Protobuf Schemas

### Protobuf Overview

After creating the Substreams manifest a custom protobuf needs to be defined in the manifest file. The protobuf is used for defining input and output.

{% hint style="info" %}
**Note**_: Protocol Buffers (protobufs) are Google's language-neutral extensible mechanism for serializing structured data. Protobufs are similar to XML, but smaller, faster, and simpler. Find additional information regarding Protocol Buffers on the_ [_Google website_](https://developers.google.com/protocol-buffers)_._

__\
__**Google Protobuf Documentation**

[https://developers.google.com/protocol-buffers](https://developers.google.com/protocol-buffers)

####

#### Google Protobuf Tutorial

[https://developers.google.com/protocol-buffers/docs/tutorials](https://developers.google.com/protocol-buffers/docs/tutorials)
{% endhint %}

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

The ERC721 smart contract contains a Transfer event. The event is targeted by creating a matching protobuf.&#x20;

Protobufs are simple data types defined using the message keyword followed by the name of the data type of interest. As mentioned, the ERC721 contains a Transfer event that has been selected to pick out of the stream of blockchain data.&#x20;

A matching Transfer message including the data type’s fields is defined within the protobuf file. The protobuf file serves as the interface between the module handlers and the data being provided by Substreams.&#x20;

ERC721 smart contracts are generic contracts used across many different Ethereum applications.&#x20;

Transfer events in this example can be targeted for a specific smart contract stored in the Ethereum blockchain, such as Bored Ape Yacht Club.&#x20;

Multitudes of more specific data types exist in the smart contract ecosystem, some extending the ERC20 and ERC721 base implementations. Developers can create more refined and complex profobufs based on the many custom data types that exist.

{% hint style="success" %}
_Tip: using a fully qualified path for protobuf files reduces the risk of conflicts when other community members build their own_ [_Substreams Packages_](../reference-and-specs/packages.md#dependencies)_._
{% endhint %}

Next, generate the associated Rust code for the protobuf.

```bash
substreams protogen ./substreams.yaml --exclude-paths="sf/ethereum,sf/substreams,google"
```

The generated Rust code will be created as `src/pb/eth.erc721.v1.rs`

Next, add a `mod.rs` file in the `src/pb` directory to export the newly generated Rust code.

{% code title="src/pb/mod.rs" %}
```rust
#[path = "eth.erc721.v1.rs"]
#[allow(dead_code)]
pub mod erc721;
```
{% endcode %}

View this file in the repo by visiting the following link.

[https://github.com/streamingfast/substreams-template/blob/develop/src/pb/mod.rs](https://github.com/streamingfast/substreams-template/blob/develop/src/pb/mod.rs)
