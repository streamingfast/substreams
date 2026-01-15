# Parquet

## Overview

The `substreams sink protojson` tool provides the ability to write data from a Substreams directly to JSONL files, encoded with ProtoJSON encoding.

For example, you could extract all of the ERC20, ERC721, and ERC1155 transfers from the Ethereum blockchain and persist the data to a files-based store.

Most existing Substreams can be used to write data to ProtoJSONL files directly.


## Running

* Example to get USDT transfers into protojson
  ```
	# Extract USDT events to ProtoJSONL of a 200 blocks range, in 100-blocks-chunks
	substreams sink protojson substreams_ethereum_usdt@v0.1.0 map_events -o ./transfers --filter=.transfers[] -n 100 -s 20000000 -t +200
	```
	
	This will create the following files:
	```
	transfers/0020000100-0020000200.jsonl
  transfers/0020000000-0020000100.jsonl
  ```

  With content like this (compacted):
  ```json
	{
  "evtTxHash": "8cd7657e49e207d2eef05776820d6234b5c33482f53c5ec4db5f3220cc029c06",
  "evtIndex": 135,
  "evtBlockTime": "2024-06-01T22:56:59Z",
  "evtBlockNumber": "20000100",
  "from": "ZfU/nt+BtrSyp9QMPKVgVNTJO5o=",
  "to": "cmjDmMZ/nem5tGPHnroITwvYNo4=",
  "value": "421253867"
  }
  ```

	
* Example to get everything related to USDT into protojson
  ```
	# Extract USDT events to ProtoJSONL of a 200 blocks range, in 100-blocks-chunks
	substreams sink protojson substreams_ethereum_usdt@v0.1.0 map_events -o ./usdt_output --filter=. -n 100 -s 20000000 -t +200
	```
	
	This will create the following files:
	```
	usdt_output/0020000100-0020000200.jsonl
  usdt_output/0020000000-0020000100.jsonl
  ```

  With content like this (compacted):
  ```json
  {
    "transfers": [
      {
        "evtTxHash": "8cd7657e49e207d2eef05776820d6234b5c33482f53c5ec4db5f3220cc029c06",
        "evtIndex": 135,
        "evtBlockTime": "2024-06-01T22:56:59Z",
        "evtBlockNumber": "20000100",
        "from": "ZfU/nt+BtrSyp9QMPKVgVNTJO5o=",
        "to": "cmjDmMZ/nem5tGPHnroITwvYNo4=",
        "value": "421253867"
      },
      { ... },
  }
  ```
