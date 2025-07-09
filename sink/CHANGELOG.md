# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## v0.4.2

* Bumped substreams to v1.10.3
* Add support for new manifest attributes like 'protobuf.excludePaths'

## v0.4.1

* Bumped substreams to v1.8.1
* Add a `NoopMode` enabling to request tier1 in a noop mode (Processing no data while being live). 

## v0.4.0

* Bumped substreams to v1.7.3
* Enable gzip compression on substreams data on the wire

## v0.3.5

* Fix another case where 'infinite-retry' would not work and the program would stop on an error.
* Enable multiple substreams authentication methods (API key, JWT), using flags `--api-key-envvar` and `--api-token-envvar`. 
* Deprecates the use of `SF_API_TOKEN` environment variable, now use default `SUBSTREAMS_API_TOKEN` or set your own using `--api-token-envvar`

## v0.3.4

* Fixed spurious error reporting when the sinker is terminating or has been canceled.

* Updated `substreams` dependency to latest version `v1.3.7`.

## v0.3.3

* Improved `substreams stream stats` log line but now using `substreams_sink_progress_message_total_processed_blocks` for `progress_block_rate` replacing the `progress_msg_rate` which wasn't meaningful anymore (and broken because the metric was never updated).

* Fixed a crash when providing a single block argument for block range if it's the same as the Substreams' start block.

* Added `--network` flag and handling proper handling.

## v0.3.2

* It's now possible to define on your handler the method `HandleBlockRangeCompletion(ctx context.Context, cursor *sink.Cursor) error` which will be called back when the `sink.Sinker` instance fully completed the request block range (infinite streaming or terminate because of an error does not trigger it).

## v0.3.1

### Substreams Progress Messages

* Bumped substreams to `v1.1.12` to support the new progress message format. Progression now relates to **stages** instead of modules. You can get stage information using the `substreams info` command starting at version `v1.1.12`.

#### Changed Prometheus Metrics

* `substreams_sink_progress_message_last_end_block` removed in favor of `substreams_sink_progress_message_last_block` (per stage)

#### Added Prometheus Metrics

* Added `substreams_sink_progress_message_total_processed_blocks`
* Added `substreams_sink_progress_message_last_block`
* Added `substreams_sink_progress_message_last_contiguous_block` (per stage)
* Added `substreams_sink_progress_message_running_jobs`(per stage)

### Other changes

* Added manifest path in error message when failing to read it.
* When back off limit expires, the returned error now also contains the last retryable error unwrapped received.

## v0.3.0

### Changed

#### CLI

* **Breaking** Flag shorthand `-p` for `--plaintext` has been re-assigned to Substreams params definition, to align with `substreams run/gui` on that aspect. There is no shorthand anymore for `--plaintext`.

  If you were using before `-p`, please convert to `--plaintext`.

  > **Note** We expect that this is affecting very few users as `--plaintext` is usually used only on developers machine.

#### Library

* gRPC `InvalidArgument` error(s) are not retried anymore like specifying and invalid start block or argument in your request.

* **Breaking** `ReadManifestAndModule` signature changed to add `params []string`, this is required for proper computation of module's output hash, upgrade by passing `nil` for this parameter.

* **Breaking** `ReadManifestAndModuleAndBlockRange` signature changed to add `params []string`, this is required for proper computation of module's output hash, upgrade by passing `nil` for this parameter.

### Added

* Added support for `--params, -p` (can be repeated multiple times) on the form `-p <module>=<value>`.

* Added logging of new `Session` received values (`linear_handoff_block`, `max_parallel_workers` and `resolved_start_block`).

* Added `--header, -H` (can be repeated multiple times) flag to pass extra headers to the server.

* Added `ClientConfig` on `sink.Sinker` instance to retrieve Substreams client configuration easily.

* Added `ReadManifestAndModuleAndBlockRange` as a convenience for `ReadManifestAndModule` and `ReadBlockRange`.

* Added `ReadBlockRange` so that it's possible to easily read a block range argument against a Substreams parsed module.

### Fixed

* The `stop_block` for infinite streaming passed to the server is now 0, it seems `MaxUint64` was causing some issues on the server, should result normally in better startup performance now.

## v0.2.6

### Added

* Added support for multiple expected output types in `sink.NewFromViper` and `sink.ReadManifestAndModule`, facilitates simple package id renames.

* Added `sink.ReadManifestAndModule` helper to easily get the full `sink` logic to load a manifest (expected output type, checks, etc.).

## v0.2.5

### Fixed

* Fixed reported `data_msg` and `progress_msg` on exit to be the real final value from the counter.

* Fixed the `end_at` block reported in a log statement.

### Changed

