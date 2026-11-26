# Noop

`substreams sink noop` runs a module without receiving its output. The endpoint processes the blocks and fills its cache, and only cursors and progress come back to the client. The sink keeps its cursor in `--state-file` (`./state.cursor` by default), so it resumes where it stopped.

```bash
substreams sink noop ./my-substreams.spkg map_events -s 20000000 -e <endpoint>
```

## Warm up a new module version

Use the noop sink to switch to a new version of a module with no downtime:

1. Keep the current sink running on the current version of the module.
2. Start `substreams sink noop` on the new version, over the block range your real sink will need. The endpoint computes the new module's stores and outputs and caches them.
3. Once the noop sink is caught up with the chain head, start the real sink (for example, `substreams sink postgres`) on the new version against a new database or schema. It reads the cached data instead of waiting for the endpoint to process the history.
4. Once the new sink is live, point your application to it and stop the old one.

Cached data is most useful for modules with stores, which must be processed from their initial block before any output can be produced.

{% hint style="info" %}
**Tip**: Run [`substreams estimate`](./sinks.md#estimate-the-cost-first) first to see how many blocks the warm-up will process.
{% endhint %}
