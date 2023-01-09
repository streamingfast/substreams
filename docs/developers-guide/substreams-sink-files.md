---
description: StreamingFast Substreams sink files
---

# `substreams-sink-files` introduction

### Overview

The `substreams-sink-files` tool provides the ability to pipe data extracted from a blockchain to various types of files-based persistence solutions. A few available options include [BigQuery](https://cloud.google.com/bigquery), [Clickhouse](https://clickhouse.com), custom scripts and other related data storage and processing tools accepting a file format. For example, you could extract all of the ERC20, ERC721, and ERC1155 transfers from the Ethereum blockchain and persist the data to a files-based store.

Substreams modules are created and prepared for the sink tool. After the sink tool begins running, automated ingestion tasks can be setup to have BigQuery, or other files-based storage solution, ingest the data.

As an example, you could use `substreams-sink-files` to sink data in `jsonl` format to a [Google Cloud Storage (GCS)](https://cloud.google.com/storage) bucket and configure a BigQuery Transfer job to run every 15 minutes. The scheduled job ingests the new files found in the GCS bucket where the data was written.

### Accompanying code example

[The accompanying Substreams module](#) associated with this documentation is responsible for extracting a handful of data fields from the Block object injected into the Rust-based map module. The sink tool processes the extracted blockchain data line-by-line and outputs the data to the files-based persistence mechanism you've chosen.

The accompanying code example extracts four data points from the Block object and packs them into the substreams.sink.files.v1 protobuf's data model. The data is passed to the protobuf as a single line of plain text, a `CSV` entry, or a `JSONL` element. StreamingFast is working on support for binary formats such as [Avro](https://avro.apache.org/), [Parquet](https://parquet.apache.org/), and others.

## Outline

1. Download code and tools
2. Run code and verify output
3. Code Walkthrough
4. Show and explain command to run tool
5. Run tool and verify output
6. Conclusion and review
7. Getting help

## Download code and tools

### Install `substreams-sink-files`

**TODO:** <i>Where is the actual binary file? What is the file type/format/extension (if any)?</i>
https://github.com/streamingfast/substreams-sink-files/releases

### Verify installation

Checking the version of `substreams-sink-files` will produce a message similar to:

```bash
substreams-sink-files version dev
```

### Update system `PATH`

If you want to use the `substreams-sink-tool` from anywhere on your system make sure the computer's `PATH` environment variable has been set correctly.

Add the following lines to the computer's `~/.bashrc` configuration file and then restart the shell session.

```bash
export GOPATH=$HOME/go
export PATH=$PATH:$GOROOT/bin:$GOPATH/bin
```

### Download accompanying code example

The accompanying code example is available in its [official GitHub repository](#). You'll need to download or clone the repository to your computer.

You can use the `git clone` command to acquire the example code.

```bash
git clone TODO_INSERT_REPO_LINK_WHEN_AVAILABLE
```

## Run code and verify output

It's a good idea to run the example code using your installation of the `substreams` CLI to make sure everything is set up and working properly.

Verify the setup for the example project by using the `make build`, `make codegen` and `substreams run` commands.

### Build the example project

First, build the Substreams module by using the included `make` command.

```bash
make
```

### Generate the required code for the example

Next, run the included `make codegen` command to create the required protobuf files.

```bash
make codegen
```

### Run the example

Last, run the project by using the `substreams run` command.

```bash
substreams run -e mainnet.eth.streamingfast.io:443 substreams-filesink-tutorial.yaml map_eth_block_for_sink --start-block 1000000 --stop-block +1
```

### Results of example

The `substreams run` command will result in output resembling the following:

```bash
----------- NEW BLOCK #1,000,000 (1000000) ---------------
{
  "@module": "map_eth_block_for_sink",
  "@block": 1000000,
  "@type": "substreams.sink.files.v1.Lines",
  "@data": {
    "lines": [
      "{\"hash\":\"8e38b4dbf6b11fcc3b9dee84fb7986e29ca0a02cecd8977c161ff7333329681e\",\"number\":1000000,\"parent_hash\":\"b4fbadf8ea452b139718e2700dc1135cfc81145031c84b7ab27cd710394f7b38\",\"timestamp\":\"2016-02-13T22:54:13Z\"}"
    ]
  }
}
```

## Code walkthrough

Take a moment to review the various files in the accompanying Substreams project. Two important files to review include the [Substreams manifest](#) and the [`lib.rs`](#) Rust source code file that contains the module handlers for the file sink example.

### Example module handler

The example code in the [`lib.rs`](#) Rust source code file contains the `map_eth_block_for_sink` module handler responsible for extracting the blockchain data and passing it to the `substreams.sink.files.v1` protobuf for the sink tool and its process.

```rust
#[substreams::handlers::map]
fn map_eth_block_for_sink(block: eth::Block) -> Result<Lines, substreams::errors::Error> {

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

The module handler uses `JSONL` for the output type. The [`json!`](https://docs.rs/serde_json/latest/serde_json/macro.json.html) macro is used to write the block data to the Rust `Vec` type by using the Rust [`vec!`](https://doc.rust-lang.org/std/macro.vec.html) macro.

**NOTE**: <i>We just need to emphasis in the tutorial that it's an example and that a lot of "rows" can be extracted per block. Giving the ERC20/ERC721/ERC1155 examples is a good idea as we coded it: https://github.com/streamingfast/substreams-eth-token-transfers/blob/develop/src/lib.rs#L24</i>

### Example manifest

The Substreams manifest for the accompanying project sets up the required `substreams-sink-files-v0.1.0.spkg`, `substreams.sink.files.v1.Lines` and `sf.ethereum.type.v2.Block` protobufs and reference to the `map_eth_block_for_sink` module handler.

{% code overflow="wrap" %}

```yaml
specVersion: v0.1.0
package:
  name: 'substreams_filesink_tutorial'
  version: v0.1.0

imports:
  eth: https://github.com/streamingfast/sf-ethereum/releases/download/v0.10.2/ethereum-v0.10.4.spkg
  sink_files: https://github.com/streamingfast/substreams-sink-files/releases/download/v0.1.0/substreams-sink-files-v0.1.0.spkg

binaries:
  default:
    type: wasm/rust-v1
    file: target/wasm32-unknown-unknown/release/substreams_ethereum_tutorial.wasm

modules:
  - name: map_eth_block_for_sink
    kind: map
    inputs:
      - source: sf.ethereum.type.v2.Block
    output:
      type: proto:substreams.sink.files.v1.Lines
```

{% endcode %}

**TODO:** <i>How to respect Sink's expected output's type with examples for JSON (maybe CSV))</i>

## Show and explain command to run tool

### Run and configure `substreams-sink-files` tool

The command to start and run the `substreams-sink-files` tool for the accompanying Substreams project will resemble:

{% code overflow="wrap" %}

```bash
substreams-sink-files run --encoder=lines --state-store=./localdata/working/state.yaml mainnet.eth.streamingfast.io:443 gs://staging.dfuseio-global.appspot.com/substreams/eth-token-transfers/spkg/substreams-v0.3.0.spkg map_eth_block_for_sink ./localdata/out
```

{% endcode %}

**TODO:**
<i>

- flags (what are the available flags? dig them out of the source code?)
- output (how to show the directories and files that get produced)
- inspect (need input for this, not sure what it is)
  </i>

## Verify output from tool

The sink tool will produce output resembling the following for a properly configured and working environment and project.

```bash
2023-01-09T07:45:02.563-0800 INFO (substreams-sink-files) starting prometheus metrics server {"listen_addr": "localhost:9102"}
2023-01-09T07:45:02.563-0800 INFO (substreams-sink-files) sink to files {"file_output_path": "./localdata/out", "file_working_dir": "./localdata/working", "endpoint": "mainnet.eth.streamingfast.io:443", "encoder": "lines", "manifest_path": "substreams-filesink-tutorial.yaml", "output_module_name": "map_eth_block_for_sink", "block_range": "", "state_store": "./localdata/working/state.yaml", "blocks_per_file": 10000, "buffer_max_size": 67108864}
2023-01-09T07:45:02.563-0800 INFO (substreams-sink-files) reading substreams manifest {"manifest_path": "substreams-filesink-tutorial.yaml"}
2023-01-09T07:45:02.563-0800 INFO (substreams-sink-files) starting pprof server {"listen_addr": "localhost:6060"}
2023-01-09T07:45:04.041-0800 INFO (pipeline) computed start block {"module_name": "map_eth_block_for_sink", "start_block": 0}
2023-01-09T07:45:04.042-0800 INFO (substreams-sink-files) ready, waiting for signal to quit
2023-01-09T07:45:04.045-0800 INFO (substreams-sink-files) setting up sink {"block_range": {"start_block": 0, "end_block": "None"}, "cursor": {"Cursor":"","Block":{}}}
2023-01-09T07:45:04.048-0800 INFO (substreams-sink-files) starting new file boundary {"boundary": "[0, 10000)"}
2023-01-09T07:45:04.049-0800 INFO (substreams-sink-files) boundary started {"boundary": "[0, 10000)"}
2023-01-09T07:45:04.049-0800 INFO (substreams-sink-files) starting stats service {"runs_each": "2s"}
2023-01-09T07:45:06.052-0800 INFO (substreams-sink-files) substreams sink stats {"progress_msg_rate": "0.000 msg/s (0 total)", "block_rate": "650.000 blocks/s (1300 total)", "last_block": "#1299 (a0f0f283e0d297dd4bcf4bbff916b1df139d08336ad970e77f26b45f9a521802)"}
```

## Conclusion and review

**NOTE**: Provide a breif recap of the purpose of the sink tool, what's required to use it, where to get the tool and example, how to run the tool and example and any gotchas or tips and tricks.

## Getting help

To get help with `substreams-sink-files` at any time, use the `substreams-sink-files -h` command. This will print helpful information about the application to the shell session.

```bash
Substreams Sink to Files (JSONL, CSV, etc.)

Usage:
  substreams-sink-files [command]

Available Commands:
  completion  Generate the autocompletion script for the specified shell
  help        Help about any command
  run         Runs extractor code

Flags:
      --delay-before-start duration   [OPERATOR] Amount of time to wait before starting any internal processes, can be used to perform to maintenance on the pod before actually letting it starts
  -h, --help                          help for substreams-sink-files
      --metrics-listen-addr string    [OPERATOR] If non-empty, the process will listen on this address for Prometheus metrics request(s) (default "localhost:9102")
      --pprof-listen-addr string      [OPERATOR] If non-empty, the process will listen on this address for pprof analysis (see https://golang.org/pkg/net/http/pprof/) (default "localhost:6060")
  -v, --version                       version for substreams-sink-files

Use "substreams-sink-files [command] --help" for more information about a command.
```

---

**-- DEV NOTES --**

**NOTE**: Quick tutorial like content, from your Substreams, do this, run that, etc. This is a more advanced tutorial, so we give quick overview of the commands with quick explanation.

**NOTE**: There need to have some content about how the limitation of this sink which write bundles only when last block of a bundle is final.

**NOTE**: Add discussion about where Substreams cursor is saved and importance of persisting this state (save as a .yaml file)

**NOTE**: Add discussion about s3, gcs, etc. vs. local files

**NOTE**: Also, quickly describe that the actual output directory can be:

A local folder
A Google Cloud Storage Bucket (gs://<bucket>/<path>)
An S3 compatible Bucket (s3://<bucket>/<path>)
An Azure bucket (az://<bucket>/<path>)
Configuration details for those could be seen at https://github.com/streamingfast/dstore#features
