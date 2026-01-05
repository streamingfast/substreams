# Agent Skills Implementation Plan for Substreams

**Status:** Ready for Implementation
**Created:** 2025-12-20
**Repository:** `streamingfast/substreams-skills` (NEW repository to create)
**Branch Strategy:** Feature branch `feature/skills` with incremental PRs
**Goal:** Create Agent Skills that package Substreams domain expertise for AI assistants.

## Background

Agent Skills are an open format (agentskills.io) for packaging instructions, scripts, and resources that AI agents can load dynamically. They consist of:
- A folder with a `SKILL.md` file containing YAML frontmatter and instructions
- Optional directories for scripts, references, and assets
- Progressive disclosure: metadata (~100 tokens) loaded at startup, instructions (<5000 tokens) loaded on activation, resources on-demand

Skills are supported by Claude Code, Cursor, VS Code, and other AI development tools.

## Implementation Strategy

This plan is designed for a remote coding agent to implement incrementally through **4 Pull Requests** in a **NEW repository**:

1. **PR #1: Repository Setup & Base Architecture** - Complete skills repository structure with validation
2. **PR #2: First Skill** - Substreams Development skill as reference implementation
3. **PR #3: Remaining Skills** - SQL and Testing skills
4. **PR #4: Documentation** - Usage guides, examples, and contribution guidelines

Each PR targets the `feature/skills` branch in the new repository and builds upon the previous.

---

## STEP 0: Repository & Branch Setup

### Prerequisites

**IMPORTANT:** The repository `streamingfast/substreams-skills` will be pre-created by the maintainer with:
- Apache 2.0 License
- Empty repository (no initial files)

### Agent Instructions: Clone and Setup

Once the repository exists, clone and set up your working branch:

```bash
# Clone the empty repository
git clone https://github.com/streamingfast/substreams-skills.git
cd substreams-skills

# Create and push feature branch
git checkout -b feature/skills
git push -u origin feature/skills
```

All subsequent PRs will target `feature/skills` for merge.

---

## PR #1: Repository Setup & Base Architecture

**Branch:** `feature/skills-base-setup`
**Target:** `feature/skills`
**Goal:** Complete repository structure with validation and CI/CD

### Acceptance Criteria

✅ Repository structure is complete
✅ README.md explains purpose and usage
✅ SKILL_DEVELOPMENT.md guides contributors
✅ Validation script works (`scripts/validate-all.sh`)
✅ GitHub Actions CI validates skills
✅ Example skill validates successfully
✅ Installation instructions are clear

### Repository Structure to Create

```
substreams-skills/
├── README.md                           # Repository overview
├── SKILL_DEVELOPMENT.md                # Contribution guide
├── LICENSE                             # Apache 2.0
├── .gitignore
├── .github/
│   └── workflows/
│       └── validate.yml                # CI: validate all skills
├── scripts/
│   └── validate-all.sh                 # Validation script
├── skills/
│   └── .gitkeep                        # Placeholder (skills added in later PRs)
└── examples/
    ├── using-skills.md                 # How to use these skills
    ├── claude-code-setup.md            # Setup for Claude Code
    └── vscode-setup.md                 # Setup for VS Code
```

### Files to Create

#### 1. README.md

```markdown
# Substreams Skills

Agent Skills for Substreams development - open-source expertise packages for AI assistants.

## What are Agent Skills?

Agent Skills are folders containing instructions and resources that AI assistants can load dynamically to gain expertise in specific domains. These skills follow the open [Agent Skills specification](https://agentskills.io/specification).

## Available Skills

Skills will be added incrementally. Check back soon for:
- **Substreams Development** - Building Substreams projects, manifests, modules
- **Substreams SQL** - SQL database sinks (PostgreSQL, ClickHouse)
- **Substreams Testing** - Testing strategies and best practices

## Installation

### Claude Code

1. Clone this repository:
   ```bash
   git clone https://github.com/streamingfast/substreams-skills.git
   cd substreams-skills
   ```

2. In Claude Code settings, add skill paths:
   ```
   ~/substreams-skills/skills/substreams-dev
   ~/substreams-skills/skills/substreams-sql
   ~/substreams-skills/skills/substreams-testing
   ```

### Cursor

Similar to Claude Code - add skill directory paths in Cursor settings.

### VS Code

VS Code 1.107+ supports Claude Skills (experimental feature). Configure in your VS Code settings:

1. Enable the experimental feature in settings
2. Add skill paths to your configuration
3. Skills will be available to Claude in VS Code

See [VS Code 1.107 release notes](https://code.visualstudio.com/updates/v1_107#_reuse-your-claude-skills-experimental) for details.

## Contributing

See [SKILL_DEVELOPMENT.md](./SKILL_DEVELOPMENT.md) for guidelines on creating new skills.

## Validation

All skills are validated against the Agent Skills specification:

```bash
# Install skills-ref CLI (if not already installed)
npm install -g @anthropic/skills-ref

# Validate all skills
./scripts/validate-all.sh
```

## License

Apache 2.0 - See [LICENSE](./LICENSE)

## Resources

- [Substreams Documentation](https://substreams.streamingfast.io)
- [Agent Skills Specification](https://agentskills.io/specification)
- [StreamingFast Discord](https://discord.gg/streamingfast)
```

#### 2. SKILL_DEVELOPMENT.md

```markdown
# Skill Development Guide

Guidelines for contributing new skills to this repository.

## Skill Format

Each skill is a directory with:
- `SKILL.md`: Main skill file with YAML frontmatter
- `references/`: Supporting documentation (loaded on-demand)

## YAML Frontmatter Requirements

```yaml
---
name: skill-name              # Required: 1-64 chars, lowercase, alphanumeric + hyphens
description: Brief description # Required: 1-1024 chars, when to use this skill
license: Apache-2.0           # Optional but recommended
compatibility:                # Optional: target platforms
  platforms: [claude-code, cursor, vscode, windsurf]
metadata:                     # Optional: additional info
  version: 1.0.0
  author: StreamingFast
  documentation: https://substreams.streamingfast.io
---
```

## Content Guidelines

### Keep it Concise
- **Metadata**: ~100 tokens (loaded at startup)
- **Main content**: <5000 tokens (loaded on activation)
- **References**: No limit (loaded on-demand)

### Structure

1. **Overview**: What this skill does
2. **When to Use**: Clear activation criteria
3. **Core Concepts**: Essential knowledge
4. **Common Workflows**: Step-by-step guides
5. **Examples**: Concrete code samples
6. **Troubleshooting**: Common issues
7. **Resources**: Links to references

### Writing Style

- **Clear and direct**: Avoid fluff
- **Actionable**: Provide specific steps
- **Examples**: Show, don't just tell
- **Progressive**: Basic to advanced
- **Links**: Reference detailed docs for deep dives

## Validation

Before submitting, validate your skill:

```bash
# Validate specific skill
skills-ref validate ./skills/your-skill

# Validate all skills
./scripts/validate-all.sh
```

## Testing

Test your skill with AI tools:
1. Load skill in Claude Code/Cursor
2. Ask questions that should activate it
3. Verify responses use skill knowledge
4. Check references load correctly

## Submitting

1. Create feature branch from `feature/skills`
2. Create your skill in `skills/your-skill/`
3. Add tests if applicable
4. Run validation: `./scripts/validate-all.sh`
5. Submit pull request to `feature/skills` with:
   - Skill description
   - When it should be used
   - Testing notes

## Review Criteria

- [ ] Frontmatter valid and complete
- [ ] Description clearly states when to use
- [ ] Content under 5000 tokens (main SKILL.md)
- [ ] References properly linked
- [ ] Examples are correct and tested
- [ ] Validation passes
- [ ] No duplicate content with existing skills

## Questions?

Open an issue or ask in [StreamingFast Discord](https://discord.gg/streamingfast)
```

