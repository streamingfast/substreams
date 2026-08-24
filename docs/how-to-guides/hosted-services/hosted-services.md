---
description: Overview of the Substreams services StreamingFast hosts for you
---

# Hosted Services

**Hosted Services** are managed Substreams deployments that StreamingFast runs for you on [The Graph Market](https://thegraph.market). You configure a service in the portal; StreamingFast provisions the runners, keeps them online, and scales them. You do not operate the sink process or the store server yourself.

{% hint style="warning" %}
Hosted Services are currently in beta. To stay up to date, join our [Discord](https://discord.gg/jZwqxJAvRs) and follow the **#announcements** channel.
{% endhint %}

There are two kinds of hosted service. Both are created from **Hosted Services** in The Graph Market — [create a new service](https://thegraph.market/sinks/new) or [list the ones you already have](https://thegraph.market/sinks).

| Kind | What StreamingFast runs | What you bring |
|---|---|---|
| **Hosted Sink** | A Substreams sink that writes package output into a SQL database | A Postgres or ClickHouse database you already operate |
| **Hosted Store** | A block-aware key/value store, queryable over gRPC and from Substreams modules | Values to store — either you push them, or a Substreams package reads the chain and fills the store |

## Hosted Sink

A hosted sink continuously reads a Substreams package and writes the output into **your** Postgres or ClickHouse database. StreamingFast hosts the sink runner only. It does not create, host, or manage the database.

Use a hosted sink when you want SQL in a database you already run (or a managed one such as Supabase, Neon, or ClickHouse Cloud) without operating `substreams-sink-sql` yourself.

Supported engines are **PostgreSQL** and **ClickHouse**, in Database Changes Mode or Relational Mappings Mode. See [Substreams:SQL](../sinks/sql/sql.md).

The database must be reachable from StreamingFast's network (a public hostname, not `localhost` or a private Docker name).

**How-to:** [Hosted Sinks](hosted-sinks.md) — create a sink, connect Postgres or ClickHouse, then [manage it](../sinks/hosted-sinks/manage-your-sink.md). Walkthroughs: [Supabase](../sinks/hosted-sinks/supabase.md), [Neon](../sinks/hosted-sinks/neon.md), [ClickHouse Cloud](../sinks/hosted-sinks/clickhouse-cloud.md).

If you would rather run the sink on your own infrastructure, see [Consuming Substreams](../sinks/sinks.md).

## Hosted Store

A hosted store is a managed, block-aware key/value map. Keys are raw bytes; values are protobuf messages. Reads target a block height, so a Substreams module that queries the store stays in sync with the stream.

The create wizard offers two feed modes:

| Mode | Who writes | When to use | How-to |
|---|---|---|---|
| **Remote feed** | You (or another service) push entries over gRPC with `Feed.Set` | Off-chain or application-owned data that Substreams should look up | [Remote Feed Hosted Store Guide](hosted-store-remote-feed.md) |
| **Substreams feed** | A Substreams package reads the chain and populates the store | On-chain lookups other modules should reuse (the same idea as a [foundational store](../composing-substreams/foundational-stores/foundational-stores.md)) | [Substreams Feed Hosted Store Guide](hosted-store-substreams-feed.md) |

Once deployed, the store is reachable at `<deployment-id>.hs.streamingfast.io:443` and is referenced from a Substreams manifest as `foundational-store: <deployment-id>@<version>`.

To run a foundational store yourself instead of using a StreamingFast-managed one, see [Hosting a Foundational Store](../../references/foundational-stores/hosting-foundational-stores.md).

## Create a service

1. Sign in to [The Graph Market](https://thegraph.market) with an organization.
2. Open [**Hosted Services**](https://thegraph.market/sinks) and click **New Service**.
3. Choose **Sink** or **Hosted Store**, then follow the wizard.

An API key is created automatically for each deployment. Manage keys at [https://thegraph.market/api-keys](https://thegraph.market/api-keys).

## Related resources

- [Hosted Sinks](hosted-sinks.md) — managed SQL sinks
- [Remote Feed Hosted Store Guide](hosted-store-remote-feed.md) — write and read a hosted store over gRPC
- [Substreams Feed Hosted Store Guide](hosted-store-substreams-feed.md) — populate a hosted store from a Substreams package
- [Foundational Stores](../composing-substreams/foundational-stores/foundational-stores.md) — chain-specific stores populated by a Substreams package
- [Consuming Substreams](../sinks/sinks.md) — self-managed sinks (SQL, stream, PubSub, files, and community sinks)
