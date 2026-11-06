# Sink Configuration

The Substreams SQL sink can be configured in multiple ways. While the sink configuration in the `substreams.yaml` file is optional, it can be useful for setting default parameters and advanced configurations.

## Optional Sink Configuration

The sink configuration in your `substreams.yaml` file is **optional**. You can run the sink without any configuration by specifying the module name directly in the CLI command:

```bash
substreams sink postgres ./substreams.yaml map_my_data --dsn $DSN
```

## Sink Configuration Options

If you choose to include sink configuration in your `substreams.yaml`, you can use the following structure:

```yaml
sink:
  module: map_program_data
  type: sf.substreams.sink.sql.v1.Service
  config:
    engine: postgres  # or clickhouse
    dbt_config:
      files: ./dbt
      run_interval_seconds: 300
      enabled: false
```

### Configuration Parameters

- **module**: The name of the Substreams module to consume
- **type**: Must be `sf.substreams.sink.sql.v1.Service`
- **config.engine**: Database engine (`postgres` or `clickhouse`)
- **config.dbt_config**: Optional DBT configuration for materialized views
  - **files**: Path to DBT files directory
  - **run_interval_seconds**: Interval for running DBT transformations
  - **enabled**: Whether DBT transformations are enabled

## CLI Override

When using the CLI, you can override or bypass the sink configuration entirely (use `sink clickhouse` instead of `sink postgres` when `config.engine` is `clickhouse`):

```bash
# Use sink config from substreams.yaml
substreams sink postgres ./substreams.yaml --dsn $DSN

# Override module name (ignores sink config)
substreams sink postgres ./substreams.yaml my_custom_module --dsn $DSN

# Use published package (no local config needed)
substreams sink postgres https://github.com/example/package.spkg --dsn $DSN
```

The CLI approach provides more flexibility and is often preferred for development and testing scenarios.
