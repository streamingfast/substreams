# Ethereum Local Development with Foundry

This guide walks you through setting up a complete local Ethereum development environment for Substreams development using Foundry. You'll deploy a sample Counter contract, generate transactions, and stream the events using Substreams.

**Estimated time:** 15-20 minutes

## What You'll Build

- Local Ethereum node (Geth in dev mode)
- Firehose integration for block streaming
- Counter smart contract with events
- Substreams module to extract contract events
- Complete Docker Compose orchestration

## Prerequisites

Ensure you have the following installed:

- **Docker 20.10+** with Docker Compose v2.0+
- **Foundry** (forge, cast, anvil) - [Installation guide](https://book.getfoundry.sh/getting-started/installation)
- **Substreams CLI** v1.7.0+ ([installation guide](../../../cli/installing-the-cli.md))
- **Rust** with `wasm32-unknown-unknown` target
- **curl** for testing endpoints

## Architecture Overview

The local environment consists of:

- **Geth** (port 8545/8546) - Ethereum node in dev mode with 1-second block time
- **Substreams** (port 9000) - Substreams Tier1 service providing gRPC streaming
- **Docker network** - Connecting all services

```
┌─────────────────┐    ┌──────────────────┐    ┌─────────────────┐
│   Your App      │    │    Substreams    │    │    Foundry      │
│                 │    │                  │    │                 │
│ ┌─────────────┐ │    │ ┌──────────────┐ │    │ ┌─────────────┐ │
│ │ Substreams  │─┼────┼►│  Substreams  │ │    │ │   Deploy    │ │
│ │    CLI      │ │    │ │   (port      │ │    │ │ Contracts   │ │
│ └─────────────┘ │    │ │    9000)     │ │    │ └─────────────┘ │
└─────────────────┘    │ └──────────────┘ │    └─────────────────┘
                       │         │        │
                       │ ┌──────────────┐ │
                       │ │     Geth     │ │
                       │ │  (ports      │ │
                       │ │ 8545/8546)   │ │
                       │ └──────────────┘ │
                       └──────────────────┘
```

## Setup Instructions

### 1. Create Project Directory

```bash
mkdir substreams-ethereum-foundry
cd substreams-ethereum-foundry
```

### 2. Create Docker Compose Configuration

Create a `docker-compose.yml` file:

```yaml
services:
  ethereum-node:
    image: ghcr.io/streamingfast/go-ethereum:geth-v1.16.7-fh3.0
    container_name: ethereum-dev-node
    entrypoint: ["/app/fireeth"]
    command:
      - start
      - reader-node,relayer,merger,firehose,substreams-tier1,substreams-tier2
      - --config-file=
      - --log-format=text
      - --log-to-file=false
      - --data-dir=/data
      - --advertise-block-id-encoding=hex
      - --advertise-chain-name=local-ethereum
      - --common-first-streamable-block=0
      - --reader-node-path=geth
      - --reader-node-arguments=--dev --dev.period=1 --vmtrace=firehose --http --http.addr=0.0.0.0 --http.port=8545 --http.api=eth,net,web3,debug,txpool --datadir=/data/geth
      - --firehose-grpc-listen-addr=:8089
      - --substreams-tier1-grpc-listen-addr=:9000
      - --substreams-tier1-block-type=sf.ethereum.type.v2.Block
    ports:
      - "8545:8545"
      - "8546:8546"
      - "8089:8089"
      - "9000:9000"
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:8545", "-X", "POST", "-H", "Content-Type: application/json", "-d", '{"jsonrpc":"2.0","method":"eth_blockNumber","params":[],"id":1}']
      interval: 10s
      timeout: 5s
      retries: 5
      start_period: 30s
    volumes:
      - ethereum_data:/data
    networks:
      - ethereum_network

  fund-address:
    image: ghcr.io/streamingfast/go-ethereum:geth-v1.16.7-fh3.0
    volumes:
      - ethereum_data:/data
    entrypoint: ["geth"]
    command:
      - attach
      - --datadir=/data/geth
      - '--exec=[
        "0xf39fd6e51aad88f6f4ce6ab8827279cfffb92266",
        "0x70997970c51812dc3a010c7d01b50e0d17dc79c8",
        "0x3c44cdddb6a900fa2b585dd299e03d12fa4293bc",
        "0x90f79bf6eb2c4f870365e785982e1f101e93b906",
        "0x15d34aaf54267db7d7c367839aaf71a00a2c6a65",
        "0x9965507d1a55bcc2695c58ba16fb37d819b0a4dc",
        "0x976ea74026e726554db657fa54763abd0c3a0aa9",
        "0x14dc79964da2c08b23698b3d3cc7ca32193d9955",
        "0x23618e81e3f5cdf7f54c3d65f7fbc0abf5b21e8f",
        "0xa0ee7a142d267c1f36714e4a8f75612f20a79720"
        ].forEach(to =>
        eth.sendTransaction({from: eth.accounts[0], to, value: web3.toWei(10000, "ether")})
        );'
    restart: on-failure
    deploy:
      restart_policy:
        condition: on-failure
        delay: 2s

volumes:
  ethereum_data:

networks:
  ethereum_network:
    driver: bridge
```

### 3. Start the Environment

```bash
docker compose up -d
```

{% hint style="warning" %}
To restart everything from scratch, use `docker compose down --volumes` to remove all data and start fresh.
{% endhint %}

## Validation Commands

### 1. Check Docker Services

Verify all containers are running and healthy:

```bash
docker compose ps
```

Expected output:
```
NAME                   COMMAND                  SERVICE           STATUS              PORTS
ethereum-dev-node      "/app/fireeth start …"   ethereum-node     Up (healthy)        0.0.0.0:8089->8089/tcp, 0.0.0.0:8545->8545/tcp, 0.0.0.0:8546->8546/tcp, 0.0.0.0:9000->9000/tcp
fund-address           "geth attach --datad…"   fund-address      Exited (0)
```

### 2. Test Substreams Connectivity

Test Substreams Tier1 gRPC connectivity:

```bash
substreams run -e localhost:9000 --plaintext common@v0.1.0 -o clock -s -1
```

Expected output:
```text
Writing clock information only (no data)
----------- BLOCK #24 (6c3dbc20ae11cb856bed9789f7845359e98de71b830f0d9599d0061ed4e962d2) age=1.959195s ---------------
...
```

{% hint style="success" %}
If all validation commands succeed, your environment is ready!
{% endhint %}

## Deploy Counter Contract with Foundry

### 1. Install Foundry

If you haven't installed Foundry yet:

```bash
curl -L https://foundry.paradigm.xyz | bash
foundryup
```

### 2. Initialize Foundry Project

```bash
forge init --no-git --force
```

### 3. Configure Foundry

Add the following network configuration to `foundry.toml` (the file already exists from `forge init`):

```toml
[profile.default]
src = "src"
out = "out"
libs = ["lib"]
solc_version = "0.8.20"

[rpc_endpoints]
local = "http://localhost:8545"

[etherscan]
# No API key needed for local development
```

### 4. Set Environment Variables

Set up the private key environment variable for easier command usage:

```bash
# Using HardHat's default test account 0 private key (pre-funded in dev environment)
export PKEY="0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80"
```

{% hint style="info" %}
This makes subsequent `forge` and `cast` commands cleaner and easier to read. You'll need to run this in each new terminal session.
{% endhint %}

### 5. Use Default Counter Contract

Foundry already created a suitable Counter contract in `src/Counter.sol`. The default contract contains:

```solidity
// SPDX-License-Identifier: UNLICENSED
pragma solidity ^0.8.13;

contract Counter {
    uint256 public number;

    function setNumber(uint256 newNumber) public {
        number = newNumber;
    }

    function increment() public {
        number++;
    }
}
```

No need to modify this file - we'll use the default contract as-is.

### 6. Create Deployment Script

Create `script/Deploy.s.sol`:

```solidity
// SPDX-License-Identifier: UNLICENSED
pragma solidity ^0.8.28;

import "forge-std/Script.sol";
import "../src/Counter.sol";

contract DeployScript is Script {
    function run() external {
        // Default private key for dev mode (account 0)
        uint256 deployerPrivateKey = 0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80;
        
        vm.startBroadcast(deployerPrivateKey);
        
        console.log("Deploying Counter contract...");
        
        Counter counter = new Counter();
        
        console.log("Counter deployed to:", address(counter));
        
        // Generate some test transactions
        console.log("Generating test transactions...");
        
        counter.setNumber(42);
        console.log("Set counter to 42 (tx1)");
        
        counter.increment();
        console.log("Incremented counter (tx2)");
        
        uint256 currentCount = counter.number();
        console.log("Current count:", currentCount);
        
        vm.stopBroadcast();
        
        console.log("\n🎉 SAVE THIS ADDRESS:", address(counter));
        console.log("You'll need it for the Substreams configuration!");
    }
}
```

### 7. Compile and Deploy

```bash
# Compile contracts
forge build

# Deploy to local network
forge create --rpc-url local --private-key $PKEY --broadcast src/Counter.sol:Counter
```

Example output:
```
...
Deployer: 0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266
Deployed to: 0x5FbDB2315678afecb367f032d93F642f64180aa3
Transaction hash: 0x83dba3bc167c38e36b79b075552f67cd6c3f924bf8efb09ab6686ee50ed4816a
```

Export the deployed contract address for easy reference:

```bash
export CONTRACT=<DEPLOYED_ADDRESS>
```

Replace `<DEPLOYED_ADDRESS>` with the actual address from the deployment output.

{% hint style="info" %}
Save your deployed contract address - you'll need it in multiple places for the Substreams module.
{% endhint %}

### 8. Verify Deployment

Test the deployed contract with the following sequence:

```bash
# 1. Read the current counter value
cast call $CONTRACT "number()(uint256)" --rpc-url local

# 2. Set a number using setNumber
cast send $CONTRACT "setNumber(uint256)" 42 --rpc-url local --private-key $PKEY

# 3. Increment the counter
cast send $CONTRACT "increment()" --rpc-url local --private-key $PKEY

# 4. Verify the value changed
cast call $CONTRACT "number()(uint256)" --rpc-url local
```

Expected output from the final call: `43` (42 + 1 from increment)

## Create Substreams Module

### 1. Extract ABI File

{% hint style="info" %}
An ABI file must be created for now to use with `substreams init`.
{% endhint %}

```bash
cat out/Counter.sol/Counter.json | jq .abi > out/Counter.sol/Counter.abi.json
```

### 2. Initialize Substreams Project

```bash
substreams init
```

Follow the interactive prompts:
- **Chosen protocol**: `EVM`
- **Chosen generator**: `evm-events-calls`
- **Please enter the project name**: `counter`
- **Please select the chain**: `Ethereum Mainnet` (or the chain you are targeting)
- **Please enter the contract address**: Use your deployed address (from `$CONTRACT` variable)
- **How do you want to provide the JSON ABI?**: `JSON in a local file`
- **Input the full path of the JSON ABI**: `out/Counter.sol/Counter.abi.json`
- **Please enter the contract initial block number**: `0`
- **Choose a short name for the contract**: `counter`
- **What do you want to track for this contract?**: `Both events and calls`
- **Is this contract a factory**: `No`
- **Add another contract?**: `No`
- **In which directory do you want to download the project?**: `./substreams`
- **How would you like to consume the Substreams?**: `To Postgres` (or choose any other one)

### 3. Build and Test Substreams

```bash
cd substreams
substreams build
substreams run -e localhost:9000 --plaintext counter-v0.1.0.spkg
```

{% hint style="note" %}
Look for the deployment block as it's the one that will contain some actual data. You can scan a specific range using `-s <DEPLOYMENT_BLOCK> -t +10` to scan 10 blocks starting from the deployment block.

You can also leave the substreams run command running and open another terminal to run `cast send $CONTRACT "increment()" --rpc-url local --private-key $PKEY` to increment the counter and see the events appear live in your Substreams output.
{% endhint %}

You should see the Increment events from your contract deployment!

## Troubleshooting

For common issues with Docker Compose, RPC connectivity, and Substreams, see the [Local Development Troubleshooting](../../generic/local-development/troubleshooting.md) guide.

## Next Steps

Now that you have a working local environment:

1. **Try Other Platforms** - Explore [HardHat](hardhat.md) or [Solana](../../solana/local-development/anchor.md) local development
2. **Advanced Substreams** - Learn about [modules](../../../../references/substreams-components/modules/modules.md), [manifests](../../../../references/substreams-components/manifests.md), and [data transformations](../../generic/using-rust-proto.md)
3. **Consuming Substreams** - Connect to [databases](../../../sinks/sql/sql.md) or [streaming platforms](../../../sinks/stream/stream.md)
4. **Production Deployment** - Move to [production endpoints](../../../../references/chains-and-endpoints.md)

## Additional Resources

- [Foundry Book](https://book.getfoundry.sh/)
- [Substreams Ethereum Reference](https://github.com/streamingfast/substreams-ethereum)
- [Substreams CLI Reference](../../../../references/cli/command-line-interface.md)
- [Creating Protobuf Schemas](../../generic/creating-protobuf-schemas.md)
