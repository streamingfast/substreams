Once you find a package that fits your needs, you can choose how you want to consume the data. Sinks are integrations that send the extracted data to a destination, such as a SQL database, a message queue, or files.

Most sinks are built into the [Substreams CLI](../cli/installing-the-cli.md) as `substreams sink <name>`. There is nothing else to install.

{% hint style="info" %}
**Tip**: Building a SQL sink? The [substreams-sql agent skill](../develop-your-own-substreams/generic/agent-skills.md) gives your AI coding assistant expert knowledge on Database Changes Mode (CDC) patterns, Relational Mappings Mode, PostgreSQL, and ClickHouse schema design.
{% endhint %}

{% hint style="info" %}
**Hosted Sinks**: Let StreamingFast run your PostgreSQL or ClickHouse sink for you, with no infrastructure to manage. See [Hosted Sinks](../hosted-services/hosted-sinks.md) and [Hosted Services](../hosted-services/hosted-services.md).
{% endhint %}

## Sinks

| Sink | How to run | Maintainer |
|------|------------|------------|
| [PostgreSQL](./sql/sql.md) | `substreams sink postgres` | StreamingFast |
| [ClickHouse](./sql/sql.md) | `substreams sink clickhouse` | StreamingFast |
| [PubSub](./pubsub.md) | `substreams sink pubsub` | StreamingFast |
| [Webhook](./webhook.md) | `substreams sink webhook` | StreamingFast |
| [ProtoJSON](./protojson.md) | `substreams sink protojson` | StreamingFast |
| [Noop](./noop.md) | `substreams sink noop` | StreamingFast |
| [Files](./files.md) (CSV, Parquet) | [substreams-sink-files](https://github.com/streamingfast/substreams-sink-files) | StreamingFast |
| [KV](./kv.md) | [substreams-sink-kv](https://github.com/streamingfast/substreams-sink-kv) | StreamingFast |
| [MongoDB](./mongodb.md) | [substreams-sink-mongodb](https://github.com/streamingfast/substreams-sink-mongodb) | StreamingFast |
| [Prometheus](./prometheus.md) | [substreams-sink-prometheus](https://github.com/pinax-network/substreams-sink-prometheus) (archived) | Pinax |
| CSV | [substreams-sink-csv](https://github.com/pinax-network/substreams-sink-csv) | Pinax |
| Webhook (JS) | [substreams-sink-webhook](https://github.com/pinax-network/substreams-sink-webhook) | Pinax |

## gRPC

To consume Substreams from your own application, stream the data over [gRPC](./grpc/grpc.md):

| Language | Library | Maintainer |
|----------|---------|------------|
| [Go](./grpc/go.md) | [substreams/sink](https://github.com/streamingfast/substreams/tree/develop/sink) | StreamingFast |
| [JavaScript](./grpc/javascript.md) | [substreams-js](https://github.com/substreams-js/substreams-js) | StreamingFast |
| Rust | [substreams-sink-examples/rust](https://github.com/streamingfast/substreams-sink-examples/tree/master/rust) | StreamingFast |
| Python | [substreams-sink-examples/python](https://github.com/streamingfast/substreams-sink-examples/tree/master/python) | StreamingFast |
| Java | [substreams-sink-examples/java](https://github.com/streamingfast/substreams-sink-examples/tree/master/java) | StreamingFast |

## Estimate the cost first

Before you sink a large block range, `substreams estimate` gives you the processed blocks and egress bytes it will cost. The endpoint runs a small sample of the range (`--sample-percentage`, 1% by default) and extrapolates from it. The module output never leaves the endpoint, so the estimate costs no egress:

```bash
substreams estimate ./substreams.yaml map_events -s 20000000 -t +1000000 -e <endpoint>
```

The result is an approximation. Use a block range that represents the activity you expect. Against an endpoint that does not support remote estimation, use `substreams estimate-local`, which samples from the client and only supports modules without stores.
