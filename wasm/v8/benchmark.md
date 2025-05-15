# Substreams Javascript runtime integration 
This project aims to integrate the V8 JavaScript engine into Substreams in order to execute Substream modules written in JavaScript/TypeScript. The V8 engine is integrated via the v8go library, and replaces traditional WASM execution.

# V8 engine main files
module.go: Loads and executes JS scripts (polyfill, prelude, bundle).

instance.go: Manages V8 execution contexts (V8Instance).

factory.go: Creates a V8 module from JavaScript code (precompiled).

integration.go: Registers the engine as javascript/v8 in the Substreams runtime.

# What's working
Functional JS processing for up to 10k Ethereum blocks without crashing (needs more testing).

Use of a single reused isolate for each block, with fresh JS context.

JS scripts broken down into polyfill.js, prelude.js, bundle.js (user-code).

Seamless integration with substreams

# Known issues
OOM (Out-of-Memory) JavaScript V8

Symptom: Fatal javascript OOM in MemoryChunk allocation failed or Mark-Compact errors.

V8 context created at each block without GC.

Repeated isolate recreation.

Attempted solutions:

Add heap limits

Manage memory by flushing the JS heap manually at the end of a run

Segfault (SIGSEGV)

Cause: NewContext(nil) or context used after Dispose() of an isolate.

# Benchmarking
rust:
time substreams run -e localhost:10016 --plaintext --noop-mode substreams.yaml map_events -s 22463000 -t +10000

  0.56s user 0.33s system 3% cpu 28.464 total

JS:
time substreams run -e localhost:10016 --plaintext --noop-mode substreams.yaml map_events -s 22463000 -t +1000

  0.59s user 0.33s system 0% cpu 2:17.91 total

Ratio (ms) : 135600 / 29000 = 4.667 ~> 4x difference
