## Substreams Sink

This is a Substreams Sink library. You can use to build any sink application that consumes Substreams in Golang

### Features
What you get by using this library:

- Handles connection and reconnections
- Throughput Logging (block rates, etc)
- Best Practices error handling

### Usage

The library provides a `Sinker` class that can be used to connect to the Substreams API. The `Sinker` class is a wrapper around the `substreams` library, which is a low-level library that provides a convenient way to connect to the Substreams API.

The user's primary responsibility when creating a custom sink is to implement handlers for processing Substreams data. The sink library provides two main patterns for creating handlers:

### Basic Handlers

For simple use cases, you can use `NewSinkerHandlers` which requires only the essential handlers:

```go
import (
	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
)

handlers := sink.NewSinkerHandlers(
	handleBlockScopedData func(ctx context.Context, data *pbsubstreamsrpc.BlockScopedData, isLive *bool, cursor *Cursor) error,
	handleBlockUndoSignal func(ctx context.Context, undoSignal *pbsubstreamsrpc.BlockUndoSignal, cursor *Cursor) error,
)
```

### Full Handlers

For advanced use cases that need session initialization, progress tracking, and debug capabilities, use `NewSinkerFullHandlers`:

```go
handlers := sink.NewSinkerFullHandlers(
	handleBlockScopedData func(ctx context.Context, data *pbsubstreamsrpc.BlockScopedData, isLive *bool, cursor *Cursor) error,
	handleBlockUndoSignal func(ctx context.Context, undoSignal *pbsubstreamsrpc.BlockUndoSignal, cursor *Cursor) error,
	handleSessionInit func(ctx context.Context, req *pbsubstreamsrpc.Request, session *pbsubstreamsrpc.SessionInit) error,
	handleProgress func(ctx context.Context, progress *pbsubstreamsrpc.ModulesProgress),
	handleInitialSnapshotData func(ctx context.Context, debug *pbsubstreamsrpc.InitialSnapshotData) error,
	handleInitialSnapshotComplete func(ctx context.Context, complete *pbsubstreamsrpc.InitialSnapshotComplete) error,
	handleError func(ctx context.Context, error *pbsubstreamsrpc.Error),
)
```

We invite you to take a look at our:
- [Basic Example](./examples/basic/main.go) (and its accompanying [README.md](./examples/basic/README.md))
- [Advanced Example](./examples/advanced/main.go) (and its accompanying [README.md](./examples/advanced/README.md))

> [!NOTE]
> We highly recommend to use the [Advanced Example](./examples/advanced/main.go) example for any serious sink implementation!

### Handler Details

#### Required Handlers

##### `handleBlockScopedData` (Required)

* `ctx context.Context` is the `sink.Sinker` actual `context.Context`.
* `data *pbsubstreamsrpc.BlockScopedData` contains the data that was received from the Substreams API, refer to it's definition for proper usage.
* `isLive *bool` will be non-nil if a LivenessChecker has been configured on the Sinker instance. When non-nil, `*isLive` indicates whether the current block being processed is considered "live" (recently produced) or historical.
* `cursor *Cursor` is the cursor at the given block, this cursor should be saved regularly as a checkpoint in case the process is interrupted.

##### `handleBlockUndoSignal` (Required)

* `ctx context.Context` is the `sink.Sinker` actual `context.Context`.
* `undoSignal *pbsubstreamsrpc.BlockUndoSignal` contains the last valid block that is still valid, any data saved after this last saved block should be discarded.
* `cursor *Cursor` is the cursor to use after the undo, this cursor should be saved regularly as a checkpoint in case the process is interrupted.

#### Optional Handlers (Available in NewSinkerFullHandlers)

##### `handleSessionInit` (Optional)

Called when a new session is initialized with the Substreams server. This is useful for logging session information or performing setup tasks.

* `ctx context.Context` is the `sink.Sinker` actual `context.Context`.
* `req *pbsubstreamsrpc.Request` is the request that was sent to the Substreams API.
* `session *pbsubstreamsrpc.SessionInit` contains the session initialization data including trace ID, resolved start block, and linear handoff information.

##### `handleProgress` (Optional)

Called periodically to report processing progress. Useful for monitoring and displaying progress information.

* `ctx context.Context` is the `sink.Sinker` actual `context.Context`.
* `progress *pbsubstreamsrpc.ModulesProgress` contains progress information for each module being processed, including processed block ranges and execution statistics.

##### `handleInitialSnapshotData` (Optional)

Called when initial snapshot data is available for store modules. Only called in development mode when debug snapshots are enabled.

