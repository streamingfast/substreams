# Published Packages

The Substreams ecosystem includes a growing registry of community-contributed packages available at [substreams.dev](https://substreams.dev). These packages provide ready-to-use Substreams modules for a wide variety of blockchain use cases, from DeFi protocols to NFT marketplaces, gaming applications, and infrastructure tools.

## What Are Published Packages?

Published packages are complete Substreams modules that have been packaged and shared by the community through the Substreams registry. These packages offer:

- **Community-driven development**: Built and maintained by developers worldwide
- **Diverse use case coverage**: Spanning DeFi, NFTs, gaming, infrastructure, and more
- **Easy discovery and integration**: Searchable registry with detailed package information
- **Collaborative improvement**: Open-source packages that benefit from community contributions

## Discovering Packages

### Using the Substreams Registry

The [substreams.dev](https://substreams.dev) website provides several ways to discover packages:

#### Search and Browse
- **Search by name**: Find packages by searching for specific protocols or use cases
- **Browse by category**: Explore packages organized by blockchain, protocol type, or functionality
- **Filter by chain**: Focus on packages for specific blockchains like Ethereum, Solana, or Cosmos

#### Sorting Options
- **Most downloaded**: Find the most popular and trusted packages
- **Recently uploaded**: Discover the latest additions to the registry
- **By contributor**: Explore packages from specific developers or organizations

#### Package Information
Each package listing includes:
- **Version history**: Track package updates and changes
- **Download statistics**: See how widely used a package is
- **Documentation**: Access usage instructions and examples
- **Source code links**: Review the implementation on GitHub

### Top Contributors

The registry features a contributor leaderboard showcasing the most active package publishers:

- **StreamingFast**: Core foundational modules and infrastructure packages
- **TopLedger**: Comprehensive DeFi and trading analytics modules
- **Pinax Network**: Multi-chain data extraction and transformation tools
- **Community developers**: Individual contributors building specialized modules

## Popular Package Categories

### DeFi Protocols
- **Uniswap V2/V3**: Swap events, liquidity changes, and pool analytics
- **Aave**: Lending, borrowing, and liquidation events
- **Compound**: Interest rate changes and market activities
- **Curve**: Pool swaps and liquidity provider actions
- **Balancer**: Weighted pool operations and governance

### NFT and Gaming
- **OpenSea**: NFT marketplace transactions and metadata
- **Axie Infinity**: Game-specific events and token transfers
- **The Sandbox**: Virtual land transactions and asset movements
- **CryptoPunks**: Historical sales and ownership changes
- **Art Blocks**: Generative art minting and trading

### Infrastructure and Tools
- **ENS (Ethereum Name Service)**: Domain registrations and resolutions
- **Chainlink**: Oracle price feeds and data updates
- **The Graph**: Subgraph deployment and indexing events
- **Gnosis Safe**: Multi-signature wallet operations
- **1inch**: DEX aggregator trades and routing

### Cross-Chain and Layer 2
- **Polygon**: Layer 2 transaction processing and bridge events
- **Arbitrum**: Rollup transactions and state updates
- **Optimism**: Optimistic rollup data and fraud proofs
- **IBC**: Inter-blockchain communication for Cosmos ecosystem

## Using Published Packages

### Installation

To use a published package in your Substreams project:

1. **Find the package** on [substreams.dev](https://substreams.dev)
2. **Copy the package URL** from the package details page
3. **Add it to your imports** in `substreams.yaml`:

```yaml
imports:
  uniswap_v3: uniswap_v3@v0.2.10
```

### Integration Example

```yaml
modules:
  - name: my_dex_analytics
    kind: map
    inputs:
      - map: uniswap_v3:pool_swaps
      - map: uniswap_v3:pool_created
    output:
      type: proto:my.analytics.DexData
```

### Rust Implementation

```rust
use substreams::prelude::*;
use uniswap_v3::pb::uniswap::v1::{Events, Pool};

#[substreams::handlers::map]
fn process_dex_data(
    events: Events,
) -> Result<DexData, substreams::errors::Error> {
    let mut data = DexData::default();

    // Process Uniswap events
    for event in events.pool_events {
        match event.r#type.as_str() {
            "SWAP" => {
                data.volume_usd += event.amount_usd;
                data.transaction_count += 1;
            }
            "POOL_CREATED" => {
                data.new_pools.push(event.pool_address.clone());
            }
            _ => {}
        }
    }

    Ok(data)
}
```

## Package Quality and Trust

### Evaluation Criteria

When selecting packages, consider:

- **Download statistics**: Higher download counts often indicate reliability
- **Contributor reputation**: Packages from established contributors tend to be well-maintained
- **Documentation quality**: Well-documented packages are easier to integrate and debug
- **Update frequency**: Regularly updated packages are more likely to be compatible with latest changes
- **Community feedback**: Check GitHub issues and discussions for user experiences

### Best Practices

- **Version pinning**: Use specific versions in your imports to ensure reproducibility
- **Testing**: Thoroughly test packages in your development environment
- **Monitoring**: Keep track of package updates and security advisories
- **Fallback plans**: Have alternatives ready in case a package becomes unavailable

## Contributing Packages

### Publishing Your Own Package

To contribute to the ecosystem:

1. **Develop your Substreams**: Create a useful, well-tested module
2. **Package it**: Use the Substreams CLI to create a `.spkg` file
3. **Publish to registry**: Upload your package to the Substreams registry
4. **Document thoroughly**: Provide clear usage instructions and examples
5. **Maintain actively**: Respond to issues and keep the package updated

### Package Guidelines

- **Clear naming**: Use descriptive names that indicate the package's purpose
- **Comprehensive documentation**: Include usage examples and API documentation
- **Semantic versioning**: Follow semantic versioning for releases
- **License clarity**: Specify the license for your package
- **Community engagement**: Respond to user feedback and contributions

## Advanced Usage Patterns

### Package Composition

Combine multiple packages for complex analytics:

```yaml
imports:
  ethereum_common: ethereum_common@v0.3.3
  uniswap_v3: uniswap_v3@v0.2.10
  aave_v2: aave-v2@v0.1.0

modules:
  - name: defi_analytics
    kind: map
    inputs:
      - map: ethereum_common:filtered_logs
      - map: uniswap_v3:pool_events
      - map: aave_v2:lending_events
    output:
      type: proto:defi.Analytics
```

## Next Steps

- Explore the [Substreams Registry](https://substreams.dev) to discover available packages
- Learn about [Foundational Modules](foundational-modules.md) for core blockchain data processing
- Check out [Foundational Stores](foundational-stores.md) for pre-computed historical data
- Read the guide on [Publishing a Substreams Package](../publish-package.md) to contribute your own modules
