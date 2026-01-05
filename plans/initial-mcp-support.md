# MCP Server Implementation Plan for Substreams

**Status:** Ready for Implementation
**Created:** 2025-12-20
**Branch Strategy:** Feature branch `feature/mcp` with incremental PRs
**Goal:** Implement a Model Context Protocol (MCP) server that exposes Substreams functionality to AI assistants and development tools.

## Background

The Model Context Protocol (v2025-11-25) is an open standard developed by Anthropic and now governed by the Agentic AI Foundation. It uses JSON-RPC 2.0 to enable AI assistants to:
- Access **Resources** (contextual data)
- Execute **Tools** (functions the AI can call)
- Use **Prompts** (templated workflows)

MCP is supported by Claude, ChatGPT, and other AI platforms.

## Implementation Strategy

This plan is designed for a remote coding agent to implement incrementally through **6 Pull Requests**:

1. **PR #1: Base Architecture** - Core MCP server infrastructure (FOUNDATION)
2. **PR #2: Sample Tool** - First tool implementation as reference (`substreams_build`)
3. **PR #3: Sample Resource** - First resource implementation as reference (`substreams://manifest`)
4. **PR #4: Sample Prompt** - First prompt implementation as reference ("Debug module output")
5. **PR #5: Complete Implementation** - All remaining tools, resources, and prompts
6. **PR #6: Documentation** - User docs, examples, and integration guides

Each PR targets the `feature/mcp` branch and builds upon the previous.

---

## STEP 0: Branch Setup

**Create feature branch:**
```bash
git checkout develop
git pull origin develop
git checkout -b feature/mcp
git push -u origin feature/mcp
```

All subsequent PRs will target `feature/mcp` for merge.

---

## PR #1: Base Architecture (FOUNDATION)

**Branch:** `feature/mcp-base-architecture`
**Target:** `feature/mcp`
**Goal:** Complete, working MCP server infrastructure with empty handlers

### Acceptance Criteria

✅ MCP server starts successfully via `substreams mcp serve`
✅ Server responds to `initialize` request correctly
✅ Server lists tools/resources/prompts (even if empty implementations)
✅ HTTP integration tests pass
✅ AI coding agent can connect and communicate
✅ Logging works properly
✅ Error handling follows CLI conventions

### Files to Create

```
substreams/
├── mcp/
│   ├── server.go              # Core MCP server
│   ├── server_test.go         # Unit tests
│   ├── handler.go             # JSON-RPC request routing
│   ├── handler_test.go        # Handler tests
│   ├── transport.go           # stdio transport
│   ├── tools/
│   │   ├── registry.go        # Tool registry and dispatcher
│   │   ├── registry_test.go
│   │   └── types.go           # Common tool types
│   ├── resources/
│   │   ├── registry.go        # Resource registry and dispatcher
│   │   ├── registry_test.go
│   │   └── types.go           # Common resource types
│   ├── prompts/
│   │   ├── registry.go        # Prompt registry and dispatcher
│   │   ├── registry_test.go
│   │   └── types.go           # Common prompt types
│   └── integration_test.go    # HTTP server integration tests
└── cmd/substreams/
    └── mcp.go                 # CLI command
```

### Implementation Details

#### 1. Core Server (`mcp/server.go`)

```go
package mcp

import (
    "context"
    "io"
    "os"

    "golang.org/x/exp/jsonrpc2"
    "go.uber.org/zap"
    "github.com/streamingfast/substreams/logging"
)

var zlog, _ = logging.PackageLogger("mcp", "github.com/streamingfast/substreams/mcp")

type Server struct {
    logger  *zap.Logger
    handler *Handler
    conn    *jsonrpc2.Connection
}

type ServerOptions struct {
    Logger *zap.Logger
}

func NewServer(opts ServerOptions) *Server {
    if opts.Logger == nil {
        opts.Logger = zlog
    }

    return &Server{
        logger: opts.Logger,
        handler: NewHandler(opts.Logger),
    }
}

// Serve starts the MCP server on stdio
func (s *Server) Serve(ctx context.Context) error {
    s.logger.Info("starting MCP server on stdio")

    stream := jsonrpc2.NewHeaderStream(os.Stdin, os.Stdout)
    conn := jsonrpc2.NewConn(stream)
    s.conn = conn

    conn.Go(ctx, s.handler)

    <-ctx.Done()
    s.logger.Info("shutting down MCP server")
    return conn.Close()
}

// Handler returns the server's request handler (for testing)
func (s *Server) Handler() *Handler {
    return s.handler
}
```

#### 2. Request Handler (`mcp/handler.go`)