#### 3. scripts/validate-all.sh

```bash
#!/bin/bash
set -e

echo "Validating all Substreams Skills..."

# Check if skills-ref is installed
if ! command -v skills-ref &> /dev/null; then
    echo "Error: skills-ref not found. Install with: npm install -g @anthropic/skills-ref"
    exit 1
fi

# Count skills
skill_count=0

# Validate each skill
for skill_dir in skills/*/; do
    if [ -d "$skill_dir" ] && [ -f "${skill_dir}SKILL.md" ]; then
        skill_name=$(basename "$skill_dir")
        echo "Validating $skill_name..."
        skills-ref validate "$skill_dir"
        echo "✓ $skill_name valid"
        ((skill_count++))
    fi
done

if [ $skill_count -eq 0 ]; then
    echo "⚠ No skills found to validate"
    exit 0
fi

echo ""
echo "All $skill_count skill(s) validated successfully!"
```

Make script executable:
```bash
chmod +x scripts/validate-all.sh
```

#### 4. .github/workflows/validate.yml

```yaml
name: Validate Skills

on:
  push:
    branches: [main, develop, feature/skills]
  pull_request:
    branches: [main, develop, feature/skills]

jobs:
  validate:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3

      - name: Setup Node.js
        uses: actions/setup-node@v3
        with:
          node-version: '18'

      - name: Install skills-ref
        run: npm install -g @anthropic/skills-ref

      - name: Validate all skills
        run: ./scripts/validate-all.sh
```

#### 5. .gitignore

```
# OS files
.DS_Store
Thumbs.db

# Editor files
.vscode/
.idea/
*.swp
*.swo
*~

# Logs
*.log

# Node modules (if any)
node_modules/

# Temp files
*.tmp
.cache/
```

#### 6. examples/using-skills.md

```markdown
# Using Substreams Skills

## Installation

1. Clone the repository:
   ```bash
   git clone https://github.com/streamingfast/substreams-skills.git
   ```

2. Configure your AI tool (see setup guides below)

## Claude Code Setup

See [claude-code-setup.md](./claude-code-setup.md) for detailed instructions.

## Example Workflows

### Creating a New Project

**Ask Claude:**
> "Help me create a new Substreams project for tracking USDC transfers on Ethereum"

**Expected behavior:**
- `substreams-dev` skill activates
- Guides through project structure
- Provides manifest template
- Shows Rust code examples

### Building a SQL Sink

**Ask Claude:**
> "How do I create a PostgreSQL sink for my Substreams transfers module?"

**Expected behavior:**
- `substreams-sql` skill activates
- Helps design database schema
- Shows sink implementation code
- Provides deployment instructions

### Testing Modules

**Ask Claude:**
> "How should I test my map_events module?"

**Expected behavior:**
- `substreams-testing` skill activates
- Suggests unit test structure
- Provides test examples
- Recommends CI/CD setup

## Updating Skills

```bash
cd ~/substreams-skills
git pull
```

Skills will auto-reload in your AI tool.
```

#### 7. examples/claude-code-setup.md

```markdown
# Claude Code Setup for Substreams Skills

## Step 1: Clone Repository

```bash
cd ~/
git clone https://github.com/streamingfast/substreams-skills.git
```

## Step 2: Configure Claude Code

1. Open Claude Code settings
2. Navigate to "Skills" section
3. Add skill directories:
   - `~/substreams-skills/skills/substreams-dev`
   - `~/substreams-skills/skills/substreams-sql`
   - `~/substreams-skills/skills/substreams-testing`

## Step 3: Verify Installation

Ask Claude:
> "What Substreams skills are available?"

You should see all available skills listed.

## Step 4: Test a Skill

Ask Claude:
> "How do I create a new Substreams map module?"

The `substreams-dev` skill should activate and provide guidance.

## Updating Skills

```bash
cd ~/substreams-skills
git pull
```

Skills will auto-reload in Claude Code.
```

#### 8. examples/vscode-setup.md

```markdown
# VS Code Setup for Substreams Skills

**Requirements:** VS Code 1.107 or later with experimental Claude Skills feature enabled.

## Step 1: Clone Repository

```bash
cd ~/
git clone https://github.com/streamingfast/substreams-skills.git
```

## Step 2: Enable Experimental Feature

1. Open VS Code settings (File → Preferences → Settings or `Cmd+,` on Mac)
2. Search for "Claude Skills" or navigate to experimental features
3. Enable the Claude Skills experimental feature

## Step 3: Configure Skills

Add skill directories to your VS Code settings (`.vscode/settings.json` or user settings):

```json
{
  "claude.skills": [
    "~/substreams-skills/skills/substreams-dev",
    "~/substreams-skills/skills/substreams-sql",
    "~/substreams-skills/skills/substreams-testing"
  ]
}
```

**Note:** Adjust paths to absolute paths if needed:
```json
{
  "claude.skills": [
    "/Users/yourname/substreams-skills/skills/substreams-dev",
    "/Users/yourname/substreams-skills/skills/substreams-sql",
    "/Users/yourname/substreams-skills/skills/substreams-testing"
  ]
}
```

## Step 4: Verify Installation

Ask Claude in VS Code:
> "What Substreams skills are available?"

You should see all available skills listed.

## Step 5: Test a Skill

Ask Claude:
> "How do I create a new Substreams map module?"

The `substreams-dev` skill should activate and provide guidance.

## Updating Skills

```bash
cd ~/substreams-skills
git pull
```

Reload VS Code window for skills to update.

## Reference

- [VS Code 1.107 Release Notes - Claude Skills](https://code.visualstudio.com/updates/v1_107#_reuse-your-claude-skills-experimental)
```

### PR #1 Checklist

- [ ] Clone repository `streamingfast/substreams-skills`
- [ ] Create `feature/skills` branch and push
- [ ] Create `feature/skills-base-setup` branch from `feature/skills`
- [ ] Add LICENSE file (Apache 2.0)
- [ ] Implement all base files listed above (including vscode-setup.md)
- [ ] Make `validate-all.sh` executable
- [ ] Test validation script (should report 0 skills)
- [ ] Verify GitHub Actions workflow syntax
- [ ] Commit and push
- [ ] Open PR to `feature/skills`
- [ ] Request review

---

## PR #2: First Skill - Substreams Development

**Branch:** `feature/skill-substreams-dev`
**Target:** `feature/skills`
**Prerequisites:** PR #1 merged
**Goal:** Implement first complete skill as reference for remaining skills

### Skill Specification

**Name:** `substreams-dev`

**Description:** Expert knowledge for developing, building, and debugging Substreams projects on any blockchain. Use when working with substreams.yaml manifests, Rust modules, protobuf schemas, or blockchain data processing.

**When to Use:**
- Creating new Substreams projects
- Writing or modifying `substreams.yaml` manifests
- Developing Rust modules for data transformation
- Understanding protobuf schemas and data structures
- Debugging module execution
- Optimizing performance

### Files to Create

