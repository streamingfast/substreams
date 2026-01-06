# Solana Local Development with Anchor

This guide walks you through setting up a complete local Solana development environment for Substreams development using Anchor. You'll deploy a sample Counter program, generate transactions, and stream the events using Substreams.

**Estimated time:** 20-25 minutes

## What You'll Build

- Local Solana validator (test mode)
- Firehose integration for block streaming
- Counter Anchor program with events
- Substreams module to extract program events
- Complete Docker Compose orchestration

## Prerequisites

Ensure you have the following installed:

- **Docker 20.10+** with Docker Compose v2.0+
- **Node.js 18+** with npm or yarn
- **Anchor CLI** (latest) - [Installation guide](https://www.anchor-lang.com/docs/installation)
- **Solana CLI** 2.x+ - [Installation guide](https://docs.solana.com/cli/install-solana-cli-tools)
- **Substreams CLI** v1.7.0+ ([installation guide](../../../cli/installing-the-cli.md))
- **Rust** with `wasm32-unknown-unknown` target
- **curl** for testing endpoints

## Architecture Overview

The local environment consists of:

- **Solana Validator** (port 8899/8900) - Test validator with unlimited SOL
- **Substreams** (port 9000) - Substreams Tier1 service providing gRPC streaming
- **Docker network** - Connecting all services

```
┌─────────────────┐    ┌──────────────────┐    ┌─────────────────┐
│   Your App      │    │    Substreams    │    │     Anchor      │
│                 │    │                  │    │                 │
│ ┌─────────────┐ │    │ ┌──────────────┐ │    │ ┌─────────────┐ │
│ │ Substreams  │─┼────┼►│  Substreams  │ │    │ │   Deploy    │ │
│ │    CLI      │ │    │ │   (port      │ │    │ │  Programs   │ │
│ └─────────────┘ │    │ │    9000)     │ │    │ └─────────────┘ │
└─────────────────┘    │ └──────────────┘ │    └─────────────────┘
                       │         │        │
                       │ ┌──────────────┐ │
                       │ │   Solana     │ │
                       │ │ Validator    │ │
                       │ │  (ports      │ │
                       │ │ 8899/8900)   │ │
                       │ └──────────────┘ │
                       └──────────────────┘
```

## Setup Instructions

### 1. Create Project Directory

```bash
mkdir substreams-solana-local
cd substreams-solana-local
```

### 2. Create Docker Compose Configuration

Create a `docker-compose.yml` file:

```yaml
services:
  solana-node:
    image: ghcr.io/beeman/solana-test-validator:2.2.15
    container_name: solana-dev-validator
    entrypoint: ["solana-test-validator"]
    command:
      - --bind-address=0.0.0.0
      - --rpc-port=8899
      - --faucet-port=8900
      - --faucet-sol=1000000
      - --enable-rpc-transaction-history
      - --enable-extended-tx-metadata-storage
    ports:
      - "8899:8899"
      - "8900:8900"
    healthcheck:
      test: ["CMD-SHELL", "timeout 1 bash -c '</dev/tcp/localhost/8899' || exit 1"]
      interval: 10s
      timeout: 5s
      retries: 10
      start_period: 60s
    volumes:
      - solana_data:/root/.config/solana/test-ledger
    networks:
      - solana_network

  firehose:
    image: ghcr.io/streamingfast/firehose-solana:v1.1.4
    container_name: firehose-solana
    entrypoint: ["/app/firecore"]
    command:
      - start
      - reader-node,merger,relayer,firehose,substreams-tier1,substreams-tier2
      - --config-file=
      - --log-format=text
      - --log-to-file=false
      - --data-dir=/data
      - --common-first-streamable-block=0
      - --advertise-block-id-encoding=base58
      - --advertise-chain-name=local-solana
      - --reader-node-path=/app/firesol
      - --reader-node-arguments=fetch rpc 0 --endpoints=http://solana-node:8899 --state-dir=/data/reader-state
      - --firehose-grpc-listen-addr=:8089
      - --substreams-tier1-grpc-listen-addr=:9000
      - --substreams-tier1-block-type=sf.solana.type.v1.Block
    ports:
      - "8089:8089"
      - "9000:9000"
    depends_on:
      solana-node:
        condition: service_healthy
    healthcheck:
      test: ["CMD", "grpc_health_probe", "-addr=localhost:9000"]
      interval: 10s
      timeout: 5s
      retries: 5
      start_period: 60s
    volumes:
      - solana_data:/data
    networks:
      - solana_network

volumes:
  solana_data:

networks:
  solana_network:
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
solana-dev-validator   "solana-test-validat…"   solana-node       Up (healthy)        0.0.0.0:8899->8899/tcp, 0.0.0.0:8900->8900/tcp
firehose-solana        "/app/firecore start…"   bigeagle-firehose Up (healthy)        0.0.0.0:8089->8089/tcp, 0.0.0.0:9000->9000/tcp
firehose-solana        "firehose-solana sta…"   firehose-solana   Up (healthy)        0.0.0.0:9000->9000/tcp
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

## Deploy Counter Program with Anchor

### 1. Install Required Tools

Anchor installation requires several prerequisites. Before proceeding, familiarize yourself with the [Anchor installation requirements](https://www.anchor-lang.com/docs/installation).

Install AVM (Anchor Version Manager):
```bash
cargo install --git https://github.com/coral-xyz/anchor avm --force
```

Install and use the latest Anchor version:
```bash
avm install latest
avm use latest
```

Verify installation:
```bash
anchor --version
# Expected output (latest version):
# anchor-cli 0.30.1

solana --version
# Expected output (version 2.x or higher):
# solana-cli 2.3.13 (src:5466f459; feat:2142755730, client:Agave)
```

### 2. Configure Solana CLI

```bash
# Set cluster to local
solana config set --url http://localhost:8899

# Create a keypair (or use existing)
solana-keygen new --outfile ~/.config/solana/id.json

# Airdrop SOL for deployment
solana airdrop 10
```

### 3. Initialize Anchor Project

```bash
anchor init counter --no-git
```

### 4. Configure Anchor

The `anchor init` command creates a default `Anchor.toml` file. Verify the configuration matches your local setup:

```toml
[provider]
cluster = "Localnet"
wallet = "~/.config/solana/id.json"
```

The cluster should be set to "Localnet" and the wallet path should match the keypair you created in the previous step.

### 5. Review Generated Program

The `anchor init counter` command created a basic program for us. The generated program source is located at `programs/counter/src/lib.rs`:

```rust
// File: programs/counter/src/lib.rs
use anchor_lang::prelude::*;

declare_id!("4xFCXVK9eym8DoPwgn9Z6gHqjmWk8B7bZimpDgP7cvNs");

#[program]
pub mod counter {
    use super::*;

    pub fn initialize(ctx: Context<Initialize>) -> Result<()> {
        msg!("Greetings from: {:?}", ctx.program_id);
        Ok(())
    }
}

#[derive(Accounts)]
pub struct Initialize {}
```

This simple program demonstrates the basic structure of an Anchor program with an `initialize` instruction.

### 6. Build and Deploy Program

Build the program:
```bash
anchor build
```

Deploy to your local cluster (as configured in `Anchor.toml`):
```bash
anchor deploy
```

The compiled program is located in `target/deploy/`, and Anchor will deploy it to the cluster specified in your `Anchor.toml` configuration (Localnet).

After deployment, note the program ID from the output - you'll need it for creating your Substreams module.

### 7. Verify Deployment

```bash
# Check program account
solana program show <PROGRAM_ID>

# Check if program is deployed
solana account <PROGRAM_ID>
```

{% hint style="info" %}
Save your deployed program ID - you'll need it for the Substreams configuration.
{% endhint %}

## Create Substreams Module

### 1. Initialize Substreams Project

Initialize a new Substreams project with the Solana Anchor template:

```bash
substreams init
```

When prompted during initialization, provide:
- **Protocol**: Solana
- **Template**: `sol-anchor-beta` (Solana Anchor program template)
- **Program ID**: Your deployed Counter program ID

The `substreams init` command generates all necessary files including:
- `substreams.yaml` manifest with Solana block configuration
- Proto schemas for your program
- Rust handler code structure
- Build configuration

### 2. Build and Test Substreams

```bash
cd substreams
substreams build
substreams run -e localhost:9000 --plaintext counter-v0.1.0.spkg -s <DEPLOYMENT_BLOCK> -t +10
```

{% hint style="note" %}
Look for the deployment block in your transaction output - it's the block that will contain actual program data. Use `-s <DEPLOYMENT_BLOCK> -t +10` to scan a specific range of blocks.
{% endhint %}

## Troubleshooting

For common issues with Docker Compose, RPC connectivity, and Substreams, see the [Local Development Troubleshooting](../../generic/local-development/troubleshooting.md) guide.

## Next Steps

Now that you have a working local Solana development environment:

1. **Try Other Platforms** - Explore [HardHat](../../evm/local-development/hardhat.md) or [Foundry](../../evm/local-development/foundry.md) local development
2. **Advanced Substreams** - Learn about [stores, modules, and data transformations](../../generic/using-rust-proto.md)
3. **Consuming Substreams** - Connect to [databases](../../../sinks/sql/sql.md) or [streaming platforms](../../../sinks/stream/stream.md)
4. **Production Deployment** - Move to [production endpoints](../../../../references/chains-and-endpoints.md)

## Additional Resources

- [Anchor Documentation](https://www.anchor-lang.com/docs)
- [Substreams Solana Reference](https://github.com/streamingfast/substreams-solana)
- [Substreams CLI Reference](../../../../references/cli/command-line-interface.md)
- [Creating Protobuf Schemas](../../generic/creating-protobuf-schemas.md)
