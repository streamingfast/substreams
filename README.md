# Substreams

> Developer preview

Substreams is a powerful blockchain indexing technology, developed for The Graph Network.

It enables you to write Rust modules, composing data streams alongside the community, and provides extremely high performance indexing by virtue of parallelization, in a streaming-first fashion.


It has all the benefits of the Firehose, like low-cost caching and archiving of blockchain data, high throughput processing, and cursor-based reorgs handling.

Substreams is the successor of https://github.com/streamingfast/sparkle. This iteration enables greater composability, provides similar powers of parallelization, and is a much simpler model to work with.

## Documentation

Full documentation is accessible at https://substreams.streamingfast.io.

### Getting Started

* [Your First Stream](https://substreams.streamingfast.io/getting-started/your-first-stream)

### Concept & Fundamentals

* [Definition](https://substreams.streamingfast.io/concepts/definition)
* [Comparison](https://substreams.streamingfast.io/concepts/comparison)
* [Modules](https://substreams.streamingfast.io/concepts/modules)
  * [Inputs](https://substreams.streamingfast.io/concept-and-fundamentals/modules/inputs)
  * [Outputs](https://substreams.streamingfast.io/concept-and-fundamentals/modules/outputs)

### Developer Guide

* [Overview](https://substreams.streamingfast.io/developer-guide/overview)
* [Installation](https://substreams.streamingfast.io/developer-guide/installation-requirements)
* [Creating your Manifest](https://substreams.streamingfast.io/developer-guide/creating-your-manifest)
* [Creating Protobuf Schemas](https://substreams.streamingfast.io/developer-guide/creating-protobuf-schemas)
* [Setting Up Handlers](https://substreams.streamingfast.io/developer-guide/setting-up-handlers)
* [Writing Module Handlers](https://substreams.streamingfast.io/developer-guide/writing-module-handlers)
* [Running Your Substreams](https://substreams.streamingfast.io/developer-guide/running-substreams)