```
skills/substreams-dev/
├── SKILL.md                    # Main skill file
└── references/
    ├── manifest-spec.md        # Manifest specification
    ├── module-types.md         # Module types guide
    ├── patterns.md             # Common patterns
    └── networks.md             # Supported networks
```

### Implementation

#### SKILL.md

```yaml
---
name: substreams-dev
description: Expert knowledge for developing, building, and debugging Substreams projects on any blockchain. Use when working with substreams.yaml manifests, Rust modules, protobuf schemas, or blockchain data processing.
license: Apache-2.0
compatibility:
  platforms: [claude-code, cursor, vscode, windsurf]
metadata:
  version: 1.0.0
  author: StreamingFast
  documentation: https://substreams.streamingfast.io
---

# Substreams Development Expert

Expert assistant for building Substreams projects - high-performance blockchain data indexing and transformation.

## Core Concepts

### What is Substreams?

Substreams is a powerful blockchain indexing technology that enables:
- **Parallel processing** of blockchain data with high performance
- **Composable modules** written in Rust (map, store, index types)
- **Protobuf schemas** for typed data structures
- **Streaming-first** architecture with cursor-based reorg handling

### Key Components

1. **Manifest** (`substreams.yaml`): Defines modules, networks, dependencies
2. **Modules**: Map (transform), Store (aggregate), Index (filter)
3. **Protobuf**: Type-safe schemas for inputs and outputs
4. **WASM**: Rust code compiled to WebAssembly for execution

## Project Structure

```
my-substreams/
├── substreams.yaml          # Manifest
├── proto/
│   └── events.proto         # Schema definitions
├── src/
│   └── lib.rs               # Rust module code
├── Cargo.toml               # Rust dependencies
└── build/                   # Generated files (gitignored)
```

## Common Workflows

### Creating a New Project

1. **Initialize**: Use `substreams init` or create manifest manually
2. **Define schema**: Create `.proto` files for your data structures
3. **Implement modules**: Write Rust handlers in `src/lib.rs`
4. **Build**: Run `substreams build` to compile to `.spkg`
5. **Test**: Run `substreams run` with small block range (recommended: 1000 blocks)
6. **Deploy**: Publish to registry or deploy as service

### Module Types

**Map Module** - Transforms input to output
```yaml
- name: map_events
  kind: map
  inputs:
    - source: sf.ethereum.type.v2.Block
  output:
    type: proto:my.types.Events
```

**Store Module** - Aggregates data across blocks
```yaml
- name: store_totals
  kind: store
  updatePolicy: add
  valueType: int64
  inputs:
    - map: map_events
```

**Index Module** - Filters blocks for efficient querying
```yaml
- name: index_transfers
  kind: index
  inputs:
    - map: map_events
  output:
    type: proto:sf.substreams.index.v1.Keys
```

### Debugging Checklist

When modules produce unexpected results:

1. **Validate manifest**: `substreams graph` to visualize dependencies
2. **Test small range**: Run 100-1000 blocks, inspect outputs carefully
3. **Check logs**: Look for WASM panics, protobuf decode errors
4. **Verify schema**: Ensure proto types match expected data
5. **Review inputs**: Confirm input modules produce correct data
6. **Initial block**: Check `initialBlock` is set appropriately

### Performance Optimization

- **Use indexes** to skip irrelevant blocks
- **Minimize store size** by storing only necessary data
- **Production mode** enables parallel execution: `--production-mode`
- **Module granularity**: Smaller, focused modules perform better
- **Avoid deep nesting**: Flatten module dependencies when possible

## Manifest Reference

See [references/manifest-spec.md](./references/manifest-spec.md) for complete specification.

### Key Sections

**Package metadata**:
```yaml
specVersion: v0.1.0
package:
  name: my-substreams
  version: v1.0.0
  doc: Description of what this substreams does
```

**Protobuf imports**:
```yaml
protobuf:
  files:
    - events.proto
  importPaths:
    - ./proto
```

**Binary reference** (WASM code):
```yaml
binaries:
  default:
    type: wasm/rust-v1
    file: ./target/wasm32-unknown-unknown/release/my_substreams.wasm
```

**Network configuration**:
```yaml
network: mainnet
```

Supported networks: See [references/networks.md](./references/networks.md)

## Rust Module Development

### Map Handler Example

```rust
use substreams::prelude::*;
use substreams_ethereum::pb::eth::v2::Block;

#[substreams::handlers::map]
pub fn map_events(block: Block) -> Result<Events, Error> {
    let mut events = Events::default();

    for trx in block.transactions() {
        for log in trx.logs() {
            // Process logs, extract events
            if is_transfer_event(log) {
                events.transfers.push(extract_transfer(log));
            }
        }
    }

    Ok(events)
}
```

### Store Handler Example

```rust
#[substreams::handlers::store]
pub fn store_totals(events: Events, store: StoreAddInt64) {
    for transfer in events.transfers {
        store.add(0, &transfer.token, transfer.amount as i64);
    }
}
```

### Best Practices

- **Handle errors gracefully**: Use `Result<T, Error>` returns
- **Log sparingly**: Excessive logging impacts performance
- **Validate inputs**: Check for null/empty data before processing
- **Use substreams helpers**: Leverage `substreams-ethereum` crate
- **Test locally first**: Always test with `substreams run` before deploying
- **Avoid excessive cloning**: Use ownership transfer (see Performance section below)

## Performance: Avoiding Excessive Cloning

**CRITICAL:** One of the greatest performance impacts in Substreams is excessive cloning of data structures.

### The Problem

Cloning large data structures is expensive:
- ❌ **Cloning a Transaction**: Copies all fields, logs, traces
- ❌ **Cloning a Block**: Copies the entire block including all transactions (EXTREMELY expensive)
- ❌ **Cloning in loops**: Multiplies the cost by number of iterations

### The Solution: Ownership Transfer

Use Rust's ownership system to transfer or borrow data instead of cloning.

#### Bad Example (Excessive Cloning)

```rust
#[substreams::handlers::map]
pub fn map_events(block: Block) -> Result<Events, Error> {
    let mut events = Events::default();

    for trx in block.transactions() {
        // ❌ BAD: Cloning entire transaction
        let transaction = trx.clone();

        for log in transaction.receipt.logs {
            // ❌ BAD: Cloning log
            let log_copy = log.clone();
            if is_transfer_event(&log_copy) {
                events.transfers.push(extract_transfer(&log_copy));
            }
        }
    }

    Ok(events)
}
```

#### Good Example (Ownership Transfer)

```rust
#[substreams::handlers::map]
pub fn map_events(block: Block) -> Result<Events, Error> {
    let mut events = Events::default();

    // ✅ GOOD: Iterate by reference
    for trx in block.transactions() {
        // ✅ GOOD: Borrow, don't clone
        for log in &trx.receipt.logs {
            if is_transfer_event(log) {
                // ✅ GOOD: Only extract what you need
                events.transfers.push(extract_transfer(log));
            }
        }
    }

    Ok(events)
}

fn is_transfer_event(log: &Log) -> bool {
    // Use reference, no cloning
    !log.topics.is_empty() &&
    log.topics[0] == TRANSFER_EVENT_SIGNATURE
}

fn extract_transfer(log: &Log) -> Transfer {
    // Extract only the fields you need
    Transfer {
        from: Hex::encode(&log.topics[1]),
        to: Hex::encode(&log.topics[2]),
        amount: Hex::encode(&log.data),
        // Don't copy the entire log
    }
}
```

### When Cloning is Acceptable

Clone only small, necessary data:

```rust
// ✅ OK: Cloning small strings
let token_address = Hex::encode(&log.address).clone();

