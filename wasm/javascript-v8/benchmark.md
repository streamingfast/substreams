# Substreams Javascript runtime integration 
This project aims to integrate the V8 JavaScript engine into Substreams in order to execute Substream modules written in JavaScript/TypeScript. The V8 engine is integrated via the v8go library, and replaces traditional WASM execution.

# V8 engine main files
module.go: Loads and executes JS scripts (polyfill, prelude, bundle).

instance.go: Manages V8 execution contexts (V8Instance).

factory.go: Creates a V8 module from JavaScript code (precompiled).

integration.go: Registers the engine as javascript-v8 in the Substreams runtime.

# What's working
Functional JS processing for up to ~6000 Ethereum blocks without crashing.

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
