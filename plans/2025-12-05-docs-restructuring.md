# Documentation Restructuring Plan

**Date:** 2025-12-05
**Status:** Approved, awaiting execution

## How-To Guides Restructuring

### 1. Reorder existing sections
- Move "Understanding Rust & Protobuf" BEFORE "Developing Substreams"
- Rename "Developing Substreams (Deep Dive)" → "Developing Substreams"
- Rename "Using a Substreams Sink" → "Consuming Substreams"

### 2. Create NEW "Composing Substreams" section
**Main landing page** - Create overview explaining:
- Substreams composability concept
- How composition can be leveraged
- Overview of the three composition sources below

**Sub-sections to create:**

#### a) Foundational Modules (NEW CONTENT)
**Source materials:**
- https://github.com/streamingfast/substreams-foundational-modules
- Inspect README files, substreams.yaml, and code (excluding foundational-store folders)
- Document: pre-transformed blockchain models, indexes, pre-filtered capabilities
- Key chains: Ethereum (events/transactions), Solana (instructions), etc.

#### b) Foundational Stores (REORGANIZE EXISTING)
**Source materials:**
- Existing: [docs/new/how-to-guides/foundational-stores/](docs/new/how-to-guides/foundational-stores/)
- https://github.com/streamingfast/substreams-foundational-store
- Document: stateful pre-made datasets, alternative when store limits reached

#### c) Published Packages (NEW CONTENT)
**Source materials:**
- Visit https://substreams.dev to understand UI/search capabilities
- Reference https://github.com/streamingfast/substreams-dev for technical details
- Extract relevant content from [docs/new/references/substreams-components/packages.md](docs/new/references/substreams-components/packages.md)
- Document: how to discover/search packages, integration examples

### 3. Remove duplicate "Foundational Stores" entry
- Currently listed twice (lines 20 and 64 in SUMMARY.md)
- Keep only in new "Composing Substreams" section

## Reference Material Restructuring

### Consolidate from ~15 to 6 top-level categories:

#### 1. CLI Reference (keep as-is)
- Source: [docs/new/references/cli/command-line-interface.md](docs/new/references/cli/command-line-interface.md)

#### 2. Core Concepts (NEW grouping)
Consolidate:
- Architecture & Parallel Execution (from [docs/new/references/architecture.md](docs/new/references/architecture.md))
- Reliability Guarantees (from [docs/new/references/reliability-guarantees.md](docs/new/references/reliability-guarantees.md))
- FAQ (from [docs/new/references/faq.md](docs/new/references/faq.md))
- **ADD:** Module concepts introduction from [docs/new/references/substreams-components/modules/modules.md](docs/new/references/substreams-components/modules/modules.md)

#### 3. Manifest & Components (FLATTEN structure)
Combine into single section:
- Manifests (from [docs/new/references/substreams-components/manifests.md](docs/new/references/substreams-components/manifests.md))
- Packages (from [docs/new/references/substreams-components/packages.md](docs/new/references/substreams-components/packages.md))
- Modules (with subsections: Types, Inputs, Outputs, Indexes, Keys, Parameterized, Dynamic Data Sources, Aggregation Windows)

#### 4. Chain Support (keep existing)
- Chains & Endpoints (from [docs/new/references/chains-and-endpoints.md](docs/new/references/chains-and-endpoints.md))
- Ethereum Data Model (from [docs/new/references/ethereum-data-model.md](docs/new/references/ethereum-data-model.md))

#### 5. Sinks Reference (consolidate)
- SQL Sink (Sink Config, DSN Reference, Reorg Handling)
- Indexer Reference (Test Locally)

#### 6. Development Tools (NEW grouping)
- Logging & Debugging (from [docs/new/references/log-and-debug.md](docs/new/references/log-and-debug.md))
- Dev Container Reference (from [docs/new/references/devcontainer-ref.md](docs/new/references/devcontainer-ref.md))

## Deliverables

1. Update [docs/SUMMARY.md](docs/SUMMARY.md) with new structure
2. Create new content files:
   - `docs/new/how-to-guides/composing-substreams/composing-substreams.md`
   - `docs/new/how-to-guides/composing-substreams/foundational-modules.md`
   - `docs/new/how-to-guides/composing-substreams/published-packages.md`
3. Move existing foundational-stores content to composing-substreams folder
4. Verify all internal documentation links remain functional

## Skipped (per your feedback)
- Chain-specific support documentation (different approach coming)
- CLI registry commands documentation
