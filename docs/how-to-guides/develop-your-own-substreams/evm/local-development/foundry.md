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
- **Fireeth** (port 8089) - Firehose integration providing gRPC streaming
- **Docker network** - Connecting all services

```
┌─────────────────┐    ┌──────────────────┐    ┌─────────────────┐
│   Your App      │    │    Substreams    │    │    Foundry      │
│                 │    │                  │    │                 │
│ ┌─────────────┐ │    │ ┌──────────────┐ │    │ ┌─────────────┐ │
│ │ Substreams  │◄┼────┼─┤   Firehose   │ │    │ │   Deploy    │ │
│ │    CLI      │ │    │ │   (port      │ │    │ │ Contracts   │ │
│ └─────────────┘ │    │ │    8089)     │ │    │ └─────────────┘ │
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
version: '3.8'

services:
  ethereum-node:
    image: ghcr.io/streamingfast/go-ethereum:geth-v1.16.7-fh3.0
    container_name: ethereum-dev-node
    command: |
      fireeth start reader-node,relayer,merger,firehose,substreams-tier1,substreams-tier2 
      --config-file= 
      --reader-node-path=geth 
      --reader-node-arguments="--dev --dev.period=1 --http --http.addr=0.0.0.0 --http.port=8545 --http.api=eth,net,web3,debug,txpool --ws --ws.addr=0.0.0.0 --ws.port=8546 --ws.api=eth,net,web3,debug,txpool --gcmode=archive --state.scheme=hash --verbosity=3" 
      --common-first-streamable-block=0 
      --firehose-grpc-listen-addr=:8089
    ports:
      - "8545:8545"   # HTTP RPC
      - "8546:8546"   # WebSocket RPC  
      - "8089:8089"   # Firehose gRPC
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

volumes:
  ethereum_data:

networks:
  ethereum_network:
    driver: bridge
```

### 3. Start the Environment

```bash
docker-compose up -d
```

Wait for the services to start (about 30-60 seconds).

## Validation Commands

### 1. Check Docker Services

Verify all containers are running and healthy:

```bash
docker-compose ps
```

Expected output:
```
NAME                   COMMAND                  SERVICE           STATUS              PORTS
ethereum-dev-node      "fireeth start reade…"   ethereum-node     Up (healthy)        0.0.0.0:8089->8089/tcp, 0.0.0.0:8545->8545/tcp, 0.0.0.0:8546->8546/tcp
```

### 2. Test RPC Connectivity

Test the Ethereum JSON-RPC endpoint:

```bash
curl -X POST -H "Content-Type: application/json" \
  --data '{"jsonrpc":"2.0","method":"eth_blockNumber","params":[],"id":1}' \
  http://localhost:8545
```

Expected response (block number will vary):
```json
{"jsonrpc":"2.0","id":1,"result":"0x1a"}
```

### 3. Test Firehose Connectivity

Test Firehose gRPC connectivity using fireeth:

```bash
docker run --rm --network=host ghcr.io/streamingfast/firehose-ethereum:latest \
  fireeth tools firehose-client localhost:8089 --plaintext -o text -- -1
```

Or using substreams CLI:

```bash
docker run --rm --network=host ghcr.io/streamingfast/substreams:latest \
  substreams run -e localhost:8089 --plaintext common@v0.1.0 -s -1
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
mkdir foundry-contracts
cd foundry-contracts
forge init --no-git
```

### 3. Configure Foundry

Create `foundry.toml`:

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

### 4. Create Counter Contract

Create `src/Counter.sol`:

```solidity
// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

contract Counter {
    uint256 private count;
    address public owner;

    event Incremented(uint256 newCount, address caller);
    event Decremented(uint256 newCount, address caller);

    constructor(uint256 initialCount) {
        count = initialCount;
        owner = msg.sender;
    }

    function increment() public {
        count += 1;
        emit Incremented(count, msg.sender);
    }

    function decrement() public {
        require(count > 0, "Counter: cannot decrement below zero");
        count -= 1;
        emit Decremented(count, msg.sender);
    }

    function getCount() public view returns (uint256) {
        return count;
    }
}
```

### 5. Create Deployment Script

Create `script/Deploy.s.sol`:

```solidity
// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

import "forge-std/Script.sol";
import "../src/Counter.sol";

contract DeployScript is Script {
    function run() external {
        // Default private key for dev mode (account 0)
        uint256 deployerPrivateKey = 0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80;
        
        vm.startBroadcast(deployerPrivateKey);
        
        console.log("Deploying Counter contract...");
        
        // Deploy with initial count of 0
        Counter counter = new Counter(0);
        
        console.log("Counter deployed to:", address(counter));
        
        // Generate some test transactions
        console.log("Generating test transactions...");
        
        counter.increment();
        console.log("Incremented counter (tx1)");
        
        counter.increment();
        console.log("Incremented counter (tx2)");
        
        uint256 currentCount = counter.getCount();
        console.log("Current count:", currentCount);
        
        vm.stopBroadcast();
        
        console.log("\n🎉 SAVE THIS ADDRESS:", address(counter));
        console.log("You'll need it for the Substreams configuration!");
    }
}
```