// ✅ OK: Cloning primitive types
let block_number = block.number.clone();

// ❌ BAD: Cloning entire structures
let block_copy = block.clone(); // Never do this!
let trx_copy = transaction.clone(); // Avoid this!
```

### Performance Tips

1. **Iterate by reference**: Use `&` when iterating
   ```rust
   for log in &trx.receipt.logs { } // Good
   for log in trx.receipt.logs.clone() { } // Bad
   ```

2. **Borrow, don't own**: Pass references to functions
   ```rust
   fn process_log(log: &Log) { } // Good
   fn process_log(log: Log) { } // Bad (takes ownership)
   ```

3. **Extract minimal data**: Only copy what you actually need
   ```rust
   // Good: Extract only needed fields
   let amount = parse_amount(&log.data);

   // Bad: Copy entire log just to get one field
   let log_copy = log.clone();
   let amount = parse_amount(&log_copy.data);
   ```

4. **Use `into()` for consumption**: When you need to consume data
   ```rust
   // When you truly need to take ownership
   events.transfers.push(Transfer {
       from: topics[1].into(), // Consumes the data
       to: topics[2].into(),
   });
   ```

### Common Pitfalls

**Pitfall #1: Cloning in filters**
```rust
// ❌ BAD
block.transactions()
    .iter()
    .filter(|trx| trx.clone().to == target) // Clone every transaction!

// ✅ GOOD
block.transactions()
    .iter()
    .filter(|trx| trx.to == target) // Just compare
```

**Pitfall #2: Unnecessary defensive copies**
```rust
// ❌ BAD
let block_copy = block.clone();
for trx in block_copy.transactions() { } // Why clone the whole block?

// ✅ GOOD
for trx in block.transactions() { } // Use the block directly
```

**Pitfall #3: Cloning for mutation**
```rust
// ❌ BAD
let mut trx_copy = trx.clone();
trx_copy.value = process(trx_copy.value); // Clone just to mutate

// ✅ GOOD
let new_value = process(&trx.value); // Process reference, create new value
```

### Measuring Impact

Use `substreams run` with timing to measure performance:

```bash
# Test with cloning (slow)
time substreams run -s 17000000 -t +1000 map_events

# Test without cloning (fast)
time substreams run -s 17000000 -t +1000 map_events

# You should see significant speedup (2-10x) by avoiding clones
```

### Remember

- 🎯 **Profile before optimizing**: Use `substreams estimate` to identify bottlenecks
- 🎯 **Clone only when necessary**: Most of the time, borrowing is sufficient
- 🎯 **Block cloning is almost never needed**: This is the #1 performance killer
- 🎯 **Transaction cloning should be rare**: Extract only the data you need

## Common Patterns

See [references/patterns.md](./references/patterns.md) for detailed examples:

- Event extraction from logs
- Store aggregation patterns
- Multi-module composition
- Parameterized modules
- Dynamic data sources

## Development Tips

1. **Start small**: Begin with 1000 block range for testing
2. **Use GUI**: `substreams gui` for visual debugging (when available)
3. **Check estimates**: `substreams estimate` before processing large ranges
4. **Version control**: Commit `.spkg` files for reproducibility
5. **Document modules**: Add `doc:` fields in manifest for clarity

## Troubleshooting

**Build fails**:
- Check Rust toolchain: `rustup target add wasm32-unknown-unknown`
- Verify proto imports are correct
- Ensure binary path in manifest matches build output

**Empty output**:
- Confirm `initialBlock` is before first relevant block
- Check module isn't filtered out by upstream index
- Verify input data exists in block range

**Performance issues**:
- Add indexes to skip irrelevant blocks
- Use `--production-mode` for large ranges
- Check store size (use `substreams gui` or estimate)

## Resources

- [Official Documentation](https://substreams.streamingfast.io)
- [Module Types Guide](./references/module-types.md)
- [Manifest Specification](./references/manifest-spec.md)
- [Common Patterns](./references/patterns.md)
- [Supported Networks](./references/networks.md)

## Getting Help

- [Discord Community](https://discord.gg/streamingfast)
- [GitHub Issues](https://github.com/streamingfast/substreams/issues)
- [Documentation](https://substreams.streamingfast.io)
```

#### Reference Files

Create reference files with comprehensive content (examples below, expand as needed):

**references/manifest-spec.md** - Complete manifest YAML specification with all fields documented

**references/module-types.md** - Deep dive on map, store, and index modules with examples

**references/patterns.md** - Common development patterns with code examples

**references/networks.md** - List of supported networks (mainnet, polygon, arbitrum, etc.)

### PR #2 Checklist

- [ ] Create `feature/skill-substreams-dev` branch from `feature/skills`
- [ ] Create `skills/substreams-dev/` directory
- [ ] Implement `SKILL.md` with complete content (<5000 tokens)
- [ ] Create all reference files in `references/` directory
- [ ] Validate skill: `skills-ref validate skills/substreams-dev`
- [ ] Test skill in Claude Code/Cursor
- [ ] Verify skill activates on relevant queries
- [ ] Update repository README if needed
- [ ] Commit and push
- [ ] Open PR to `feature/skills`
- [ ] Request review

---

## PR #3: Remaining Skills

**Branch:** `feature/remaining-skills`
**Target:** `feature/skills`
**Prerequisites:** PR #2 merged and reviewed
**Goal:** Implement remaining 2 skills based on approved pattern

### Skills to Implement

1. **Substreams SQL** (`substreams-sql`)
2. **Substreams Testing** (`substreams-testing`)

---

### Skill 2: Substreams SQL (`substreams-sql`)

**Location:** `skills/substreams-sql/`

**Description:** Expert knowledge for building SQL database sinks with Substreams. Use when working with PostgreSQL or ClickHouse sinks, schema design, or database optimization for blockchain data.

**Key Content Areas:**

Based on these resources:
- https://github.com/streamingfast/substreams-sink-sql/blob/develop/README.md
- https://github.com/streamingfast/substreams-sink-clickhouse-showcase/blob/main/README.md
- https://github.com/streamingfast/substreams-sink-clickhouse-showcase/blob/main/DEEP_DIVE.md

**IMPORTANT: Two Input Format Approaches**

1. **Database Changes (CDC-style)**
   - Similar to Change Data Capture
   - Define operations: INSERT, UPDATE, DELETE
   - Used by `substreams-sink-sql` and `substreams-sink-clickhouse`

2. **Relational Mappings (Protobuf extraction)**
   - Direct extraction from Protobuf to relational tables
   - Schema defined in SQL
   - Mapping configuration in YAML

#### SKILL.md Structure

```yaml
---
name: substreams-sql
description: Expert knowledge for building SQL database sinks with Substreams. Use when working with PostgreSQL or ClickHouse sinks, schema design, or database optimization for blockchain data.
license: Apache-2.0
compatibility:
  platforms: [claude-code, cursor, vscode, windsurf]
metadata:
  version: 1.0.0
  author: StreamingFast
  documentation: https://substreams.streamingfast.io
---

# Substreams SQL Sink Expert

Expert assistant for building SQL database sinks with Substreams.

## Overview

Substreams SQL sinks stream blockchain data directly into SQL databases:
- **PostgreSQL**: Full-featured relational database
- **ClickHouse**: High-performance columnar database for analytics

## Two Input Format Approaches

### 1. Database Changes (CDC-Style)

**Use when:** You need fine-grained control over database operations

**How it works:**
- Substreams module outputs `DatabaseChanges` protobuf
- Each change specifies: table, operation (INSERT/UPDATE/DELETE), fields
- Sink applies changes transactionally

**Example module output:**
```rust
use substreams_database_change::pb::database::DatabaseChanges;

