# GraphQL Layer

{% hint style="warning" %}
**Hosted Sinks is currently in beta.** The service is under active development and changes may occur. To stay up to date with the latest information, join our [Discord Server](https://discord.gg/jZwqxJAvRs) and follow the **#announcements** channel.
{% endhint %}

The **GraphQL layer** serves a read-only GraphQL API over the database your [Hosted Sink](../../hosted-services/hosted-sinks.md) writes into. It is powered by [Hasura](https://hasura.io/) and lets you query your indexed data over HTTP, and browse it in a console, without standing up an API server of your own.

The layer is a separate deployment that reads the same database as the sink. You can add it or remove it at any time without touching the pipeline: the sink keeps writing exactly as before. In the portal it appears alongside the sink it serves, labeled **GraphQL**.

## What the API exposes

Hasura tracks every table and view in the sink's target schema and generates GraphQL fields for each one. Query fields are prefixed with the database name. The sink's own bookkeeping tables (`_cursor_`, `_blocks_`, `_sink_info_`) are excluded.

As the sink creates tables, the layer picks them up and tracks them. Views and other objects you add to the schema yourself are tracked the same way. The layer never creates or alters tables — it only reads.

The API is **read-only**. Every request is served with a role that has `SELECT` access and nothing else, so no mutation, insert, update, or delete fields are generated. The sink is the only writer to your database.

Both Postgres and ClickHouse sinks are supported.

## Adding the layer

### During sink creation

In the [New Sink](https://thegraph.market/sinks/new) wizard, the **GraphQL API** step comes after **Output selection**. Select **Expose a GraphQL API over this database** and continue.

### For an existing sink

Open the sink's detail page, click **Edit**, and go to the **GraphQL** tab. Select **Expose a GraphQL API over this database**, enter the database password the sink writes with, and click **Save**.

{% hint style="info" %}
The database password is requested again because stored credentials are never handed back out. The layer stages its own copy in the secret store.
{% endhint %}

The layer enters the `Deploying` state while it provisions, then transitions to `Deployed`.

## Using the API

The sink detail page has a **GraphQL** panel with the endpoint, an example request, and a link to the console. The endpoint has no path — append `/v1/graphql`, `/api/rest`, or `/console` depending on what you are calling.

### Authentication

Requests authenticate with one of your organization's API keys, passed in the `X-Api-Key` header. This is the same key used elsewhere on [The Graph Market](https://thegraph.market).

### Example query

```bash
curl https://<your-sink-endpoint>/v1/graphql \
  -H "X-Api-Key: <your-api-key>" \
  -d '{"query":"{ __typename }"}'
```

Replace `<your-sink-endpoint>` and `<your-api-key>` with the values from the GraphQL panel. The panel's **Copy with key** control copies the command with a working key already filled in.

### RESTified endpoints

A GraphQL query saved as a REST endpoint from the console is served under `/api/rest`, each at its own path, and authenticates the same way.

## The console

The console is a browser UI for exploring the schema and running queries. Open it from the **Explore** link in the GraphQL panel, or by appending `/console` to the endpoint.

The console signs you in with your existing portal session — there is no separate login or device code to confirm. Queries run against the same read-only API. The schema, sources, and permissions cannot be changed from the console; the one write it allows is saving a query as a REST endpoint.

## Removing the layer

Open the sink's detail page, click **Edit**, go to the **GraphQL** tab, clear **Expose a GraphQL API over this database**, and click **Save**.

The API and its console stop answering. Your database and the sink writing to it are untouched.

Deleting a sink removes its GraphQL layer first, so you never need to remove the layer by hand before deleting the sink.

## Troubleshooting

**The endpoint is not shown** — it appears once the layer finishes deploying. Check the layer's status on the sink detail page.

**Queries return an empty schema** — the layer is running but could not reach the database, or the sink has not created any tables yet. The GraphQL panel shows a message when the database connection failed; check that the sink is deployed and writing.

**`401` responses** — confirm the `X-Api-Key` header carries a current API key for the organization that owns the sink.

**A table is missing from the schema** — the layer tracks tables in the sink's target schema only, and skips the `_cursor_`, `_blocks_`, and `_sink_info_` bookkeeping tables. Confirm the table exists in that schema.