### 6. Compile and Deploy

```bash
# Compile contracts
forge build

# Deploy to local network
forge script script/Deploy.s.sol --rpc-url local --broadcast --legacy
```

**SAVE THE CONTRACT ADDRESS** from the deployment output!

### 7. Verify Deployment

```bash
# Replace <ADDRESS> with your deployed contract address
cast call <ADDRESS> "getCount()(uint256)" --rpc-url local
```

Expected output: `2` (since we incremented twice)

{% hint style="info" %}
Save your deployed contract address - you'll need it in multiple places for the Substreams module.
{% endhint %}

## Create Substreams Module

### 1. Initialize Substreams Project

```bash
cd .. # Back to substreams-ethereum-foundry
mkdir substreams
cd substreams
```

### 2. Create Protobuf Schema

Create `proto/counter.proto`:

```protobuf
syntax = "proto3";

package counter.v1;

message CounterEvents {
  repeated CounterEvent events = 1;
}

message CounterEvent {
  string tx_hash = 1;
  uint64 block_number = 2;
  uint64 log_index = 3;
  string contract_address = 4;
  string event_type = 5; // "Incremented" or "Decremented"
  uint64 new_count = 6;
  string caller = 7;
}
```

### 3. Create Substreams Manifest

Create `substreams.yaml`:

```yaml
specVersion: v0.1.0
package:
  name: counter_events
  version: v0.1.0

imports:
  ethereum: https://github.com/streamingfast/substreams-ethereum/releases/download/v0.10.0/substreams-ethereum-v0.10.0.spkg

protobuf:
  files:
    - counter.proto
  importPaths:
    - ./proto

binaries:
  default:
    type: wasm/rust-v1
    file: ./target/wasm32-unknown-unknown/release/substreams.wasm

modules:
  - name: map_counter_events
    kind: map
    initialBlock: 0
    inputs:
      - params: string
      - source: sf.ethereum.type.v2.Block
    output:
      type: proto:counter.v1.CounterEvents

params:
  map_counter_events: "contract=0x0000000000000000000000000000000000000000" # Replace with your contract address

network: local
```

### 4. Create Cargo Configuration

Create `Cargo.toml`:

```toml
[package]
name = "substreams-counter"
version = "0.1.0"
edition = "2021"

[lib]
crate-type = ["cdylib"]

[dependencies]
substreams = "0.6"
substreams-ethereum = "0.10"
prost = "0.12"
prost-types = "0.12"
hex = "0.4"

[build-dependencies]
substreams-ethereum = "0.10"
```

### 5. Create Rust Handler

Create `src/lib.rs`:

```rust
use substreams::prelude::*;
use substreams_ethereum::pb::eth::v2 as eth;
use substreams_ethereum::{Event, NULL_ADDRESS};

mod pb;
use pb::counter::v1::{CounterEvent, CounterEvents};

// ABI-generated event structs
#[derive(Event)]
#[event(name = "Incremented")]
struct Incremented {
    #[indexed]
    new_count: u64,
    #[indexed] 
    caller: Vec<u8>,
}

#[derive(Event)]
#[event(name = "Decremented")]
struct Decremented {
    #[indexed]
    new_count: u64,
    #[indexed]
    caller: Vec<u8>,
}

#[substreams::handlers::map]
fn map_counter_events(params: String, block: eth::Block) -> Result<CounterEvents, substreams::errors::Error> {
    let contract_address = extract_contract_address(&params)?;
    
    let mut events = Vec::new();
    
    for transaction in block.transaction_traces {
        for call in transaction.calls {
            for log in call.logs {
                if log.address != contract_address {
                    continue;
                }
                
                // Try to decode as Incremented event
                if let Some(incremented) = Incremented::match_and_decode(&log) {
                    events.push(CounterEvent {
                        tx_hash: format!("0x{}", hex::encode(&transaction.hash)),
                        block_number: block.number,
                        log_index: log.index,
                        contract_address: format!("0x{}", hex::encode(&log.address)),
                        event_type: "Incremented".to_string(),
                        new_count: incremented.new_count,
                        caller: format!("0x{}", hex::encode(&incremented.caller)),
                    });
                }
                
                // Try to decode as Decremented event
                if let Some(decremented) = Decremented::match_and_decode(&log) {
                    events.push(CounterEvent {
                        tx_hash: format!("0x{}", hex::encode(&transaction.hash)),
                        block_number: block.number,
                        log_index: log.index,
                        contract_address: format!("0x{}", hex::encode(&log.address)),
                        event_type: "Decremented".to_string(),
                        new_count: decremented.new_count,
                        caller: format!("0x{}", hex::encode(&decremented.caller)),
                    });
                }
            }
        }
    }
    
    Ok(CounterEvents { events })
}

fn extract_contract_address(params: &str) -> Result<Vec<u8>, substreams::errors::Error> {
    for param in params.split('&') {
        if let Some(contract) = param.strip_prefix("contract=") {
            if contract.starts_with("0x") {
                return Ok(hex::decode(&contract[2..]).map_err(|e| {
                    substreams::errors::Error::Unexpected(format!("Invalid contract address: {}", e))
                })?);
            }
        }
    }
    Err(substreams::errors::Error::Unexpected("Contract address parameter not found".to_string()))
}
```

