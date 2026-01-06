# Ethereum Local Development with HardHat

This guide walks you through setting up a complete local Ethereum development environment for Substreams development using HardHat. You'll deploy a sample Counter contract, generate transactions, and stream the events using Substreams.

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
- **Node.js 18+** with npm or yarn
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
│   Your App      │    │    Substreams    │    │   HardHat       │
│                 │    │                  │    │                 │
│ ┌─────────────┐ │    │ ┌──────────────┐ │    │ ┌─────────────┐ │
│ │ Substreams  │◄┼────┼─┤  Substreams  │ │    │ │   Deploy    │ │
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
mkdir substreams-ethereum-local
cd substreams-ethereum-local
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

## Deploy Counter Contract with HardHat

### 1. Initialize HardHat Project

{% hint style="info" %}
This guide mostly follows [HardHat's setup tutorial](https://hardhat.org/docs/tutorial/setup) in a streamlined way.
{% endhint %}

```bash
npx hardhat --init
```

Follow the interactive prompts:
- First choose `Hardhat 3 Beta (recommended for new projects)`
- Second use `.` as the relative path
- Third choose `A TypeScript Hardhat project using Node Test Runner and Viem`
- Fourth choose `Yes` when requested to install dependencies

### 2. Configure HardHat

Update `hardhat.config.ts` to add the local network:

```typescript
export default defineConfig({
  ...,
  networks: {
    local: {
      type: "http",
      chainType: "l1",
      chainId: 1337,
      url: "http://localhost:8545",
    },
    ...
  },
});
```

### 3. Counter Contract

HardHat created a contract for us! You can view the contract at `contracts/Counter.sol`:

```solidity
// SPDX-License-Identifier: UNLICENSED
pragma solidity ^0.8.28;

contract Counter {
  uint public x;

  event Increment(uint by);

  function inc() public {
    x++;
    emit Increment(1);
  }

  function incBy(uint by) public {
    require(by > 0, "incBy: increment should be positive");
    x += by;
    emit Increment(by);
  }
}
```

### 4. Deploy the Contract

HardHat created a deployment script at `ignition/modules/Counter.ts`. Deploy the contract:

```bash
npx hardhat ignition deploy ignition/modules/Counter.ts --network=local
```

Example output:
```
...
Deployed Addresses

CounterModule#Counter - <CONTRACT_ADDRESS>
```

**SAVE THE CONTRACT ADDRESS** - you'll need it for Substreams configuration!

### 5. Verify Deployment

```bash
npx hardhat console --network local
```

In the console:
```javascript
const counter = await viem.getContractAt("Counter", "<CONTRACT_ADDRESS>");
await counter.read.x(); // Should return current counter value
```

{% hint style="info" %}
Save your deployed contract address - you'll need it in multiple places for the Substreams module.
{% endhint %}

## Create Substreams Module

### 1. Extract ABI File

{% hint style="info" %}
An ABI file must be created for now to use with `substreams init`.
{% endhint %}

```bash
cat artifacts/contracts/Counter.sol/Counter.json | jq .abi > artifacts/contracts/Counter.sol/Counter.abi.json
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
- **Please enter the contract address**: `<CONTRACT_ADDRESS>` (use your deployed address)
- **How do you want to provide the JSON ABI?**: `JSON in a local file`
- **Input the full path of the JSON ABI**: `artifacts/contracts/Counter.sol/Counter.abi.json`
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
{% endhint %}

You should see the Increment events from your contract deployment!

{% hint style="note" %}
Dev mode produces blocks every 1 second when transactions are pending. If you don't see events, ensure your contract deployment transactions were successful.
{% endhint %}

## Troubleshooting

For common issues with Docker Compose, RPC connectivity, Substreams, and platform-specific problems, see the [Local Development Troubleshooting Guide](../../generic/local-development/troubleshooting.md).

## Next Steps

Now that you have a working local environment:

1. **Try Other Platforms** - Explore [Foundry](foundry.md) or [Solana](../../solana/local-development/anchor.md) local development
2. **Advanced Substreams** - Learn about [stores, modules, and data transformations](../../../generic/using-rust-proto.md)
3. **Consuming Substreams** - Connect to [databases](../../../../sinks/sql/sql.md) or [streaming platforms](../../../../sinks/stream/stream.md)
4. **Production Deployment** - Move to [production endpoints](../../../../../references/chains-and-endpoints.md)

## Additional Resources

- [HardHat Documentation](https://hardhat.org/docs)
- [Substreams Ethereum Reference](https://github.com/streamingfast/substreams-ethereum)
- [Substreams CLI Reference](../../../../references/cli/command-line-interface.md)
- [Creating Protobuf Schemas](../../../generic/creating-protobuf-schemas.md)