* `ctx context.Context` is the `sink.Sinker` actual `context.Context`.
* `snapshotData *pbsubstreamsrpc.InitialSnapshotData` contains the initial state data for store modules, useful for debugging.

##### `handleInitialSnapshotComplete` (Optional)

Called when all initial snapshot data has been delivered. Signals that the snapshot phase is complete.

* `ctx context.Context` is the `sink.Sinker` actual `context.Context`.
* `complete *pbsubstreamsrpc.InitialSnapshotComplete` indicates completion of the initial snapshot delivery.

##### `handleError` (Optional)

Called when errors are received from the Substreams server. Useful for custom error logging or handling.

* `ctx context.Context` is the `sink.Sinker` actual `context.Context`.
* `error *pbsubstreamsrpc.Error` contains error information from the Substreams API.

### Alternative: Interface-Based Handlers

Instead of using function-based handlers, you can implement the handler interfaces directly. This approach provides better type safety and organization for complex sinks:

```go
// Implement the required interface
type MySink struct {
    // your sink fields
}

func (s *MySink) HandleBlockScopedData(ctx context.Context, data *pbsubstreamsrpc.BlockScopedData, isLive *bool, cursor *Cursor) error {
    // your implementation
}

func (s *MySink) HandleBlockUndoSignal(ctx context.Context, undoSignal *pbsubstreamsrpc.BlockUndoSignal, cursor *Cursor) error {
    // your implementation
}

// Optional interfaces you can implement
func (s *MySink) HandleProgress(ctx context.Context, progress *pbsubstreamsrpc.ModulesProgress) {
    // your implementation
}

func (s *MySink) HandleSessionInit(ctx context.Context, req *pbsubstreamsrpc.Request, session *pbsubstreamsrpc.SessionInit) error {
    // your implementation
}

func (s *MySink) HandleInitialSnapshotData(ctx context.Context, snapshotData *pbsubstreamsrpc.InitialSnapshotData) error {
    // your implementation
}

func (s *MySink) HandleInitialSnapshotComplete(ctx context.Context, complete *pbsubstreamsrpc.InitialSnapshotComplete) error {
    // your implementation
}

func (s *MySink) HandleError(ctx context.Context, err error) {
    // your implementation
}

func (s *MySink) HandleBlockRangeCompletion(ctx context.Context, cursor *Cursor) error {
    // Called when the sinker finishes processing the requested block range
    // Only called when streaming has a defined end block (not in live mode)
    // your implementation
}

// Pass your sink directly to the sinker
mySink := &MySink{}
sinker.Run(ctx, cursor, mySink)
```

The sink library automatically detects which interfaces your handler implements and calls the appropriate methods.

#### Handlers Flow

The basic pattern for using the `Sinker` is as follows:

1. **Setup Phase**: Create your data layer responsible for decoding Substreams data and saving it to desired storage
2. **Handler Implementation**: Implement the required handlers (either using functions or interfaces)
3. **Sinker Creation**: Create a `Sinker` object using `sink.New` with your configuration
4. **Handler Registration**: Pass your handlers to the sinker using `sinker.Run(ctx, cursor, handlers)`

##### Execution Flow

When the sinker is running, handlers are called in the following order:

1. **`HandleSessionInit`** (if implemented) - Called once when the session starts
2. **`HandleInitialSnapshotData`** (if implemented) - Called for each initial snapshot in development mode
3. **`HandleInitialSnapshotComplete`** (if implemented) - Called when all snapshots are delivered
4. **`HandleProgress`** (if implemented) - Called periodically during processing
5. **`HandleBlockScopedData`** (required) - Called for each block's data
6. **`HandleBlockUndoSignal`** (required) - Called when blockchain reorganizations occur
7. **`HandleError`** (if implemented) - Called when errors are received from the server
8. **`HandleBlockRangeCompletion`** (if implemented) - Called when the specified block range is fully processed

##### Data Processing

The `HandleBlockScopedData` handler is called for each block scoped data message received from the Substreams API. It contains all the data output for the given Substreams module. In your handler, you are responsible for:

- Decoding and processing the data
- Saving the data to your storage system
- Persisting the cursor for checkpoint recovery
- Returning an error if there's a problem (which will trigger retries or termination based on configuration)

##### Handling Blockchain Reorganizations

The `HandleBlockUndoSignal` handler is called when a blockchain reorganization (fork) occurs. The message contains:

- `LastValidBlock`: Points to the last block that should be assumed to be part of the canonical chain
- `LastValidCursor`: The cursor that should be used as the current active position

