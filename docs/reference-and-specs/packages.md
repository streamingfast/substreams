---
description: StreamingFast Substreams packages reference
---

# Packages

A Substreams _package_ is a **single file** containing all dependencies, `Protobuf` definitions (as FileDescriptors), compiled WASM code, and the modules tree specifications.&#x20;

Packages allow developers to begin streaming immediately!

The standard file extension for a Substreams package is `.spkg`.

Substreams packages are protobuf-serialized files. See the [example model](../../proto/sf/substreams/v1/package.proto) in the official Github repository for an example.

The Substreams packages conform to both [https://buf.build](https://buf.build) [Images](https://docs.buf.build/reference/images) and the standard protobuf FileDescriptorSet. This means Substreams packages can be used with multiple code generation tools as a source for schema definitions.

### Creating packages

Packages are created using the `substreams pack` command and passing the Substreams manifest file.

```
substreams pack ./substreams.yaml
```

### Dependencies

When `imports` is defined in a new `substreams.yaml`, it can load modules and protobuf definitions from _other_ Substreams packages.

_**Note, local protobuf filenames take precedence over the imported package's proto files in this situation**._&#x20;

To avoid conflicts it's important to use unique `.proto` filenames. It's also important to use namespaces with fully qualified paths. These efforts help avoid potential naming collisions.

### Where to find them

See the [Substreams Playground](https://github.com/streamingfast/substreams-playground) for examples.
