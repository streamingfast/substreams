---
description: StreamingFast Substreams prerequisites
---

# Prerequisites

**Essential Knowledge**

Substreams can be used by blockchain, subgraph, Rust, JavaScript, Python, and other types of developers.

Working with Substreams requires a myriad of knowledge, skills, and tools including:

* [Git](https://git-scm.com/),
* [YAML](https://yaml.org/),
* [Google Protocol Buffers](https://developers.google.com/protocol-buffers),
* Substreams CLI,
* Substreams engine,
* [Rust](https://www.rust-lang.org/),
* Buf,
* `protoc-gen-prost,`
* [CMake](https://cmake.org/install/) (_Linux only_),
* [Build essential](https://itsfoss.com/build-essential-ubuntu/) (_Linux only_),
* and [Graph Node](https://github.com/graphprotocol/graph-node) and [GraphQL](https://graphql.org/) for [subgraph](https://thegraph.com/docs/en/developing/creating-a-subgraph/) implementations.

{% hint style="info" %}
**Note**: [PostgreSQL](https://www.postgresql.org/), [MongoDB](https://www.mongodb.com/) and other backend knowledge may also be required when working with different sink types for Substreams.
{% endhint %}

In addition, developers are required to obtain a StreamingFast [authentication token](../reference-and-specs/authentication.md). The StreamingFast test endpoint servers require authentication when connecting to them to process Substreams requests.
