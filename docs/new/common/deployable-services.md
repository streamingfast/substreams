The Substreams Deployable Services define a common interface to easily deploy your Substreams to one of the supported sinks, such as SQL or subgraphs. Essentially, it facilitates sending data to a variety of sinks by simply using the Substreams CLI.

## Hoes Does It Work?

1. Choose what sink you want to use (SQL or subgraphs).
2. Add the `sink` configuration to your manifest.
3. Use the `substreams alpha service` command to deploy, stop or remove your services.

### Choose a Sink

Depending on your needs, you must choose how you want to consume the data: using a SQL database or a subgraph. Substreams using the SQL sink must have a `db_out` module and those using the subgraph sink must have a `graph_out` module.

### Add the Sink Configuration

The `sink` configuration in a Substreams manifest defines what sink should be used. To get more information about the Substreams manifest, refer to the [Manifest & Modules page](manifest-modules.md)

Every sink has different configuration fields available, so check out the Manifest Reference for more information. In the following example, a SQL sink is defined:

```
sink:
  module: db_out
  type: sf.substreams.sink.sql.v1.Service
  config:
    schema: "./schema.sql"
    engine: clickhouse
    postgraphile_frontend:
      enabled: false
    pgweb_frontend:
      enabled: false
    dbt_config:
      enabled: true
      files: "./path/to/folder"
      run_interval_seconds: 300
```

### Deploy the Service

Use the `substreams alpha service <COMMAND>` command to manage your services. Once your Substreams has the corresponding manifest configuration, you can deploy it by using the `substreams alpha service deploy` command.

You will get a service ID, which is a unique identifier for your service. This will allow you to manage your service and apply actions to it, such a stopping or removing it.