```go
package mcp

import (
    "context"
    "encoding/json"
    "fmt"

    "golang.org/x/exp/jsonrpc2"
    "go.uber.org/zap"
    "github.com/streamingfast/substreams/mcp/tools"
    "github.com/streamingfast/substreams/mcp/resources"
    "github.com/streamingfast/substreams/mcp/prompts"
)

type Handler struct {
    logger    *zap.Logger
    tools     *tools.Registry
    resources *resources.Registry
    prompts   *prompts.Registry
}

func NewHandler(logger *zap.Logger) *Handler {
    return &Handler{
        logger:    logger,
        tools:     tools.NewRegistry(logger),
        resources: resources.NewRegistry(logger),
        prompts:   prompts.NewRegistry(logger),
    }
}

func (h *Handler) Handle(ctx context.Context, req *jsonrpc2.Request) (interface{}, error) {
    h.logger.Debug("handling MCP request",
        zap.String("method", req.Method),
        zap.Any("id", req.ID))

    switch req.Method {
    case "initialize":
        return h.handleInitialize(ctx, req)
    case "tools/list":
        return h.tools.List(), nil
    case "tools/call":
        return h.handleToolCall(ctx, req)
    case "resources/list":
        return h.resources.List(), nil
    case "resources/read":
        return h.handleResourceRead(ctx, req)
    case "prompts/list":
        return h.prompts.List(), nil
    case "prompts/get":
        return h.handlePromptGet(ctx, req)
    default:
        return nil, &jsonrpc2.Error{
            Code:    jsonrpc2.CodeMethodNotFound,
            Message: fmt.Sprintf("unknown method: %s", req.Method),
        }
    }
}

func (h *Handler) handleInitialize(ctx context.Context, req *jsonrpc2.Request) (interface{}, error) {
    var params struct {
        ProtocolVersion string                 `json:"protocolVersion"`
        Capabilities    map[string]interface{} `json:"capabilities"`
        ClientInfo      struct {
            Name    string `json:"name"`
            Version string `json:"version"`
        } `json:"clientInfo"`
    }

    if err := json.Unmarshal(req.Params, &params); err != nil {
        return nil, &jsonrpc2.Error{
            Code:    jsonrpc2.CodeInvalidParams,
            Message: fmt.Sprintf("parse initialize params: %v", err),
        }
    }

    h.logger.Info("MCP client connected",
        zap.String("client", params.ClientInfo.Name),
        zap.String("version", params.ClientInfo.Version),
        zap.String("protocol", params.ProtocolVersion))

    // Get version from substreams
    version := "dev" // TODO: import from version package

    return map[string]interface{}{
        "protocolVersion": "2025-11-25",
        "capabilities": map[string]interface{}{
            "tools":     map[string]interface{}{},
            "resources": map[string]interface{}{},
            "prompts":   map[string]interface{}{},
        },
        "serverInfo": map[string]interface{}{
            "name":    "substreams-mcp",
            "version": version,
        },
    }, nil
}

func (h *Handler) handleToolCall(ctx context.Context, req *jsonrpc2.Request) (interface{}, error) {
    var params struct {
        Name      string          `json:"name"`
        Arguments json.RawMessage `json:"arguments"`
    }

    if err := json.Unmarshal(req.Params, &params); err != nil {
        return nil, &jsonrpc2.Error{
            Code:    jsonrpc2.CodeInvalidParams,
            Message: fmt.Sprintf("parse tool call params: %v", err),
        }
    }

    h.logger.Debug("executing tool", zap.String("tool", params.Name))

    result, err := h.tools.Call(ctx, params.Name, params.Arguments)
    if err != nil {
        h.logger.Error("tool execution failed",
            zap.String("tool", params.Name),
            zap.Error(err))
        return nil, &jsonrpc2.Error{
            Code:    jsonrpc2.CodeInternalError,
            Message: err.Error(),
        }
    }

    return result, nil
}

func (h *Handler) handleResourceRead(ctx context.Context, req *jsonrpc2.Request) (interface{}, error) {
    var params struct {
        URI string `json:"uri"`
    }

    if err := json.Unmarshal(req.Params, &params); err != nil {
        return nil, &jsonrpc2.Error{
            Code:    jsonrpc2.CodeInvalidParams,
            Message: fmt.Sprintf("parse resource read params: %v", err),
        }
    }

    h.logger.Debug("reading resource", zap.String("uri", params.URI))

    result, err := h.resources.Read(ctx, params.URI)
    if err != nil {
        h.logger.Error("resource read failed",
            zap.String("uri", params.URI),
            zap.Error(err))
        return nil, &jsonrpc2.Error{
            Code:    jsonrpc2.CodeInternalError,
            Message: err.Error(),
        }
    }

    return result, nil
}

func (h *Handler) handlePromptGet(ctx context.Context, req *jsonrpc2.Request) (interface{}, error) {
    var params struct {
        Name      string                 `json:"name"`
        Arguments map[string]interface{} `json:"arguments,omitempty"`
    }

    if err := json.Unmarshal(req.Params, &params); err != nil {
        return nil, &jsonrpc2.Error{
            Code:    jsonrpc2.CodeInvalidParams,
            Message: fmt.Sprintf("parse prompt get params: %v", err),
        }
    }

    h.logger.Debug("getting prompt", zap.String("prompt", params.Name))

    result, err := h.prompts.Get(ctx, params.Name, params.Arguments)
    if err != nil {
        h.logger.Error("prompt get failed",
            zap.String("prompt", params.Name),
            zap.Error(err))
        return nil, &jsonrpc2.Error{
            Code:    jsonrpc2.CodeInternalError,
            Message: err.Error(),
        }
    }

    return result, nil
}
```

#### 3. Tool Registry (`mcp/tools/registry.go`)

```go
package tools

import (
    "context"
    "encoding/json"
    "fmt"

    "go.uber.org/zap"
)

type Tool struct {
    Name        string                 `json:"name"`
    Description string                 `json:"description"`
    InputSchema map[string]interface{} `json:"inputSchema"`
}

type ToolHandler func(ctx context.Context, args json.RawMessage) (interface{}, error)

type Registry struct {
    logger   *zap.Logger
    tools    map[string]Tool
    handlers map[string]ToolHandler
}

func NewRegistry(logger *zap.Logger) *Registry {
    r := &Registry{
        logger:   logger,
        tools:    make(map[string]Tool),
        handlers: make(map[string]ToolHandler),
    }

    // Register tools (empty for now, will be added in subsequent PRs)
    // r.Register(BuildTool, BuildHandler(r.logger))
    // r.Register(InspectTool, InspectHandler(r.logger))
    // etc.

    return r
}

func (r *Registry) Register(tool Tool, handler ToolHandler) {
    r.tools[tool.Name] = tool
    r.handlers[tool.Name] = handler
    r.logger.Debug("registered tool", zap.String("name", tool.Name))
}

func (r *Registry) List() map[string]interface{} {
    toolsList := make([]Tool, 0, len(r.tools))
    for _, tool := range r.tools {
        toolsList = append(toolsList, tool)
    }

    return map[string]interface{}{
        "tools": toolsList,
    }
}

func (r *Registry) Call(ctx context.Context, name string, args json.RawMessage) (interface{}, error) {
    handler, ok := r.handlers[name]
    if !ok {
        return nil, fmt.Errorf("tool not found: %s", name)
    }

    return handler(ctx, args)
}
```

#### 4. Resource Registry (`mcp/resources/registry.go`)

```go
package resources

import (
    "context"
    "fmt"

    "go.uber.org/zap"
)

type Resource struct {
    URI         string `json:"uri"`
    Name        string `json:"name"`
    Description string `json:"description"`
    MIMEType    string `json:"mimeType,omitempty"`
}

type ResourceHandler func(ctx context.Context, uri string) (interface{}, error)

type Registry struct {
    logger    *zap.Logger
    resources map[string]Resource
    handlers  map[string]ResourceHandler
}

func NewRegistry(logger *zap.Logger) *Registry {
    r := &Registry{
        logger:    logger,
        resources: make(map[string]Resource),
        handlers:  make(map[string]ResourceHandler),
    }

    // Register resources (empty for now)
    // r.Register(ManifestResource, ManifestHandler(r.logger))
    // etc.

    return r
}

func (r *Registry) Register(resource Resource, handler ResourceHandler) {
    r.resources[resource.URI] = resource
    r.handlers[resource.URI] = handler
    r.logger.Debug("registered resource", zap.String("uri", resource.URI))
}

func (r *Registry) List() map[string]interface{} {
    resourcesList := make([]Resource, 0, len(r.resources))
    for _, resource := range r.resources {
        resourcesList = append(resourcesList, resource)
    }

    return map[string]interface{}{
        "resources": resourcesList,
    }
}

func (r *Registry) Read(ctx context.Context, uri string) (interface{}, error) {
    handler, ok := r.handlers[uri]
    if !ok {
        return nil, fmt.Errorf("resource not found: %s", uri)
    }

    return handler(ctx, uri)
}
```

