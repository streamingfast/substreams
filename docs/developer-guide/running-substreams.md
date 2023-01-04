---
description: Running StreamingFast Substreams for the first time
---

# Running Substreams

After a successful build Substreams can be started with the following command, explained in greater detail below.

```
substreams run -e mainnet.eth.streamingfast.io:443 \
   substreams.yaml \
   map_transfers \
   --start-block 12292922 \
   --stop-block +1
```

### Explanation

#### Substreams Run

First, start the Substreams CLI tool passing it a `run` command.

#### Firehose URI

The server address is required by Substreams to connect to for data retrieval. The data provider for Substreams is located at the address. This is a running Firehose instance.\
`-e mainnet.eth.streamingfast.io:443`

#### Substreams YAML Configuration File

Inform Substreams where to find the `substreams.yaml` configuration file.

#### Module

The `map_transfers` module is defined in the manifest and it is the module that will be run by Substreams.

#### Block Mapping

Start mapping at the specific block 12292922 by using passing the flag and block number. \
`--start-block 12292922`

Cease block execution with `--stop-block +1.` The +1 option will request a single block. In the example, the next block would be 12292923.

### Successful Substreams Results

The following messages will be printed to the terminal for a successfully installed and configured Substreams setup.

```bash
 substreams run -e mainnet.eth.streamingfast.io:443 \
   substreams.yaml \
   map_transfers \
   --start-block 12292922 \
   --stop-block +1
2022-05-30T10:52:27.256-0400 INFO (substreams) connecting...
2022-05-30T10:52:27.389-0400 INFO (substreams) connected

----------- IRREVERSIBLE BLOCK #12,292,922 (12292922) ---------------
map_transfers: log: NFT Contract bc4ca0eda7647a8ab7c2061c2e118a18a936f13d invoked
[...]
map_transfers: message "eth.erc721.v1.Transfers": {
  "transfers": [
    {
      "from": "AAAAAAAAAAAAAAAAAAAAAAAAAAA=",
      "to": "q6cWGn+2nIjhbtn0Vc5it5HuTQM=",
      "trxHash": "z7GX9i7Fx/DnGhHsDEoOOUo6pB21OG6FUm+GyEs/J5Y=",
      "ordinal": "85"
    },
    ...
    {
      "from": "AAAAAAAAAAAAAAAAAAAAAAAAAAA=",
      "to": "q6cWGn+2nIjhbtn0Vc5it5HuTQM=",
      "tokenId": "29",
      "trxHash": "z7GX9i7Fx/DnGhHsDEoOOUo6pB21OG6FUm+GyEs/J5Y=",
      "ordinal": "114"
    }
  ]
}
```

The example output shown above contains data for different transfers from data in the blockchain. These transfers can also be verified on [Etherscan](https://etherscan.io/tx/0xcfb197f62ec5c7f0e71a11ec0c4a0e394a3aa41db5386e85526f86c84b3f2796).

## Development abd production mode

### Development mode

Production and development modes impact the execution of Substreams. Key aspects of execution include:

- The time required to reach the first byte.
- The speed that large ranges get executed.
- The module logs and outputs that are sent back to the client.

## Parallel execution in development versus production modes

Differences between production and development modes include:

- Forward parallel execution is enabled in production mode and disabled in development mode.
- The time required to reach the first byte in development mode is faster than in production mode.

Specific attributes of development mode include:

- The client will receive all of the executed module's logs.
- It's possible to request specific store snapshots in the execution tree.
- Multiple module's output is possible.

## Production and development parallel execution examples

In most cases, you will run production mode, using a Substreams sink. Development mode is enabled by default in the CLI unless -p is specified.

Examples: (given the dependencies: `[block] --> [map_pools] --> [store_pools] --> [map_transfers])`

- Running the `substreams run substreams.yaml map_transfers` command executes in development mode and only prints the `map_transfers` module's outputs and logs.
- Running the `substreams run substreams.yaml map_transfers --debug-modules-output=map_pools,map_transfers,store_pools` command executes in development mode and only prints the outputs of the `map_pools`, `map_transfers`, and `store_pools` modules.
- Running the `substreams run substreams.yaml map_transfers -s 1000 -t +5 --debug-modules-initial-snapshot=store_pools` command executes in development mode and prints all the entries in the `store_pools` module at block 999, then continues with outputs and logs from the `map_transfers` module in blocks 1000 through 1004.

## Backward and forward parallel execution steps

The two steps involved during parallel execution are backward execution and forward execution.

Backward parallel execution consists of executing in parallel block ranges, from the module's initial block, up to the start block of the request. If the start block of the request matches the module's initial block no backwards execution is performed.

Forward parallel execution consists of executing in parallel block ranges from the start block of the request up to last known final block, also called an irreversible block, or the stop block of the request depending on which is smaller. Forward parallel execution significantly improves the performance of Substreams.

Backward parallel execution will occur in both development and production modes. Forward parallel execution only occurs in production mode.