#[substreams::handlers::map]
pub fn db_out(events: Events) -> Result<DatabaseChanges, Error> {
    let mut changes = DatabaseChanges::default();

    for transfer in events.transfers {
        changes.push_change(
            "transfers",                           // table
            vec![("tx_hash", &transfer.tx_hash)], // primary key
            0,                                     // ordinal
            Operation::Create,
            vec![
                ("block_num", transfer.block_num.to_string()),
                ("from_addr", transfer.from_addr),
                ("to_addr", transfer.to_addr),
                ("amount", transfer.amount),
            ],
        );
    }

    Ok(changes)
}
```

**Schema (PostgreSQL):**
```sql
CREATE TABLE transfers (
    tx_hash TEXT PRIMARY KEY,
    block_num BIGINT NOT NULL,
    from_addr TEXT NOT NULL,
    to_addr TEXT NOT NULL,
    amount NUMERIC(78, 0) NOT NULL
);
```

**Running the sink:**
```bash
substreams-sink-postgres run \
  "postgres://user:pass@localhost/mydb" \
  "mainnet.eth.streamingfast.io:443" \
  my-substreams.spkg \
  db_out
```

### 2. Relational Mappings (Protobuf Extraction)

**Use when:** Simple 1:1 mapping from Protobuf to tables

**How it works:**
- Define SQL schema
- Create YAML mapping configuration
- Sink extracts fields automatically from Protobuf

**Protobuf definition:**
```protobuf
message Transfers {
  repeated Transfer items = 1;
}

message Transfer {
  uint64 block_num = 1;
  string tx_hash = 2;
  string from_addr = 3;
  string to_addr = 4;
  string amount = 5;
}
```

**Mapping YAML:**
```yaml
tables:
  transfers:
    message: Transfer
    primary_key: tx_hash
    fields:
      - name: block_num
        type: uint64
      - name: tx_hash
        type: string
      - name: from_addr
        type: string
      - name: to_addr
        type: string
      - name: amount
        type: string
```

**Schema (same as above)**

**Running:**
```bash
substreams-sink-sql run \
  --mapping mapping.yaml \
  "postgres://user:pass@localhost/mydb" \
  "mainnet.eth.streamingfast.io:443" \
  my-substreams.spkg \
  map_transfers
```

## PostgreSQL vs ClickHouse

### PostgreSQL
- **Best for:** Transactional workloads, relational queries
- **Features:** ACID compliance, complex joins, constraints
- **Schema:** Standard SQL DDL

### ClickHouse
- **Best for:** Analytics, time-series, large-scale aggregations
- **Features:** Columnar storage, fast aggregations, compression
- **Schema:** ClickHouse-specific syntax

**ClickHouse Example:**
```sql
CREATE TABLE transfers (
    block_num UInt64,
    block_time DateTime,
    tx_hash String,
    from_addr String,
    to_addr String,
    amount UInt256
) ENGINE = MergeTree()
ORDER BY (block_num, tx_hash);
```

## Schema Design Best Practices

- **Include block metadata**: `block_num`, `block_time`, `tx_hash` for traceability
- **Index frequently queried fields**: addresses, timestamps, identifiers
- **Use appropriate types**: NUMERIC for large numbers in PostgreSQL, UInt256 in ClickHouse
- **Consider partitioning**: For very large datasets (by date, block range)
- **Plan for reorgs**: Use cursor tables for proper rollback handling

## Performance Optimization

### Database Tuning

**PostgreSQL**:
- Increase `shared_buffers` for large workloads
- Tune `work_mem` for complex queries
- Use connection pooling (pgBouncer)
- Consider `UNLOGGED` tables for non-critical data

**ClickHouse**:
- Choose optimal `ORDER BY` columns (most to least selective)
- Use `ReplacingMergeTree` for deduplication
- Partition by time for easier management
- Tune `max_insert_block_size`
- Use Materialized Views for real-time aggregations (see below)

### Sink Configuration

- **Batch size**: Larger batches = better throughput, higher latency
- **Flush interval**: Balance between freshness and performance
- **Parallel workers**: Multiple sink instances for horizontal scaling

## ClickHouse Materialized Views

**Materialized Views** in ClickHouse enable real-time aggregations and transformations as data is inserted.

### Use Cases

- **Real-time aggregations**: Calculate totals, averages, counts on the fly
- **Pre-computed analytics**: Speed up complex queries
- **Data transformations**: Reshape data for different query patterns
- **Time-series rollups**: Hourly/daily aggregates from block-level data

### Basic Example: Transfer Volume by Token

**Base table** (raw transfers):
```sql
CREATE TABLE transfers (
    block_num UInt64,
    block_time DateTime,
    tx_hash String,
    token_address String,
    from_addr String,
    to_addr String,
    amount UInt256
) ENGINE = MergeTree()
ORDER BY (block_num, tx_hash);
```

**Aggregation table** (stores hourly volumes):
```sql
CREATE TABLE transfer_volume_hourly (
    hour DateTime,
    token_address String,
    transfer_count UInt64,
    total_volume UInt256,
    unique_senders AggregateFunction(uniq, String),
    unique_receivers AggregateFunction(uniq, String)
) ENGINE = AggregatingMergeTree()
ORDER BY (token_address, hour);
```

**Materialized View** (automatically populates aggregation table):
```sql
CREATE MATERIALIZED VIEW transfer_volume_hourly_mv
TO transfer_volume_hourly
AS SELECT
    toStartOfHour(block_time) AS hour,
    token_address,
    count() AS transfer_count,
    sum(amount) AS total_volume,
    uniqState(from_addr) AS unique_senders,
    uniqState(to_addr) AS unique_receivers
FROM transfers
GROUP BY hour, token_address;
```

**Query aggregated data:**
```sql
-- Get hourly volumes for a specific token
SELECT
    hour,
    transfer_count,
    total_volume,
    uniqMerge(unique_senders) AS unique_senders,
    uniqMerge(unique_receivers) AS unique_receivers
FROM transfer_volume_hourly
WHERE token_address = '0x...'
  AND hour >= now() - INTERVAL 24 HOUR
ORDER BY hour;
```

### Advanced Example: Multi-Level Aggregations

**Daily aggregates from hourly data:**
```sql
CREATE TABLE transfer_volume_daily (
    date Date,
    token_address String,
    transfer_count UInt64,
    total_volume UInt256
) ENGINE = SummingMergeTree()
ORDER BY (token_address, date);

CREATE MATERIALIZED VIEW transfer_volume_daily_mv
TO transfer_volume_daily
AS SELECT
    toDate(hour) AS date,
    token_address,
    transfer_count,
    total_volume