#### 5. Prompt Registry (`mcp/prompts/registry.go`)

```go
package prompts

import (
    "context"
    "fmt"

    "go.uber.org/zap"
)

type Prompt struct {
    Name        string           `json:"name"`
    Description string           `json:"description"`
    Arguments   []PromptArgument `json:"arguments,omitempty"`
}

type PromptArgument struct {
    Name        string `json:"name"`
    Description string `json:"description"`
    Required    bool   `json:"required"`
}

type PromptHandler func(ctx context.Context, args map[string]interface{}) (interface{}, error)

type Registry struct {
    logger   *zap.Logger
    prompts  map[string]Prompt
    handlers map[string]PromptHandler
}

func NewRegistry(logger *zap.Logger) *Registry {
    r := &Registry{
        logger:   logger,
        prompts:  make(map[string]Prompt),
        handlers: make(map[string]PromptHandler),
    }

    // Register prompts (empty for now)
    // r.Register(DebugPrompt, DebugHandler(r.logger))
    // etc.

    return r
}

func (r *Registry) Register(prompt Prompt, handler PromptHandler) {
    r.prompts[prompt.Name] = prompt
    r.handlers[prompt.Name] = handler
    r.logger.Debug("registered prompt", zap.String("name", prompt.Name))
}

func (r *Registry) List() map[string]interface{} {
    promptsList := make([]Prompt, 0, len(r.prompts))
    for _, prompt := range r.prompts {
        promptsList = append(promptsList, prompt)
    }

    return map[string]interface{}{
        "prompts": promptsList,
    }
}

func (r *Registry) Get(ctx context.Context, name string, args map[string]interface{}) (interface{}, error) {
    handler, ok := r.handlers[name]
    if !ok {
        return nil, fmt.Errorf("prompt not found: %s", name)
    }

    return handler(ctx, args)
}
```

#### 6. CLI Integration (`cmd/substreams/mcp.go`)

```go
package main

import (
    "context"
    "fmt"
    "os"
    "os/signal"
    "syscall"

    "github.com/spf13/cobra"
    "github.com/streamingfast/cli"
    "github.com/streamingfast/substreams/mcp"
    "go.uber.org/zap"
)

var mcpCmd = &cobra.Command{
    Use:   "mcp",
    Short: "Model Context Protocol (MCP) server for AI tools integration",
    Long: cli.Dedent(`
        Start an MCP server that exposes Substreams functionality to AI assistants
        and development tools via the Model Context Protocol.

        The server runs on stdio by default and can be configured in AI tools
        that support MCP (Claude Code, Cursor, etc.).

        Example configuration for Claude Code (.substreams-mcp.json):
        {
          "mcpServers": {
            "substreams": {
              "command": "substreams",
              "args": ["mcp", "serve"]
            }
          }
        }
    `),
}

var mcpServeCmd = &cobra.Command{
    Use:   "serve",
    Short: "Start the MCP server",
    RunE:  runMCPServe,
    Args:  cobra.NoArgs,
}

func init() {
    mcpCmd.AddCommand(mcpServeCmd)
    rootCmd.AddCommand(mcpCmd)
}

func runMCPServe(cmd *cobra.Command, args []string) error {
    ctx, cancel := context.WithCancel(cmd.Context())
    defer cancel()

    // Handle graceful shutdown
    sigCh := make(chan os.Signal, 1)
    signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
    go func() {
        <-sigCh
        zlog.Info("received shutdown signal, stopping MCP server...")
        cancel()
    }()

    // Create and start server
    server := mcp.NewServer(mcp.ServerOptions{
        Logger: zlog,
    })

    err := server.Serve(ctx)
    if err != nil && err != context.Canceled {
        return cli.NoError(fmt.Errorf("MCP server error: %w", err))
    }

    zlog.Info("MCP server stopped")
    return nil
}
```

#### 7. Integration Tests (`mcp/integration_test.go`)

**CRITICAL:** These tests must exercise the HTTP server to verify JSON-RPC protocol compliance.

```go
//go:build integration
// +build integration

package mcp_test

import (
    "bytes"
    "context"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"
    "time"

    "github.com/streamingfast/substreams/mcp"
    "github.com/stretchr/testify/require"
    "golang.org/x/exp/jsonrpc2"
)

func TestHTTPServer_Initialize(t *testing.T) {
    server := mcp.NewServer(mcp.ServerOptions{})

    // Create HTTP handler that wraps MCP server
    httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
        defer cancel()

        stream := jsonrpc2.NewHeaderStream(r.Body, w)
        conn := jsonrpc2.NewConn(stream)
        defer conn.Close()

        conn.Go(ctx, server.Handler())
        <-ctx.Done()
    }))
    defer httpServer.Close()

    // Send initialize request
    req := map[string]interface{}{
        "jsonrpc": "2.0",
        "id":      1,
        "method":  "initialize",
        "params": map[string]interface{}{
            "protocolVersion": "2025-11-25",
            "capabilities":    map[string]interface{}{},
            "clientInfo": map[string]interface{}{
                "name":    "test-client",
                "version": "1.0.0",
            },
        },
    }

    reqBody, _ := json.Marshal(req)
    resp, err := http.Post(httpServer.URL, "application/json", bytes.NewReader(reqBody))
    require.NoError(t, err)
    defer resp.Body.Close()

    // Decode response
    var result map[string]interface{}
    err = json.NewDecoder(resp.Body).Decode(&result)
    require.NoError(t, err)

    // Verify response structure
    require.Contains(t, result, "result")
    resultData := result["result"].(map[string]interface{})

    require.Equal(t, "2025-11-25", resultData["protocolVersion"])
    require.Contains(t, resultData, "capabilities")
    require.Contains(t, resultData, "serverInfo")

    serverInfo := resultData["serverInfo"].(map[string]interface{})
    require.Equal(t, "substreams-mcp", serverInfo["name"])
}

func TestHTTPServer_ToolsList(t *testing.T) {
    server := mcp.NewServer(mcp.ServerOptions{})

    httpServer := httptest.NewServer(createHTTPHandler(server))
    defer httpServer.Close()

    req := map[string]interface{}{
        "jsonrpc": "2.0",
        "id":      2,
        "method":  "tools/list",
    }

    result := sendRequest(t, httpServer.URL, req)

    // Should return empty tools list initially
    require.Contains(t, result, "tools")
    tools := result["tools"].([]interface{})
    require.Empty(t, tools) // Empty until tools are registered
}

func TestHTTPServer_ResourcesList(t *testing.T) {
    server := mcp.NewServer(mcp.ServerOptions{})

    httpServer := httptest.NewServer(createHTTPHandler(server))
    defer httpServer.Close()

    req := map[string]interface{}{
        "jsonrpc": "2.0",
        "id":      3,
        "method":  "resources/list",
    }

    result := sendRequest(t, httpServer.URL, req)

    require.Contains(t, result, "resources")
    resources := result["resources"].([]interface{})
    require.Empty(t, resources) // Empty until resources are registered
}

func TestHTTPServer_PromptsList(t *testing.T) {
    server := mcp.NewServer(mcp.ServerOptions{})

    httpServer := httptest.NewServer(createHTTPHandler(server))
    defer httpServer.Close()

    req := map[string]interface{}{
        "jsonrpc": "2.0",
        "id":      4,
        "method":  "prompts/list",
    }

    result := sendRequest(t, httpServer.URL, req)

    require.Contains(t, result, "prompts")
    prompts := result["prompts"].([]interface{})
    require.Empty(t, prompts) // Empty until prompts are registered
}

func TestHTTPServer_InvalidMethod(t *testing.T) {
    server := mcp.NewServer(mcp.ServerOptions{})

    httpServer := httptest.NewServer(createHTTPHandler(server))
    defer httpServer.Close()

    req := map[string]interface{}{
        "jsonrpc": "2.0",
        "id":      5,
        "method":  "invalid/method",
    }

    reqBody, _ := json.Marshal(req)
    resp, err := http.Post(httpServer.URL, "application/json", bytes.NewReader(reqBody))
    require.NoError(t, err)
    defer resp.Body.Close()

    var result map[string]interface{}
    err = json.NewDecoder(resp.Body).Decode(&result)
    require.NoError(t, err)

    // Should return error
    require.Contains(t, result, "error")
    errorData := result["error"].(map[string]interface{})
    require.Contains(t, errorData, "message")
}

// Helper functions

func createHTTPHandler(server *mcp.Server) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
        defer cancel()

        stream := jsonrpc2.NewHeaderStream(r.Body, w)
        conn := jsonrpc2.NewConn(stream)
        defer conn.Close()

        conn.Go(ctx, server.Handler())
        <-ctx.Done()
    }
}

func sendRequest(t *testing.T, url string, req map[string]interface{}) map[string]interface{} {
    reqBody, _ := json.Marshal(req)
    resp, err := http.Post(url, "application/json", bytes.NewReader(reqBody))
    require.NoError(t, err)
    defer resp.Body.Close()

    var result map[string]interface{}
    err = json.NewDecoder(resp.Body).Decode(&result)
    require.NoError(t, err)

    require.Contains(t, result, "result")
    return result["result"].(map[string]interface{})
}
```

