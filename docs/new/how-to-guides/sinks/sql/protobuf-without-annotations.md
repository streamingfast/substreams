
# Direct Mapping from Protobuf (Without Annotations)

The easiest way to map your Substreams output to SQL tables is to let the SQL sink handle everything for you. With this method, the sink automatically creates a table for each Protobuf message type you emit from your Substreams.

Consider the following Protobuf output of Substreams:

```protobuf
message Pool {
   string token0 = 1;
   string token1 = 2;
   uint64 created_at = 3;
}
```

The SQL sink will **automatically** create a table called `pools` with the corresponding columns, `token0`, `token1` and `created_at`. For every new `Pool` message outputted from the Substreams, a new row will be inserted into the table.

## Running the Sink

You can run the sink with the following syntax:

```bash
substreams-sink-sql from-proto <DSN> <SUBSTREAMS_PACKAGE>
```

The `substreams.yaml` file of your package must contain the sink configuration:

```yaml
sink:
  module: map_program_data
  type: sf.substreams.sink.sql.v1.Service
  config:
    engine: postgres
```

## Example: Pump Fun

Let’s walk through a real-world example of dumping all Pump.Fun data decoded via an IDL into a Postgres database.

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

...

network: solana-mainnet 

sink: # 2.
  module: map_program_data
  type: sf.substreams.sink.sql.v1.Service
  config:
    engine: postgres
```

1. The `map_program_data` module maps a Solana `Block` to the different instructions and events of the Pump.Fun IDL.
1. The `sink` section defines the SQL sink configuration. In this example, the sink will directly map `map_program_data` to the tables of the database.
**The sink is able to infer the table names, so it is not necessary to provide a `schema.sql` file.**

### Run the Sink

To run the sink, you will need a Postgres database. You can use a Docker container to spin up one in your computer.

- Define the `DSN` string, which will contain the credentials of the database.

```bash
export DSN=postgres://myuser:mypassword@localhost:5432/mydatabase?sslmode=disable
```

- Run the sink using `substreams-sink-sql from-proto`:

```bash
substreams-sink-sql from-proto $DSN ./substreams.yaml --no-proto-option
```

The `--no-proto-option` flag instructs the sink to infer the name of the tables.