**Important**: You must treat every piece of data from blocks where `BlockScopedData.Clock.Number > LastValidBlock.Number` as invalid and remove it from your storage. For example, if your entities include block numbers, you should delete all entities where `blockNumber > LastValidBlock.Number`.

##### Error Handling

The sink library provides robust error handling:

- **Retryable Errors**: Wrap errors in `derr.NewRetryableError(err)` to indicate they should be retried
- **Fatal Errors**: Return unwrapped errors to indicate fatal conditions that should stop processing
- **Backoff Strategy**: The library uses exponential backoff for retryable errors
- **Infinite Retry**: Can be configured for production deployments that should never stop

### Practical Example

See examples from the 'examples' folder

#### From Viper

Our sink(s) are all using Viper/Cobra and a set of public StreamingFast libraries to deal with flags, logging, environment variables, etc. If you use the same structure as us, you can benefit from `sink.AddFlagsToSet` and `sink.ConfigFromViper` which both perform most of the boilerplate to bootstrap a sinker from flags.

#### Configuration Options

The `SinkerConfig` struct provides extensive configuration options:

```go
config := &sink.SinkerConfig{
    // Required: Substreams package and output module
    Pkg:          pkg,                    // *pbsubstreams.Package
    OutputModule: outputModule,           // *pbsubstreams.Module

    // Connection settings
    ClientConfig: clientConfig,           // *client.SubstreamsClientConfig

    // Block range
    StartBlock:   0,                      // int64
    StopBlock:    0,                      // uint64 (0 means live streaming)

    // Processing mode
    Mode:         sink.SubstreamsModeDevelopment, // or SubstreamsModeProduction

    // Error handling
    InfiniteRetry: true,                  // bool - retry forever on errors

    // Performance tuning
    FinalBlocksOnly: false,               // bool - disable undo handling for faster processing
    UndoBufferSize:  12,                  // int - number of blocks to buffer for undo handling

    // Development features
    DevOutputModules:    []string{},      // Debug specific modules
    DevOutputSnapshots:  []string{},      // Get initial snapshots for stores

    // Monitoring
    LivenessChecker: livenessChecker,     // *sink.LivenessChecker (optional)

    // Logging
    Logger: logger,                       // *zap.Logger
    Tracer: tracer,                       // *otellib.Tracer
}
```

### Launching

The sinker can be launched by calling the `Run` method on the `Sinker` object. The `Run` method will block until the sinker is stopped or encounters an error.

```go
ctx := context.Background()
cursor, err := sink.NewCursor("") // Start from beginning, or load from checkpoint
if err != nil {
    return fmt.Errorf("creating cursor: %w", err)
}

// Run the sinker with your handlers
err = sinker.Run(ctx, cursor, handlers)
if err != nil {
    return fmt.Errorf("sinker failed: %w", err)
}
```

The sinker implements the [shutter](https://github.com/streamingfast/shutter/blob/develop/shutter.go) interface which can be used to handle all shutdown logic (e.g., flushing any remaining data to storage, stopping the sink in case of database disconnection, etc.).

#### Graceful Shutdown

You can implement graceful shutdown using context cancellation:

```go
ctx, cancel := context.WithCancel(context.Background())

// Handle shutdown signals
go func() {
    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
    <-sigChan
    fmt.Println("Shutting down gracefully...")
    cancel()
}()

err := sinker.Run(ctx, cursor, handlers)
if err != nil && err != context.Canceled {
    log.Fatalf("Sinker error: %v", err)
}
```

#### Monitoring and Statistics

The sinker provides built-in statistics that can be accessed after completion:

```go
sinker.Run(ctx, cursor, handlers)

// Print statistics
fmt.Printf("Total Processed Bytes: %d\n", sink.ProgressMessageProcessedBytes.Get())
fmt.Printf("Total Processed Blocks: %d\n", sink.ProgressMessageTotalProcessedBlocks.Get())
fmt.Printf("Total Received Bytes: %d\n", sink.DataMessageSizeBytes.Get())
```

### Example Uses

The following repositories are production examples of how the sink library can be used:

* [substreams-sink-mongodb](https://github.com/streamingfast/substreams-sink-mongodb) - MongoDB sink implementation
* [substreams-sink-postgres](https://github.com/streamingfast/substreams-sink-postgres) - PostgreSQL sink implementation
* [substreams-sink-files](https://github.com/streamingfast/substreams-sink-files) - File-based sink implementation
* [substreams-sink-webhook](https://github.com/streamingfast/substreams-sink-webhook) - Webhook sink for HTTP endpoints

These examples demonstrate various patterns including database integration, file output, and HTTP webhook delivery.