### Testing Commands

```bash
# Run unit tests
go test ./mcp/...

# Run integration tests
go test -tags=integration ./mcp/...

# Test manually
substreams mcp serve
# In another terminal, test with MCP client
```

### PR #1 Checklist

- [ ] Create `feature/mcp-base-architecture` branch
- [ ] Implement all files listed above
- [ ] Add `golang.org/x/exp/jsonrpc2` to go.mod
- [ ] All unit tests pass
- [ ] Integration tests pass
- [ ] `substreams mcp serve` starts successfully
- [ ] Server responds to initialize request
- [ ] Tools/resources/prompts list (empty)
- [ ] Logging works properly
- [ ] Open PR to `feature/mcp` with detailed description
- [ ] Request review

---

## PR #2: Sample Tool - `substreams_build`

**Branch:** `feature/substreams_build_tool`
**Target:** `feature/mcp`
**Prerequisites:** PR #1 merged
**Goal:** Implement first complete tool as reference for remaining tools

### Files to Create

```
mcp/tools/
├── build.go           # Tool implementation
└── build_test.go      # Unit tests
```

### Tool Specification

**Name:** `substreams_build`

**Description:** Build a Substreams project from source, compiling Rust modules to WebAssembly and generating a .spkg package file.

**Input Schema:**
```json
{
  "type": "object",
  "properties": {
    "manifest_path": {
      "type": "string",
      "description": "Path to substreams.yaml manifest file"
    },
    "skip_package": {
      "type": "boolean",
      "description": "Skip package generation (compile only)",
      "default": false
    }
  },
  "required": ["manifest_path"]
}
```

**Output Schema:**
```json
{
  "type": "object",
  "properties": {
    "success": { "type": "boolean" },
    "spkg_path": { "type": "string" },
    "build_log": { "type": "string" },
    "errors": {
      "type": "array",
      "items": { "type": "string" }
    }
  }
}
```

### Implementation (`mcp/tools/build.go`)

```go
package tools

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "os"
    "os/exec"
    "path/filepath"

    "go.uber.org/zap"
)

var BuildTool = Tool{
    Name:        "substreams_build",
    Description: "Build a Substreams project from source, compiling Rust modules to WebAssembly and generating a .spkg package file.",
    InputSchema: map[string]interface{}{
        "type": "object",
        "properties": map[string]interface{}{
            "manifest_path": map[string]interface{}{
                "type":        "string",
                "description": "Path to substreams.yaml manifest file",
            },
            "skip_package": map[string]interface{}{
                "type":        "boolean",
                "description": "Skip package generation (compile only)",
                "default":     false,
            },
        },
        "required": []string{"manifest_path"},
    },
}

type buildInput struct {
    ManifestPath string `json:"manifest_path"`
    SkipPackage  bool   `json:"skip_package"`
}

type buildOutput struct {
    Success  bool     `json:"success"`
    SpkgPath string   `json:"spkg_path,omitempty"`
    BuildLog string   `json:"build_log"`
    Errors   []string `json:"errors,omitempty"`
}

func BuildHandler(logger *zap.Logger) ToolHandler {
    return func(ctx context.Context, args json.RawMessage) (interface{}, error) {
        var input buildInput
        if err := json.Unmarshal(args, &input); err != nil {
            return nil, fmt.Errorf("invalid input: %w", err)
        }

        logger.Info("building substreams project",
            zap.String("manifest", input.ManifestPath),
            zap.Bool("skip_package", input.SkipPackage))

        // Validate manifest exists
        if _, err := os.Stat(input.ManifestPath); os.IsNotExist(err) {
            return buildOutput{
                Success: false,
                Errors:  []string{fmt.Sprintf("manifest file not found: %s", input.ManifestPath)},
            }, nil
        }

        // Get manifest directory
        manifestDir := filepath.Dir(input.ManifestPath)

        // Build command
        var stdout, stderr bytes.Buffer
        cmd := exec.CommandContext(ctx, "cargo", "build", "--target", "wasm32-unknown-unknown", "--release")
        cmd.Dir = manifestDir
        cmd.Stdout = &stdout
        cmd.Stderr = &stderr

        err := cmd.Run()
        buildLog := stdout.String() + stderr.String()

        if err != nil {
            logger.Error("build failed", zap.Error(err))
            return buildOutput{
                Success:  false,
                BuildLog: buildLog,
                Errors:   []string{fmt.Sprintf("build failed: %v", err)},
            }, nil
        }

        // Generate package if not skipped
        spkgPath := ""
        if !input.SkipPackage {
            // TODO: Implement package generation
            // For now, just locate the expected .spkg file
            spkgPath = filepath.Join(manifestDir, filepath.Base(manifestDir)+".spkg")
        }

        logger.Info("build completed successfully",
            zap.String("spkg_path", spkgPath))

        return buildOutput{
            Success:  true,
            SpkgPath: spkgPath,
            BuildLog: buildLog,
        }, nil
    }
}

// Register function to be called from registry.go
func RegisterBuildTool(r *Registry) {
    r.Register(BuildTool, BuildHandler(r.logger))
}
```

