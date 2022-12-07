# Substreams Sinks

## **Introduction**

Blockchain data captured with Substreams can be persisted in databases and other types of storage. Working under the StreamingFasst methodology of development the databases are referred to as “sinks.”

Creating a solution to persist Substreams data consists of setting up a standard Substreams project with the addition of setting up the database environment, writing a schema to match the data output by Substreams, and running a specialized sink tool provided by StreamingFast. Sink tools are currently available for PostgreSQL, MongoDB, and file-based persistence.

## **Substreams Sink Basics**

Substreams developers author Rust-based modules that extract targeted blockchain data for persistence into a database. Additional information and example code for creating and working with standard Substreams modules are available in the documentation. An understanding of basic Substreams fundamentals is suggested before continuing.

An important consideration is the structure of the extracted information in the Substreams modules. There is a direct mapping between the structure of the database tables, their fields, and the structure of the data formed in the Substreams modules.

Using the example code, tooling, and following the prescription provided in the documentation is the best path to getting up and running with Substreams and database persistence.

## Sink Project Structure & Requirements

Persisting data extracted from the blockchain using Substreams to a database is a straightforward process, with a few caveats. Substreams developers must follow specific patterns and practices outlined in this documentation and the associated example.

## Substreams Sink Tools

StreamingFast provides several tools to assist Substreams developers interested in persisting data to databases; each can be found in its official GitHub repository.

**PostgreSQL**
https://github.com/streamingfast/substreams-sink-postgres

**MongoDB**
https://github.com/streamingfast/substreams-sink-mongodb

**File Based Storage**
https://github.com/streamingfast/substreams-sink-files

## DatabaseChanges Rust Crate

The StreamingFast DatabaseChanges Rust crate provides definitions for database changes that are emitted by Substreams. The DatabaseChanges Crate provides an assortment of functionality to ease the process of working with database-enabled Substreams development efforts. Find more information about the crate at the following link.

https://docs.rs/substreams-database-change/latest/substreams_database_change/pb/database/struct.DatabaseChanges.html

DatabaseChanges uses its protobuf definition. The protobuf definition can be viewed at the following link for a peek into the crates implementation.

https://github.com/streamingfast/substreams-database-change/blob/develop/proto/database/v1/database.proto

Full source code is provided by StreamingFast for the DatabaseChanges crate found in its official GitHub repository.

https://github.com/streamingfast/substreams-database-change

An output type of proto:substreams.database.v1.DatabaseChanges is required by the map module in the Substreams manifest when working with a sink.

## Database Schemas & Data Structures

The database schema requires a table named “cursors”. The cursors table needs to define columns for id, cursor, and block_num. The schema will also define one or more tables that match the output from the blockchain data extracted in the prescribed db_out Substreams map module.

The following code snippets illustrate an extremely simple database table schema definition and associated Rust map module data structure.

**Basic Rust data structure from Substreams map module example**

    BlockMeta {
    number: blk.number,
    hash: blk.hash,
    parent_hash: header.parent_hash,
    },

**Basic table schema definition for PostgreSQL matching map module data**

    create table block_meta
    (
    id  text not null constraint block_meta_pk primary key,
    at  text,
    number  integer,
    hash  text,
    parent_hash text,
    );

Database types currently supported by Substreams sink solutions include INTEGER, DOUBLE, BOOLEAN, TIMESTAMP, NULL. DATE, and STRING.

## Advanced Considerations & DeltaProto

Larger, more robust Substreams codebases use multiple modules of both types, map, and store. Data is extracted and processed in the map modules and passed to a store module to build up an aggregate collection of blockchain data. Store modules defined in the Substreams manifest output DatabaseChanges that map modules ingest.

Typed data defined through a custom protobuf is passed into the map module through a DeltaProto Vec in larger, production-type Substreams scenarios. DeltaProto is made available through the StreamingFast Substreams crate. DeltaProto isn’t specific to database-related Substreams development.

Find the details for DeltaProto in the Substreams Rust documentation at the following link.

https://docs.rs/substreams/latest/substreams/store/struct.DeltaProto.html

The substreams-eth-block-meta example demonstrates DeltaProto in action. Check out the source code in the project’s official GitHub repository.

https://github.com/streamingfast/substreams-eth-block-meta

## Substreams Sink Tutorial

_[_**_TODO_**_: Create new simple example that adds database persistence to the eth chain-agnostic example.]_

1. Clone and test chain-agnostic example.
   https://github.com/seanmooretechwriter/substreams-ethereum-tutorial

2. Run the chain-agnostic example and briefly explain what data is extracted. (Link to new chain agnostic example and doc page when published.)

3. Modify the map module name (to db_out) and output type (to DatabaseChanges) in the manifest

4. Create a database schema matching the data extracted in the eth chain-agnostic example.

5. Run the schema with PostgreSQL tools to make the database.

6. Install the substreams-sink-postgres tool.

7. Run the StreamingFast PostgreSQL sink took on the chain-agnostic example.

8. Provide a simple command line query with PostgreSQL to display persisted data in the database.
