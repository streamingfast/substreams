# Substreams Packages

- [Packages](#packages)
    - [Definition](#definition)
    - [Where to find them](#where-to-find-them)

### Definition

Packages are single files containing all dependencies, protobuf
definitions (as FileDescriptors), compiled WASM code and modules tree
specifications. They allow you to start streaming right away!

Their conventional extension is `.spkg`.

They are protobuf-serialized files that use this model: https://github.com/streamingfast/substreams/blob/develop/proto/sf/substreams/v1/package.proto

You will notice that the conform to both https://buf.build [Images](https://docs.buf.build/reference/images) and standard Protobuf FileDescriptorSet, meaning they can be used with multiple code generation tools to scaffold.


### Where to find them

There is currently no single point of reference for Substreams
modules. See https://github.com/streamingfast/substreams-playground
for now.