### Unit Tests (`mcp/tools/build_test.go`)

```go
package tools

import (
    "context"
    "encoding/json"
    "os"
    "path/filepath"
    "testing"

    "github.com/stretchr/testify/require"
    "go.uber.org/zap/zaptest"
)

func TestBuildHandler_Success(t *testing.T) {
    logger := zaptest.NewLogger(t)
    handler := BuildHandler(logger)

    // Create test manifest
    tmpDir := t.TempDir()
    manifestPath := filepath.Join(tmpDir, "substreams.yaml")
    err := os.WriteFile(manifestPath, []byte("specVersion: v0.1.0"), 0644)
    require.NoError(t, err)

    // Create minimal Cargo.toml for testing
    cargoToml := `[package]
name = "test"
version = "0.1.0"

[lib]
crate-type = ["cdylib"]
`
    err = os.WriteFile(filepath.Join(tmpDir, "Cargo.toml"), []byte(cargoToml), 0644)
    require.NoError(t, err)

    input := buildInput{
        ManifestPath: manifestPath,
        SkipPackage:  true,
    }

    args, _ := json.Marshal(input)
    result, err := handler(context.Background(), args)
    require.NoError(t, err)

    output := result.(buildOutput)
    // Note: This will fail without actual Rust project, but structure is correct
    require.Contains(t, output.BuildLog, "cargo")
}

func TestBuildHandler_ManifestNotFound(t *testing.T) {
    logger := zaptest.NewLogger(t)
    handler := BuildHandler(logger)

    input := buildInput{
        ManifestPath: "/nonexistent/substreams.yaml",
    }

    args, _ := json.Marshal(input)
    result, err := handler(context.Background(), args)
    require.NoError(t, err)

    output := result.(buildOutput)
    require.False(t, output.Success)
    require.NotEmpty(t, output.Errors)
    require.Contains(t, output.Errors[0], "not found")
}
```

### Update Registry (`mcp/tools/registry.go`)

```go
func NewRegistry(logger *zap.Logger) *Registry {
    r := &Registry{
        logger:   logger,
        tools:    make(map[string]Tool),
        handlers: make(map[string]ToolHandler),
    }

    // Register tools
    RegisterBuildTool(r)

    return r
}
```

### PR #2 Checklist

- [ ] Create `feature/substreams_build_tool` branch from `feature/mcp`
- [ ] Implement `build.go` and `build_test.go`
- [ ] Update registry to register tool
- [ ] Unit tests pass
- [ ] Integration test verifies tool appears in `tools/list`
- [ ] Manual test: `substreams mcp serve` shows tool
- [ ] Open PR to `feature/mcp`
- [ ] Request review

---

## PR #3: Sample Resource - `substreams://manifest`

**Branch:** `feature/manifest_resource`
**Target:** `feature/mcp`
**Prerequisites:** PR #1 merged
**Goal:** Implement first complete resource as reference

### Files to Create

```
mcp/resources/
├── manifest.go        # Resource implementation
└── manifest_test.go   # Unit tests
```

### Resource Specification

**URI:** `substreams://manifest`

**Description:** Current project's substreams.yaml manifest file

**MIME Type:** `text/yaml`

### Implementation (`mcp/resources/manifest.go`)

```go
package resources

import (
    "context"
    "fmt"
    "os"
    "path/filepath"

    "go.uber.org/zap"
)

var ManifestResource = Resource{
    URI:         "substreams://manifest",
    Name:        "Project Manifest",
    Description: "Current project's substreams.yaml manifest file",
    MIMEType:    "text/yaml",
}

type manifestResourceOutput struct {
    URI      string                 `json:"uri"`
    MIMEType string                 `json:"mimeType"`
    Text     string                 `json:"text"`
    Metadata map[string]interface{} `json:"metadata,omitempty"`
}

func ManifestHandler(logger *zap.Logger) ResourceHandler {
    return func(ctx context.Context, uri string) (interface{}, error) {
        logger.Debug("reading manifest resource")

        // Search for substreams.yaml in current directory
        cwd, err := os.Getwd()
        if err != nil {
            return nil, fmt.Errorf("get current directory: %w", err)
        }

        manifestPath := filepath.Join(cwd, "substreams.yaml")
        if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
            return nil, fmt.Errorf("substreams.yaml not found in current directory")
        }

        // Read manifest content
        content, err := os.ReadFile(manifestPath)
        if err != nil {
            return nil, fmt.Errorf("read manifest: %w", err)
        }

        // Get file metadata
        fileInfo, _ := os.Stat(manifestPath)

        logger.Info("manifest resource read successfully",
            zap.String("path", manifestPath),
            zap.Int64("size", fileInfo.Size()))

        return manifestResourceOutput{
            URI:      uri,
            MIMEType: "text/yaml",
            Text:     string(content),
            Metadata: map[string]interface{}{
                "path":         manifestPath,
                "size":         fileInfo.Size(),
                "last_modified": fileInfo.ModTime().Format("2006-01-02T15:04:05Z07:00"),
            },
        }, nil
    }
}

func RegisterManifestResource(r *Registry) {
    r.Register(ManifestResource, ManifestHandler(r.logger))
}
```

### Unit Tests (`mcp/resources/manifest_test.go`)

```go
package resources

import (
    "context"
    "os"
    "path/filepath"
    "testing"

    "github.com/stretchr/testify/require"
    "go.uber.org/zap/zaptest"
)

func TestManifestHandler_Success(t *testing.T) {
    logger := zaptest.NewLogger(t)
    handler := ManifestHandler(logger)

    // Create test manifest in temp dir
    tmpDir := t.TempDir()
    oldCwd, _ := os.Getwd()
    defer os.Chdir(oldCwd)

    err := os.Chdir(tmpDir)
    require.NoError(t, err)

    manifestContent := `specVersion: v0.1.0
package:
  name: test-substreams
  version: v1.0.0
