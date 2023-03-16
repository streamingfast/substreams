---
description: StreamingFast Substreams protobuf schemas
---

# Protobuf schemas

## Protobuf overview

Substreams uses Google Protocol Buffers extensively. Protocol Buffers, also referred to as protobufs, are used as the API for data models specific to the different blockchains. Manifests contain references to the protobufs for your Substreams module.

{% hint style="success" %}
**Tip**: Protobufs define the input and output for modules.
{% endhint %}

Learn more about the details of Google Protocol Buffers in the official documentation provided by Google.

**Google Protocol Buffer Documentation**

[Learn more about Google Protocol Buffers](https://developers.google.com/protocol-buffers) in the official documentation provided by Google.

**Google Protocol Buffer Tutorial**

[Explore examples and additional learning material](https://developers.google.com/protocol-buffers/docs/tutorials) for Google Protocol Buffers provided by Google.

### Protobuf definition for Substreams

Define a protobuf model as [`proto:eth.erc721.v1.Transfers`](https://github.com/streamingfast/substreams-template/blob/develop/proto/erc721.proto) representing a list of ERC721 transfers.

{% hint style="info" %}
**Note**: The `Transfers` protobuf in the Substreams Template example is located in the proto directory.
{% endhint %}

{% code title="eth/erc721/v1/erc721.proto" lineNumbers="true" %}
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

[View the `erc721.proto`](https://github.com/streamingfast/substreams-template/blob/develop/proto/erc721.proto) file in the official Substreams Template example repository.

#### Identifying data types

The ERC721 smart contract used in the Substreams Template example contains a `Transfer` event. You can use the event data through a custom protobuf.

The protobuf file serves as the interface between the module handlers and the data being provided by Substreams.

{% hint style="success" %}
**Tip**: Protobufs are platform-independent and are defined and used for various blockchains.

* The ERC721 smart contracts used in the Substreams Template example are generic contracts used across many different Ethereum applications.
* The size and scope of the Substreams module dictates the number of and complexity of protobufs.
{% endhint %}

The Substreams Template example extracts `Transfer` events from the [Bored Ape Yacht Club smart contract](https://etherscan.io/address/0xbc4ca0eda7647a8ab7c2061c2e118a18a936f13d) which is located on the Ethereum blockchain.

Several specific data types exist in the Ethereum smart contract ecosystem, some extending the ERC20 and ERC721 base modules. Complex protobufs are created and refined based on the various data types used across the different blockchains.

{% hint style="success" %}
**Tip**_:_ The use of fully qualified protobuf file paths reduces the risk of naming conflicts when other community members build their [Substreams packages](../reference-and-specs/packages.md#dependencies).
{% endhint %}

### Generating protobufs

The [`substreams` CLI](../reference-and-specs/command-line-interface.md) is used to generate the associated Rust code for the protobuf.

Notice the `protogen` command and Substreams manifest passed into the [`substreams` CLI](../reference-and-specs/command-line-interface.md).

{% code overflow="wrap" %}
```bash
substreams protogen ./substreams.yaml --exclude-paths="sf/ethereum,sf/substreams,google"
```
{% endcode %}

The pairing code is generated and saved into the [`src/pb/eth.erc721.v1.rs`](https://github.com/streamingfast/substreams-template/blob/develop/src/pb/eth.erc721.v1.rs)Rust file.

The [`mod.rs`](https://github.com/streamingfast/substreams-template/blob/develop/src/pb/mod.rs) file located in the `src/pb` directory of the Substreams Template example is responsible for exporting the freshly generated Rust code.

{% code title="src/pb/mod.rs" overflow="wrap" lineNumbers="true" %}
```rust
#[path = "eth.erc721.v1.rs"]
#[allow(dead_code)]
pub mod erc721;
```
{% endcode %}

View the [`mod.rs`](https://github.com/streamingfast/substreams-template/blob/develop/src/pb/mod.rs) file in the repository.

### Protobuf and Rust optional fields

Protocol buffers define fields' type by using standard primitive data types, such as integers, booleans, and floats or a complex data type such as `message`, `enum`, `oneof` or `map`. View the [full list](https://developers.google.com/protocol-buffers/docs/proto#scalar) of types in the [Google Protocol Buffers documentation](https://developers.google.com/protocol-buffers/docs/overview).

Any primitive data types in a message generate the corresponding Rust type,[`String`](https://doc.rust-lang.org/std/string/struct.String.html) for `string`, `u64` for `uint64,` and assign the default value of the corresponding Rust type if the field is not present in a message, an empty string for [`String`](https://doc.rust-lang.org/std/string/struct.String.html), 0 for integer types, `false` for `bool`.

Rust generates the corresponding `message` type wrapped by an [`Option`](https://doc.rust-lang.org/rust-by-example/std/option.html) enum type for fields referencing other complex `messages`. The [`None`](https://doc.rust-lang.org/std/option/) variant is used if the field is not present in the message.

The [`Option`](https://doc.rust-lang.org/rust-by-example/std/option.html) [`enum`](https://doc.rust-lang.org/book/ch06-01-defining-an-enum.html) is used to represent the presence through [`Some(x)`](https://doc.rust-lang.org/std/option/) or absence through [`None`](https://doc.rust-lang.org/std/option/) of a value in Rust. [`Option`](https://doc.rust-lang.org/rust-by-example/std/option.html) allows developers to distinguish between a field containing a value versus a field without an assigned a value.

{% hint style="info" %}
**Note**: The standard approach to represent nullable data in Rust is to wrap optional values in [`Option<T>`](https://doc.rust-lang.org/rust-by-example/std/option.html).
{% endhint %}

The Rust [`match`](https://doc.rust-lang.org/rust-by-example/flow\_control/match.html) keyword is used to compare the value of an [`Option`](https://doc.rust-lang.org/rust-by-example/std/option.html) to a [`Some`](https://doc.rust-lang.org/std/option/) or [`None`](https://doc.rust-lang.org/std/option/) variant. Handle a type wrapped [`Option`](https://doc.rust-lang.org/rust-by-example/std/option.html) in Rust by using:

```rust
match person.Location {
    Some(location) => { /* Value is present, do something */ }
    None => { /* Value is absent, do something */ }
}
```

If you are only interested in finding the presence of a value, use the [`if let`](https://doc.rust-lang.org/rust-by-example/flow\_control/if\_let.html) statement to handle the [`Some(x)`](https://doc.rust-lang.org/std/option/) arm of the [`match`](https://doc.rust-lang.org/rust-by-example/flow\_control/match.html) code.

```rust
if let Some(location) = person.location {
    // Value is present, do something
}
```

If a value is present, use the [`.unwrap()`](https://doc.rust-lang.org/rust-by-example/error/option\_unwrap.html) call on the [`Option`](https://doc.rust-lang.org/rust-by-example/std/option.html) to obtain the wrapped data. You'll need to account for these types of scenarios if you control the creation of the messages yourself or if the field is documented as always being present.

{% hint style="info" %}
**Note**: You need to be **absolutely sure** **the field is always defined**, otherwise Substreams panics and never completes, getting stuck on a block indefinitely.
{% endhint %}

_**PROST!**_ is a tool for generating Rust code from protobuf definitions. [Learn more about `prost`](https://github.com/tokio-rs/prost) in the project's official GitHub repository.

[Learn more about `Option`](https://doc.rust-lang.org/rust-by-example/std/option.html) in the official Rust documentation.
