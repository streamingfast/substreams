# Relational Mapping

If you want to use a relational model (e.g., creating one-to-many), you can annotate your Protobuf to indicate the primary and foreign keys in your database.

To map your Protobuf definitions directly to database tables and establish relationships between objects, you need to annotate your Protobuf messages with table names, primary keys, and relationship metadata.

{% hint style="warning" %} 
Relational mappings from Protobuf are currently in beta. Postgres support is stable, but ClickHouse support is still under development. [Reference releases](https://github.com/streamingfast/substreams-sink-sql/releases)
{% endhint %}

```
message Swap {
    option (sf.substreams.sink.sql.schema.v1.table) = {
        name: "swaps"
        child_of: "pools on id"
    };

    string id = 1;
    uint64 date = 2;
}

message Pool {
    option (sf.substreams.sink.sql.schema.v1.table) = { name: "pools" };

    string id = 1 [(sf.substreams.sink.sql.schema.v1.field) = {  primary_key: true}];
    string token_mint0 = 2;
    string token_mint1 = 3;
    repeated Swap swaps = 4;
}
```

In the example above, two entities are defined: `Swap` and `Pool`, where each Swap belongs to a Pool. As a result, the Pool object includes a list of Swaps, linked by the Pool’s _id using the child_of annotation. The SQL sink **will automatically generate the corresponding tables** and relationships based on the annotations in the Protobuf.

## Run the Sink

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

## Example: SPL Token

Let’s walk through a real-world example of storing SPL Token instructions in a Postgres database.

Clone the [SPL Token Substreams GitHub repository](https://github.com/streamingfast/substreams-spl-token).

### Inspect the Project

- Observe the `substreams.yaml` file:

```yaml
...

modules:
  - name: map_spl_instructions # 1.
    kind: map
    initialBlock: 158569587
    inputs:
      - params: string
      - map: solana_common:transactions_by_programid_and_account_without_votes
    output:
      type: proto:sf.solana.spl.v1.type.SplInstructions

network: solana

params:
  map_spl_instructions: "spl_token_address=4vMsoUT2BWatFweudnQM1xedRLfJgJ7hswhcpz4xgBTy|spl_token_decimal=9" # 2.
  solana_common:transactions_by_programid_and_account_without_votes: "program:TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA && account:4vMsoUT2BWatFweudnQM1xedRLfJgJ7hswhcpz4xgBTy"

sink: 
  module: map_spl_instructions # 3.
  type: sf.substreams.sink.sql.v1.Service
  config:
    dbt_config:
      files: ./dbt # 4.
      run_interval_seconds: 300
      enabled: false
```
1. The `map_spl_instructions` module maps Solana transactions to the output Protobuf, `SplInstructions`.
1. Configuration of the `map_spl_instructions` module. Here, you define the specific token you want to track and number of decimals to perform operations.
1. The `sink` section defines the SQL sink configuration. In this example, the sink will directly map `map_spl_instructions` to the tables of the database.
**The sink is able to infer the table names, so it is not necessary to provide a `schema.sql` file.**
1. DBT configuration to create materialized views on top of the inserted data.

**NOTE:** The `sink` block in the manifest is not necessary if you provide the module name when executing the SQL CLI command. For example: `substreams-sink-sql from-proto psql://.. substreams.yaml map_my_data`

- Observe the annotations in Protobuf:

```
message SplInstructions {
  repeated Instruction instructions = 1;
}

message Instruction {
  option (schema.table) = {
    name: "instructions"
  };

  string instruction_id =1 [(schema.field) = { primary_key: true }];
  string transaction_hash = 2;

  oneof Item {
    Mint mint = 10;
    Burn burn = 11;
    Transfer transfer = 12;
    InitializedAccount initialized_account = 13;
  }
}

message Transfer {
  option (parquet.table_name) = "transfers";
  option (schema.table) = {
    name: "transfers"
    child_of: "instructions on instruction_id"
  };

  string from = 2;
  string to = 3;
  double amount = 4;
}

message Mint {
  option (parquet.table_name) = "mints";
  option (schema.table) = {
    name: "mints"
    child_of: "instructions on instruction_id"
  };

  string to = 2;
  double amount = 3;
}
```
1. A root `Instruction` object is defined with table name `instructions`:
```
option (schema.table) = {
  name: "instructions"
};
```
1. An SPL Token instruction could be one of: `transfer`, `mint`, `burn` or `initialized_account`. A table for each of these possible instruction types will be created.
1. All these objects will have a foreign key relation with the root `instruction`, which is defined by the `child_of` relation. For example:
```
message Transfer {
  option (parquet.table_name) = "transfers";
  option (schema.table) = {
    name: "transfers"
    child_of: "instructions on instruction_id"
  };

  string from = 2;
  string to = 3;
  double amount = 4;
}
```

### Run the Sink

To run the sink, you will need a Postgres database. You can use a Docker container to spin one up on your computer.

- Define the `DSN` string, which will contain the credentials of the database.

```bash
export DSN=postgres://myuser:mypassword@localhost:5432/mydatabase?sslmode=disable
```

- Run the sink:

```bash
substreams-sink-sql from-proto $DSN ./substreams.yaml
```

{% hint style="note" %} 
For PostgreSQL users, you can run the sink without relations, no annotations required. The sink will infer the name of the table using the name of the messages that you output.
{% endhint %}
