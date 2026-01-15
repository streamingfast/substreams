# Files

### Purpose

This documentation exists to assist you in understanding and beginning to use the StreamingFast [`substreams-sink-files`](https://github.com/streamingfast/substreams-sink-files) tool. The Substreams module paired with this tutorial is a basic example of the elements required for sinking blockchain data into files-based storage solutions.

### Overview

The `substreams-sink-files` tool provides the ability to pipe data extracted from a blockchain to various types of files-based persistence solutions.

For example, you could extract all of the ERC20, ERC721, and ERC1155 transfers from the Ethereum blockchain and persist the data to a files-based store.

It supports CSV, and Parquet format. (for ProtoJSON, use `substreams sink protojson` command: [ProtoJSON](how-to-guides/sinks/protojson.md))

See [substreams-sink-files README](https://github.com/streamingfast/substreams-sink-files) for details
