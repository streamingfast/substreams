# protodecode

The `protodecode` package provides protocol buffer decoding functionality for Substreams TUI and other components.

## Overview

This package was extracted from the TUI package to provide a reusable decoder for protocol buffer messages. It handles the decoding of dynamic protobuf messages from Substreams modules.

## Key Components

### Decoder

The `Decoder` struct is the main component that handles:
- Message descriptor management
- Message type mapping
- Dynamic message decoding for both map outputs and store deltas

### OutputStreamPattern

Supports pattern matching for output stream names using regular expressions or exact string matching.

## Usage

### Pattern-based Decoder (for TUI package)

```go
package main

import (
    "github.com/streamingfast/substreams/protodecode"
    pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

func main() {
    // Create a decoder for a package with pattern matching
    decoder, err := protodecode.NewDecoder(pkg, outputStreamNames)
    if err != nil {
        panic(err)
    }
    
    // Check if a module has a message type
    if decoder.HasMessageType("my_module") {
        msgType := decoder.GetMessageType("my_module")
        msgDesc := decoder.GetMessageDescriptor("my_module")
        
        // Decode a dynamic message (returns only data content)
        dataContent := decoder.DecodeDynamicMessage(msgDesc, anyMessage)
        
        // Wrap with metadata if needed
        wrappedResult, err := decoder.WrapMessage(msgType, blockNum, modName, dataContent)
        
        // Or decode store deltas
        deltaResult := decoder.DecodeDynamicStoreDeltas(msgType, msgDesc, deltaBytes)
    }
}
```

### Manifest-based Decoder (for TUI2 package)

```go
package main

import (
    "github.com/streamingfast/substreams/protodecode"
    "github.com/streamingfast/substreams/manifest"
    pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

func main() {
    // Build message descriptors from the package
    msgDescs, err := manifest.BuildMessageDescriptors(pkg)
    if err != nil {
        panic(err)
    }
    
    // Create a decoder from manifest descriptors
    decoder, err := protodecode.NewDecoderFromManifest(pkg, msgDescs)
    if err != nil {
        panic(err)
    }
    
    // Set formatting options for pretty printing
    decoder.SetFormatting("  ", true)
    
    // Use the decoder as before
    if decoder.HasMessageType("my_module") {
        msgType := decoder.GetMessageType("my_module")
        msgDesc := decoder.GetMessageDescriptor("my_module")
        
        // Returns only the data content
        dataContent := decoder.DecodeDynamicMessage(msgDesc, anyMessage)
        
        // Wrap with metadata if full format is needed
        result, err := decoder.WrapMessage(msgType, blockNum, modName, dataContent)
    }
}
```

## Features

- **Dynamic Message Decoding**: Decode protobuf messages without compile-time knowledge of their structure
- **Store Delta Decoding**: Handle store delta decoding with proper type handling
- **Pattern Matching**: Support for regex and exact string matching for output stream names
- **Manifest Integration**: Direct integration with manifest.ModuleDescriptor for TUI2 compatibility
- **Flexible Formatting**: Configurable JSON output formatting with indentation and default values
- **Error Handling**: Graceful handling of unknown types and decoding errors
- **JSON Output**: Clean data content output with optional metadata wrapping

## API Reference

### Constructors

- `NewDecoder(pkg, outputStreamNames)` - Creates a decoder with pattern matching (used by TUI)
- `NewDecoderFromManifest(pkg, msgDescs)` - Creates a decoder from manifest descriptors (used by TUI2)

### Methods

- `HasMessageType(moduleName)` - Check if a module has a message type
- `GetMessageType(moduleName)` - Get the message type for a module
- `GetMessageDescriptor(moduleName)` - Get the message descriptor for a module
- `DecodeDynamicMessage(msgDesc, anyMessage)` - Decode a map output message (returns json.RawMessage with data content only)
- `DecodeDynamicStoreDeltas(msgType, msgDesc, deltaBytes)` - Decode store delta bytes (returns json.RawMessage)
- `WrapMessage(msgType, blockNum, modName, data)` - Wrap json.RawMessage data with metadata (@module, @block, @type, @data)
- `SetFormatting(indent, emitDefaults)` - Configure JSON formatting options

## Error Handling

The decoder handles various error conditions:
- Unknown message types (returns `UnknownWrap`)
- Decoding errors (returns `ErrorWrap`)
- Invalid protobuf data (returns formatted error messages)

## Thread Safety

The decoder is designed to be thread-safe for read operations once initialized. The internal maps are populated during initialization and not modified afterward.