# DSN Reference

DSN stands for Data Source Name (or Database Source Name) and defines how to connect to your database. The `substreams sink postgres` / `substreams sink clickhouse` commands expect a URL input (via the `--dsn` flag or the `SUBSTREAMS_SINK_DSN` environment variable) that specifies the driver, connection details, and options.

## General Format

The generic format of a DSN is:

```
<scheme>://<username>:<password>@<hostname>:<port>/<database_name>?<options>
```

The URL's scheme determines the database driver to use (`postgres`, `clickhouse`, etc.).

## PostgreSQL

### DSN Format

```bash
postgres://<user>:<password>@<host>:<port>/<database>?<options>
# Alternative schemes also supported:
psql://<user>:<password>@<host>:<port>/<database>?<options>
```

### Examples

```bash
# Basic connection
postgres://myuser:mypassword@localhost:5432/mydatabase

# With SSL disabled
postgres://myuser:mypassword@localhost:5432/mydatabase?sslmode=disable

# With specific schema
postgres://myuser:mypassword@localhost:5432/mydatabase?schema=public

# Multiple options
postgres://myuser:mypassword@localhost:5432/mydatabase?sslmode=disable&schema=analytics
```

### Supported Options

PostgreSQL DSN supports all options from [libpq official documentation](https://www.postgresql.org/docs/current/libpq-connect.html#LIBPQ-PARAMKEYWORDS). Common options include:

- `sslmode`: SSL connection mode (`disable`, `require`, `verify-ca`, `verify-full`)
- `schema`: Select a particular schema within the database
- `connect_timeout`: Connection timeout in seconds
- `application_name`: Application name for connection tracking

{% hint style="warning" %}
Do not pass `user`, `password`, `host`, `port`, or `dbname` in the options query parameters as they are automatically extracted from the DSN URL.
{% endhint %}

## ClickHouse

### DSN Format

```bash
clickhouse://<user>:<password>@<host>:<port>/<database>?<options>
```

### Examples

```bash
# Local ClickHouse (unencrypted)
clickhouse://default:@localhost:9000/default

# ClickHouse with authentication
clickhouse://myuser:mypassword@localhost:9000/mydatabase

# ClickHouse Cloud (encrypted)
clickhouse://default:mypassword@abc123.clickhouse.cloud:9440/default?secure=true

# With additional options
clickhouse://myuser:mypassword@localhost:9000/mydatabase?compress=true&debug=true
```

### Port Configuration

| Connection Type | Port | Description |
|----------------|------|-------------|
| **Unencrypted** | `9000` | Standard ClickHouse native protocol port |
| **Encrypted (Cloud)** | `9440` | Native protocol SSL/TLS port for ClickHouse Cloud |
| **HTTP** | `8123` | HTTP interface (not supported by the SQL sink) |
| **HTTPS** | `8443` | HTTPS interface (not supported by the SQL sink) |

### Supported Options

- `secure`: Enable TLS encryption (`true`/`false`)
- `skip_verify`: Skip TLS certificate verification (`true`/`false`)
- `compress`: Enable compression (`true`/`false`)
- `debug`: Enable debug logging (`true`/`false`)

### ClickHouse Cloud Configuration

{% hint style="important" %}
For ClickHouse Cloud, you **must** use:
- Port `9440` (Native protocol SSL/TLS port)
- `secure=true` option
- Often `skip_verify=true` for certificate issues

Example: `clickhouse://default:password@instance-id.clickhouse.cloud:9440/default?secure=true`
{% endhint %}

## Environment Variable Substitution

DSN strings support environment variable substitution using the format `${VARIABLE_NAME}`:

```bash
# Using environment variables
export DB_USER="myuser"
export DB_PASS="mypassword"
export DSN="postgres://${DB_USER}:${DB_PASS}@localhost:5432/mydatabase"

substreams sink postgres ./substreams.yaml --dsn $DSN
```

Values are inserted as is, so a password with reserved characters (`/`, `?`, `#`, `%`, space) still needs URL encoding. Use `--password-envvar` instead to avoid that.

## Password from an Environment Variable

Leave the password out of the DSN and the sink reads it from the environment variable named by `--password-envvar`. The default is `POSTGRES_PASSWORD` for `substreams sink postgres` and `CLICKHOUSE_PASSWORD` for `substreams sink clickhouse`. The value is URL-encoded for you, so it can contain any character:

```bash
export CLICKHOUSE_PASSWORD='p@ss:w/rd#1'
substreams sink clickhouse ./substreams.yaml --dsn "clickhouse://default@localhost:9000/default"
```

A password written in the DSN takes priority, and the sink logs a warning when the variable is ignored.

## Troubleshooting

### Common Issues

**ClickHouse "read: EOF" error:**
- Add `?secure=true` to your DSN for ClickHouse Cloud
- Ensure you're using port `9440` for encrypted connections

**PostgreSQL connection refused:**
- Verify the port (usually `5432`)
- Check if `sslmode=disable` is needed for local development

**Authentication failures:**
- Ensure username and password are URL-encoded if they contain special characters, or pass the password with `--password-envvar`
- Verify database user has appropriate permissions

### URL Encoding

If your username, password, or database name contains special characters, they must be URL-encoded:

```bash
# Special characters in password
postgres://myuser:my%40password@localhost:5432/mydatabase

# Special characters in username  
postgres://my%2Buser:mypassword@localhost:5432/mydatabase
```
