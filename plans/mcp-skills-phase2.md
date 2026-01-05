# MCP & Skills Phase 2: Enhanced Features

**Status:** Future Work
**Created:** 2025-12-20
**Prerequisites:** Phase 1 MCP server and initial skills must be complete
**Goal:** Add advanced capabilities and expand the ecosystem

## Overview

This document outlines future enhancements for MCP server and Skills after successful Phase 1 deployment.

## Enhanced MCP Server Features

### 1. Advanced Tool Capabilities

#### Long-Running Operations with Sampling
**Enhance `substreams_run` tool:**
- Implement intelligent sampling for long-running streams
- Current: Simple sampling (first 10 blocks, then every 10th)
- Enhanced: Adaptive sampling based on output size and time
- Support progress notifications via MCP progress protocol
- Enable proper cancellation handling

**Technical:**
```go
type RunProgress struct {
    BlocksProcessed  uint64
    TotalBlocks      uint64
    SamplingRate     uint64
    EstimatedTimeLeft string
}

// Send progress updates
func (t *RunTool) reportProgress(ctx context.Context, progress RunProgress) {
    // Use MCP progress notification
    notification := ProgressNotification{
        ProgressToken: ctx.Value("progressToken"),
        Progress: progress.BlocksProcessed,
        Total: progress.TotalBlocks,
    }
    // Send via MCP protocol
}
```

#### Interactive Project Creation
**New tool: `substreams_init_interactive`**
- Full conversational project initialization
- Multi-turn interaction for gathering requirements
- Generate complete project structure
- Support templates and customization

**Flow:**
1. Ask about blockchain network
2. Query about use case (transfers, swaps, NFTs, etc.)
3. Suggest module structure
4. Generate manifest, proto, and Rust code
5. Provide next steps

### 2. Resource Watching

**Enable real-time updates:**
- Watch manifest file changes (`substreams://manifest`)
- Notify on build completion
- Live reload during development
- Support MCP resource subscription protocol

**Use cases:**
- AI assistant automatically detects manifest changes
- Suggests rebuilding after code modifications
- Warns about validation errors in real-time

### 3. HTTP Transport

**Add HTTP with Server-Sent Events (SSE):**
```go
// mcp/http.go
type HTTPServer struct {
    handler *Handler
    port    int
}

func (s *HTTPServer) Serve(ctx context.Context) error {
    mux := http.NewServeMux()

    // SSE endpoint for streaming
    mux.HandleFunc("/mcp", s.handleSSE)

    // Health check
    mux.HandleFunc("/health", s.handleHealth)

    server := &http.Server{
        Addr:    fmt.Sprintf(":%d", s.port),
        Handler: mux,
    }

    return server.ListenAndServe()
}
```

**CLI:**
```bash
substreams mcp serve --transport http --port 3000
```

**Benefits:**
- Browser-based clients
- Easier debugging
- Multi-client support

### 4. Deployment Tools

**New tool: `substreams_deploy`**
- Deploy to Substreams service
- Configure deployment parameters
- Monitor deployment status

**Input:**
```json
{
  "package": "path/to/my.spkg",
  "network": "mainnet.eth",
  "output_module": "map_events",
  "start_block": 17000000,
  "params": {}
}
```

**Output:**
```json
{
  "deployment_id": "dep_abc123",
  "status": "deploying",
  "endpoint": "https://...",
  "dashboard_url": "https://app.streamingfast.io/deployments/dep_abc123"
}
```

**New tool: `substreams_logs`**
- Fetch deployment logs
- Stream live logs
- Filter by severity

### 5. Enhanced Graph Visualization

**Improve `substreams_graph` tool:**
- Generate interactive graphs (HTML)
- Include module statistics (block range, dependencies)
- Visualize data flow with sample data
- Export to multiple formats (Mermaid, DOT, SVG, PNG)

## Expanded Skills

### 4. Substreams Performance (`substreams-performance`)

**Focus:**
- Performance profiling
- Optimization techniques
- Parallelization strategies
- Cost estimation

**When to use:**
- Slow module execution
- High costs
- Large-scale deployments

### 5. Substreams Advanced Patterns (`substreams-patterns`)

**Focus:**
- Multi-chain deployments
- Cross-module composition
- Dynamic data sources
- Advanced indexing strategies

**When to use:**
- Complex architectures
- Multi-protocol integrations
- Advanced use cases

### 6. Substreams Deployment (`substreams-deployment`)

**Focus:**
- Service deployment
- Monitoring and observability
- Production best practices
- Scaling strategies

**When to use:**
- Production deployments
- Service management
- Performance monitoring

## Community Ecosystem

### Skill Marketplace

**Features:**
- Community-contributed skills
- Skill ratings and reviews
- Version management
- Dependency tracking

**Platform:**
- GitHub-based (tags for releases)
- Searchable catalog
- Installation via CLI

**Example:**
```bash
# Install community skill
substreams skills install ethereum-defi

# Update skills
substreams skills update

# List installed
substreams skills list
```

### Skill Templates

**Provide templates for common skill types:**
- Protocol-specific (Uniswap, Aave, etc.)
- Chain-specific (Ethereum, Solana, etc.)
- Pattern-specific (DEX, Lending, NFT)

### Skill Development Toolkit

**Tools for skill creators:**
- Skill generator/scaffolding
- Testing framework for skills
- Documentation generator
- Publishing workflow

