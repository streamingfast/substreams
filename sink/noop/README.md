# Noop Sink

The noop sink is a minimal sink implementation that receives very little data from the server but allows it to prepare the server-side cache, keeping it hot.

## Purpose

The noop sink is designed for preparing the caches on the server, particularly when using stores

## Features

- **Cursor Management**: Saves and loads cursor state from a file
- **No Data Processing**: Receives data but performs no operations on it
- **Minimal Resource Usage**: Very lightweight operation
- **Statistics Reporting**: Provides standard sink statistics

## Usage

```bash
substreams sink noop [manifest] [module_name]
```

### Flags

- `--state-file`: File where the sink will store its cursor (default: `./state.cursor`)
- Standard substreams flags (endpoint, start-block, stop-block, etc.)

## Configuration

The noop sink automatically enables `noop-mode` in the SinkerConfig, which tells the substreams server to not send actual data but only populate the cache

## Example

```bash
# Run noop sink with default settings
substreams sink noop

# Run with custom manifest and state file
substreams sink noop ./my-substreams.yaml my_module --state-file ./noop.cursor
```