FROM transfer_volume_hourly;
```

### Best Practices

1. **Order columns wisely**: Most selective first in `ORDER BY`
   ```sql
   ORDER BY (token_address, hour) -- token_address is more selective
   ```

2. **Use appropriate engines**:
   - `AggregatingMergeTree`: For complex aggregations with AggregateFunction
   - `SummingMergeTree`: For simple sums
   - `ReplacingMergeTree`: For deduplication

3. **Partition large tables**:
   ```sql
   ENGINE = MergeTree()
   PARTITION BY toYYYYMM(block_time)
   ORDER BY (block_num, tx_hash)
   ```

4. **Create views AFTER base table has data**:
   ```sql
   -- First: Load historical data into base table
   -- Then: Create materialized view
   -- View only processes NEW inserts, not existing data
   ```

5. **Populate historical aggregates manually**:
   ```sql
   -- Insert historical data into aggregation table
   INSERT INTO transfer_volume_hourly
   SELECT
       toStartOfHour(block_time) AS hour,
       token_address,
       count() AS transfer_count,
       sum(amount) AS total_volume,
       uniqState(from_addr) AS unique_senders,
       uniqState(to_addr) AS unique_receivers
   FROM transfers
   WHERE block_time < now() -- Only historical data
   GROUP BY hour, token_address;
   ```

### Common Patterns

**Top N by volume:**
```sql
CREATE MATERIALIZED VIEW top_tokens_hourly_mv
TO top_tokens_hourly
AS SELECT
    toStartOfHour(block_time) AS hour,
    token_address,
    sum(amount) AS volume
FROM transfers
GROUP BY hour, token_address
ORDER BY volume DESC
LIMIT 100 BY hour; -- Top 100 per hour
```

**Moving averages:**
```sql
SELECT
    hour,
    token_address,
    total_volume,
    avg(total_volume) OVER (
        PARTITION BY token_address
        ORDER BY hour
        ROWS BETWEEN 23 PRECEDING AND CURRENT ROW
    ) AS moving_avg_24h
FROM transfer_volume_hourly
WHERE token_address = '0x...';
```

**Percentage change:**
```sql
SELECT
    hour,
    token_address,
    total_volume,
    (total_volume - lagInFrame(total_volume, 1) OVER (
        PARTITION BY token_address ORDER BY hour
    )) / lagInFrame(total_volume, 1) OVER (
        PARTITION BY token_address ORDER BY hour
    ) * 100 AS pct_change
FROM transfer_volume_hourly;
```

### Performance Considerations

- **Materialized Views add write overhead**: Each insert processes the view
- **Trade-off**: Slower writes, much faster reads
- **Monitor view performance**: Check `system.query_log` for slow views
- **Optimize view queries**: Simple aggregations perform best
- **Consider data volume**: Views work best with high read/write ratio

### Debugging Materialized Views

**Check view status:**
```sql
SELECT * FROM system.tables
WHERE engine LIKE '%MaterializedView%';
```

**View data flow:**
```sql
SELECT * FROM system.query_log
WHERE query LIKE '%transfer_volume_hourly_mv%'
ORDER BY event_time DESC
LIMIT 10;
```

**Verify aggregations:**
```sql
-- Compare raw vs aggregated counts
SELECT count() FROM transfers WHERE toStartOfHour(block_time) = '2024-01-01 12:00:00';
SELECT transfer_count FROM transfer_volume_hourly WHERE hour = '2024-01-01 12:00:00';
```

## Troubleshooting

**Slow ingestion**:
- Check database indexes
- Increase batch size
- Profile slow queries
- Consider sharding/partitioning

**Reorg handling issues**:
- Verify cursor table is working
- Check for unique constraint violations
- Ensure proper transaction handling

**Data type errors**:
- Match protobuf types to SQL types carefully
- Handle NULL values explicitly
- Use appropriate precision for large numbers

## Resources

- [substreams-sink-sql Documentation](https://github.com/streamingfast/substreams-sink-sql)
- [ClickHouse Showcase](https://github.com/streamingfast/substreams-sink-clickhouse-showcase)
- [Schema Design Guide](./references/schema-design.md)
- [ClickHouse Deep Dive](./references/clickhouse-guide.md)
```

**Reference files:**
- `references/schema-design.md` - Database schema design patterns
- `references/clickhouse-guide.md` - ClickHouse-specific optimization

---

### Skill 3: Substreams Testing (`substreams-testing`)

**Location:** `skills/substreams-testing/`

**Description:** Expert knowledge for testing Substreams modules, including unit tests, integration tests, and validation strategies. Use when testing Rust handlers, validating outputs, or setting up CI/CD.

**Key Content Areas:**

**IMPORTANT Additions:**

1. **`firecore tools` for data exploration**
   - Extract full blockchain data
   - Scan through data using `jq`
   - Common workflow at StreamingFast for digging into data

2. **Performance Testing**
   - Always test in production mode
   - **Note:** Production mode has caching, which can skew pure performance results
   - Mention strategies to account for caching

#### SKILL.md Structure

```yaml
---
name: substreams-testing
description: Expert knowledge for testing Substreams modules, including unit tests, integration tests, and validation strategies. Use when testing Rust handlers, validating outputs, or setting up CI/CD.
license: Apache-2.0
compatibility:
  platforms: [claude-code, cursor, vscode, windsurf]
metadata:
  version: 1.0.0
  author: StreamingFast
  documentation: https://substreams.streamingfast.io
---

# Substreams Testing Expert

Expert assistant for testing and validating Substreams modules.

## Testing Strategy

### Test Pyramid

1. **Unit Tests** (Rust): Test individual functions and logic
2. **Integration Tests**: Test with real block data
3. **End-to-End Tests**: Validate full pipeline
4. **Data Exploration**: Use `firecore tools` to investigate blockchain data

## Unit Testing in Rust

### Setup

```toml
# Cargo.toml
[dev-dependencies]
substreams-ethereum = "0.9"
prost = "0.11"
```

### Example Test

```rust
#[cfg(test)]
mod tests {
    use super::*;
    use substreams_ethereum::pb::eth::v2::{Block, TransactionTrace, Log};

    #[test]
    fn test_extract_transfer() {
        // Create test block
        let mut block = Block::default();
        block.number = 12345;

        // Add test transaction with transfer event
        let mut trx = TransactionTrace::default();
        let mut log = Log::default();
        log.address = vec![0x12, 0x34]; // Token address
        // ... set up log data
        trx.receipt.logs.push(log);
        block.transaction_traces.push(trx);

        // Test handler
        let result = map_transfers(block).unwrap();

        assert_eq!(result.transfers.len(), 1);
        assert_eq!(result.transfers[0].amount, "1000000000000000000");
    }
}
```

## Integration Testing with Real Blocks

### Using substreams run

```bash
# Test against specific block range
substreams run \
  -e mainnet.eth.streamingfast.io:443 \
  substreams.yaml \
  map_events \
  --start-block 17000000 \
  --stop-block +1000
```

### Automated Validation

```bash
# Run and capture output
substreams run ... -o jsonl > output.jsonl

# Validate with script
python validate_output.py output.jsonl
```

### Golden File Testing

Save expected outputs and compare:

```bash
# Generate golden file (first time)
substreams run ... > golden/block_17000000.json

# Validate against golden file
substreams run ... | diff - golden/block_17000000.json
```

## Data Exploration with firecore tools

**IMPORTANT:** This is a common workflow at StreamingFast for investigating blockchain data.

### Prerequisites

Set your Firehose API token (same token as Substreams):
```bash
export FIREHOSE_API_TOKEN="your-token-here"
# Or alternatively:
export FIREHOSE_API_KEY="your-token-here"