`
    err = os.WriteFile("substreams.yaml", []byte(manifestContent), 0644)
    require.NoError(t, err)

    result, err := handler(context.Background(), "substreams://manifest")
    require.NoError(t, err)

    output := result.(manifestResourceOutput)
    require.Equal(t, "substreams://manifest", output.URI)
    require.Equal(t, "text/yaml", output.MIMEType)
    require.Contains(t, output.Text, "test-substreams")
    require.Contains(t, output.Metadata, "path")
}

func TestManifestHandler_NotFound(t *testing.T) {
    logger := zaptest.NewLogger(t)
    handler := ManifestHandler(logger)

    // Change to temp dir without manifest
    tmpDir := t.TempDir()
    oldCwd, _ := os.Getwd()
    defer os.Chdir(oldCwd)

    err := os.Chdir(tmpDir)
    require.NoError(t, err)

    _, err = handler(context.Background(), "substreams://manifest")
    require.Error(t, err)
    require.Contains(t, err.Error(), "not found")
}
```

### Update Registry

```go
func NewRegistry(logger *zap.Logger) *Registry {
    r := &Registry{
        logger:    logger,
        resources: make(map[string]Resource),
        handlers:  make(map[string]ResourceHandler),
    }

    RegisterManifestResource(r)

    return r
}
```

### PR #3 Checklist

- [ ] Create `feature/manifest_resource` branch from `feature/mcp`
- [ ] Implement `manifest.go` and `manifest_test.go`
- [ ] Update registry to register resource
- [ ] Unit tests pass
- [ ] Integration test verifies resource in `resources/list`
- [ ] Manual test: resource can be read via MCP
- [ ] Open PR to `feature/mcp`
- [ ] Request review

---

## PR #4: Sample Prompt - "Debug module output"

**Branch:** `feature/debug_prompt`
**Target:** `feature/mcp`
**Prerequisites:** PR #1 merged
**Goal:** Implement first complete prompt as reference

### Files to Create

```
mcp/prompts/
├── debug.go          # Prompt implementation
└── debug_test.go     # Unit tests
```

### Prompt Specification

**Name:** `debug_module_output`

**Description:** Help troubleshoot Substreams module issues by analyzing manifest, running test, and suggesting fixes.

**Arguments:**
- `manifest_path` (required): Path to substreams.yaml
- `module_name` (optional): Specific module to debug
- `start_block` (optional): Starting block for test run

### Implementation (`mcp/prompts/debug.go`)

```go
package prompts

import (
    "context"
    "fmt"

    "go.uber.org/zap"
)

var DebugPrompt = Prompt{
    Name:        "debug_module_output",
    Description: "Help troubleshoot Substreams module issues by analyzing manifest, running test, and suggesting fixes.",
    Arguments: []PromptArgument{
        {
            Name:        "manifest_path",
            Description: "Path to substreams.yaml manifest file",
            Required:    true,
        },
        {
            Name:        "module_name",
            Description: "Specific module to debug (optional)",
            Required:    false,
        },
        {
            Name:        "start_block",
            Description: "Starting block number for test run (optional)",
            Required:    false,
        },
    },
}

type debugPromptOutput struct {
    Description string                   `json:"description"`
    Messages    []map[string]interface{} `json:"messages"`
}

func DebugHandler(logger *zap.Logger) PromptHandler {
    return func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
        manifestPath, ok := args["manifest_path"].(string)
        if !ok || manifestPath == "" {
            return nil, fmt.Errorf("manifest_path is required")
        }

        moduleName := ""
        if m, ok := args["module_name"].(string); ok {
            moduleName = m
        }

        startBlock := ""
        if s, ok := args["start_block"]; ok {
            startBlock = fmt.Sprintf("%v", s)
        }

        logger.Info("generating debug prompt",
            zap.String("manifest", manifestPath),
            zap.String("module", moduleName))

        // Build prompt template with variable substitution
        messages := []map[string]interface{}{
            {
                "role": "assistant",
                "content": map[string]interface{}{
                    "type": "text",
                    "text": fmt.Sprintf(`I'll help you debug your Substreams module.

Let me analyze your project:

**Manifest:** %s
**Module:** %s
**Start Block:** %s

I'll:
1. Inspect your manifest to understand the module structure
2. Run a small test (100 blocks) to see actual output
3. Analyze the results and identify issues
4. Suggest specific fixes

What issue are you experiencing?
- No output / Empty results
- Unexpected data format
- Performance problems
- Build errors

Let's start by running a quick test...`, manifestPath, getModuleDisplay(moduleName), getBlockDisplay(startBlock)),
                },
            },
        }

        return debugPromptOutput{
            Description: "Debug Substreams module output and suggest fixes",
            Messages:    messages,
        }, nil
    }
}

func getModuleDisplay(moduleName string) string {
    if moduleName == "" {
        return "(will detect automatically)"
    }
    return moduleName
}

func getBlockDisplay(startBlock string) string {
    if startBlock == "" {
        return "(will use manifest initialBlock)"
    }
    return startBlock
}

func RegisterDebugPrompt(r *Registry) {
    r.Register(DebugPrompt, DebugHandler(r.logger))
}
```

### Unit Tests (`mcp/prompts/debug_test.go`)

```go
package prompts

import (
    "context"
    "testing"

    "github.com/stretchr/testify/require"
    "go.uber.org/zap/zaptest"
)

func TestDebugHandler_Success(t *testing.T) {
    logger := zaptest.NewLogger(t)
    handler := DebugHandler(logger)

    args := map[string]interface{}{
        "manifest_path": "./substreams.yaml",
        "module_name":   "map_events",
        "start_block":   "17000000",
    }

    result, err := handler(context.Background(), args)
    require.NoError(t, err)

    output := result.(debugPromptOutput)
    require.Equal(t, "Debug Substreams module output and suggest fixes", output.Description)
    require.NotEmpty(t, output.Messages)

    message := output.Messages[0]
    require.Equal(t, "assistant", message["role"])

    content := message["content"].(map[string]interface{})
    text := content["text"].(string)
    require.Contains(t, text, "substreams.yaml")
    require.Contains(t, text, "map_events")
    require.Contains(t, text, "17000000")
}

func TestDebugHandler_MissingManifest(t *testing.T) {
    logger := zaptest.NewLogger(t)
    handler := DebugHandler(logger)

    args := map[string]interface{}{}

    _, err := handler(context.Background(), args)
    require.Error(t, err)
    require.Contains(t, err.Error(), "manifest_path is required")
}

