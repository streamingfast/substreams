# `db_out` module

If you require more control over the tables and the data that you want to store into the database, then creating a `db_out` module would be the best option.

You will create a new module, `db_out`, which maps the output of your Substreams to the [DatabaseChanges data model](https://docs.rs/substreams-database-change/latest/substreams_database_change/pb/database/struct.DatabaseChanges.html), which is a format that the SQL sink understands.

## Running the Sink

To index a `db_out` module, you will have to run two different commands: `substreams-sink-sql setup` to create the necessary tables from a given `schema.sql` file, and `substreams-sink-sql run` to perform the actual execution.

```bash
substreams-sink-sql setup <DSN> <SUBSTREAMS_PACKAGE>
```

The `substreams.yaml` file of your package must contain the sink configuration:

```yaml
sink:
  module: map_program_data
  type: sf.substreams.sink.sql.v1.Service
  config:
    engine: postgres
    schema: schema.sql
```

## Example: Pump.Fun

Consider that you want to dump all the Pump.Fun data decoded with an IDL into your database.

Clone the [Pump Fun Substreams GitHub repository](https://github.com/enoldev/pump-fun-substreams).

### Inpsect the Project

- Observe the `substreams.yaml` file:

```yaml
...

modules:
 - name: map_program_data # 1.
   kind: map
   initialBlock: 298724475
   inputs:
   - map: solana:blocks_without_votes
   output:
     type: proto:substreams.v1.program.Data
   blockFilter:
     module: solana:program_ids_without_votes
     query:
       string: program:6EF8rrecthR5Dkzon8Nwu78hRvfCKubJ14M5uBEwF6P

 - name: db_out # 2.
   kind: map
   initialBlock: 339837174
   inputs:
   - map: map_program_data
   output:
     type: proto:sf.substreams.sink.database.v1.DatabaseChanges

network: solana-mainnet 

sink: # 3.
  module: map_program_data
  type: sf.substreams.sink.sql.v1.Service
  config:
    engine: postgres
```

1. The `map_program_data` module maps a Solana `Block` to the different instructions and events of the Pump.Fun IDL.
1. The `db_out` module maps the output of `map_program_data` to `DatabaseChanges`, a format that the SQL sink can understand.
1. The `sink` section defines the SQL sink configuration. In this example, the sink will map `db_out` to the tables of the database.
**When using a `db_out` module, it is necessary to specify a `schema.sql` file**

### Run the Sink

To run the sink, you will need a Postgres database. You can use a Docker container to spin up one in your computer.

- Define the `DSN` string, which will contain the credentials of the database.

```bash
export DSN=postgres://myuser:mypassword@localhost:5432/mydatabase?sslmode=disable
```

- Configure the sink to create the necessary tables.

```bash
substreams-sink-sql setup $DSN ./substreams.yaml
```

- Run the sink

```bash
substreams-sink-sql run $DSN ./substreams.yaml
```
