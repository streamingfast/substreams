# Hosted Sink: Supabase

This walkthrough connects a Hosted Sink to a [Supabase](https://supabase.com) Postgres database.

## 1. Create a Supabase Project

1. Sign in to [supabase.com](https://supabase.com) and open your organization.
2. Click **New project**, choose a region close to StreamingFast's infrastructure, and set a strong database password.
3. Wait for the project to finish provisioning (roughly 1–2 minutes).

## 2. Get Connection Credentials

In your project, go to **Settings → Database → Connection parameters**.

| Supabase field | Value to copy |
|---|---|
| Host | `db.<project-ref>.supabase.co` |
| Port | `5432` |
| Database name | `postgres` (default) |
| Username | `postgres` (or a dedicated user you create) |
| Password | The password you set during project creation |

{% hint style="info" %}
Use the **direct connection** host (`db.<project-ref>.supabase.co`) rather than the Session or Transaction pooler hosts. The pooler ports (5432/6543 on `aws-0-<region>.pooler.supabase.com`) use PgBouncer, which can cause issues with the sink's prepared statements.
{% endhint %}

## 3. Create a Dedicated Database User (Recommended)

Run the following in the Supabase **SQL Editor** to create a user scoped to your target schema:

```sql
CREATE USER substreams_sink WITH PASSWORD 'a-strong-password';
CREATE SCHEMA IF NOT EXISTS substreams;
GRANT ALL PRIVILEGES ON SCHEMA substreams TO substreams_sink;
GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA substreams TO substreams_sink;
ALTER DEFAULT PRIVILEGES IN SCHEMA substreams GRANT ALL ON TABLES TO substreams_sink;
```

Replace `substreams` with your intended schema name.

## 4. Allow External Connections

Supabase Postgres accepts connections from the public internet by default. No additional firewall steps are needed for the Hosted Sink to reach your database.

## 5. Configure the Hosted Sink

In [The Graph Market](https://thegraph.market/sinks/new), create a new sink and fill in the **Output** section:

| Field | Value |
|---|---|
| Host | `db.<project-ref>.supabase.co` |
| Port | `5432` |
| Database | `postgres` |
| Schema | `substreams` (or the schema you created above) |
| User | `substreams_sink` |
| Password | Password for that user |
| SSL Mode | `require` |

Supabase requires SSL — use `require` at minimum. Use `verify-full` if you want certificate validation.

## 6. Deploy

Complete the remaining sink configuration (package, execution settings) and click **Deploy sink**. The sink will connect to your Supabase database and begin writing data.

## Troubleshooting

**Connection refused / timeout** — Confirm you are using the direct connection host, not the pooler host. Check that port `5432` is not blocked by any additional network policy in your Supabase project (**Settings → Database → Network restrictions**).

**Permission denied** — Ensure the sink user has been granted privileges on the target schema and that `ALTER DEFAULT PRIVILEGES` was applied so future tables are also accessible.

**SSL errors** — Supabase requires SSL. Set SSL Mode to `require` or higher; `disable` will be rejected.
