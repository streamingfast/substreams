---
description: StreamingFast Substreams documentation
---

# 🔀 Substreams

### Substreams Documentation

#### Introducing StreamingFast Substreams

For a detailed walk-through of Substreams check out the video, created by StreamingFast's Alex, covering aspects of the product's history, design, and uses. The introduction video will help new Substresms users gain an understanding to provide a deeper context.

{% embed url="https://www.youtube.com/watch?v=qWxffTKpciU" %}
Learn about Substreams from StreamingFast's Alex.
{% endembed %}

### Substreams visual representation

Substreams has two perspectives as illustrated in the high-level visual diagram. One perspective is the architecture of and Substreams engine itself. The other perspective is from that of an end-user developer. &#x20;

Essentially the developer of an end-user application will design and create a data refinement strategy.&#x20;

The Substreams engine will use the data refinement strategy to isolate a very specific data set. Substreams receives data from StreamingFast Firehose in the form of streams.&#x20;

The streamed data is passed from Firehose through Substreams, then refined, and finally routed to wherever the developer desires, from relational databases to flat files, or even straight into an application.&#x20;

<img src=".gitbook/assets/substreams.excalidraw (1).svg" alt="StreamingFast Substreams high-level visual diagram" class="gitbook-drawing">

**Substreams in Detail**&#x20;

Substreams enables blockchain developers to write Rust modules that compose data streams alongside the community,

Substreams provides extremely high-performance indexing by virtue of parallelization, in a streaming-first fashion,

Substreams has all the benefits of the Firehose, like low-cost caching and archiving of blockchain data, high throughput processing, and cursor-based reorgs handling,

Substreams is the successor of [StreamingFast Sparkle](https://github.com/streamingfast/sparkle). The current Substreams iteration enables greater composability, provides similar powers of parallelization, and is also a _much_ simpler model to work with.

### Resources

* [Getting Started Guide](developer-guide/overview.md)
* Sample [Substreams Template](https://github.com/streamingfast/substreams-template) repository
* Additional examples available in the [Substreams Playground](https://github.com/streamingfast/substreams-playground)&#x20;

### Community

_**Need help? Reach out!**_

Substreams is an open-source community effort, so feel free to suggest new topics, report issues, and provide feedback wherever it could be helpful. Please contribute through GitHub [pull requests](https://docs.github.com/en/pull-requests/collaborating-with-pull-requests/proposing-changes-to-your-work-with-pull-requests/about-pull-requests).

Connect with the StreamingFast team on Discord!

* [StreamingFast Discord](https://discord.gg/jZwqxJAvRs)
* [The Graph Discord](https://discord.gg/vtvv7FP)
* [StreamingFast on Twitter](https://twitter.com/streamingfastio)
* [StreamingFast on YouTube](https://www.youtube.com/c/streamingfast)

### License

Substreams is currently licensed under [Apache 2.0](../LICENSE/).
