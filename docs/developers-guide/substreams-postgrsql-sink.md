---
description: StreamingFast Substreams PostgreSQL sink
---

# `substreams-sink-postgres` introduction

### Purpose

This documentation exists to assist you in understanding and beginning to use the StreamingFast [`substreams-sink-postgres`](https://github.com/streamingfast/substreams-sink-postgres) tool. The Substreams module paired with this tutorial is a basic example demonstrating how to use Substreams and PostgreSQL together.

### Overview

The [`substreams-sink-postgres`](https://github.com/streamingfast/substreams-sink-postgres) tool provides the ability to pipe data extracted from a blockchain into a PostgreSQL database.

---

<b>DEV NOTES</b>

TODO: Go through this outine to compare and contrast it to what's currently in the sink files and sink kv documentation. We need to be as consistent as possible.

Here a first draft outline:

- Overview (discuss about transformation required to fit expected model, that the sink consumes this and populate a database.)
- Prepare your Substreams (how to respect database changes format, examples and explanations, how to check https://github.com/streamingfast/substreams-eth-block-meta for examples of the format).
- Dependencies requires (Docker compose to launch a local Postgres instance, schema population)
- Run and configure substreams-sink-postgres (launching, flags, output, inspect, results)
- Discussion about where Substreams cursor is saved (in a table)
- Discussion about batching of writes (each 1000 blocks when not live, still need to be determined when we will be live)
- Conclusion