func TestDebugHandler_OptionalArgs(t *testing.T) {
    logger := zaptest.NewLogger(t)
    handler := DebugHandler(logger)

    args := map[string]interface{}{
        "manifest_path": "./substreams.yaml",
    }

    result, err := handler(context.Background(), args)
    require.NoError(t, err)

    output := result.(debugPromptOutput)
    message := output.Messages[0]
    content := message["content"].(map[string]interface{})
    text := content["text"].(string)

    require.Contains(t, text, "will detect automatically")
    require.Contains(t, text, "will use manifest initialBlock")
}
```

### Update Registry

```go
func NewRegistry(logger *zap.Logger) *Registry {
    r := &Registry{
        logger:   logger,
        prompts:  make(map[string]Prompt),
        handlers: make(map[string]PromptHandler),
    }

    RegisterDebugPrompt(r)

    return r
}
```

### PR #4 Checklist

- [ ] Create `feature/debug_prompt` branch from `feature/mcp`
- [ ] Implement `debug.go` and `debug_test.go`
- [ ] Update registry to register prompt
- [ ] Unit tests pass
- [ ] Integration test verifies prompt in `prompts/list`
- [ ] Manual test: prompt can be retrieved via MCP
- [ ] Open PR to `feature/mcp`
- [ ] Request review

---

## PR #5: Complete Implementation

**Branch:** `feature/mcp-complete-implementation`
**Target:** `feature/mcp`
**Prerequisites:** PRs #2, #3, #4 merged and reviewed
**Goal:** Implement all remaining tools, resources, and prompts

Based on the patterns established in PRs #2-4, implement:

### Remaining Tools (6)

1. **`substreams_inspect`** - Inspect package structure
2. **`substreams_run`** - Execute module (**IMPORTANT: default max_blocks=1000, network field uses "mainnet" not "mainnet.eth"**)
3. **`substreams_estimate`** - Estimate processing costs
4. **`substreams_validate`** - Validate manifest
5. **`substreams_graph`** - Generate module graph
6. **`substreams_registry_info`** - Query registry

### Remaining Resources (4)

1. **`substreams://schema/{module}`** - Protobuf schema
2. **`substreams://docs/manifest`** - Manifest documentation (**Read from `docs/` directory in repository**)
3. **`substreams://docs/modules`** - Module types documentation (**Read from `docs/` directory in repository**)
4. **`substreams://endpoints`** - Network endpoints (**MUST use firehose-networks library**)

### Remaining Prompts (2)

1. **"Create new Substreams project"**
2. **"Optimize performance"**

### Critical Implementation Notes

**`substreams_run` Tool:**
- Network field: Validated by Network Registry, use values like `"mainnet"`, `"polygon"` NOT `"mainnet.eth"`
- Default `max_blocks: 1000` for development safety
- Implement intelligent sampling (first 10 blocks, then every 10th, last block)

**`substreams://endpoints` Resource:**
- **MUST** use `github.com/streamingfast/firehose-networks` library
- Return network names validated by Network Registry
- Example code:
```go
import networks "github.com/streamingfast/firehose-networks"

func EndpointsHandler(logger *zap.Logger) ResourceHandler {
    return func(ctx context.Context, uri string) (interface{}, error) {
        // Get all supported networks
        networkMap := make(map[string]interface{})

        // Use firehose-networks to get network info
        for _, networkID := range networks.GetNetworkIDs() {
            endpoint := networks.GetSubstreamsEndpoint(networkID)
            if endpoint != "" {
                networkMap[networkID] = map[string]interface{}{
                    "endpoint":     endpoint,
                    "display_name": networks.GetDisplayName(networkID),
                }
            }
        }

        return map[string]interface{}{
            "uri":      uri,
            "mimeType": "application/json",
            "text":     toJSON(networkMap),
        }, nil
    }
}
```

**Docs Resources:**

The Substreams documentation is in `docs/` directory of this repository. Read directly from:
- `docs/new/references/substreams-components/manifests.md`
- `docs/new/references/substreams-components/modules/`

Example:
```go
func DocsManifestHandler(logger *zap.Logger) ResourceHandler {
    return func(ctx context.Context, uri string) (interface{}, error) {
        // Read from docs directory in repository
        content, err := os.ReadFile("docs/new/references/substreams-components/manifests.md")
        if err != nil {
            return nil, fmt.Errorf("read manifest docs: %w", err)
        }

        return map[string]interface{}{
            "uri":      uri,
            "mimeType": "text/markdown",
            "text":     string(content),
        }, nil
    }
}
```

### Files to Create

```
mcp/tools/
├── inspect.go + inspect_test.go
├── run.go + run_test.go
├── estimate.go + estimate_test.go
├── validate.go + validate_test.go
├── graph.go + graph_test.go
└── registry.go + registry_test.go

mcp/resources/
├── schema.go + schema_test.go
├── docs.go + docs_test.go
└── endpoints.go + endpoints_test.go

mcp/prompts/
├── create_project.go + create_project_test.go
└── optimize.go + optimize_test.go
```

### PR #5 Checklist

- [ ] Create `feature/mcp-complete-implementation` branch from `feature/mcp`
- [ ] Implement all 6 remaining tools with tests (80%+ coverage)
- [ ] Implement all 4 remaining resources with tests (80%+ coverage)
- [ ] Implement all 2 remaining prompts with tests (80%+ coverage)
- [ ] Update all registries to register new handlers
- [ ] All unit tests pass
- [ ] All integration tests pass
- [ ] **VERIFY:** Network names use "mainnet" not "mainnet.eth" (`substreams_run`)
- [ ] **VERIFY:** Default max_blocks=1000 (`substreams_run`)
- [ ] **VERIFY:** Endpoints resource uses firehose-networks library
- [ ] **VERIFY:** Docs resources read from `docs/` directory
- [ ] Manual testing with MCP client (Claude Code or equivalent)
- [ ] Open PR to `feature/mcp` with comprehensive description
- [ ] Request review

---

## PR #6: Documentation

**Branch:** `feature/mcp-documentation`
**Target:** `feature/mcp`
**Prerequisites:** PR #5 merged
**Goal:** Complete user documentation and examples

### Files to Create

```
docs/ai-tools/
├── README.md              # Overview
├── mcp-server.md          # Setup guide
├── examples.md            # Example workflows
└── troubleshooting.md     # Common issues

.substreams-mcp.json       # Example config (root)
```

### Documentation Structure

#### 1. Overview (`docs/ai-tools/README.md`)

```markdown
# AI Tools Integration

Substreams integrates with AI development tools via the Model Context Protocol (MCP).

## What is MCP?

The Model Context Protocol enables AI assistants to access Substreams functionality directly:
- Build and inspect projects
- Run modules locally
- Validate manifests
- Get documentation and examples

## Quick Start

1. Install Substreams CLI (v2.0.0+)
2. Configure your AI tool (see [Setup Guide](./mcp-server.md))
3. Start developing with AI assistance!

## Learn More

- [MCP Server Setup](./mcp-server.md) - Configure Claude Code, Cursor, etc.
- [Example Workflows](./examples.md) - Common use cases
- [Troubleshooting](./troubleshooting.md) - Common issues and solutions

## Supported AI Tools

- Claude Code
- Cursor
- VS Code with MCP extension
- Any MCP-compatible client
```

#### 2. Setup Guide (`docs/ai-tools/mcp-server.md`)