* Moved `sink.NewRetryableError` to [derr package](https://github.com/streamingfast/derr)

## v0.2.4

### Added

* It's now possible to pass `sink.WithRetryBackOff(backOff)` instance instead of using the default configured back off (only from code).

  > **Note** If `infinite-retry == false` is configured on the sinker, your back off will be wrapped with `backoff.WithMaxRetries(backOff, 15)`, this is not yet configurable.

## v0.2.3

### Changed

* Improved what can be parsed as a block range value ([see spec](./README.md#accepted-block-range)).

## v0.2.2

### Fixed

* Fixed `cursor` not being updated when sink has no undo buffer configured.

### Added

* Added possibility to disable block buffer by specifying `WithBlockDataBuffer(0)`.
* Added possibility to call `sinker.ApiToken()` and `sinker.EndpointConfig()` to retrieve client configuration of the sinker instance.
* Added new Prometheus metric `head_block_time_drift{service=substreams_sink}` to record the head block time drift against `now`.
* Added new Prometheus metric `substreams_sink_message_size_bytes` to record messages total bytes received from Substreams server.
* Added new Prometheus metric `substreams_sink_data_message_size_bytes` to record `BlockScopedProgressData` messages total bytes received from Substreams server.
* Added new Prometheus metric `substreams_sink_progress_message_last_end_block` to record received `ModuleProgress` reporting progress last processed block.
* Added new Prometheus metric `substreams_sink_unknown_message` to record total message of unknown type received (could happen if server send messages that client don't understand due to version mistmatch).
* Added new Prometheus metric `substreams_sink_backprocessing_completion` to record that backprocessing (if scheduled for your request is now completed, note that forward parallel execution could still be active).

## v0.2.1

### Added

* Added possibility to ignore flag to set when using `sink.AddFlagsToSet`, for example:

  ```go
  sink.AddFlagsToSet(flags, sink.FlagIgnore(sink.FlagFinalBlocksOnly))
  ```

### Changed

* When specifying `WithFinalBlocksOnly()`, undo buffer is automatically sets to 0 (no buffering).

### Fixed

* Fixed bug when flag are ignored and not defined.

## v0.2.0

### Highlights

#### Update to `sf.substreams.rpc.v2` protocol and changed re-org handling

We have now updated the sink to use `sf.substreams.rpc.v2` protocol for communicating with the Substreams backend. This new protocol introduces a quite different way of receiving undo signal(s) from Substreams.

Each occurrences of `pbsubstreams.Request`, `pbsubstreams.Response`, `pbsubstreams.BlockScopedData` (and a few others) must be moved to `pbsubstreamsrpc.<Name>` where import statement is `pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"`. Note that `pbsubstreams.ForkStep` has been removed completely, below we discuss how to determine final block height.

You must now pass a `sink.BlockUndoSignalHandler` when running your `sink.Sinker` instance, this new handler will receive undo signal which are now just contains the last canonical block to revert back to and the cursor to use.

While previously you would receive steps like:

1. `BlockScopedData` (Step: `New`, Block #5a, Cursor `a`)
1. `BlockScopedData` (Step: `New`, Block #6b, Cursor `b`)
1. `BlockScopedData` (Step: `New`, Block #7b, Cursor `c`)
1. `BlockScopedData` (Step: `Undo`, Block #7b, Cursor `c`)
1. `BlockScopedData` (Step: `Undo`, Block #6b, Cursor `b`)
1. `BlockScopedData` (Step: `New`, Block #6a, Cursor `e`)

Now the signalling will be like:

1. `BlockScopedData` (Block #5a, Cursor `a`)
1. `BlockScopedData` (Block #6b, Cursor `b`)
1. `BlockScopedData` (Block #7b, Cursor `c`)
1. `BlockUndoSignal` (Block #5a, Cursor `a'`)
1. `BlockScopedData` (Block #6a, Cursor `e`)

Now a `BlockUndoSignal` must be treated as "delete every data that has been recorded after block height specified by block in BlockUndoSignal". In the example above, this means we must delete changes done by `Block #7b` and `Block #6b`. The exact details depends on your own logic. If for example all your added record contain a block number, a simple way is to do `delete all records where block_num > 5` which is the block num received in the `BlockUndoSignal` (this is true for append only records, so when only `INSERT` are allowed).

The `pbsubstreams.ForkStep` has been removed completely. The default behavior is to send `pbsubstreamsrpc.BlockScopedData` and `pbsubstreamsprc.BlockUndoSignal` which corresponds to old `ForkStepNew` and `ForkStepUndo`. The `ForkStepIrreversible` are not sent anymore. Instead, the `pbsubstreamsrpc.BlockScopedData` message gained a filed `FinalBlockHeight` which determines for the received block at which block height Substreams is considering blocks to be final. If you wish to only receive final blocks, you can use `sink.WithFinalBlocksOnly` sinker `Option`.

The `sink.Sinker` and `sink.New(...)` change in signature also to make nesting easier. Now the handlers must be passed when calling `Run`. The constructor also removed the possibility to pass which `ForkStep` to handle.

The `sink.BlockScopedDataHandler` signature changed, the message is now the first argument, the second argument determines if the block is live, it will be non-nil if a `sink.WithLivenessChecker` has been configured. If the liveness checker determines the block is live, current implementation checks if block's timestamp is within defined delta, the `isLive` will be pointing to a non-nil `true` value, otherwise a non-nil `false` value. Finally the cursor is the last element.

The `sink.BlockUndoSignalHandler` must be defined to correctly received undo signal from the Substreams RPC. If can be left `nil` only if `sink.WithFinalBlocksOnly` is configured. How you handle the undo signal is left on the consumer.

##### Before

```go
handler := func(ctx context.Context, cursor *sink.Cursor, data *pbsubstreams.BlockScopedData) error { ... }
steps := []pbsubstreams.ForkStep{pbsubstreams.ForkStep_STEP_NEW, pbsubstreams.ForkStep_STEP_UNDO}

sinkOptions := []sink.Option{...}
// If you had `steps := []pbsubstreams.ForkStep{pbsubstreams.ForkStep_STEP_IRREVERSIBLE}`, now pass `sink.WithFinalBlocksOnly` as a `SinkOption` instead
// sinkOptions := append(sinkOptions, sink.WithFinalBlocksOnly())

sink, err = sink.New(
	mode,
	s.Pkg.Modules,
	s.OutputModule,
	s.OutputModuleHash,
	s.handleBlockScopeData,
	s.ClientConfig,
	steps,
	s.logger,
	s.tracer,
	sinkOptions...,
)
if err != nil { ... }

if err := s.sink.Start(ctx, s.blockRange, cursor); err != nil {
    return fmt.Errorf("sink failed: %w", err)
}
```

##### After

```go
handleData := func(ctx context.Context, data *pbsubstreamsrpc.BlockScopedData, isLive *bool, cursor *Cursor) error { ... }
handleUndo := func(ctx context.Context, undo *pbsubstreamsrpc.BlockUndoSignal, cursor *Cursor) error { ... }

sinkOptions := []sink.Option{sink.WithBlockRange(s.blockRange)}
// If you had `steps := []pbsubstreams.ForkStep{pbsubstreams.ForkStep_STEP_IRREVERSIBLE}`, now pass `sink.WithFinalBlocksOnly` as a `SinkOption` instead
// sinkOptions := append(sinkOptions, sink.WithFinalBlocksOnly())

sink, err = sink.New(
	mode,
	s.Pkg.Modules,
	s.OutputModule,
	s.OutputModuleHash,
	s.ClientConfig,
	s.logger,
	s.tracer,
	sinkOptions...,
)
if err != nil { ... }

sink.OnTerminating(func (err error) {
    if err != nil {
        // Sinker failed, do something with err
    }
})

go sink.Run(ctx, cursor, sink.NewSinkerHandlers(handleData, handleUndo))
```

#### Liveness Checker

The `sink.Sinker` library added support for checking if a block is live or not. You can configure our `sink.DeltaLivenessChecker` check passing it as an `Option` on `sink.New(..., sink.WithLivenessChecker(sink.NewDeltaLivenessChecker(delta)))`. The `sink.DeltaLivenessChecker` determines that a block is live is `time.Now() - block.Timestamp() <= delta`.

Once a liveness checker is configured, the `isLive` argument in your `sink.BlockScopedDataHandler` will start to be non-nil and the value will be result of having called `sink.DeltaLivenessChecker.IsLive(block)`.

#### Retryable Errors

Errors coming out of the handler(s) are not retried by default anymore. This because errors coming out of your handler are usually not retryable. If you wish to keep this behavior, you can use `sink.NewRetryableError(err)` to make it back retryable.

#### Cursor

The `sink.Cursor` backing implementation changed which will now avoid the need to pass to which block the cursor points to. The block pointed to is now extracted directly from the cursor value.

- `sink.NewCursor(cursor, block)` becomes simply `sink.NewCursor(cursor)`.

### Added

- Added `sink.NewFromViper` to easily create an instance from viper predefined flags, expected that `cli.ConfigureViper` or `cli.ConfigureViperForCommand` has been used, use `sink.AddFlagsToSet` to add flags to a flag set.
    - This new method make it much easier to maintain the a sink flags and configures it from flags. This changelog will list changes made to flags so when updating, you can copy it over to your own changelog.

- Added `sink.AddFlagsToSet` to easily add all sinker flags to a `pflag.FlagSet` instance.

- Added `sink.WithBlockRange` sinker `Option` to limit the `Sinker` to a specific range (runs for whole chain if unset).

- Added `sink.WithLivenessChecker` sinker `Option` to configure a liveness check on the sinker instance.

### Changed

- Stats are printed each 15s when logger level is info or higher and 5s when it's debug or lower.

- **Deprecation** The flag `--irreversible-only` is deprecated, use `--final-blocks-only` instead.

- **Breaking** The `sink.New` signature changed, handlers

- **Breaking** The `sink.New` signature changed
    - The `sink.BlockScopedDataHandler` handler must not be passed in the constructor anymore.
    - The `forkSteps` arguments has been removed.

- **Breaking** The `sink.Sinker` field `BlockScopedDataHandler` has been removed (renamed and made private).

- **Breaking** Type name `sink.BlockScopeDataHandler` signature changed, the argument `data` is now of type `pbsubstreamsrpc.BlockScopedData` (imported via `pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"`).

## v0.1.0

- **Breaking** Type name `sink.BlockScopeDataHandler` has been renamed to `sink.BlockScopedDataHandler` (note the `d` on `Scoped`).
First official release of the library, latest release until refactor to support the new upcoming Substreams V2 RPC protocol.
