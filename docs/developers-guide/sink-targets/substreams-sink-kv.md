---
description: StreamingFast Substreams Key/Value store sink
---

# Key/value store

## Purpose

This documentation will assist you in using [`substreams-sink-kv`](https://github.com/streamingfast/substreams-sink-kv) to write data from your existing substreams into a key-value store and serve it back through Connect-Web/GRPC.

## Overview

`substreams-sink-kv` works by reading the output of specially-designed substreams module (usually called `kv_out`) that produces data in a protobuf-encoded structure called `sf.substreams.sink.kv.v1.KVOperations`.

The data is written to a key-value store. Currently supported KV store are Badger, BigTable and TiKV.

A Connect-Web interface makes the data available directly from the `substreams-sink-kv` process. Alternatively, you can consume the data directly from your key-value store.

## Requirements

* An existing substreams (including `substreams.yaml` and Rust code) that you want to instrument for `substreams-sink-kv`.
* A key-value store where you want to send your data (a badger local file can be used for development)
* Knowledge about Substreams development (start [here](https://substreams.streamingfast.io/getting-started/quickstart))
* Rust installation and compiler

## Installation

* Install [substreams-sink-kv CLI](https://github.com/streamingfast/substreams-sink-kv/releases)
* Install [substreams CLI](https://substreams.streamingfast.io/getting-started/installing-the-cli)
* Install [grpcurl](https://github.com/fullstorydev/grpcurl/releases) to easily read the data back from the KV store

## Instrumenting your Substreams

### Assumptions

The following instructions will assume that you are instrumenting [substreams-eth-block-meta](https://github.com/streamingfast/substreams-eth-block-meta), which contains:

* A store `store_block_meta_end` defined like [this](https://github.com/streamingfast/substreams-eth-block-meta/blob/v0.4.0/substreams.yaml#L29-L34):

```yaml
# substreams.yaml
...
  - name: store_block_meta_end
    kind: store
    updatePolicy: set
    valueType: proto:eth.block_meta.v1.BlockMeta
    inputs:
      - source: sf.ethereum.type.v2.Block
```

* a `eth.block_meta.v1.BlockMeta` protobuf structure like [this](https://github.com/streamingfast/substreams-eth-block-meta/blob/v0.4.0/proto/block\_meta.proto#L7-L12):

```
message BlockMeta {
  uint64 number = 1;
  bytes hash = 2;
  bytes parent_hash = 3;
  google.protobuf.Timestamp timestamp = 4;
}
```

> **Note** The [substreams-eth-block-meta](https://github.com/streamingfast/substreams-eth-block-meta) is already instrumented for sink-kv, the proposed changes here are a simplified version of what has been implemented. Please adjust the proposed code to your own substreams.

### Import the Cargo module

1. Add the `substreams-sink-kv` crate to your `Cargo.toml`:

```toml
# Cargo.toml

[dependencies]
substreams-sink-kv = "0.1.1"
# ...

```

1. Add `map` module implementation function named `kv_out` to your `src/lib.rs`:

```yaml
# substreams.yaml
...
  - name: kv_out
    kind: map
    inputs:
      - store: store_block_meta_end
        mode: deltas
    output:
      type: proto:sf.substreams.sink.kv.v1.KVOperations
```

1. Add a `kv_out` public function to your `src/lib.rs`:

```
// src/lib.rs

#[path = "kv_out.rs"]
mod kv;
use substreams_sink_kv::pb::kv::KvOperations;

#[substreams::handlers::map]
pub fn kv_out(
    deltas: store::Deltas<DeltaProto<BlockMeta>>,
) -> Result<KvOperations, Error> {

    // Create an empty 'KvOperations' structure
    let mut kv_ops: KvOperations = Default::default();

    // Call a function that will push key-value operations from the deltas
    kv::process_deltas(&mut kv_ops, deltas);

    // Here, we could add more operations to the kv_ops
    // ...

    Ok(kv_ops)
}
```

1. Add the `kv::process_deltas` transformation function referenced in the last snippet:

```
// src/kv_out.rs

use substreams::proto;
use substreams::store::{self, DeltaProto};
use substreams_sink_kv::pb::kv::KvOperations;

use crate::pb::block_meta::BlockMeta;

pub fn process_deltas(ops: &mut KvOperations, deltas: store::Deltas<DeltaProto<BlockMeta>>) {
    use substreams::pb::substreams::store_delta::Operation;

    for delta in deltas.deltas {
        match delta.operation {
            // KV Operations do not distinguish between Create and Update.
            Operation::Create | Operation::Update => {
                let val = proto::encode(&delta.new_value).unwrap();
                ops.push_new(delta.key, val, delta.ordinal);
            }
            Operation::Delete => ops.push_delete(&delta.key, delta.ordinal),
            x => panic!("unsupported opeation {:?}", x),
        }
    }
}
```

## Test your substreams

1. Compile your changes in your rust code:

```
cargo build --release --target=wasm32-unknown-unknown
```

1. Run with `substreams` command directly:

```bash
substreams run -e mainnet.eth.streamingfast.io:443 substreams.yaml kv_out --start-block 1000000 --stop-block +1
```

> **Note** To connect to a public StreamingFast substreams endpoint, you will need an authentication token, follow this [guide](https://substreams.streamingfast.io/reference-and-specs/authentication) to obtain one.

1. Run with `substreams-sink-kv`:

```bash
substreams-sink-kv \
  run \
  "badger3://$(pwd)/badger_data.db" \
  mainnet.eth.streamingfast.io:443 \
  manifest.yaml \
  kv_out
```

You should see output similar to this one:

```bash
2023-01-12T10:08:31.803-0500 INFO (sink-kv) starting prometheus metrics server {"listen_addr": "localhost:9102"}
2023-01-12T10:08:31.803-0500 INFO (sink-kv) sink to kv {"dsn": "badger3:///Users/stepd/repos/substreams-sink-kv/badger_data.db", "endpoint": "mainnet.eth.streamingfast.io:443", "manifest_path": "https://github.com/streamingfast/substreams-eth-block-meta/releases/download/v0.4.0/substreams-eth-block-meta-v0.4.0.spkg", "output_module_name": "kv_out", "block_range": ""}
2023-01-12T10:08:31.803-0500 INFO (sink-kv) starting pprof server {"listen_addr": "localhost:6060"}
2023-01-12T10:08:31.826-0500 INFO (sink-kv) reading substreams manifest {"manifest_path": "https://github.com/streamingfast/substreams-eth-block-meta/releases/download/v0.4.0/substreams-eth-block-meta-v0.4.0.spkg"}
2023-01-12T10:08:32.186-0500 INFO (sink-kv) validating output store {"output_store": "kv_out"}
2023-01-12T10:08:32.186-0500 INFO (sink-kv) resolved block range {"start_block": 0, "stop_block": 0}
2023-01-12T10:08:32.186-0500 INFO (sink-kv) starting to listen on {"addr": "localhost:8000"}
2023-01-12T10:08:32.186-0500 INFO (sink-kv) starting stats service {"runs_each": "2s"}
2023-01-12T10:08:32.186-0500 INFO (sink-kv) no block data buffer provided. since undo steps are possible, using default buffer size {"size": 12}
2023-01-12T10:08:32.186-0500 INFO (sink-kv) starting stats service {"runs_each": "2s"}
2023-01-12T10:08:32.186-0500 INFO (sink-kv) ready, waiting for signal to quit
2023-01-12T10:08:32.186-0500 INFO (sink-kv) launching server {"listen_addr": "localhost:8000"}
2023-01-12T10:08:32.187-0500 INFO (sink-kv) serving plaintext {"listen_addr": "localhost:8000"}
2023-01-12T10:08:32.278-0500 INFO (sink-kv) session init {"trace_id": "a3c59bd7992c433402b70f9541565d2d"}
2023-01-12T10:08:34.186-0500 INFO (sink-kv) substreams sink stats {"db_flush_rate": "10.500 flush/s (21 total)", "data_msg_rate": "0.000 msg/s (0 total)", "progress_msg_rate": "0.000 msg/s (0 total)", "block_rate": "0.000 blocks/s (0 total)", "flushed_entries": 0, "last_block": "None"}
2023-01-12T10:08:34.186-0500 INFO (sink-kv) substreams sink stats {"progress_msg_rate": "16551.500 msg/s (33103 total)", "block_rate": "10941.500 blocks/s (21883 total)", "last_block": "#291883 (66d03f819dde948b297c8d582889246d7ba11a5b947335497f8716a7b608f78e)"}
```

> **Note** This writes the data to a local folder "./badger\_data.db/" in Badger format. You can `rm -rf ./badger_data.db` between your tests to cleanup all existing data.

1. Look at the stored data

You can scan the whole dataset using the 'Scan' command:

```bash
grpcurl --plaintext -d '{"begin": "", "limit":100}' localhost:8000 sf.substreams.sink.kv.v1.Kv/Scan
```

You can look at data by key prefix:

```bash
grpcurl --plaintext   -d '{"prefix": "day:first:201511", "limit":31}' localhost:8000 sf.substreams.sink.kv.v1.Kv/GetByPrefix
```

## Consume the key-value data from a web-page using Connect-Web

The [Connect-Web](https://connect.build/docs/web/getting-started) library allows you to quickly bootstrap a web-based client for your key-value store.

### Requirements

* [NodeJS](https://nodejs.dev/download)
* [npm](https://docs.npmjs.com/downloading-and-installing-node-js-and-npm)
* [buf CLI](https://docs.buf.build/installation)

### Start from our example for `substreams-eth-block-meta`

You can checkout and run our connect-web-example like this:

```bash
git clone git@github.com:streamingfast/substreams-sink-kv
cd substreams-sink-kv/connect-web-example
npm install
npm run dev
```

Then, enter a key in the text box. The app currently only decodes `eth.block_meta.v1.BlockMeta`, so you will likely receive the corresponding value encoded in hex string.

To decode the value of your own data structures, add your `.proto` files under `proto/` and generate Rust bindings like this:

```bash
npm run buf:generate
```

You should see this output:

```
> connect-web-example@0.0.0 buf:generate
> buf generate ../proto/substreams/sink/kv/v1 && buf generate ./proto
```

Then, modify the code from `src/App.tsx` to decode your custom type, from this:

```rust
    import { BlockMeta } from "../gen/block_meta_pb";

    ...

    const blkmeta = BlockMeta.fromBinary(response.value);
    output = JSON.stringify(blkmeta, (key, value) => {
        if (key === "hash") {
            return "0x" + bufferToHex(blkmeta.hash);
        }
        if (key === "parentHash") {
            return "0x" + bufferToHex(blkmeta.parentHash);
        }
        return value;
    }, 2);
```

to this:

```rust
    import { MyData } from "../gen/my_data_pb";

    ...

    const decoded = MyData.fromBinary(response.value);
    output = JSON.stringify(decoded, null, 2);
```

### Bootstrap your own application

If you want to start with an empty application, you can follow [these instructions](https://github.com/streamingfast/substreams-sink-kv/tree/main/connect-web-example/README.md)

## Sending to a production key-value store

Until now, we've used the **badger** database as a store, for simplicity. However, `substreams-sink-kv` also supports **TiKV** and **bigtable**.

* `tikv://pd0,pd1,pd2:2379?prefix=namespace_prefix`
* `bigkv://project.instance/namespace-prefix?createTables=true`

See [kvdb](https://github.com/streamingfast/kvdb) for more details.

## Conclusion and review

The ability to route data extracted from the blockchain by using Substreams is powerful and useful. Key-value stores aren't the only type of sink the data extracted by Substreams can be piped into. Review the core Substreams sinks documentation for [additional information on other types of sinks](https://substreams.streamingfast.io/developers-guide/substreams-sinks) and sinking strategies.
