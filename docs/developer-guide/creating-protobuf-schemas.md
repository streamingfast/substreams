# Creating Protobuf Schemas

Now that our manifest has been written, it is time to create your custom Protobuf definition, the one we'll use as an input/output in your manifest file.

Protocol Buffers are Google's language-neutral extensible mechanism for serializing structured data – think XML, but smaller, faster, and simpler. If you have not used Protobuf before, here are a couple of resources to get started:

* Protobuf - [https://developers.google.com/protocol-buffers](https://developers.google.com/protocol-buffers)
* Tutorials - [https://developers.google.com/protocol-buffers/docs/tutorials](https://developers.google.com/protocol-buffers/docs/tutorials)

We have defined a protobuf __ model as `proto:eth.erc721.v1.Transfers` which represents a list of ERC721 transfers.

Firstly, let's create the `proto` folder:

```bash
mkdir proto
cd proto
```

and in there, create our first protobuf definition file:

{% code title="erc721.proto" %}
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

Now that we have created our custom Protobuf definition file. we will generate the associated Rust code.

```bash
substreams protogen substreams.yaml --exclude-paths="sf/ethereum,sf/substreams,google"
```

You should now see your generated Rust code here `src/pb/eth.erc721.v1.rs`

lastly we need to add a Rust `mod.rs` file in the `src/pb` directory to export the newly generated Rust code

{% code title="src/pb/mod.rs" %}
```rust
#[path = "eth.erc721.v1.rs"]
#[allow(dead_code)]
pub mod erc721;
```
{% endcode %}