### 6. Generate Protobuf Code

```bash
substreams protogen --exclude-paths="google,sf/substreams"
```

### 7. Build Substreams Module

```bash
cargo build --target wasm32-unknown-unknown --release
```

### 8. Update Contract Address

**IMPORTANT:** Update the contract address in `substreams.yaml`:

```yaml
params:
  map_counter_events: "contract=YOUR_CONTRACT_ADDRESS_HERE" # Replace with actual address
```

### 9. Test Substreams

```bash
substreams gui -e localhost:8089 --plaintext map_counter_events -s 0
```

You should see the Incremented events from your contract deployment!

{% hint style="note" %}
Dev mode produces blocks every 1 second when transactions are pending. If you don't see events, ensure your contract deployment transactions were successful.
{% endhint %}

## Troubleshooting

### Docker Issues

**Problem:** Container fails to start
```bash
# Check logs
docker-compose logs ethereum-node

# Restart services
docker-compose down
docker-compose up -d
```

**Problem:** Port conflicts
```bash
# Check what's using the ports
lsof -i :8545
lsof -i :8089

# Kill conflicting processes or change ports in docker-compose.yml
```

### RPC Connection Problems

**Problem:** Connection refused to localhost:8545
- Ensure Docker container is running and healthy
- Check firewall settings
- Verify port mapping in docker-compose.yml

**Problem:** Invalid JSON-RPC response
- Wait for container to fully initialize (check health status)
- Verify Geth is running with correct parameters

### Foundry-Specific Troubleshooting

**Problem:** "failed to get chain id"
- Verify RPC endpoint is accessible: `curl -X POST -H "Content-Type: application/json" --data '{"jsonrpc":"2.0","method":"eth_chainId","params":[],"id":1}' http://localhost:8545`
- Use `--chain-id 1337` flag if needed

**Problem:** "InvalidTransaction" during deployment
- Use `--legacy` flag for legacy transaction format
- Ensure sufficient gas limit

**Problem:** "NonceTooLow" or "NonceTooHigh"
- Wait for previous transactions to be mined
- Reset if needed: restart the Docker container

### Firehose Connectivity Issues

**Problem:** gRPC connection failed
- Verify Firehose is listening on port 8089
- Check Docker network connectivity
- Ensure `--plaintext` flag is used for local development

### Substreams Build Issues

**Problem:** `wasm32-unknown-unknown` target not found
```bash
rustup target add wasm32-unknown-unknown
```

**Problem:** Protobuf generation fails
- Ensure `substreams protogen` excludes system paths
- Check proto file syntax

**Problem:** No events in Substreams output
- Verify contract address in substreams.yaml matches deployed contract
- Check that transactions actually generated events
- Ensure block range includes deployment block

### Mac-Specific Networking

**Problem:** Docker networking issues on macOS
- Use `host.docker.internal` instead of `localhost` in some contexts
- Ensure Docker Desktop networking is properly configured

## Next Steps

Now that you have a working local environment:

1. **Extend the Contract** - Add more events and functions to explore different patterns
2. **Advanced Substreams** - Implement stores, multiple modules, and complex data transformations  
3. **Testing Scenarios** - Create reproducible test cases with specific blockchain states
4. **Integration** - Connect your Substreams to sinks like databases or message queues

## Additional Resources

- [Foundry Book](https://book.getfoundry.sh/)
- [Substreams Ethereum Reference](https://github.com/streamingfast/substreams-ethereum)
- [Geth Development Mode](https://geth.ethereum.org/docs/developers/dapp-developer/dev-mode)
- [Firehose Ethereum](https://github.com/streamingfast/firehose-ethereum)
