Check out the [Getting Started Guide](./intro-your-first-application.md) for more information on how to initialize your project. There are two options within `substreams init` to initialize your Injective Substreams:

- `injective-minimal`: creates a simple Substreams that extracts raw data from the block (generates Rust code).
- `injective-events`: creates a Substreams that extracts Injective events filtered by _type_ and/or _attributes_.

## Injective Foundational Modules

The `injective-events` codegen path relies on one of the [Injective Foundational Modules](https://github.com/streamingfast/substreams-foundational-modules/tree/develop/injective-common).  A Foundational Module extracts the most relevant data from blockchain, so that you don't have to code it yourself.

Specifically, the `injective-events` path uses the [filtered_events](https://github.com/streamingfast/substreams-foundational-modules/blob/develop/injective-common/substreams.yaml#L58) module from the Injective Foundational Modules to retrieve all the events filtered by event type and/or attributes.

