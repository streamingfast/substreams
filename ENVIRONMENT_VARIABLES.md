# Environment Variables

This document lists all environment variables used by the Substreams project, organized by their usage context.

## Command-Specific Variables

### `substreams auth` Command

#### `LOCAL_DEVELOPMENT`
**Development environment indicator**
- **Purpose**: Indicates if running in local development mode
- **Usage**: Set to "true" to use localhost URLs instead of production URLs
- **Default**: `false` (uses production URLs: https://thegraph.market/auth/substreams-devenv)
- **Location**: `cmd/substreams/auth.go`

### `substreams registry` Commands

#### `SUBSTREAMS_REGISTRY_TOKEN`
**Registry authentication token**
- **Purpose**: Authentication token for Substreams registry operations (publish, etc.)
- **Usage**: Used for publishing to the registry, falls back to reading from `~/.config/substreams/registry-token`
- **Default**: None (falls back to file-based token)
- **Location**: `cmd/substreams/registry-publish.go`

#### `SUBSTREAMS_REGISTRY_ENDPOINT`
**Registry endpoint override**
- **Purpose**: Override the default Substreams registry endpoint
- **Usage**: Allows using custom registry endpoints for development or private registries
- **Default**: `https://substreams.dev`
- **Location**: `cmd/substreams/registry.go`

#### `SUBSTREAMS_DOWNLOAD_ENDPOINT`
**Download endpoint override**
- **Purpose**: Override the default Substreams package download endpoint
- **Usage**: Allows using custom download endpoints for packages
- **Default**: `https://spkg.io`
- **Location**: `cmd/substreams/registry.go`

#### `SUBSTREAMS_CODEGEN_ENDPOINT`
**Codegen endpoint override**
- **Purpose**: Override the default Substreams code generation endpoint
- **Usage**: Allows using custom codegen endpoints
- **Default**: `https://codegen.substreams.dev`
- **Location**: `cmd/substreams/registry.go`

## Server Engine Variables

### Performance tuning

#### `SUBSTREAMS_STORE_SIZE_LIMIT`
**Store size limit**
- **Purpose**: Set maximum size limit for Substreams stores in bytes
- **Usage**: Set as unsigned integer (bytes) to limit store memory usage
- **Default**: 1073741824 (1GiB)
- **Location**: `service/utils.go`

#### SUBSTREAMS_LOG_TOTAL_STORE_SIZE
**Log total store size**
- **Purpose**: Log total store size in bytes
- **Usage**: Set as "true" to add a "info" log when logging a store
- **Default**: false
- **Location**: `service/active_requests/active_requests.go`

### Ethereum RPC Call Configuration

#### `ETH_CALL_FALLBACK_TO_LATEST_DURATION`
**Ethereum call fallback duration**
- **Purpose**: Enable fallback to "latest" block for Ethereum eth_call/eth_getBalance operations
- **Usage**: Set as duration string (e.g., "30s", "5m") to enable fallback to "latest" when block age exceeds this threshold, and automatically retry "block not found" errors with "latest"
- **Default**: None (disabled)
- **Warning**: Enables non-deterministic behavior - requires `X-substreams-acknowledge-non-deterministic` header
- **Location**: `service/tier1.go`, `service/utils.go`

#### `ETH_CALL_USE_BLOCK_NUMBER_DURATION`
**Ethereum call block number duration**
- **Purpose**: Use block number instead of block hash for Ethereum eth_call/eth_getBalance operations
- **Usage**: Set as duration string (e.g., "30s", "5m") to use block number instead of block hash when block age exceeds this threshold
- **Default**: None (disabled)
- **Warning**: Can lead to non-deterministic behavior due to chain reorganizations - ensure that you set a duration that is high enough with 100% certainty
- **Location**: `service/tier1.go`, `service/utils.go`

#### `SUBSTREAMS_WORKERS_RAMPUP_TIME`
**Worker ramp-up timing**
- **Purpose**: Configure the time for workers to ramp up during startup
- **Usage**: Set as duration string (e.g., "30s", "2m")
- **Default**: Built-in default value
- **Location**: `orchestrator/work/globalworkerpool.go`

#### `SUBSTREAMS_WORKER_MAX_RETRIES`
**Worker retry limit**
- **Usage**: Set as integer value
- **Default**: Built-in default value
- **Location**: `orchestrator/work/worker.go`

#### `SUBSTREAMS_WORKER_MAX_TIMEOUT_RETRIES`
**Worker timeout retry limit**
- **Purpose**: Maximum number of retries specifically for timeout errors
- **Usage**: Set as integer value
- **Default**: Built-in default value
- **Location**: `orchestrator/work/worker.go`

### WASM Runtime Engine

#### `SUBSTREAMS_WASM_RUNTIME`
**WASM runtime selection**
- **Purpose**: Select the WASM runtime engine to use for executing Substreams modules
- **Usage**: Chooses between available WASM runtimes (e.g., "wasmtime") -- this is currently the only one that works
- **Default**: `wasmtime`
- **Location**: `wasm/registry.go`

#### `SUBSTREAMS_WASM_CACHE_ENABLED`
**Development-only WASM caching**
- **Purpose**: Enable WASM instance caching/reuse ⚠️ **WARNING: produces non-deterministic output**
- **Usage**: Set to "true" to enable caching for development/testing only
- **Default**: `false`
- **Warning**: **Never use in production** as it produces non-deterministic output and will poison your cache
- **Location**: `wasm/registry.go`

### Debugging and Logging

#### `SUBSTREAMS_PRINT_STACK`
**Debug stack traces**
- **Purpose**: Enable printing of stack traces for debugging execution issues
- **Usage**: Set to "true" or "1" to enable stack trace printing
- **Default**: `false`
- **Location**: `pipeline/process_block.go`

#### `SUBSTREAMS_BIGGEST_PARTIAL_BLOCK_INDEX`
**Partial block composition limit**
- **Purpose**: Configure the maximum block index used to compose the last partial block of a sequence when a request fetches "partial-blocks-only"
- **Usage**: Set as integer value to control partial block composition behavior
- **Default**: `10`
- **Location**: `pipeline/process_block.go`

### `SUBSTREAMS_DEBUG_SCHEDULER_STATE`
**Debug scheduler state**
- **Purpose**: Enable verbose logging of scheduler state changes for debugging
- **Usage**: Set to "true" to enable debug output showing scheduler transitions
- **Default**: `false`
- **Location**: `orchestrator/parallelprocessor.go`, `orchestrator/scheduler/scheduler.go`

### `SUBSTREAMS_TIER1_DEBUG_API_ADDR` and `SUBSTREAMS_TIER2_DEBUG_API_ADDR`
**Debug API address**
- **Purpose**: Listen on a specific address for debug API requests and responses
- **Usage**: If non-empty, the API will listen on the specified address for debug requests and responses
- **Default**: None
- **Location**: `service/tier2.go`

#### `SUBSTREAMS_SEND_HOSTNAME`
**Include hostname in stream headers**
- **Purpose**: Send hostname information in stream headers for debugging and monitoring
- **Usage**: Set to "true" to include hostname in metadata sent to clients
- **Default**: `false`
- **Location**: `service/tier2.go`

## Client Library Variables

### `GRPC_XDS_BOOTSTRAP`
**gRPC XDS bootstrap configuration**
- **Purpose**: Enables gRPC XDS (Extended Discovery Service) for advanced load balancing
- **Usage**: When set to a file path, enables XDS-based gRPC client configuration
- **Default**: None (uses basic gRPC client without XDS)
- **Location**: `client/client.go`

## General CLI Tool Variables

### `SF_API_TOKEN`
**StreamingFast API token fallback**
- **Purpose**: StreamingFast API token, used as fallback when specific API token environment variable is not set
- **Usage**: Fallback authentication token in various commands and tools
- **Default**: None
- **Location**: `tools/cmd.go`, `wasm/bench/generate.go`

### `SUBSTREAMS_API_KEY`
**Substreams API key fallback**
- **Purpose**: Substreams API key, used as fallback when specific API key environment variable is not set
- **Usage**: Fallback API key in various commands
- **Default**: None
- **Location**: `tools/cmd.go`

### `BUFBUILD_AUTH_TOKEN`
**Buf.build authentication**
- **Purpose**: Authentication token for Buf.build protocol buffer registry
- **Usage**: Used when loading protocol buffer descriptor sets from Buf.build
- **Default**: None (uses anonymous access)
- **Location**: `manifest/protobuf.go`

### `SUBSTREAMS_ENDPOINTS_CONFIG_{NETWORK_NAME}`
**Network endpoint configuration**
- **Purpose**: Override endpoint configuration for specific blockchain networks
- **Usage**: Dynamically configures endpoints based on network name (e.g., `SUBSTREAMS_ENDPOINTS_CONFIG_ETHEREUM`)
- **Default**: None
- **Location**: `manifest/utils.go`
