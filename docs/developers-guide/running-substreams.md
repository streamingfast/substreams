---
description: Running StreamingFast Substreams for the first time
---

# Running substreams

After a successful build Substreams can be started with the following command, explained in greater detail below.

```bash
substreams run -e mainnet.eth.streamingfast.io:443 \
   substreams.yaml \
   map_transfers \
   --start-block 12292922 \
   --stop-block +1
```

### Explanation

#### Substreams `run`

First, start the Substreams CLI tool passing it a `run` command.

#### Firehose URI

The server address is required by Substreams to connect to for data retrieval. The data provider for Substreams is located at the address. This is a running Firehose instance.\
`-e mainnet.eth.streamingfast.io:443`

#### Substreams YAML configuration file

Inform Substreams where to find the `substreams.yaml` configuration file.

#### Module

The `map_transfers` module is defined in the manifest and it is the module that will be run by Substreams.

#### Block mapping

Start mapping at the specific block 12292922 by using passing the flag and block number.\
`--start-block 12292922`

Cease block processing with `--stop-block +1.` The +1 option will request a single block. In the example, the next block would be 12292923.

### Successful Substreams results

Messages will be printed to the terminal for a successfully installed and configured Substreams setup.

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

The example output contains data for different transfers from data in the blockchain. These transfers can also be [verified on Etherscan](https://etherscan.io/tx/0xcfb197f62ec5c7f0e71a11ec0c4a0e394a3aa41db5386e85526f86c84b3f2796).