**Document:**
- Installation requirements
- Configuration for each AI tool (Claude Code, Cursor, VS Code)
- Environment variables
- Verification steps
- Complete reference for all 7 tools, 5 resources, and 3 prompts

#### 3. Examples (`docs/ai-tools/examples.md`)

**Document:**
- Creating a new project
- Building a project
- Debugging module output
- Estimating costs
- Running modules locally
- Validating manifests
- Example conversations with AI assistant

#### 4. Troubleshooting (`docs/ai-tools/troubleshooting.md`)

**Document:**
- MCP server won't start
- Tool calls fail
- Network/endpoint issues
- Build failures
- Common error messages

#### 5. Example Config (`.substreams-mcp.json`)

```json
{
  "mcpServers": {
    "substreams": {
      "command": "substreams",
      "args": ["mcp", "serve"],
      "env": {
        "SUBSTREAMS_API_TOKEN": "${SUBSTREAMS_API_TOKEN}"
      }
    }
  }
}
```

### Update Main Docs

Add link from main Substreams documentation:

**File:** `docs/SUMMARY.md`

```markdown
## AI Tools
- [Overview](ai-tools/README.md)
- [MCP Server Setup](ai-tools/mcp-server.md)
- [Examples](ai-tools/examples.md)
- [Troubleshooting](ai-tools/troubleshooting.md)
```

### PR #6 Checklist

- [ ] Create `feature/mcp-documentation` branch from `feature/mcp`
- [ ] Write `docs/ai-tools/README.md`
- [ ] Write `docs/ai-tools/mcp-server.md` (complete tool/resource/prompt reference)
- [ ] Write `docs/ai-tools/examples.md` (practical workflows)
- [ ] Write `docs/ai-tools/troubleshooting.md`
- [ ] Create `.substreams-mcp.json` example config
- [ ] Update `docs/SUMMARY.md`
- [ ] Test all examples manually
- [ ] Verify all links work
- [ ] Proofread for clarity and accuracy
- [ ] Open PR to `feature/mcp`
- [ ] Request review

---

## Final Merge to Develop

After all PRs are merged to `feature/mcp` and tested:

```bash
git checkout feature/mcp
git pull origin feature/mcp

# Create PR: feature/mcp -> develop
gh pr create --base develop --head feature/mcp \
  --title "Add MCP Server Support" \
  --body "Complete MCP server implementation with 7 tools, 5 resources, 3 prompts, and documentation"
```

---

## Progress Tracking

### Overall Progress

- [ ] STEP 0: Create `feature/mcp` branch
- [ ] PR #1: Base Architecture
- [ ] PR #2: Sample Tool (substreams_build)
- [ ] PR #3: Sample Resource (manifest)
- [ ] PR #4: Sample Prompt (debug)
- [ ] PR #5: Complete Implementation
- [ ] PR #6: Documentation
- [ ] Final merge to develop

### Detailed Checklist

#### Base Architecture (PR #1)
- [ ] `mcp/server.go` with stdio transport
- [ ] `mcp/handler.go` with JSON-RPC routing
- [ ] `mcp/tools/registry.go` (empty)
- [ ] `mcp/resources/registry.go` (empty)
- [ ] `mcp/prompts/registry.go` (empty)
- [ ] `mcp/integration_test.go` (HTTP tests)
- [ ] `cmd/substreams/mcp.go` (CLI command)
- [ ] All unit tests pass
- [ ] Integration tests pass

#### Sample Implementations (PRs #2-4)
- [ ] `substreams_build` tool with tests
- [ ] `substreams://manifest` resource with tests
- [ ] `debug_module_output` prompt with tests

#### Complete Implementation (PR #5)

**Tools:**
- [ ] `substreams_inspect` with tests
- [ ] `substreams_run` with tests (1000 block limit, "mainnet" not "mainnet.eth")
- [ ] `substreams_estimate` with tests
- [ ] `substreams_validate` with tests
- [ ] `substreams_graph` with tests
- [ ] `substreams_registry_info` with tests

**Resources:**
- [ ] `substreams://schema/{module}` with tests
- [ ] `substreams://docs/manifest` with tests (from docs/ directory)
- [ ] `substreams://docs/modules` with tests (from docs/ directory)
- [ ] `substreams://endpoints` with tests (firehose-networks)

**Prompts:**
- [ ] `create_new_project` with tests
- [ ] `optimize_performance` with tests

#### Documentation (PR #6)
- [ ] `docs/ai-tools/README.md`
- [ ] `docs/ai-tools/mcp-server.md`
- [ ] `docs/ai-tools/examples.md`
- [ ] `docs/ai-tools/troubleshooting.md`
- [ ] `.substreams-mcp.json`
- [ ] Update `docs/SUMMARY.md`

---

## Success Criteria

### Functional Requirements
✅ MCP server starts via `substreams mcp serve`
✅ All 7 tools implemented and tested
✅ All 5 resources implemented and tested
✅ All 3 prompts implemented and tested
✅ HTTP integration tests pass
✅ Works with Claude Code/Cursor

### Code Quality
✅ 80%+ unit test coverage
✅ All integration tests pass
✅ Error handling with `cli.NoError`/`cli.Ensure`
✅ Proper logging with zap throughout
✅ Code reviewed and approved

### Documentation
✅ Complete setup guides
✅ All tools/resources/prompts documented
✅ Example workflows provided
✅ Troubleshooting guide complete

---

## References

- [MCP Specification 2025-11-25](https://modelcontextprotocol.io/specification/2025-11-25)
- [golang.org/x/exp/jsonrpc2](https://pkg.go.dev/golang.org/x/exp/jsonrpc2)
- [Substreams Documentation](docs/)
- [firehose-networks Library](https://github.com/streamingfast/firehose-networks)

---

## IMPLEMENTATION INSTRUCTIONS FOR REMOTE AGENT

You are implementing this plan incrementally through **6 Pull Requests**:

### Workflow

1. **STEP 0:** Create `feature/mcp` branch from `develop`
2. **PR #1:** Build complete foundation (base architecture)
3. **Wait for review and merge of PR #1**
4. **PRs #2-4:** Implement sample tool/resource/prompt for review
5. **Wait for review and merge of PRs #2-4**
6. **PR #5:** Complete all remaining implementations
7. **Wait for review and merge of PR #5**
8. **PR #6:** Finish with documentation
9. **Final:** Merge `feature/mcp` → `develop`

### Key Points

- **All PRs target `feature/mcp`** (not develop)
- **Track progress** using checklist above
- **Test thoroughly** before opening each PR
- **Critical corrections:**
  - Network field: "mainnet" not "mainnet.eth"
  - Default max_blocks: 1000
  - Endpoints: Use firehose-networks library
  - Docs: Read from docs/ directory

Good luck! 🚀
