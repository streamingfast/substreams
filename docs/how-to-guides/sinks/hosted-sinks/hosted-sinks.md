# Hosted Sinks

{% hint style="warning" %}
**Hosted Sinks is currently in beta.** The service is under active development and changes may occur. To stay up to date with the latest information, join our [Discord Server](https://discord.gg/jZwqxJAvRs) and follow the **#announcements** channel.
{% endhint %}

**Hosted Sinks** is a managed service on [The Graph Market](https://thegraph.market) that runs your Substreams sink for you. Instead of provisioning servers, managing deployments, or operating sink processes yourself, you configure a sink in the portal and StreamingFast handles the infrastructure.

A hosted sink continuously reads data from your Substreams package and writes it to your own Postgres or ClickHouse database.

{% hint style="info" %}
Hosted Sinks is available to organizations on The Graph Market. Your database must be publicly reachable from StreamingFast's infrastructure.
{% endhint %}

## Requirements

Before creating a sink, you need:

- An account on [The Graph Market](https://thegraph.market) with an active organization.
- A Substreams package (`.spkg`) that outputs data using a `db_out` module or relational mappings. See [Substreams:SQL](../sql/sql.md) for how to build one.
- A running Postgres (default port `5432`) or ClickHouse (default port `9000`) database that is network-accessible from the internet.
- Your database schema already applied, or a schema that your Substreams package will create automatically. See [Sink Config](../../../references/sql/sink-config.md).

## Creating a Sink

Navigate to [**Sinks**](https://thegraph.market/sinks) in the sidebar and click **New Sink**.

![](<../../../assets/add-sinks-graph-market.png>)

### 1. Basics

Enter a **Sink name**. This is a human-readable label for the deployment (e.g., `mainnet-erc20-transfers`).

### 2. Substreams Package

Choose how you want to supply the `.spkg`:

| Source | When to use |
|---|---|
| **URL** | The package is hosted at a public URL (e.g., GitHub releases, IPFS). |
| **Substreams.dev** | The package is published on [the Substreams Registry](https://substreams.dev). Enter its ID (e.g., `ethereum_common@v0.3.3`). |

### 3. Output

Choose **Postgres** or **ClickHouse** and fill in the connection details. 
See walkthroughs provided below for different SQL services you can rely on, with guided setups to run your Hosted Sink.

**Postgres fields:**

| Field | Description |
|---|---|
| Host | Database hostname or IP address. |
| Port | Default `5432`. |
| Database | Database name. |
| Schema | Target schema. Default `public`. |
| User | Database user with write access. |
| Password | Password for the user. |
| SSL Mode | One of `disable`, `require`, `verify-ca`, or `verify-full`. |

**ClickHouse fields:**

| Field | Description |
|---|---|
| Host | ClickHouse hostname or IP address. |
| Port | Default `9000` (native protocol). |
| Database | Database name. Default `default`. |
| User | ClickHouse user. |
| Password | Password for the user. |
| Secure (TLS) | Enable TLS for the connection. |

{% hint style="warning" %}
Credentials are stored securely and used only to connect the sink process to your database. Use a dedicated database user with write access restricted to the target schema.
{% endhint %}

### 4. Execution

| Field | Description |
|---|---|
| Start block | Block number to start processing from. Use `0` to start from genesis. |
| Stop block | Block number to stop at. Use `0` to follow the chain head indefinitely. |
| Output module | The name of the Substreams module whose output is written to the database (e.g., `db_out`). |

**Advanced options** (optional):

| Field | Description |
|---|---|
| Filters | A filter expression passed to the Substreams execution. |
| Parameters | Module parameters passed at execution time. |

### 5. Deploy

Click **Deploy sink**. The sink enters the `Deploying` state while the infrastructure provisions. Once pods are ready it transitions to `Deployed` and begins indexing.

## Provider Walkthroughs

Step-by-step guides for connecting a Hosted Sink to common managed database providers:

**Postgres**
- [Supabase](./supabase.md) — managed Postgres with a generous free tier.
- [Neon](./neon.md) — serverless Postgres that scales to zero.

**ClickHouse**
- [ClickHouse Cloud](./clickhouse-cloud.md) — fully managed ClickHouse from the creators of ClickHouse.

## Next Steps

- [Monitor and manage your sink](./manage-your-sink.md) — check status, view logs, edit config, stop, or delete.
