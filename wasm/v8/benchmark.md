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
BenchmarkV8_RunJS_Separate-20    	     439	  11896017 ns/op	  245857 B/op	       4 allocs/op
BenchmarkV8_RunJS_Separate-20    	     627	   9786396 ns/op	  245804 B/op	       4 allocs/op
BenchmarkV8_RunJS_Separate-20    	     601	   9851238 ns/op	  245795 B/op	       4 allocs/op
BenchmarkV8_RunJS_Separate-20    	     591	   9162641 ns/op	  245795 B/op	       4 allocs/op
BenchmarkV8_RunJS_Separate-20    	     591	   9021966 ns/op	  245795 B/op	       4 allocs/op
BenchmarkV8_RunJS_Separate-20    	     620	   9194892 ns/op	  245795 B/op	       4 allocs/op
BenchmarkV8_RunJS_Separate-20    	     600	   9270623 ns/op	  245796 B/op	       4 allocs/op
BenchmarkV8_RunJS_Separate-20    	     597	   9353082 ns/op	  245794 B/op	       4 allocs/op
BenchmarkV8_RunJS_Separate-20    	     638	   9373087 ns/op	  245795 B/op	       4 allocs/op
BenchmarkV8_RunJS_Separate-20    	     613	   9183373 ns/op	  245794 B/op	       4 allocs/op
BenchmarkV8_RunJS_Concat-20      	     571	   9333131 ns/op	  237587 B/op	       2 allocs/op
BenchmarkV8_RunJS_Concat-20      	     592	   9445163 ns/op	  237587 B/op	       2 allocs/op
BenchmarkV8_RunJS_Concat-20      	     592	   9563747 ns/op	  237588 B/op	       2 allocs/op
BenchmarkV8_RunJS_Concat-20      	     565	  10969330 ns/op	  237587 B/op	       2 allocs/op
BenchmarkV8_RunJS_Concat-20      	     452	  12623718 ns/op	  237587 B/op	       2 allocs/op
BenchmarkV8_RunJS_Concat-20      	     448	  12730569 ns/op	  237586 B/op	       2 allocs/op
BenchmarkV8_RunJS_Concat-20      	     438	  12580124 ns/op	  237587 B/op	       2 allocs/op
BenchmarkV8_RunJS_Concat-20      	     458	  11558631 ns/op	  237588 B/op	       2 allocs/op
BenchmarkV8_RunJS_Concat-20      	     480	  12225078 ns/op	  237587 B/op	       2 allocs/op
BenchmarkV8_RunJS_Concat-20      	     555	  12014610 ns/op	  237586 B/op	       2 allocs/op
PASS
ok  	github.com/streamingfast/substreams/wasm/bench	114.437s

**Conclusion:** the **Separate** strategy is consistently faster (~7% median improvement) and uses ~40% less memory. The document retains the two-step loading as the default approach.