## Analytics & Monitoring

### Usage Metrics

**Track:**
- MCP tool call frequency
- Tool success/failure rates
- Average execution time
- Most used resources
- Skill activation rates

**Implementation:**
```go
type Metrics struct {
    ToolCalls map[string]int64
    ToolErrors map[string]int64
    ToolLatency map[string]time.Duration
    SkillActivations map[string]int64
}

func (m *Metrics) RecordToolCall(tool string, duration time.Duration, err error) {
    m.ToolCalls[tool]++
    m.ToolLatency[tool] = duration
    if err != nil {
        m.ToolErrors[tool]++
    }
}
```

**Reporting:**
- Anonymous telemetry (opt-in)
- Dashboard for monitoring
- Error tracking integration

### Health Monitoring

**MCP Server Health:**
- Uptime monitoring
- Memory/CPU usage
- Request queue depth
- Error rates

**Alerting:**
- High error rates
- Performance degradation
- Version compatibility issues

## Advanced Security

### Sandboxing

**Isolate tool execution:**
- Run tools in sandboxed environment
- Limit resource access
- Prevent unauthorized API calls

### Audit Logging

**Track all operations:**
- Tool calls with parameters
- Resource access
- Authentication events
- Error conditions

**Use cases:**
- Security audits
- Debugging
- Compliance

### Fine-Grained Permissions

**Per-tool authorization:**
```json
{
  "permissions": {
    "substreams_run": {
      "allowed": true,
      "max_blocks": 10000,
      "allowed_networks": ["mainnet.eth", "polygon"]
    },
    "substreams_deploy": {
      "allowed": false
    }
  }
}
```

## Integration with Other Tools

### VS Code Extension

**Deep integration:**
- Inline MCP server status
- Tool execution from command palette
- Skill browser and installer
- Manifest validation on save

### CLI Enhancements

**New commands:**
```bash
# Validate with AI assistance
substreams validate --ai-assist

# Optimize manifest
substreams optimize

# Generate tests
substreams generate-tests
```

### IDE Plugins

**Support for:**
- IntelliJ/WebStorm
- Neovim
- Emacs

## Multi-Project Support

**Enable MCP server to handle multiple projects:**
- Project workspace management
- Switch between projects
- Cross-project references

**Implementation:**
```go
type Workspace struct {
    Projects map[string]*Project
    Active   string
}

// New resource: substreams://projects
func (r *ResourceRegistry) ListProjects() []ProjectInfo {
    return workspace.Projects
}

// Tool parameter: project_id
{
  "package": "project:my-transfers/substreams.yaml",
  ...
}
```

## Performance Optimizations

### Caching

**Cache expensive operations:**
- Package inspection results
- Registry lookups
- Schema parsing
- Graph generation

**Implementation:**
```go
type Cache struct {
    inspectCache   *lru.Cache
    registryCache  *lru.Cache
    schemaCach     *lru.Cache
}

func (c *Cache) GetOrCompute(key string, fn func() interface{}) interface{} {
    if val, ok := c.inspectCache.Get(key); ok {
        return val
    }
    val := fn()
    c.inspectCache.Add(key, val)
    return val
}
```

### Parallel Execution

**Run independent tool calls concurrently:**
- Batch resource reads
- Parallel validation checks
- Concurrent registry queries

## Documentation Enhancements

### Interactive Documentation

**MCP resource for searchable docs:**
```
substreams://docs/search?q=store+modules
```

**Returns:**
- Relevant documentation sections
- Code examples
- Links to full docs

### Video Tutorials

**Create tutorial videos:**
- "Getting Started with Substreams MCP"
- "Building Your First Skill"
- "Advanced MCP Features"

**Integration:**
- Link from docs
- Embed in skills
- YouTube playlist

### Live Workshops

**Community workshops:**
- Monthly office hours
- Skill development sessions
- Use case deep dives

## Future Integrations

### GitHub Copilot

**Potential integration:**
- Substreams-aware code completion
- Context from MCP server
- Manifest autocomplete

### ChatGPT Actions

**Expose via ChatGPT:**
- Custom GPT with Substreams knowledge
- Direct tool access
- Conversational debugging

### LangChain/LlamaIndex

**Agent framework integration:**
- Substreams as data source
- MCP tools as agent actions
- Skills as knowledge base

## Migration Path from Phase 1

**Backward compatibility:**
- All Phase 1 features remain supported
- Graceful feature flagging for new capabilities
- Version negotiation in MCP protocol

**Incremental rollout:**
1. Release enhanced tools (opt-in)
2. Add new skills to repository
3. Announce new features
4. Gather feedback and iterate
5. Make new features default

## Success Metrics

**Adoption:**
- Number of users with Phase 2 features enabled
- Skill marketplace contributions
- Community-created skills

**Performance:**
- Tool execution time improvements
- Cache hit rates
- Concurrent request handling

**Satisfaction:**
- User feedback scores
- Feature request themes
- Support ticket reduction

## Timeline (Tentative)

**Months 3-4:**
- Advanced tool capabilities
- Resource watching
- HTTP transport

**Months 5-6:**
- Deployment tools
- Enhanced visualization
- Performance skills

**Months 7-8:**
- Community ecosystem
- Analytics platform
- Security enhancements

**Months 9+:**
- Third-party integrations
- Advanced features based on feedback

---

**Note:** This is a living document. Priorities may shift based on Phase 1 feedback and community needs.
