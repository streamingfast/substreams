---
description: Get off the ground by using Substreams by StreamingFast with init command
---

A new init command has been released to be able to more easily boostrap your Substreams.

# Requirements
1. Substreams cli: you need to have the cli installed. Navigate to the Installing the Cli page on how to install it
2. Rust installed: to develop substreams, you need to install Rust. Visit the official Rust installation [page](https://www.rust-lang.org/tools/install)
3. (Optional) Docker installed: if you do not want to install Rust and run the build commands via docker, you have to install Docker. Visit the official installation [page](https://docs.docker.com/engine/install/)

# Init command
```bash
$> substreams init
Project name: my-first-substreams
Protocol: Ethereum
Ethereum chain: Mainnet
Track contract: n
Generating Ethereum Mainnet project using Bored Ape Yacht Club contract for demo purposes
Retrieving Ethereum Mainnet contract information (ABI & creation block)
Writing project files
Generating Protobuf Rust code
Project "my-first-substreams" initialized at "/absolute/path/to/my-first/substreams/"
```

There are a good number of options that you can pass in. You can choose the chain that you want, and you can pass in the contract address. If you omit the contract address, the default will be Bored Ape Yacht Club contract.

# Run your initialized Substreams
```bash
# run command to fetch your auth token
make build && substreams run substreams.yaml db_out -e mainnet.eth.streamingfast.io:443 -t +1000
```

This will build your substreams and if the is successful, it will run your substreams for the first 1000 blocks from your initial block on eth mainnet.
