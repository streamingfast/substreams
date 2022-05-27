# Running Your Substreams

You can run substreams directly from the command line, using `substreams run`. See the [reference doc for its usage](../reference-and-specs/using-the-cli.md#run).

## From your language

By generating some code and connecting directly to the streams.

See [https://github.com/streamingfast/substreams-playground](https://github.com/streamingfast/substreams-playground)

In particular this Python example: [https://github.com/streamingfast/substreams-playground/tree/master/consumers/python](https://github.com/streamingfast/substreams-playground/tree/master/consumers/python)

## From a substreams-compatible _sink_ program

Like `substreams-mongo`, `substreams-postgres`, `substreams-kafka`.

## From the `graph-node` in a Subgraph

Soon(TM), we hope to have an integration directly within The Graph's `graph-node`, and have ways to declare the consumption of Substreams in a Subgraph manifest.

This would enable all of the query layer of Subgraphs to benefit from the power of Substreams as a transformation layer.

Stay tuned.
