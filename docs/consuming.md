# Consuming Substreams

What does it mean to consume Substreams?


## From your language

By generating some code and connecting directly to the streams.

See https://github.com/streamingfast/substreams-playground


## From a substreams-compatible _sink_ program

Like `substreams-mongo`, `substreams-postgres`, `substreams-kafka`, etc..


## From the `graph-node` in a Subgraph

Soon(TM), we hope to have an integration directly within The Graph's
`graph-node`, and have ways to declare the consumption of Substreams
in a Subgraph manifest.

Stay tuned.