# This is the same token as SUBSTREAMS_API_TOKEN
# Just use FIREHOSE_ prefix instead of SUBSTREAMS_
```

### Extract Block Data

```bash
# Extract full block data to JSON
# Use short network name (mainnet, sepolia, polygon, arbitrum, etc.)

# Extract last block (-1)
firecore tools firehose-client mainnet -o json -- -1 | jq .

# Extract specific block range
firecore tools firehose-client mainnet -o json -- 17000000:17000100 | jq .

# Extract single block
firecore tools firehose-single-block-client mainnet -o json -- 17000000 | jq .

# Save to file
firecore tools firehose-client mainnet -o json -- 17000000:17000100 > blocks.jsonl

# For other networks
firecore tools firehose-client sepolia -o json -- -1 | jq .
firecore tools firehose-client polygon -o json -- 45000000:45000100 | jq .
firecore tools firehose-client arbitrum -o json -- 120000000:120000100 | jq .
```

### Scan with jq

```bash
# Find all Transfer events
cat blocks.jsonl | jq -r '
  .block.transactionTraces[]
  | select(.receipt.logs[])
  | .receipt.logs[]
  | select(.topics[0] == "0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef")
'

# Extract specific field patterns
cat blocks.jsonl | jq -r '
  .block.transactionTraces[].calls[]
  | select(.callType == "CALL")
  | {to: .to, value: .value, input: .input}
'

# Count events by type
cat blocks.jsonl | jq -r '
  .block.transactionTraces[].receipt.logs[].topics[0]
' | sort | uniq -c | sort -rn
```

### Common firecore Patterns

**Extract transactions to specific address:**
```bash
firecore tools firehose-client mainnet -o json -- 17000000:17001000 | jq -r '
  .block.transactionTraces[]
  | select(.to == "0x...")
'
```

**Find contract creations:**
```bash
firecore tools firehose-client mainnet -o json -- 17000000:17001000 | jq -r '
  .block.transactionTraces[]
  | select(.type == "CREATE")
'
```

**Analyze gas usage:**
```bash
firecore tools firehose-client mainnet -o json -- 17000000:17001000 | jq -r '
  .block.transactionTraces[]
  | {hash: .hash, gasUsed: .receipt.gasUsed}
'
```

## Performance Testing

### IMPORTANT: Production Mode Considerations

**Always test in production mode:**
```bash
substreams run --production-mode ...
```

**Caching Impact:**
- Production mode has caching support
- **Can skew pure performance results**
- First run: Measures actual performance
- Subsequent runs: Benefits from cache, faster but not representative

**Strategies to account for caching:**

1. **Clear cache between runs:**
   ```bash
   # Use different output modules or block ranges
   substreams run --production-mode -s 17000000 -t +10000 ...
   substreams run --production-mode -s 18000000 -t +10000 ...
   ```

2. **Test cold start performance:**
   - Use fresh block ranges not previously processed
   - Clear server-side cache if testing infrastructure

3. **Measure with timing:**
   ```bash
   time substreams run --production-mode -s 17000000 -t +10000 ...
   ```

4. **Profile with different settings:**
   ```bash
   # Development mode (no caching)
   time substreams run -s 17000000 -t +1000 ...

   # Production mode (with caching)
   time substreams run --production-mode -s 17000000 -t +10000 ...
   ```

## CI/CD Integration

### GitHub Actions Example

```yaml
name: Test Substreams

on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3

      - name: Install Rust
        uses: actions-rs/toolchain@v1
        with:
          toolchain: stable
          target: wasm32-unknown-unknown

      - name: Install Substreams CLI
        run: |
          curl https://get.substreams.dev | bash

      - name: Run Rust Tests
        run: cargo test

      - name: Build Package
        run: substreams build

      - name: Integration Test
        env:
          SUBSTREAMS_API_TOKEN: ${{ secrets.SUBSTREAMS_API_TOKEN }}
        run: |
          substreams run -e mainnet.eth.streamingfast.io:443 \
            substreams.yaml map_events \
            -s 17000000 -t +100 \
            -o jsonl > output.jsonl

          python tests/validate.py output.jsonl
```

## Validation Strategies

### Data Correctness

- **Compare with known events**: Test against verified transactions
- **Check invariants**: Balances sum to expected totals, etc.
- **Cross-validate**: Compare with other indexers (Etherscan, etc.)

### Regression Testing

- Maintain test cases for bug fixes
- Test edge cases (empty blocks, reverts, etc.)
- Validate reorg handling

## Troubleshooting Tests

**Tests pass locally but fail in CI**:
- Check environment variables (API tokens)
- Verify network connectivity
- Ensure consistent Rust/Substreams versions

**Flaky tests**:
- Add retries for network operations
- Use deterministic test data
- Mock external dependencies

**Slow tests**:
- Use smaller block ranges
- Cache test fixtures
- Parallelize test execution

## Resources

- [Test Strategies Guide](./references/test-strategies.md)
- [firecore tools Documentation](https://github.com/streamingfast/firehose-core)
- [CI/CD Examples](https://github.com/streamingfast/substreams-examples)
```

**Reference files:**
- `references/test-strategies.md` - Comprehensive testing patterns and examples
- `references/firecore-usage.md` - Advanced firecore tools usage

### PR #3 Checklist

- [ ] Create `feature/remaining-skills` branch from `feature/skills`
- [ ] Implement `substreams-sql` skill with all content
  - [ ] Include Database Changes (CDC) approach
  - [ ] Include Relational Mappings approach
  - [ ] PostgreSQL and ClickHouse examples
  - [ ] Reference files
- [ ] Implement `substreams-testing` skill with all content
  - [ ] Include `firecore tools` section
  - [ ] Include performance testing with caching notes
  - [ ] Reference files
- [ ] Validate all skills: `./scripts/validate-all.sh`
- [ ] Test skills in Claude Code/Cursor
- [ ] Verify skills activate appropriately
- [ ] Update repository README (list all 3 skills)
- [ ] Commit and push
- [ ] Open PR to `feature/skills`
- [ ] Request review

---

## PR #4: Documentation & Final Polish

**Branch:** `feature/skills-documentation`
**Target:** `feature/skills`
**Prerequisites:** PR #3 merged
**Goal:** Complete documentation, examples, and final polish

### Files to Update/Create

```
substreams-skills/
├── README.md              # Update with all 3 skills
├── examples/
│   ├── using-skills.md    # Update with all skills
│   ├── cursor-setup.md    # Add Cursor setup guide
│   └── vscode-setup.md    # Already created in PR #1
└── CHANGELOG.md           # Create changelog
```

### Updates Needed

#### 1. README.md

Update to include all 3 skills with descriptions:

```markdown
## Available Skills

### 1. [Substreams Development](./skills/substreams-dev)
Expert knowledge for building Substreams projects, including manifests, Rust modules, and protobuf schemas.

**Use when:** Creating projects, writing modules, debugging, optimizing performance

### 2. [Substreams SQL](./skills/substreams-sql)
Expert knowledge for building SQL database sinks (PostgreSQL, ClickHouse) with two input approaches: Database Changes (CDC) and Relational Mappings.

**Use when:** Designing schemas, implementing sinks, optimizing databases

### 3. [Substreams Testing](./skills/substreams-testing)
Expert knowledge for testing Substreams modules with unit tests, integration tests, and data exploration using firecore tools.

**Use when:** Writing tests, setting up CI/CD, validating outputs, exploring blockchain data
```

