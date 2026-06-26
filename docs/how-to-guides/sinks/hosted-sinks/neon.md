# Hosted Sink: Neon

This walkthrough connects a Hosted Sink to a [Neon](https://neon.tech) serverless Postgres database.

## 1. Create a Neon Project

1. Sign in to [console.neon.tech](https://console.neon.tech) and click **New project**.
2. Choose a name, Postgres version, and a region close to StreamingFast's infrastructure.
3. Neon creates a default database (`neondb`) and branch (`main`) automatically.

## 2. Get Connection Credentials

In your project, go to the **Dashboard** and open the **Connection Details** panel. Select:
- **Branch**: `main`
- **Database**: your target database (default: `neondb`)
- **Role**: `neondb_owner` or a role you create

The connection string looks like:

```
postgres://<user>:<password>@<endpoint-host>.neon.tech/neondb?sslmode=require
```

Pull the individual fields from it:

| Field | Example |
|---|---|
| Host | `ep-quiet-forest-a1b2c3d4.us-east-2.aws.neon.tech` |
| Port | `5432` |
| Database | `neondb` |
| User | `neondb_owner` |
| Password | shown once — copy it now |

## 3. Create a Dedicated Role and Schema (Recommended)

Open the Neon **SQL Editor** and run:

```sql
CREATE ROLE substreams_sink WITH LOGIN PASSWORD 'a-strong-password';
CREATE SCHEMA IF NOT EXISTS substreams;
GRANT ALL PRIVILEGES ON SCHEMA substreams TO substreams_sink;
GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA substreams TO substreams_sink;
ALTER DEFAULT PRIVILEGES IN SCHEMA substreams GRANT ALL ON TABLES TO substreams_sink;
```

Replace `substreams` with your intended schema name.

## 4. Configure the Hosted Sink

In [The Graph Market](https://thegraph.market/sinks/new), create a new sink and fill in the **Output** section:

| Field | Value |
|---|---|
| Host | `ep-<name>.<region>.aws.neon.tech` |
| Port | `5432` |
| Database | `neondb` (or your database name) |
| Schema | `substreams` (or the schema you created above) |
| User | `substreams_sink` |
| Password | Password for that role |
| SSL Mode | `require` |

{% hint style="warning" %}
Neon endpoints **suspend after a period of inactivity** on the free tier. A Hosted Sink running continuously will keep the endpoint active, but if the sink is stopped for an extended period the first reconnect may be slow. Consider upgrading to a paid Neon plan for production workloads.
{% endhint %}

## 5. Deploy

Complete the remaining sink configuration (package, execution settings) and click **Deploy sink**. The sink will connect to your Neon database and begin writing data.

## Troubleshooting

**Endpoint suspended on first connect** — Neon free-tier endpoints auto-suspend. The sink will retry on reconnect but may log an initial connection error. This is transient; the endpoint wakes within a few seconds.

**SSL required** — Neon requires SSL. Set SSL Mode to `require`; `disable` will be rejected.

**Permission denied on new tables** — Run `ALTER DEFAULT PRIVILEGES IN SCHEMA substreams GRANT ALL ON TABLES TO substreams_sink;` so that tables created by the sink are automatically accessible to the role.
