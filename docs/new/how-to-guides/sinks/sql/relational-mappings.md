# Relational Mapping

If you want to use a relational model (e.g., creating one-to-many), you can annotate your Protobuf to indicate the primary and foreign keys in your database.

To map your Protobuf definitions directly to database tables and establish relationships between objects, you need to annotate your Protobuf messages with table names, primary keys, and relationship metadata.

{% hint style="warning" %} 
Relational mappings from Protobuf are currently in beta. Postgres support is stable, but ClickHouse support is still under development. [Reference releases](https://github.com/streamingfast/substreams-sink-sql/releases)
{% endhint %}

```proto
syntax = "proto3";

import "sf/substreams/sink/sql/schema/v1/schema.proto";

message Swap {
    option (schema.table) = {
        name: "swaps"
        child_of: "pools on id"
    };

    string id = 1;
    uint64 date = 2;
}

message Pool {
    option (schema.table) = { name: "pools" };

    string id = 1 [(schema.field) = { primary_key: true }];
    string token_mint0 = 2;
    string token_mint1 = 3;
    repeated Swap swaps = 4;
}
```

In the example above, two entities are defined: `Swap` and `Pool`, where each Swap belongs to a Pool. As a result, the Pool object includes a list of Swaps, linked by the Pool’s _id using the child_of annotation. The SQL sink **will automatically generate the corresponding tables** and relationships based on the annotations in the Protobuf.

## Run the Sink

First, let's spin up a PostgreSQL database using Docker:

```bash
docker run --name postgres-db -e POSTGRES_PASSWORD=password -e POSTGRES_DB=substreams -p 5432:5432 -d postgres:15
```

You can run the sink with the following syntax:

```bash
export DSN="postgres://postgres:password@localhost:5432/substreams?sslmode=disable"
substreams-sink-sql from-proto $DSN https://github.com/streamingfast/substreams-spl-token/releases/download/v0.1.0/solana-spl-token-v0.1.0.spkg
```

### Database Connection (DSN)

The DSN (Data Source Name) defines how to connect to your database. The format varies by database type:

**PostgreSQL:**
```bash
postgres://<user>:<password>@<host>:<port>/<database>?<options>
```

**ClickHouse:**
```bash
# Not encrypted
clickhouse://<user>:<password>@<host>:9000/<database>?<options>

# Encrypted (ClickHouse Cloud)
clickhouse://<user>:<password>@<host>:9440/<database>?secure=true&skip_verify=true&<options>
```

{% hint style="info" %}
For ClickHouse Cloud, use port `9440` with `secure=true` option. The standard port `9000` is for unencrypted connections.
{% endhint %}

For complete DSN format details and additional database options, see the [DSN Reference](../../../references/sql/dsn-reference.md).

## Example: SPL Token

Let’s walk through a real-world example of storing SPL Token instructions in a Postgres database.

Clone the [SPL Token Substreams GitHub repository](https://github.com/streamingfast/substreams-spl-token).

### Inspect the Project

- Observe the `substreams.yaml` file:

```yaml
specVersion: v0.1.0
package:
  name: solana-spl-token
  version: v0.1.0
  url: https://github.com/streamingfast/substreams-spl-token

imports:
  solana_common: https://github.com/streamingfast/substreams-foundational-modules/releases/download/solana-common-v0.3.0/solana-common-v0.3.0.spkg

protobuf:
  files:
    - sf/solana/v1/spl/type/spl.proto
  descriptorSets:
    - module: buf.build/streamingfast/substreams-sink-sql
  importPaths:
    - ./proto

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
```
1. The `map_spl_instructions` module maps Solana transactions to the output Protobuf, `SplInstructions`.
2. Configuration of the `map_spl_instructions` module. Here, you define the specific token you want to track and number of decimals to perform operations.

**The sink is able to infer the table names, so it is not necessary to provide a `schema.sql` file.**

- Observe the annotations in Protobuf:

```proto
syntax = "proto3";

import "google/protobuf/descriptor.proto";
import "sf/substreams/sink/sql/schema/v1/schema.proto";

package sf.solana.spl.v1.type;

message SplInstructions {
  repeated Instruction instructions = 1;
}

message Instruction {
  option (schema.table) = {
    name: "instructions"
  };

  string instruction_id = 1 [(schema.field) = { primary_key: true }];
  string transaction_hash = 2;

  oneof Item {
    Mint mint = 10;
    Burn burn = 11;
    Transfer transfer = 12;
    InitializedAccount initialized_account = 13;
  }
}

message Transfer {
  option (schema.table) = {
    name: "transfers"
    child_of: "instructions on instruction_id"
  };

  string from = 2;
  string to = 3;
  double amount = 4;
}

message Mint {
  option (schema.table) = {
    name: "mints"
    child_of: "instructions on instruction_id"
  };

  string to = 2;
  double amount = 3;
}
```
1. A root `Instruction` object is defined with table name `instructions`:
```proto
option (schema.table) = {
  name: "instructions"
};
```
2. An SPL Token instruction could be one of: `transfer`, `mint`, `burn` or `initialized_account`. A table for each of these possible instruction types will be created.
3. All these objects will have a foreign key relation with the root `instruction`, which is defined by the `child_of` relation. For example:
```proto
message Transfer {
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
export DSN="postgres://postgres:password@localhost:5432/substreams?sslmode=disable"
```

- Run the sink using the published package:

```bash
substreams-sink-sql from-proto $DSN https://github.com/streamingfast/substreams-spl-token/releases/download/v0.1.0/solana-spl-token-v0.1.0.spkg
```

Alternatively, if you have cloned the repository locally, you can run:

```bash
substreams-sink-sql from-proto $DSN ./substreams.yaml
```

## Run the Sink Without Relations

{% hint style="note" %} 
For PostgreSQL users, you can run the sink without relations, no annotations required. The sink will infer the name of the table using the name of the messages that you output.
{% endhint %}

When you run the sink without explicit table annotations, it automatically infers table structures using the following rules:

### Default Table Inference

**1. Single Table from Root Message**

If your output message contains no repeated fields of message types, the entire message becomes a single table:

```proto
message PoolCreated {
    string pool_address = 1;
    string token0 = 2;
    string token1 = 3;
    uint64 block_number = 4;
}
```

This creates a table named `pool_created` where each Substreams output message becomes one row.

**2. Multiple Tables from Repeated Fields**

If your output message contains repeated fields of message types, each repeated field becomes a separate table:

```proto
message BlockEvents {
    repeated SwapEvent swaps = 1;
    repeated MintEvent mints = 2;
    repeated BurnEvent burns = 3;
}

message SwapEvent {
    string pool = 1;
    string amount0 = 2;
    string amount1 = 3;
}

message MintEvent {
    string pool = 1;
    string liquidity = 2;
}
```

This creates three tables: `swaps`, `mints`, and `burns`. All swap events from all processed blocks are collected into the `swaps` table, and so on.

The table names are automatically derived from the field names by converting them to snake_case. For example, `SwapEvent` becomes `swap_event`, but when used as a repeated field name like `swaps`, it uses the field name directly.
