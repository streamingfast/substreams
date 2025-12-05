# Composing Substreams

Substreams are designed with composability at their core, allowing developers to build upon existing modules and data sources to create powerful, interconnected data processing pipelines. This composability enables you to leverage pre-built components, reducing development time and ensuring consistency across your blockchain data infrastructure.

## Understanding Substreams Composability

Composability in Substreams means that you can:

- **Reuse existing modules**: Build upon proven, tested modules rather than starting from scratch
- **Chain data transformations**: Connect outputs from one module as inputs to another
- **Share common functionality**: Leverage shared libraries and utilities across different projects
- **Create modular architectures**: Design systems where components can be easily swapped or upgraded

## How Composition Can Be Leveraged

Substreams composition allows you to:

1. **Accelerate Development**: Start with foundational modules that handle common blockchain data patterns
2. **Ensure Data Consistency**: Use standardized modules that provide consistent data formats across different use cases
3. **Reduce Maintenance Overhead**: Benefit from community-maintained modules that are regularly updated and optimized
4. **Focus on Business Logic**: Spend more time on your unique value proposition rather than basic data extraction

## Three Main Sources for Composition

Substreams offers three primary sources for composable modules and data:

### 1. Foundational Modules
Pre-built modules that provide common blockchain data transformations and extractions. These modules handle standard patterns like event filtering, transaction parsing, and data normalization across different blockchain networks.

**Key Benefits:**
- Pre-transformed blockchain models
- Standardized data formats
- Optimized performance
- Multi-chain support

### 2. Foundational Stores
Stateful, pre-made datasets that provide historical blockchain data in an easily queryable format. These stores are particularly useful when you need to access large amounts of historical data or when store size limits become a constraint.

**Key Benefits:**
- Pre-computed historical data
- Efficient data access patterns
- Alternative when store limits are reached
- Reduced computational overhead

### 3. Published Packages
A growing ecosystem of community-contributed Substreams packages available through the Substreams registry. These packages cover a wide range of use cases, from DeFi protocols to NFT marketplaces.

**Key Benefits:**
- Community-driven development
- Diverse use case coverage
- Easy discovery and integration
- Collaborative improvement

## Getting Started with Composition

To begin composing Substreams:

1. **Identify Your Data Needs**: Determine what blockchain data you need to process
2. **Explore Available Components**: Check foundational modules, stores, and published packages
3. **Design Your Pipeline**: Plan how to connect different components to achieve your goals
4. **Implement and Test**: Build your composed Substreams and validate the results
5. **Contribute Back**: Consider publishing your own modules for the community

The following sections will dive deeper into each composition source, providing practical examples and implementation guidance.