#### 2. examples/using-skills.md

Update with examples for all 3 skills and their specific features.

#### 3. examples/cursor-setup.md

Create setup guide for Cursor (similar to Claude Code setup).

#### 4. CHANGELOG.md

```markdown
# Changelog

## v1.0.0 - 2025-12-20

### Added
- Initial release with 3 core skills
- Substreams Development skill
- Substreams SQL skill (Database Changes + Relational Mappings)
- Substreams Testing skill (including firecore tools usage)
- Validation workflow
- Documentation and examples
```

### Integration with Substreams Repository

**IMPORTANT:** While this is a separate repository, create documentation in the main Substreams repo linking to these skills.

Create this file in the main `substreams` repository (separate from this PR):

**Location:** `substreams/docs/ai-tools/agent-skills.md`

```markdown
# Agent Skills for Substreams

Substreams provides Agent Skills for AI-assisted development.

## What are Agent Skills?

Agent Skills are packages of domain expertise that AI assistants can load to help you build Substreams projects more efficiently.

## Available Skills

All Substreams skills are available in the [substreams-skills repository](https://github.com/streamingfast/substreams-skills):

- **Substreams Development** - Building projects, manifests, modules
- **Substreams SQL** - SQL sinks with PostgreSQL and ClickHouse
- **Substreams Testing** - Testing strategies and data exploration

## Installation

See the [substreams-skills repository](https://github.com/streamingfast/substreams-skills) for installation instructions.

## Quick Start

1. Clone skills repository
2. Configure your AI tool (Claude Code, Cursor, etc.)
3. Start developing with AI assistance

Full instructions: https://github.com/streamingfast/substreams-skills
```

### PR #4 Checklist

- [ ] Create `feature/skills-documentation` branch from `feature/skills`
- [ ] Update README.md with all 3 skills
- [ ] Update examples/using-skills.md with comprehensive examples
- [ ] Create examples/cursor-setup.md
- [ ] Verify examples/vscode-setup.md is correct (created in PR #1)
- [ ] Create CHANGELOG.md
- [ ] Verify all links work
- [ ] Test installation instructions
- [ ] Run final validation: `./scripts/validate-all.sh`
- [ ] Proofread all documentation
- [ ] Commit and push
- [ ] Open PR to `feature/skills`
- [ ] Request review

---

## Final Merge to Main/Develop

After all PRs are merged to `feature/skills` and tested:

```bash
git checkout feature/skills
git pull origin feature/skills

# Create PR: feature/skills -> main (or develop, depending on repo convention)
gh pr create --base main --head feature/skills \
  --title "Initial Substreams Skills Release" \
  --body "Complete Agent Skills implementation with 3 core skills: Development, SQL, and Testing"
```

---

## Progress Tracking

### Overall Progress

- [ ] STEP 0: Create new repository and `feature/skills` branch
- [ ] PR #1: Repository Setup & Base Architecture
- [ ] PR #2: First Skill (substreams-dev)
- [ ] PR #3: Remaining Skills (sql, testing)
- [ ] PR #4: Documentation & Polish
- [ ] Final merge to main/develop

### Detailed Checklist

#### Repository Setup (PR #1)
- [ ] Repository `streamingfast/substreams-skills` exists (pre-created by maintainer)
- [ ] Clone and create `feature/skills` branch
- [ ] Add LICENSE file (Apache 2.0)
- [ ] README.md
- [ ] SKILL_DEVELOPMENT.md
- [ ] scripts/validate-all.sh (executable)
- [ ] .github/workflows/validate.yml
- [ ] .gitignore
- [ ] examples/using-skills.md
- [ ] examples/claude-code-setup.md
- [ ] Validation workflow passes

#### First Skill (PR #2)
- [ ] skills/substreams-dev/SKILL.md
- [ ] skills/substreams-dev/references/manifest-spec.md
- [ ] skills/substreams-dev/references/module-types.md
- [ ] skills/substreams-dev/references/patterns.md
- [ ] skills/substreams-dev/references/networks.md
- [ ] Skill validates successfully
- [ ] Tested in Claude Code/Cursor

#### Remaining Skills (PR #3)

**Substreams SQL:**
- [ ] skills/substreams-sql/SKILL.md
  - [ ] Database Changes (CDC) approach
  - [ ] Relational Mappings approach
  - [ ] PostgreSQL examples
  - [ ] ClickHouse examples
- [ ] skills/substreams-sql/references/schema-design.md
- [ ] skills/substreams-sql/references/clickhouse-guide.md
- [ ] Skill validates successfully

**Substreams Testing:**
- [ ] skills/substreams-testing/SKILL.md
  - [ ] Unit testing section
  - [ ] Integration testing section
  - [ ] firecore tools section
  - [ ] Performance testing with caching notes
- [ ] skills/substreams-testing/references/test-strategies.md
- [ ] skills/substreams-testing/references/firecore-usage.md
- [ ] Skill validates successfully

- [ ] All skills tested in AI tools
- [ ] README updated with all skills

#### Documentation (PR #4)
- [ ] README.md updated (all 3 skills listed)
- [ ] examples/using-skills.md updated (all examples)
- [ ] examples/cursor-setup.md created
- [ ] CHANGELOG.md created
- [ ] All links verified
- [ ] Installation instructions tested
- [ ] Final validation passes

---

## Success Criteria

### Functional Requirements
✅ Repository structure complete
✅ All 3 skills implemented and validated
✅ Skills work in Claude Code and Cursor
✅ Validation workflow passes in CI/CD
✅ Documentation is comprehensive

### Content Quality
✅ Each SKILL.md under 5000 tokens
✅ Reference files provide deep-dive content
✅ Examples are tested and accurate
✅ Skills activate appropriately based on context

### Developer Experience
✅ Clear installation instructions
✅ Easy to contribute new skills
✅ Validation feedback is helpful
✅ Skills are discoverable

---

## References

- [Agent Skills Specification](https://agentskills.io/specification)
- [Anthropic Skills Repository](https://github.com/anthropics/skills)
- [Substreams Documentation](https://substreams.streamingfast.io)
- [substreams-sink-sql](https://github.com/streamingfast/substreams-sink-sql)
- [substreams-sink-clickhouse-showcase](https://github.com/streamingfast/substreams-sink-clickhouse-showcase)

---

## IMPLEMENTATION INSTRUCTIONS FOR REMOTE AGENT

You are implementing this plan in a **NEW repository** through **4 Pull Requests**:

### Workflow

1. **STEP 0:** Create new repository `streamingfast/substreams-skills` and `feature/skills` branch
2. **PR #1:** Set up complete repository structure with validation
3. **Wait for review and merge of PR #1**
4. **PR #2:** Implement first skill (substreams-dev) as reference
5. **Wait for review and merge of PR #2**
6. **PR #3:** Implement remaining 2 skills (sql, testing)
7. **Wait for review and merge of PR #3**
8. **PR #4:** Complete documentation and polish
9. **Final:** Merge `feature/skills` → `main`

### Key Points

- **NEW repository:** Not in existing substreams repo
- **All PRs target `feature/skills`** branch
- **Track progress** using checklist above
- **Validate frequently:** Run `./scripts/validate-all.sh` often
- **Critical content:**
  - SQL skill: Two input formats (Database Changes + Relational Mappings)
  - Testing skill: Include firecore tools usage
  - Testing skill: Note production mode caching impact

Good luck! 🚀
