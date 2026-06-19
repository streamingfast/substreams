# Hosted Sink: ClickHouse Cloud

This walkthrough connects a Hosted Sink to a [ClickHouse Cloud](https://clickhouse.com/cloud) managed ClickHouse service.

## 1. Create a ClickHouse Cloud Service

1. Sign in to [clickhouse.cloud](https://clickhouse.cloud) and open your organization.
2. Click **New service**, choose a cloud provider and region close to StreamingFast's infrastructure, and select a service tier. Hosted Sinks are provided from us-central1 in Iowa.
3. Set the **default user** password during creation — save it immediately, it is only shown once.
4. Wait for the service to reach **Running** status (typically 1–3 minutes).

## 2. Get Connection Credentials

In your service, go to **Connect → Native (TCP)**.

| Field | Example |
|---|---|
| Host | `abc123def456.us-east-1.aws.clickhouse.cloud` |
| Port | `9440` (native TLS) |
| Database | `default` |
| User | `default` |
| Password | Set during service creation |

{% hint style="info" %}
ClickHouse Cloud exposes the native protocol on port **9440** (TLS) rather than the standard `9000`. Enable **Secure (TLS)** in the sink configuration.
{% endhint %}

## 3. Open the IP Access List

By default, ClickHouse Cloud restricts inbound connections. You must allow StreamingFast's infrastructure to reach your service. Hosted Sinks runs in **us-central1 (Iowa)**.

1. In your service, go to **Settings → Security → IP Access List**.
2. Click **Add entry** and add `0.0.0.0/0` to allow connections from any IP. Note, if you require a restricted IP range, please contact us at support@streamingfast.io.
3. Save the access list.

## 4. Create a Dedicated User and Database (Recommended)

Connect to your service using the ClickHouse Cloud SQL console (**Connect → SQL console**) and run:

```sql
CREATE DATABASE IF NOT EXISTS substreams;

CREATE USER substreams_sink
  IDENTIFIED BY 'a-strong-password';

GRANT SELECT, INSERT, CREATE TABLE, DROP TABLE, ALTER, TRUNCATE
  ON substreams.*
  TO substreams_sink;
```

Replace `substreams` with your intended database name.

## 5. Configure the Hosted Sink

In [The Graph Market](https://thegraph.market/sinks/new), create a new sink and fill in the **Output** section:

| Field | Value |
|---|---|
| Host | `<hash>.<region>.aws.clickhouse.cloud` |
| Port | `9440` |
| Database | `substreams` (or your database name) |
| User | `substreams_sink` |
| Password | Password for that user |
| Secure (TLS) | **Enabled** |

## 6. Deploy

Complete the remaining sink configuration (package, execution settings) and click **Deploy sink**. The sink will connect to your ClickHouse Cloud service and begin writing data.

## Troubleshooting

**Connection refused** — Confirm the IP Access List includes StreamingFast's egress IPs or is set to allow `0.0.0.0/0`. Also confirm port `9440` is selected, not `9000`.

**Authentication failed** — Double-check the user and password. ClickHouse Cloud passwords are case-sensitive. If you reset the default user password in the console, the old credential is immediately invalidated.

**TLS handshake error** — Ensure **Secure (TLS)** is enabled. ClickHouse Cloud's native port `9440` requires TLS; connections without it will fail.

**Database or table not found** — Ensure the sink user has been granted `CREATE TABLE` on the target database so the sink can initialize the schema on first run.
