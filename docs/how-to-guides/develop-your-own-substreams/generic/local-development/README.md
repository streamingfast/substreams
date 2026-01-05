# Local Development

Developing Substreams locally provides several advantages over using production endpoints:

- **Faster iteration cycles** - No network latency or rate limits
- **Complete control** - Customize blockchain state and transactions
- **Cost-effective** - No API usage fees during development
- **Reproducible testing** - Consistent blockchain state for testing
- **Offline development** - Work without internet connectivity

This section provides complete local development environments for different blockchain platforms, each including:

- Local blockchain node (development mode)
- Firehose integration for block streaming
- Example smart contracts/programs
- Complete Substreams modules
- Docker Compose orchestration

## Available Platforms

### Ethereum Development

- **[Ethereum with HardHat](ethereum-hardhat.md)** - Complete Ethereum development environment using HardHat for contract deployment and testing
- **[Ethereum with Foundry](ethereum-foundry.md)** - Alternative Ethereum setup using Foundry toolkit for faster compilation and deployment

### Solana Development

- **[Solana with Anchor](solana.md)** - Complete Solana development environment using Anchor framework for program development

## Prerequisites

Before starting with any platform, ensure you have:

- **Docker 20.10+** with Docker Compose v2.0+
- **4GB RAM** and **20GB disk space** available
- **Substreams CLI** v1.7.0+ ([installation guide](../../cli/installing-the-cli.md))
- **Rust** with `wasm32-unknown-unknown` target
- Platform-specific tools (detailed in each guide)

## When to Use Local vs Production

**Use local development when:**
- Building and testing new Substreams modules
- Learning Substreams concepts
- Developing with custom smart contracts
- Need reproducible test scenarios
- Working offline or with limited connectivity

**Use production endpoints when:**
- Consuming real blockchain data
- Building production applications
- Need access to historical data
- Require high availability and reliability

## Getting Started

Choose your preferred blockchain platform from the guides above. Each guide provides:

1. **15-25 minute** complete setup time
2. **Step-by-step instructions** with validation commands
3. **Working examples** you can modify and extend
4. **Troubleshooting sections** for common issues
5. **Next steps** for advanced development

{% hint style="warning" %}
These Docker configurations are for local development only - never use in production environments.
{% endhint %}

## Additional Resources

- [Substreams CLI Reference](../../../cli/command-line-interface.md)
- [Creating Protobuf Schemas](../creating-protobuf-schemas.md)
- [Dev Container Reference](../../../../references/devcontainer-ref.md)
