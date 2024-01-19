## Mapping Blocks

This module takes a raw Ethereum block and returns a reduced version of a block, with just three pieces of information: hash, parent hash, and block number.

Let's run the Substreams first, and then go through the code.

### Running the Substreams

Running a Substreams usually requires three steps: generating the Rust Protobufs, building the WASM container, and using the Substream CLI to start the streaming. Make sure to run the following commands in the `substreams-explorer/ethereum-explorer` folder:

1. **Generate the Protobuf objects:** The `.proto` files define a data model regardless of any programming language. However, in order to use this model in your Rust application, you must generate the corresponding Rust data structures.

```bash
make protogen
```

2. **Build the WASM module:** The following command generates a WASM container from the Rust application, which you can find at `/target/wasm32-unknown-unknown/release/substreams.wasm`. Note that this is the same path provided in the Substreams manifest (`substreams.yml`).

```bash
make build
```

3. **Streaming data through the CLI:** The following command streams the Ethereum blockchain data, and applies the transformations contained in the `map_block_meta` module to every block.

```bash
$ substreams run -e mainnet.eth.streamingfast.io:443 substreams.yaml map_block_meta --start-block 17712040 --stop-block +1
```

Let's break down the command into pieces:

- `mainnet.eth.streamingfast.io:443`: is the StreamingFast Ethereum Mainnet endpoint where you are sending your Substreams for execution.
- `substreams.yaml`: specifies the Substreams manifest.
- `map_block_meta`: specifies the module to execute. Since the Ethereum Explorer application contains several modules, it is necessary to specify which one you want to execute.
- `--start-block 17712040`: specifies the starting block (i.e. the block where Substreams will start streaming).
- `--stop-block +1`: specifies how many blocks after the starting block should be considered. In this example, `+1` means that the streaming will start at `17712040` and finish at `17712041` (just one block).

The output of the command should be similar to:

```bash
...output omitted...

----------- BLOCK #17,712,040 (31ad07fed936990d3c75314589b15cbdec91e4cc53a984a43de622b314c38d0b) ---------------
{
  "@module": "map_block_meta",
  "@block": 17712040,
  "@type": "eth.block_meta.v1.BlockMeta",
  "@data": {
    "number": "17712040",
    "hash": "31ad07fed936990d3c75314589b15cbdec91e4cc53a984a43de622b314c38d0b",
    "parentHash": "1385f853d28b16ad7ebc5d51b6f2ef6d43df4b57bd4c6fe4ef8ccb6f266d8b91"
  }
}

all done
```

As you can see, the output is formatted as JSON, and the `@data` field contains the actual output Protobuf of the module (`BlockMeta`).

The `BlockMeta` definition:

```protobuf
syntax = "proto3";

package eth.block_meta.v1;

message BlockMeta {
  uint64 number = 1;
  string hash = 2;
  string parent_hash = 3;
}
```

The JSON output:

```json
"@data": {
    "number": "17712040",
    "hash": "31ad07fed936990d3c75314589b15cbdec91e4cc53a984a43de622b314c38d0b",
    "parentHash": "1385f853d28b16ad7ebc5d51b6f2ef6d43df4b57bd4c6fe4ef8ccb6f266d8b91"
}
```

### Inspecting the Code

Although the code (which is in the `map_block_meta.rs` file) for this module is pretty straightforward to understand, let's discuss its main parts.

Declaration of the module in the manifest (`substreams.yml`):

```yaml
modules:
  - name: map_block_meta
    kind: map
    inputs:
      - source: sf.ethereum.type.v2.Block
    output:
      type: proto:eth.block_meta.v1.BlockMeta
```

Code of the module:

```rust
#[substreams::handlers::map]
fn map_block_meta(blk: Block) -> Result<BlockMeta, substreams::errors::Error> {
    let header = blk.header.as_ref().unwrap();

    Ok(BlockMeta {
        number: blk.number,
        hash: Hex::encode(&blk.hash),
        parent_hash: Hex::encode(&header.parent_hash),
    })
}
```

The `#[substreams::handlers::map]` attribute annotates the `map_block_meta` function as a Substreams mapper. The name of the function must match the name of the module in the Substreams manifest. The input of the function is a raw Ethereum block (`pb::eth::v2::Block`).

In order to get the block metadata, you use the `header` property.

```rust
let header = blk.header.as_ref().unwrap();
```

Then, you simply create a `BlockMeta` struct and return it.

```rust
Ok(BlockMeta {
    number: blk.number,
    hash: Hex::encode(&blk.hash),
    parent_hash: Hex::encode(&header.parent_hash),
})
```
