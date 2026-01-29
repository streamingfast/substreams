# Agent Skills for Substreams

AI coding assistants can be enhanced with specialized Substreams expertise through agent skills. These open-source knowledge packages give AI assistants deep understanding of Substreams development patterns, best practices, and debugging techniques.

## Available Skills

### Substreams Development (`substreams-dev`)

Expert knowledge for developing, building, and debugging Substreams projects on any blockchain:

- Creating and configuring `substreams.yaml` manifests
- Writing efficient Rust modules (map, store, index types)
- Protobuf schema design and code generation
- Performance optimization and avoiding excessive cloning
- Debugging and troubleshooting common issues

### Substreams SQL (`substreams-sql`)

Expert knowledge for building SQL database sinks from Substreams data:

- **Database Changes (CDC)** - Stream individual row changes for real-time consistency
- **Relational Mappings** - Transform data into normalized tables with proper relationships
- **PostgreSQL** - Advanced patterns, indexing strategies, and performance optimization
- **ClickHouse** - Analytics-optimized schemas, materialized views, and time-series patterns
- **Schema Design** - Best practices for blockchain data modeling

### Substreams Testing (`substreams-testing`)

Expert knowledge for testing Substreams applications at all levels:

- **Unit Testing** - Testing individual functions with real blockchain data
- **Integration Testing** - End-to-end workflows with real block processing
- **Performance Testing** - Benchmarking, memory profiling, and production mode validation
- **FireCore Tools** - Using Firehose, StreamingFast API, and testing utilities
- **CI/CD Integration** - Automated testing pipelines and regression detection

## Installation

### Claude Code

Install the plugin from the marketplace:

```bash
claude plugin marketplace add https://github.com/streamingfast/substreams-skills
```

Then enable the skills:

1. Run `/plugin` to open the plugin manager
2. Go to the **Discover** tab
3. Find and install the desired skills (e.g., `substreams-dev`, `substreams-sql`)
4. Restart Claude instance(s) for skills to be discovered

After installation, Claude automatically uses Substreams expertise when working on relevant projects.

**Alternative: Local Development**

Clone and load directly without installing from the marketplace:

```bash
git clone https://github.com/streamingfast/substreams-skills.git
claude --plugin-dir ./substreams-skills
```

### Cursor

Clone the repository and add the skill directory path in Cursor settings:

```
~/substreams-skills/skills/substreams-dev
```

### VS Code

VS Code 1.107+ supports Claude Skills as an experimental feature:

1. Enable the experimental feature in settings
2. Add skill paths to your configuration
3. Skills will be available to Claude in VS Code

See the [VS Code 1.107 release notes](https://code.visualstudio.com/updates/v1_107#_reuse-your-claude-skills-experimental) for details.

## Resources

- [Substreams Skills Repository](https://github.com/streamingfast/substreams-skills)
- [Claude Code Plugins Documentation](https://docs.anthropic.com/en/docs/claude-code/plugins)
