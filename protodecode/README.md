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

```go
package main

import (
    "github.com/streamingfast/substreams/protodecode"
    pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

func main() {
    // Create a decoder for a package
    decoder, err := protodecode.NewDecoder(pkg, outputStreamNames)
    if err != nil {
        panic(err)
    }
    
    // Check if a module has a message type
    if decoder.HasMessageType("my_module") {
        msgType := decoder.GetMessageType("my_module")
        msgDesc := decoder.GetMessageDescriptor("my_module")
        
        // Decode a dynamic message
        result := decoder.DecodeDynamicMessage(msgType, msgDesc, blockNum, modName, anyMessage)
        
        // Or decode store deltas
        deltaResult := decoder.DecodeDynamicStoreDeltas(msgType, msgDesc, blockNum, modName, deltaBytes)
    }
}
```

## Features

- **Dynamic Message Decoding**: Decode protobuf messages without compile-time knowledge of their structure
- **Store Delta Decoding**: Handle store delta decoding with proper type handling
- **Pattern Matching**: Support for regex and exact string matching for output stream names
- **Error Handling**: Graceful handling of unknown types and decoding errors
- **JSON Output**: Structured JSON output with module metadata

## Error Handling

The decoder handles various error conditions:
- Unknown message types (returns `UnknownWrap`)
- Decoding errors (returns `ErrorWrap`)
- Invalid protobuf data (returns formatted error messages)

## Thread Safety

The decoder is designed to be thread-safe for read operations once initialized. The internal maps are populated during initialization and not modified afterward.