---
description: StreamingFast Substreams sink files
---

# `substreams-sink-files` introduction

### Overview

The `substreams-sink-files` tool provides the ability to pipe data extracted from a blockchain to various types of files-based persistence solutions.

For example, you could extract all of the ERC20, ERC721, and ERC1155 transfers from the Ethereum blockchain and persist the data to a files-based store.

Substreams modules are created and prepared specifically for the sink tool. After the sink tool begins running, automated tasks can be setup to have [BigQuery](https://cloud.google.com/bigquery), or other files-based storage solutions, ingest the data.

By using the automated ingestion tasks you can also route the data to [Clickhouse](https://clickhouse.com), custom scripts and other related data storage and processing tools accepting a file format. This can only be accomplished indirectly. It's possible to automate further ingestion from files to data stores.

You could use `substreams-sink-files` to sink data in `JSONL` format to a [Google Cloud Storage (GCS)](https://cloud.google.com/storage) bucket and configure a BigQuery Transfer job to run every 15 minutes. The scheduled job ingests the new files found in the GCS bucket where the data, extracted by the Substreams, was written.

### Accompanying code example

The accompanying Substreams module associated with this documentation is responsible for extracting a handful of data fields from the Block object injected into the Rust-based map module. The sink tool processes the extracted blockchain data line-by-line and outputs the data to the files-based persistence mechanism you've chosen.

The accompanying code example extracts four data points from the Block object and packs them into the `substreams.sink.files.v1` protobuf's data model. The data is passed to the protobuf as a single line of plain text.

Binary formats such as [Avro](https://avro.apache.org/) or [Parquet](https://parquet.apache.org/) is possible, however, support is not available. Contributions are welcome to help with support of binary data formats. [Contact the StreamingFast team on Discord](https://discord.gg/mYPcRAzeVN) to learn more and discuss specifics.

## Outline

1. Download code and tools
2. Run code and verify output
3. Substreams modifications
4. Understanding the sink tool
5. Run tool and verify output
6. Conclusion and review

## Download code and tools

### Install `substreams-sink-files`

Clone the `substreams-sink-files` repository to obtain the files required to work with the sink.

```bash
git clone https://github.com/streamingfast/substreams-sink-files.git substreams-sink-files-tutorial
```

**<i>TODO: I'm still not clear on how we want them to install substreams-sink-files. Please advise.</i>**

Checking the version of `substreams-sink-files` will produce a message similar to:

```bash
substreams-sink-files version v0.2.0
```

Add the following lines to the computer's `~/.bashrc` configuration file and then restart the shell session to use the `substreams-sink-tool` from anywhere on your system.

```bash
export GOPATH=$HOME/go
```

### Accompanying code example

The accompanying code example for this tutorial is available in the `substreams-sink-tool` respository. The Substreams project for the tutorial is located in the `docs/tutorial/` directory.

**<i>TODO: Explanation of what is required from Substreams module perspective. We need to talk here about the Protobuf gen required to pull sf.substreams.sink.files.v1 model and modifications required, e.g. outputting a JSON model for each entity, one entity per line.</i>**

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

The example code in the [`lib.rs`](#) Rust source code file contains the `jsonl_out` module handler responsible for extracting the blockchain data. The module handler is responsible for passing the data to the `sf.substreams.sink.files.v1` protobuf for the sink tool and its processes.

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

The module handler uses `JSONL` for the output type, `CSV` is also supported.. The [`json!`](https://docs.rs/serde_json/latest/serde_json/macro.json.html) macro is used to write the block data to the Rust `Vec` type by using the Rust [`vec!`](https://doc.rust-lang.org/std/macro.vec.html) macro.

**TODO**: <i>We just need to emphasis in the tutorial that it's an example and that a lot of "rows" can be extracted per block. Giving the ERC20/ERC721/ERC1155 examples is a good idea as we coded it: https://github.com/streamingfast/substreams-eth-token-transfers/blob/develop/src/lib.rs#L24</i>

**TODO:** <i>How to respect Sink's expected output's type with examples for JSON (maybe CSV))</i>

## Understanding the sink tool

### Run and configure the `substreams-sink-files` tool

The command to start and run the `substreams-sink-files` tool for the accompanying Substreams project will resemble:

{% code overflow="wrap" %}

```bash
substreams-sink-files run --encoder=lines --state-store=./localdata/working/state.yaml mainnet.eth.streamingfast.io:443 substreams.yaml jsonl_out ./localdata/out
```

{% endcode %}

Flags

- `file_output_path`
  The path to a directory the sink will write its files to during processing.
- `file_working_dir`
  **TODO:** <i>Description here.</i>
- `endpoint`
  The URI of the firehose service Substreams is connecting to.
- `encoder`
  **TODO:** <i>Description here.</i>
- `manifest_path`
  The path to the Substreams manifest for the Substreams module prepared for the sink.
- `output_module_name`
  The name of the Rust-based Substreams module the sink will run during its processing.
- `block_range`
  **TODO:** <i>Description here.</i>
- `state_store`
  **TODO:** <i>Description here.</i>
- `blocks_per_file`
  The number of Block objects from the blockchain to process while the sink is processing.
- `buffer_max_size`
  **TODO:** <i>Description here.</i>

**TODO:**
<i>

- output (how to show the directories and files that get produced)
- inspect (need input for this, not sure what it is)
  </i>

## Verify output from tool

Running the sink tool results in visual output printed to the terminal and directories and files being written to the local system or a cloud provider bucket if configured.

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

## Conclusion and review

The ability to route data extracted from the blockchain by using Substreams is powerful and useful. Files aren't the only type of sink the data extracted by Substreams can be piped into. Review the core Substreams sinks documentation for [additional information on other types of sinks](https://substreams.streamingfast.io/developers-guide/substreams-sinks) and sinking strategies.

### Recap

To use `substreams-sink-files` you need to clone the official repository, install the tooling, generate the required files from the substreams CLI for the example Substreams module and run the sink tool.

The `substreams-sink-files` is available in its official GitHub repository.

Use the tool through the `run` command passing all of the required flags including `file_output_path`, `file_working_dir`, `endpoint`, `encoder`, `manifest_path`, `output_module_name`, `block_range`, `state_store`, `blocks_per_file`, and `buffer_max_size`.

You have to ensure the sinking strategy has been defined, the appropriate file types have been targeted, and accounted for, and the module handler code in your Substreams module has been properly updated.

---

**-- DEV NOTES --**

**TODO**: There need to have some content about how the limitation of this sink which write bundles only when last block of a bundle is final.

**TODO**: Add discussion about where Substreams cursor is saved and importance of persisting this state (save as a .yaml file)

**TODO**: Add discussion about s3, gcs, etc. vs. local files

A local folder
A Google Cloud Storage Bucket (gs://<bucket>/<path>)
An S3 compatible Bucket (s3://<bucket>/<path>)
An Azure bucket (az://<bucket>/<path>)
Configuration details for those could be seen at https://github.com/streamingfast/dstore#features