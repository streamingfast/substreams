---
description: StreamingFast Substreams sink files
---

# Files

### Purpose

This documentation exists to assist you in understanding and beginning to use the StreamingFast [`substreams-sink-file`](https://github.com/streamingfast/substreams-sink-files)`s` tool. The Substreams module paired with this tutorial is a basic example of the elements required for sinking blockchain data into files-based storage solutions.

### Overview

The `substreams-sink-files` tool provides the ability to pipe data extracted from a blockchain to various types of files-based persistence solutions.

For example, you could extract all of the ERC20, ERC721, and ERC1155 transfers from the Ethereum blockchain and persist the data to a files-based store.

Substreams modules are created and prepared specifically for the sink tool. After the sink tool begins running, automated tasks can be set up to have [BigQuery](https://cloud.google.com/bigquery), [Clickhouse](https://clickhouse.com), custom scripts, or other files-based storage solutions, ingest the data. This can only be accomplished indirectly. It's possible to automate further ingestion from files to data stores.

You could use `substreams-sink-files` to sink data in `JSONL` format to a [Google Cloud Storage (GCS)](https://cloud.google.com/storage) bucket and configure a BigQuery Transfer job to run every 15 minutes. The scheduled job ingests the new files found in the GCS bucket where the data, extracted by the Substreams, was written.

### Accompanying code example

The accompanying Substreams module associated with this documentation is responsible for extracting a handful of data fields from the Block object injected into the Rust-based map module. The sink tool processes the extracted blockchain data line-by-line and outputs the data to the files-based persistence mechanism you've chosen.

The accompanying code example extracts four data points from the Block object and packs them into the `substreams.sink.files.v1` protobuf's data model. The data is passed to the protobuf as a single line of plain text.

Binary formats such as [Avro](https://avro.apache.org/) or [Parquet](https://parquet.apache.org/) is possible, however, support is not available. Contributions are welcome to help with support of binary data formats. [Contact the StreamingFast team on Discord](https://discord.gg/mYPcRAzeVN) to learn more and discuss specifics.

## Installation

### Install `substreams-sink-files`

Install `substreams-sink-files` by using the pre-built binary release [available in the official GitHub repository](https://github.com/streamingfast/substreams-sink-files/releases).

Extract `substreams-sink-files` into a folder and ensure this folder is referenced globally via your `PATH` environment variable.

### Accompanying code example

The accompanying code example for this tutorial is available in the `substreams-sink-files` respository. The Substreams project for the tutorial is located in the `docs/tutorial/` directory.

Run the included `make codegen` command to create the required protobuf files.

```bash
make codegen
```

It's a good idea to run the example code using your installation of the `substreams` CLI to make sure everything is set up and working properly.

Verify the setup for the example project by using the `make build` and `substreams run` commands.

Build the Substreams module by using the included `make` command.

```bash
make
```

Run the project by using the `substreams run` command.

```bash
substreams run -e mainnet.eth.streamingfast.io:443 substreams.yaml jsonl_out --start-block 1000000 --stop-block +1
```

The `substreams run` command will result in output resembling the following:

```bash
----------- NEW BLOCK #1,000,000 (1000000) ---------------
{
  "@module": "jsonl_out",
  "@block": 1000000,
  "@type": "sf.substreams.sink.files.v1",
  "@data": {
    "lines": [
      "{\"hash\":\"8e38b4dbf6b11fcc3b9dee84fb7986e29ca0a02cecd8977c161ff7333329681e\",\"number\":1000000,\"parent_hash\":\"b4fbadf8ea452b139718e2700dc1135cfc81145031c84b7ab27cd710394f7b38\",\"timestamp\":\"2016-02-13T22:54:13Z\"}"
    ]
  }
}
```

## Substreams modifications

### Module handler changes for sink

The example code in the [`lib.rs`](https://github.com/streamingfast/substreams-sink-files/blob/master/docs/tutorial/src/lib.rs) Rust source code file contains the `jsonl_out` module handler responsible for extracting the blockchain data. The module handler is responsible for passing the data to the `sf.substreams.sink.files.v1` protobuf for the sink tool and its processes.

```rust
#[substreams::handlers::map]
fn jsonl_out(block: eth::Block) -> Result<Lines, substreams::errors::Error> {

    let header = block.header.as_ref().unwrap();

    Ok(pb::sinkfiles::Lines {
        lines: vec![json!({
            "number": block.number,
            "hash": Hex(&block.hash).to_string(),
            "parent_hash": Hex(&header.parent_hash).to_string(),
            "timestamp": header.timestamp.as_ref().unwrap().to_string()
        })
        .to_string()],
    })
}
```

This module handler uses `JSONL` for the output type, any other plain-text line-based format can be supported, `CSV` for example. The [`json!`](https://docs.rs/serde\_json/latest/serde\_json/macro.json.html) macro is used to write the block data to the Rust `Vec` type by using the Rust [`vec!`](https://doc.rust-lang.org/std/macro.vec.html) macro.

The example code is intentionally very basic. StreamingFast [provides a more robust and full example](https://github.com/streamingfast/substreams-eth-token-transfers/blob/develop/src/lib.rs#L24) demonstrating how to extract data related to transfers from Ethereum. A crucial aspect of working with Substreams and sinks is a significant amount of data can be extracted from a Block object. The data is extracted and packed into a row. The row is represented by the JSONL or CSV based protobuf you're responsible for designing for your sink.

The output type for sink is a list of lines. The line content can be any type anything that is formatted as plain text, and line based. For example, a basic string like the transaction's hash, would result in files containing all the hashes for the transactions, one per line.

### Core steps for Substreams sink modules

* Import sink `.spkg` files, re-generate protobufs and create and add a mod.rs file.
* Create a map module outputting sf.substreams.sink.files.v1 format. This module extracts the entity to be written, one per block from the block or another module's dependencies. Each line will be in JSON format. You can use the json! macro from the [`serde_json`](https://docs.rs/serde\_json/latest/serde\_json) crate to assist creating your structure, one per line.
* Add the correct module definition to the Substreams manifest `substreams.yaml`.

```yaml
imports:
  sink_files: https://github.com/streamingfast/substreams-sink-files/releases/download/v0.2.0/substreams-sink-files-v0.2.0.spkg

binaries:
  default:
    type: wasm/rust-v1
    file: target/wasm32-unknown-unknown/release/substreams.wasm

modules:
  - name: jsonl_out
    kind: map
    inputs:
      - source: sf.ethereum.type.v2.Block
    output:
      type: proto:sf.substreams.sink.files.v1.Lines
```

## Understanding the sink tool

### Run and configure the `substreams-sink-files` tool

The command to start and run the `substreams-sink-files` tool for the accompanying Substreams project will resemble:

{% code overflow="wrap" %}
```bash
substreams-sink-files run --encoder=lines --state-store=./output/state.yaml mainnet.eth.streamingfast.io:443 substreams.yaml jsonl_out ./output/files
```
{% endcode %}

## Verify output from tool

Running the sink tool results in logging information printed to the terminal and directories and files being written to the local system or a cloud provider bucket if configured.

The sink tool will produce output in the terminal resembling the following for a properly configured and working environment and project.

```bash
2023-01-09T07:45:02.563-0800 INFO (substreams-sink-files) starting prometheus metrics server {"listen_addr": "localhost:9102"}
2023-01-09T07:45:02.563-0800 INFO (substreams-sink-files) sink to files {"file_output_path": "./localdata/out", "file_working_dir": "./localdata/working", "endpoint": "mainnet.eth.streamingfast.io:443", "encoder": "lines", "manifest_path": "substreams.yaml", "output_module_name": "jsonl_out", "block_range": "", "state_store": "./localdata/working/state.yaml", "blocks_per_file": 10000, "buffer_max_size": 67108864}
2023-01-09T07:45:02.563-0800 INFO (substreams-sink-files) reading substreams manifest {"manifest_path": "substreams.yaml"}
2023-01-09T07:45:02.563-0800 INFO (substreams-sink-files) starting pprof server {"listen_addr": "localhost:6060"}
2023-01-09T07:45:04.041-0800 INFO (pipeline) computed start block {"module_name": "jsonl_out", "start_block": 0}
2023-01-09T07:45:04.042-0800 INFO (substreams-sink-files) ready, waiting for signal to quit
2023-01-09T07:45:04.045-0800 INFO (substreams-sink-files) setting up sink {"block_range": {"start_block": 0, "end_block": "None"}, "cursor": {"Cursor":"","Block":{}}}
2023-01-09T07:45:04.048-0800 INFO (substreams-sink-files) starting new file boundary {"boundary": "[0, 10000)"}
2023-01-09T07:45:04.049-0800 INFO (substreams-sink-files) boundary started {"boundary": "[0, 10000)"}
2023-01-09T07:45:04.049-0800 INFO (substreams-sink-files) starting stats service {"runs_each": "2s"}
2023-01-09T07:45:06.052-0800 INFO (substreams-sink-files) substreams sink stats {"progress_msg_rate": "0.000 msg/s (0 total)", "block_rate": "650.000 blocks/s (1300 total)", "last_block": "#1299 (a0f0f283e0d297dd4bcf4bbff916b1df139d08336ad970e77f26b45f9a521802)"}
```

One bundle of data is created for every 10K blocks during the sink process.

To view the files the `substreams-sink-files` tool generates navigate into the directory you used for the output path. The directory referenced in the example points to the `localdata/out` directory. List the files in the output directory using the standard `ls` command to reveal the files created by the `substreams-sink-files` tool.

```bash
...
0000000000-0000010000.jsonl	0000090000-0000100000.jsonl	0000180000-0000190000.jsonl
0000010000-0000020000.jsonl	0000100000-0000110000.jsonl	0000190000-0000200000.jsonl
0000020000-0000030000.jsonl	0000110000-0000120000.jsonl	0000200000-0000210000.jsonl
0000030000-0000040000.jsonl	0000120000-0000130000.jsonl	0000210000-0000220000.jsonl
0000040000-0000050000.jsonl	0000130000-0000140000.jsonl	0000220000-0000230000.jsonl
0000050000-0000060000.jsonl	0000140000-0000150000.jsonl	0000230000-0000240000.jsonl
0000060000-0000070000.jsonl	0000150000-0000160000.jsonl	0000240000-0000250000.jsonl
0000070000-0000080000.jsonl	0000160000-0000170000.jsonl	0000250000-0000260000.jsonl
0000080000-0000090000.jsonl	0000170000-0000180000.jsonl
...
```

The block range spanned by the example is from block 0000000000 to block 0000260000. The blocks contain all the lines received for the full 10K of processed blocks by default. The block range is controlled by using the `--file-block-count` flag.

### Cursors

When you use Substreams, it sends back a block to a consumer using an opaque cursor. This cursor points to the exact location within the blockchain where the block is. In case your connection terminates or the process restarts, upon re-connection, Substreams sends back the cursor of the last written bundle in the request so that the stream of data can be resumed exactly where it left off and data integrity is maintained.

You will find that the cursor is saved in a file on disk. The location of this file is specified by the flag `--state-store` which points to a local folder. It is important that you ensure that this file is properly saved to a persistent location. If the file is lost, the `substreams-sink-files` tool will restart from the beginning of the chain, redoing all the previous processing.

Therefore, It is crucial that this file is properly persisted and follows your deployment of `substreams-sink-files` to avoid any data loss.

### Cloud-based storage

You can use the `substreams-sink-files` tool to route data to files on your local file system and cloud-based storage solutions. To use a cloud-based solution such as Google Cloud Storage bucket, S3 compatible bucket, or Azure bucket, you need to make sure it is set up properly. Then, instead of referencing a local file in the `substreams-sink-files run` command, use the path to the bucket. The paths resemble `gs://<bucket>/<path>`, `s3://<bucket>/<path>`, and `az://<bucket>/<path>` respectively. Be sure to update the values according to your account and provider.

### Limitations

When you use the `substreams-sink-files` tool, you will find that it syncs up to the most recent "final" block of the chain. This means it is not real-time. Additionally, the tool writes bundles to disk when it has seen 10,000 blocks. As a result, the latency of the last available bundle can be delayed by around 10,000 blocks.

## Conclusion and review

The ability to route data extracted from the blockchain by using Substreams is powerful and useful. Files aren't the only type of sink the data extracted by Substreams can be piped into. Review the core Substreams sinks documentation for [additional information on other types of sinks](./) and sinking strategies.

To use `substreams-sink-files` you need to clone the official repository, install the tooling, generate the required files from the substreams CLI for the example Substreams module and run the sink tool.

You have to ensure the sinking strategy has been defined, the appropriate file types have been targeted, and accounted for, and the module handler code in your Substreams module has been properly updated. You need to start the `substreams-sink-files` tool and use the `run` command being sure to provide all of the required values for the various flags.
