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

**Repeated** isolate recreation.

Attempted solutions:

Add heap limits

Manage memory by flushing the JS heap manually at the end of a run

Segfault (SIGSEGV)

Cause: NewContext(nil) or context used after Dispose() of an isolate.

# Benchmarking

The vast majority of compute needed to execute javascript comes from loading and running the different needed libraries. The v8 runtime is pretty fast. The main optimization route I'm currently looking at is how we can scale and handle these files while keeping our overhead small.

For 1000 blocks

rust:
time substreams run -e localhost:10016 --plaintext --noop-mode substreams.yaml map_events -s 22463000 -t +1000

0.56s user 0.33s system 3% cpu 28.464 total

JS:
time substreams run -e localhost:10016 --plaintext --noop-mode substreams.yaml map_events -s 22463000 -t +1000

0.59s user 0.33s system 0% cpu 2:17.91 total

Ratio (ms) : 135600 / 29000 = 4.667 ~> 4x difference

for 10k blocks this translates to (approximately) 4.73 minutes for the rust version and 21 minutes for the JS.

This time is different when running on docker since we're in production mode and have access to the cache. When using the cache the difference between the two is negligible but when un-cached the difference is very noticeable.

On a clean instance you can see a 50-70% slower execution on the JS side than the rust-wasm execution, no cache, for a 1000 blocs. This trend is also true for a number of blocks n that converge to 10k blocks.

### Polyfill + Bundle Loading Benchmark

Benchmark comparing two strategies for loading JS scripts on a fresh context:

- **Separate**: load `polyfill.js` then `bundle.js` in two `RunScript` calls.
- **Concat**: concatenate the two files into one buffer and execute a single `RunScript`.

After 10 repeated runs (5 s each), we observe:

goos: linux
goarch: amd64
pkg: github.com/streamingfast/substreams/wasm/bench
cpu: 12th Gen Intel(R) Core(TM) i7-12700H
BenchmarkV8_RunJS_Separate-20    	     488	  10729938 ns/op	  139327 B/op	       2 allocs/op
BenchmarkV8_RunJS_Separate-20    	     616	   9960795 ns/op	  139282 B/op	       2 allocs/op
BenchmarkV8_RunJS_Separate-20    	     693	   8043474 ns/op	  139281 B/op	       2 allocs/op
BenchmarkV8_RunJS_Separate-20    	     600	   9468498 ns/op	  139281 B/op	       2 allocs/op
BenchmarkV8_RunJS_Separate-20    	     608	   9159027 ns/op	  139282 B/op	       2 allocs/op
BenchmarkV8_RunJS_Separate-20    	     687	   8497250 ns/op	  139281 B/op	       2 allocs/op
BenchmarkV8_RunJS_Separate-20    	     744	   7683086 ns/op	  139282 B/op	       2 allocs/op
BenchmarkV8_RunJS_Separate-20    	     752	   7763650 ns/op	  139281 B/op	       2 allocs/op
BenchmarkV8_RunJS_Separate-20    	     678	   8002928 ns/op	  139281 B/op	       2 allocs/op
BenchmarkV8_RunJS_Separate-20    	     732	   7609759 ns/op	  139282 B/op	       2 allocs/op
BenchmarkV8_RunJS_Concat-20      	     650	   9030096 ns/op	  237587 B/op	       2 allocs/op
BenchmarkV8_RunJS_Concat-20      	     637	   8901528 ns/op	  237586 B/op	       2 allocs/op
BenchmarkV8_RunJS_Concat-20      	     645	   8819324 ns/op	  237587 B/op	       2 allocs/op
BenchmarkV8_RunJS_Concat-20      	     618	   8755518 ns/op	  237586 B/op	       2 allocs/op
BenchmarkV8_RunJS_Concat-20      	     632	   8995774 ns/op	  237586 B/op	       2 allocs/op
BenchmarkV8_RunJS_Concat-20      	     615	   8969516 ns/op	  237587 B/op	       2 allocs/op
BenchmarkV8_RunJS_Concat-20      	     633	   8888080 ns/op	  237586 B/op	       2 allocs/op
BenchmarkV8_RunJS_Concat-20      	     658	   8622677 ns/op	  237587 B/op	       2 allocs/op
BenchmarkV8_RunJS_Concat-20      	     638	   8762887 ns/op	  237595 B/op	       2 allocs/op
BenchmarkV8_RunJS_Concat-20      	     598	   8770252 ns/op	  237587 B/op	       2 allocs/op
PASS
ok  	github.com/streamingfast/substreams/wasm/bench	112.985s

**Conclusion:** the **Separate** strategy is consistently faster (~7% median improvement) and uses ~40% less memory. The document retains the two-step loading as the default approach.
