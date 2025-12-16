---
description: Substreams Dev Container Reference
---

The Substreams Dev Container is a tool to help you build your first project. You can either run it remotely through Github codespaces or locally by cloning the [substreams-starter repository](https://github.com/streamingfast/substreams-starter?tab=readme-ov-file). Inside the Dev Container, the `substreams init` command sets up a code-generated Substreams project, allowing you to easily build a SQL-based solution for data handling.

## Prerequisites

- Ensure Docker and VS Code are up-to-date.

## First Navigating the Dev Container

Upon entering the Dev Container, you can either build or import your own `substreams.yaml` and associate modules within the minimal path, or opt for the automatically generated Substreams paths. Then running `Substreams Build` generates the Protobuf files.

- **Minimal**: Starts you with the raw block `.proto` and requires development. This path is intended for experienced users.
- **Non-Minimal**: Extracts filtered data using network-specific caches and Protobufs taken from corresponding foundational modules. This path generates a working Substreams out of the box.

To publish your work with the broader community, publish your `.spkg` to [Substreams registry](https://substreams.dev/) using:  

- `substreams registry login`
- `substreams registry verify` (to check if your package is ready for publishing)
- `substreams registry publish`

{% hint style="success" %}
**Tip**: If you run into any problems within the Dev Container, use the `help` command to access trouble shooting tools. 
{% endhint %}

## Building a Sink for Your Project

You can configure your Substreams project to query data from a SQL database:

Run `substreams codegen sql` for SQL-based queries. For more information on configuring a SQL sink, refer to the [SQL documentation](../how-to-guides/sinks/sql/sql.md).

## Common Errors

- When running locally, make sure to verify that all Docker containers are healthy by running the `dev-status` command. 
- If you input the wrong start-block while generating your project, navigate to the `substreams.yaml` to change the block number, then re-run `substreams build`.
