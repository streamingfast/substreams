# Migration Guide: substreams-sink → substreams/sink

This guide helps you migrate from the standalone `github.com/streamingfast/substreams-sink` library to the integrated `github.com/streamingfast/substreams/sink` package.

## Overview

The Substreams sink functionality has been consolidated into the main [Substreams repository](https://github.com/streamingfast/substreams) under the `sink/` package. This migration brings several improvements:

- Unified dependency management with Substreams core
- Enhanced flag system with inclusion/exclusion controls
- Simplified configuration via manifest-based endpoint inference
- Better CLI interface with standardized flags

## Quick Reference

| Aspect | Old (substreams-sink) | New (substreams/sink) |
|--------|----------------------|---------------------|
| **Import Path** | `github.com/streamingfast/substreams-sink` | `github.com/streamingfast/substreams/sink` |
| **Flag Exclusion** | `sink.FlagIgnore(...)` | `sink.FlagExcludeDefault(...)` |
| **Flag Interface** | `FlagIgnored` | `FlagInclusionExclusion` |
| **NewFromViper Params** | `(cmd, type, endpoint, manifest, module, blockRange, logger, tracer)` | `(cmd, type, manifest, module, userAgent, logger, tracer)` |
| **Endpoint Handling** | Required parameter | Inferred from manifest or `--endpoint` flag |
| **Block Range** | Single `blockRange` string | Separate `--start-block` and `--stop-block` flags |

## Migration Approaches

### Minimal Migration (Recommended)

This approach updates only the essential import paths and function calls while keeping deprecated elements:

```go
// Before
import "github.com/streamingfast/substreams-sink"

func setupFlags(flags *pflag.FlagSet) {
    sink.AddFlagsToSet(flags, sink.FlagIgnore(sink.FlagFinalBlocksOnly))
}

func createSinker(cmd *cobra.Command) (*sink.Sinker, error) {
    return sink.NewFromViper(
        cmd,
        "sf.substreams.database.v1.DatabaseChanges",
        endpoint,      // explicit endpoint
        manifestPath,
        moduleName,
        blockRange,    // e.g., "1000:2000"
        zlog,
        tracer,
    )
}
```

```go
// After (minimal migration)
import "github.com/streamingfast/substreams/sink"

func setupFlags(flags *pflag.FlagSet) {
    // Use deprecated but compatible FlagIgnore
    sink.AddFlagsToSet(flags, sink.FlagIgnore(sink.FlagFinalBlocksOnly))
}

func createSinker(cmd *cobra.Command) (*sink.Sinker, error) {
    return sink.NewFromViper(
        cmd,
        "sf.substreams.database.v1.DatabaseChanges",
        manifestPath,
        moduleName,
        "your-app/1.0.0", // userAgent parameter
        zlog,
        tracer,
    )
}
```

### Full Migration (Best Practices)

This approach updates to the new APIs and removes all deprecated elements:

```go
// Before
import "github.com/streamingfast/substreams-sink"

func setupFlags(flags *pflag.FlagSet) {
    sink.AddFlagsToSet(flags, sink.FlagIgnore(sink.FlagFinalBlocksOnly))
}

func createSinker(cmd *cobra.Command) (*sink.Sinker, error) {
    return sink.NewFromViper(
        cmd,
        "sf.substreams.database.v1.DatabaseChanges",
        endpoint,
        manifestPath,
        moduleName,
        blockRange,
        zlog,
        tracer,
    )
}
```

```go
// After (full migration)
import "github.com/streamingfast/substreams/sink"

func setupFlags(flags *pflag.FlagSet) {
    // Use new FlagExcludeDefault API
    sink.AddFlagsToSet(flags, sink.FlagExcludeDefault(sink.FlagFinalBlocksOnly))

    // Or include optional flags
    // sink.AddFlagsToSet(flags, sink.FlagIncludeOptional(sink.FlagCursor))
}

func createSinker(cmd *cobra.Command) (*sink.Sinker, error) {
    return sink.NewFromViper(
        cmd,
        "sf.substreams.database.v1.DatabaseChanges",
        manifestPath,
        moduleName,
        "your-app/1.0.0", // userAgent parameter
        zlog,
        tracer,
    )
}
```

## Key Changes

### 1. Import Path

```go
// Before
import "github.com/streamingfast/substreams-sink"

// After
import "github.com/streamingfast/substreams/sink"
```

### 2. NewFromViper Function Signature

The function signature has changed to use flag-based configuration:

```go
// Before
func NewFromViper(
    cmd *cobra.Command,
    expectedOutputModuleType string,
    endpoint, manifestPath, outputModuleName, blockRange string,
    zlog *zap.Logger,
    tracer logging.Tracer,
) (*Sinker, error)

// After
func NewFromViper(
    cmd *cobra.Command,
    expectedOutputModuleType string,
    manifestPath string,
    outputModuleName string,
    userAgent string,
    zlog *zap.Logger,
    tracer logging.Tracer,
) (*Sinker, error)
```

**Key differences:**
- Removed `endpoint` parameter (now handled via `--endpoint` flag or manifest inference)
- Removed `blockRange` parameter (now handled via `--start-block` and `--stop-block` flags)
- Added `userAgent` parameter for client identification

### 3. Flag Management

```go
// Before: Using deprecated interface
var ignored sink.FlagIgnored = sink.FlagIgnore(sink.FlagFinalBlocksOnly)

// After: Using new interface
var control sink.FlagInclusionExclusion = sink.FlagExcludeDefault(sink.FlagFinalBlocksOnly)

// Or include optional flags
var control sink.FlagInclusionExclusion = sink.FlagIncludeOptional(sink.FlagCursor)
```

### 4. Endpoint Configuration

```go
// Before: Explicit endpoint parameter
sinker, err := sink.NewFromViper(cmd, moduleType, "https://eth.substreams.pinax.network:443", ...)

// After: Endpoint from manifest or flag
// Option 1: Let manifest determine endpoint (recommended)
sinker, err := sink.NewFromViper(cmd, moduleType, manifestPath, ...)

// Option 2: Override with flag
// Use --endpoint flag in your CLI: --endpoint https://eth.substreams.pinax.network:443
```

**Manifest Path Formats:**
The `manifestPath` parameter now supports multiple formats:
- Local files: `./substreams.yaml` or `/path/to/substreams.yaml`
- Remote .spkg files: `https://spkg.io/streamingfast/substreams-eth-block-meta-v0.4.3.spkg`
- Short notation: `substreams_template@v0.1.0` (for packages in the registry)

### 5. Block Range Configuration

```go
// Before: Single block range string
sinker, err := sink.NewFromViper(cmd, moduleType, endpoint, manifest, module, "1000:2000", ...)

// After: Separate start/stop flags
// Use CLI flags: --start-block 1000 --stop-block 2000
sinker, err := sink.NewFromViper(cmd, moduleType, manifest, module, userAgent, ...)
```

## Available Flags

The new version provides a comprehensive set of flags. Here are the key ones:

### Default Flags (automatically included)
- `--endpoint` (-e): Substreams gRPC endpoint
- `--start-block` (-s): Start block number
- `--stop-block` (-t): Stop block number
- `--network`: Network override
- `--params` (-p): Module parameters
- `--insecure`: Skip TLS verification
- `--plaintext`: Use plaintext connection
- `--undo-buffer-size`: Undo buffer size
- `--final-blocks-only`: Process only final blocks
- `--development-mode`: Enable development mode
- `--header` (-H): Additional headers
- And more...

### Optional Flags (explicitly included)
- `--cursor` (-c): Stream from cursor

### Flag Control Examples

```go
// Exclude default flags
sink.AddFlagsToSet(flags, sink.FlagExcludeDefault(
    sink.FlagFinalBlocksOnly,
    sink.FlagDevelopmentMode,
))

// Include optional flags
sink.AddFlagsToSet(flags, sink.FlagIncludeOptional(
    sink.FlagCursor,
))

// Combine exclusions and inclusions
sink.AddFlagsToSet(flags,
    sink.FlagExcludeDefault(sink.FlagFinalBlocksOnly),
    sink.FlagIncludeOptional(sink.FlagCursor),
)
```

## Real-World Example

Here's a complete before/after example:

### Before (substreams-sink)

```go
package main

import (
    "github.com/spf13/cobra"
    "github.com/streamingfast/cli"
    "github.com/streamingfast/substreams-sink"
)

func main() {
    cli.Run("my-sink", "My custom sink",
        &cobra.Command{
            Use:   "run <endpoint> <manifest> <module> [<start>:<stop>]",
            RunE:  runCmd,
            PreRunE: func(cmd *cobra.Command, args []string) error {
                sink.AddFlagsToSet(cmd.Flags(), sink.FlagIgnore(sink.FlagFinalBlocksOnly))
                return nil
            },
        },
    )
}

func runCmd(cmd *cobra.Command, args []string) error {
    sinker, err := sink.NewFromViper(
        cmd,
        "sf.substreams.database.v1.DatabaseChanges",
        args[0], // endpoint
        args[1], // manifest
        args[2], // module
        args[3], // block range
        zlog,
        tracer,
    )
    // ... rest of implementation
}
```

### After (substreams/sink)

```go
package main

import (
    "github.com/spf13/cobra"
    "github.com/streamingfast/cli"
    "github.com/streamingfast/substreams/sink"
)

func main() {
    cli.Run("my-sink", "My custom sink",
        &cobra.Command{
            Use:   "run [<manifest> [<module>]]",
            RunE:  runCmd,
            PreRunE: func(cmd *cobra.Command, args []string) error {
                // Use new API
                sink.AddFlagsToSet(cmd.Flags(), sink.FlagExcludeDefault(sink.FlagFinalBlocksOnly))
                return nil
            },
        },
    )
}

func runCmd(cmd *cobra.Command, args []string) error {
    manifestPath := "substreams.yaml" // default
    moduleName := ""                  // infer from manifest

    if len(args) > 0 {
        manifestPath = args[0]
    }
    if len(args) > 1 {
        moduleName = args[1]
    }

    sinker, err := sink.NewFromViper(
        cmd,
        "sf.substreams.database.v1.DatabaseChanges",
        manifestPath,
        moduleName,
        "my-sink/1.0.0", // userAgent
        zlog,
        tracer,
    )
    // ... rest of implementation
}
```

## CLI Usage Changes

### Before
```bash
my-sink run https://eth.substreams.pinax.network:443 ./substreams.yaml map_transfers 1000:2000
```

### After
```bash
# Basic usage with local manifest
my-sink run ./substreams.yaml map_transfers --start-block 1000 --stop-block 2000

# Using remote .spkg files
my-sink run https://spkg.io/streamingfast/substreams-eth-block-meta-v0.4.3.spkg db_out

# Using short notation for registry packages
my-sink run substreams_template@v0.1.0

# Override endpoint explicitly if needed
my-sink run ./substreams.yaml map_transfers --endpoint https://eth.substreams.pinax.network:443 --start-block 1000 --stop-block 2000

# Endpoint is inferred from manifest network field when not specified
```

## Troubleshooting

### Common Issues

1. **Missing userAgent parameter**: Add a userAgent string to your `NewFromViper` call
2. **Endpoint not found**: Ensure your manifest has proper network configuration or use `--endpoint` flag
3. **Flag not found errors**: Update flag exclusion calls from `FlagIgnore` to `FlagExcludeDefault`

### Compatibility

The new version maintains backward compatibility through deprecated APIs, allowing for gradual migration. However, we recommend updating to the new APIs for better maintainability.

## Conclusion

This migration brings your sink implementation up to date with the latest Substreams architecture while providing more flexibility and better CLI ergonomics. The minimal migration path allows for quick updates, while the full migration provides access to all new features and best practices.